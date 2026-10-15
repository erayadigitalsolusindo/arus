package sales

// Laporan penjualan PER FAKTUR: satu baris per nota (tanpa isi barang).
//
// Siap big data (AGENTS.md backend):
//   - paginasi KEYSET (created_at, id) menurun — bukan OFFSET, tanpa count(*) OVER();
//   - rentang tanggal sargable + top-N per outlet (LATERAL … LIMIT): indeks
//     sales_outlet_time_idx (tenant_id, outlet_id, created_at DESC) dibaca
//     terurut dan berhenti di N baris — biaya tidak tumbuh dengan jumlah nota;
//   - ringkasan seluruh hasil hanya dihitung pada halaman pertama (tanpa cursor);
//     halaman berikutnya hanya membaca satu halaman nota + metode bayarnya;
//   - metode bayar diambil per halaman lewat `sale_id = ANY(...)`, tanpa JOIN
//     yang menggandakan baris sales;
//   - pencarian nomor faktur memakai ILIKE, dilayani indeks trigram sales_docno_trgm_idx;
//   - ekspor Excel dialirkan (StreamWriter) dan dibatasi maxInvoiceExportRows.

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/shopspring/decimal"
	"github.com/xuri/excelize/v2"

	"aciraba/internal/authz"
	"aciraba/internal/platform/db"
)

const (
	defaultInvoiceLimit = 100
	maxInvoiceLimit     = 300
	// maxInvoiceExportRows = batas faktur pada ekspor; melebihi ini hasil dipotong
	// dan result.Truncated = true.
	maxInvoiceExportRows = 50000
)

// InvoiceParams filter laporan penjualan per faktur.
type InvoiceParams struct {
	AllOutlets bool
	OutletID   string
	From, To   string
	CashierID  string
	Status     string // "", "completed", "void"
	Search     string // potongan nomor faktur
	Cursor     string
	Limit      int
	ForExport  bool
}

// InvoiceRow satu faktur.
type InvoiceRow struct {
	ID          string `json:"id"`
	DocNo       string `json:"doc_no"`
	Date        string `json:"date"`
	Time        string `json:"time"`
	Status      string `json:"status"`
	OutletCode  string `json:"outlet_code"`
	OutletName  string `json:"outlet_name"`
	CashierName string `json:"cashier_name"`
	MemberName  string `json:"member_name"`
	Salesperson string `json:"salesperson_name"`
	Subtotal    string `json:"subtotal"`
	Discount    string `json:"discount"`
	TaxStore    string `json:"tax_store"`
	TaxGov      string `json:"tax_gov"`
	OtherCost   string `json:"other_cost"`
	Total       string `json:"total"`
	Paid        string `json:"paid"`
	Change      string `json:"change"`
	Receivable  string `json:"receivable"`
	Methods     string `json:"methods"` // "Tunai 50.000,00; QRIS 20.000,00" → nama + nominal mentah
}

// InvoiceResult hasil laporan per faktur.
type InvoiceResult struct {
	From       string        `json:"from"`
	To         string        `json:"to"`
	TenantName string        `json:"tenant_name"`
	OutletName string        `json:"outlet_name"`
	Data       []InvoiceRow  `json:"data"`
	Summary    DetailSummary `json:"summary"`
	// HasSummary = false pada halaman lanjutan (ringkasan hanya di halaman pertama).
	HasSummary bool   `json:"has_summary"`
	NextCursor string `json:"next_cursor,omitempty"`
	Truncated  bool   `json:"truncated,omitempty"`
}

// invoiceOutletsCTE = outlet yang dibaca + batas waktu absolut rentang tanggal
// lokal masing-masing ([from 00:00, to+1 00:00) di zona waktu outlet). Batas
// dihitung per outlet (bukan fungsi pada kolom created_at) agar dapat dipakai
// indeks sales_outlet_time_idx.
const invoiceOutletsCTE = `
WITH ol AS (
  SELECT o.id, o.code, o.name, o.timezone,
         ($3::date)::timestamp AT TIME ZONE o.timezone AS lo,
         (($4::date) + 1)::timestamp AT TIME ZONE o.timezone AS hi
  FROM outlets o
  WHERE o.tenant_id = $1 AND o.id = ANY($2::uuid[])
)
`

