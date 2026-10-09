// Package purchasing: pembelian dari pemasok (Fase 6.2). Satu nota = satu transaksi DB: header, baris, biaya lain, hutang
// (bila kredit), movement stok PURCHASE (ledger) dan HPP rata-rata tertimbang per cabang berubah bersama atau batal bersama.
//
// Harga beli adalah masukan pengguna (bukan dihitung dari master, tidak seperti penjualan); yang dihitung server adalah
// nilai baris (diskon 4 tingkat), PPN masukan, alokasi biaya lain, HPP baris dan HPP rata-rata baru (lihat calc.go).
package purchasing

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/shopspring/decimal"

	"aciraba/internal/authz"
	"aciraba/internal/platform/db"
	"aciraba/internal/platform/sanitize"

	"github.com/jackc/pgx/v5"
)

// Module = modul izin: purchase_invoices (buat), purchase_list (lihat daftar), keduanya boleh melihat nota.
const (
	ModuleInvoices = "purchase_invoices"
	ModuleList     = "purchase_list"
)

const (
	maxLines     = 500
	maxCosts     = 20
	maxCostName  = 80
	maxBackdated = 400 // hari ke belakang yang masih boleh dipakai sebagai tanggal pembelian
)

var maxMoney = decimal.New(1, 12)

// Kode galat per field (diterjemahkan klien: errors.FIELD_<kode>).
const (
	codeItemUnavailable = "ITEM_UNAVAILABLE"
	codeNotStocked      = "NOT_STOCKED"
	codeSupplierInact   = "SUPPLIER_INACTIVE"
	codeInvoiceDup      = "INVOICE_DUPLICATE"
	codeTooMany         = "TOO_MANY"
	codeTooHigh         = "TOO_HIGH"
)

var (
	ErrOutletInactive   = errors.New("outlet tidak aktif")
	ErrKeyRequired      = errors.New("Idempotency-Key wajib diisi")
	ErrKeyMismatch      = errors.New("Idempotency-Key sudah dipakai untuk permintaan yang berbeda")
	ErrNotFound         = errors.New("pembelian tidak ditemukan")
	ErrOutletForbidden  = errors.New("tidak punya akses ke outlet nota ini")
	ErrNotEditable      = errors.New("nota tidak dapat diubah (sudah dibatalkan atau digantikan revisi)")
	ErrEditWindowClosed = errors.New("batas waktu edit nota sudah lewat")
	ErrOutletMismatch   = errors.New("nota milik outlet lain")
	ErrPayablePaid      = errors.New("hutang nota ini sudah dibayar")
	idemKeyPattern      = regexp.MustCompile(`^[A-Za-z0-9_.:-]{8,100}$`)
)

// FieldErrors = kode galat per field, mis. {"lines.0.unit_price": "INVALID"}.
type FieldErrors map[string]string

func (f FieldErrors) Error() string { return "input tidak valid" }

// ---- Permintaan ----

type LineIn struct {
	ItemID       uuid.UUID     `json:"item_id"`
	QtyDisplay   json.Number   `json:"qty_display"`   // stok masuk ke Display (satuan dasar)
	QtyWarehouse json.Number   `json:"qty_warehouse"` // stok masuk ke Gudang
	UnitPrice    json.Number   `json:"unit_price"`    // harga beli per satuan dasar, sebelum diskon, tanpa PPN
	Discounts    []json.Number `json:"discounts"`     // maks 4 tingkat bertingkat; < 100 = persen, ≥ 100 = rupiah
}

type CostIn struct {
	Name   string      `json:"name"`
	Amount json.Number `json:"amount"`
}

type Request struct {
	SupplierID        uuid.UUID   `json:"supplier_id"`
	SupplierInvoiceNo string      `json:"supplier_invoice_no"`
	PurchaseDate      string      `json:"purchase_date"` // YYYY-MM-DD; kosong = hari ini (zona waktu outlet)
	PaymentType       string      `json:"payment_type"`  // cash | credit
	DueDate           string      `json:"due_date"`      // hanya kredit; kosong = tanpa jatuh tempo
	TaxPct            json.Number `json:"tax_pct"`       // PPN masukan atas subtotal
	OtherCosts        []CostIn    `json:"other_costs"`
	Note              string      `json:"note"`
	Lines             []LineIn    `json:"lines"`
}

