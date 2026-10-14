package sales

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/shopspring/decimal"

	"aciraba/internal/authz"
	"aciraba/internal/platform/db"
)

const maxReportDays = 366

// FieldErrors maps parameter names to validation error slugs.
type FieldErrors map[string]string

func (f FieldErrors) Error() string {
	if len(f) == 0 {
		return ""
	}
	var b []byte
	for k, v := range f {
		b = append(b, k...)
		b = append(b, ':', ' ')
		b = append(b, v...)
		b = append(b, ' ')
	}
	return string(b)
}

// Service of sales reports.
type Service struct {
	pool *pgxpool.Pool
}

func NewService(pool *pgxpool.Pool) *Service {
	return &Service{pool: pool}
}

// ByDateParams filter laporan per tanggal.
type ByDateParams struct {
	AllOutlets bool
	OutletID   string
	From, To   string
	CashierID  string
	// HideEmpty = true: hari tanpa penjualan/void/retur tidak dikembalikan
	// (hasil lebih ringkas untuk rentang panjang; default false = deret tanggal penuh).
	HideEmpty bool
}

// DayRow satu baris tanggal pada laporan.
type DayRow struct {
	Date        string       `json:"date"` // YYYY-MM-DD (zona waktu outlet)
	SaleCount   int          `json:"sale_count"`
	VoidCount   int          `json:"void_count"`
	Subtotal    string       `json:"subtotal"`
	Discount    string       `json:"discount"`
	TaxStore    string       `json:"tax_store"`
	TaxGov      string       `json:"tax_gov"`
	Total       string       `json:"total"`
	Paid        string       `json:"paid"`     // uang masuk netto (tunai sudah bersih kembalian)
	Receivable  string       `json:"receivable"`
	VoidTotal   string       `json:"void_total"` // Σ nota void (positif)
	Methods     []MethodRow  `json:"methods,omitempty"`
	ReturnCount int          `json:"return_count"`
	ReturnTotal string       `json:"return_total"`
	NetTotal    string       `json:"net_total"` // paid − void − return
}

// MethodRow breakdown satu metode.
type MethodRow struct {
	MethodID  string `json:"method_id,omitempty"` // UUID string; kosong jika tidak ada metode
	Name      string `json:"name"`
	Kind      string `json:"kind"`
	Amount    string `json:"amount"`
	Fee       string `json:"fee"`
	Surcharge string `json:"surcharge"`
}

// ByDateResult hasil laporan per tanggal.
type ByDateResult struct {
	From string    `json:"from"`
	To   string    `json:"to"`
	Data []DayRow  `json:"data"`
}

