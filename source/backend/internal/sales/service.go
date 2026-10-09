// Package sales: penjualan/kasir (Fase 5.1). Satu nota = satu transaksi DB: header, baris, pembayaran, movement stok
// (ledger), dan audit commit atau batal bersama. Harga dan total dihitung SERVER dari master (AGENTS.md §3.3);
// klien hanya mengirim item, satuan, qty, potongan, dan pembayaran.
//
// Urutan hitung (diputuskan 2026-10-08, tanpa pembulatan):
//
//	baris   = harga satuan × qty − potongan baris
//	subtotal = Σ baris
//	dasar   = subtotal − potongan global
//	total   = dasar + pajak toko + pajak negara (% outlet × dasar, bila diminta) + biaya lain
package sales

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
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/shopspring/decimal"

	"aciraba/internal/approval"
	"aciraba/internal/audit"
	"aciraba/internal/authz"
	gen "aciraba/internal/gen"
	"aciraba/internal/item"
	"aciraba/internal/member"
	"aciraba/internal/platform/db"
	"aciraba/internal/platform/sanitize"
	"aciraba/internal/receivable"
	"aciraba/internal/stock"
	"aciraba/internal/voucher"
)

const (
	maxLines    = 500
	maxPayments = 10
	maxCosts    = 20
	maxCostName = 80
)

var maxMoney = decimal.New(1, 12) // batas atas nilai uang/qty (sama dengan batas kolom yang masuk akal)

// Kode galat per field (diterjemahkan klien: errors.FIELD_<kode>).
const (
	codeBelowCost     = "BELOW_COST"
	codeStockShort    = "STOCK_INSUFFICIENT"
	codeDiscountOver  = "DISCOUNT_TOO_HIGH"
	codeUnitUnknown   = "UNIT_UNKNOWN"
	codeItemMissing   = "ITEM_UNAVAILABLE"
	codePaymentShort  = "PAYMENT_SHORT"
	codeNonCashOver   = "NON_CASH_OVER"
	codeTooMany       = "TOO_MANY"
	codeBaseQtyFormat = "INVALID"

	codeMemberRequired      = "MEMBER_REQUIRED"
	codeMemberInactive      = "MEMBER_INACTIVE"
	codeSalespersonInactive = "SALESPERSON_INACTIVE"
	codePointsInsufficient  = "POINTS_INSUFFICIENT"
	codeRedeemNotAllowed    = "REDEEM_NOT_ALLOWED"
	codeRedeemTooHigh       = "REDEEM_TOO_HIGH"
	codeRedeemBelowCost     = "REDEEM_BELOW_COST"

	codeVoucherTooHigh   = "VOUCHER_TOO_HIGH"
	codeVoucherBelowCost = "VOUCHER_BELOW_COST"
	codeVoucherDuplicate = "DUPLICATE"

	codeCreditNotNeeded = "CREDIT_NOT_NEEDED"
)

const (
	maxRedeem   = 10_000_000
	maxVouchers = 5
)

var (
	ErrOutletInactive  = errors.New("outlet tidak aktif")
	ErrKeyRequired     = errors.New("Idempotency-Key wajib diisi")
	ErrKeyMismatch     = errors.New("Idempotency-Key sudah dipakai untuk permintaan yang berbeda")
	ErrNotFound        = errors.New("nota tidak ditemukan")
	ErrOutletForbidden = errors.New("tidak punya akses ke outlet nota ini")
	ErrReceivablePaid  = errors.New("piutang nota ini sudah dibayar sebagian/seluruhnya")
	validMethods       = map[string]bool{"cash": true, "debit": true, "credit_card": true, "ewallet": true, "transfer": true}
	idemKeyPattern     = regexp.MustCompile(`^[A-Za-z0-9_.:-]{8,100}$`)
)

// FieldErrors = kode galat per field, mis. {"lines.0.qty": "INVALID"}.
type FieldErrors map[string]string

func (f FieldErrors) Error() string { return "input tidak valid" }

// CreditLimitError = nota kredit melewati limit piutang member dan belum disetujui penyetuju (PIN).
type CreditLimitError struct{ Limit, Outstanding, Receivable dec }

func (e *CreditLimitError) Error() string { return "limit piutang member terlampaui" }

// StockError = stok barang tidak cukup (nota dibatalkan seluruhnya).
type StockError struct {
	ItemID uuid.UUID
	SKU    string
	Name   string
}

func (e *StockError) Error() string { return fmt.Sprintf("stok %s tidak mencukupi", e.Name) }
func (e *StockError) Unwrap() error { return stock.ErrInsufficient }

// ---- Permintaan ----

type LineIn struct {
	ItemID   uuid.UUID   `json:"item_id"`
	UnitID   *uuid.UUID  `json:"unit_id"` // kosong = satuan dasar barang
	Qty      json.Number `json:"qty"`
	Discount json.Number `json:"discount"` // potongan baris (nilai rupiah untuk seluruh baris)
	// UnitPrice = harga satuan pengganti (ubah harga). Hanya sah dengan persetujuan PIN Owner/Supervisor (Request.Approval).
	UnitPrice json.Number `json:"unit_price"`
	Note      string      `json:"note"`
}

type PaymentIn struct {
	// MethodID = metode dari master Metode Pembayaran (disarankan). Method (jenis dasar: cash, debit, ...) hanya untuk
	// pemanggil lama: dipetakan ke metode aktif tertua berjenis itu.
	MethodID *uuid.UUID  `json:"method_id"`
	Method   string      `json:"method"`
	Amount   json.Number `json:"amount"` // yang DITERIMA; tunai boleh melebihi sisa (selisihnya kembalian)
	RefNo    string      `json:"ref_no"`
}

// ApprovalIn = penyetuju (Owner/Supervisor) beserta PIN-nya untuk ubah harga.
// CostIn = satu baris rincian biaya lain-lain (mis. Ongkir).
type CostIn struct {
	Name   string      `json:"name"`
	Amount json.Number `json:"amount"`
}

type ApprovalIn struct {
	UserID uuid.UUID `json:"user_id"`
	PIN    string    `json:"pin"`
}

type Request struct {
	Approval  *ApprovalIn `json:"approval"`
	Lines     []LineIn    `json:"lines"`
	Discount  json.Number `json:"discount"`   // potongan global
	OtherCost json.Number `json:"other_cost"` // biaya lain-lain (satu angka tanpa rincian)
	// OtherCosts = rincian biaya lain-lain (nama + jumlah). Bila ada, totalnya menjadi other_cost; other_cost yang ikut dikirim harus sama.
	OtherCosts []CostIn    `json:"other_costs"`
	ApplyTax   bool        `json:"apply_tax"` // terapkan pajak toko & negara sesuai tarif outlet
	Payments   []PaymentIn `json:"payments"`
	Note       string      `json:"note"`
	// Member (opsional): nota diperhitungkan poinnya; RedeemPoints = poin yang ditukar jadi potongan nota (level member menentukan nilainya).
	MemberID     *uuid.UUID `json:"member_id"`
	RedeemPoints int        `json:"redeem_points"`
	// Salesman (opsional): label nota untuk laporan/komisi; kosong = Umum.
	SalespersonID *uuid.UUID `json:"salesperson_id"`
	// Kupon belanja global (kode dibuat pemilik): potongan dihitung server sebelum pajak; boleh lebih dari satu.
	VoucherCodes []string `json:"voucher_codes"`
	// Credit = nota kredit: bagian yang belum dibayar (total − pembayaran/DP) menjadi piutang member. Wajib memilih member.
	// Melewati limit piutang member butuh persetujuan PIN penyetuju berizin credit_limit.approve (Approval).
	Credit bool `json:"credit"`
}

// ---- Hasil ----

type Line struct {
	ItemID    uuid.UUID `json:"item_id"`
	SKU       string    `json:"sku"`
	Name      string    `json:"name"`
	UnitID    uuid.UUID `json:"unit_id"`
	Unit      string    `json:"unit"`
	Factor    string    `json:"factor"`
	Qty       string    `json:"qty"`
	UnitPrice string    `json:"unit_price"`
	Discount  string    `json:"discount"`
	LineTotal string    `json:"line_total"`
	Note      string    `json:"note"`
	// Hanya pada quote: Issue = STOCK_INSUFFICIENT | BELOW_COST (baris tak boleh dibayar); Available = stok display
	// yang tersisa (satuan dasar) untuk barang berstok yang tak boleh minus.
	// ListPrice = harga hasil hitung server sebelum diubah; PriceOverride = harga baris ini diubah dengan persetujuan.
	ListPrice     string `json:"list_price,omitempty"`
	PriceOverride bool   `json:"price_override,omitempty"`
	Issue         string `json:"issue,omitempty"`
	Available     string `json:"available,omitempty"`
}

