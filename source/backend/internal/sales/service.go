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
	"aciraba/internal/stock"
)

const (
	maxLines    = 500
	maxPayments = 10
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

	codeMemberRequired     = "MEMBER_REQUIRED"
	codeMemberInactive     = "MEMBER_INACTIVE"
	codePointsInsufficient = "POINTS_INSUFFICIENT"
	codeRedeemNotAllowed   = "REDEEM_NOT_ALLOWED"
	codeRedeemTooHigh      = "REDEEM_TOO_HIGH"
	codeRedeemBelowCost    = "REDEEM_BELOW_COST"
)

const maxRedeem = 10_000_000

var (
	ErrOutletInactive = errors.New("outlet tidak aktif")
	ErrKeyRequired    = errors.New("Idempotency-Key wajib diisi")
	ErrKeyMismatch    = errors.New("Idempotency-Key sudah dipakai untuk permintaan yang berbeda")
	ErrNotFound       = errors.New("nota tidak ditemukan")
	validMethods      = map[string]bool{"cash": true, "debit": true, "credit_card": true, "ewallet": true, "transfer": true}
	idemKeyPattern    = regexp.MustCompile(`^[A-Za-z0-9_.:-]{8,100}$`)
)

// FieldErrors = kode galat per field, mis. {"lines.0.qty": "INVALID"}.
type FieldErrors map[string]string

func (f FieldErrors) Error() string { return "input tidak valid" }

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
	Method string      `json:"method"`
	Amount json.Number `json:"amount"` // yang DITERIMA; tunai boleh melebihi sisa (selisihnya kembalian)
	RefNo  string      `json:"ref_no"`
}

// ApprovalIn = penyetuju (Owner/Supervisor) beserta PIN-nya untuk ubah harga.
type ApprovalIn struct {
	UserID uuid.UUID `json:"user_id"`
	PIN    string    `json:"pin"`
}

type Request struct {
	Approval  *ApprovalIn `json:"approval"`
	Lines     []LineIn    `json:"lines"`
	Discount  json.Number `json:"discount"`   // potongan global
	OtherCost json.Number `json:"other_cost"` // biaya lain-lain
	ApplyTax  bool        `json:"apply_tax"`  // terapkan pajak toko & negara sesuai tarif outlet
	Payments  []PaymentIn `json:"payments"`
	Note      string      `json:"note"`
	// Member (opsional): nota diperhitungkan poinnya; RedeemPoints = poin yang ditukar jadi potongan nota (level member menentukan nilainya).
	MemberID     *uuid.UUID `json:"member_id"`
	RedeemPoints int        `json:"redeem_points"`
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
	Method string `json:"method"`
	Amount string `json:"amount"`
	RefNo  string `json:"ref_no"`
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
	// Member & poin nota (Discount sudah memuat RedeemAmount).
	Member         *MemberInfo `json:"member,omitempty"`
	PointsEarned   int         `json:"points_earned"`
	PointsRedeemed int         `json:"points_redeemed"`
	RedeemAmount   string      `json:"redeem_amount"`
}

// MemberInfo = identitas member pada nota/quote.
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

type normPayment struct {
	method string
	amount dec
	refNo  string
}

type norm struct {
	lines     []normLine
	discount  dec
	otherCost dec
	applyTax  bool
	payments  []normPayment
	note      string
	memberID  *uuid.UUID
	redeem    int
}