// ByDate laporan penjualan agregat per tanggal (zona waktu outlet), dengan breakdown
// per metode pembayaran dan retur. Hanya nota completed dihitung untuk total/paid;
// void dan retur ditampilkan terpisah.
func (s *Service) ByDate(ctx context.Context, a authz.Actor, p ByDateParams) (ByDateResult, error) {
	var result ByDateResult

	err := db.WithTenant(ctx, s.pool, a.TenantID, func(tx pgx.Tx) error {
		today := time.Now().UTC() // default; akan overriden jika from/to kosong dari query outlet
		// Ambil local day nyata dari outlet (zona waktu outlet).
		var localDay pgtype.Date
		err := tx.QueryRow(ctx, `
			SELECT (now() AT TIME ZONE timezone)::date FROM outlets WHERE tenant_id = $1 AND id = $2`,
			a.TenantID, a.OutletID).Scan(&localDay)
		if err != nil {
			return err
		}
		if localDay.Valid {
			today = localDay.Time
		}
		fd, td := today, today
		fe := make(FieldErrors)

		if p.From != "" {
			if fd, err = time.Parse("2006-01-02", p.From); err != nil {
				fe["from"] = "INVALID"
			}
		}
		if p.To != "" {
			if td, err = time.Parse("2006-01-02", p.To); err != nil {
				fe["to"] = "INVALID"
			}
		}
		if len(fe) == 0 && td.Before(fd) {
			fe["to"] = "MUST_BE_AFTER_FROM"
		}
		if td.Sub(fd) > maxReportDays*24*time.Hour {
			fe["to"] = "MAX_366_DAYS"
		}
		if len(fe) > 0 {
			return fe
		}

		var cashier uuid.UUID
		if p.CashierID != "" {
			id, err := uuid.Parse(p.CashierID)
			if err != nil || id == uuid.Nil {
				return errors.New("cashier_id: INVALID")
			}
			cashier = id
		}

		outlets := []uuid.UUID{a.OutletID}
		if p.AllOutlets && len(a.Outlets) > 0 {
			outlets = outlets[:0]
			for id := range a.Outlets {
				outlets = append(outlets, id)
			}
		}
		if p.OutletID != "" {
			id, err := uuid.Parse(p.OutletID)
			if err != nil || id == uuid.Nil {
				return errors.New("outlet_id: INVALID")
			}
			outlets = []uuid.UUID{id}
		}

		result.From, result.To = fd.Format("2006-01-02"), td.Format("2006-01-02")

		// Laporan 1 tahun menyentuh ratusan ribu baris untuk agregasi + sort;
		// beri ruang sort/hash transaksi ini agar tidak tumpah ke disk.
		// SET LOCAL hanya berlaku di transaksi ini (aman untuk pooler).
		if _, err := tx.Exec(ctx, "SET LOCAL work_mem = '64MB'"); err != nil {
			return err
		}

		// Prefilter UTC untuk indeks: tanggal lokal [fd, td] di zona waktu APAPUN
		// (offset ekstrem UTC+14 s/d UTC−12) selalu berada dalam [fd−1 hari, td+2 hari)
		// UTC. Memangkas pemindaian ke rentang indeks sales_outlet_time_idx
		// (tenant, outlet, created_at); filter tanggal lokal yang tepat tetap
		// dipegang predikat (created_at AT TIME ZONE ...) di tiap CTE.
		dayStart := func(d time.Time, addDays int) time.Time {
			return time.Date(d.Year(), d.Month(), d.Day(), 0, 0, 0, 0, time.UTC).AddDate(0, 0, addDays)
		}
		padFrom, padTo := dayStart(fd, -1), dayStart(td, 2)

		// ── 1. Sales + payment breakdown per date ────────────────────────────
		rows, err := tx.Query(ctx, byDateSalesSQL,
			a.TenantID, outlets, fd, td, cashier, padFrom, padTo)
		if err != nil {
			return err
		}
		days := make(map[string]*DayRow) // keyed by date string

		for rows.Next() {
			var date string
			var sc, vc int
			var subtotal, discount, taxStore, taxGov, total, paid, receivable, voidTotal decimal.Decimal
			var methodID pgtype.UUID
			var methodName, methodKind pgtype.Text
			var mAmt, mFee, mSurch decimal.Decimal

			if err := rows.Scan(
				&date, &sc, &vc,
				&subtotal, &discount, &taxStore, &taxGov,
				&total, &paid, &receivable, &voidTotal,
				&methodID, &methodName, &methodKind,
				&mAmt, &mFee, &mSurch,
			); err != nil {
				rows.Close()
				return err
			}

			row, seen := days[date]
			if !seen {
				row = &DayRow{Date: date}
				days[date] = row
			}

			// Header totals — only fill once per date (first row for that date).
			if !seen {
				row.SaleCount = sc
				row.VoidCount = vc
				row.Subtotal = subtotal.StringFixed(2)
				row.Discount = discount.StringFixed(2)
				row.TaxStore = taxStore.StringFixed(2)
				row.TaxGov = taxGov.StringFixed(2)
				row.Total = total.StringFixed(2)
				row.Paid = paid.StringFixed(2)
				row.Receivable = receivable.StringFixed(2)
				row.VoidTotal = voidTotal.StringFixed(2)
			}

			if methodName.Valid && methodName.String != "" {
				row.Methods = append(row.Methods, MethodRow{
					MethodID:  methodID.String(),
					Name:      methodName.String,
					Kind:      methodKind.String,
					Amount:    mAmt.StringFixed(2),
					Fee:       mFee.StringFixed(2),
					Surcharge: mSurch.StringFixed(2),
				})
			}
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return err
		}

		// ── 2. Returns per return_date ─────────────────────────────────────
		rrows, err := tx.Query(ctx, byDateReturnsSQL,
			a.TenantID, outlets, fd, td, cashier, padFrom, padTo)
		if err != nil {
			return err
		}
		for rrows.Next() {
			var date string
			var rc int
			var rt decimal.Decimal
			if err := rrows.Scan(&date, &rc, &rt); err != nil {
				rrows.Close()
				return err
			}
			if row, ok := days[date]; ok {
				row.ReturnCount = rc
				row.ReturnTotal = rt.StringFixed(2)
			}
		}
		rrows.Close()
		if err := rrows.Err(); err != nil {
			return err
		}

		// ── 3. Sort + compute net_total ────────────────────────────────────
		sorted := make([]DayRow, 0, len(days))
		for _, row := range days {
			if p.HideEmpty && row.SaleCount == 0 && row.VoidCount == 0 && row.ReturnCount == 0 {
				continue // hari tanpa aktivitas penjualan/void/retur disembunyikan
			}
			paid, _ := decimal.NewFromString(row.Paid)
			void, _ := decimal.NewFromString(row.VoidTotal)
			ret, _ := decimal.NewFromString(row.ReturnTotal)
			net := paid.Sub(void).Sub(ret)
			row.NetTotal = net.StringFixed(2)
			sorted = append(sorted, *row)
		}
		// Bubble sort ASC by date (dataset kecil; heap-sort overhead tidak worth untuk < 366 item).
		for i := 0; i < len(sorted)-1; i++ {
			for j := i + 1; j < len(sorted); j++ {
				if sorted[j].Date < sorted[i].Date {
					sorted[i], sorted[j] = sorted[j], sorted[i]
				}
			}
		}
		result.Data = sorted
		return nil
	})
	return result, err
}

