// Package subscription: tenant melihat paketnya dan membeli/memperpanjang paket lewat QRIS EraPay milik PLATFORM
// (API key platform di env, bukan key EraPay tenant). Aktivasi langganan lewat fungsi DB subscription_activate() sekali per
// pembayaran lunas; penjadwal merekonsiliasi pembayaran yang lunas setelah tenant menutup halaman.
package subscription

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
	"aciraba/internal/erapay"
	"aciraba/internal/plan"
	"aciraba/internal/platform/db"
)

var (
	ErrOwnerOnly     = errors.New("hanya Owner yang dapat membeli paket")
	ErrImpersonating = errors.New("tidak tersedia pada sesi masuk-sebagai")
	ErrDisabled      = errors.New("pembayaran paket belum diaktifkan di server")
	ErrPlanNotFound  = errors.New("paket tidak tersedia")
	ErrNotFound      = errors.New("pembayaran tidak ditemukan")
	ErrKeyConflict   = errors.New("kunci idempotensi sudah dipakai untuk paket lain")
	ErrUnavailable   = errors.New("EraPay tidak dapat dihubungi, coba lagi")
)

const ActionSubscriptionActivate = "tenant.plan_activate"

type Service struct {
	pool   *pgxpool.Pool
	client *erapay.Client // nil = pembayaran paket mati
}

// NewService: client dibuat dari alamat EraPay server dan API key platform; kosong salah satunya = pembayaran mati.
func NewService(pool *pgxpool.Pool, eraBaseURL, platformKey string) *Service {
	return &Service{pool: pool, client: erapay.NewClient(eraBaseURL, platformKey)}
}

// PlanInfo = paket yang ditampilkan di halaman upgrade.
type PlanInfo struct {
	ID           uuid.UUID `json:"id"`
	Code         string    `json:"code"`
	Name         string    `json:"name"`
	Description  string    `json:"description"`
	Price        string    `json:"price"`
	DurationDays *int      `json:"duration_days"`
	MaxTxPerDay  *int      `json:"max_tx_per_day"`
	MaxUsers     *int      `json:"max_users"`
}

type Status struct {
	Current         plan.Effective `json:"current"`
	Usage           plan.Usage     `json:"usage"`
	Plans           []PlanInfo     `json:"plans"` // paket yang bisa dibeli sendiri
	PaymentsEnabled bool           `json:"payments_enabled"`
}

func nullInt(v pgtype.Int4) *int {
	if !v.Valid {
		return nil
	}
	n := int(v.Int32)
	return &n
}

// Status = paket efektif, pemakaian hari ini (tanggal lokal outlet aktif), dan paket yang dapat dibeli.
func (s *Service) Status(ctx context.Context, a authz.Actor) (Status, error) {
	st := Status{PaymentsEnabled: s.client != nil, Plans: []PlanInfo{}}
	err := db.WithTenant(ctx, s.pool, a.TenantID, func(tx pgx.Tx) error {
		var err error
		if st.Current, err = plan.Get(ctx, tx, a.TenantID); err != nil {
			return err
		}
		var day pgtype.Date
		if err := tx.QueryRow(ctx, `SELECT (now() AT TIME ZONE coalesce((SELECT timezone FROM outlets WHERE tenant_id = $1 AND id = $2), 'Asia/Jakarta'))::date`,
			a.TenantID, a.OutletID).Scan(&day); err != nil {
			return err
		}
		if st.Usage, err = plan.UsageFor(ctx, tx, a.TenantID, day); err != nil {
			return err
		}
		rows, err := tx.Query(ctx, `SELECT id, code, name, description, price, duration_days, max_tx_per_day, max_users
			FROM plans WHERE self_service AND active ORDER BY sort_order, price`)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var (
				p              PlanInfo
				price          decimal.Decimal
				dur, mtx, musr pgtype.Int4
			)
			if err := rows.Scan(&p.ID, &p.Code, &p.Name, &p.Description, &price, &dur, &mtx, &musr); err != nil {
				return err
			}
			p.Price, p.DurationDays, p.MaxTxPerDay, p.MaxUsers = price.StringFixed(0), nullInt(dur), nullInt(mtx), nullInt(musr)
			st.Plans = append(st.Plans, p)
		}
		return rows.Err()
	})
	return st, err
}

// Payment = bentuk respons pembayaran paket ke klien.
type Payment struct {
	ID               uuid.UUID  `json:"id"`
	PlanName         string     `json:"plan_name"`
	Status           string     `json:"status"`
	Amount           string     `json:"amount"`
	QRValue          string     `json:"qr_value,omitempty"`
	ExpiresAt        *time.Time `json:"expires_at,omitempty"`
	Applied          bool       `json:"applied"`                     // langganan sudah aktif dari pembayaran ini
	SubscriptionEnds *time.Time `json:"subscription_ends,omitempty"` // tanggal berakhir langganan setelah aktif
}

