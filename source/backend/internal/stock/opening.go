package stock

import (
	"context"
	"errors"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/shopspring/decimal"

	"aciraba/internal/audit"
	"aciraba/internal/authz"
	gen "aciraba/internal/gen"
	"aciraba/internal/platform/db"
	"aciraba/internal/platform/sanitize"
)

// Saldo awal stok (Fase 4.2, FR-ONB-03/07). Outlet = outlet aktif sesi (dari token, bukan dari body).
// Mengubah saldo awal = movement OPENING sebesar selisih terhadap saldo bucket saat ini, sehingga ledger tetap
// append-only dan kartu stok memperlihatkan riwayat koreksinya.

var (
	ErrAlreadyLocked   = errors.New("tanggal mulai operasional sudah dikunci")
	ErrNotFound        = errors.New("data tidak ditemukan")
	ErrOutletForbidden = errors.New("tidak punya akses ke outlet dokumen ini")
)

// FieldErrors = kode galat per field (REQUIRED/INVALID), diterjemahkan klien.
type FieldErrors map[string]string

func (f FieldErrors) Error() string { return "input tidak valid" }

const (
	defLimit = 20
	maxLimit = 100
	maxQ     = 200
)

type Service struct{ pool *pgxpool.Pool }

func NewService(pool *pgxpool.Pool) *Service { return &Service{pool: pool} }

// OpeningStatus = status penguncian saldo awal outlet aktif.
type OpeningStatus struct {
	Locked    bool       `json:"locked"`
	StartDate *string    `json:"start_date"`
	LockedAt  *time.Time `json:"locked_at"`
	// Stats = ringkasan isian saldo awal outlet aktif (hanya dari GET /status; kosong pada hasil kunci).
	Stats *OpeningStats `json:"stats,omitempty"`
}

// OpeningStats = berapa barang yang sudah punya saldo dan total qty per bucket (satuan dasar, dijumlah lintas barang).
type OpeningStats struct {
	Items     int    `json:"items"`  // barang berstok aktif
	Filled    int    `json:"filled"` // barang yang punya saldo di salah satu bucket
	Display   string `json:"display"`
	Warehouse string `json:"warehouse"`
	Returns   string `json:"returns"`
}

// OpeningRow = satu barang beserta stok outlet aktif per bucket.
type OpeningRow struct {
	ID        uuid.UUID `json:"id"`
	SKU       string    `json:"sku"`
	Name      string    `json:"name"`
	Unit      string    `json:"unit"`
	Display   string    `json:"display"`
	Warehouse string    `json:"warehouse"`
	Returns   string    `json:"returns"`
}

type ListParams struct {
	Q             string
	Limit, Offset int
}

func (s *Service) Status(ctx context.Context, a authz.Actor) (OpeningStatus, error) {
	var out OpeningStatus
	err := db.WithTenant(ctx, s.pool, a.TenantID, func(tx pgx.Tx) error {
		st, err := gen.New(tx).StockOutletLockState(ctx, gen.StockOutletLockStateParams{TenantID: a.TenantID, ID: a.OutletID})
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNotFound
		}
		if err != nil {
			return err
		}
		out = statusOf(st.StockLockedAt.Valid, st.StockLockedAt.Time, st.OpsStartDate.Valid, st.OpsStartDate.Time)
		var stats OpeningStats
		var d, w, r decimal.Decimal
		if err := tx.QueryRow(ctx, `SELECT
			(SELECT count(*) FROM items WHERE tenant_id = $1 AND kind = $3 AND active),
			count(DISTINCT item_id) FILTER (WHERE qty <> 0),
			coalesce(sum(qty) FILTER (WHERE bucket = $4), 0),
			coalesce(sum(qty) FILTER (WHERE bucket = $5), 0),
			coalesce(sum(qty) FILTER (WHERE bucket = $6), 0)
			FROM stock_balances WHERE tenant_id = $1 AND outlet_id = $2`,
			a.TenantID, a.OutletID, "goods", string(BucketDisplay), string(BucketWarehouse), string(BucketReturns)).Scan(&stats.Items, &stats.Filled, &d, &w, &r); err != nil {
			return err
		}
		stats.Display, stats.Warehouse, stats.Returns = d.String(), w.String(), r.String()
		out.Stats = &stats
		return nil
	})
	return out, err
}

func statusOf(locked bool, at time.Time, hasDate bool, date time.Time) OpeningStatus {
	st := OpeningStatus{Locked: locked}
	if locked {
		st.LockedAt = &at
	}
	if hasDate {
		d := date.Format("2006-01-02")
		st.StartDate = &d
	}
	return st
}

func (s *Service) List(ctx context.Context, a authz.Actor, p ListParams) ([]OpeningRow, int, error) {
	q, ok := sanitize.Text(p.Q)
	if !ok || utf8.RuneCountInString(q) > maxQ {
		return nil, 0, FieldErrors{"q": sanitize.Invalid}
	}
	limit := p.Limit
	if limit <= 0 {
		limit = defLimit
	}
	out := []OpeningRow{}
	total := 0
	err := db.WithTenant(ctx, s.pool, a.TenantID, func(tx pgx.Tx) error {
		rows, err := gen.New(tx).StockOpeningList(ctx, gen.StockOpeningListParams{
			TenantID: a.TenantID, OutletID: a.OutletID, Q: likeEscape(q), PageLimit: int32(min(limit, maxLimit)), PageOffset: int32(max(p.Offset, 0)),
		})
		for _, r := range rows {
			out = append(out, OpeningRow{ID: r.ID, SKU: r.Sku, Name: r.Name, Unit: r.UnitName,
				Display: r.Display.String(), Warehouse: r.Warehouse.String(), Returns: r.Returns.String()})
			total = int(r.Total)
		}
		return err
	})
	return out, total, err
}