type Payment struct {
	Method     string    `json:"method"` // jenis dasar
	MethodID   uuid.UUID `json:"method_id"`
	MethodName string    `json:"method_name"` // nama saat nota dibuat
	Amount     string    `json:"amount"`
	RefNo      string    `json:"ref_no"`
	FeePct     string    `json:"fee_pct"`
	Fee        string    `json:"fee"`        // biaya metode (MDR)
	FeeBearer  string    `json:"fee_bearer"` // "store" = ditanggung toko; "customer" = ditagihkan ke pelanggan
}

type Sale struct {
	ID          uuid.UUID `json:"id"`
	DocNo       string    `json:"doc_no"`
	Status      string    `json:"status"`
	OutletID    uuid.UUID `json:"outlet_id"`
	CashierName string    `json:"cashier"`
	CreatedAt   time.Time `json:"created_at"`
	Note        string    `json:"note"`
	Lines       []Line    `json:"lines"`
	Payments    []Payment `json:"payments"`
	ApprovedBy  string    `json:"approved_by,omitempty"`
	Subtotal    string    `json:"subtotal"`
	Discount    string    `json:"discount"`
	TaxStorePct string    `json:"tax_store_pct"`
	TaxGovPct   string    `json:"tax_gov_pct"`
	TaxStore    string    `json:"tax_store"`
	TaxGov      string    `json:"tax_gov"`
	OtherCost   string    `json:"other_cost"`
	Total       string    `json:"total"`
	Paid        string    `json:"paid"`
	Change      string    `json:"change"`
	// Surcharge = biaya metode yang ditagihkan ke pelanggan (di luar Total). Ditagih = Total + Surcharge.
	Surcharge string `json:"surcharge"`
	// Receivable = sisa yang belum dibayar saat nota dibuat (nota kredit); Credit = keadaan piutangnya sekarang.
	Receivable string               `json:"receivable"`
	Credit     *receivable.SaleInfo `json:"credit,omitempty"`
	// Member & poin nota (Discount sudah memuat RedeemAmount).
	Member         *MemberInfo      `json:"member,omitempty"`
	Salesperson    *SalespersonInfo `json:"salesperson,omitempty"`
	PointsEarned   int              `json:"points_earned"`
	PointsRedeemed int              `json:"points_redeemed"`
	RedeemAmount   string           `json:"redeem_amount"`
	Vouchers       []VoucherInfo    `json:"vouchers"`
	// OtherCosts = rincian biaya lain-lain (kosong untuk nota tanpa rincian; OtherCost tetap totalnya).
	OtherCosts []CostInfo `json:"other_costs"`
	// Revisi/batal: Revision 1 = nota asli; RootID = nota asli rantai; SupersededBy terisi bila sudah digantikan revisi.
	Revision       int        `json:"revision"`
	RootID         uuid.UUID  `json:"root_id"`
	SupersedesID   *uuid.UUID `json:"supersedes_id,omitempty"`
	SupersededBy   *uuid.UUID `json:"superseded_by,omitempty"`
	RevisionReason string     `json:"revision_reason,omitempty"`
	VoidReason     string     `json:"void_reason,omitempty"`
}

// CostInfo = satu baris rincian biaya lain-lain pada nota.
type CostInfo struct {
	Name   string `json:"name"`
	Amount string `json:"amount"`
}

// VoucherInfo = kupon yang dipakai nota/quote beserta potongannya (sudah termasuk di Discount).
type VoucherInfo struct {
	Code   string `json:"code"`
	Name   string `json:"name"`
	Kind   string `json:"kind"`
	Value  string `json:"value"`
	Amount string `json:"amount"`
}

// MemberInfo = identitas member pada nota/quote.
type SalespersonInfo struct {
	ID   uuid.UUID `json:"id"`
	Name string    `json:"name"`
}

type MemberInfo struct {
	ID     uuid.UUID `json:"id"`
	Code   string    `json:"code"`
	Name   string    `json:"name"`
	Points int       `json:"points"` // saldo poin saat quote
}

type Service struct {
	pool      *pgxpool.Pool
	approvals *approval.Service // nil = ubah harga tidak tersedia (tes)
}

func NewService(pool *pgxpool.Pool, approvals *approval.Service) *Service {
	return &Service{pool: pool, approvals: approvals}
}

// ---- Validasi & normalisasi ----

type dec = decimal.Decimal

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
	in       LineIn
	qty      dec
	discount dec
	note     string
	override *dec // harga satuan pengganti (nil = tidak diubah)
}

type normCost struct {
	name   string
	amount dec
}

type normPayment struct {
	method   string // jenis dasar (diisi saat normalize bila hanya jenis dikirim; selalu diisi resolvePayments)
	methodID *uuid.UUID
	name     string
	amount   dec
	refNo    string
	feePct   dec    // snapshot tarif metode saat nota dibuat
	bearer   string // "store" | "customer"
	feeFlat  dec
	fee      dec // biaya yang ditanggung toko (tidak mengubah total belanja)
}

type norm struct {
	lines       []normLine
	discount    dec
	otherCost   dec
	costs       []normCost // rincian biaya lain (bila ada, otherCost = jumlahnya)
	applyTax    bool
	payments    []normPayment
	note        string
	memberID    *uuid.UUID
	redeem      int
	salesperson *uuid.UUID
	vouchers    []string // kode kupon (HURUF BESAR, tanpa duplikat)
	credit      bool
	// keep (hanya edit nota): baris nota asli per item|satuan. Baris yang cocok mempertahankan harga, harga normal, dan HPP
	// saat transaksi awal (tidak dihitung ulang dari master); hanya baris baru yang memakai harga/HPP sekarang.
	keep map[string][]gen.SalesLinesRow
}

func normalize(in Request) (norm, FieldErrors) {
	f := FieldErrors{}
	n := norm{applyTax: in.ApplyTax, memberID: in.MemberID, redeem: in.RedeemPoints, credit: in.Credit}
	if in.MemberID != nil && *in.MemberID == uuid.Nil {
		n.memberID = nil
	}
	if in.Credit && n.memberID == nil {
		f["credit"] = codeMemberRequired
	}
	if in.SalespersonID != nil && *in.SalespersonID != uuid.Nil {
		n.salesperson = in.SalespersonID
	}
	if in.RedeemPoints < 0 || in.RedeemPoints > maxRedeem {
		f["redeem_points"] = sanitize.Invalid
	} else if in.RedeemPoints > 0 && n.memberID == nil {
		f["redeem_points"] = codeMemberRequired
	}
	if len(in.VoucherCodes) > maxVouchers {
		f["voucher_codes"] = codeTooMany
	}
	seenV := map[string]bool{}
	for i, raw := range in.VoucherCodes {
		if i >= maxVouchers {
			break
		}
		k := fmt.Sprintf("voucher_codes.%d", i)
		c := voucher.NormalizeCode(raw)
		switch {
		case !voucher.CodeRe.MatchString(c):
			f[k] = voucher.CodeUnknown
		case seenV[c]:
			f[k] = codeVoucherDuplicate
		default:
			seenV[c] = true
			n.vouchers = append(n.vouchers, c)
		}
	}
	if len(in.Lines) == 0 {
		f["lines"] = sanitize.Required
	} else if len(in.Lines) > maxLines {
		f["lines"] = codeTooMany
	}
	for i, l := range in.Lines {
		if i >= maxLines {
			break
		}
		k := fmt.Sprintf("lines.%d.", i)
		nl := normLine{in: l}
		if l.ItemID == uuid.Nil {
			f[k+"item_id"] = sanitize.Required
		}
		var c string
		if nl.qty, c = parseDec(l.Qty, 3, true); c != "" {
			f[k+"qty"] = c
		} else if !nl.qty.IsPositive() {
			f[k+"qty"] = sanitize.Invalid
		}
		if nl.discount, c = parseDec(l.Discount, 2, false); c != "" {
			f[k+"discount"] = c
		}
		if strings.TrimSpace(l.UnitPrice.String()) != "" {
			if p, c := parseDec(l.UnitPrice, 2, true); c != "" {
				f[k+"unit_price"] = c
			} else {
				nl.override = &p
			}
		}
		note, ok := sanitize.Text(l.Note)
		if !ok || utf8.RuneCountInString(note) > 200 {
			f[k+"note"] = sanitize.Invalid
		}
		nl.note = note
		n.lines = append(n.lines, nl)
	}
	var c string
	if n.discount, c = parseDec(in.Discount, 2, false); c != "" {
		f["discount"] = c
	}
	if n.otherCost, c = parseDec(in.OtherCost, 2, false); c != "" {
		f["other_cost"] = c
	}
	if len(in.OtherCosts) > maxCosts {
		f["other_costs"] = codeTooMany
	}
	sum := decimal.Zero
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
		sum = sum.Add(amt)
		n.costs = append(n.costs, normCost{name: name, amount: amt})
	}
	if len(n.costs) > 0 {
		if strings.TrimSpace(in.OtherCost.String()) != "" && !n.otherCost.Equal(sum) {
			f["other_cost"] = sanitize.Invalid
		}
		n.otherCost = sum
	}
	note, ok := sanitize.Text(in.Note)
	if !ok || utf8.RuneCountInString(note) > 500 {
		f["note"] = sanitize.Invalid
	}
	n.note = note
	if len(in.Payments) > maxPayments {
		f["payments"] = codeTooMany
	}
	for i, p := range in.Payments {
		if i >= maxPayments {
			break
		}
		k := fmt.Sprintf("payments.%d.", i)
		np := normPayment{method: p.Method, methodID: p.MethodID}
		if p.MethodID != nil {
			np.method = "" // jenis ditentukan dari master
			if *p.MethodID == uuid.Nil {
				f[k+"method_id"] = sanitize.Invalid
			}
		} else if !validMethods[p.Method] {
			f[k+"method"] = sanitize.Invalid
		}
		if np.amount, c = parseDec(p.Amount, 2, true); c != "" {
			f[k+"amount"] = c
		} else if !np.amount.IsPositive() {
			f[k+"amount"] = sanitize.Invalid
		}
		ref, ok := sanitize.Text(p.RefNo)
		if !ok || utf8.RuneCountInString(ref) > 100 {
			f[k+"ref_no"] = sanitize.Invalid
		}
		np.refNo = ref
		n.payments = append(n.payments, np)
	}
	return n, f
}

