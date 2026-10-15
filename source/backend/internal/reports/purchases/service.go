// Package purchases provides purchase report endpoints (per faktur, per faktur
// detail, per barang, per tanggal).
//
// Siap big data (AGENTS.md backend): paginasi keyset (tanpa OFFSET/count besar),
// rentang tanggal sargable pada purchases_list_idx
// (tenant_id, outlet_id, purchase_date DESC, created_at DESC), top-N per outlet
// lewat LATERAL, ringkasan hanya di halaman pertama.
package purchases

import (
	"context"
	"encoding/base64"
	"errors"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/shopspring/decimal"

	"aciraba/internal/authz"
	"aciraba/internal/platform/db"
)

const (
	// ModuleReports = izin melihat laporan pembelian.
	ModuleReports = "purchase_reports"

	maxReportDays = 366
	defaultLimit  = 100
	maxLimit      = 300
	// maxExportRows = batas faktur/baris pada ekspor; melebihi ini hasil dipotong (Truncated).
	maxExportRows = 50000
	// maxPDFRows = batas baris PDF (berkas dibuat di memori).
	maxPDFRows = 5000
)

// ErrOutletForbidden = outlet yang diminta di luar akses pemanggil.
var ErrOutletForbidden = errors.New("OUTLET_FORBIDDEN")

// FieldErrors memetakan nama parameter ke kode galat validasi.
type FieldErrors map[string]string

func (f FieldErrors) Error() string {
	var b strings.Builder
	for k, v := range f {
		b.WriteString(k + ": " + v + " ")
	}
	return b.String()
}

// Service laporan pembelian.
type Service struct {
	pool *pgxpool.Pool
}

func NewService(pool *pgxpool.Pool) *Service { return &Service{pool: pool} }

// Common = filter bersama semua laporan pembelian.
type Common struct {
	AllOutlets  bool
	OutletID    string
	From, To    string
	SupplierID  string
	PaymentType string // "", "cash", "credit"
}

// Header = kop hasil (dipakai layar & ekspor).
type Header struct {
	From         string `json:"from"`
	To           string `json:"to"`
	TenantName   string `json:"tenant_name"`
	OutletName   string `json:"outlet_name"`
	OutletCode   string `json:"outlet_code"`
	SupplierName string `json:"supplier_name"`
}

// resolved = parameter yang sudah divalidasi.
type resolved struct {
	outlets  []uuid.UUID
	from, to time.Time
	supplier uuid.UUID
	payType  string
	hdr      Header
}

var zeroUUID = uuid.Nil

// resolve memvalidasi filter bersama dan mengisi kop. Dipanggil di dalam transaksi tenant.
func resolve(ctx context.Context, tx pgx.Tx, a authz.Actor, c Common) (resolved, error) {
	var r resolved
	var localDay pgtype.Date
	if err := tx.QueryRow(ctx, `
		SELECT (now() AT TIME ZONE timezone)::date FROM outlets WHERE tenant_id = $1 AND id = $2`,
		a.TenantID, a.OutletID).Scan(&localDay); err != nil {
		return r, err
	}
	today := time.Now().UTC()
	if localDay.Valid {
		today = localDay.Time
	}
	fd, td := today, today
	fe := FieldErrors{}
	var perr error
	if c.From != "" {
		if fd, perr = time.Parse("2006-01-02", c.From); perr != nil {
			fe["from"] = "INVALID"
		}
	}
	if c.To != "" {
		if td, perr = time.Parse("2006-01-02", c.To); perr != nil {
			fe["to"] = "INVALID"
		}
	}
	if len(fe) == 0 && td.Before(fd) {
		fe["to"] = "MUST_BE_AFTER_FROM"
	}
	if td.Sub(fd) > maxReportDays*24*time.Hour {
		fe["to"] = "MAX_366_DAYS"
	}
	if c.PaymentType != "" && c.PaymentType != "cash" && c.PaymentType != "credit" {
		fe["payment_type"] = "INVALID"
	}
	if c.SupplierID != "" {
		id, err := uuid.Parse(c.SupplierID)
		if err != nil || id == zeroUUID {
			fe["supplier_id"] = "INVALID"
		} else {
			r.supplier = id
		}
	}
	if len(fe) > 0 {
		return r, fe
	}
	r.from, r.to, r.payType = fd, td, c.PaymentType

	r.outlets = []uuid.UUID{a.OutletID}
	if c.AllOutlets && len(a.Outlets) > 0 {
		r.outlets = r.outlets[:0]
		for id := range a.Outlets {
			r.outlets = append(r.outlets, id)
		}
	}
	if c.OutletID != "" {
		id, err := uuid.Parse(c.OutletID)
		if err != nil || id == zeroUUID {
			return r, FieldErrors{"outlet_id": "INVALID"}
		}
		if id != a.OutletID && !a.Outlets[id] {
			return r, ErrOutletForbidden
		}
		r.outlets = []uuid.UUID{id}
	}

	r.hdr.From, r.hdr.To = fd.Format("2006-01-02"), td.Format("2006-01-02")
	if err := tx.QueryRow(ctx, `SELECT name FROM tenants WHERE id = $1`, a.TenantID).Scan(&r.hdr.TenantName); err != nil {
		return r, err
	}
	if len(r.outlets) == 1 {
		if err := tx.QueryRow(ctx, `SELECT name, code FROM outlets WHERE tenant_id = $1 AND id = $2`,
			a.TenantID, r.outlets[0]).Scan(&r.hdr.OutletName, &r.hdr.OutletCode); err != nil {
			return r, err
		}
	} else {
		r.hdr.OutletName = "Semua Outlet"
	}
	if r.supplier != zeroUUID {
		if err := tx.QueryRow(ctx, `SELECT name FROM suppliers WHERE tenant_id = $1 AND id = $2`,
			a.TenantID, r.supplier).Scan(&r.hdr.SupplierName); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return r, FieldErrors{"supplier_id": "NOT_FOUND"}
			}
			return r, err
		}
	}
	return r, nil
}

