package sales

// Laporan penjualan PER PELANGGAN (member): omset per pelanggan dan riwayat
// faktur per pelanggan (opsional sampai isi barang). Nota tanpa member (non
// member) TIDAK ikut. Hanya nota berstatus completed (void/superseded tidak).
//
// Siap big data (AGENTS.md backend):
//   - omset: satu agregat GROUP BY member_id atas nota di rentang, dibaca lewat
//     sales_outlet_time_idx (batas waktu per outlet, sargable); master member
//     di-join SETELAH agregasi (∝ pelanggan yang membeli di rentang);
//   - total keseluruhan lewat window function pada agregat yang sama;
//   - hasil dibatasi LIMIT (layar default 100 maks 1000; ekspor dibatasi);
//   - faktur per pelanggan dibaca lewat sales_member_idx
//     (tenant_id, member_id, created_at DESC) dengan paginasi KEYSET
//     (created_at, id) — bukan OFFSET; isi barang diambil per halaman lewat
//     `sale_id = ANY(...)` (UNIQUE (sale_id, position)).

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/shopspring/decimal"

	"aciraba/internal/authz"
	"aciraba/internal/platform/db"
)

const (
	defaultCustomerLimit = 100
	maxCustomerLimit     = 1000
	// maxCustomerExportRows = batas pelanggan ekspor omset; PDF/XLSX faktur per
	// pelanggan memakai batas pelanggan & faktur di bawah.
	maxCustomerExportRows = 20000
	maxCustomerPDFRows    = 3000

	defaultCustInvLimit = 50
	maxCustInvLimit     = 200
	// maxCustInvExportCustomers/Sales = batas ekspor faktur per pelanggan.
	maxCustInvExportCustomers = 500
	maxCustInvExportSales     = 5000
)

// reportScope = parameter umum laporan (rentang tanggal lokal outlet, outlet,
// kasir) yang sudah divalidasi + label kop.
type reportScope struct {
	From, To    time.Time
	Outlets     []uuid.UUID
	Cashier     uuid.UUID
	Loc         *time.Location
	TenantName  string
	OutletName  string
	OutletCode  string
	CashierName string
}