// hash = sidik jari permintaan yang sudah dinormalkan (kunci idempotensi yang sama dengan isi berbeda ditolak).
func (n norm) hash() string {
	type l struct {
		Item, Unit, Qty, Disc, Note, Price string
	}
	type p struct{ M, A, R string }
	v := struct {
		L    []l
		P    []p
		D, O string
		T    bool
		N    string
		M    string
		R    int
		S    string
		V    []string
		C    []p
		K    bool
	}{K: n.credit, V: n.vouchers, D: n.discount.String(), O: n.otherCost.String(), T: n.applyTax, N: n.note, R: n.redeem}
	if n.memberID != nil {
		v.M = n.memberID.String()
	}
	if n.salesperson != nil {
		v.S = n.salesperson.String()
	}
	for _, c := range n.costs {
		v.C = append(v.C, p{c.name, c.amount.String(), ""})
	}
	for _, x := range n.lines {
		u := ""
		if x.in.UnitID != nil {
			u = x.in.UnitID.String()
		}
		pr := ""
		if x.override != nil {
			pr = x.override.String()
		}
		v.L = append(v.L, l{x.in.ItemID.String(), u, x.qty.String(), x.discount.String(), x.note, pr})
	}
	for _, x := range n.payments {
		m := x.method
		if x.methodID != nil {
			m = x.methodID.String()
		}
		v.P = append(v.P, p{m, x.amount.String(), x.refNo})
	}
	raw, _ := json.Marshal(v)
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

func numToDec(n pgtype.Numeric) (dec, bool) {
	if !n.Valid || n.NaN || n.InfinityModifier != pgtype.Finite {
		return decimal.Zero, false
	}
	return decimal.NewFromBigInt(n.Int, n.Exp), true
}

// ---- Hitung ----

type itemInfo struct {
	row   gen.SalesItemsForPricingRow
	base  dec // harga jual per satuan dasar (cabang bila ada, selain itu default)
	tiers []item.Tier
	alts  map[uuid.UUID]gen.SalesAltUnitsRow
}

type calcLine struct {
	item       itemInfo
	unitID     uuid.UUID
	unitName   string
	factor     dec
	qty        dec
	baseQty    dec
	unitPrice  dec
	unitCost   dec
	discount   dec
	total      dec
	note       string
	issue      string
	available  string
	listPrice  dec
	overridden bool
	keptPrice  bool // harga ubahan dari nota asli (sudah disetujui dulu): tidak butuh PIN lagi
	keptDisc   bool // potongan baris sama dengan nota asli: tidak butuh PIN lagi
}

type totals struct {
	subtotal, discount, taxStore, taxGov, other, total, paid, change, receivable dec
	taxStorePct, taxGovPct                                                       dec
	surcharge                                                                    dec // biaya metode yang ditagihkan ke pelanggan (di luar total)
}

var hundred = decimal.NewFromInt(100)

func (s *Service) loadInfo(ctx context.Context, q *gen.Queries, a authz.Actor, ids []uuid.UUID) (map[uuid.UUID]*itemInfo, error) {
	rows, err := q.SalesItemsForPricing(ctx, gen.SalesItemsForPricingParams{TenantID: a.TenantID, OutletID: a.OutletID, Ids: ids})
	if err != nil {
		return nil, err
	}
	out := make(map[uuid.UUID]*itemInfo, len(rows))
	for _, r := range rows {
		info := &itemInfo{row: r, base: r.SellPrice, alts: map[uuid.UUID]gen.SalesAltUnitsRow{}}
		if p, ok := numToDec(r.OutletPrice); ok {
			info.base = p
		}
		out[r.ID] = info
	}
	tiers, err := q.SalesTiers(ctx, gen.SalesTiersParams{TenantID: a.TenantID, OutletID: pgtype.UUID{Bytes: a.OutletID, Valid: true}, Ids: ids})
	if err != nil {
		return nil, err
	}
	// Set cabang menggantikan SELURUH set default (aturan produk), jadi pisahkan dulu lalu pilih.
	def, own := map[uuid.UUID][]item.Tier{}, map[uuid.UUID][]item.Tier{}
	for _, t := range tiers {
		x := item.Tier{MinQty: t.MinQty, Price: t.Price}
		if t.OutletID.Valid {
			own[t.ItemID] = append(own[t.ItemID], x)
		} else {
			def[t.ItemID] = append(def[t.ItemID], x)
		}
	}
	for id, info := range out {
		info.tiers = item.ChooseTiers(def[id], own[id])
	}
	alts, err := q.SalesAltUnits(ctx, gen.SalesAltUnitsParams{TenantID: a.TenantID, Ids: ids})
	if err != nil {
		return nil, err
	}
	for _, u := range alts {
		if info := out[u.ItemID]; info != nil {
			info.alts[u.UnitID] = u
		}
	}
	return out, nil
}

// price menghitung semua baris dan total. Mengembalikan galat per field bila ada yang tidak valid.
func price(n norm, infos map[uuid.UUID]*itemInfo, taxStorePct, taxGovPct dec) ([]calcLine, totals, FieldErrors) {
	f := FieldErrors{}
	lines := make([]calcLine, len(n.lines))
	totalBase := map[uuid.UUID]dec{}

	// Pass 1: satuan, faktor, qty dasar (grosir ditentukan dari TOTAL qty dasar per barang di seluruh nota).
	for i, l := range n.lines {
		k := fmt.Sprintf("lines.%d.", i)
		info := infos[l.in.ItemID]
		if info == nil || !info.row.Active {
			f[k+"item_id"] = codeItemMissing
			continue
		}
		cl := calcLine{item: *info, unitID: info.row.UnitID, unitName: info.row.UnitName, factor: decimal.NewFromInt(1), qty: l.qty, discount: l.discount, note: l.note}
		if l.in.UnitID != nil && *l.in.UnitID != info.row.UnitID {
			alt, ok := info.alts[*l.in.UnitID]
			if !ok {
				f[k+"unit_id"] = codeUnitUnknown
				continue
			}
			cl.unitID, cl.unitName, cl.factor = alt.UnitID, alt.UnitName, alt.Factor
		}
		cl.baseQty = cl.qty.Mul(cl.factor)
		if !cl.baseQty.Equal(cl.baseQty.Round(3)) {
			f[k+"qty"] = codeBaseQtyFormat // qty dasar harus bisa dicatat di stok (3 desimal)
			continue
		}
		lines[i] = cl
		totalBase[info.row.ID] = totalBase[info.row.ID].Add(cl.baseQty)
	}
	if len(f) > 0 {
		return nil, totals{}, f
	}

	// Pass 2: harga, potongan, HPP.
	var t totals
	keep := cloneKeep(n.keep)
	for i := range lines {
		k := fmt.Sprintf("lines.%d.", i)
		cl := &lines[i]
		info := cl.item
		fixed := false
		if cl.unitID != info.row.UnitID { // satuan tambahan dengan harga manual: dipakai apa adanya, tanpa grosir
			if p, has := numToDec(info.alts[cl.unitID].SellPrice); has {
				cl.unitPrice, fixed = p, true
			}
		}
		if !fixed {
			perBase, err := item.PriceFor(info.base, info.tiers, totalBase[info.row.ID])
			if err != nil {
				f[k+"qty"] = sanitize.Invalid
				continue
			}
			cl.unitPrice = perBase.Mul(cl.factor).Round(2)
		}
		cl.listPrice = cl.unitPrice
		if ov := n.lines[i].override; ov != nil && !ov.Equal(cl.listPrice) {
			cl.unitPrice, cl.overridden = *ov, true
		}
		cl.unitCost = info.row.AvgCost.Mul(cl.factor).Round(2)
		if k := popKeep(keep, info.row.ID, cl.unitID); k != nil {
			// Baris yang sudah ada di nota asli: harga, harga normal, dan HPP dipertahankan.
			cl.listPrice, cl.unitCost = k.ListPrice, k.UnitCost
			cl.unitPrice, cl.overridden, cl.keptPrice = k.UnitPrice, k.PriceOverride, true
			if ov := n.lines[i].override; ov != nil && !ov.Equal(k.UnitPrice) {
				cl.unitPrice, cl.overridden, cl.keptPrice = *ov, !ov.Equal(cl.listPrice), false
			}
			cl.keptDisc = cl.discount.Equal(k.Discount)
		}
		gross := cl.unitPrice.Mul(cl.qty).Round(2)
		if cl.discount.GreaterThan(gross) {
			f[k+"discount"] = codeDiscountOver
			continue
		}
		cl.total = gross.Sub(cl.discount)
		if !info.row.SellBelowCost && cl.total.LessThan(cl.unitCost.Mul(cl.qty).Round(2)) {
			cl.issue = codeBelowCost // Create menolaknya; quote hanya menandai agar kasir bisa memperbaiki
		}
		t.subtotal = t.subtotal.Add(cl.total)
	}
	// Stok: total qty dasar per barang (lintas baris) tidak boleh melebihi stok display kecuali barang boleh minus.
	// Ini peringatan dini untuk kasir; penjaga yang sebenarnya (atomik) adalah stock.Apply saat nota disimpan.
	for i := range lines {
		info := lines[i].item
		if info.row.Kind != "goods" || info.row.AllowNegativeStock {
			continue
		}
		lines[i].available = info.row.StockDisplay.String()
		if totalBase[info.row.ID].GreaterThan(info.row.StockDisplay) && lines[i].issue == "" {
			lines[i].issue = codeStockShort
		}
	}
	if len(f) > 0 {
		return nil, totals{}, f
	}

	t.discount = n.discount
	if t.discount.GreaterThan(t.subtotal) {
		f["discount"] = codeDiscountOver
		return nil, totals{}, f
	}
	base := t.subtotal.Sub(t.discount)
	if n.applyTax {
		t.taxStorePct, t.taxGovPct = taxStorePct, taxGovPct
		t.taxStore = base.Mul(taxStorePct).Div(hundred).Round(2)
		t.taxGov = base.Mul(taxGovPct).Div(hundred).Round(2)
	}
	t.other = n.otherCost
	t.total = base.Add(t.taxStore).Add(t.taxGov).Add(t.other)

	if t.total.GreaterThanOrEqual(maxMoney) {
		f["lines"] = sanitize.Invalid
		return nil, totals{}, f
	}
	return lines, t, nil
}

// resolvePayments memetakan tiap pembayaran ke metode di master (harus ada dan aktif) dan mengisi jenis dasar + nama
// (snapshot untuk nota). Hanya dipanggil saat menyimpan; jenis dari master-lah yang dipakai settle().
func resolvePayments(ctx context.Context, q *gen.Queries, a authz.Actor, n norm) (norm, FieldErrors) {
	f := FieldErrors{}
	out := make([]normPayment, len(n.payments))
	for i, p := range n.payments {
		k := fmt.Sprintf("payments.%d.", i)
		var id uuid.UUID
		var name, kind string
		var active bool
		var feePct, feeFlat dec
		var bearer string
		var err error
		if p.methodID != nil {
			var r gen.SalesPaymentMethodByIDRow
			r, err = q.SalesPaymentMethodByID(ctx, gen.SalesPaymentMethodByIDParams{TenantID: a.TenantID, ID: *p.methodID})
			id, name, kind, active, feePct, feeFlat, bearer = r.ID, r.Name, r.Kind, r.Active, r.FeePct, r.FeeFlat, r.FeeBearer
		} else {
			var r gen.SalesPaymentMethodByKindRow
			r, err = q.SalesPaymentMethodByKind(ctx, gen.SalesPaymentMethodByKindParams{TenantID: a.TenantID, Kind: p.method})
			id, name, kind, active, feePct, feeFlat, bearer = r.ID, r.Name, r.Kind, r.Active, r.FeePct, r.FeeFlat, r.FeeBearer
		}
		switch {
		case errors.Is(err, pgx.ErrNoRows):
			f[k+"method_id"] = sanitize.Invalid
		case err != nil:
			return n, FieldErrors{"payments": "INTERNAL"}
		case !active:
			f[k+"method_id"] = "METHOD_INACTIVE"
		}
		p.methodID, p.name, p.method = &id, name, kind
		p.feePct, p.feeFlat, p.bearer = feePct, feeFlat, bearer
		p.fee = methodFee(p.amount, feePct, feeFlat)
		out[i] = p
	}
	if len(f) > 0 {
		return n, f
	}
	n.payments = out
	return n, nil
}

// methodFee = biaya metode (MDR) atas jumlah yang dibayar: amount × pct% + flat, dibulatkan 2 desimal. Ditanggung toko.
func methodFee(amount, pct, flat dec) dec {
	if !pct.IsPositive() && !flat.IsPositive() {
		return decimal.Zero
	}
	return amount.Mul(pct).Div(decimal.NewFromInt(100)).Add(flat).Round(2)
}

// settle memeriksa pembayaran terhadap total: non-tunai tidak boleh melebihi total (tidak ada kembalian kartu) dan
// total harus tertutup penuh; kembalian = diterima − total (selalu bisa diambil dari tunai).
func settle(n norm, t totals) (totals, FieldErrors) {
	cash := decimal.Zero
	for _, p := range n.payments {
		t.paid = t.paid.Add(p.amount)
		if p.bearer == "customer" {
			t.surcharge = t.surcharge.Add(p.fee)
		}
		if p.method == "cash" {
			cash = cash.Add(p.amount)
		}
	}
	switch {
	case t.paid.Sub(cash).GreaterThan(t.total):
		return t, FieldErrors{"payments": codeNonCashOver}
	case n.credit:
		// Nota kredit: pembayaran (DP) harus kurang dari total; sisanya menjadi piutang. Tidak ada kembalian.
		if !t.paid.LessThan(t.total) {
			return t, FieldErrors{"credit": codeCreditNotNeeded}
		}
		t.receivable = t.total.Sub(t.paid)
		return t, nil
	case t.paid.LessThan(t.total):
		return t, FieldErrors{"payments": codePaymentShort}
	}
	t.change = t.paid.Sub(t.total)
	return t, nil
}

// ---- Simpan ----

type replay struct{ id uuid.UUID }

func (replay) Error() string { return "replay" }

// Create menyimpan satu nota. replayed=true bila Idempotency-Key yang sama sudah pernah menyimpan nota
// (nota yang sama dikembalikan; tidak ada nota/stok ganda).
func (s *Service) Create(ctx context.Context, a authz.Actor, key string, in Request) (sale Sale, replayed bool, err error) {
	if !idemKeyPattern.MatchString(key) {
		return Sale{}, false, ErrKeyRequired
	}
	n, f := normalize(in)
	if len(f) > 0 {
		return Sale{}, false, f
	}
	h := n.hash()

	err = db.WithTenant(ctx, s.pool, a.TenantID, func(tx pgx.Tx) error {
		return s.save(ctx, tx, a, n, key, h, in.Approval, nil)
	})
	var rp replay
	if errors.As(err, &rp) {
		sale, err = s.Get(ctx, a, rp.id)
		return sale, true, err
	}
	if err != nil {
		return Sale{}, false, err
	}
	// Baca kembali nota yang baru disimpan (satu sumber bentuk respons).
	var id uuid.UUID
	err = db.WithTenant(ctx, s.pool, a.TenantID, func(tx pgx.Tx) error {
		ex, err := gen.New(tx).SalesByIdemKey(ctx, gen.SalesByIdemKeyParams{TenantID: a.TenantID, IdempotencyKey: key})
		id = ex.ID
		return err
	})
	if err != nil {
		return Sale{}, false, err
	}
	sale, err = s.Get(ctx, a, id)
	return sale, false, err
}

// save menyimpan satu nota di dalam transaksi pemanggil: hitung harga/kupon/member, nomor, header, baris, pembayaran,
// stok, poin, kupon, dan audit. ed != nil = revisi hasil edit (nota lama sudah dibalik dampaknya oleh pemanggil).
func (s *Service) save(ctx context.Context, tx pgx.Tx, a authz.Actor, n norm, key, h string, approvalIn *ApprovalIn, ed *editCtx) error {
	q := gen.New(tx)
	if ex, err := q.SalesByIdemKey(ctx, gen.SalesByIdemKeyParams{TenantID: a.TenantID, IdempotencyKey: key}); err == nil {
		if ex.RequestHash != h {
			return ErrKeyMismatch
		}
		return replay{ex.ID}
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return err
	}

	out, err := q.SalesOutletInfo(ctx, gen.SalesOutletInfoParams{TenantID: a.TenantID, ID: a.OutletID})
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && !out.Active) {
		return ErrOutletInactive
	}
	if err != nil {
		return err
	}

	day := out.LocalDay // hari bisnis untuk kupon/member; revisi memakai hari nota asli
	if ed != nil {
		day = ed.localDay
	}

	idSet := map[uuid.UUID]bool{}
	var ids []uuid.UUID
	for _, l := range n.lines {
		if !idSet[l.in.ItemID] {
			idSet[l.in.ItemID] = true
			ids = append(ids, l.in.ItemID)
		}
	}
	infos, err := s.loadInfo(ctx, q, a, ids)
	if err != nil {
		return err
	}
	lines, t, fe := price(n, infos, out.TaxStorePct, out.TaxGovPct)
	if len(fe) > 0 {
		return fe
	}
	vc, nv, lines, t, fe, err := resolveVouchers(ctx, tx, a, n, day, true, infos, out.TaxStorePct, out.TaxGovPct, lines, t)
	if err != nil {
		return err
	}
	if len(fe) > 0 {
		return fe
	}
	mc, lines, t, fe, err := resolveMember(ctx, tx, a, nv, day, true, infos, out.TaxStorePct, out.TaxGovPct, lines, t)
	if err != nil {
		return err
	}
	if n.salesperson != nil {
		sp, err := q.SalesSalespersonState(ctx, gen.SalesSalespersonStateParams{TenantID: a.TenantID, ID: *n.salesperson})
		if errors.Is(err, pgx.ErrNoRows) {
			return FieldErrors{"salesperson_id": sanitize.Invalid}
		}
		if err != nil {
			return err
		}
		if !sp.Active {
			return FieldErrors{"salesperson_id": codeSalespersonInactive}
		}
	}
	if len(fe) > 0 {
		return fe
	}
	for i, l := range lines {
		if l.issue == codeBelowCost {
			return FieldErrors{fmt.Sprintf("lines.%d.item_id", i): codeBelowCost}
		}
	}
	var approver approval.Approver
	overrides := overriddenLines(lines)
	discounted := discountedLines(lines)
	if len(overrides) > 0 || len(discounted) > 0 {
		if s.approvals == nil || approvalIn == nil {
			return approval.ErrPinRequired
		}
		if approver, err = s.approvals.Verify(ctx, tx, a, approvalIn.UserID, approvalIn.PIN); err != nil {
			return err
		}
	}
	if n, fe = resolvePayments(ctx, q, a, n); len(fe) > 0 {
		return fe
	}
	if t, fe = settle(n, t); len(fe) > 0 {
		return fe
	}
	// Nota kredit: cek limit piutang member (member sudah terkunci FOR UPDATE di resolveMember, jadi dua kasir yang
	// berkredit ke member yang sama antre dan yang kedua melihat piutang yang pertama).
	var creditApprover approval.Approver
	var terms receivable.Terms
	if t.receivable.IsPositive() {
		if terms, err = receivable.MemberTerms(ctx, tx, a.TenantID, mc.sm.ID); err != nil {
			return err
		}
		excl := uuid.Nil
		if ed != nil {
			excl = ed.orig.ID // piutang nota lama ikut digantikan revisi ini
		}
		outstanding, err := receivable.Outstanding(ctx, tx, a.TenantID, mc.sm.ID, excl)
		if err != nil {
			return err
		}
		if terms.Limit.IsPositive() && outstanding.Add(t.receivable).GreaterThan(terms.Limit) {
			if s.approvals == nil || approvalIn == nil {
				return &CreditLimitError{Limit: terms.Limit, Outstanding: outstanding, Receivable: t.receivable}
			}
			if creditApprover, err = s.approvals.VerifyFor(ctx, tx, a, approval.ModuleCreditLimit, a.OutletID, approvalIn.UserID, approvalIn.PIN); err != nil {
				return err
			}
		}
	}

	var docNo string
	var hdr gen.SalesInsertRow
	cashier := pgtype.UUID{Bytes: a.UserID, Valid: a.UserID != uuid.Nil}
	approvedBy := pgtype.UUID{Bytes: approver.ID, Valid: approver.ID != uuid.Nil}
	if ed == nil {
		no, nerr := q.SalesNextNo(ctx, gen.SalesNextNoParams{TenantID: a.TenantID, OutletID: a.OutletID, Day: out.LocalDay})
		if nerr != nil {
			return nerr
		}
		docNo = fmt.Sprintf("%s-%s-%04d", strings.ToUpper(out.Code), out.LocalDay.Time.Format("060102"), no)
		hdr, err = q.SalesInsert(ctx, gen.SalesInsertParams{
			TenantID: a.TenantID, OutletID: a.OutletID, DocNo: docNo, IdempotencyKey: key, RequestHash: h,
			CashierID: cashier, ApprovedBy: approvedBy, Note: n.note,
			Subtotal: t.subtotal, Discount: t.discount, TaxStorePct: t.taxStorePct, TaxGovPct: t.taxGovPct,
			TaxStore: t.taxStore, TaxGov: t.taxGov, OtherCost: t.other, Total: t.total, Paid: t.paid, Change: t.change, Surcharge: t.surcharge, Receivable: t.receivable,
			MemberID: pgtype.UUID{Bytes: mc.id(), Valid: mc.sm != nil}, PointsEarned: int32(mc.earn), PointsRedeemed: int32(n.redeemApplied(mc)), RedeemAmount: mc.redeemAmt,
			SalespersonID: pgtype.UUID{Bytes: n.salespersonBytes(), Valid: n.salesperson != nil},
		})
	} else {
		// Revisi: nomor = nomor asli + penanda revisi; kasir & tanggal bisnis dibawa dari nota asli.
		docNo = ed.docNo
		var r gen.SalesInsertRevisionRow
		r, err = q.SalesInsertRevision(ctx, gen.SalesInsertRevisionParams{
			TenantID: a.TenantID, OutletID: a.OutletID, DocNo: docNo, IdempotencyKey: key, RequestHash: h,
			CashierID: ed.orig.CashierID, ApprovedBy: approvedBy, Note: n.note,
			Subtotal: t.subtotal, Discount: t.discount, TaxStorePct: t.taxStorePct, TaxGovPct: t.taxGovPct,
			TaxStore: t.taxStore, TaxGov: t.taxGov, OtherCost: t.other, Total: t.total, Paid: t.paid, Change: t.change, Surcharge: t.surcharge, Receivable: t.receivable,
			MemberID: pgtype.UUID{Bytes: mc.id(), Valid: mc.sm != nil}, PointsEarned: int32(mc.earn), PointsRedeemed: int32(n.redeemApplied(mc)), RedeemAmount: mc.redeemAmt,
			SalespersonID: pgtype.UUID{Bytes: n.salespersonBytes(), Valid: n.salesperson != nil},
			CreatedAt:     ed.orig.CreatedAt, RootID: pgtype.UUID{Bytes: ed.rootID, Valid: true}, Revision: ed.orig.Revision + 1,
			SupersedesID: pgtype.UUID{Bytes: ed.orig.ID, Valid: true}, RevisionReason: pgtype.Text{String: ed.reason, Valid: true},
			RevisedBy: cashier,
		})
		hdr = gen.SalesInsertRow{ID: r.ID, CreatedAt: r.CreatedAt}
	}
	if errors.Is(err, pgx.ErrNoRows) {
		// Pengiriman ganda bersamaan: yang lain menang. Batalkan transaksi ini (nomor tidak terpakai) lalu kembalikan nota itu.
		ex, e2 := q.SalesByIdemKey(ctx, gen.SalesByIdemKeyParams{TenantID: a.TenantID, IdempotencyKey: key})
		if e2 != nil {
			return e2
		}
		if ex.RequestHash != h {
			return ErrKeyMismatch
		}
		return replay{ex.ID}
	}
	if err != nil {
		return err
	}
	if ed != nil {
		// Nota lama resmi digantikan; baris kunci FOR UPDATE sudah dipegang pemanggil, jadi tepat satu baris berubah.
		if n, uerr := q.SalesMarkSuperseded(ctx, gen.SalesMarkSupersededParams{TenantID: a.TenantID, ID: ed.orig.ID, SupersededBy: pgtype.UUID{Bytes: hdr.ID, Valid: true}}); uerr != nil {
			return uerr
		} else if n != 1 {
			return ErrNotEditable
		}
	}

	var moves []stock.Movement
	for i, l := range lines {
		if err := q.SalesLineInsert(ctx, gen.SalesLineInsertParams{
			TenantID: a.TenantID, SaleID: hdr.ID, Position: int32(i + 1), ItemID: l.item.row.ID, Sku: l.item.row.Sku,
			Name: l.item.row.Name, UnitID: l.unitID, UnitName: l.unitName, Factor: l.factor, Qty: l.qty,
			UnitPrice: l.unitPrice, UnitCost: l.unitCost, Discount: l.discount, LineTotal: l.total, Note: l.note,
			ListPrice: l.listPrice, PriceOverride: l.overridden,
		}); err != nil {
			return err
		}
		if l.item.row.Kind == "goods" {
			moves = append(moves, stock.Movement{TenantID: a.TenantID, OutletID: a.OutletID, ItemID: l.item.row.ID,
				Bucket: stock.BucketDisplay, Delta: l.baseQty.Neg(), RefType: stock.RefSale, RefID: hdr.ID, Note: docNo, ActorID: a.UserID})
		}
	}
	if t.receivable.IsPositive() {
		if err := receivable.Create(ctx, tx, a.TenantID, a.OutletID, hdr.ID, mc.sm.ID, t.receivable, receivable.DueDate(day, terms.DueDays)); err != nil {
			return err
		}
	}
	for i, oc := range n.costs {
		if err := q.SalesCostInsert(ctx, gen.SalesCostInsertParams{TenantID: a.TenantID, SaleID: hdr.ID, Position: int32(i + 1), Name: oc.name, Amount: oc.amount}); err != nil {
			return err
		}
	}
	for i, p := range n.payments {
		if err := q.SalesPaymentInsert(ctx, gen.SalesPaymentInsertParams{TenantID: a.TenantID, SaleID: hdr.ID, Position: int32(i + 1),
			Method: p.method, MethodID: *p.methodID, MethodName: p.name, Amount: p.amount, RefNo: p.refNo,
			FeePct: p.feePct, FeeFlat: p.feeFlat, FeeAmount: p.fee, FeeBearer: p.bearer}); err != nil {
			return err
		}
	}
	if mc.sm != nil {
		if err := member.ApplySale(ctx, tx, a, mc.sm.ID, hdr.ID, docNo, n.redeemApplied(mc), mc.earn); err != nil {
			return err
		}
	}
	for i, v := range vc.applied {
		if err := q.SalesVoucherInsert(ctx, gen.SalesVoucherInsertParams{TenantID: a.TenantID, SaleID: hdr.ID, VoucherID: v.ID, Position: int32(i + 1),
			Code: v.Code, Name: v.Name, Kind: v.Kind, Value: v.Value, Amount: v.Amount}); err != nil {
			return err
		}
		if err := voucher.Consume(ctx, tx, a.TenantID, v.ID); err != nil {
			return err
		}
	}
	moves = mergeMoves(moves)
	if _, err := stock.ApplyAll(ctx, tx, moves); err != nil {
		var ins *stock.InsufficientError
		if errors.As(err, &ins) {
			if info := infos[ins.ItemID]; info != nil {
				return &StockError{ItemID: ins.ItemID, SKU: info.row.Sku, Name: info.row.Name}
			}
		}
		return err
	}
	if len(overrides) > 0 {
		items := make([]map[string]string, 0, len(overrides))
		for _, l := range overrides {
			items = append(items, map[string]string{"sku": l.item.row.Sku, "list_price": l.listPrice.String(), "price": l.unitPrice.String()})
		}
		if err := audit.Record(ctx, tx, audit.FromActor(a), audit.Entry{
			Action: audit.ActionSalePriceOverride, Entity: audit.EntitySale, EntityID: hdr.ID.String(),
			Details: map[string]any{"doc_no": docNo, "approver_id": approver.ID.String(), "approver": approver.Name, "lines": items},
		}); err != nil {
			return err
		}
	}
	if len(discounted) > 0 {
		items := make([]map[string]string, 0, len(discounted))
		for _, l := range discounted {
			items = append(items, map[string]string{"sku": l.item.row.Sku, "price": l.unitPrice.String(), "qty": l.qty.String(), "discount": l.discount.String()})
		}
		if err := audit.Record(ctx, tx, audit.FromActor(a), audit.Entry{
			Action: audit.ActionSaleLineDiscount, Entity: audit.EntitySale, EntityID: hdr.ID.String(),
			Details: map[string]any{"doc_no": docNo, "approver_id": approver.ID.String(), "approver": approver.Name, "lines": items},
		}); err != nil {
			return err
		}
	}
	details := map[string]any{"doc_no": docNo, "outlet_id": a.OutletID.String(), "lines": len(lines),
		"total": t.total.String(), "paid": t.paid.String(), "discount": t.discount.String(),
		"member": mc.code(), "points_earned": mc.earn, "points_redeemed": n.redeemApplied(mc), "vouchers": vc.codes()}
	if t.receivable.IsPositive() {
		details["receivable"] = t.receivable.String()
		if creditApprover.ID != uuid.Nil {
			details["credit_approver_id"], details["credit_approver"] = creditApprover.ID.String(), creditApprover.Name
		}
	}
	if ed == nil {
		return audit.Record(ctx, tx, audit.FromActor(a), audit.Entry{Action: audit.ActionSaleCreate, Entity: audit.EntitySale, EntityID: hdr.ID.String(), Details: details})
	}
	details["previous_doc_no"], details["previous_total"] = ed.orig.DocNo, ed.orig.Total.String()
	details["revision"], details["reason"] = ed.orig.Revision+1, ed.reason
	details["approver_id"], details["approver"] = ed.editor.ID.String(), ed.editor.Name
	if err := audit.Record(ctx, tx, audit.FromActor(a), audit.Entry{Action: audit.ActionSaleEdit, Entity: audit.EntitySale, EntityID: hdr.ID.String(), Details: details}); err != nil {
		return err
	}
	return audit.Record(ctx, tx, audit.FromActor(a), audit.Entry{Action: audit.ActionSaleSuperseded, Entity: audit.EntitySale, EntityID: ed.orig.ID.String(),
		Details: map[string]any{"doc_no": ed.orig.DocNo, "superseded_by": docNo, "reason": ed.reason, "approver_id": ed.editor.ID.String(), "approver": ed.editor.Name}})
}