func clampLimit(limit int) int {
	if limit <= 0 {
		return defaultLimit
	}
	return min(limit, maxLimit)
}

// escapeLike menetralkan wildcard LIKE pada input pencarian.
func escapeLike(s string) string {
	return strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(s)
}

func money(d decimal.Decimal) string { return d.StringFixed(2) }

// cursor keyset (purchase_date, created_at, id).
func encodeCursor(day string, t time.Time, id uuid.UUID) string {
	return base64.RawURLEncoding.EncodeToString([]byte(day + "|" + strconv.FormatInt(t.UnixMicro(), 10) + "|" + id.String()))
}

func decodeCursor(c string) (time.Time, time.Time, uuid.UUID, bool) {
	raw, err := base64.RawURLEncoding.DecodeString(c)
	if err != nil {
		return time.Time{}, time.Time{}, zeroUUID, false
	}
	parts := strings.Split(string(raw), "|")
	if len(parts) != 3 {
		return time.Time{}, time.Time{}, zeroUUID, false
	}
	day, err1 := time.Parse("2006-01-02", parts[0])
	us, err2 := strconv.ParseInt(parts[1], 10, 64)
	id, err3 := uuid.Parse(parts[2])
	if err1 != nil || err2 != nil || err3 != nil {
		return time.Time{}, time.Time{}, zeroUUID, false
	}
	return day, time.UnixMicro(us).UTC(), id, true
}

// ── Per faktur & per faktur detail ───────────────────────────────────────────

// InvoiceParams filter laporan pembelian per faktur / per faktur detail.
type InvoiceParams struct {
	Common
	Status    string // "", "completed", "void"
	Search    string // potongan nomor faktur / nomor faktur pemasok
	Cursor    string
	Limit     int
	ForExport bool
	ExportMax int
	// WithLines = sertakan baris barang (per faktur detail).
	WithLines bool
}

// LineRow satu baris barang pada faktur.
type LineRow struct {
	Position  int    `json:"position"`
	SKU       string `json:"sku"`
	Name      string `json:"name"`
	Unit      string `json:"unit"`
	Qty       string `json:"qty"`
	UnitPrice string `json:"unit_price"`
	Disc1     string `json:"disc1"`
	Disc2     string `json:"disc2"`
	Disc3     string `json:"disc3"`
	Disc4     string `json:"disc4"`
	LineTotal string `json:"line_total"`
	UnitCost  string `json:"unit_cost"`
}