// resolveScope memvalidasi tanggal/outlet/kasir dan memuat label kop. Outlet
// diambil dari token; all_outlets hanya untuk outlet yang boleh diakses.
func (s *Service) resolveScope(ctx context.Context, tx pgx.Tx, a authz.Actor,
	from, to, outletID, cashierID string, allOutlets bool) (reportScope, error) {
	var sc reportScope
	var localDay pgtype.Date
	var tz string
	if err := tx.QueryRow(ctx, `
		SELECT (now() AT TIME ZONE timezone)::date, timezone FROM outlets WHERE tenant_id = $1 AND id = $2`,
		a.TenantID, a.OutletID).Scan(&localDay, &tz); err != nil {
		return sc, err
	}
	sc.Loc = time.UTC
	if l, err := time.LoadLocation(tz); err == nil {
		sc.Loc = l
	}
	today := time.Now().UTC()
	if localDay.Valid {
		today = localDay.Time
	}
	fd, td := today, today
	fe := FieldErrors{}
	if from != "" {
		var perr error
		if fd, perr = time.Parse("2006-01-02", from); perr != nil {
			fe["from"] = "INVALID"
		}
	}
	if to != "" {
		var perr error
		if td, perr = time.Parse("2006-01-02", to); perr != nil {
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
		return sc, fe
	}
	sc.From, sc.To = fd, td

	if cashierID != "" {
		id, err := uuid.Parse(cashierID)
		if err != nil || id == uuid.Nil {
			return sc, FieldErrors{"cashier_id": "INVALID"}
		}
		sc.Cashier = id
		if err := tx.QueryRow(ctx, `SELECT name FROM users WHERE tenant_id = $1 AND id = $2`,
			a.TenantID, id).Scan(&sc.CashierName); err != nil {
			return sc, err
		}
	}

	sc.Outlets = []uuid.UUID{a.OutletID}
	if allOutlets && len(a.Outlets) > 0 {
		sc.Outlets = sc.Outlets[:0]
		for id := range a.Outlets {
			sc.Outlets = append(sc.Outlets, id)
		}
	}
	if outletID != "" {
		id, err := uuid.Parse(outletID)
		if err != nil || id == uuid.Nil {
			return sc, FieldErrors{"outlet_id": "INVALID"}
		}
		if id != a.OutletID && !a.Outlets[id] {
			return sc, ErrOutletForbidden
		}
		sc.Outlets = []uuid.UUID{id}
	}

	if err := tx.QueryRow(ctx, `SELECT name FROM tenants WHERE id = $1`, a.TenantID).Scan(&sc.TenantName); err != nil {
		return sc, err
	}
	if len(sc.Outlets) == 1 {
		if err := tx.QueryRow(ctx, `SELECT name, code FROM outlets WHERE tenant_id = $1 AND id = $2`,
			a.TenantID, sc.Outlets[0]).Scan(&sc.OutletName, &sc.OutletCode); err != nil {
			return sc, err
		}
	} else {
		sc.OutletName = "Semua Outlet"
	}
	return sc, nil
}

// searchPatterns memecah pencarian jadi pola ILIKE per kata (semua kata harus cocok).
func searchPatterns(q string) (has bool, pats []string, err error) {
	pats = []string{}
	q = strings.TrimSpace(q)
	if q == "" {
		return false, pats, nil
	}
	fields := strings.Fields(q)
	if len(q) > 64 || len(fields) > 8 {
		return false, pats, FieldErrors{"q": "TOO_LONG"}
	}
	for _, f := range fields {
		pats = append(pats, "%"+escapeLike(f)+"%")
	}
	return true, pats, nil
}

// ── Omset per pelanggan ────────────────────────────────────────────────────────

// CustomerParams filter laporan omset per pelanggan.
type CustomerParams struct {
	AllOutlets bool
	OutletID   string
	From, To   string
	CashierID  string
	Search     string // nama / kode / telepon pelanggan
	Sort       string // "total" (default), "count", "name", "last"
	Limit      int
	ForExport  bool
	// ExportMax = batas pelanggan ekspor (0 = maxCustomerExportRows).
	ExportMax int
}

// CustomerRow omset satu pelanggan.
type CustomerRow struct {
	MemberID     string `json:"member_id"`
	Code         string `json:"code"`
	Name         string `json:"name"`
	Phone        string `json:"phone"`
	InvoiceCount int    `json:"invoice_count"`
	Subtotal     string `json:"subtotal"`
	Discount     string `json:"discount"`
	Total        string `json:"total"`   // omset = Σ total nota
	Average      string `json:"average"` // omset ÷ jumlah faktur
	LastDate     string `json:"last_date"`
}

// CustomerSummary total seluruh pelanggan (bukan hanya yang tampil).
type CustomerSummary struct {
	CustomerCount int    `json:"customer_count"`
	InvoiceCount  int    `json:"invoice_count"`
	Subtotal      string `json:"subtotal"`
	Discount      string `json:"discount"`
	Total         string `json:"total"`
}

// CustomerResult hasil laporan omset per pelanggan.
type CustomerResult struct {
	From        string          `json:"from"`
	To          string          `json:"to"`
	TenantName  string          `json:"tenant_name"`
	OutletName  string          `json:"outlet_name"`
	OutletCode  string          `json:"outlet_code"`
	CashierName string          `json:"cashier_name"`
	SortLabel   string          `json:"sort_label"`
	Data        []CustomerRow   `json:"data"`
	Summary     CustomerSummary `json:"summary"`
	// Truncated = pelanggan yang tampil lebih sedikit dari seluruh pelanggan (LIMIT).
	Truncated bool `json:"truncated,omitempty"`
}

// customerSortSQL = whitelist ORDER BY (nilai tak pernah dari input mentah).
var customerSortSQL = map[string]string{
	"total": "b.tot DESC, b.cnt DESC, b.member_id",
	"count": "b.cnt DESC, b.tot DESC, b.member_id",
	"name":  "lower(b.name), b.member_id",
	"last":  "b.last_at DESC, b.member_id",
}

var customerSortLabel = map[string]string{
	"total": "Omset", "count": "Jumlah Faktur", "name": "Nama Pelanggan", "last": "Pembelian Terakhir",
}

// customerAggSQL ($1 tenant, $2 outlet[], $3 from, $4 to, $5 kasir, $6 cari?,
// $7 pola ILIKE, $8 limit): agregat per member dihitung sekali; master member
// di-join setelahnya. Nota dibaca lewat LATERAL per outlet agar batas waktu
// per outlet dipakai sebagai kondisi indeks.
func customerAggSQL(order string) string {
	return invoiceOutletsCTE + `,
a AS (
  SELECT s.member_id,
         count(*)        AS cnt,
         sum(s.subtotal) AS sub,
         sum(s.discount) AS disc,
         sum(s.total)    AS tot,
         max(s.created_at) AS last_at
  FROM ol
  CROSS JOIN LATERAL (
    SELECT s.member_id, s.subtotal, s.discount, s.total, s.created_at
    FROM sales s
    WHERE s.tenant_id = $1 AND s.outlet_id = ol.id
      AND s.created_at >= ol.lo AND s.created_at < ol.hi
      AND s.status = 'completed' AND s.member_id IS NOT NULL
      AND ($5::uuid = '00000000-0000-0000-0000-000000000000' OR s.cashier_id = $5::uuid)
    OFFSET 0 -- pagar: cegah planner meratakan LATERAL menjadi hash join + filter
  ) s
  GROUP BY s.member_id
), b AS (
  SELECT a.*, m.code, m.name, m.phone
  FROM a
  JOIN members m ON m.tenant_id = $1 AND m.id = a.member_id
  WHERE $6::bool = false OR (m.name || ' ' || m.code || ' ' || m.phone) ILIKE ALL($7::text[])
), top AS (
  SELECT b.*, count(*) OVER ()::int AS n, sum(b.cnt) OVER () AS t_cnt,
         sum(b.sub) OVER () AS t_sub, sum(b.disc) OVER () AS t_disc, sum(b.tot) OVER () AS t_tot
  FROM b
  ORDER BY ` + order + `
  LIMIT $8
)
SELECT b.member_id, b.code, b.name, b.phone, b.cnt::int, b.sub, b.disc, b.tot, b.last_at,
       b.n, b.t_cnt::int, b.t_sub, b.t_disc, b.t_tot
FROM top b
ORDER BY ` + order + `
`
}

// Customers: laporan omset per pelanggan (non member tidak ikut).
func (s *Service) Customers(ctx context.Context, a authz.Actor, p CustomerParams) (CustomerResult, error) {
	var result CustomerResult
	err := db.WithTenant(ctx, s.pool, a.TenantID, func(tx pgx.Tx) error {
		sc, err := s.resolveScope(ctx, tx, a, p.From, p.To, p.OutletID, p.CashierID, p.AllOutlets)
		if err != nil {
			return err
		}
		sortKey := strings.TrimSpace(p.Sort)
		if sortKey == "" {
			sortKey = "total"
		}
		order, ok := customerSortSQL[sortKey]
		if !ok {
			return FieldErrors{"sort": "INVALID"}
		}
		hasSearch, pats, err := searchPatterns(p.Search)
		if err != nil {
			return err
		}
		limit := p.Limit
		if limit <= 0 {
			limit = defaultCustomerLimit
		}
		limit = min(limit, maxCustomerLimit)
		if p.ForExport {
			limit = maxCustomerExportRows
			if p.ExportMax > 0 && p.ExportMax < limit {
				limit = p.ExportMax
			}
		}

		result.From, result.To = sc.From.Format("2006-01-02"), sc.To.Format("2006-01-02")
		result.TenantName, result.OutletName, result.OutletCode = sc.TenantName, sc.OutletName, sc.OutletCode
		result.CashierName = sc.CashierName
		result.SortLabel = customerSortLabel[sortKey]

		rows, err := tx.Query(ctx, customerAggSQL(order),
			a.TenantID, sc.Outlets, sc.From, sc.To, sc.Cashier, hasSearch, pats, limit)
		if err != nil {
			return err
		}
		defer rows.Close()
		list := []CustomerRow{}
		for rows.Next() {
			var r CustomerRow
			var id uuid.UUID
			var cnt, total, tCnt int
			var sub, disc, tot, tSub, tDisc, tTot decimal.Decimal
			var lastAt time.Time
			if err := rows.Scan(&id, &r.Code, &r.Name, &r.Phone, &cnt, &sub, &disc, &tot, &lastAt,
				&total, &tCnt, &tSub, &tDisc, &tTot); err != nil {
				return err
			}
			r.MemberID = id.String()
			r.InvoiceCount = cnt
			r.Subtotal, r.Discount, r.Total = ds(sub), ds(disc), ds(tot)
			if cnt > 0 {
				r.Average = ds(tot.Div(decimal.NewFromInt(int64(cnt))).Round(2))
			} else {
				r.Average = "0.00"
			}
			r.LastDate = lastAt.In(sc.Loc).Format("2006-01-02")
			list = append(list, r)
			result.Summary = CustomerSummary{
				CustomerCount: total, InvoiceCount: tCnt,
				Subtotal: ds(tSub), Discount: ds(tDisc), Total: ds(tTot),
			}
		}
		if err := rows.Err(); err != nil {
			return err
		}
		if len(list) == 0 {
			result.Summary = CustomerSummary{Subtotal: "0.00", Discount: "0.00", Total: "0.00"}
		}
		result.Truncated = result.Summary.CustomerCount > len(list)
		result.Data = list
		return nil
	})
	return result, err
}

// CustomerFileName nama berkas ekspor omset per pelanggan.
func CustomerFileName(from, to, ext string) string {
	return fmt.Sprintf("omset-perpelanggan_%s_%s.%s", from, to, ext)
}

// ── Faktur per pelanggan (opsional + isi barang) ───────────────────────────────

// CustInvParams filter laporan pelanggan per faktur.
//   - MemberID terisi  → faktur SATU pelanggan, paginasi keyset (untuk akordion layar).
//   - MemberID kosong  → (ekspor) semua pelanggan hasil Search/Sort, faktur dikelompokkan.
type CustInvParams struct {
	AllOutlets bool
	OutletID   string
	From, To   string
	CashierID  string
	MemberID   string
	Search     string
	Sort       string
	Cursor     string
	Limit      int
	WithItems  bool // sertakan isi barang tiap faktur
	ForExport  bool
}

// CustInvSale satu faktur milik pelanggan.
type CustInvSale struct {
	ID          string       `json:"id"`
	DocNo       string       `json:"doc_no"`
	Date        string       `json:"date"`
	Time        string       `json:"time"`
	OutletCode  string       `json:"outlet_code"`
	CashierName string       `json:"cashier_name"`
	Subtotal    string       `json:"subtotal"`
	Discount    string       `json:"discount"`
	Total       string       `json:"total"`
	Paid        string       `json:"paid"`
	Receivable  string       `json:"receivable"`
	Lines       []DetailLine `json:"lines,omitempty"`
}

// CustInvCustomer satu pelanggan beserta fakturnya.
type CustInvCustomer struct {
	CustomerRow
	Sales []CustInvSale `json:"sales"`
}

// CustInvResult hasil laporan pelanggan per faktur.
type CustInvResult struct {
	From        string            `json:"from"`
	To          string            `json:"to"`
	TenantName  string            `json:"tenant_name"`
	OutletName  string            `json:"outlet_name"`
	OutletCode  string            `json:"outlet_code"`
	CashierName string            `json:"cashier_name"`
	WithItems   bool              `json:"with_items"`
	Customers   []CustInvCustomer `json:"customers"`
	Summary     CustomerSummary   `json:"summary"`
	NextCursor  string            `json:"next_cursor,omitempty"`
	Truncated   bool              `json:"truncated,omitempty"`
}

// custInvSalesSQL: faktur completed milik member pada rentang, urut terbaru →
// lama, keyset. Dibaca lewat sales_member_idx; batas waktu terluas (min lo, max
// hi) dipakai sebagai kondisi indeks, batas tepat per outlet disaring setelahnya
// (hanya baris milik member, jadi kecil).
// $1 tenant, $2 outlet[], $3 from, $4 to, $5 kasir, $6 member[], $7 cursor?,
// $8 cursor_at, $9 cursor_id, $10 limit.
const custInvSalesSQL = invoiceOutletsCTE + `,
bnd AS (SELECT min(lo) AS lo, max(hi) AS hi FROM ol)
SELECT s.id, s.member_id, s.doc_no, s.created_at,
       to_char(s.created_at AT TIME ZONE ol.timezone, 'YYYY-MM-DD'),
       to_char(s.created_at AT TIME ZONE ol.timezone, 'HH24:MI'),
       ol.code, coalesce(u.name, ''),
       s.subtotal, s.discount, s.total, s.paid - s.change, s.receivable
FROM bnd, sales s
JOIN ol ON ol.id = s.outlet_id
LEFT JOIN users u ON u.tenant_id = $1 AND u.id = s.cashier_id
WHERE s.tenant_id = $1 AND s.member_id = ANY($6::uuid[])
  AND s.created_at >= bnd.lo AND s.created_at < bnd.hi
  AND s.created_at >= ol.lo AND s.created_at < ol.hi
  AND s.status = 'completed'
  AND ($5::uuid = '00000000-0000-0000-0000-000000000000' OR s.cashier_id = $5::uuid)
  AND ($7::bool = false OR (s.created_at, s.id) < ($8::timestamptz, $9::uuid))
ORDER BY s.created_at DESC, s.id DESC
LIMIT $10
`

// CustomerInvoices: pelanggan per faktur (+ isi barang bila WithItems).
func (s *Service) CustomerInvoices(ctx context.Context, a authz.Actor, p CustInvParams) (CustInvResult, error) {
	var result CustInvResult
	err := db.WithTenant(ctx, s.pool, a.TenantID, func(tx pgx.Tx) error {
		sc, err := s.resolveScope(ctx, tx, a, p.From, p.To, p.OutletID, p.CashierID, p.AllOutlets)
		if err != nil {
			return err
		}
		result.From, result.To = sc.From.Format("2006-01-02"), sc.To.Format("2006-01-02")
		result.TenantName, result.OutletName, result.OutletCode = sc.TenantName, sc.OutletName, sc.OutletCode
		result.CashierName = sc.CashierName
		result.WithItems = p.WithItems
		result.Customers = []CustInvCustomer{}

		// ── Daftar pelanggan ───────────────────────────────────────────────
		var custs []CustInvCustomer
		saleLimit := 0
		hasCursor := false
		var cursorAt time.Time
		cursorID := uuid.Nil
		if p.MemberID != "" {
			mid, perr := uuid.Parse(p.MemberID)
			if perr != nil || mid == uuid.Nil {
				return FieldErrors{"member_id": "INVALID"}
			}
			var c CustInvCustomer
			if err := tx.QueryRow(ctx, `SELECT code, name, phone FROM members WHERE tenant_id = $1 AND id = $2`,
				a.TenantID, mid).Scan(&c.Code, &c.Name, &c.Phone); err != nil {
				if err == pgx.ErrNoRows {
					return FieldErrors{"member_id": "NOT_FOUND"}
				}
				return err
			}
			c.MemberID = mid.String()
			c.Sales = []CustInvSale{}
			custs = []CustInvCustomer{c}

			limit := p.Limit
			if limit <= 0 {
				limit = defaultCustInvLimit
			}
			saleLimit = min(limit, maxCustInvLimit) + 1 // +1 = penanda halaman berikutnya
			if p.ForExport {
				saleLimit = maxCustInvExportSales + 1
			} else if p.Cursor != "" {
				var ok bool
				if cursorAt, cursorID, ok = detailDecodeCursor(p.Cursor); !ok {
					return FieldErrors{"cursor": "INVALID"}
				}
				hasCursor = true
			}
		} else {
			if !p.ForExport {
				return FieldErrors{"member_id": "REQUIRED"} // layar selalu per pelanggan (akordion)
			}
			cr, err := s.Customers(ctx, a, CustomerParams{
				AllOutlets: p.AllOutlets, OutletID: p.OutletID, From: p.From, To: p.To,
				CashierID: p.CashierID, Search: p.Search, Sort: p.Sort,
				ForExport: true, ExportMax: maxCustInvExportCustomers,
			})
			if err != nil {
				return err
			}
			result.Summary = cr.Summary
			result.Truncated = cr.Truncated
			for _, r := range cr.Data {
				custs = append(custs, CustInvCustomer{CustomerRow: r, Sales: []CustInvSale{}})
			}
			saleLimit = maxCustInvExportSales + 1
		}
		if len(custs) == 0 {
			return nil
		}

		idx := make(map[string]int, len(custs))
		mids := make([]uuid.UUID, len(custs))
		for i := range custs {
			idx[custs[i].MemberID] = i
			mids[i] = uuid.MustParse(custs[i].MemberID)
		}

		// ── Faktur ─────────────────────────────────────────────────────────
		rows, err := tx.Query(ctx, custInvSalesSQL, a.TenantID, sc.Outlets, sc.From, sc.To, sc.Cashier,
			mids, hasCursor, cursorAt, cursorID, saleLimit)
		if err != nil {
			return err
		}
		type loaded struct {
			CustInvSale
			member    string
			createdAt time.Time
		}
		var ls []loaded
		for rows.Next() {
			var l loaded
			var sid, mid uuid.UUID
			var sub, disc, tot, paid, recv decimal.Decimal
			if err := rows.Scan(&sid, &mid, &l.DocNo, &l.createdAt, &l.Date, &l.Time,
				&l.OutletCode, &l.CashierName, &sub, &disc, &tot, &paid, &recv); err != nil {
				rows.Close()
				return err
			}
			l.ID, l.member = sid.String(), mid.String()
			l.Subtotal, l.Discount, l.Total, l.Paid, l.Receivable = ds(sub), ds(disc), ds(tot), ds(paid), ds(recv)
			ls = append(ls, l)
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return err
		}

		if p.MemberID != "" && !p.ForExport {
			if len(ls) >= saleLimit {
				ls = ls[:saleLimit-1]
				last := ls[len(ls)-1]
				result.NextCursor = detailEncodeCursor(last.createdAt, uuid.MustParse(last.ID))
			}
		} else if len(ls) > maxCustInvExportSales {
			ls = ls[:maxCustInvExportSales]
			result.Truncated = true
		}

		// ── Isi barang per halaman ─────────────────────────────────────────
		saleIdx := make(map[string]int, len(ls))
		if p.WithItems && len(ls) > 0 {
			ids := make([]uuid.UUID, len(ls))
			for i := range ls {
				ids[i] = uuid.MustParse(ls[i].ID)
				saleIdx[ls[i].ID] = i
				ls[i].Lines = []DetailLine{}
			}
			lrows, err := tx.Query(ctx, detailLinesSQL, a.TenantID, ids, uuid.Nil)
			if err != nil {
				return err
			}
			for lrows.Next() {
				var sid uuid.UUID
				var l DetailLine
				var qty, price, disc, lineTotal decimal.Decimal
				if err := lrows.Scan(&sid, &l.Position, &l.SKU, &l.Name, &l.UnitName,
					&l.CategoryName, &qty, &price, &disc, &lineTotal); err != nil {
					lrows.Close()
					return err
				}
				l.Qty, l.UnitPrice, l.Discount, l.LineTotal = qty.StringFixed(3), ds(price), ds(disc), ds(lineTotal)
				if i, ok := saleIdx[sid.String()]; ok {
					ls[i].Lines = append(ls[i].Lines, l)
				}
			}
			lrows.Close()
			if err := lrows.Err(); err != nil {
				return err
			}
		}

		// Ekspor: faktur lama → baru per pelanggan (urut baca layar: terbaru dulu).
		if p.ForExport {
			for i, j := 0, len(ls)-1; i < j; i, j = i+1, j-1 {
				ls[i], ls[j] = ls[j], ls[i]
			}
		}
		for _, l := range ls {
			if i, ok := idx[l.member]; ok {
				custs[i].Sales = append(custs[i].Sales, l.CustInvSale)
			}
		}
		result.Customers = custs
		return nil
	})
	return result, err
}

// CustInvFileName nama berkas ekspor pelanggan per faktur.
func CustInvFileName(from, to string, withItems bool, ext string) string {
	kind := "perfaktur"
	if withItems {
		kind = "perfaktur-detail"
	}
	return fmt.Sprintf("pelanggan-%s_%s_%s.%s", kind, from, to, ext)
}