// NOTE: SQL di bawah menggunakan pola yang sama dengan sales/queries.sql.
// Untuk production, pindahkan ke queries.sql + regenerate via sqlc.

// byDateSalesSQL: agregasi penjualan per tanggal (zona waktu outlet) dengan breakdown metode.
// paid = Σ(paid − change) untuk nota completed; void_total = Σ total nota void (positif).
const byDateSalesSQL = `
WITH
outlet_tz AS (
    SELECT id, timezone FROM outlets WHERE tenant_id = $1 AND id = ANY($2::uuid[])
),
day_range AS (
    SELECT generate_series($3::date, $4::date, '1 day'::interval)::date AS day
),
-- Header totals per date (completed + void counts, subtotals, taxes, totals, receivable).
sales_hdr AS (
    SELECT
        (s.created_at AT TIME ZONE o.timezone)::date AS sale_date,
        count(*) FILTER (WHERE s.status = 'completed') AS sale_count,
        count(*) FILTER (WHERE s.status = 'void') AS void_count,
        coalesce(sum(s.subtotal) FILTER (WHERE s.status = 'completed'), 0) AS subtotal,
        coalesce(sum(s.discount) FILTER (WHERE s.status = 'completed'), 0) AS discount,
        coalesce(sum(s.tax_store) FILTER (WHERE s.status = 'completed'), 0) AS tax_store,
        coalesce(sum(s.tax_gov) FILTER (WHERE s.status = 'completed'), 0) AS tax_gov,
        coalesce(sum(s.total) FILTER (WHERE s.status = 'completed'), 0) AS total,
            -- paid netto: Σ(paid − change) per completed sale (change hanya ada di transaksi tunai).
        coalesce(sum(s.paid - s.change) FILTER (WHERE s.status = 'completed'), 0) AS paid,
        coalesce(sum(s.receivable) FILTER (WHERE s.status = 'completed'), 0) AS receivable,
        coalesce(sum(s.total) FILTER (WHERE s.status = 'void'), 0) AS void_total
    FROM sales s
    JOIN outlet_tz o ON o.id = s.outlet_id
    WHERE s.tenant_id = $1
      AND s.outlet_id = ANY($2::uuid[])
      AND s.status IN ('completed', 'void')
      AND s.created_at >= $6 AND s.created_at < $7
      AND ($5::uuid = '00000000-0000-0000-0000-000000000000' OR s.cashier_id = $5)
      AND (s.created_at AT TIME ZONE o.timezone)::date BETWEEN $3::date AND $4::date
    GROUP BY (s.created_at AT TIME ZONE o.timezone)::date
),
-- Payment breakdown per date per method. paid netto per method: tunai dikurangi kembalian per nota.
pay_sub AS (
    SELECT
        (s.created_at AT TIME ZONE o.timezone)::date AS sale_date,
        s.id                                          AS sale_id,
        s.change                                      AS sale_change,
        p.method_id,
        pm.name                                       AS method_name,
        pm.kind                                       AS method_kind,
            -- Netto tunai = Σ pembayaran tunai − kembalian nota ini.
        coalesce(sum(p.amount), 0)
            - CASE WHEN pm.kind = 'cash' THEN s.change ELSE 0 END AS amt,
            -- fee: biaya MDR yang ditanggung toko (mengurangi pendapatan).
        coalesce(sum(p.fee_amount) FILTER (WHERE p.fee_bearer = 'store'), 0)    AS fee,
            -- surcharge: biaya MDR yang ditagihkan ke pelanggan (tambahan di luar total nota).
        coalesce(sum(p.fee_amount) FILTER (WHERE p.fee_bearer = 'customer'), 0) AS surcharge
    FROM sales s
    JOIN outlet_tz o ON o.id = s.outlet_id
    JOIN sale_payments p ON p.tenant_id = s.tenant_id AND p.sale_id = s.id
    JOIN payment_methods pm ON pm.tenant_id = s.tenant_id AND pm.id = p.method_id
    WHERE s.tenant_id = $1
      AND s.outlet_id = ANY($2::uuid[])
      AND s.status = 'completed'
      AND s.created_at >= $6 AND s.created_at < $7
      AND ($5::uuid = '00000000-0000-0000-0000-000000000000' OR s.cashier_id = $5)
      AND (s.created_at AT TIME ZONE o.timezone)::date BETWEEN $3::date AND $4::date
    GROUP BY (s.created_at AT TIME ZONE o.timezone)::date, s.id, s.change, p.method_id, pm.name, pm.kind
),
pay_hdr AS (
    SELECT
        sale_date,
        method_id,
        method_name,
        method_kind,
        sum(amt)        AS amount,
        sum(fee)        AS fee,
        sum(surcharge)  AS surcharge
    FROM pay_sub
    GROUP BY sale_date, method_id, method_name, method_kind
)
SELECT
    d.day::text                        AS date,
    coalesce(h.sale_count, 0)          AS sale_count,
    coalesce(h.void_count, 0)          AS void_count,
    coalesce(h.subtotal, 0)            AS subtotal,
    coalesce(h.discount, 0)            AS discount,
    coalesce(h.tax_store, 0)           AS tax_store,
    coalesce(h.tax_gov, 0)             AS tax_gov,
    coalesce(h.total, 0)               AS total,
    coalesce(h.paid, 0)                AS paid,
    coalesce(h.receivable, 0)          AS receivable,
    coalesce(h.void_total, 0)         AS void_total,
    p.method_id,
    p.method_name,
    p.method_kind,
    coalesce(p.amount, 0)             AS method_amount,
    coalesce(p.fee, 0)                AS method_fee,
    coalesce(p.surcharge, 0)          AS method_surcharge
FROM day_range d
LEFT JOIN sales_hdr h ON h.sale_date = d.day
LEFT JOIN pay_hdr p  ON p.sale_date = d.day
ORDER BY d.day, (p.method_kind = 'cash') DESC, lower(p.method_name)
`