// InvoiceRow satu faktur pembelian.
type InvoiceRow struct {
	ID                string    `json:"id"`
	DocNo             string    `json:"doc_no"`
	SupplierInvoiceNo string    `json:"supplier_invoice_no"`
	Date              string    `json:"date"`
	Status            string    `json:"status"`
	OutletCode        string    `json:"outlet_code"`
	OutletName        string    `json:"outlet_name"`
	SupplierName      string    `json:"supplier_name"`
	PaymentType       string    `json:"payment_type"`
	DueDate           string    `json:"due_date"`
	Note              string    `json:"note"`
	Subtotal          string    `json:"subtotal"`
	TaxPct            string    `json:"tax_pct"`
	TaxAmount         string    `json:"tax_amount"`
	OtherCost         string    `json:"other_cost"`
	Total             string    `json:"total"`
	Lines             []LineRow `json:"lines,omitempty"`
}

// InvoiceSummary total seluruh hasil filter (hanya status completed untuk uang).
type InvoiceSummary struct {
	PurchaseCount int    `json:"purchase_count"`
	VoidCount     int    `json:"void_count"`
	CashCount     int    `json:"cash_count"`
	CreditCount   int    `json:"credit_count"`
	Subtotal      string `json:"subtotal"`
	TaxAmount     string `json:"tax_amount"`
	OtherCost     string `json:"other_cost"`
	Total         string `json:"total"`
	CashTotal     string `json:"cash_total"`
	CreditTotal   string `json:"credit_total"`
}

// InvoiceResult hasil laporan per faktur.
type InvoiceResult struct {
	Header
	StatusLabel string         `json:"status_label"`
	Data        []InvoiceRow   `json:"data"`
	Summary     InvoiceSummary `json:"summary"`
	HasSummary  bool           `json:"has_summary"`
	NextCursor  string         `json:"next_cursor,omitempty"`
	Truncated   bool           `json:"truncated,omitempty"`
}

const outletsCTE = `
WITH ol AS (
  SELECT o.id, o.code, o.name FROM outlets o
  WHERE o.tenant_id = $1 AND o.id = ANY($2::uuid[])
)
`

// $1 tenant, $2 outlets, $3 from, $4 to, $5 supplier, $6 payment_type, $7 status, $8 search
const filterSQL = `
      AND p.status <> 'superseded'
      AND ($5::uuid = '00000000-0000-0000-0000-000000000000' OR p.supplier_id = $5::uuid)
      AND ($6::text = '' OR p.payment_type = $6::text)
      AND ($7::text = '' OR p.status = $7::text)
      AND ($8::text = '' OR p.doc_no ILIKE '%' || $8::text || '%' OR p.supplier_invoice_no ILIKE '%' || $8::text || '%')
`

const invoiceCols = `p.id, p.doc_no, p.supplier_invoice_no, p.purchase_date, p.status, p.created_at, p.outlet_id,
       p.supplier_id, p.payment_type, p.due_date, p.note, p.subtotal, p.tax_pct, p.tax_amount, p.other_cost, p.total`

// invoiceRowsSQL: top-N per outlet (LATERAL) lalu digabung; relasi di-join setelah dipotong.
const invoiceRowsSQL = outletsCTE + `
SELECT s.id, s.doc_no, s.supplier_invoice_no, s.purchase_date, s.status, s.created_at,
       ol.code, ol.name, coalesce(sp.name, ''), s.payment_type, s.due_date, s.note,
       s.subtotal, s.tax_pct, s.tax_amount, s.other_cost, s.total
FROM (
  SELECT l.* FROM ol
  CROSS JOIN LATERAL (
    SELECT ` + invoiceCols + `
    FROM purchases p
    WHERE p.tenant_id = $1 AND p.outlet_id = ol.id
      AND p.purchase_date >= $3::date AND p.purchase_date <= $4::date
` + filterSQL + `
      AND ($9::bool = false OR (p.purchase_date, p.created_at, p.id) < ($10::date, $11::timestamptz, $12::uuid))
    ORDER BY p.purchase_date DESC, p.created_at DESC, p.id DESC
    LIMIT $13
  ) l
  ORDER BY l.purchase_date DESC, l.created_at DESC, l.id DESC
  LIMIT $13
) s
JOIN ol ON ol.id = s.outlet_id
LEFT JOIN suppliers sp ON sp.tenant_id = $1 AND sp.id = s.supplier_id
ORDER BY s.purchase_date DESC, s.created_at DESC, s.id DESC
`