const invoiceFilterSQL = `
      AND s.status <> 'superseded'
      AND ($5::uuid = '00000000-0000-0000-0000-000000000000' OR s.cashier_id = $5::uuid)
      AND ($6::text = '' OR s.status = $6::text)
      AND ($7::text = '' OR s.doc_no ILIKE '%' || $7::text || '%')
`

// invoiceRowsSQL: top-N per outlet lewat LATERAL (membaca indeks terurut dan
// berhenti di N baris), lalu digabung & dipotong lagi ke N. Biaya = O(outlet × N),
// tidak bergantung pada jumlah nota di rentang. Relasi (kasir, member, sales)
// di-join SETELAH dipotong, hanya untuk ≤ N baris.
const invoiceRowsSQL = invoiceOutletsCTE + `
SELECT s.id, s.doc_no, s.status, s.created_at,
       to_char(s.created_at AT TIME ZONE ol.timezone, 'YYYY-MM-DD'),
       to_char(s.created_at AT TIME ZONE ol.timezone, 'HH24:MI'),
       ol.code, ol.name,
       coalesce(u.name, ''), coalesce(mb.name, ''), coalesce(sp.name, ''),
       s.subtotal, s.discount, s.tax_store, s.tax_gov, s.other_cost,
       s.total, s.paid, s.change, s.receivable
FROM (
  SELECT l.*
  FROM ol
  CROSS JOIN LATERAL (
    SELECT s.id, s.outlet_id, s.doc_no, s.status, s.created_at,
           s.cashier_id, s.member_id, s.salesperson_id,
           s.subtotal, s.discount, s.tax_store, s.tax_gov, s.other_cost,
           s.total, s.paid, s.change, s.receivable
    FROM sales s
    WHERE s.tenant_id = $1 AND s.outlet_id = ol.id
      AND s.created_at >= ol.lo AND s.created_at < ol.hi
` + invoiceFilterSQL + `
      AND ($8::bool = false OR (s.created_at, s.id) < ($9::timestamptz, $10::uuid))
    ORDER BY s.created_at DESC, s.id DESC
    LIMIT $11
  ) l
  ORDER BY l.created_at DESC, l.id DESC
  LIMIT $11
) s
JOIN ol ON ol.id = s.outlet_id
LEFT JOIN users u ON u.tenant_id = $1 AND u.id = s.cashier_id
LEFT JOIN members mb ON mb.tenant_id = $1 AND mb.id = s.member_id
LEFT JOIN salespeople sp ON sp.tenant_id = $1 AND sp.id = s.salesperson_id
ORDER BY s.created_at DESC, s.id DESC
`

// invoiceSearchRowsSQL: dipakai saat ada pencarian nomor faktur. Pencarian
// ILIKE dilayani indeks trigram sales_docno_trgm_idx (kandidat sedikit), jadi
// bentuk datar + sort lebih murah daripada LATERAL yang memindai indeks waktu.
const invoiceSearchRowsSQL = invoiceOutletsCTE + `
SELECT s.id, s.doc_no, s.status, s.created_at,
       to_char(s.created_at AT TIME ZONE ol.timezone, 'YYYY-MM-DD'),
       to_char(s.created_at AT TIME ZONE ol.timezone, 'HH24:MI'),
       ol.code, ol.name,
       coalesce(u.name, ''), coalesce(mb.name, ''), coalesce(sp.name, ''),
       s.subtotal, s.discount, s.tax_store, s.tax_gov, s.other_cost,
       s.total, s.paid, s.change, s.receivable
FROM sales s
JOIN ol ON ol.id = s.outlet_id
LEFT JOIN users u ON u.tenant_id = $1 AND u.id = s.cashier_id
LEFT JOIN members mb ON mb.tenant_id = $1 AND mb.id = s.member_id
LEFT JOIN salespeople sp ON sp.tenant_id = $1 AND sp.id = s.salesperson_id
WHERE s.tenant_id = $1
  AND s.created_at >= ol.lo AND s.created_at < ol.hi
` + invoiceFilterSQL + `
  AND ($8::bool = false OR (s.created_at, s.id) < ($9::timestamptz, $10::uuid))
ORDER BY s.created_at DESC, s.id DESC
LIMIT $11
`