// byDateReturnsSQL: retur aktif diringkas per tanggal retur (zona waktu outlet).
const byDateReturnsSQL = `
WITH
outlet_tz AS (
    SELECT id, timezone FROM outlets WHERE tenant_id = $1 AND id = ANY($2::uuid[])
),
day_range AS (
    SELECT generate_series($3::date, $4::date, '1 day'::interval)::date AS day
),
ret_hdr AS (
    SELECT
        (r.created_at AT TIME ZONE o.timezone)::date AS ret_date,
        count(*)                                       AS return_count,
        coalesce(sum(r.total), 0)                     AS return_total
    FROM sales_returns r
    JOIN outlet_tz o ON o.id = r.outlet_id
    JOIN sales s ON s.tenant_id = r.tenant_id AND s.id = r.sale_id
    WHERE r.tenant_id = $1
      AND r.outlet_id = ANY($2::uuid[])
      AND r.status = 'completed'
      AND r.created_at >= $6 AND r.created_at < $7
      AND ($5::uuid = '00000000-0000-0000-0000-000000000000' OR s.cashier_id = $5)
      AND (r.created_at AT TIME ZONE o.timezone)::date BETWEEN $3::date AND $4::date
    GROUP BY (r.created_at AT TIME ZONE o.timezone)::date
)
SELECT
    d.day::text                  AS date,
    coalesce(r.return_count, 0) AS return_count,
    coalesce(r.return_total, 0) AS return_total
FROM day_range d
LEFT JOIN ret_hdr r ON r.ret_date = d.day
ORDER BY d.day
`
