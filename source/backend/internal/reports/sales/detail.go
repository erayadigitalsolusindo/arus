package sales

// Laporan penjualan DETAIL per tanggal (rinci per nota, lalu isi nota).
//
// Laporan per tanggal membaca agregat tabel sales untuk seluruh rentang;
// laporan ini memaginasi rinci per nota. Karena itu (AGENTS.md backend:
// wajib siap big data):
//   - paginasi KEYSET (created_at, id) menurun — bukan OFFSET, tanpa count(*) OVER();
//   - rentang tanggal dibatasi dan baris per halaman dibatasi (default 100, maks 300);
//   - isi nota (baris + pembayaran) diambil per halaman lewat `sale_id = ANY(...)`,
//     bukan JOIN besar yang menggandakan baris sales;
//   - ekspor dibatasi jumlah nota (maxDetailExportSales) agar tidak memakan
//     memori tak terbatas pada rentang besar.

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
	"github.com/shopspring/decimal"

	"aciraba/internal/authz"
	"aciraba/internal/platform/db"
)

const (
	defaultDetailLimit = 100
	maxDetailLimit     = 300

	// maxDetailExportSales = batas nota untuk ekspor PDF/XLSX. Melebihi ini
	// hasil dipotong dan result.Truncated = true (UI memintaDepositkan rentang).
	maxDetailExportSales = 3000

	// maxDetailDays = batas rentang laporan detail (sama dengan laporan ringkasan).
	maxDetailDays = 366
)

// ErrOutletForbidden = outlet yang diminta bukan outlet aktif pemanggil dan
// tidak termasuk haknya (AGENTS.md §3.1: cabang di luar akses tak pernah terbaca).
var ErrOutletForbidden = errors.New("OUTLET_FORBIDDEN")

// DetailParams filter laporan penjualan detail.
type DetailParams struct {
	AllOutlets bool
	OutletID   string
	From, To   string
	CashierID  string
	CategoryID string
	Status     string // "", "completed", "void"
	Cursor     string // keyset: base64(created_at_unix_micro|id)
	Limit      int    // default 100, maks 300
	// ForExport = true → ambil semua nota yang cocok (tanpa cursor) sampai
	// maxDetailExportSales, untuk NeedsXLSX / NeedsPDF.
	ForExport bool
}

// DetailLine satu baris barang pada nota.
type DetailLine struct {
	Position     int    `json:"position"`
	SKU          string `json:"sku"`
	Name         string `json:"name"`
	UnitName     string `json:"unit_name"`
	CategoryName string `json:"category_name"`
	Qty          string `json:"qty"`
	UnitPrice    string `json:"unit_price"`
	Discount     string `json:"discount"`
	LineTotal    string `json:"line_total"`
}

// DetailMethod satu pembayaran pada nota.
type DetailMethod struct {
	Name   string `json:"name"`
	Kind   string `json:"kind"`
	Amount string `json:"amount"`
}

// DetailSale satu nota beserta isinya.
type DetailSale struct {
	ID          string `json:"id"`
	DocNo       string `json:"doc_no"`
	Time        string `json:"time"` // HH:MM (zona waktu outlet)
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
	// LinesTotal = Σ baris yang ditampilkan. Sama dengan Subtotal bila tanpa
	// filter kategori; lebih kecil bila hanya sebagian baris kategori yang tampil.
	LinesTotal string         `json:"lines_total"`
	Lines      []DetailLine   `json:"lines"`
	Methods    []DetailMethod `json:"methods,omitempty"`
}

// DetailDay satu tanggal beserta nota-notanya.
type DetailDay struct {
	Date       string       `json:"date"`
	SaleCount  int          `json:"sale_count"`
	VoidCount  int          `json:"void_count"`
	Subtotal   string       `json:"subtotal"`
	Discount   string       `json:"discount"`
	TaxStore   string       `json:"tax_store"`
	TaxGov     string       `json:"tax_gov"`
	Total      string       `json:"total"`
	Paid       string       `json:"paid"`
	Receivable string       `json:"receivable"`
	Sales      []DetailSale `json:"sales"`
}