// invoiceSearchRowsSQL: pencarian ILIKE dilayani indeks trigram (kandidat sedikit).
const invoiceSearchRowsSQL = outletsCTE + `
SELECT p.id, p.doc_no, p.supplier_invoice_no, p.purchase_date, p.status, p.created_at,
       ol.code, ol.name, coalesce(sp.name, ''), p.payment_type, p.due_date, p.note,
       p.subtotal, p.tax_pct, p.tax_amount, p.other_cost, p.total
FROM purchases p
JOIN ol ON ol.id = p.outlet_id
LEFT JOIN suppliers sp ON sp.tenant_id = $1 AND sp.id = p.supplier_id
WHERE p.tenant_id = $1
  AND p.purchase_date >= $3::date AND p.purchase_date <= $4::date
` + filterSQL + `
  AND ($9::bool = false OR (p.purchase_date, p.created_at, p.id) < ($10::date, $11::timestamptz, $12::uuid))
ORDER BY p.purchase_date DESC, p.created_at DESC, p.id DESC
LIMIT $13
`

// invoiceSummarySQL: uang hanya status completed (void tidak ikut total).
const invoiceSummarySQL = outletsCTE + `
SELECT count(*) FILTER (WHERE p.status = 'completed')::int,
       count(*) FILTER (WHERE p.status = 'void')::int,
       count(*) FILTER (WHERE p.status = 'completed' AND p.payment_type = 'cash')::int,
       count(*) FILTER (WHERE p.status = 'completed' AND p.payment_type = 'credit')::int,
       coalesce(sum(p.subtotal) FILTER (WHERE p.status = 'completed'), 0),
       coalesce(sum(p.tax_amount) FILTER (WHERE p.status = 'completed'), 0),
       coalesce(sum(p.other_cost) FILTER (WHERE p.status = 'completed'), 0),
       coalesce(sum(p.total) FILTER (WHERE p.status = 'completed'), 0),
       coalesce(sum(p.total) FILTER (WHERE p.status = 'completed' AND p.payment_type = 'cash'), 0),
       coalesce(sum(p.total) FILTER (WHERE p.status = 'completed' AND p.payment_type = 'credit'), 0)
FROM ol
JOIN purchases p ON p.tenant_id = $1 AND p.outlet_id = ol.id
                AND p.purchase_date >= $3::date AND p.purchase_date <= $4::date
WHERE true
` + filterSQL

const linesSQL = `
SELECT purchase_id, position, item_sku, item_name, unit_name, qty, unit_price,
       disc1, disc2, disc3, disc4, line_total, unit_cost
FROM purchase_lines
WHERE tenant_id = $1 AND purchase_id = ANY($2::uuid[])
ORDER BY purchase_id, position
`

