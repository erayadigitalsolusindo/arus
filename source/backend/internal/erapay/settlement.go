package erapay

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/shopspring/decimal"

	"aciraba/internal/authz"
	"aciraba/internal/platform/db"
	"aciraba/internal/platform/httpx"
)

// Settlement EraPay: tampilan "uang di ARUS vs uang di EraPay" per tagihan QRIS. Uang masuk ke EraPay = tagihan berstatus
// succeeded di EraPay; uang di ARUS = nominal QRIS pada nota aktif yang tertaut ke tagihan itu.
const (
	settlementLimit   = 300
	settlementRefresh = 30 // maks tagihan menunggu yang ditanyakan ulang ke EraPay dalam satu permintaan
	maxSettlementDays = 62
)

// Keadaan satu tagihan (dihitung server supaya semua klien memakai definisi yang sama).
const (
	StateMatched = "matched" // lunas di EraPay dan tercatat di nota aktif
	StateWaiting = "waiting" // belum ada keputusan EraPay (QR belum dibayar / menunggu)
	StateOrphan  = "orphan"  // lunas di EraPay tetapi tidak ada nota aktif: uang masuk, penjualan belum tercatat
	StateRefund  = "refund"  // nota dibatalkan tetapi tagihan sudah lunas: uang masih di EraPay, perlu dikembalikan
	StateUnpaid  = "unpaid"  // nota tercatat tetapi EraPay menyatakan gagal/batal/kedaluwarsa
	StateClosed  = "closed"  // tagihan gagal/kedaluwarsa/batal tanpa nota: tidak ada uang
)

type SettlementRow struct {
	ChargeID   uuid.UUID `json:"charge_id"`
	PaymentID  string    `json:"payment_id"` // ID transaksi di EraPay
	Gateway    string    `json:"gateway"`
	Status     string    `json:"status"` // status di EraPay
	State      string    `json:"state"`
	Amount     string    `json:"amount"`      // nominal tagihan
	ArusAmount string    `json:"arus_amount"` // uang menurut ARUS (nota aktif)
	EraAmount  string    `json:"era_amount"`  // uang menurut EraPay (lunas)
	CreatedAt  time.Time `json:"created_at"`
	Outlet     string    `json:"outlet"`
	Cashier    string    `json:"cashier"`
	DocNo      string    `json:"doc_no,omitempty"`
	SaleStatus string    `json:"sale_status,omitempty"`
}

type SettlementTotals struct {
	Arus      string `json:"arus"`
	Era       string `json:"era"`
	Diff      string `json:"diff"` // Era - Arus; tidak nol berarti ada yang perlu dicek
	Count     int    `json:"count"`
	Matched   int    `json:"matched"`
	Waiting   int    `json:"waiting"`
	Attention int    `json:"attention"` // orphan + refund + unpaid
}

type SettlementResult struct {
	From       string           `json:"from"`
	To         string           `json:"to"`
	Configured bool             `json:"configured"`
	Refreshed  int              `json:"refreshed"`
	Truncated  bool             `json:"truncated"`
	Totals     SettlementTotals `json:"totals"`
	Data       []SettlementRow  `json:"data"`
}

func stateOf(status string, saleID *uuid.UUID, saleStatus string) string {
	linked := saleID != nil
	active := linked && saleStatus == "completed"
	switch status {
	case StatusSucceeded:
		switch {
		case active:
			return StateMatched
		case linked && saleStatus == "void":
			return StateRefund
		default:
			return StateOrphan
		}
	case "failed", "expired", "cancelled":
		if active {
			return StateUnpaid
		}
		return StateClosed
	default: // pending / unknown
		return StateWaiting
	}
}

// FieldErrors = kode galat per field (INVALID), diterjemahkan klien.
type FieldErrors map[string]string

func (f FieldErrors) Error() string { return "input tidak valid" }