// Ringkasan: aturan sama dengan detailSummarySQL (uang hanya status uang,
// paid = netto paid − change). Hanya dijalankan pada halaman pertama.
const invoiceSummarySQL = invoiceOutletsCTE + `
SELECT count(*) FILTER (WHERE s.status = 'completed')::int,
       count(*) FILTER (WHERE s.status = 'void')::int,
       coalesce(sum(s.subtotal) FILTER (WHERE s.status = coalesce(nullif($6::text, ''), 'completed')), 0),
       coalesce(sum(s.discount) FILTER (WHERE s.status = coalesce(nullif($6::text, ''), 'completed')), 0),
       coalesce(sum(s.tax_store) FILTER (WHERE s.status = coalesce(nullif($6::text, ''), 'completed')), 0),
       coalesce(sum(s.tax_gov) FILTER (WHERE s.status = coalesce(nullif($6::text, ''), 'completed')), 0),
       coalesce(sum(s.total) FILTER (WHERE s.status = coalesce(nullif($6::text, ''), 'completed')), 0),
       coalesce(sum(s.paid - s.change) FILTER (WHERE s.status = coalesce(nullif($6::text, ''), 'completed')), 0),
       coalesce(sum(s.receivable) FILTER (WHERE s.status = coalesce(nullif($6::text, ''), 'completed')), 0)
FROM ol
JOIN sales s ON s.tenant_id = $1 AND s.outlet_id = ol.id
            AND s.created_at >= ol.lo AND s.created_at < ol.hi
` + invoiceFilterSQL + `
`