func normalize(in Request) (norm, FieldErrors) {
	f := FieldErrors{}
	n := norm{applyTax: in.ApplyTax, memberID: in.MemberID, redeem: in.RedeemPoints}
	if in.MemberID != nil && *in.MemberID == uuid.Nil {
		n.memberID = nil
	}
	if in.RedeemPoints < 0 || in.RedeemPoints > maxRedeem {
		f["redeem_points"] = sanitize.Invalid
	} else if in.RedeemPoints > 0 && n.memberID == nil {
		f["redeem_points"] = codeMemberRequired
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
		np := normPayment{method: p.Method}
		if !validMethods[p.Method] {
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
	}{D: n.discount.String(), O: n.otherCost.String(), T: n.applyTax, N: n.note, R: n.redeem}
	if n.memberID != nil {
		v.M = n.memberID.String()
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
		v.P = append(v.P, p{x.method, x.amount.String(), x.refNo})
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
}

type totals struct {
	subtotal, discount, taxStore, taxGov, other, total, paid, change dec
	taxStorePct, taxGovPct                                           dec
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

// settle memeriksa pembayaran terhadap total: non-tunai tidak boleh melebihi total (tidak ada kembalian kartu) dan
// total harus tertutup penuh; kembalian = diterima − total (selalu bisa diambil dari tunai).
func settle(n norm, t totals) (totals, FieldErrors) {
	cash := decimal.Zero
	for _, p := range n.payments {
		t.paid = t.paid.Add(p.amount)
		if p.method == "cash" {
			cash = cash.Add(p.amount)
		}
	}
	switch {
	case t.paid.Sub(cash).GreaterThan(t.total):
		return t, FieldErrors{"payments": codeNonCashOver}
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
		mc, lines, t, fe, err := resolveMember(ctx, tx, a, n, out.LocalDay, true, infos, out.TaxStorePct, out.TaxGovPct, lines, t)
		if err != nil {
			return err
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
			if s.approvals == nil || in.Approval == nil {
				return approval.ErrPinRequired
			}
			if approver, err = s.approvals.Verify(ctx, tx, a, in.Approval.UserID, in.Approval.PIN); err != nil {
				return err
			}
		}
		if t, fe = settle(n, t); len(fe) > 0 {
			return fe
		}

		no, err := q.SalesNextNo(ctx, gen.SalesNextNoParams{TenantID: a.TenantID, OutletID: a.OutletID, Day: out.LocalDay})
		if err != nil {
			return err
		}
		docNo := fmt.Sprintf("%s-%s-%04d", strings.ToUpper(out.Code), out.LocalDay.Time.Format("060102"), no)

		hdr, err := q.SalesInsert(ctx, gen.SalesInsertParams{
			TenantID: a.TenantID, OutletID: a.OutletID, DocNo: docNo, IdempotencyKey: key, RequestHash: h,
			CashierID: pgtype.UUID{Bytes: a.UserID, Valid: a.UserID != uuid.Nil}, ApprovedBy: pgtype.UUID{Bytes: approver.ID, Valid: approver.ID != uuid.Nil}, Note: n.note,
			Subtotal: t.subtotal, Discount: t.discount, TaxStorePct: t.taxStorePct, TaxGovPct: t.taxGovPct,
			TaxStore: t.taxStore, TaxGov: t.taxGov, OtherCost: t.other, Total: t.total, Paid: t.paid, Change: t.change,
			MemberID: pgtype.UUID{Bytes: mc.id(), Valid: mc.sm != nil}, PointsEarned: int32(mc.earn), PointsRedeemed: int32(n.redeemApplied(mc)), RedeemAmount: mc.redeemAmt,
		})
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
		for i, p := range n.payments {
			if err := q.SalesPaymentInsert(ctx, gen.SalesPaymentInsertParams{TenantID: a.TenantID, SaleID: hdr.ID, Position: int32(i + 1),
				Method: p.method, Amount: p.amount, RefNo: p.refNo}); err != nil {
				return err
			}
		}
		if mc.sm != nil {
			if err := member.ApplySale(ctx, tx, a, mc.sm.ID, hdr.ID, docNo, n.redeemApplied(mc), mc.earn); err != nil {
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
		return audit.Record(ctx, tx, audit.FromActor(a), audit.Entry{
			Action: audit.ActionSaleCreate, Entity: audit.EntitySale, EntityID: hdr.ID.String(),
			Details: map[string]any{"doc_no": docNo, "outlet_id": a.OutletID.String(), "lines": len(lines),
				"total": t.total.String(), "paid": t.paid.String(), "discount": t.discount.String(),
				"member": mc.code(), "points_earned": mc.earn, "points_redeemed": n.redeemApplied(mc)},
		})
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
		ls, err := q.SalesLines(ctx, gen.SalesLinesParams{TenantID: a.TenantID, SaleID: id})
		if err != nil {
			return err
		}
		ps, err := q.SalesPayments(ctx, gen.SalesPaymentsParams{TenantID: a.TenantID, SaleID: id})
		if err != nil {
			return err
		}
		var mi *MemberInfo
		if h.MemberID.Valid {
			mi = &MemberInfo{ID: uuid.UUID(h.MemberID.Bytes), Code: h.MemberCode, Name: h.MemberName}
		}
		out = Sale{ID: h.ID, DocNo: h.DocNo, Status: h.Status, OutletID: h.OutletID, CashierName: h.CashierName, CreatedAt: h.CreatedAt.Time,
			ApprovedBy: h.ApproverName, Note: h.Note, Subtotal: h.Subtotal.StringFixed(2), Discount: h.Discount.StringFixed(2),
			TaxStorePct: h.TaxStorePct.StringFixed(2), TaxGovPct: h.TaxGovPct.StringFixed(2),
			TaxStore: h.TaxStore.StringFixed(2), TaxGov: h.TaxGov.StringFixed(2), OtherCost: h.OtherCost.StringFixed(2),
			Total: h.Total.StringFixed(2), Paid: h.Paid.StringFixed(2), Change: h.Change.StringFixed(2),
			Member: mi, PointsEarned: int(h.PointsEarned), PointsRedeemed: int(h.PointsRedeemed), RedeemAmount: h.RedeemAmount.StringFixed(2),
			Lines: make([]Line, 0, len(ls)), Payments: make([]Payment, 0, len(ps))}
		for _, l := range ls {
			out.Lines = append(out.Lines, Line{ItemID: l.ItemID, SKU: l.Sku, Name: l.Name, UnitID: l.UnitID, Unit: l.UnitName,
				Factor: l.Factor.String(), Qty: l.Qty.String(), UnitPrice: l.UnitPrice.StringFixed(2), Discount: l.Discount.StringFixed(2),
				LineTotal: l.LineTotal.StringFixed(2), Note: l.Note, ListPrice: l.ListPrice.StringFixed(2), PriceOverride: l.PriceOverride})
		}
		for _, p := range ps {
			out.Payments = append(out.Payments, Payment{Method: p.Method, Amount: p.Amount.StringFixed(2), RefNo: p.RefNo})
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
}

func (s *Service) Quote(ctx context.Context, a authz.Actor, in Request) (Quote, error) {
	in.Payments = nil
	n, f := normalize(in)
	if len(f) > 0 {
		return Quote{}, f
	}
	var out Quote
	err := db.WithTenant(ctx, s.pool, a.TenantID, func(tx pgx.Tx) error {
		q := gen.New(tx)
		o, err := q.SalesOutletInfo(ctx, gen.SalesOutletInfoParams{TenantID: a.TenantID, ID: a.OutletID})
		if errors.Is(err, pgx.ErrNoRows) || (err == nil && !o.Active) {
			return ErrOutletInactive
		}
		if err != nil {
			return err
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
			return err
		}
		lines, t, fe := price(n, infos, o.TaxStorePct, o.TaxGovPct)
		if len(fe) > 0 {
			return fe
		}
		mc, lines, t, fe, err := resolveMember(ctx, tx, a, n, o.LocalDay, false, infos, o.TaxStorePct, o.TaxGovPct, lines, t)
		if err != nil {
			return err
		}
		if len(fe) > 0 {
			return fe
		}
		out = Quote{RedeemAmount: mc.redeemAmt.StringFixed(2), PointsEarn: mc.earn, Subtotal: t.subtotal.StringFixed(2), Discount: t.discount.StringFixed(2), TaxStorePct: t.taxStorePct.StringFixed(2),
			TaxGovPct: t.taxGovPct.StringFixed(2), TaxStore: t.taxStore.StringFixed(2), TaxGov: t.taxGov.StringFixed(2),
			OtherCost: t.other.StringFixed(2), Total: t.total.StringFixed(2), Lines: make([]Line, 0, len(lines))}
		if mc.sm != nil {
			out.Member = &MemberInfo{ID: mc.sm.ID, Code: mc.sm.Code, Name: mc.sm.Name, Points: mc.sm.Points}
		}
		for _, l := range lines {
			out.Lines = append(out.Lines, Line{ItemID: l.item.row.ID, SKU: l.item.row.Sku, Name: l.item.row.Name, UnitID: l.unitID, Unit: l.unitName,
				Factor: l.factor.String(), Qty: l.qty.String(), UnitPrice: l.unitPrice.StringFixed(2), Discount: l.discount.StringFixed(2),
				LineTotal: l.total.StringFixed(2), Note: l.note, Issue: l.issue, Available: l.available, ListPrice: l.listPrice.StringFixed(2), PriceOverride: l.overridden})
		}
		return nil
	})
	return out, err
}

// overriddenLines = baris yang harganya diubah dari harga hasil hitung server.
func overriddenLines(lines []calcLine) []calcLine {
	var out []calcLine
	for _, l := range lines {
		if l.overridden {
			out = append(out, l)
		}
	}
	return out
}

// discountedLines = baris yang diberi potongan manual (butuh persetujuan PIN seperti ubah harga).
func discountedLines(lines []calcLine) []calcLine {
	var out []calcLine
	for _, l := range lines {
		if l.discount.IsPositive() {
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
		costFloor := decimal.Zero
		for _, l := range lines {
			if !l.item.row.SellBelowCost {
				costFloor = costFloor.Add(l.unitCost.Mul(l.qty).Round(2))
			}
		}
		if t.subtotal.Sub(t.discount).LessThan(costFloor) {
			return mc, lines, t, FieldErrors{"redeem_points": codeRedeemBelowCost}, nil
		}
		mc.redeemAmt = amt
	}
	mc.earn = member.PointsFor(t.subtotal.Sub(t.discount), sm.SpendPerPoint)
	return mc, lines, t, nil, nil
}
