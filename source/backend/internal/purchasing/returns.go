package purchasing

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/shopspring/decimal"

	"aciraba/internal/audit"
	"aciraba/internal/authz"
	gen "aciraba/internal/gen"
	"aciraba/internal/payable"
	"aciraba/internal/platform/sanitize"
	"aciraba/internal/stock"
)

// Retur pembelian (Fase 6.6, keputusan pengguna 2026-10-09):
//   - merujuk satu nota pembelian 'completed' di outlet aktif; qty retur per baris ≤ dibeli − sudah diretur (retur aktif);
//   - barang keluar HANYA dari bucket Retur (tanpa jalan pintas dari Display/Gudang: mutasi dulu ke Retur);
//   - nilai = nilai baris nota setelah diskon × qty retur ÷ qty beli (retur yang menghabiskan baris mengambil sisa nilainya)
//     + PPN dengan tarif nota; biaya lain nota tidak dikembalikan;
//   - nilai memotong sisa hutang nota dulu; kelebihannya = dana dikembalikan pemasok (wajib metode bayar);
//   - HPP rata-rata cabang dihitung mundur dengan HPP baris nota (rumus sama dengan batal nota);
//   - batal retur: barang kembali ke bucket Retur, potongan hutang hilang; ditolak bila hutang nota sudah dibayar
//     sesudah retur dibuat. Nota yang punya retur aktif tidak boleh diedit/dibatalkan (lihat reverse).

const ModuleReturns = "purchase_returns"

type ReturnLineIn struct {
	Position int         `json:"position"` // posisi baris nota asal
	Qty      json.Number `json:"qty"`      // satuan dasar
}

type ReturnRequest struct {
	PurchaseID     uuid.UUID      `json:"purchase_id"`
	Lines          []ReturnLineIn `json:"lines"`
	Note           string         `json:"note"`
	RefundMethodID *uuid.UUID     `json:"refund_method_id"` // wajib bila ada dana dikembalikan
	RefundRef      string         `json:"refund_ref"`
}

type retLineNorm struct {
	pos int
	qty dec
}

type retNorm struct {
	purchase uuid.UUID
	lines    []retLineNorm
	note     string
	method   uuid.UUID // Nil = tidak diisi
	ref      string
}

func normalizeReturn(in ReturnRequest, strict bool) (retNorm, FieldErrors) {
	f := FieldErrors{}
	n := retNorm{purchase: in.PurchaseID}
	if in.PurchaseID == uuid.Nil {
		f["purchase_id"] = sanitize.Required
	}
	switch {
	case len(in.Lines) == 0 && strict:
		f["lines"] = sanitize.Required
	case len(in.Lines) > maxLines:
		f["lines"] = codeTooMany
	}
	seen := map[int]bool{}
	for i, l := range in.Lines {
		if i >= maxLines {
			break
		}
		key := "lines." + strconv.Itoa(i)
		if l.Position < 0 {
			f[key+".position"] = sanitize.Invalid
		} else if seen[l.Position] {
			f[key+".position"] = "DUPLICATE"
		}
		seen[l.Position] = true
		q, code := parseDec(l.Qty, 3, true)
		if code == "" && !q.IsPositive() {
			code = sanitize.Invalid
		}
		if code != "" {
			f[key+".qty"] = code
		}
		n.lines = append(n.lines, retLineNorm{pos: l.Position, qty: q})
	}
	note, code := sanitize.Multiline(in.Note, 500)
	if code != "" {
		f["note"] = code
	}
	n.note = note
	ref, ok := sanitize.Text(in.RefundRef)
	if !ok || utf8.RuneCountInString(ref) > 100 {
		f["refund_ref"] = sanitize.Invalid
	}
	n.ref = ref
	if in.RefundMethodID != nil {
		n.method = *in.RefundMethodID
	}
	return n, f
}