// Kondisi rentang waktu: tanggal lokal outlet; dua batas UTC tambahan agar indeks (tenant_id, created_at) dipakai.
const settlementRange = `
  c.created_at >= $3::timestamptz - interval '1 day' AND c.created_at < $4::timestamptz + interval '2 days'
  AND c.created_at >= ($5::date)::timestamp AT TIME ZONE o.timezone
  AND c.created_at < ($6::date + 1)::timestamp AT TIME ZONE o.timezone`

// Settlement membaca tagihan QRIS pada rentang tanggal (YYYY-MM-DD, zona waktu outlet) untuk outlet aktif, atau semua
// outlet yang boleh diakses bila allOutlets. refresh=true: tagihan yang masih menunggu ditanyakan ulang ke EraPay
// (maks settlementRefresh per permintaan) sebelum dihitung; tidak mengubah atau membatalkan nota.
func (s *Service) Settlement(ctx context.Context, a authz.Actor, from, to string, allOutlets, refresh bool) (SettlementResult, error) {
	res := SettlementResult{Data: []SettlementRow{}}
	outlets := []uuid.UUID{a.OutletID}
	if allOutlets && len(a.Outlets) > 0 {
		outlets = outlets[:0]
		for id := range a.Outlets {
			outlets = append(outlets, id)
		}
	}
	var fd, td time.Time
	fe := FieldErrors{}
	err := db.WithTenant(ctx, s.pool, a.TenantID, func(tx pgx.Tx) error {
		var today time.Time
		if err := tx.QueryRow(ctx, `SELECT (now() AT TIME ZONE timezone)::date FROM outlets WHERE tenant_id = $1 AND id = $2`,
			a.TenantID, a.OutletID).Scan(&today); err != nil {
			return err
		}
		fd, td = today, today
		var err error
		if strings.TrimSpace(from) != "" {
			if fd, err = time.Parse("2006-01-02", from); err != nil {
				fe["from"] = "INVALID"
			}
		}
		if strings.TrimSpace(to) != "" {
			if td, err = time.Parse("2006-01-02", to); err != nil {
				fe["to"] = "INVALID"
			}
		}
		if len(fe) == 0 && (td.Before(fd) || td.Sub(fd) > maxSettlementDays*24*time.Hour) {
			fe["to"] = "INVALID"
		}
		return nil
	})
	if err != nil {
		return res, err
	}
	if len(fe) > 0 {
		return res, fe
	}
	res.From, res.To = fd.Format("2006-01-02"), td.Format("2006-01-02")

	if cfg, err := s.Config(ctx, a); err == nil {
		res.Configured = cfg.Configured
	}
	if refresh && res.Configured {
		n, err := s.refreshPending(ctx, a, outlets, fd, td)
		if err != nil {
			return res, err
		}
		res.Refreshed = n
	}

	era, arus := decimal.Zero, decimal.Zero
	err = db.WithTenant(ctx, s.pool, a.TenantID, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `
			SELECT c.id, coalesce(c.erapay_payment_id, ''), coalesce(c.gateway, ''), c.status, c.amount, c.created_at,
			       coalesce(o.name, ''), coalesce(u.name, ''), s.id, coalesce(s.doc_no, ''), coalesce(s.status, '')
			FROM erapay_charges c
			JOIN outlets o ON o.tenant_id = c.tenant_id AND o.id = c.outlet_id
			LEFT JOIN users u ON u.tenant_id = c.tenant_id AND u.id = c.created_by
			LEFT JOIN sales s ON s.tenant_id = c.tenant_id AND s.id = c.sale_id
			WHERE c.tenant_id = $1 AND c.outlet_id = ANY($2) AND `+settlementRange+`
			  AND (c.erapay_payment_id IS NOT NULL OR c.sale_id IS NOT NULL)
			ORDER BY c.created_at DESC, c.id DESC
			LIMIT $7`, a.TenantID, outlets, fd, td, fd, td, settlementLimit+1)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var r SettlementRow
			var amount decimal.Decimal
			var saleID *uuid.UUID
			if err := rows.Scan(&r.ChargeID, &r.PaymentID, &r.Gateway, &r.Status, &amount, &r.CreatedAt,
				&r.Outlet, &r.Cashier, &saleID, &r.DocNo, &r.SaleStatus); err != nil {
				return err
			}
			if len(res.Data) >= settlementLimit {
				res.Truncated = true
				break
			}
			r.State = stateOf(r.Status, saleID, r.SaleStatus)
			r.Amount = amount.StringFixed(2)
			r.ArusAmount, r.EraAmount = "0.00", "0.00"
			if saleID != nil && r.SaleStatus == "completed" {
				r.ArusAmount = r.Amount
				arus = arus.Add(amount)
			}
			if r.Status == StatusSucceeded {
				r.EraAmount = r.Amount
				era = era.Add(amount)
			}
			res.Totals.Count++
			switch r.State {
			case StateMatched:
				res.Totals.Matched++
			case StateWaiting:
				res.Totals.Waiting++
			case StateOrphan, StateRefund, StateUnpaid:
				res.Totals.Attention++
			}
			res.Data = append(res.Data, r)
		}
		return rows.Err()
	})
	res.Totals.Arus, res.Totals.Era, res.Totals.Diff = arus.StringFixed(2), era.StringFixed(2), era.Sub(arus).StringFixed(2)
	return res, err
}

