// Package debt provides laporan hutang (pemasok) dan piutang (member): beredar per dokumen
// dengan umur (aging), pembayaran, saldo per pihak, buku bantu, dan saldo kredit di pemasok.
//
// Siap big data (AGENTS.md backend): semua daftar memakai paginasi keyset tanpa OFFSET/count
// besar. Daftar beredar diurut (created_at, id) lewat indeks dokumen; saldo per pihak dan
// saldo di pemasok mengiterasi pihak berurut (lower(name), id) dengan LATERAL yang hanya
// menjumlah dokumen pihak itu; pembayaran memakai LATERAL per outlet pada indeks outlet+waktu;
// buku bantu per satu pihak dengan keyset (tanggal, waktu, urutan, id) dan saldo berjalan
// dibawa di kursor. Ringkasan hanya dihitung pada halaman pertama.
//
// Saldo dokumen dihitung per tanggal (as-of): amount − Σ pembayaran sampai tanggal itu
// − Σ potongan retur completed sampai tanggal itu (bukan kolom yang dimutasi).
package debt

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
)

const (
	maxReportDays = 366
	defaultLimit  = 100
	maxLimit      = 300
	// maxExportRows = batas baris pada ekspor Excel; melebihi ini hasil dipotong (Truncated).
	maxExportRows = 50000
	// maxPDFRows = batas baris PDF (berkas dibuat di memori).
	maxPDFRows = 5000
)

// Kind = jenis laporan.
type Kind int