func (n retNorm) hash() string {
	parts := []string{n.purchase.String(), n.note, n.method.String(), n.ref}
	for _, l := range n.lines {
		parts = append(parts, strconv.Itoa(l.pos)+":"+l.qty.String())
	}
	raw, _ := json.Marshal(parts)
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

// ---- Sumber (nota asal) ----

type retNota struct {
	id          uuid.UUID
	outletID    uuid.UUID
	outletCode  string
	outletOK    bool
	status      string
	docNo       string
	invoiceNo   string
	date        pgtype.Date
	paymentType string
	supplierID  uuid.UUID
	supplier    string
	taxPct      dec
	taxAmount   dec
	today       pgtype.Date
}

type retSrcLine struct {
	pos         int
	itemID      uuid.UUID
	sku, name   string
	unit        string
	qty         dec
	lineTotal   dec
	unitCost    dec
	retQty      dec
	retValue    dec
	returnStock dec
}

// loadNota membaca nota asal (dikunci FOR UPDATE bila lock — kunci yang sama dengan bayar hutang dan edit/batal nota).
func loadNota(ctx context.Context, tx pgx.Tx, tenant, id uuid.UUID, lock bool) (retNota, error) {
	q := `
		SELECT p.id, p.outlet_id, o.code, o.active, p.status, p.doc_no, p.supplier_invoice_no, p.purchase_date, p.payment_type,
		       p.supplier_id, s.name, p.tax_pct, p.tax_amount, (now() AT TIME ZONE o.timezone)::date
		FROM purchases p
		JOIN outlets o   ON o.tenant_id = p.tenant_id AND o.id = p.outlet_id
		JOIN suppliers s ON s.tenant_id = p.tenant_id AND s.id = p.supplier_id
		WHERE p.tenant_id = $1 AND p.id = $2`
	if lock {
		q += ` FOR UPDATE OF p`
	}
	var n retNota
	err := tx.QueryRow(ctx, q, tenant, id).Scan(&n.id, &n.outletID, &n.outletCode, &n.outletOK, &n.status, &n.docNo, &n.invoiceNo, &n.date,
		&n.paymentType, &n.supplierID, &n.supplier, &n.taxPct, &n.taxAmount, &n.today)
	if errors.Is(err, pgx.ErrNoRows) {
		return n, ErrNotFound
	}
	return n, err
}

// loadSrcLines = baris nota + jumlah yang sudah diretur (retur aktif) + stok bucket Retur di outlet nota.
func loadSrcLines(ctx context.Context, tx pgx.Tx, tenant uuid.UUID, nota retNota) ([]retSrcLine, error) {
	rows, err := tx.Query(ctx, `
		SELECT l.position, l.item_id, l.item_sku, l.item_name, l.unit_name, l.qty, l.line_total, l.unit_cost,
		       coalesce(rr.qty, 0), coalesce(rr.value, 0), coalesce(sb.qty, 0)
		FROM purchase_lines l
		LEFT JOIN LATERAL (
		  SELECT sum(rl.qty) AS qty, sum(rl.value) AS value
		  FROM purchase_return_lines rl JOIN purchase_returns r ON r.tenant_id = rl.tenant_id AND r.id = rl.return_id
		  WHERE rl.tenant_id = l.tenant_id AND r.purchase_id = l.purchase_id AND rl.purchase_position = l.position AND r.status = 'completed'
		) rr ON true
		LEFT JOIN stock_balances sb ON sb.tenant_id = l.tenant_id AND sb.outlet_id = $3 AND sb.item_id = l.item_id AND sb.bucket = 'returns'
		WHERE l.tenant_id = $1 AND l.purchase_id = $2
		ORDER BY l.position`, tenant, nota.id, nota.outletID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []retSrcLine
	for rows.Next() {
		var l retSrcLine
		var pos int32
		if err := rows.Scan(&pos, &l.itemID, &l.sku, &l.name, &l.unit, &l.qty, &l.lineTotal, &l.unitCost, &l.retQty, &l.retValue, &l.returnStock); err != nil {
			return nil, err
		}
		l.pos = int(pos)
		out = append(out, l)
	}
	return out, rows.Err()
}

func returnedTax(ctx context.Context, tx pgx.Tx, tenant, purchase uuid.UUID) (dec, error) {
	var t dec
	err := tx.QueryRow(ctx, `SELECT coalesce(sum(tax_amount), 0) FROM purchase_returns WHERE tenant_id = $1 AND purchase_id = $2 AND status = 'completed'`,
		tenant, purchase).Scan(&t)
	return t, err
}

func hasActiveReturns(ctx context.Context, tx pgx.Tx, tenant, purchase uuid.UUID) (bool, error) {
	var n int
	err := tx.QueryRow(ctx, `SELECT count(*) FROM purchase_returns WHERE tenant_id = $1 AND purchase_id = $2 AND status = 'completed'`, tenant, purchase).Scan(&n)
	return n > 0, err
}

// ReturnSource = nota asal beserta sisa yang masih bisa diretur per baris (untuk form retur).
type ReturnSource struct {
	PurchaseID        uuid.UUID          `json:"purchase_id"`
	DocNo             string             `json:"doc_no"`
	SupplierID        uuid.UUID          `json:"supplier_id"`
	SupplierName      string             `json:"supplier_name"`
	SupplierInvoiceNo string             `json:"supplier_invoice_no"`
	PurchaseDate      string             `json:"purchase_date"`
	PaymentType       string             `json:"payment_type"`
	TaxPct            string             `json:"tax_pct"`
	PayableBalance    *string            `json:"payable_balance"` // nil = nota tanpa hutang (tunai)
	Lines             []ReturnSourceLine `json:"lines"`
}

type ReturnSourceLine struct {
	Position    int       `json:"position"`
	ItemID      uuid.UUID `json:"item_id"`
	SKU         string    `json:"sku"`
	Name        string    `json:"name"`
	Unit        string    `json:"unit"`
	Qty         string    `json:"qty"`          // dibeli
	Returned    string    `json:"returned"`     // sudah diretur (retur aktif)
	Returnable  string    `json:"returnable"`   // sisa yang boleh diretur
	NetPrice    string    `json:"net_price"`    // nilai baris ÷ qty (setelah diskon, tanpa PPN)
	UnitCost    string    `json:"unit_cost"`    // HPP baris
	ReturnStock string    `json:"return_stock"` // stok bucket Retur di outlet nota
}

// checkNotaForReturn: nota harus completed dan berada di outlet aktif (stok diambil dari bucket Retur outlet itu).
func checkNotaForReturn(a authz.Actor, n retNota) error {
	if !canAccessOutlet(a, n.outletID) {
		return ErrOutletForbidden
	}
	if n.outletID != a.OutletID {
		return ErrOutletMismatch
	}
	if n.status != "completed" {
		return ErrNotReturnable
	}
	return nil
}

func (s *Service) ReturnSource(ctx context.Context, a authz.Actor, purchaseID uuid.UUID) (ReturnSource, error) {
	var out ReturnSource
	err := s.tx(ctx, a, func(tx pgx.Tx) error {
		nota, err := loadNota(ctx, tx, a.TenantID, purchaseID, false)
		if err != nil {
			return err
		}
		if err := checkNotaForReturn(a, nota); err != nil {
			return err
		}
		lines, err := loadSrcLines(ctx, tx, a.TenantID, nota)
		if err != nil {
			return err
		}
		out = ReturnSource{PurchaseID: nota.id, DocNo: nota.docNo, SupplierID: nota.supplierID, SupplierName: nota.supplier,
			SupplierInvoiceNo: nota.invoiceNo, PurchaseDate: nota.date.Time.Format("2006-01-02"), PaymentType: nota.paymentType,
			TaxPct: nota.taxPct.StringFixed(2), Lines: []ReturnSourceLine{}}
		if st, ok, err := payable.StateOf(ctx, tx, a.TenantID, nota.id); err != nil {
			return err
		} else if ok {
			b := st.Balance().StringFixed(2)
			out.PayableBalance = &b
		}
		for _, l := range lines {
			net := decimal.Zero
			if l.qty.IsPositive() {
				net = l.lineTotal.Div(l.qty).Round(4)
			}
			out.Lines = append(out.Lines, ReturnSourceLine{Position: l.pos, ItemID: l.itemID, SKU: l.sku, Name: l.name, Unit: l.unit,
				Qty: l.qty.String(), Returned: l.retQty.String(), Returnable: l.qty.Sub(l.retQty).String(), NetPrice: priceString(net),
				UnitCost: l.unitCost.StringFixed(2), ReturnStock: l.returnStock.String()})
		}
		return nil
	})
	return out, err
}

// ---- Hitung ----

type ReturnQuoteLine struct {
	Position    int       `json:"position"`
	ItemID      uuid.UUID `json:"item_id"`
	SKU         string    `json:"sku"`
	Name        string    `json:"name"`
	Unit        string    `json:"unit"`
	Qty         string    `json:"qty"`
	Value       string    `json:"value"`
	UnitCost    string    `json:"unit_cost"`
	ReturnStock string    `json:"return_stock"`
	Issue       string    `json:"issue,omitempty"` // STOCK_INSUFFICIENT = stok bucket Retur kurang (total per barang)
}

type ReturnQuote struct {
	Lines          []ReturnQuoteLine `json:"lines"`
	Subtotal       string            `json:"subtotal"`
	TaxAmount      string            `json:"tax_amount"`
	Total          string            `json:"total"`
	PayableBalance *string           `json:"payable_balance"` // sisa hutang nota SEBELUM retur ini; nil = tanpa hutang
	PayableCut     string            `json:"payable_cut"`
	Refund         string            `json:"refund"`
}

type retCalcLine struct {
	src   retSrcLine
	qty   dec
	value dec
}

type retCalc struct {
	lines              []retCalcLine
	subtotal, tax      dec
	total, cut, refund dec
	payable            *payable.State
}

// calcReturn menerapkan aturan qty & nilai. Dipakai pratinjau dan simpan (setelah nota dikunci) sehingga hasilnya sama.
func calcReturn(ctx context.Context, tx pgx.Tx, a authz.Actor, nota retNota, n retNorm) (retCalc, FieldErrors, error) {
	src, err := loadSrcLines(ctx, tx, a.TenantID, nota)
	if err != nil {
		return retCalc{}, nil, err
	}
	byPos := map[int]retSrcLine{}
	for _, l := range src {
		byPos[l.pos] = l
	}
	f := FieldErrors{}
	var c retCalc
	full := map[int]bool{}
	for i, in := range n.lines {
		key := "lines." + strconv.Itoa(i)
		l, ok := byPos[in.pos]
		if !ok {
			f[key+".position"] = sanitize.Invalid
			continue
		}
		if in.qty.IsZero() {
			continue // galat qty sudah dicatat normalisasi
		}
		remaining := l.qty.Sub(l.retQty)
		if in.qty.GreaterThan(remaining) {
			f[key+".qty"] = codeTooHigh
			continue
		}
		left := l.lineTotal.Sub(l.retValue)
		var v dec
		if in.qty.Equal(remaining) {
			v = left // retur yang menghabiskan baris mengambil sisa nilainya (Σ retur = nilai baris tepat)
			full[l.pos] = true
		} else {
			v = l.lineTotal.Mul(in.qty).Div(l.qty).Round(2)
			if v.GreaterThan(left) {
				v = left
			}
		}
		if v.IsNegative() {
			v = decimal.Zero
		}
		c.lines = append(c.lines, retCalcLine{src: l, qty: in.qty, value: v})
		c.subtotal = c.subtotal.Add(v)
	}
	if len(f) > 0 {
		return retCalc{}, f, nil
	}
	taxDone, err := returnedTax(ctx, tx, a.TenantID, nota.id)
	if err != nil {
		return retCalc{}, nil, err
	}
	taxLeft := nota.taxAmount.Sub(taxDone)
	if taxLeft.IsNegative() {
		taxLeft = decimal.Zero
	}
	allDone := len(src) > 0
	for _, l := range src {
		if !full[l.pos] && l.qty.GreaterThan(l.retQty) {
			allDone = false
		}
	}
	if allDone {
		c.tax = taxLeft // nota habis diretur: PPN yang dikembalikan = sisa PPN nota tepat
	} else {
		c.tax = c.subtotal.Mul(nota.taxPct).Div(decimal.NewFromInt(100)).Round(2)
		if c.tax.GreaterThan(taxLeft) {
			c.tax = taxLeft
		}
	}
	c.total = c.subtotal.Add(c.tax)
	st, ok, err := payable.StateOf(ctx, tx, a.TenantID, nota.id)
	if err != nil {
		return retCalc{}, nil, err
	}
	if ok {
		c.payable = &st
		bal := st.Balance()
		if bal.IsPositive() {
			c.cut = decimal.Min(bal, c.total)
		}
	}
	c.refund = c.total.Sub(c.cut)
	return c, nil, nil
}

func (c retCalc) quote() ReturnQuote {
	q := ReturnQuote{Lines: []ReturnQuoteLine{}, Subtotal: c.subtotal.StringFixed(2), TaxAmount: c.tax.StringFixed(2),
		Total: c.total.StringFixed(2), PayableCut: c.cut.StringFixed(2), Refund: c.refund.StringFixed(2)}
	if c.payable != nil {
		b := c.payable.Balance().StringFixed(2)
		q.PayableBalance = &b
	}
	need := map[uuid.UUID]dec{}
	for _, l := range c.lines {
		need[l.src.itemID] = need[l.src.itemID].Add(l.qty)
	}
	for _, l := range c.lines {
		ql := ReturnQuoteLine{Position: l.src.pos, ItemID: l.src.itemID, SKU: l.src.sku, Name: l.src.name, Unit: l.src.unit,
			Qty: l.qty.String(), Value: l.value.StringFixed(2), UnitCost: l.src.unitCost.StringFixed(2), ReturnStock: l.src.returnStock.String()}
		if need[l.src.itemID].GreaterThan(l.src.returnStock) {
			ql.Issue = "STOCK_INSUFFICIENT"
		}
		q.Lines = append(q.Lines, ql)
	}
	return q
}

// QuoteReturn = pratinjau nilai retur; tidak menulis apa pun. Baris boleh belum lengkap (qty kosong dilewati).
func (s *Service) QuoteReturn(ctx context.Context, a authz.Actor, in ReturnRequest) (ReturnQuote, error) {
	n, f := normalizeReturn(in, false)
	if len(f) > 0 {
		return ReturnQuote{}, f
	}
	var out ReturnQuote
	err := s.tx(ctx, a, func(tx pgx.Tx) error {
		nota, err := loadNota(ctx, tx, a.TenantID, n.purchase, false)
		if err != nil {
			return err
		}
		if err := checkNotaForReturn(a, nota); err != nil {
			return err
		}
		c, fe, err := calcReturn(ctx, tx, a, nota, n)
		if err != nil {
			return err
		}
		if len(fe) > 0 {
			return fe
		}
		out = c.quote()
		return nil
	})
	return out, err
}

// ---- Simpan ----

type returnReplay struct{ id uuid.UUID }

func (returnReplay) Error() string { return "replay" }

func checkReturnKey(ctx context.Context, tx pgx.Tx, tenant uuid.UUID, key, h string) error {
	var id uuid.UUID
	var hash string
	err := tx.QueryRow(ctx, `SELECT id, request_hash FROM purchase_returns WHERE tenant_id = $1 AND idempotency_key = $2`, tenant, key).Scan(&id, &hash)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	if hash != h {
		return ErrKeyMismatch
	}
	return returnReplay{id}
}

type retItemAgg struct {
	qty, value dec // value = Σ qty × HPP baris
}

func aggregateByItem(lines []retCalcLine) (map[uuid.UUID]*retItemAgg, []uuid.UUID) {
	m := map[uuid.UUID]*retItemAgg{}
	var ids []uuid.UUID
	for _, l := range lines {
		g := m[l.src.itemID]
		if g == nil {
			g = &retItemAgg{}
			m[l.src.itemID] = g
			ids = append(ids, l.src.itemID)
		}
		g.qty = g.qty.Add(l.qty)
		g.value = g.value.Add(l.qty.Mul(l.src.unitCost))
	}
	sort.Slice(ids, func(i, j int) bool { return bytes.Compare(ids[i][:], ids[j][:]) < 0 })
	return m, ids
}

// moveReturns menggerakkan stok bucket Retur (keluar bila out, kembali bila !out) lalu menghitung ulang HPP rata-rata
// cabang. Barang dikunci terurut id (FOR NO KEY UPDATE, lihat PurchaseItemLock) dan stok dibaca SETELAH ApplyAll
// (baris saldo sudah terkunci, sehingga penjualan bersamaan tak membuat HPP dihitung dari stok basi).
func moveReturns(ctx context.Context, tx pgx.Tx, a authz.Actor, outlet uuid.UUID, retID uuid.UUID, note string, lines []retCalcLine, out bool) error {
	q := gen.New(tx)
	agg, ids := aggregateByItem(lines)
	type st struct{ avg, last dec }
	states := map[uuid.UUID]st{}
	var moves []stock.Movement
	for _, id := range ids {
		if _, err := q.PurchaseItemLock(ctx, gen.PurchaseItemLockParams{TenantID: a.TenantID, OutletID: outlet, ID: id}); err != nil {
			return err
		}
		cur, err := q.PurchaseItemState(ctx, gen.PurchaseItemStateParams{TenantID: a.TenantID, OutletID: outlet, ID: id})
		if err != nil {
			return err
		}
		states[id] = st{avg: cur.AvgCost, last: cur.LastCost}
		delta := agg[id].qty
		if out {
			delta = delta.Neg()
		}
		moves = append(moves, stock.Movement{TenantID: a.TenantID, OutletID: outlet, ItemID: id, Bucket: stock.BucketReturns,
			Delta: delta, RefType: stock.RefPurchaseReturn, RefID: retID, Note: note, ActorID: a.UserID})
	}
	if _, err := stock.ApplyAll(ctx, tx, moves); err != nil {
		return err
	}
	for _, id := range ids {
		now, err := q.StockOutletQty(ctx, gen.StockOutletQtyParams{TenantID: a.TenantID, OutletID: outlet, ItemID: id})
		if err != nil {
			return err
		}
		g, cur := agg[id], states[id]
		avg := cur.avg
		if out {
			// Stok sebelum retur = stok sekarang + qty yang keluar.
			if next, ok := reverseAvg(now.Add(g.qty), cur.avg, g.qty, g.value); ok {
				avg = next
			}
		} else {
			before := now.Sub(g.qty)
			unit := decimal.Zero
			if g.qty.IsPositive() {
				unit = g.value.Div(g.qty)
			}
			if !before.IsPositive() || !cur.avg.IsPositive() {
				avg = unit.Round(2)
			} else {
				avg = before.Mul(cur.avg).Add(g.value).Div(before.Add(g.qty)).Round(2)
			}
		}
		if err := q.StockSetCost(ctx, gen.StockSetCostParams{TenantID: a.TenantID, OutletID: outlet, ItemID: id, AvgCost: avg, LastCost: cur.last}); err != nil {
			return err
		}
	}
	return nil
}

// CreateReturn mencatat retur pembelian. replayed=true bila Idempotency-Key yang sama sudah mencatat retur ini.
func (s *Service) CreateReturn(ctx context.Context, a authz.Actor, key string, in ReturnRequest) (r PurchaseReturn, replayed bool, err error) {
	if !idemKeyPattern.MatchString(key) {
		return PurchaseReturn{}, false, ErrKeyRequired
	}
	n, f := normalizeReturn(in, true)
	if len(f) > 0 {
		return PurchaseReturn{}, false, f
	}
	h := n.hash()
	var newID uuid.UUID
	err = s.tx(ctx, a, func(tx pgx.Tx) error {
		if e := checkReturnKey(ctx, tx, a.TenantID, key, h); e != nil {
			return e
		}
		nota, err := loadNota(ctx, tx, a.TenantID, n.purchase, true)
		if err != nil {
			return err
		}
		// Kiriman ganda bersamaan: yang kedua menunggu kunci nota, jadi kuncinya baru terlihat sekarang.
		if e := checkReturnKey(ctx, tx, a.TenantID, key, h); e != nil {
			return e
		}
		if err := checkNotaForReturn(a, nota); err != nil {
			return err
		}
		if !nota.outletOK {
			return ErrOutletInactive
		}
		c, fe, err := calcReturn(ctx, tx, a, nota, n)
		if err != nil {
			return err
		}
		if len(fe) > 0 {
			return fe
		}
		var (
			mID          pgtype.UUID
			mKind, mName pgtype.Text
			refund, mRef = c.refund, ""
		)
		if refund.IsPositive() {
			if n.method == uuid.Nil {
				return FieldErrors{"refund_method_id": sanitize.Required}
			}
			var kind, name string
			var active bool
			e := tx.QueryRow(ctx, `SELECT kind, name, active FROM payment_methods WHERE tenant_id = $1 AND id = $2`, a.TenantID, n.method).Scan(&kind, &name, &active)
			switch {
			case errors.Is(e, pgx.ErrNoRows):
				return FieldErrors{"refund_method_id": sanitize.Invalid}
			case e != nil:
				return e
			case !active:
				return FieldErrors{"refund_method_id": "METHOD_INACTIVE"}
			}
			mID = pgtype.UUID{Bytes: n.method, Valid: true}
			mKind = pgtype.Text{String: kind, Valid: true}
			mName = pgtype.Text{String: name, Valid: true}
			mRef = n.ref
		}

		var no int64
		if err := tx.QueryRow(ctx, `
			INSERT INTO purchase_return_counters (tenant_id, outlet_id, day, last_no) VALUES ($1, $2, $3, 1)
			ON CONFLICT (tenant_id, outlet_id, day) DO UPDATE SET last_no = purchase_return_counters.last_no + 1
			RETURNING last_no`, a.TenantID, nota.outletID, nota.today).Scan(&no); err != nil {
			return err
		}
		docNo := fmt.Sprintf("RB-%s-%s-%04d", strings.ToUpper(nota.outletCode), nota.today.Time.Format("060102"), no)
		newID = uuid.New()
		if _, err := tx.Exec(ctx, `
			INSERT INTO purchase_returns (id, tenant_id, outlet_id, purchase_id, supplier_id, doc_no, idempotency_key, request_hash, return_date,
			  note, subtotal, tax_amount, total, payable_cut, refund, refund_method, refund_method_id, refund_method_name, refund_ref, created_by)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, coalesce($18, ''), $19, $20)`,
			newID, a.TenantID, nota.outletID, nota.id, nota.supplierID, docNo, key, h, nota.today,
			n.note, c.subtotal, c.tax, c.total, c.cut, c.refund, mKind, mID, mName, mRef, nullUUID(a.UserID)); err != nil {
			return err
		}
		for i, l := range c.lines {
			if _, err := tx.Exec(ctx, `
				INSERT INTO purchase_return_lines (tenant_id, return_id, position, purchase_position, item_id, item_sku, item_name, unit_name, qty, value, unit_cost)
				VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)`,
				a.TenantID, newID, i, l.src.pos, l.src.itemID, l.src.sku, l.src.name, l.src.unit, l.qty, l.value, l.src.unitCost); err != nil {
				return err
			}
		}
		if err := moveReturns(ctx, tx, a, nota.outletID, newID, docNo, c.lines, true); err != nil {
			return err
		}
		return audit.Record(ctx, tx, audit.FromActor(a), audit.Entry{
			Action: audit.ActionPurchaseReturnCreate, Entity: audit.EntityPurchaseReturn, EntityID: newID.String(),
			Details: map[string]any{"doc_no": docNo, "purchase_id": nota.id.String(), "purchase_doc_no": nota.docNo,
				"total": c.total.String(), "payable_cut": c.cut.String(), "refund": c.refund.String(), "lines": len(c.lines)},
		})
	})
	var rp returnReplay
	if errors.As(err, &rp) {
		r, err = s.GetReturn(ctx, a, rp.id)
		return r, true, err
	}
	if err != nil {
		return PurchaseReturn{}, false, err
	}
	r, err = s.GetReturn(ctx, a, newID)
	return r, false, err
}

// VoidReturn membatalkan retur: barang kembali ke bucket Retur outlet retur, HPP dihitung maju dengan HPP baris,
// potongan hutang hilang (saldo hutang dihitung dari retur berstatus completed saja).
func (s *Service) VoidReturn(ctx context.Context, a authz.Actor, id uuid.UUID, reason string) (PurchaseReturn, error) {
	reason, f := checkReason(reason)
	if len(f) > 0 {
		return PurchaseReturn{}, f
	}
	err := s.tx(ctx, a, func(tx pgx.Tx) error {
		var purchaseID uuid.UUID
		err := tx.QueryRow(ctx, `SELECT purchase_id FROM purchase_returns WHERE tenant_id = $1 AND id = $2`, a.TenantID, id).Scan(&purchaseID)
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNotFound
		}
		if err != nil {
			return err
		}
		// Urutan kunci sama dengan CreateReturn/Pay: nota dulu, baru dokumen retur.
		if _, err := loadNota(ctx, tx, a.TenantID, purchaseID, true); err != nil {
			return err
		}
		var (
			outlet    uuid.UUID
			status    string
			docNo     string
			createdAt time.Time
		)
		if err := tx.QueryRow(ctx, `SELECT outlet_id, status, doc_no, created_at FROM purchase_returns WHERE tenant_id = $1 AND id = $2 FOR UPDATE`,
			a.TenantID, id).Scan(&outlet, &status, &docNo, &createdAt); err != nil {
			return err
		}
		if !canAccessOutlet(a, outlet) {
			return ErrOutletForbidden
		}
		if status != "completed" {
			return ErrReturnNotActive
		}
		if paid, err := payable.HasPaymentsSince(ctx, tx, a.TenantID, purchaseID, createdAt); err != nil {
			return err
		} else if paid {
			return ErrReturnLocked
		}
		rows, err := tx.Query(ctx, `SELECT item_id, qty, unit_cost FROM purchase_return_lines WHERE tenant_id = $1 AND return_id = $2 ORDER BY position`, a.TenantID, id)
		if err != nil {
			return err
		}
		var lines []retCalcLine
		for rows.Next() {
			var l retCalcLine
			if err := rows.Scan(&l.src.itemID, &l.qty, &l.src.unitCost); err != nil {
				rows.Close()
				return err
			}
			lines = append(lines, l)
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return err
		}
		if err := moveReturns(ctx, tx, a, outlet, id, docNo+" (batal)", lines, false); err != nil {
			return err
		}
		tag, err := tx.Exec(ctx, `UPDATE purchase_returns SET status = 'void', void_reason = $3, voided_at = now(), voided_by = $4
			WHERE tenant_id = $1 AND id = $2 AND status = 'completed'`, a.TenantID, id, reason, nullUUID(a.UserID))
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return ErrReturnNotActive
		}
		return audit.Record(ctx, tx, audit.FromActor(a), audit.Entry{
			Action: audit.ActionPurchaseReturnVoid, Entity: audit.EntityPurchaseReturn, EntityID: id.String(),
			Details: map[string]any{"doc_no": docNo, "purchase_id": purchaseID.String(), "reason": reason},
		})
	})
	if err != nil {
		return PurchaseReturn{}, err
	}
	return s.GetReturn(ctx, a, id)
}