// SetOpening menetapkan saldo awal (qty ≥ 0, maks 3 desimal) satu barang pada satu bucket outlet aktif.
// qty adalah saldo target, bukan selisih. Mengembalikan saldo bucket setelahnya.
func (s *Service) SetOpening(ctx context.Context, a authz.Actor, itemID uuid.UUID, bucket, qtyStr string) (decimal.Decimal, error) {
	f := FieldErrors{}
	if !validBuckets[bucket] {
		f["bucket"] = sanitize.Invalid
	}
	qty, err := decimal.NewFromString(qtyStr)
	switch {
	case qtyStr == "":
		f["qty"] = sanitize.Required
	case err != nil || qty.IsNegative() || !qty.Equal(qty.Round(3)) || qty.GreaterThanOrEqual(maxQty):
		f["qty"] = sanitize.Invalid
	}
	if len(f) > 0 {
		return decimal.Zero, f
	}
	var after decimal.Decimal
	err = db.WithTenant(ctx, s.pool, a.TenantID, func(tx pgx.Tx) error {
		q := gen.New(tx)
		it, err := q.StockItemLock(ctx, gen.StockItemLockParams{TenantID: a.TenantID, ID: itemID})
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrItemNotFound
		}
		if err != nil {
			return err
		}
		if it.Kind != "goods" {
			return ErrNotStocked
		}
		cur, err := q.StockBalanceGet(ctx, gen.StockBalanceGetParams{TenantID: a.TenantID, OutletID: a.OutletID, ItemID: itemID, Bucket: bucket})
		if errors.Is(err, pgx.ErrNoRows) {
			cur, err = decimal.Zero, nil
		}
		if err != nil {
			return err
		}
		after = cur
		delta := qty.Sub(cur)
		if delta.IsZero() {
			// Tetap periksa kunci agar klien mendapat kabar yang benar bila saldo awal sudah dikunci.
			st, err := q.StockOutletLockState(ctx, gen.StockOutletLockStateParams{TenantID: a.TenantID, ID: a.OutletID})
			if err != nil {
				return err
			}
			if st.StockLockedAt.Valid {
				return ErrOpeningLocked
			}
			return nil
		}
		res, err := Apply(ctx, tx, Movement{TenantID: a.TenantID, OutletID: a.OutletID, ItemID: itemID, Bucket: bucket, Delta: delta,
			RefType: RefOpening, ActorID: a.UserID, Note: "Saldo awal"})
		if err != nil {
			return err
		}
		after = res.BalanceAfter
		return audit.Record(ctx, tx, audit.FromActor(a), audit.Entry{
			Action: audit.ActionStockOpening, Entity: audit.EntityStock, EntityID: itemID.String(),
			Details: map[string]any{"sku": it.Sku, "name": it.Name, "outlet_id": a.OutletID.String(), "bucket": bucket,
				"before": cur.String(), "after": after.String()},
		})
	})
	return after, err
}

// Lock mengunci tanggal mulai operasional outlet aktif (YYYY-MM-DD); sesudahnya saldo awal tidak bisa diubah.
func (s *Service) Lock(ctx context.Context, a authz.Actor, startDate string) (OpeningStatus, error) {
	d, err := time.Parse("2006-01-02", startDate)
	switch {
	case startDate == "":
		return OpeningStatus{}, FieldErrors{"start_date": sanitize.Required}
	case err != nil || d.Year() < 2000 || d.Year() > 2100:
		return OpeningStatus{}, FieldErrors{"start_date": sanitize.Invalid}
	}
	var out OpeningStatus
	err = db.WithTenant(ctx, s.pool, a.TenantID, func(tx pgx.Tx) error {
		r, err := gen.New(tx).StockOutletLock(ctx, gen.StockOutletLockParams{
			TenantID: a.TenantID, OutletID: a.OutletID, StartDate: pgDate(d)})
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrAlreadyLocked
		}
		if err != nil {
			return err
		}
		out = statusOf(r.StockLockedAt.Valid, r.StockLockedAt.Time, r.OpsStartDate.Valid, r.OpsStartDate.Time)
		return audit.Record(ctx, tx, audit.FromActor(a), audit.Entry{
			Action: audit.ActionStockOpeningLock, Entity: audit.EntityOutlet, EntityID: a.OutletID.String(),
			Details: map[string]any{"start_date": startDate},
		})
	})
	return out, err
}

func pgDate(d time.Time) pgtype.Date { return pgtype.Date{Time: d, Valid: true} }

func likeEscape(s string) string {
	return strings.NewReplacer("\\", "\\\\", "%", "\\%", "_", "\\_").Replace(s)
}