type payRow struct {
	Payment
	plan      uuid.UUID
	paymentID string
	amountDec decimal.Decimal
}

const payCols = `sp.id, p.name, sp.status, sp.amount, coalesce(sp.qr_value, ''), sp.expires_at, sp.applied_at IS NOT NULL, coalesce(sp.erapay_payment_id, ''), sp.plan_id`

func scanPay(row pgx.Row) (payRow, error) {
	var r payRow
	if err := row.Scan(&r.ID, &r.PlanName, &r.Status, &r.amountDec, &r.QRValue, &r.ExpiresAt, &r.Applied, &r.paymentID, &r.plan); err != nil {
		return payRow{}, err
	}
	r.Amount = r.amountDec.StringFixed(0)
	return r, nil
}

func loadPay(ctx context.Context, tx pgx.Tx, tenant uuid.UUID, where string, args ...any) (payRow, error) {
	return scanPay(tx.QueryRow(ctx, `SELECT `+payCols+` FROM subscription_payments sp JOIN plans p ON p.id = sp.plan_id WHERE sp.tenant_id = $1 AND `+where, append([]any{tenant}, args...)...))
}

func checkBuyer(a authz.Actor) error {
	if a.Impersonator != uuid.Nil {
		return ErrImpersonating
	}
	if !a.Perms.All {
		return ErrOwnerOnly
	}
	return nil
}