func keepKey(item, unit uuid.UUID) string { return item.String() + "|" + unit.String() }

func cloneKeep(in map[string][]gen.SalesLinesRow) map[string][]gen.SalesLinesRow {
	out := make(map[string][]gen.SalesLinesRow, len(in))
	for k, v := range in {
		out[k] = append([]gen.SalesLinesRow(nil), v...)
	}
	return out
}

// popKeep mengambil baris nota asli berikutnya untuk item+satuan itu (urutan baris dipertahankan bila ada baris ganda).
func popKeep(keep map[string][]gen.SalesLinesRow, item, unit uuid.UUID) *gen.SalesLinesRow {
	k := keepKey(item, unit)
	if len(keep[k]) == 0 {
		return nil
	}
	row := keep[k][0]
	keep[k] = keep[k][1:]
	return &row
}

// mergeMoves menjumlahkan movement untuk barang yang sama (baris ganda/satuan berbeda) menjadi satu pengurangan,
// supaya ledger memuat satu baris per barang per nota dan pemeriksaan stok memakai total sebenarnya.
func mergeMoves(in []stock.Movement) []stock.Movement {
	idx := map[uuid.UUID]int{}
	var out []stock.Movement
	for _, m := range in {
		if i, ok := idx[m.ItemID]; ok {
			out[i].Delta = out[i].Delta.Add(m.Delta)
			continue
		}
		idx[m.ItemID] = len(out)
		out = append(out, m)
	}
	return out
}