// refreshPending menanyakan ulang ke EraPay tagihan yang masih menunggu pada rentang itu. Gagal menghubungi EraPay tidak
// menggagalkan laporan: status tersimpan terakhir yang ditampilkan.
func (s *Service) refreshPending(ctx context.Context, a authz.Actor, outlets []uuid.UUID, fd, td time.Time) (int, error) {
	type pend struct {
		id        uuid.UUID
		paymentID string
	}
	var list []pend
	err := db.WithTenant(ctx, s.pool, a.TenantID, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `
			SELECT c.id, c.erapay_payment_id FROM erapay_charges c
			JOIN outlets o ON o.tenant_id = c.tenant_id AND o.id = c.outlet_id
			WHERE c.tenant_id = $1 AND c.outlet_id = ANY($2) AND c.status IN ('pending', 'unknown')
			  AND c.erapay_payment_id IS NOT NULL AND `+settlementRange+`
			ORDER BY c.created_at DESC LIMIT $7`, a.TenantID, outlets, fd, td, fd, td, settlementRefresh)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var p pend
			if err := rows.Scan(&p.id, &p.paymentID); err != nil {
				return err
			}
			list = append(list, p)
		}
		return rows.Err()
	})
	if err != nil || len(list) == 0 {
		return 0, err
	}
	cl, err := s.client(ctx, a.TenantID)
	if err != nil || cl == nil {
		return 0, err
	}
	n := 0
	for _, p := range list {
		pay, gerr := cl.Get(ctx, p.paymentID)
		if gerr != nil {
			continue // EraPay tidak terjangkau untuk tagihan ini: tampilkan status tersimpan
		}
		if _, err := s.store(ctx, a.TenantID, p.id, pay); err != nil {
			return n, err
		}
		n++
	}
	return n, nil
}

// settlement: GET /payments/erapay/settlement?from=&to=&all_outlets=&refresh= (izin sales_reports.view).
func (h *Handler) settlement(w http.ResponseWriter, r *http.Request) {
	qs := r.URL.Query()
	res, err := h.svc.Settlement(r.Context(), actor(r), qs.Get("from"), qs.Get("to"), qs.Get("all_outlets") == "true", qs.Get("refresh") == "true")
	var fe FieldErrors
	switch {
	case errors.As(err, &fe):
		httpx.ValidationError(w, fe)
	case err != nil:
		h.fail(w, r, err)
	default:
		httpx.JSON(w, http.StatusOK, res)
	}
}