// parseDec membaca desimal non-negatif dengan maksimal `places` angka di belakang koma. Kosong → nol (bila !required).
func parseDec(n json.Number, places int32, required bool) (dec, string) {
	s := strings.TrimSpace(n.String())
	if s == "" {
		if required {
			return decimal.Zero, sanitize.Required
		}
		return decimal.Zero, ""
	}
	d, err := decimal.NewFromString(s)
	if err != nil || d.IsNegative() || !d.Equal(d.Round(places)) || d.GreaterThanOrEqual(maxMoney) {
		return decimal.Zero, sanitize.Invalid
	}
	return d, ""
}

type normLine struct {
	itemID    uuid.UUID
	qtyDisp   dec
	qtyWare   dec
	qty       dec
	price     dec
	disc      [4]dec
	lineTotal dec
}

type normCost struct {
	name   string
	amount dec
}

type norm struct {
	supplier uuid.UUID
	invoice  string
	date     string // masukan mentah (kosong = hari ini)
	credit   bool
	due      string
	taxPct   dec
	costs    []normCost
	other    dec
	note     string
	lines    []normLine
}

func parseDate(s string) (time.Time, bool) {
	t, err := time.Parse("2006-01-02", s)
	return t, err == nil
}

// normalize memeriksa permintaan. strict=false (quote/pratinjau) tidak mewajibkan pemasok & jenis pembayaran.
func normalize(in Request, strict bool) (norm, FieldErrors) {
	f := FieldErrors{}
	n := norm{supplier: in.SupplierID, date: strings.TrimSpace(in.PurchaseDate), due: strings.TrimSpace(in.DueDate)}
	if in.SupplierID == uuid.Nil && strict {
		f["supplier_id"] = sanitize.Required
	}
	switch in.PaymentType {
	case "cash":
	case "credit":
		n.credit = true
	default:
		if strict {
			f["payment_type"] = sanitize.Invalid
		}
	}
	if n.date != "" {
		if _, ok := parseDate(n.date); !ok {
			f["purchase_date"] = sanitize.Invalid
		}
	}
	if n.due != "" {
		if !n.credit {
			f["due_date"] = sanitize.Invalid
		} else if _, ok := parseDate(n.due); !ok {
			f["due_date"] = sanitize.Invalid
		}
	}
	inv, ok := sanitize.Text(in.SupplierInvoiceNo)
	if !ok || utf8.RuneCountInString(inv) > 60 {
		f["supplier_invoice_no"] = sanitize.Invalid
	}
	n.invoice = inv
	note, ok := sanitize.Text(in.Note)
	if !ok || utf8.RuneCountInString(note) > 500 {
		f["note"] = sanitize.Invalid
	}
	n.note = note

	var c string
	if n.taxPct, c = parseDec(in.TaxPct, 2, false); c != "" {
		f["tax_pct"] = c
	} else if n.taxPct.GreaterThan(decimal.NewFromInt(100)) {
		f["tax_pct"] = sanitize.Invalid
	}

	if len(in.OtherCosts) > maxCosts {
		f["other_costs"] = codeTooMany
	}
	for i, oc := range in.OtherCosts {
		if i >= maxCosts {
			break
		}
		k := fmt.Sprintf("other_costs.%d.", i)
		name, ok := sanitize.Text(oc.Name)
		if !ok || utf8.RuneCountInString(name) > maxCostName {
			f[k+"name"] = sanitize.Invalid
		}
		amt, c := parseDec(oc.Amount, 2, true)
		if c != "" {
			f[k+"amount"] = c
		} else if !amt.IsPositive() {
			f[k+"amount"] = sanitize.Invalid
		}
		n.other = n.other.Add(amt)
		n.costs = append(n.costs, normCost{name: name, amount: amt})
	}

	if len(in.Lines) == 0 {
		if strict {
			f["lines"] = sanitize.Required
		}
	} else if len(in.Lines) > maxLines {
		f["lines"] = codeTooMany
	}
	for i, l := range in.Lines {
		if i >= maxLines {
			break
		}
		k := fmt.Sprintf("lines.%d.", i)
		nl := normLine{itemID: l.ItemID}
		if l.ItemID == uuid.Nil {
			f[k+"item_id"] = sanitize.Required
		}
		if nl.qtyDisp, c = parseDec(l.QtyDisplay, 3, false); c != "" {
			f[k+"qty_display"] = c
		}
		if nl.qtyWare, c = parseDec(l.QtyWarehouse, 3, false); c != "" {
			f[k+"qty_warehouse"] = c
		}
		nl.qty = nl.qtyDisp.Add(nl.qtyWare)
		if !nl.qty.IsPositive() && f[k+"qty_display"] == "" && f[k+"qty_warehouse"] == "" {
			f[k+"qty_display"] = sanitize.Required
		}
		if nl.price, c = parseDec(l.UnitPrice, 4, true); c != "" {
			f[k+"unit_price"] = c
		}
		if len(l.Discounts) > 4 {
			f[k+"discounts"] = codeTooMany
		}
		for j, ds := range l.Discounts {
			if j >= 4 {
				break
			}
			dv, c := parseDec(ds, 2, false)
			if c != "" {
				f[fmt.Sprintf("%sdiscounts.%d", k, j)] = sanitize.Invalid
				continue
			}
			nl.disc[j] = dv
		}
		n.lines = append(n.lines, nl)
	}
	return n, f
}