// Get membaca satu nota milik tenant (RLS) beserta baris dan pembayarannya.
func (s *Service) Get(ctx context.Context, a authz.Actor, id uuid.UUID) (Sale, error) {
	var out Sale
	err := db.WithTenant(ctx, s.pool, a.TenantID, func(tx pgx.Tx) error {
		q := gen.New(tx)
		h, err := q.SalesGet(ctx, gen.SalesGetParams{TenantID: a.TenantID, ID: id})
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNotFound
		}
		if err != nil {
			return err
		}
		// Nota di cabang yang tidak boleh diakses pemanggil ditolak (RLS hanya memisahkan tenant, bukan cabang).
		if h.OutletID != a.OutletID && !a.Outlets[h.OutletID] {
			return ErrOutletForbidden
		}
		// Nota kasir lain hanya boleh dibuka pemegang izin daftar penjualan (atau Platform Admin hanya-baca).
		if (!h.CashierID.Valid || uuid.UUID(h.CashierID.Bytes) != a.UserID) && a.Impersonator == uuid.Nil && !a.Perms.Has("sales_list", authz.ActView) {
			return ErrNotFound
		}
		ls, err := q.SalesLines(ctx, gen.SalesLinesParams{TenantID: a.TenantID, SaleID: id})
		if err != nil {
			return err
		}
		ps, err := q.SalesPayments(ctx, gen.SalesPaymentsParams{TenantID: a.TenantID, SaleID: id})
		if err != nil {
			return err
		}
		var spi *SalespersonInfo
		if h.SalespersonID.Valid {
			spi = &SalespersonInfo{ID: uuid.UUID(h.SalespersonID.Bytes), Name: h.SalespersonName}
		}
		var mi *MemberInfo
		if h.MemberID.Valid {
			mi = &MemberInfo{ID: uuid.UUID(h.MemberID.Bytes), Code: h.MemberCode, Name: h.MemberName}
		}
		out = Sale{ID: h.ID, DocNo: h.DocNo, Status: h.Status, OutletID: h.OutletID, CashierName: h.CashierName, CreatedAt: h.CreatedAt.Time,
			ApprovedBy: h.ApproverName, Note: h.Note, Subtotal: h.Subtotal.StringFixed(2), Discount: h.Discount.StringFixed(2),
			TaxStorePct: h.TaxStorePct.StringFixed(2), TaxGovPct: h.TaxGovPct.StringFixed(2),
			TaxStore: h.TaxStore.StringFixed(2), TaxGov: h.TaxGov.StringFixed(2), OtherCost: h.OtherCost.StringFixed(2),
			Total: h.Total.StringFixed(2), Paid: h.Paid.StringFixed(2), Change: h.Change.StringFixed(2), Surcharge: h.Surcharge.StringFixed(2), Receivable: h.Receivable.StringFixed(2),
			Salesperson: spi, Member: mi, PointsEarned: int(h.PointsEarned), PointsRedeemed: int(h.PointsRedeemed), RedeemAmount: h.RedeemAmount.StringFixed(2),
			Lines: make([]Line, 0, len(ls)), Payments: make([]Payment, 0, len(ps)),
			Revision: int(h.Revision), RootID: h.ID, RevisionReason: h.RevisionReason, VoidReason: h.VoidReason}
		if h.RootID.Valid {
			out.RootID = uuid.UUID(h.RootID.Bytes)
		}
		if h.SupersedesID.Valid {
			v := uuid.UUID(h.SupersedesID.Bytes)
			out.SupersedesID = &v
		}
		if h.SupersededBy.Valid {
			v := uuid.UUID(h.SupersededBy.Bytes)
			out.SupersededBy = &v
		}
		for _, l := range ls {
			out.Lines = append(out.Lines, Line{ItemID: l.ItemID, SKU: l.Sku, Name: l.Name, UnitID: l.UnitID, Unit: l.UnitName,
				Factor: l.Factor.String(), Qty: l.Qty.String(), UnitPrice: l.UnitPrice.StringFixed(2), Discount: l.Discount.StringFixed(2),
				LineTotal: l.LineTotal.StringFixed(2), Note: l.Note, ListPrice: l.ListPrice.StringFixed(2), PriceOverride: l.PriceOverride})
		}
		vs, err := q.SalesVouchers(ctx, gen.SalesVouchersParams{TenantID: a.TenantID, SaleID: id})
		if err != nil {
			return err
		}
		out.Vouchers = make([]VoucherInfo, 0, len(vs))
		for _, v := range vs {
			out.Vouchers = append(out.Vouchers, VoucherInfo{Code: v.Code, Name: v.Name, Kind: v.Kind, Value: v.Value.StringFixed(2), Amount: v.Amount.StringFixed(2)})
		}
		cs, err := q.SalesCosts(ctx, gen.SalesCostsParams{TenantID: a.TenantID, SaleID: id})
		if err != nil {
			return err
		}
		out.OtherCosts = make([]CostInfo, 0, len(cs))
		for _, c := range cs {
			out.OtherCosts = append(out.OtherCosts, CostInfo{Name: c.Name, Amount: c.Amount.StringFixed(2)})
		}
		if h.Receivable.IsPositive() {
			if out.Credit, err = receivable.ForSale(ctx, tx, a.TenantID, id); err != nil {
				return err
			}
		}
		for _, p := range ps {
			out.Payments = append(out.Payments, Payment{Method: p.Method, MethodID: p.MethodID, MethodName: p.MethodName, Amount: p.Amount.StringFixed(2), RefNo: p.RefNo,
				FeePct: p.FeePct.StringFixed(2), Fee: p.FeeAmount.StringFixed(2), FeeBearer: p.FeeBearer})
		}
		return nil
	})
	return out, err
}