// Checkout membuat (atau mengembalikan, bila key sama) tagihan QRIS untuk membeli paket planID.
func (s *Service) Checkout(ctx context.Context, a authz.Actor, key string, planID uuid.UUID) (Payment, error) {
	if err := checkBuyer(a); err != nil {
		return Payment{}, err
	}
	if s.client == nil {
		return Payment{}, ErrDisabled
	}
	var c payRow
	err := db.WithTenant(ctx, s.pool, a.TenantID, func(tx pgx.Tx) error {
		var price decimal.Decimal
		var days int
		err := tx.QueryRow(ctx, `SELECT price, duration_days FROM plans WHERE id = $1 AND self_service AND active`, planID).Scan(&price, &days)
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrPlanNotFound
		}
		if err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO subscription_payments (tenant_id, plan_id, idempotency_key, amount, duration_days, created_by)
			VALUES ($1, $2, $3, $4, $5, $6) ON CONFLICT (tenant_id, idempotency_key) DO NOTHING`, a.TenantID, planID, key, price, days, a.UserID); err != nil {
			return err
		}
		c, err = loadPay(ctx, tx, a.TenantID, `sp.idempotency_key = $2`, key)
		return err
	})
	if err != nil {
		return Payment{}, err
	}
	if c.plan != planID {
		return Payment{}, ErrKeyConflict
	}
	if c.paymentID == "" { // belum sampai ke EraPay: kirim; Idempotency-Key = id pembayaran, aman diulang
		p, err := s.client.CreateQRIS(ctx, c.ID.String(), c.ID.String(), c.amountDec.IntPart(), "Paket "+c.PlanName+" ARUS")
		if errors.Is(err, erapay.ErrRejected) {
			return Payment{}, err
		}
		if err != nil {
			return Payment{}, ErrUnavailable
		}
		err = db.WithTenant(ctx, s.pool, a.TenantID, func(tx pgx.Tx) error {
			var e error
			c, e = store(ctx, tx, a.TenantID, c.ID, p)
			return e
		})
		if err != nil {
			return Payment{}, err
		}
	}
	return c.Payment, nil
}

func normStatus(s string) string {
	switch s {
	case "succeeded", "failed", "expired", "cancelled", "pending":
		return s
	case "created":
		return "pending"
	}
	return "unknown"
}

// store menyimpan hasil EraPay (status akhir tidak diturunkan lagi) dan mengembalikan barisnya.
func store(ctx context.Context, tx pgx.Tx, tenant, id uuid.UUID, p erapay.Payment) (payRow, error) {
	var qr *string
	if p.QRCode != nil && p.QRCode.Value != "" {
		qr = &p.QRCode.Value
	}
	if _, err := tx.Exec(ctx, `UPDATE subscription_payments SET
			erapay_payment_id = coalesce(erapay_payment_id, $3),
			status = CASE WHEN status IN ('succeeded','failed','expired','cancelled') THEN status ELSE $4 END,
			qr_value = coalesce($5, qr_value), expires_at = coalesce($6, expires_at)
		WHERE id = $1 AND tenant_id = $2`, id, tenant, p.ID, normStatus(p.Status), qr, p.ExpiresAt); err != nil {
		return payRow{}, err
	}
	return loadPay(ctx, tx, tenant, `sp.id = $2`, id)
}

// settle memutakhirkan satu pembayaran dari EraPay dan mengaktifkan langganan bila lunas (sekali). Dipakai klien (polling) dan
// penjadwal. Gagal menghubungi EraPay bukan galat: status tersimpan dikembalikan, dicoba lagi nanti.
func (s *Service) settle(ctx context.Context, tx pgx.Tx, tenant, id uuid.UUID) (payRow, error) {
	var locked uuid.UUID
	if err := tx.QueryRow(ctx, `SELECT id FROM subscription_payments WHERE id = $1 AND tenant_id = $2 FOR UPDATE`, id, tenant).Scan(&locked); err != nil {
		return payRow{}, err
	}
	c, err := loadPay(ctx, tx, tenant, `sp.id = $2`, id)
	if err != nil {
		return payRow{}, err
	}
	if c.paymentID != "" && (c.Status == "pending" || c.Status == "unknown") && s.client != nil {
		if p, gerr := s.client.Get(ctx, c.paymentID); gerr == nil {
			if c, err = store(ctx, tx, tenant, id, p); err != nil {
				return payRow{}, err
			}
		}
	}
	if c.Status == "succeeded" && !c.Applied {
		var ends pgtype.Timestamptz
		if err := tx.QueryRow(ctx, `SELECT subscription_activate($1, $2)`, tenant, id).Scan(&ends); err != nil {
			return payRow{}, err
		}
		if ends.Valid {
			if err := audit.Record(ctx, tx, audit.Actor{TenantID: tenant, Name: "Sistem"}, audit.Entry{
				Action: ActionSubscriptionActivate, Entity: audit.EntityTenant, EntityID: tenant.String(),
				Details: map[string]any{"payment_id": id.String(), "plan": c.PlanName, "expires_at": ends.Time},
			}); err != nil {
				return payRow{}, err
			}
		}
		if c, err = loadPay(ctx, tx, tenant, `sp.id = $2`, id); err != nil {
			return payRow{}, err
		}
	}
	return c, nil
}

// GetPayment = status pembayaran (memutakhirkan dari EraPay dan mengaktifkan langganan bila lunas).
func (s *Service) GetPayment(ctx context.Context, a authz.Actor, id uuid.UUID) (Payment, error) {
	if err := checkBuyer(a); err != nil {
		return Payment{}, err
	}
	var c payRow
	err := db.WithTenant(ctx, s.pool, a.TenantID, func(tx pgx.Tx) error {
		var e error
		c, e = s.settle(ctx, tx, a.TenantID, id)
		return e
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return Payment{}, ErrNotFound
	}
	if err != nil {
		return Payment{}, err
	}
	if c.Applied {
		_ = db.WithTenant(ctx, s.pool, a.TenantID, func(tx pgx.Tx) error {
			e, err := plan.Get(ctx, tx, a.TenantID)
			c.SubscriptionEnds = e.ExpiresAt
			return err
		})
	}
	return c.Payment, nil
}

const reconcileInterval = time.Minute

// Reconciler mengaktifkan langganan dari pembayaran yang lunas setelah tenant menutup halaman.
type Reconciler struct {
	svc *Service
	log *slog.Logger
}

func NewReconciler(svc *Service, log *slog.Logger) *Reconciler { return &Reconciler{svc: svc, log: log} }

func (r *Reconciler) Run(ctx context.Context) {
	if r.svc.client == nil {
		return
	}
	t := time.NewTicker(reconcileInterval)
	defer t.Stop()
	for {
		r.Tick(ctx)
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
	}
}

// Tick memproses pembayaran terbuka; mengembalikan jumlah yang diperiksa.
func (r *Reconciler) Tick(ctx context.Context) int {
	rows, err := r.svc.pool.Query(ctx, `SELECT tenant_id, id FROM subscription_reconcile_targets(200)`)
	if err != nil {
		r.log.Error("rekonsiliasi paket gagal membaca target", "err", err)
		return 0
	}
	type target struct{ tenant, id uuid.UUID }
	var ts []target
	for rows.Next() {
		var t target
		if err := rows.Scan(&t.tenant, &t.id); err == nil {
			ts = append(ts, t)
		}
	}
	rows.Close()
	for _, t := range ts {
		err := db.WithTenant(ctx, r.svc.pool, t.tenant, func(tx pgx.Tx) error {
			_, e := r.svc.settle(ctx, tx, t.tenant, t.id)
			return e
		})
		if err != nil {
			r.log.Error("rekonsiliasi paket gagal", "payment", t.id, "err", err)
		}
	}
	return len(ts)
}