// hash = sidik jari permintaan yang sudah dinormalkan (kunci idempotensi yang sama dengan isi berbeda ditolak).
func (n norm) hash() string {
	type l struct {
		Item, D, W, P string
		Disc          [4]string
	}
	type cst struct{ N, A string }
	v := struct {
		S, I, D, T, Du, Tax, Note string
		L                         []l
		C                         []cst
	}{S: n.supplier.String(), I: n.invoice, D: n.date, Du: n.due, Tax: n.taxPct.String(), Note: n.note}
	if n.credit {
		v.T = "credit"
	} else {
		v.T = "cash"
	}
	for _, c := range n.costs {
		v.C = append(v.C, cst{c.name, c.amount.String()})
	}
	for _, x := range n.lines {
		e := l{Item: x.itemID.String(), D: x.qtyDisp.String(), W: x.qtyWare.String(), P: x.price.String()}
		for i, d := range x.disc {
			e.Disc[i] = d.String()
		}
		v.L = append(v.L, e)
	}
	raw, _ := json.Marshal(v)
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

// ---- Perhitungan nominal (murni, tanpa DB) ----

type amounts struct {
	lines    []lineAmt
	subtotal dec
	taxPct   dec
	tax      dec
	other    dec
	total    dec
}

type lineAmt struct {
	total, alloc, unitCost dec
}

// computeAmounts menghitung nilai baris, PPN, total, alokasi biaya lain dan HPP baris. Tidak menyentuh stok/HPP lama.
func computeAmounts(n norm) (amounts, FieldErrors) {
	f := FieldErrors{}
	a := amounts{taxPct: n.taxPct, other: n.other}
	weights := make([]dec, len(n.lines))
	qtys := make([]dec, len(n.lines))
	for i, l := range n.lines {
		lt, ok := lineTotal(l.qty, l.price, l.disc)
		if !ok {
			f[fmt.Sprintf("lines.%d.discounts", i)] = sanitize.Invalid
		}
		if lt.GreaterThanOrEqual(maxLine) {
			f[fmt.Sprintf("lines.%d.unit_price", i)] = codeTooHigh
		}
		a.lines = append(a.lines, lineAmt{total: lt})
		a.subtotal = a.subtotal.Add(lt)
		weights[i], qtys[i] = lt, l.qty
	}
	a.tax = a.subtotal.Mul(n.taxPct).Shift(-2).Round(2)
	a.total = a.subtotal.Add(a.tax).Add(a.other)
	if a.total.GreaterThanOrEqual(maxTotal) && len(f) == 0 {
		f["lines"] = codeTooHigh
	}
	if len(f) > 0 {
		return a, f
	}
	alloc := allocate(a.other, weights, qtys)
	for i := range a.lines {
		a.lines[i].alloc = alloc[i]
		a.lines[i].unitCost = a.lines[i].total.Add(alloc[i]).DivRound(n.lines[i].qty, 2)
	}
	return a, nil
}

// ---- HPP rata-rata (butuh data barang) ----

// itemCost = keadaan HPP satu barang di cabang sebelum nota: stok semua bucket dan HPP rata-rata.
type itemCost struct {
	stock0 dec
	avg0   dec
}

type costStep struct {
	stockBefore, avgBefore, avgAfter dec
}

// runAverages menjalankan rata-rata tertimbang berurutan menurut baris nota (barang yang sama di beberapa baris
// dihitung bertahap). Mengembalikan langkah per baris + HPP akhir dan harga beli akhir per barang.
func runAverages(n norm, a amounts, state map[uuid.UUID]itemCost) (steps []costStep, finalAvg, lastCost map[uuid.UUID]dec) {
	stock := map[uuid.UUID]dec{}
	avg := map[uuid.UUID]dec{}
	finalAvg, lastCost = map[uuid.UUID]dec{}, map[uuid.UUID]dec{}
	for i, l := range n.lines {
		s, ok := stock[l.itemID]
		if !ok {
			st := state[l.itemID]
			s, avg[l.itemID] = st.stock0, st.avg0
		}
		before, avgBefore := s, avg[l.itemID]
		after := weightedAvg(before, avgBefore, l.qty, a.lines[i].unitCost)
		steps = append(steps, costStep{stockBefore: before, avgBefore: avgBefore, avgAfter: after})
		stock[l.itemID], avg[l.itemID] = before.Add(l.qty), after
		finalAvg[l.itemID], lastCost[l.itemID] = after, a.lines[i].unitCost
	}
	return steps, finalAvg, lastCost
}

type Service struct {
	pool *pgxpool.Pool
}

func NewService(pool *pgxpool.Pool) *Service { return &Service{pool: pool} }

// resolveDates menerjemahkan tanggal pembelian dan jatuh tempo menurut hari lokal outlet.
func resolveDates(n norm, localDay time.Time) (purchase time.Time, due *time.Time, f FieldErrors) {
	f = FieldErrors{}
	purchase = localDay
	if n.date != "" {
		purchase, _ = parseDate(n.date)
		if purchase.After(localDay) || purchase.Before(localDay.AddDate(0, 0, -maxBackdated)) {
			f["purchase_date"] = sanitize.Invalid
		}
	}
	if n.due != "" {
		d, _ := parseDate(n.due)
		if d.Before(purchase) || d.After(purchase.AddDate(5, 0, 0)) {
			f["due_date"] = sanitize.Invalid
		}
		due = &d
	}
	return purchase, due, f
}

// checkItem memvalidasi satu barang (ada, aktif, berstok); galat dipasang pada tiap baris yang memakainya.
func checkItem(idx []int, found, active bool, kind string, f FieldErrors) {
	code := ""
	switch {
	case !found || !active:
		code = codeItemUnavailable
	case kind != "goods":
		code = codeNotStocked
	}
	if code == "" {
		return
	}
	for _, i := range idx {
		f[fmt.Sprintf("lines.%d.item_id", i)] = code
	}
}

// distinctItems = id barang unik terurut (urutan penguncian yang sama di semua transaksi → tanpa deadlock) + indeks barisnya.
func distinctItems(n norm) ([]uuid.UUID, map[uuid.UUID][]int) {
	at := map[uuid.UUID][]int{}
	var ids []uuid.UUID
	for i, l := range n.lines {
		if _, ok := at[l.itemID]; !ok {
			ids = append(ids, l.itemID)
		}
		at[l.itemID] = append(at[l.itemID], i)
	}
	for i := 1; i < len(ids); i++ { // insertion sort pada string uuid (urutan sama dengan stock.ApplyAll)
		for j := i; j > 0 && ids[j].String() < ids[j-1].String(); j-- {
			ids[j], ids[j-1] = ids[j-1], ids[j]
		}
	}
	return ids, at
}

func (s *Service) tx(ctx context.Context, a authz.Actor, fn func(tx pgx.Tx) error) error {
	return db.WithTenant(ctx, s.pool, a.TenantID, fn)
}
