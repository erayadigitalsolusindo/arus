package sales

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/shopspring/decimal"

	"aciraba/internal/audit"
	"aciraba/internal/authz"
	gen "aciraba/internal/gen"
	"aciraba/internal/platform/db"
)

// EraPayReconciler = penjadwal rekonsiliasi nota yang disimpan paksa (erapay_pending) ke EraPay. Tiap beberapa menit ia
// membaca daftar charge lintas tenant yang masih menunggu (erapay_reconcile_targets, SECURITY DEFINER agar role aplikasi
// bisa menyebut tenant), lalu memroses satu per satu di dalam transaksi RLS tenant-nya sendiri:
//   - EraPay menjawab lunas   → penanda "perlu dicek" dihapus (sale.erapay_paid).
//   - EraPay menjawab gagal/batal/kedaluwarsa → nota di-VOID otomatis: stok kembali (sale.erapay_unpaid + sale.void).
//   - EraPay tidak terjangkau → dibiarkan, dicek lagi putaran berikutnya (tidak pernah diputus otomatis oleh status lama).
//
// Aman dijalankan di beberapa instance: pollution dihindari dengan kunci baris (FOR UPDATE) pada charge & nota.
type EraPayReconciler struct {
	svc  *Service
	pool *pgxpool.Pool
	log  *slog.Logger
	now  func() time.Time

	// OnlyTenant != uuid.Nil membatasi rekonsiliasi ke satu tenant (test dan alat operasi).
	OnlyTenant uuid.UUID
}

const erapayReconcileInterval = 2 * time.Minute

func NewEraPayReconciler(svc *Service, pool *pgxpool.Pool, log *slog.Logger) *EraPayReconciler {
	return &EraPayReconciler{svc: svc, pool: pool, log: log, now: time.Now}
}

// Run berjalan sampai ctx selesai.
func (s *EraPayReconciler) Run(ctx context.Context) {
	t := time.NewTicker(erapayReconcileInterval)
	defer t.Stop()
	s.log.Info("rekonsiliasi EraPay aktif", "interval", erapayReconcileInterval.String())
	for {
		s.Tick(ctx)
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
	}
}

type erapayTarget struct {
	tenant, outlet, saleID, chargeID uuid.UUID
	paymentID                        string
	docNo                            string
	amount                           decimal.Decimal
}

// Tick memproses semua charge yang perlu dicek. Mengembalikan jumlah yang dapat diputus (lunas → penanda dihapus,
// tidak lunas → divoid). Untuk test dan alat operasi.
func (s *EraPayReconciler) Tick(ctx context.Context) (checked int) {
	if s.svc.payVerifier == nil {
		return 0
	}
	rows, err := s.pool.Query(ctx, `SELECT tenant_id, outlet_id, sale_id, charge_id, payment_id, doc_no, amount FROM erapay_reconcile_targets()`)
	if err != nil {
		s.log.Warn("rekonsiliasi EraPay: gagal membaca daftar target", "err", err)
		return 0
	}
	var targets []erapayTarget
	for rows.Next() {
		var t erapayTarget
		if err := rows.Scan(&t.tenant, &t.outlet, &t.saleID, &t.chargeID, &t.paymentID, &t.docNo, &t.amount); err != nil {
			rows.Close()
			s.log.Warn("rekonsiliasi EraPay: baris target tidak terbaca", "err", err)
			return 0
		}
		targets = append(targets, t)
	}
	rows.Close()
	if rows.Err() != nil {
		s.log.Warn("rekonsiliasi EraPay: gagal membaca daftar target", "err", rows.Err())
		return 0
	}
	for _, t := range targets {
		if ctx.Err() != nil {
			return checked
		}
		if s.OnlyTenant != uuid.Nil && s.OnlyTenant != t.tenant {
			continue
		}
		start := s.now()
		if err := s.process(ctx, t); err != nil {
			s.log.Warn("rekonsiliasi EraPay gagal", "tenant", t.tenant, "sale", t.saleID, "charge", t.chargeID, "err", err)
			continue
		}
		s.log.Info("rekonsiliasi EraPay: diproses", "tenant", t.tenant, "sale", t.saleID, "doc_no", t.docNo, "durasi", time.Since(start))
		checked++
	}
	return checked
}