// Invoices: laporan pembelian per faktur (WithLines = per faktur detail).
func (s *Service) Invoices(ctx context.Context, a authz.Actor, p InvoiceParams) (InvoiceResult, error) {
	var result InvoiceResult
	err := db.WithTenant(ctx, s.pool, a.TenantID, func(tx pgx.Tx) error {
		rs, err := resolve(ctx, tx, a, p.Common)
		if err != nil {
			return err
		}
		result.Header = rs.hdr

		status := strings.TrimSpace(p.Status)
		if status != "" && status != "completed" && status != "void" {
			return FieldErrors{"status": "INVALID"}
		}
		switch status {
		case "completed":
			result.StatusLabel = "Completed"
		case "void":
			result.StatusLabel = "Void"
		}
		search := strings.TrimSpace(p.Search)
		if len(search) > 64 {
			return FieldErrors{"q": "TOO_LONG"}
		}
		search = escapeLike(search)

		hasCursor := p.Cursor != "" && !p.ForExport
		var curDay, curAt time.Time
		curID := zeroUUID
		if hasCursor {
			var ok bool
			if curDay, curAt, curID, ok = decodeCursor(p.Cursor); !ok {
				return FieldErrors{"cursor": "INVALID"}
			}
		}
		limit := clampLimit(p.Limit)
		rowLimit := limit + 1
		exportMax := maxExportRows
		if p.ExportMax > 0 && p.ExportMax < exportMax {
			exportMax = p.ExportMax
		}
		if p.ForExport {
			rowLimit = exportMax + 1
		}

		if !hasCursor {
			var sm InvoiceSummary
			var sub, tax, oc, tot, cash, credit decimal.Decimal
			if err := tx.QueryRow(ctx, invoiceSummarySQL,
				a.TenantID, rs.outlets, rs.from, rs.to, rs.supplier, rs.payType, status, search).
				Scan(&sm.PurchaseCount, &sm.VoidCount, &sm.CashCount, &sm.CreditCount,
					&sub, &tax, &oc, &tot, &cash, &credit); err != nil {
				return err
			}
			sm.Subtotal, sm.TaxAmount, sm.OtherCost = money(sub), money(tax), money(oc)
			sm.Total, sm.CashTotal, sm.CreditTotal = money(tot), money(cash), money(credit)
			result.Summary, result.HasSummary = sm, true
		}

		rowsSQL := invoiceRowsSQL
		if search != "" {
			rowsSQL = invoiceSearchRowsSQL
		}
		rows, err := tx.Query(ctx, rowsSQL,
			a.TenantID, rs.outlets, rs.from, rs.to, rs.supplier, rs.payType, status, search,
			hasCursor, curDay, curAt, curID, rowLimit)
		if err != nil {
			return err
		}
		list := []InvoiceRow{}
		var stamps []time.Time
		var days []time.Time
		for rows.Next() {
			var r InvoiceRow
			var id uuid.UUID
			var day pgtype.Date
			var due pgtype.Date
			var at time.Time
			var sub, pct, tax, oc, tot decimal.Decimal
			if err := rows.Scan(&id, &r.DocNo, &r.SupplierInvoiceNo, &day, &r.Status, &at,
				&r.OutletCode, &r.OutletName, &r.SupplierName, &r.PaymentType, &due, &r.Note,
				&sub, &pct, &tax, &oc, &tot); err != nil {
				rows.Close()
				return err
			}
			r.ID = id.String()
			r.Date = day.Time.Format("2006-01-02")
			if due.Valid {
				r.DueDate = due.Time.Format("2006-01-02")
			}
			r.Subtotal, r.TaxPct, r.TaxAmount, r.OtherCost, r.Total = money(sub), pct.StringFixed(2), money(tax), money(oc), money(tot)
			list = append(list, r)
			stamps = append(stamps, at)
			days = append(days, day.Time)
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return err
		}

		if p.ForExport {
			if len(list) > exportMax {
				list = list[:exportMax]
				result.Truncated = true
			}
		} else if len(list) > limit {
			list = list[:limit]
			result.NextCursor = encodeCursor(days[limit-1].Format("2006-01-02"), stamps[limit-1], uuid.MustParse(list[limit-1].ID))
		}

		if p.WithLines {
			for start := 0; start < len(list); start += 2000 {
				end := min(start+2000, len(list))
				ids := make([]uuid.UUID, 0, end-start)
				idx := make(map[string]int, end-start)
				for i := start; i < end; i++ {
					ids = append(ids, uuid.MustParse(list[i].ID))
					idx[list[i].ID] = i
				}
				lrows, err := tx.Query(ctx, linesSQL, a.TenantID, ids)
				if err != nil {
					return err
				}
				for lrows.Next() {
					var pid uuid.UUID
					var l LineRow
					var qty, price, d1, d2, d3, d4, lt, uc decimal.Decimal
					if err := lrows.Scan(&pid, &l.Position, &l.SKU, &l.Name, &l.Unit, &qty, &price,
						&d1, &d2, &d3, &d4, &lt, &uc); err != nil {
						lrows.Close()
						return err
					}
					l.Qty, l.UnitPrice = qty.String(), money(price)
					l.Disc1, l.Disc2, l.Disc3, l.Disc4 = d1.String(), d2.String(), d3.String(), d4.String()
					l.LineTotal, l.UnitCost = money(lt), money(uc)
					if i, ok := idx[pid.String()]; ok {
						list[i].Lines = append(list[i].Lines, l)
					}
				}
				lrows.Close()
				if err := lrows.Err(); err != nil {
					return err
				}
			}
		}
		result.Data = list
		return nil
	})
	return result, err
}

// ── Per barang ───────────────────────────────────────────────────────────────