// Quote = hasil hitung server tanpa menyimpan: pratinjau harga/total untuk layar kasir (harga grosir, satuan,
// pajak outlet). Tidak memeriksa stok dan tidak butuh pembayaran. Angka yang dipakai kasir selalu angka ini.
type Quote struct {
	Lines       []Line `json:"lines"`
	Subtotal    string `json:"subtotal"`
	Discount    string `json:"discount"`
	TaxStorePct string `json:"tax_store_pct"`
	TaxGovPct   string `json:"tax_gov_pct"`
	TaxStore    string `json:"tax_store"`
	TaxGov      string `json:"tax_gov"`
	OtherCost   string `json:"other_cost"`
	Total       string `json:"total"`
	// Member (bila dipilih): potongan dari tukar poin (sudah termasuk di Discount) dan poin yang akan diperoleh.
	Member       *MemberInfo `json:"member,omitempty"`
	RedeemAmount string      `json:"redeem_amount"`
	PointsEarn   int         `json:"points_earn"`
	// Kupon yang lolos dan total potongannya (sudah termasuk di Discount).
	Vouchers      []VoucherInfo `json:"vouchers"`
	VoucherAmount string        `json:"voucher_amount"`
	// Credit (bila member dipilih): syarat kredit member dan piutangnya sekarang, agar kasir tahu apakah nota kredit
	// melewati limit (Limit "0.00" = tanpa batas). Simpan nota tetap memeriksa ulang di server.
	Credit *CreditTerms `json:"credit,omitempty"`
}