func (s *EraPayReconciler) actor(t erapayTarget) authz.Actor {
	return authz.Actor{TenantID: t.tenant, OutletID: t.outlet, Name: "Rekonsiliasi EraPay",
		Perms: authz.ParseStored([]byte(`{"*":true}`))}
}

// process memutuskan nasib satu charge yang perlu dicek: sudah lunas → penanda dihapus; gagal/batal/kedaluwarsa → nota
// di-VOID otomatis (stok kembali). Status belum-akhir → tidak melakukan apa-apa.
func (s *EraPayReconciler) process(ctx context.Context, t erapayTarget) error {
	return db.WithTenant(ctx, s.pool, t.tenant, func(tx pgx.Tx) error {
		q := gen.New(tx)
		status, err := s.svc.payVerifier.PollChargeTx(ctx, tx, t.tenant, t.paymentID)
		if err != nil {
			return err
		}
		switch status {
		case "succeeded":
			n, err := q.SalesClearEraPayPending(ctx, gen.SalesClearEraPayPendingParams{TenantID: t.tenant, ID: t.saleID})
			if err != nil {
				return err
			}
			if n == 0 {
				return nil // nota sudah tidak berflag (mis. sudah dibatalkan manual): tidak perlu menambah audit
			}
			return audit.Record(ctx, tx, audit.FromActor(s.actor(t)), audit.Entry{
				Action: audit.ActionSaleEraPayPaid, Entity: audit.EntitySale, EntityID: t.saleID.String(),
				Details: map[string]any{"doc_no": t.docNo, "charge_id": t.chargeID.String(), "amount": t.amount.StringFixed(2)},
			})
		case "failed", "expired", "cancelled":
			return s.voidUnpaid(ctx, tx, q, t)
		default: // pending / unknown: masih menunggu keputusan dari penyedia
			return nil
		}
	})
}

// voidUnpaid membatalkan nota yang ternyata tidak lunas di EraPay; stok dikembalikan persis seperti batal manual
// (reverseEffects) lalu nota ditandai void. Reversible tidak diminta (tidak ada manusia yang mengetik PIN).
func (s *EraPayReconciler) voidUnpaid(ctx context.Context, tx pgx.Tx, q *gen.Queries, t erapayTarget) error {
	orig, err := q.SalesLockForEdit(ctx, gen.SalesLockForEditParams{TenantID: t.tenant, ID: t.saleID})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	if orig.Status != "completed" {
		return nil // nota sudah dibatalkan/digantikan: tidak perlu tindakan
	}
	actor := s.actor(t)
	if err := reverseEffects(ctx, tx, q, actor, orig); err != nil {
		return err
	}
	n, err := q.SalesMarkVoid(ctx, gen.SalesMarkVoidParams{TenantID: t.tenant, ID: t.saleID,
		Reason: pgtype.Text{String: "tidak lunas di EraPay (rekonsiliasi otomatis)", Valid: true}})
	if err != nil {
		return err
	}
	if n != 1 {
		return nil
	}
	const reason = "tidak lunas di EraPay (rekonsiliasi otomatis)"
	if err := audit.Record(ctx, tx, audit.FromActor(actor), audit.Entry{
		Action: audit.ActionSaleEraPayUnpaid, Entity: audit.EntitySale, EntityID: t.saleID.String(),
		Details: map[string]any{"doc_no": t.docNo, "charge_id": t.chargeID.String(), "amount": t.amount.StringFixed(2), "status": orig.Status},
	}); err != nil {
		return err
	}
	return audit.Record(ctx, tx, audit.FromActor(actor), audit.Entry{
		Action: audit.ActionSaleVoid, Entity: audit.EntitySale, EntityID: t.saleID.String(),
		Details: map[string]any{"doc_no": t.docNo, "reason": reason, "total": orig.Total.String(), "source": "erapay_reconcile"},
	})
}