// ItemParams filter laporan pembelian per barang.
type ItemParams struct {
	Common
	CategoryID string
	Search     string // nama / SKU barang
	Sort       string // total (default) | qty | name
	Limit      int
	ForExport  bool
	ExportMax  int
}

// ItemRow satu barang.
type ItemRow struct {
	ItemID        string `json:"item_id"`
	SKU           string `json:"sku"`
	Name          string `json:"name"`
	Unit          string `json:"unit"`
	PurchaseCount int    `json:"purchase_count"`
	Qty           string `json:"qty"`
	Total         string `json:"total"`
	AvgPrice      string `json:"avg_price"`
	LastPrice     string `json:"last_price"`
}

// ItemSummary total seluruh barang yang cocok filter (bukan hanya yang tampil).
type ItemSummary struct {
	ItemCount int    `json:"item_count"`
	Total     string `json:"total"`
}

// ItemResult hasil laporan per barang.
type ItemResult struct {
	Header
	Data      []ItemRow   `json:"data"`
	Summary   ItemSummary `json:"summary"`
	Truncated bool        `json:"truncated,omitempty"`
}

// itemsSQL: agregasi per barang untuk faktur completed. $1 tenant, $2 outlets, $3 from,
// $4 to, $5 supplier, $6 payment_type, $7 category, $8 search, $9 sort, $10 limit.
// Satu pemindaian: total seluruh hasil lewat window (count/sum OVER) pada baris agregat.
const itemsSQL = `
WITH agg AS (
  SELECT l.item_id,
         count(DISTINCT p.id)::int AS purchase_count,
         sum(l.qty) AS qty,
         sum(l.line_total) AS total,
         (array_agg(l.unit_price ORDER BY p.purchase_date DESC, p.created_at DESC))[1] AS last_price,
         (array_agg(l.item_sku ORDER BY p.purchase_date DESC, p.created_at DESC))[1] AS sku,
         (array_agg(l.item_name ORDER BY p.purchase_date DESC, p.created_at DESC))[1] AS name,
         (array_agg(l.unit_name ORDER BY p.purchase_date DESC, p.created_at DESC))[1] AS unit
  FROM purchases p
  JOIN purchase_lines l ON l.tenant_id = p.tenant_id AND l.purchase_id = p.id
  WHERE p.tenant_id = $1 AND p.outlet_id = ANY($2::uuid[])
    AND p.purchase_date >= $3::date AND p.purchase_date <= $4::date
    AND p.status = 'completed'
    AND ($5::uuid = '00000000-0000-0000-0000-000000000000' OR p.supplier_id = $5::uuid)
    AND ($6::text = '' OR p.payment_type = $6::text)
    AND ($7::uuid = '00000000-0000-0000-0000-000000000000' OR EXISTS (
          SELECT 1 FROM items i WHERE i.tenant_id = l.tenant_id AND i.id = l.item_id AND i.category_id = $7::uuid))
    AND ($8::text = '' OR l.item_name ILIKE '%' || $8::text || '%' OR l.item_sku ILIKE '%' || $8::text || '%')
  GROUP BY l.item_id
)
SELECT item_id, sku, name, unit, purchase_count, qty, total, last_price,
       count(*) OVER ()::int, coalesce(sum(total) OVER (), 0)
FROM agg
ORDER BY
  CASE WHEN $9::text = 'name' THEN lower(name) END ASC,
  CASE WHEN $9::text = 'qty' THEN qty END DESC,
  CASE WHEN $9::text NOT IN ('name', 'qty') THEN total END DESC,
  item_id
LIMIT $10
`