// DetailSummary = Σ SELURUH nota yang cocok filter (bukan hanya halaman dimuat).
type DetailSummary struct {
	SaleCount  int    `json:"sale_count"`
	VoidCount  int    `json:"void_count"`
	Subtotal   string `json:"subtotal"`
	Discount   string `json:"discount"`
	TaxStore   string `json:"tax_store"`
	TaxGov     string `json:"tax_gov"`
	Total      string `json:"total"`
	Paid       string `json:"paid"`
	Receivable string `json:"receivable"`
}

// DetailResult hasil laporan penjualan detail.
type DetailResult struct {
	From         string        `json:"from"`
	To           string        `json:"to"`
	TenantName   string        `json:"tenant_name"`
	OutletName   string        `json:"outlet_name"`
	OutletCode   string        `json:"outlet_code"`
	CashierName  string        `json:"cashier_name"`
	CategoryName string        `json:"category_name"`
	StatusLabel  string        `json:"status_label"`
	Data         []DetailDay   `json:"data"`
	Summary      DetailSummary `json:"summary"`
	// NextCursor = keyset halaman berikutnya (layar); kosong = habis.
	NextCursor string `json:"next_cursor,omitempty"`
	// Truncated = true bila hasil melebihi batas ekspor (hanya pada format file).
	Truncated bool `json:"truncated,omitempty"`
}

func detailEncodeCursor(t time.Time, id uuid.UUID) string {
	return base64.RawURLEncoding.EncodeToString([]byte(strconv.FormatInt(t.UnixMicro(), 10) + "|" + id.String()))
}

func detailDecodeCursor(c string) (time.Time, uuid.UUID, bool) {
	raw, err := base64.RawURLEncoding.DecodeString(c)
	if err != nil {
		return time.Time{}, uuid.Nil, false
	}
	us, ids, ok := strings.Cut(string(raw), "|")
	if !ok {
		return time.Time{}, uuid.Nil, false
	}
	n, err1 := strconv.ParseInt(us, 10, 64)
	id, err2 := uuid.Parse(ids)
	if err1 != nil || err2 != nil {
		return time.Time{}, uuid.Nil, false
	}
	return time.UnixMicro(n).UTC(), id, true
}

// detailOutletsCTE = outlet yang dibaca + batas waktu absolut rentang tanggal
// lokal masing-masing ([from 00:00, to+1 00:00) di zona waktu outlet). Dihitung
// per outlet (bukan fungsi pada kolom created_at) agar sales_outlet_time_idx
// (tenant_id, outlet_id, created_at DESC) terpakai.
const detailOutletsCTE = `
WITH ol AS (
  SELECT o.id, o.code, o.name, o.timezone,
         ($3::date)::timestamp AT TIME ZONE o.timezone AS lo,
         (($4::date) + 1)::timestamp AT TIME ZONE o.timezone AS hi
  FROM outlets o
  WHERE o.tenant_id = $1 AND o.id = ANY($2::uuid[])
)
`

// detailFilterSQL = filter nota (kasir, status, kategori). Filter kategori memakai
// EXISTS (semi-join, bukan JOIN yang menggandakan baris): nota yang tampil hanya
// nota yang punya minimal satu baris barang di kategori itu.
const detailFilterSQL = `
      AND s.status <> 'superseded'
      AND ($5::uuid = '00000000-0000-0000-0000-000000000000' OR s.cashier_id = $5::uuid)
      AND ($6::text = '' OR s.status = $6::text)
      AND ($7::uuid = '00000000-0000-0000-0000-000000000000' OR EXISTS (
            SELECT 1 FROM sale_lines fl
            JOIN items fi ON fi.tenant_id = fl.tenant_id AND fi.id = fl.item_id
            WHERE fl.tenant_id = s.tenant_id AND fl.sale_id = s.id AND fi.category_id = $7::uuid))
`