// CreditTerms = syarat kredit member saat quote.
type CreditTerms struct {
	Limit       string `json:"limit"`
	Outstanding string `json:"outstanding"`
	DueDays     int    `json:"due_days"`
}

func (s *Service) Quote(ctx context.Context, a authz.Actor, in Request) (Quote, error) {
	in.Payments = nil
	n, f := normalize(in)
	if len(f) > 0 {
		return Quote{}, f
	}
	var out Quote
	err := db.WithTenant(ctx, s.pool, a.TenantID, func(tx pgx.Tx) (err error) {
		out, err = s.quoteTx(ctx, tx, a, n, nil, uuid.Nil)
		return err
	})
	return out, err
}

// errQuoteRollback membatalkan transaksi pratinjau edit (hasil sudah diambil; tidak ada yang boleh tersimpan).
var errQuoteRollback = errors.New("pratinjau edit selesai")

// QuoteEdit = pratinjau hitung untuk EDIT nota: persis seperti Edit, dampak nota lama dibalik lebih dulu (stok, poin yang
// ditukar, kuota kupon kembali) dan baris yang sudah ada mempertahankan harga/HPP aslinya — lalu SELURUH transaksi
// dibatalkan. Total yang tampil di layar edit = total yang akan disimpan.
func (s *Service) QuoteEdit(ctx context.Context, a authz.Actor, id uuid.UUID, in Request) (Quote, error) {
	in.Payments = nil
	n, f := normalize(in)
	if len(f) > 0 {
		return Quote{}, f
	}
	var out Quote
	err := db.WithTenant(ctx, s.pool, a.TenantID, func(tx pgx.Tx) error {
		q := gen.New(tx)
		orig, err := lockForEdit(ctx, q, a, id, true)
		if err != nil {
			return err
		}
		olds, err := q.SalesLines(ctx, gen.SalesLinesParams{TenantID: a.TenantID, SaleID: orig.ID})
		if err != nil {
			return err
		}
		n.keep = make(map[string][]gen.SalesLinesRow, len(olds))
		for _, l := range olds {
			k := keepKey(l.ItemID, l.UnitID)
			n.keep[k] = append(n.keep[k], l)
		}
		if err := reverseEffects(ctx, tx, q, a, orig); err != nil {
			return err
		}
		day := orig.LocalDay
		if out, err = s.quoteTx(ctx, tx, a, n, &day, orig.ID); err != nil {
			return err
		}
		return errQuoteRollback
	})
	if errors.Is(err, errQuoteRollback) {
		err = nil
	}
	return out, err
}

