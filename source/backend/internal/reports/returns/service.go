// Package returns provides retur reports: retur penjualan dan retur pembelian,
// masing-masing per faktur (satu baris per dokumen retur) dan per faktur detail
// (dengan baris barang).
//
// Siap big data (AGENTS.md backend): paginasi keyset (return_date, created_at, id)
// tanpa OFFSET/count besar, top-N per outlet lewat LATERAL pada
// {sales,purchase}_returns_list_idx, ringkasan hanya di halaman pertama.
// Pencarian / filter pihak (member, pemasok) memakai bentuk datar agar dapat
// dilayani indeks trigram / indeks pihak, bukan memindai indeks waktu.
package returns

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
	maxReportDays = 366
	defaultLimit  = 100
	maxLimit      = 300
	// maxExportRows = batas dokumen retur pada ekspor; melebihi ini hasil dipotong (Truncated).
	maxExportRows = 50000
	// maxPDFRows = batas dokumen PDF (berkas dibuat di memori).
	maxPDFRows = 5000
)

// Kind = jenis retur.
type Kind int

const (
	// KindSale = retur penjualan (izin sales_reports).
	KindSale Kind = iota
	// KindPurchase = retur pembelian (izin purchase_reports).
	KindPurchase
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

// Service laporan retur.
type Service struct {
	pool *pgxpool.Pool
}

func NewService(pool *pgxpool.Pool) *Service { return &Service{pool: pool} }

// Params filter laporan retur per faktur / per faktur detail.
type Params struct {
	AllOutlets bool
	OutletID   string
	From, To   string
	PartyID    string // member (retur penjualan) / pemasok (retur pembelian)
	Status     string // "", "completed", "void"
	Settle     string // "", "offset" (potong piutang/hutang), "refund" (uang dikembalikan)
	Search     string // potongan nomor retur / nomor nota-faktur asal
	Cursor     string
	Limit      int
	ForExport  bool
	ExportMax  int
	// WithLines = sertakan baris barang (per faktur detail).
	WithLines bool
}

// Header = kop hasil (dipakai layar & ekspor).
type Header struct {
	From        string `json:"from"`
	To          string `json:"to"`
	TenantName  string `json:"tenant_name"`
	OutletName  string `json:"outlet_name"`
	OutletCode  string `json:"outlet_code"`
	PartyName   string `json:"party_name"`
	StatusLabel string `json:"status_label"`
	SettleLabel string `json:"settle_label"`
}

// Line = satu baris barang pada dokumen retur.
type Line struct {
	Position  int    `json:"position"`
	SKU       string `json:"sku"`
	Name      string `json:"name"`
	Unit      string `json:"unit"`
	Qty       string `json:"qty"`
	UnitPrice string `json:"unit_price"` // nilai ÷ qty (sebelum diskon)
	Value     string `json:"value"`
	Discount  string `json:"discount"`
	TaxAmount string `json:"tax_amount"`
	Net       string `json:"net"` // nilai − diskon + PPN
}

// Row = satu dokumen retur.
type Row struct {
	ID           string `json:"id"`
	DocNo        string `json:"doc_no"`
	RefDocNo     string `json:"ref_doc_no"` // nota (penjualan) / faktur (pembelian) asal
	Date         string `json:"date"`
	Status       string `json:"status"`
	OutletCode   string `json:"outlet_code"`
	OutletName   string `json:"outlet_name"`
	PartyName    string `json:"party_name"`
	UserName     string `json:"user_name"`
	Note         string `json:"note"`
	VoidReason   string `json:"void_reason"`
	Subtotal     string `json:"subtotal"`
	Discount     string `json:"discount"`
	TaxAmount    string `json:"tax_amount"`
	Surcharge    string `json:"surcharge"`
	Total        string `json:"total"`
	Offset       string `json:"offset"` // memotong piutang / hutang
	Refund       string `json:"refund"` // uang dikembalikan
	RefundMethod string `json:"refund_method"`
	Lines        []Line `json:"lines,omitempty"`
}

// Summary total seluruh hasil filter (uang hanya dokumen completed).
type Summary struct {
	ReturnCount int    `json:"return_count"`
	VoidCount   int    `json:"void_count"`
	Subtotal    string `json:"subtotal"`
	Discount    string `json:"discount"`
	TaxAmount   string `json:"tax_amount"`
	Surcharge   string `json:"surcharge"`
	Total       string `json:"total"`
	Offset      string `json:"offset"`
	Refund      string `json:"refund"`
	// BaseTotal = total penjualan/pembelian completed pada periode, outlet, dan pihak yang sama;
	// RatioPct = Total ÷ BaseTotal × 100 (rasio retur).
	BaseTotal string `json:"base_total"`
	RatioPct  string `json:"ratio_pct"`
}

// Result hasil laporan retur.
type Result struct {
	Header
	Data       []Row   `json:"data"`
	Summary    Summary `json:"summary"`
	HasSummary bool    `json:"has_summary"`
	NextCursor string  `json:"next_cursor,omitempty"`
	Truncated  bool    `json:"truncated,omitempty"`
}

var zeroUUID = uuid.Nil

// sqlSet = fragmen SQL yang berbeda antara retur penjualan dan retur pembelian.
type sqlSet struct {
	ret       string // tabel retur
	retLines  string // tabel baris retur
	ref       string // tabel dokumen asal
	refCol    string // kolom FK dokumen asal
	party     string // tabel pihak
	partyCol  string
	offsetCol string
	discount  string // ekspresi diskon header
	surcharge string
	lineExtra string // diskon & PPN baris
	partyName string // SELECT name … untuk kop
	base      string // SQL total dasar rasio ($1 tenant, $2 outlet, $3 from, $4 to, $5 pihak)
}

var sets = map[Kind]sqlSet{
	KindSale: {
		ret: "sales_returns", retLines: "sales_return_lines", ref: "sales", refCol: "sale_id",
		party: "members", partyCol: "member_id", offsetCol: "receivable_cut",
		discount: "r.discount", surcharge: "r.surcharge", lineExtra: "discount, tax_amount",
		partyName: `SELECT name FROM members WHERE tenant_id = $1 AND id = $2`,
		base: outletsCTE + `
SELECT coalesce(sum(s.total), 0)
FROM ol
JOIN sales s ON s.tenant_id = $1 AND s.outlet_id = ol.id AND s.created_at >= ol.lo AND s.created_at < ol.hi
WHERE s.status = 'completed'
  AND ($5::uuid = '00000000-0000-0000-0000-000000000000' OR s.member_id = $5::uuid)`,
	},
	KindPurchase: {
		ret: "purchase_returns", retLines: "purchase_return_lines", ref: "purchases", refCol: "purchase_id",
		party: "suppliers", partyCol: "supplier_id", offsetCol: "payable_cut",
		discount: "0::numeric", surcharge: "0::numeric", lineExtra: "0::numeric, 0::numeric",
		partyName: `SELECT name FROM suppliers WHERE tenant_id = $1 AND id = $2`,
		base: outletsCTE + `
SELECT coalesce(sum(p.total), 0)
FROM ol
JOIN purchases p ON p.tenant_id = $1 AND p.outlet_id = ol.id AND p.purchase_date >= $3::date AND p.purchase_date <= $4::date
WHERE p.status = 'completed'
  AND ($5::uuid = '00000000-0000-0000-0000-000000000000' OR p.supplier_id = $5::uuid)`,
	},
}

// outletsCTE: outlet yang dibaca + batas waktu absolut rentang tanggal lokal ([from 00:00, to+1 00:00)
// di zona waktu outlet; hanya dipakai untuk total dasar penjualan).
const outletsCTE = `
WITH ol AS (
  SELECT o.id, o.code, o.name,
         ($3::date)::timestamp AT TIME ZONE o.timezone AS lo,
         (($4::date) + 1)::timestamp AT TIME ZONE o.timezone AS hi
  FROM outlets o
  WHERE o.tenant_id = $1 AND o.id = ANY($2::uuid[])
)
`

// $1 tenant, $2 outlets, $3 from, $4 to, $5 pihak, $6 status, $7 settle, $8 search,
// $9 hasCursor, $10 cursor date, $11 cursor created_at, $12 cursor id, $13 limit.
func (k sqlSet) filter() string {
	r := strings.NewReplacer("{PARTY}", k.partyCol, "{OFFSET}", k.offsetCol)
	return r.Replace(`
      AND ($5::uuid = '00000000-0000-0000-0000-000000000000' OR r.{PARTY} = $5::uuid)
      AND ($6::text = '' OR r.status = $6::text)
      AND ($7::text = '' OR ($7::text = 'offset' AND r.{OFFSET} > 0) OR ($7::text = 'refund' AND r.refund > 0))
`)
}

const searchCond = `
      AND ($8::text = '' OR r.doc_no ILIKE '%' || $8::text || '%' OR ref.doc_no ILIKE '%' || $8::text || '%')
`

// noSearchCond dipakai bentuk tanpa pencarian: $8 selalu kosong di sana, tetapi parameter
// harus tetap dirujuk agar tipenya dapat ditentukan PostgreSQL.
const noSearchCond = `
      AND $8::text = ''
`

const cursorCond = `
      AND ($9::bool = false OR (r.return_date, r.created_at, r.id) < ($10::date, $11::timestamptz, $12::uuid))
`

// innerCols = kolom dokumen retur dengan nama seragam untuk kedua jenis.
func (k sqlSet) innerCols() string {
	r := strings.NewReplacer("{REFCOL}", k.refCol, "{PARTY}", k.partyCol, "{OFFSET}", k.offsetCol,
		"{DISC}", k.discount, "{SURCH}", k.surcharge)
	return r.Replace(`r.id, r.outlet_id, r.{REFCOL} AS ref_id, r.{PARTY} AS party_id, r.doc_no, r.return_date, r.status,
           r.note, r.void_reason, r.subtotal, {DISC} AS discount, r.tax_amount, {SURCH} AS surcharge, r.total,
           r.{OFFSET} AS offset_amt, r.refund, r.refund_method_name, r.refund_method, r.created_by, r.created_at`)
}

// rowsSQL: bentuk LATERAL (top-N per outlet) untuk daftar polos; bentuk datar untuk
// pencarian / filter pihak. Relasi di-join setelah dipotong, hanya untuk ≤ N baris.
func (k sqlSet) rowsSQL(flat bool) string {
	var inner string
	if flat {
		inner = `
  SELECT ` + k.innerCols() + `
  FROM ` + k.ret + ` r
  LEFT JOIN ` + k.ref + ` ref ON ref.tenant_id = $1 AND ref.id = r.` + k.refCol + `
  WHERE r.tenant_id = $1 AND r.outlet_id = ANY($2::uuid[])
    AND r.return_date >= $3::date AND r.return_date <= $4::date
` + k.filter() + searchCond + cursorCond + `
  ORDER BY r.return_date DESC, r.created_at DESC, r.id DESC
  LIMIT $13`
	} else {
		inner = `
  SELECT l.* FROM ol
  CROSS JOIN LATERAL (
    SELECT ` + k.innerCols() + `
    FROM ` + k.ret + ` r
    WHERE r.tenant_id = $1 AND r.outlet_id = ol.id
      AND r.return_date >= $3::date AND r.return_date <= $4::date
` + k.filter() + noSearchCond + cursorCond + `
    ORDER BY r.return_date DESC, r.created_at DESC, r.id DESC
    LIMIT $13
  ) l
  ORDER BY l.return_date DESC, l.created_at DESC, l.id DESC
  LIMIT $13`
	}
	return outletsCTE + `
SELECT r.id, r.doc_no, coalesce(ref.doc_no, ''), r.return_date, r.status, r.created_at, ol.code, ol.name,
       coalesce(pt.name, ''), coalesce(u.name, ''), r.note, r.void_reason,
       r.subtotal, r.discount, r.tax_amount, r.surcharge, r.total, r.offset_amt, r.refund,
       coalesce(nullif(r.refund_method_name, ''), coalesce(r.refund_method, ''))
FROM (` + inner + `
) r
JOIN ol ON ol.id = r.outlet_id
LEFT JOIN ` + k.ref + ` ref ON ref.tenant_id = $1 AND ref.id = r.ref_id
LEFT JOIN ` + k.party + ` pt ON pt.tenant_id = $1 AND pt.id = r.party_id
LEFT JOIN users u ON u.tenant_id = $1 AND u.id = r.created_by
ORDER BY r.return_date DESC, r.created_at DESC, r.id DESC
`
}

// summarySQL: uang hanya status completed (void dihitung terpisah).
func (k sqlSet) summarySQL(withSearch bool) string {
	join, cond := "", noSearchCond
	if withSearch {
		join = `LEFT JOIN ` + k.ref + ` ref ON ref.tenant_id = $1 AND ref.id = r.` + k.refCol
		cond = searchCond
	}
	disc, surch := k.discount, k.surcharge
	return outletsCTE + `
SELECT count(*) FILTER (WHERE r.status = 'completed')::int,
       count(*) FILTER (WHERE r.status = 'void')::int,
       coalesce(sum(r.subtotal) FILTER (WHERE r.status = 'completed'), 0),
       coalesce(sum(` + disc + `) FILTER (WHERE r.status = 'completed'), 0),
       coalesce(sum(r.tax_amount) FILTER (WHERE r.status = 'completed'), 0),
       coalesce(sum(` + surch + `) FILTER (WHERE r.status = 'completed'), 0),
       coalesce(sum(r.total) FILTER (WHERE r.status = 'completed'), 0),
       coalesce(sum(r.` + k.offsetCol + `) FILTER (WHERE r.status = 'completed'), 0),
       coalesce(sum(r.refund) FILTER (WHERE r.status = 'completed'), 0)
FROM ol
JOIN ` + k.ret + ` r ON r.tenant_id = $1 AND r.outlet_id = ol.id
                  AND r.return_date >= $3::date AND r.return_date <= $4::date
` + join + `
WHERE true
` + k.filter() + cond
}

func (k sqlSet) linesSQL() string {
	return `SELECT return_id, position, item_sku, item_name, unit_name, qty, value, ` + k.lineExtra + `
FROM ` + k.retLines + `
WHERE tenant_id = $1 AND return_id = ANY($2::uuid[])
ORDER BY return_id, position`
}

// resolved = parameter yang sudah divalidasi.
type resolved struct {
	outlets  []uuid.UUID
	from, to time.Time
	party    uuid.UUID
	status   string
	settle   string
	search   string
	hdr      Header
}

// resolve memvalidasi filter dan mengisi kop. Dipanggil di dalam transaksi tenant.
func resolve(ctx context.Context, tx pgx.Tx, a authz.Actor, k Kind, p Params) (resolved, error) {
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
	if p.From != "" {
		if fd, perr = time.Parse("2006-01-02", p.From); perr != nil {
			fe["from"] = "INVALID"
		}
	}
	if p.To != "" {
		if td, perr = time.Parse("2006-01-02", p.To); perr != nil {
			fe["to"] = "INVALID"
		}
	}
	if len(fe) == 0 && td.Before(fd) {
		fe["to"] = "MUST_BE_AFTER_FROM"
	}
	if td.Sub(fd) > maxReportDays*24*time.Hour {
		fe["to"] = "MAX_366_DAYS"
	}
	r.status = strings.TrimSpace(p.Status)
	if r.status != "" && r.status != "completed" && r.status != "void" {
		fe["status"] = "INVALID"
	}
	r.settle = strings.TrimSpace(p.Settle)
	if r.settle != "" && r.settle != "offset" && r.settle != "refund" {
		fe["settle"] = "INVALID"
	}
	if p.PartyID != "" {
		id, err := uuid.Parse(p.PartyID)
		if err != nil || id == zeroUUID {
			fe["party_id"] = "INVALID"
		} else {
			r.party = id
		}
	}
	r.search = strings.TrimSpace(p.Search)
	if len(r.search) > 64 {
		fe["q"] = "TOO_LONG"
	}
	if len(fe) > 0 {
		return r, fe
	}
	r.search = escapeLike(r.search)
	r.from, r.to = fd, td

	r.outlets = []uuid.UUID{a.OutletID}
	if p.AllOutlets && len(a.Outlets) > 0 {
		r.outlets = r.outlets[:0]
		for id := range a.Outlets {
			r.outlets = append(r.outlets, id)
		}
	}
	if p.OutletID != "" {
		id, err := uuid.Parse(p.OutletID)
		if err != nil || id == zeroUUID {
			return r, FieldErrors{"outlet_id": "INVALID"}
		}
		if id != a.OutletID && !a.Outlets[id] {
			return r, ErrOutletForbidden
		}
		r.outlets = []uuid.UUID{id}
	}

	r.hdr.From, r.hdr.To = fd.Format("2006-01-02"), td.Format("2006-01-02")
	switch r.status {
	case "completed":
		r.hdr.StatusLabel = "Completed"
	case "void":
		r.hdr.StatusLabel = "Void"
	}
	switch r.settle {
	case "offset":
		r.hdr.SettleLabel = map[Kind]string{KindSale: "Potong Piutang", KindPurchase: "Potong Hutang"}[k]
	case "refund":
		r.hdr.SettleLabel = "Uang Dikembalikan"
	}
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
	if r.party != zeroUUID {
		if err := tx.QueryRow(ctx, sets[k].partyName, a.TenantID, r.party).Scan(&r.hdr.PartyName); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return r, FieldErrors{"party_id": "NOT_FOUND"}
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

// cursor keyset (return_date, created_at, id).
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

// ratioPct = rasio retur terhadap nilai dasar (2 desimal; dasar 0 → 0).
func ratioPct(total, base decimal.Decimal) string {
	if !base.IsPositive() {
		return "0.00"
	}
	return total.Mul(decimal.NewFromInt(100)).Div(base).Round(2).StringFixed(2)
}

// Invoices: laporan retur per faktur (WithLines = per faktur detail).
func (s *Service) Invoices(ctx context.Context, a authz.Actor, k Kind, p Params) (Result, error) {
	var result Result
	set, ok := sets[k]
	if !ok {
		return result, errors.New("returns: jenis tidak dikenal")
	}
	err := db.WithTenant(ctx, s.pool, a.TenantID, func(tx pgx.Tx) error {
		rs, err := resolve(ctx, tx, a, k, p)
		if err != nil {
			return err
		}
		result.Header = rs.hdr

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
			var sm Summary
			var sub, disc, tax, sur, tot, off, ref, base decimal.Decimal
			if err := tx.QueryRow(ctx, set.summarySQL(rs.search != ""),
				a.TenantID, rs.outlets, rs.from, rs.to, rs.party, rs.status, rs.settle, rs.search).
				Scan(&sm.ReturnCount, &sm.VoidCount, &sub, &disc, &tax, &sur, &tot, &off, &ref); err != nil {
				return err
			}
			if err := tx.QueryRow(ctx, set.base, a.TenantID, rs.outlets, rs.from, rs.to, rs.party).Scan(&base); err != nil {
				return err
			}
			sm.Subtotal, sm.Discount, sm.TaxAmount, sm.Surcharge = money(sub), money(disc), money(tax), money(sur)
			sm.Total, sm.Offset, sm.Refund = money(tot), money(off), money(ref)
			sm.BaseTotal, sm.RatioPct = money(base), ratioPct(tot, base)
			result.Summary, result.HasSummary = sm, true
		}

		// Pencarian atau filter pihak → bentuk datar (indeks trigram / indeks pihak).
		flat := rs.search != "" || rs.party != zeroUUID
		rows, err := tx.Query(ctx, set.rowsSQL(flat),
			a.TenantID, rs.outlets, rs.from, rs.to, rs.party, rs.status, rs.settle, rs.search,
			hasCursor, curDay, curAt, curID, rowLimit)
		if err != nil {
			return err
		}
		list := []Row{}
		var stamps, days []time.Time
		for rows.Next() {
			var r Row
			var id uuid.UUID
			var day pgtype.Date
			var at time.Time
			var sub, disc, tax, sur, tot, off, ref decimal.Decimal
			if err := rows.Scan(&id, &r.DocNo, &r.RefDocNo, &day, &r.Status, &at, &r.OutletCode, &r.OutletName,
				&r.PartyName, &r.UserName, &r.Note, &r.VoidReason,
				&sub, &disc, &tax, &sur, &tot, &off, &ref, &r.RefundMethod); err != nil {
				rows.Close()
				return err
			}
			r.ID, r.Date = id.String(), day.Time.Format("2006-01-02")
			r.Subtotal, r.Discount, r.TaxAmount, r.Surcharge = money(sub), money(disc), money(tax), money(sur)
			r.Total, r.Offset, r.Refund = money(tot), money(off), money(ref)
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
				lrows, err := tx.Query(ctx, set.linesSQL(), a.TenantID, ids)
				if err != nil {
					return err
				}
				for lrows.Next() {
					var rid uuid.UUID
					var l Line
					var qty, value, disc, tax decimal.Decimal
					if err := lrows.Scan(&rid, &l.Position, &l.SKU, &l.Name, &l.Unit, &qty, &value, &disc, &tax); err != nil {
						lrows.Close()
						return err
					}
					l.Qty, l.Value, l.Discount, l.TaxAmount = qty.String(), money(value), money(disc), money(tax)
					l.Net = money(value.Sub(disc).Add(tax))
					l.UnitPrice = "0.00"
					if qty.IsPositive() {
						l.UnitPrice = money(value.Div(qty).Round(2))
					}
					if i, ok := idx[rid.String()]; ok {
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

// sumMoney menjumlahkan string desimal (awalan "-" = pengurang). Dipakai untuk total ekspor.
func sumMoney(parts ...string) string {
	var sum decimal.Decimal
	for _, p := range parts {
		d, err := decimal.NewFromString(p)
		if err != nil {
			continue
		}
		sum = sum.Add(d)
	}
	return money(sum)
}