// detailSalesSQL memilih nota pada rentang tanggal dengan paginasi keyset.
// Top-N per outlet lewat LATERAL (membaca indeks terurut dan berhenti di N baris),
// lalu digabung & dipotong lagi ke N: biaya O(outlet × N), tidak bergantung pada
// jumlah nota di rentang. Relasi (kasir, member, sales) di-join SETELAH dipotong.
// Tanggal lokal dihitung di SQL (AT TIME ZONE) → pengelompokan sama dengan
// laporan per tanggal.
const detailSalesSQL = detailOutletsCTE + `
SELECT s.id, s.doc_no, s.status, s.created_at,
       to_char(s.created_at AT TIME ZONE ol.timezone, 'YYYY-MM-DD') AS day,
       to_char(s.created_at AT TIME ZONE ol.timezone, 'HH24:MI')    AS clock,
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
` + detailFilterSQL + `
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

// detailSummarySQL = Σ seluruh nota yang cocok filter (di luar paginasi).
// Uang hanya menjumlahkan status uang (completed secara bawaan; bila filter
// status=void dipilih, void yang dihitung); paid = uang masuk netto (paid − change),
// sama dengan laporan per tanggal. void_count selalu jumlah nota batal.
const detailSummarySQL = detailOutletsCTE + `
SELECT count(*) FILTER (WHERE s.status = 'completed')::int,
       count(*) FILTER (WHERE s.status = 'void')::int,
       coalesce(sum(s.subtotal) FILTER (WHERE s.status = coalesce(nullif($6::text, ''), 'completed')::text), 0),
       coalesce(sum(s.discount) FILTER (WHERE s.status = coalesce(nullif($6::text, ''), 'completed')::text), 0),
       coalesce(sum(s.tax_store) FILTER (WHERE s.status = coalesce(nullif($6::text, ''), 'completed')::text), 0),
       coalesce(sum(s.tax_gov) FILTER (WHERE s.status = coalesce(nullif($6::text, ''), 'completed')::text), 0),
       coalesce(sum(s.total) FILTER (WHERE s.status = coalesce(nullif($6::text, ''), 'completed')::text), 0),
       coalesce(sum(s.paid - s.change) FILTER (WHERE s.status = coalesce(nullif($6::text, ''), 'completed')::text), 0),
       coalesce(sum(s.receivable) FILTER (WHERE s.status = coalesce(nullif($6::text, ''), 'completed')::text), 0)
FROM ol
JOIN sales s ON s.tenant_id = $1 AND s.outlet_id = ol.id
            AND s.created_at >= ol.lo AND s.created_at < ol.hi
` + detailFilterSQL + `
`

// detailLinesSQL = isi nota pada satu halaman. Baris diambil sekali per nota
// lewat sale_id = ANY (indeks UNIQUE (sale_id, position)); difilter kategori
// bila kategori dipilih, agar LinesTotal jujur hanya menghitung baris yang tampil.
const detailLinesSQL = `
SELECT l.sale_id, l.position, l.sku, l.name, l.unit_name,
       coalesce(c.name, ''), l.qty, l.unit_price, l.discount, l.line_total
FROM sale_lines l
LEFT JOIN items i ON i.tenant_id = l.tenant_id AND i.id = l.item_id
LEFT JOIN categories c ON c.tenant_id = i.tenant_id AND c.id = i.category_id
WHERE l.tenant_id = $1 AND l.sale_id = ANY($2::uuid[])
  AND ($3::uuid = '00000000-0000-0000-0000-000000000000' OR i.category_id = $3::uuid)