// Items: laporan pembelian per barang.
func (s *Service) Items(ctx context.Context, a authz.Actor, p ItemParams) (ItemResult, error) {
	var result ItemResult
	err := db.WithTenant(ctx, s.pool, a.TenantID, func(tx pgx.Tx) error {
		rs, err := resolve(ctx, tx, a, p.Common)
		if err != nil {
			return err
		}
		result.Header = rs.hdr
		category := zeroUUID
		if p.CategoryID != "" {
			id, err := uuid.Parse(p.CategoryID)
			if err != nil || id == zeroUUID {
				return FieldErrors{"category_id": "INVALID"}
			}
			category = id
		}
		search := strings.TrimSpace(p.Search)
		if len(search) > 64 {
			return FieldErrors{"q": "TOO_LONG"}
		}
		search = escapeLike(search)
		sortKey := p.Sort
		if sortKey != "name" && sortKey != "qty" {
			sortKey = "total"
		}
		limit := clampLimit(p.Limit)
		if p.ForExport {
			limit = maxExportRows
			if p.ExportMax > 0 {
				limit = p.ExportMax
			}
		}
		if _, err := tx.Exec(ctx, "SET LOCAL work_mem = '64MB'"); err != nil {
			return err
		}
		rows, err := tx.Query(ctx, itemsSQL, a.TenantID, rs.outlets, rs.from, rs.to,
			rs.supplier, rs.payType, category, search, sortKey, limit)
		if err != nil {
			return err
		}
		defer rows.Close()
		result.Data = []ItemRow{}
		var grand decimal.Decimal
		for rows.Next() {
			var r ItemRow
			var id uuid.UUID
			var qty, total, last decimal.Decimal
			var n int
			if err := rows.Scan(&id, &r.SKU, &r.Name, &r.Unit, &r.PurchaseCount, &qty, &total, &last, &n, &grand); err != nil {
				return err
			}
			r.ItemID, r.Qty, r.Total, r.LastPrice = id.String(), qty.String(), money(total), money(last)
			if qty.IsPositive() {
				r.AvgPrice = money(total.Div(qty).Round(2))
			} else {
				r.AvgPrice = "0.00"
			}
			result.Summary.ItemCount = n
			result.Data = append(result.Data, r)
		}
		if err := rows.Err(); err != nil {
			return err
		}
		result.Summary.Total = money(grand)
		result.Truncated = p.ForExport && result.Summary.ItemCount > len(result.Data)
		return nil
	})
	return result, err
}

// ── Per tanggal ──────────────────────────────────────────────────────────────

// ByDateParams filter laporan pembelian per tanggal.
type ByDateParams struct {
	Common
	HideEmpty bool
}

// DayRow satu tanggal.
type DayRow struct {
	Date          string `json:"date"`
	PurchaseCount int    `json:"purchase_count"`
	VoidCount     int    `json:"void_count"`
	Subtotal      string `json:"subtotal"`
	TaxAmount     string `json:"tax_amount"`
	OtherCost     string `json:"other_cost"`
	Total         string `json:"total"`
	CashTotal     string `json:"cash_total"`
	CreditTotal   string `json:"credit_total"`
	VoidTotal     string `json:"void_total"`
	ReturnCount   int    `json:"return_count"`
	ReturnTotal   string `json:"return_total"`
	NetTotal      string `json:"net_total"` // total − retur (void tidak ikut)
}

// ByDateResult hasil laporan per tanggal.
type ByDateResult struct {
	Header
	Data   []DayRow `json:"data"`
	Totals DayRow   `json:"totals"`
}

// byDateSQL: agregasi harian faktur (sargable per hari bisnis) + retur per tanggal retur.
// Retur tidak ikut filter metode bayar; mengikuti filter pemasok dan outlet.
const byDateSQL = `
WITH days AS (
  SELECT generate_series($3::date, $4::date, '1 day'::interval)::date AS day
),
pur AS (
  SELECT p.purchase_date AS day,
         count(*) FILTER (WHERE p.status = 'completed')::int AS cnt,
         count(*) FILTER (WHERE p.status = 'void')::int AS void_cnt,
         coalesce(sum(p.subtotal) FILTER (WHERE p.status = 'completed'), 0) AS subtotal,
         coalesce(sum(p.tax_amount) FILTER (WHERE p.status = 'completed'), 0) AS tax,
         coalesce(sum(p.other_cost) FILTER (WHERE p.status = 'completed'), 0) AS other_cost,
         coalesce(sum(p.total) FILTER (WHERE p.status = 'completed'), 0) AS total,
         coalesce(sum(p.total) FILTER (WHERE p.status = 'completed' AND p.payment_type = 'cash'), 0) AS cash_total,
         coalesce(sum(p.total) FILTER (WHERE p.status = 'completed' AND p.payment_type = 'credit'), 0) AS credit_total,
         coalesce(sum(p.total) FILTER (WHERE p.status = 'void'), 0) AS void_total
  FROM purchases p
  WHERE p.tenant_id = $1 AND p.outlet_id = ANY($2::uuid[])
    AND p.purchase_date >= $3::date AND p.purchase_date <= $4::date
    AND p.status IN ('completed', 'void')
    AND ($5::uuid = '00000000-0000-0000-0000-000000000000' OR p.supplier_id = $5::uuid)
    AND ($6::text = '' OR p.payment_type = $6::text)
  GROUP BY p.purchase_date
),
ret AS (
  SELECT r.return_date AS day, count(*)::int AS cnt, coalesce(sum(r.total), 0) AS total
  FROM purchase_returns r
  WHERE r.tenant_id = $1 AND r.outlet_id = ANY($2::uuid[])
    AND r.return_date >= $3::date AND r.return_date <= $4::date
    AND r.status = 'completed'
    AND ($5::uuid = '00000000-0000-0000-0000-000000000000' OR r.supplier_id = $5::uuid)
  GROUP BY r.return_date
)
SELECT d.day,
       coalesce(p.cnt, 0), coalesce(p.void_cnt, 0),
       coalesce(p.subtotal, 0), coalesce(p.tax, 0), coalesce(p.other_cost, 0), coalesce(p.total, 0),
       coalesce(p.cash_total, 0), coalesce(p.credit_total, 0), coalesce(p.void_total, 0),
       coalesce(r.cnt, 0), coalesce(r.total, 0)
FROM days d
LEFT JOIN pur p ON p.day = d.day
LEFT JOIN ret r ON r.day = d.day
ORDER BY d.day
`