const (
	// KindPayable = hutang ke pemasok (izin supplier_payables).
	KindPayable Kind = iota
	// KindReceivable = piutang member (izin member_receivables).
	KindReceivable
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

// Service laporan hutang / piutang.
type Service struct {
	pool *pgxpool.Pool
}

func NewService(pool *pgxpool.Pool) *Service { return &Service{pool: pool} }

// Params filter semua laporan (tiap laporan memakai sebagian).
type Params struct {
	AllOutlets bool
	OutletID   string
	AsOf       string // beredar & saldo per pihak
	From, To   string // pembayaran, buku bantu, saldo di pemasok
	PartyID    string // pemasok (hutang) / member (piutang)
	MethodID   string // metode bayar (pembayaran)
	Search     string
	Cursor     string
	Limit      int
	ForExport  bool
	ExportMax  int
}

// Header = kop hasil (dipakai layar & ekspor).
type Header struct {
	AsOf       string `json:"as_of,omitempty"`
	From       string `json:"from,omitempty"`
	To         string `json:"to,omitempty"`
	TenantName string `json:"tenant_name"`
	OutletName string `json:"outlet_name"`
	OutletCode string `json:"outlet_code"`
	PartyCode  string `json:"party_code,omitempty"`
	PartyName  string `json:"party_name,omitempty"`
}

var zeroUUID = uuid.Nil

// sqlSet = fragmen SQL yang berbeda antara hutang dan piutang. Alias baku: d = dokumen
// (payables / receivables), o = outlet dokumen.
type sqlSet struct {
	docTable   string // tabel dokumen
	partyCol   string // kolom pihak pada dokumen
	partyTable string
	extraJoin  string // join untuk tanggal & nomor dokumen
	validity   string // syarat dokumen masih berlaku (selain voided_at IS NULL)
	docDate    string // ekspresi tanggal dokumen (tanggal lokal outlet)
	docNo      string
	refNo      string
	payTable   string
	payFK      string
	payUser    string // kolom pembuat pembayaran
	payFee     string // ekspresi biaya pembayaran
	settleTbl  string
	cut        string // subquery potongan retur s.d. $3, terkait alias d
	retTable   string
	retFK      string // kolom FK retur ke dokumen asal
	retDocFK   string // kolom pada dokumen yang dirujuk retur
	retCutCol  string
}

var sets = map[Kind]sqlSet{
	KindPayable: {
		docTable: "payables", partyCol: "supplier_id", partyTable: "suppliers",
		extraJoin: `JOIN purchases pu ON pu.tenant_id = d.tenant_id AND pu.id = d.purchase_id`,
		validity:  ``,
		docDate:   `pu.purchase_date`, docNo: `pu.doc_no`, refNo: `pu.supplier_invoice_no`,
		payTable: "payable_payments", payFK: "payable_id", payUser: "paid_by", payFee: "0::numeric", settleTbl: "payable_settlements",
		cut: `SELECT sum(r.payable_cut) AS cut FROM purchase_returns r
		      WHERE r.tenant_id = d.tenant_id AND r.purchase_id = d.purchase_id AND r.status = 'completed' AND r.return_date <= $3::date`,
		retTable: "purchase_returns", retFK: "purchase_id", retDocFK: "purchase_id", retCutCol: "payable_cut",
	},
	KindReceivable: {
		docTable: "receivables", partyCol: "member_id", partyTable: "members",
		extraJoin: `LEFT JOIN sales s ON s.tenant_id = d.tenant_id AND s.id = d.sale_id`,
		validity:  `AND (d.sale_id IS NULL OR s.status = 'completed')`,
		docDate:   `coalesce((s.created_at AT TIME ZONE o.timezone)::date, d.doc_date)`, docNo: `coalesce(s.doc_no, d.doc_no, '')`, refNo: `d.ref_no`,
		payTable: "receivable_payments", payFK: "receivable_id", payUser: "received_by", payFee: "x.fee_amount", settleTbl: "receivable_settlements",
		cut: `SELECT sum(r.receivable_cut) AS cut FROM sales_returns r
		      WHERE r.tenant_id = d.tenant_id AND r.sale_id = d.sale_id AND r.status = 'completed' AND r.return_date <= $3::date`,
		retTable: "sales_returns", retFK: "sale_id", retDocFK: "sale_id", retCutCol: "receivable_cut",
	},
}

// balExpr = sisa dokumen per tanggal as-of (alias p, c dari docRows).
const balExpr = `(d.amount - coalesce(p.paid, 0) - coalesce(c.cut, 0))`

// docRows = SELECT dokumen + saldo per $3 (as-of). Parameter wajib: $1 tenant, $2 outlet[], $3 as-of.
// `where` (diawali AND) dan `tail` (ORDER BY/LIMIT) ditambahkan apa adanya.
func (k sqlSet) docRows(where, tail string) string {
	return `SELECT d.id, d.outlet_id, d.` + k.partyCol + ` AS party_id, ` + k.docDate + ` AS doc_date, ` + k.docNo + ` AS doc_no,
       ` + k.refNo + ` AS ref_no, d.due_date, d.created_at, d.amount,
       coalesce(p.paid, 0) AS paid, coalesce(c.cut, 0) AS cut, ` + balExpr + ` AS bal
FROM ` + k.docTable + ` d
` + k.extraJoin + `
JOIN outlets o ON o.tenant_id = d.tenant_id AND o.id = d.outlet_id
LEFT JOIN LATERAL (SELECT sum(x.amount) AS paid FROM ` + k.payTable + ` x
                   WHERE x.tenant_id = d.tenant_id AND x.` + k.payFK + ` = d.id
                     AND (x.created_at AT TIME ZONE o.timezone)::date <= $3::date) p ON true
LEFT JOIN LATERAL (` + k.cut + `) c ON true
WHERE d.tenant_id = $1 AND d.outlet_id = ANY($2::uuid[]) AND d.voided_at IS NULL ` + k.validity + `
  AND ` + k.docDate + ` <= $3::date
` + where + `
` + tail
}

const bucketExpr = `CASE WHEN x.due_date IS NULL OR x.due_date >= $3::date THEN 0
     WHEN $3::date - x.due_date <= 30 THEN 1 WHEN $3::date - x.due_date <= 60 THEN 2
     WHEN $3::date - x.due_date <= 90 THEN 3 ELSE 4 END`

// aggCols = kolom agregat standar atas hasil docRows beralias x (kolom bk = bucket umur).
const aggCols = `count(*)::int,
       coalesce(sum(x.amount), 0), coalesce(sum(x.paid), 0), coalesce(sum(x.cut), 0), coalesce(sum(x.bal), 0),
       coalesce(sum(x.bal) FILTER (WHERE x.bk = 0), 0), coalesce(sum(x.bal) FILTER (WHERE x.bk = 1), 0),
       coalesce(sum(x.bal) FILTER (WHERE x.bk = 2), 0), coalesce(sum(x.bal) FILTER (WHERE x.bk = 3), 0),
       coalesce(sum(x.bal) FILTER (WHERE x.bk = 4), 0)`

// Buckets = sisa menurut umur terlambat (hari setelah jatuh tempo, relatif tanggal as-of).
type Buckets struct {
	Current string `json:"current"` // belum jatuh tempo / tanpa jatuh tempo
	D1To30  string `json:"d1_30"`
	D31To60 string `json:"d31_60"`
	D61To90 string `json:"d61_90"`
	Over90  string `json:"over_90"`
}

func newBuckets(b [5]decimal.Decimal) Buckets {
	return Buckets{money(b[0]), money(b[1]), money(b[2]), money(b[3]), money(b[4])}
}

// overdue = jumlah semua bucket terlambat.
func (b Buckets) overdueSum() decimal.Decimal {
	var s decimal.Decimal
	for _, v := range []string{b.D1To30, b.D31To60, b.D61To90, b.Over90} {
		d, _ := decimal.NewFromString(v)
		s = s.Add(d)
	}
	return s
}

// resolved = parameter yang sudah divalidasi.
type resolved struct {
	outlets  []uuid.UUID
	asOf     time.Time
	from, to time.Time
	party    uuid.UUID
	method   uuid.UUID
	search   string
	tz       string // zona waktu outlet aktif (batas waktu saldo di pemasok)
	hdr      Header
}

// need = bagian parameter yang divalidasi.
type need struct {
	asOf, period, party bool // party = wajib (buku bantu)
}

// resolve memvalidasi filter dan mengisi kop. Dipanggil di dalam transaksi tenant.
func resolve(ctx context.Context, tx pgx.Tx, a authz.Actor, k Kind, p Params, n need) (resolved, error) {
	var r resolved
	var localDay pgtype.Date
	if err := tx.QueryRow(ctx, `
		SELECT (now() AT TIME ZONE timezone)::date, timezone FROM outlets WHERE tenant_id = $1 AND id = $2`,
		a.TenantID, a.OutletID).Scan(&localDay, &r.tz); err != nil {
		return r, err
	}
	today := time.Now().UTC()
	if localDay.Valid {
		today = localDay.Time
	}
	fe := FieldErrors{}
	date := func(raw, field string, def time.Time) time.Time {
		if raw == "" {
			return def
		}
		d, err := time.Parse("2006-01-02", raw)
		if err != nil {
			fe[field] = "INVALID"
			return def
		}
		return d
	}
	if n.asOf {
		r.asOf = date(p.AsOf, "as_of", today)
	}
	if n.period {
		r.from, r.to = date(p.From, "from", today), date(p.To, "to", today)
		if len(fe) == 0 && r.to.Before(r.from) {
			fe["to"] = "MUST_BE_AFTER_FROM"
		}
		if r.to.Sub(r.from) > maxReportDays*24*time.Hour {
			fe["to"] = "MAX_366_DAYS"
		}
	}
	if p.PartyID != "" {
		id, err := uuid.Parse(p.PartyID)
		if err != nil || id == zeroUUID {
			fe["party_id"] = "INVALID"
		} else {
			r.party = id
		}
	} else if n.party {
		fe["party_id"] = "REQUIRED"
	}
	if p.MethodID != "" {
		id, err := uuid.Parse(p.MethodID)
		if err != nil || id == zeroUUID {
			fe["method_id"] = "INVALID"
		} else {
			r.method = id
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
	if n.asOf {
		r.hdr.AsOf = r.asOf.Format("2006-01-02")
	}
	if n.period {
		r.hdr.From, r.hdr.To = r.from.Format("2006-01-02"), r.to.Format("2006-01-02")
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
		if err := tx.QueryRow(ctx, `SELECT coalesce(code, ''), name FROM `+sets[k].partyTable+` WHERE tenant_id = $1 AND id = $2`,
			a.TenantID, r.party).Scan(&r.hdr.PartyCode, &r.hdr.PartyName); err != nil {
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

func dateStr(d pgtype.Date) string {
	if !d.Valid {
		return ""
	}
	return d.Time.Format("2006-01-02")
}

func exportMax(p Params) int {
	m := maxExportRows
	if p.ExportMax > 0 && p.ExportMax < m {
		m = p.ExportMax
	}
	return m
}

// --- kursor keyset ---

// encodeCursor menggabungkan bagian dengan "|" (bagian terakhir boleh memuat apa saja selain "|").
func encodeCursor(parts ...string) string {
	return base64.RawURLEncoding.EncodeToString([]byte(strings.Join(parts, "|")))
}

func decodeCursor(c string, n int) ([]string, bool) {
	raw, err := base64.RawURLEncoding.DecodeString(c)
	if err != nil {
		return nil, false
	}
	parts := strings.SplitN(string(raw), "|", n)
	if len(parts) != n {
		return nil, false
	}
	return parts, true
}

func tsCursor(t time.Time) string { return strconv.FormatInt(t.UnixMicro(), 10) }

func parseTS(s string) (time.Time, bool) {
	us, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return time.Time{}, false
	}
	return time.UnixMicro(us).UTC(), true
}