// quoteTx menghitung quote di dalam transaksi pemanggil (dayOverride = hari bisnis untuk kupon/member).
func (s *Service) quoteTx(ctx context.Context, tx pgx.Tx, a authz.Actor, n norm, dayOverride *pgtype.Date, excludeSale uuid.UUID) (Quote, error) {
	q := gen.New(tx)
	o, err := q.SalesOutletInfo(ctx, gen.SalesOutletInfoParams{TenantID: a.TenantID, ID: a.OutletID})
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && !o.Active) {
		return Quote{}, ErrOutletInactive
	}
	if err != nil {
		return Quote{}, err
	}
	day := o.LocalDay // hari bisnis untuk kupon/member; pratinjau edit memakai hari nota asli
	if dayOverride != nil {
		day = *dayOverride
	}
	seen := map[uuid.UUID]bool{}
	var ids []uuid.UUID
	for _, l := range n.lines {
		if !seen[l.in.ItemID] {
			seen[l.in.ItemID] = true
			ids = append(ids, l.in.ItemID)
		}
	}
	infos, err := s.loadInfo(ctx, q, a, ids)
	if err != nil {
		return Quote{}, err
	}
	lines, t, fe := price(n, infos, o.TaxStorePct, o.TaxGovPct)
	if len(fe) > 0 {
		return Quote{}, fe
	}
	vc, nv, lines, t, fe, err := resolveVouchers(ctx, tx, a, n, day, false, infos, o.TaxStorePct, o.TaxGovPct, lines, t)
	if err != nil {
		return Quote{}, err
	}
	if len(fe) > 0 {
		return Quote{}, fe
	}
	mc, lines, t, fe, err := resolveMember(ctx, tx, a, nv, day, false, infos, o.TaxStorePct, o.TaxGovPct, lines, t)
	if err != nil {
		return Quote{}, err
	}
	if len(fe) > 0 {
		return Quote{}, fe
	}
	out := Quote{RedeemAmount: mc.redeemAmt.StringFixed(2), PointsEarn: mc.earn, Subtotal: t.subtotal.StringFixed(2), Discount: t.discount.StringFixed(2), TaxStorePct: t.taxStorePct.StringFixed(2),
		TaxGovPct: t.taxGovPct.StringFixed(2), TaxStore: t.taxStore.StringFixed(2), TaxGov: t.taxGov.StringFixed(2),
		OtherCost: t.other.StringFixed(2), Total: t.total.StringFixed(2), Lines: make([]Line, 0, len(lines))}
	out.Vouchers, out.VoucherAmount = vc.infos(), vc.total().StringFixed(2)
	if mc.sm != nil {
		out.Member = &MemberInfo{ID: mc.sm.ID, Code: mc.sm.Code, Name: mc.sm.Name, Points: mc.sm.Points}
		terms, err := receivable.MemberTerms(ctx, tx, a.TenantID, mc.sm.ID)
		if err != nil {
			return Quote{}, err
		}
		outstanding, err := receivable.Outstanding(ctx, tx, a.TenantID, mc.sm.ID, excludeSale)
		if err != nil {
			return Quote{}, err
		}
		out.Credit = &CreditTerms{Limit: terms.Limit.StringFixed(2), Outstanding: outstanding.StringFixed(2), DueDays: terms.DueDays}
	}
	for _, l := range lines {
		out.Lines = append(out.Lines, Line{ItemID: l.item.row.ID, SKU: l.item.row.Sku, Name: l.item.row.Name, UnitID: l.unitID, Unit: l.unitName,
			Factor: l.factor.String(), Qty: l.qty.String(), UnitPrice: l.unitPrice.StringFixed(2), Discount: l.discount.StringFixed(2),
			LineTotal: l.total.StringFixed(2), Note: l.note, Issue: l.issue, Available: l.available, ListPrice: l.listPrice.StringFixed(2), PriceOverride: l.overridden})
	}
	return out, nil
}

// overriddenLines = baris yang harganya diubah dari harga hasil hitung server.
func overriddenLines(lines []calcLine) []calcLine {
	var out []calcLine
	for _, l := range lines {
		if l.overridden && !l.keptPrice {
			out = append(out, l)
		}
	}
	return out
}

// discountedLines = baris yang diberi potongan manual (butuh persetujuan PIN seperti ubah harga).
func discountedLines(lines []calcLine) []calcLine {
	var out []calcLine
	for _, l := range lines {
		if l.discount.IsPositive() && !l.keptDisc {
			out = append(out, l)
		}
	}
	return out
}

// memberCalc = hasil perhitungan member untuk satu nota (kosong bila nota tanpa member).
type memberCalc struct {
	sm        *member.SaleMember
	redeemAmt dec
	earn      int
}

func (m memberCalc) id() uuid.UUID {
	if m.sm == nil {
		return uuid.Nil
	}
	return m.sm.ID
}

func (m memberCalc) code() string {
	if m.sm == nil {
		return ""
	}
	return m.sm.Code
}

// redeemApplied = jumlah poin yang benar-benar ditukar pada nota ini.
func (n norm) salespersonBytes() [16]byte {
	if n.salesperson == nil {
		return [16]byte{}
	}
	return *n.salesperson
}

func (n norm) redeemApplied(m memberCalc) int {
	if m.sm == nil {
		return 0
	}
	return n.redeem
}

// resolveMember memuat member (dikunci bila lock=true), memeriksa dan menghitung tukar poin, lalu poin yang akan
// diperoleh. Tukar poin menjadi potongan nota tambahan, sehingga total dihitung ulang. Poin diperoleh dihitung dari
// (subtotal − seluruh potongan) menurut aturan level member saat ini.
func resolveMember(ctx context.Context, tx pgx.Tx, a authz.Actor, n norm, localDay pgtype.Date, lock bool, infos map[uuid.UUID]*itemInfo,
	taxStorePct, taxGovPct dec, lines []calcLine, t totals) (memberCalc, []calcLine, totals, FieldErrors, error) {
	if n.memberID == nil {
		return memberCalc{}, lines, t, nil, nil
	}
	sm, err := member.LockForSale(ctx, tx, a.TenantID, *n.memberID, localDay, lock)
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		return memberCalc{}, lines, t, FieldErrors{"member_id": sanitize.Invalid}, nil
	case errors.Is(err, member.ErrMemberInactive):
		return memberCalc{}, lines, t, FieldErrors{"member_id": codeMemberInactive}, nil
	case err != nil:
		return memberCalc{}, lines, t, nil, err
	}
	mc := memberCalc{sm: &sm}
	if n.redeem > 0 {
		switch {
		case !sm.PointValue.IsPositive():
			return mc, lines, t, FieldErrors{"redeem_points": codeRedeemNotAllowed}, nil
		case n.redeem > sm.Points:
			return mc, lines, t, FieldErrors{"redeem_points": codePointsInsufficient}, nil
		}
		amt := member.RedeemAmount(n.redeem, sm.PointValue)
		if amt.GreaterThan(t.subtotal.Sub(t.discount)) {
			return mc, lines, t, FieldErrors{"redeem_points": codeRedeemTooHigh}, nil
		}
		n.discount = n.discount.Add(amt)
		var fe FieldErrors
		if lines, t, fe = price(n, infos, taxStorePct, taxGovPct); len(fe) > 0 {
			return mc, nil, t, fe, nil
		}
		// Potongan dari poin tidak boleh membuat nota di bawah HPP barang (kecuali barang boleh jual rugi): tukar poin tidak
		// memakai persetujuan PIN seperti ubah harga, jadi batas HPP harus dijaga di sini.
		if t.subtotal.Sub(t.discount).LessThan(costFloor(lines)) {
			return mc, lines, t, FieldErrors{"redeem_points": codeRedeemBelowCost}, nil
		}
		mc.redeemAmt = amt
	}
	mc.earn = member.PointsFor(t.subtotal.Sub(t.discount), sm.SpendPerPoint)
	return mc, lines, t, nil, nil
}