// escapeLike menetralkan wildcard LIKE pada input pencarian.
func escapeLike(s string) string {
	r := strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`)
	return r.Replace(s)
}

// Invoices: laporan penjualan per faktur.
func (s *Service) Invoices(ctx context.Context, a authz.Actor, p InvoiceParams) (InvoiceResult, error) {
	var result InvoiceResult

	err := db.WithTenant(ctx, s.pool, a.TenantID, func(tx pgx.Tx) error {
		var localDay pgtype.Date
		if err := tx.QueryRow(ctx, `
			SELECT (now() AT TIME ZONE timezone)::date FROM outlets WHERE tenant_id = $1 AND id = $2`,
			a.TenantID, a.OutletID).Scan(&localDay); err != nil {
			return err
		}
		today := time.Now().UTC()
		if localDay.Valid {
			today = localDay.Time
		}
		fd, td := today, today
		fe := FieldErrors{}
		if p.From != "" {
			var perr error
			if fd, perr = time.Parse("2006-01-02", p.From); perr != nil {
				fe["from"] = "INVALID"
			}
		}
		if p.To != "" {
			var perr error
			if td, perr = time.Parse("2006-01-02", p.To); perr != nil {
				fe["to"] = "INVALID"
			}
		}
		if len(fe) == 0 && td.Before(fd) {
			fe["to"] = "MUST_BE_AFTER_FROM"
		}
		if td.Sub(fd) > maxDetailDays*24*time.Hour {
			fe["to"] = "MAX_366_DAYS"
		}
		if len(fe) > 0 {
			return fe
		}

		status := strings.TrimSpace(p.Status)
		if status != "" && status != "completed" && status != "void" {
			return FieldErrors{"status": "INVALID"}
		}
		cashier := uuid.Nil
		if p.CashierID != "" {
			id, err := uuid.Parse(p.CashierID)
			if err != nil || id == uuid.Nil {
				return FieldErrors{"cashier_id": "INVALID"}
			}
			cashier = id
		}
		search := strings.TrimSpace(p.Search)
		if len(search) > 64 {
			return FieldErrors{"q": "TOO_LONG"}
		}
		search = escapeLike(search)

		hasCursor := p.Cursor != "" && !p.ForExport
		var cursorAt time.Time
		cursorID := uuid.Nil
		if hasCursor {
			var ok bool
			if cursorAt, cursorID, ok = detailDecodeCursor(p.Cursor); !ok {
				return FieldErrors{"cursor": "INVALID"}
			}
		}
		limit := p.Limit
		if limit <= 0 {
			limit = defaultInvoiceLimit
		}
		if limit > maxInvoiceLimit {
			limit = maxInvoiceLimit
		}
		rowLimit := limit + 1
		if p.ForExport {
			rowLimit = maxInvoiceExportRows + 1
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
				return FieldErrors{"outlet_id": "INVALID"}
			}
			if id != a.OutletID && !a.Outlets[id] {
				return ErrOutletForbidden
			}
			outlets = []uuid.UUID{id}
		}

		result.From, result.To = fd.Format("2006-01-02"), td.Format("2006-01-02")
		if err := tx.QueryRow(ctx, `SELECT name FROM tenants WHERE id = $1`, a.TenantID).Scan(&result.TenantName); err != nil {
			return err
		}
		if len(outlets) == 1 {
			if err := tx.QueryRow(ctx, `SELECT name FROM outlets WHERE tenant_id = $1 AND id = $2`,
				a.TenantID, outlets[0]).Scan(&result.OutletName); err != nil {
				return err
			}
		} else {
			result.OutletName = "Semua Outlet"
		}

		// Ringkasan hanya pada halaman pertama / ekspor.
		if !hasCursor {
			var sc, vc int
			var sub, disc, taxS, taxG, tot, paid, recv decimal.Decimal
			if err := tx.QueryRow(ctx, invoiceSummarySQL,
				a.TenantID, outlets, fd, td, cashier, status, search).
				Scan(&sc, &vc, &sub, &disc, &taxS, &taxG, &tot, &paid, &recv); err != nil {
				return err
			}
			result.HasSummary = true
			result.Summary = DetailSummary{
				SaleCount: sc, VoidCount: vc,
				Subtotal: sub.StringFixed(2), Discount: disc.StringFixed(2),
				TaxStore: taxS.StringFixed(2), TaxGov: taxG.StringFixed(2),
				Total: tot.StringFixed(2), Paid: paid.StringFixed(2),
				Receivable: recv.StringFixed(2),
			}
		}

		rowsSQL := invoiceRowsSQL
		if search != "" {
			rowsSQL = invoiceSearchRowsSQL
		}
		rows, err := tx.Query(ctx, rowsSQL,
			a.TenantID, outlets, fd, td, cashier, status, search,
			hasCursor, cursorAt, cursorID, rowLimit)
		if err != nil {
			return err
		}
		list := []InvoiceRow{}
		var stamps []time.Time
		for rows.Next() {
			var r InvoiceRow
			var id uuid.UUID
			var at time.Time
			var sub, disc, taxS, taxG, oc, tot, paid, chg, recv decimal.Decimal
			if err := rows.Scan(&id, &r.DocNo, &r.Status, &at, &r.Date, &r.Time,
				&r.OutletCode, &r.OutletName, &r.CashierName, &r.MemberName, &r.Salesperson,
				&sub, &disc, &taxS, &taxG, &oc, &tot, &paid, &chg, &recv); err != nil {
				rows.Close()
				return err
			}
			r.ID = id.String()
			r.Subtotal, r.Discount = sub.StringFixed(2), disc.StringFixed(2)
			r.TaxStore, r.TaxGov, r.OtherCost = taxS.StringFixed(2), taxG.StringFixed(2), oc.StringFixed(2)
			r.Total, r.Paid, r.Change, r.Receivable = tot.StringFixed(2), paid.StringFixed(2), chg.StringFixed(2), recv.StringFixed(2)
			list = append(list, r)
			stamps = append(stamps, at)
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return err
		}

		if p.ForExport {
			if len(list) > maxInvoiceExportRows {
				list = list[:maxInvoiceExportRows]
				result.Truncated = true
			}
		} else if len(list) > limit {
			list = list[:limit]
			result.NextCursor = detailEncodeCursor(stamps[limit-1], uuid.MustParse(list[limit-1].ID))
		}

		// Metode bayar: satu query per halaman (ekspor: per potongan 2000 id).
		for start := 0; start < len(list); start += 2000 {
			end := min(start+2000, len(list))
			ids := make([]uuid.UUID, 0, end-start)
			idx := make(map[string]int, end-start)
			for i := start; i < end; i++ {
				ids = append(ids, uuid.MustParse(list[i].ID))
				idx[list[i].ID] = i
			}
			mrows, err := tx.Query(ctx, detailMethodsSQL, a.TenantID, ids)
			if err != nil {
				return err
			}
			for mrows.Next() {
				var saleID uuid.UUID
				var name, kind string
				var amt decimal.Decimal
				if err := mrows.Scan(&saleID, &name, &kind, &amt); err != nil {
					mrows.Close()
					return err
				}
				if i, ok := idx[saleID.String()]; ok {
					if list[i].Methods != "" {
						list[i].Methods += "; "
					}
					list[i].Methods += name + " " + amt.StringFixed(2)
				}
			}
			mrows.Close()
			if err := mrows.Err(); err != nil {
				return err
			}
		}

		result.Data = list
		return nil
	})
	return result, err
}

// InvoiceFileName nama berkas ekspor per faktur.
func InvoiceFileName(from, to, ext string) string {
	return fmt.Sprintf("penjualan-perfaktur_%s_%s.%s", from, to, ext)
}

// InvoiceToXLSX: satu sheet, dialirkan per baris (memori tetap kecil).
func InvoiceToXLSX(res InvoiceResult) ([]byte, error) {
	f := excelize.NewFile()
	const sheet = "Per Faktur"
	if _, err := f.NewSheet(sheet); err != nil {
		return nil, err
	}
	f.DeleteSheet("Sheet1")
	sw, err := f.NewStreamWriter(sheet)
	if err != nil {
		return nil, err
	}
	head, err := f.NewStyle(&excelize.Style{Font: &excelize.Font{Bold: true}})
	if err != nil {
		return nil, err
	}
	num, err := f.NewStyle(&excelize.Style{NumFmt: 4})
	if err != nil {
		return nil, err
	}

	headers := []any{"Tanggal", "Jam", "Faktur", "Status", "Outlet", "Kasir", "Member", "Salesperson",
		"Subtotal", "Diskon", "Pajak Toko", "Pajak Negara", "Biaya Lain", "Total", "Diterima",
		"Kembalian", "Piutang", "Metode Bayar"}
	hrow := make([]any, len(headers))
	for i, h := range headers {
		hrow[i] = excelize.Cell{StyleID: head, Value: h}
	}
	if err := sw.SetRow("A1", hrow); err != nil {
		return nil, err
	}
	money := func(s string) any { return excelize.Cell{StyleID: num, Value: moneyFloat(s)} }
	r := 2
	for _, x := range res.Data {
		status := x.Status
		if status == "completed" {
			status = ""
		}
		cell, _ := excelize.CoordinatesToCellName(1, r)
		if err := sw.SetRow(cell, []any{
			x.Date, x.Time, x.DocNo, status, x.OutletCode, x.CashierName, x.MemberName, x.Salesperson,
			money(x.Subtotal), money(x.Discount), money(x.TaxStore), money(x.TaxGov), money(x.OtherCost),
			money(x.Total), money(x.Paid), money(x.Change), money(x.Receivable), x.Methods,
		}); err != nil {
			return nil, err
		}
		r++
	}
	if err := sw.Flush(); err != nil {
		return nil, err
	}
	buf, err := f.WriteToBuffer()
	if err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