// ByDate: laporan pembelian per tanggal (deret tanggal penuh ≤366 baris).
func (s *Service) ByDate(ctx context.Context, a authz.Actor, p ByDateParams) (ByDateResult, error) {
	var result ByDateResult
	err := db.WithTenant(ctx, s.pool, a.TenantID, func(tx pgx.Tx) error {
		rs, err := resolve(ctx, tx, a, p.Common)
		if err != nil {
			return err
		}
		result.Header = rs.hdr
		rows, err := tx.Query(ctx, byDateSQL, a.TenantID, rs.outlets, rs.from, rs.to, rs.supplier, rs.payType)
		if err != nil {
			return err
		}
		defer rows.Close()
		result.Data = []DayRow{}
		var gSub, gTax, gOC, gTot, gCash, gCred, gVoid, gRet decimal.Decimal
		var gCnt, gVoidCnt, gRetCnt int
		for rows.Next() {
			var r DayRow
			var day pgtype.Date
			var sub, tax, oc, tot, cash, cred, voidT, ret decimal.Decimal
			if err := rows.Scan(&day, &r.PurchaseCount, &r.VoidCount, &sub, &tax, &oc, &tot, &cash, &cred, &voidT,
				&r.ReturnCount, &ret); err != nil {
				return err
			}
			if p.HideEmpty && r.PurchaseCount == 0 && r.VoidCount == 0 && r.ReturnCount == 0 {
				continue
			}
			r.Date = day.Time.Format("2006-01-02")
			r.Subtotal, r.TaxAmount, r.OtherCost, r.Total = money(sub), money(tax), money(oc), money(tot)
			r.CashTotal, r.CreditTotal, r.VoidTotal, r.ReturnTotal = money(cash), money(cred), money(voidT), money(ret)
			r.NetTotal = money(tot.Sub(ret))
			result.Data = append(result.Data, r)
			gCnt += r.PurchaseCount
			gVoidCnt += r.VoidCount
			gRetCnt += r.ReturnCount
			gSub, gTax, gOC, gTot = gSub.Add(sub), gTax.Add(tax), gOC.Add(oc), gTot.Add(tot)
			gCash, gCred, gVoid, gRet = gCash.Add(cash), gCred.Add(cred), gVoid.Add(voidT), gRet.Add(ret)
		}
		if err := rows.Err(); err != nil {
			return err
		}
		result.Totals = DayRow{
			PurchaseCount: gCnt, VoidCount: gVoidCnt, ReturnCount: gRetCnt,
			Subtotal: money(gSub), TaxAmount: money(gTax), OtherCost: money(gOC), Total: money(gTot),
			CashTotal: money(gCash), CreditTotal: money(gCred), VoidTotal: money(gVoid),
			ReturnTotal: money(gRet), NetTotal: money(gTot.Sub(gRet)),
		}
		return nil
	})
	return result, err
}