ORDER BY l.sale_id, l.position
`

// detailMethodsSQL = metode pembayaran per nota pada halaman ini. Nama & jenis
// dibaca dari payment_methods saat laporan (ganti nama ikut nama baru), sama
// seperti laporan per tanggal.
const detailMethodsSQL = `
SELECT p.sale_id, pm.name, pm.kind, p.amount
FROM sale_payments p
JOIN payment_methods pm ON pm.tenant_id = p.tenant_id AND pm.id = p.method_id
WHERE p.tenant_id = $1 AND p.sale_id = ANY($2::uuid[])
ORDER BY p.sale_id, p.position
`

// loadedSale = nota saat dimuat (menyimpan tanggal lokal + created_at untuk
// pengelompokan per hari dan cursor keyset).
type loadedSale struct {
	DetailSale
	Day       string
	createdAt time.Time
	Sub       decimal.Decimal
	Disc      decimal.Decimal
	TaxS      decimal.Decimal
	TaxG      decimal.Decimal
	Tot       decimal.Decimal
	PaidV     decimal.Decimal
	Chgv      decimal.Decimal
	Recv      decimal.Decimal
}

// dayAcc = akumulator total satu tanggal. Hanya mencatat nota ber-status uang
// (completed, atau void bila filter status=void); paid = netto (paid − change)
// agar identik dengan laporan penjualan per tanggal.
type dayAcc struct {
	sub, disc, taxS, taxG, tot, paid, recv decimal.Decimal
}

// Detail: laporan penjualan detail per tanggal (rinci per nota).
func (s *Service) Detail(ctx context.Context, a authz.Actor, p DetailParams) (DetailResult, error) {
	var result DetailResult

	err := db.WithTenant(ctx, s.pool, a.TenantID, func(tx pgx.Tx) error {
		// ── Tanggal default = hari lokal outlet aktif ───────────────────────
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

		// ── Filter lain ────────────────────────────────────────────────────
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
		category := uuid.Nil
		if p.CategoryID != "" {
			id, err := uuid.Parse(p.CategoryID)
			if err != nil || id == uuid.Nil {
				return FieldErrors{"category_id": "INVALID"}
			}
			category = id
		}
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
			limit = defaultDetailLimit
		}
		if limit > maxDetailLimit {
			limit = maxDetailLimit
		}
		rowLimit := limit + 1
		if p.ForExport {
			rowLimit = maxDetailExportSales + 1
		}

		// Outlet dari token; all_outlets hanya untuk outlet yang boleh diakses.
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
		if cashier != uuid.Nil {
			if err := tx.QueryRow(ctx, `SELECT name FROM users WHERE tenant_id = $1 AND id = $2`,
				a.TenantID, cashier).Scan(&result.CashierName); err != nil {
				return err
			}
		}
		if category != uuid.Nil {
			if err := tx.QueryRow(ctx, `SELECT name FROM categories WHERE tenant_id = $1 AND id = $2`,
				a.TenantID, category).Scan(&result.CategoryName); err != nil {
				return err
			}
		}
		switch status {
		case "completed":
			result.StatusLabel = "Completed"
		case "void":
			result.StatusLabel = "Void"
		}
		if err := tx.QueryRow(ctx, `SELECT name FROM tenants WHERE id = $1`, a.TenantID).Scan(&result.TenantName); err != nil {
			return err
		}
		if len(outlets) == 1 {
			if err := tx.QueryRow(ctx, `SELECT code, name FROM outlets WHERE tenant_id = $1 AND id = $2`,
				a.TenantID, outlets[0]).Scan(&result.OutletCode, &result.OutletName); err != nil {
				return err
			}
		} else {
			result.OutletName = "Semua Outlet"
		}

		// ── Ringkasan seluruh hasil filter (di luar paginasi) ───────────────
		var sc, vc int
		var sumSub, sumDisc, sumTaxStore, sumTaxGov, sumTotal, sumPaid, sumRecv decimal.Decimal
		if err := tx.QueryRow(ctx, detailSummarySQL,
			a.TenantID, outlets, fd, td, cashier, status, category).
			Scan(&sc, &vc, &sumSub, &sumDisc, &sumTaxStore, &sumTaxGov, &sumTotal, &sumPaid, &sumRecv); err != nil {
			return err
		}
		// sc = jumlah nota completed (SQL); vc = jumlah void. Bila filter
		// status=void dipilih, sc = 0 dan ringkasan uang menjumlahkan void.
		result.Summary = DetailSummary{
			SaleCount: sc, VoidCount: vc,
			Subtotal: sumSub.StringFixed(2), Discount: sumDisc.StringFixed(2),
			TaxStore: sumTaxStore.StringFixed(2), TaxGov: sumTaxGov.StringFixed(2),
			Total: sumTotal.StringFixed(2), Paid: sumPaid.StringFixed(2),
			Receivable: sumRecv.StringFixed(2),
		}

		// ── Halaman nota ────────────────────────────────────────────────────
		rows, err := tx.Query(ctx, detailSalesSQL,
			a.TenantID, outlets, fd, td, cashier, status, category,
			hasCursor, cursorAt, cursorID, rowLimit)
		if err != nil {
			return err
		}
		var loaded []loadedSale
		for rows.Next() {
			var ls loadedSale
			var saleID uuid.UUID
			var day, clock string
			var sub, disc, taxS, taxG, oc, tot, paid, chg, recv decimal.Decimal
			if err := rows.Scan(&saleID, &ls.DocNo, &ls.Status, &ls.createdAt, &day, &clock,
				&ls.OutletCode, &ls.OutletName, &ls.CashierName, &ls.MemberName, &ls.Salesperson,
				&sub, &disc, &taxS, &taxG, &oc,
				&tot, &paid, &chg, &recv); err != nil {
				rows.Close()
				return err
			}
			ls.ID = saleID.String()
			ls.Day, ls.Time = day, clock
			ls.Sub, ls.Disc, ls.TaxS, ls.TaxG, ls.Tot, ls.PaidV, ls.Chgv, ls.Recv = sub, disc, taxS, taxG, tot, paid, chg, recv
			ls.Subtotal, ls.Discount = sub.StringFixed(2), disc.StringFixed(2)
			ls.TaxStore, ls.TaxGov = taxS.StringFixed(2), taxG.StringFixed(2)
			ls.OtherCost = oc.StringFixed(2)
			ls.Total = tot.StringFixed(2)
			ls.Paid, ls.Change, ls.Receivable = paid.StringFixed(2), chg.StringFixed(2), recv.StringFixed(2)
			ls.Lines = []DetailLine{}
			loaded = append(loaded, ls)
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return err
		}

		if p.ForExport {
			if len(loaded) > maxDetailExportSales {
				loaded = loaded[:maxDetailExportSales]
				result.Truncated = true
			}
		} else if len(loaded) >= limit {
			// Baris ekstra (limit+1) hanya penanda "ada halaman berikutnya"; cursor menunjuk baris
			// TERAKHIR halaman ini supaya halaman berikutnya tidak melompati/mengulang.
			if len(loaded) > limit {
				loaded = loaded[:limit]
			}
			last := loaded[len(loaded)-1]
			result.NextCursor = detailEncodeCursor(last.createdAt, uuid.MustParse(last.ID))
		}

		// ── Isi nota: baris + metode, satu query masing-masing per halaman ──
		if len(loaded) > 0 {
			idx := make(map[string]int, len(loaded))
			ids := make([]uuid.UUID, len(loaded))
			for i := range loaded {
				id, perr := uuid.Parse(loaded[i].ID)
				if perr != nil {
					return perr
				}
				idx[loaded[i].ID] = i
				ids[i] = id
			}

			lrows, err := tx.Query(ctx, detailLinesSQL, a.TenantID, ids, category)
			if err != nil {
				return err
			}
			for lrows.Next() {
				var saleID uuid.UUID
				var l DetailLine
				var qty, price, disc, lineTotal decimal.Decimal
				if err := lrows.Scan(&saleID, &l.Position, &l.SKU, &l.Name, &l.UnitName,
					&l.CategoryName, &qty, &price, &disc, &lineTotal); err != nil {
					lrows.Close()
					return err
				}
				l.Qty = qty.StringFixed(3)
				l.UnitPrice = price.StringFixed(2)
				l.Discount = disc.StringFixed(2)
				l.LineTotal = lineTotal.StringFixed(2)
				if i, ok := idx[saleID.String()]; ok {
					loaded[i].Lines = append(loaded[i].Lines, l)
				}
			}
			lrows.Close()
			if err := lrows.Err(); err != nil {
				return err
			}
			// LinesTotal = Σ baris yang tampil (subset dari Subtotal bila difilter kategori).
			for i := range loaded {
				var sum decimal.Decimal
				for _, l := range loaded[i].Lines {
					d, _ := decimal.NewFromString(l.LineTotal)
					sum = sum.Add(d)
				}
				loaded[i].LinesTotal = sum.StringFixed(2)
			}

			mrows, err := tx.Query(ctx, detailMethodsSQL, a.TenantID, ids)
			if err != nil {
				return err
			}
			for mrows.Next() {
				var saleID uuid.UUID
				var m DetailMethod
				var amt decimal.Decimal
				if err := mrows.Scan(&saleID, &m.Name, &m.Kind, &amt); err != nil {
					mrows.Close()
					return err
				}
				m.Amount = amt.StringFixed(2)
				if i, ok := idx[saleID.String()]; ok {
					loaded[i].Methods = append(loaded[i].Methods, m)
				}
			}
			mrows.Close()
			if err := mrows.Err(); err != nil {
				return err
			}
		}

		// ── Group per tanggal (naik: tanggal terlama → terbaru) ──────────────
		// Status uang: completed (bawaan); bila pengguna memilih filter
		// status=void, void yang diringkas agar ringkasan tetap menjelaskan
		// baris yang sedang dilihat. paid = netto (paid − change) — sama dengan
		// laporan per tanggal.
		moneyStatus := "completed"
		if status == "void" {
			moneyStatus = "void"
		}
		dayIdx := map[string]int{}
		days := []DetailDay{}
		acc := map[string]*dayAcc{}
		for i := range loaded {
			ls := &loaded[i]
			di, ok := dayIdx[ls.Day]
			if !ok {
				di = len(days)
				dayIdx[ls.Day] = di
				days = append(days, DetailDay{Date: ls.Day, Sales: []DetailSale{}})
				acc[ls.Day] = &dayAcc{}
			}
			days[di].Sales = append(days[di].Sales, ls.DetailSale)
			if ls.Status == "void" {
				days[di].VoidCount++
			}
			// Hanya status uang yang menambah total per tanggal.
			if ls.Status != moneyStatus {
				continue
			}
			days[di].SaleCount++
			a := acc[ls.Day]
			a.sub = a.sub.Add(ls.Sub)
			a.disc = a.disc.Add(ls.Disc)
			a.taxS = a.taxS.Add(ls.TaxS)
			a.taxG = a.taxG.Add(ls.TaxG)
			a.tot = a.tot.Add(ls.Tot)
			a.paid = a.paid.Add(ls.PaidV).Sub(ls.Chgv)
			a.recv = a.recv.Add(ls.Recv)
		}
		for i := range days {
			a := acc[days[i].Date]
			days[i].Subtotal = a.sub.StringFixed(2)
			days[i].Discount = a.disc.StringFixed(2)
			days[i].TaxStore = a.taxS.StringFixed(2)
			days[i].TaxGov = a.taxG.StringFixed(2)
			days[i].Total = a.tot.StringFixed(2)
			days[i].Paid = a.paid.StringFixed(2)
			days[i].Receivable = a.recv.StringFixed(2)
		}
		// sort tanggal naik
		for i := 0; i < len(days)-1; i++ {
			for j := i + 1; j < len(days); j++ {
				if days[j].Date < days[i].Date {
					days[i], days[j] = days[j], days[i]
				}
			}
		}
		result.Data = days
		return nil
	})
	return result, err
}
