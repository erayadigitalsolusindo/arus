package sales

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
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
	"aciraba/internal/member"
	"aciraba/internal/platform/db"
	"aciraba/internal/platform/sanitize"
	"aciraba/internal/stock"
	"aciraba/internal/wallet"
)

const ModuleReturns = "sales_returns"

var (
	ErrSaleNotReturnable  = errors.New("nota tidak dapat diretur")
	ErrSaleHasReturns     = errors.New("nota memiliki retur aktif")
	ErrSaleReturnInactive = errors.New("retur sudah dibatalkan")
	// ErrSaleReturnRefunded: dana kembali retur sudah diserahkan ke pelanggan (bukan ke deposit) — retur tidak bisa dibatalkan.
	ErrSaleReturnRefunded = errors.New("dana kembali retur sudah diserahkan ke pelanggan")
	// ErrSaleReturnLocked: piutang nota sudah dibayar sesudah retur dibuat — potongan piutang tidak bisa ditarik lagi.
	ErrSaleReturnLocked = errors.New("piutang nota sudah dibayar setelah retur dibuat")
)

type SaleReturnLineIn struct {
	Position int         `json:"position"`
	Qty      json.Number `json:"qty"`
}

type SaleReturnRequest struct {
	SaleID         uuid.UUID          `json:"sale_id"`
	Lines          []SaleReturnLineIn `json:"lines"`
	Note           string             `json:"note"`
	RefundMethodID *uuid.UUID         `json:"refund_method_id"`
	RefundRef      string             `json:"refund_ref"`
}

type saleReturnLineNorm struct {
	position int
	qty      dec
}

type saleReturnNorm struct {
	sale   uuid.UUID
	lines  []saleReturnLineNorm
	note   string
	method uuid.UUID
	ref    string
}

func normalizeSaleReturn(in SaleReturnRequest, strict bool) (saleReturnNorm, FieldErrors) {
	fields := FieldErrors{}
	n := saleReturnNorm{sale: in.SaleID}
	if in.SaleID == uuid.Nil {
		fields["sale_id"] = sanitize.Required
	}
	if strict && len(in.Lines) == 0 {
		fields["lines"] = sanitize.Required
	}
	if len(in.Lines) > maxLines {
		fields["lines"] = codeTooMany
	}
	seen := map[int]bool{}
	for i, line := range in.Lines {
		key := "lines." + strconv.Itoa(i)
		if line.Position < 0 || seen[line.Position] {
			fields[key+".position"] = sanitize.Invalid
		}
		seen[line.Position] = true
		qty, code := parseDec(line.Qty, 3, true)
		if code == "" && !qty.IsPositive() {
			code = sanitize.Invalid
		}
		if code != "" {
			fields[key+".qty"] = code
		}
		n.lines = append(n.lines, saleReturnLineNorm{position: line.Position, qty: qty})
	}
	note, code := sanitize.Multiline(in.Note, 500)
	if code != "" {
		fields["note"] = code
	}
	n.note = note
	ref, ok := sanitize.Text(in.RefundRef)
	if !ok || utf8.RuneCountInString(ref) > 100 {
		fields["refund_ref"] = sanitize.Invalid
	}
	n.ref = ref
	if in.RefundMethodID != nil {
		n.method = *in.RefundMethodID
	}
	return n, fields
}

func (n saleReturnNorm) hash() string {
	parts := []string{n.sale.String(), n.note, n.method.String(), n.ref}
	for _, line := range n.lines {
		parts = append(parts, strconv.Itoa(line.position)+":"+line.qty.String())
	}
	data, _ := json.Marshal(parts)
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

type saleReturnSale struct {
	id, outletID      uuid.UUID
	outletCode        string
	outletName        string
	outletActive      bool
	status            string
	docNo             string
	memberID          uuid.UUID
	hasMember         bool
	memberName        string
	subtotal          dec
	discount          dec
	redeemAmount      dec
	taxStore          dec
	taxGov            dec
	surcharge         dec
	pointsEarned      int
	pointsRedeemed    int
	today             pgtype.Date
	returnedSubtotal  dec
	returnedDiscount  dec
	returnedTax       dec
	returnedSurcharge dec
	returnedEarned    int
	returnedRedeemed  int
}

type saleReturnSourceLine struct {
	position        int
	itemID          uuid.UUID
	sku, name, unit string
	kind            string
	factor, qty     dec
	lineTotal       dec
	unitCost        dec
	returnedQty     dec
	returnedValue   dec
	returnStock     dec
}

func loadSaleReturnSale(ctx context.Context, tx pgx.Tx, tenant, id uuid.UUID, lock bool) (saleReturnSale, error) {
	query := `SELECT s.id, s.outlet_id, o.code, o.name, o.active, s.status, s.doc_no, s.member_id,
	                 coalesce(m.name, ''), s.subtotal, s.discount, s.redeem_amount, s.tax_store, s.tax_gov, s.surcharge,
	                 s.points_earned, s.points_redeemed, (now() AT TIME ZONE o.timezone)::date
	          FROM sales s JOIN outlets o ON o.tenant_id = s.tenant_id AND o.id = s.outlet_id
	          LEFT JOIN members m ON m.tenant_id = s.tenant_id AND m.id = s.member_id
	          WHERE s.tenant_id = $1 AND s.id = $2`
	if lock {
		query += ` FOR UPDATE OF s`
	}
	var sale saleReturnSale
	var memberID pgtype.UUID
	err := tx.QueryRow(ctx, query, tenant, id).Scan(&sale.id, &sale.outletID, &sale.outletCode, &sale.outletName, &sale.outletActive,
		&sale.status, &sale.docNo, &memberID, &sale.memberName, &sale.subtotal, &sale.discount, &sale.redeemAmount,
		&sale.taxStore, &sale.taxGov, &sale.surcharge, &sale.pointsEarned, &sale.pointsRedeemed, &sale.today)
	if errors.Is(err, pgx.ErrNoRows) {
		return sale, ErrNotFound
	}
	if err != nil {
		return sale, err
	}
	if memberID.Valid {
		sale.memberID, sale.hasMember = uuid.UUID(memberID.Bytes), true
	}
	return sale, nil
}

func checkSaleReturnAccess(a authz.Actor, sale saleReturnSale) error {
	if sale.outletID != a.OutletID {
		if !a.Outlets[sale.outletID] {
			return ErrOutletForbidden
		}
		return ErrOutletMismatch
	}
	if !sale.outletActive {
		return ErrOutletInactive
	}
	if sale.status != "completed" {
		return ErrSaleNotReturnable
	}
	return nil
}

func loadSaleReturnLines(ctx context.Context, tx pgx.Tx, tenant uuid.UUID, sale saleReturnSale) ([]saleReturnSourceLine, error) {
	rows, err := tx.Query(ctx, `
		SELECT l.position, l.item_id, l.sku, l.name, l.unit_name, i.kind, l.factor, l.qty, l.line_total, l.unit_cost,
		       coalesce(rr.qty, 0), coalesce(rr.value, 0), coalesce(sb.qty, 0)
		FROM sale_lines l
		JOIN items i ON i.tenant_id = l.tenant_id AND i.id = l.item_id
		LEFT JOIN LATERAL (
		  SELECT sum(rl.qty) AS qty, sum(rl.value) AS value
		  FROM sales_return_lines rl JOIN sales_returns r ON r.tenant_id = rl.tenant_id AND r.id = rl.return_id
		  WHERE rl.tenant_id = l.tenant_id AND r.sale_id = l.sale_id AND rl.sale_position = l.position AND r.status = 'completed'
		) rr ON true
		LEFT JOIN stock_balances sb ON sb.tenant_id = l.tenant_id AND sb.outlet_id = $3 AND sb.item_id = l.item_id AND sb.bucket = 'returns'
		WHERE l.tenant_id = $1 AND l.sale_id = $2 ORDER BY l.position`, tenant, sale.id, sale.outletID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	lines := []saleReturnSourceLine{}
	for rows.Next() {
		var line saleReturnSourceLine
		var position int32
		if err := rows.Scan(&position, &line.itemID, &line.sku, &line.name, &line.unit, &line.kind, &line.factor, &line.qty,
			&line.lineTotal, &line.unitCost, &line.returnedQty, &line.returnedValue, &line.returnStock); err != nil {
			return nil, err
		}
		line.position = int(position)
		lines = append(lines, line)
	}
	return lines, rows.Err()
}

func loadSaleReturnTotals(ctx context.Context, tx pgx.Tx, tenant, saleID uuid.UUID, sale *saleReturnSale) error {
	return tx.QueryRow(ctx, `SELECT coalesce(sum(subtotal), 0), coalesce(sum(discount), 0), coalesce(sum(tax_amount), 0),
		coalesce(sum(surcharge), 0), coalesce(sum(points_earned_reversed), 0), coalesce(sum(points_redeemed_restored), 0)
		FROM sales_returns WHERE tenant_id = $1 AND sale_id = $2 AND status = 'completed'`, tenant, saleID).Scan(&sale.returnedSubtotal, &sale.returnedDiscount,
		&sale.returnedTax, &sale.returnedSurcharge, &sale.returnedEarned, &sale.returnedRedeemed)
}

func saleReceivableBalance(ctx context.Context, tx pgx.Tx, tenant, saleID uuid.UUID) (dec, bool, error) {
	var amount dec
	err := tx.QueryRow(ctx, `SELECT greatest(0, r.amount
		- coalesce((SELECT sum(p.amount) FROM receivable_payments p WHERE p.tenant_id = r.tenant_id AND p.receivable_id = r.id), 0)
		- coalesce((SELECT sum(sr.receivable_cut) FROM sales_returns sr WHERE sr.tenant_id = r.tenant_id AND sr.sale_id = r.sale_id AND sr.status = 'completed'), 0))
		FROM receivables r WHERE r.tenant_id = $1 AND r.sale_id = $2`, tenant, saleID).Scan(&amount)
	if errors.Is(err, pgx.ErrNoRows) {
		return decimal.Zero, false, nil
	}
	return amount, err == nil, err
}

func (s *Service) loadSaleReturnSource(ctx context.Context, tx pgx.Tx, a authz.Actor, saleID uuid.UUID, lock bool) (saleReturnSale, []saleReturnSourceLine, dec, bool, error) {
	sale, err := loadSaleReturnSale(ctx, tx, a.TenantID, saleID, lock)
	if err != nil {
		return sale, nil, decimal.Zero, false, err
	}
	if err := checkSaleReturnAccess(a, sale); err != nil {
		return sale, nil, decimal.Zero, false, err
	}
	if err := loadSaleReturnTotals(ctx, tx, a.TenantID, sale.id, &sale); err != nil {
		return sale, nil, decimal.Zero, false, err
	}
	lines, err := loadSaleReturnLines(ctx, tx, a.TenantID, sale)
	if err != nil {
		return sale, nil, decimal.Zero, false, err
	}
	balance, hasReceivable, err := saleReceivableBalance(ctx, tx, a.TenantID, sale.id)
	return sale, lines, balance, hasReceivable, err
}

type SaleReturnSource struct {
	SaleID     uuid.UUID              `json:"sale_id"`
	DocNo      string                 `json:"doc_no"`
	OutletName string                 `json:"outlet_name"`
	CreatedAt  time.Time              `json:"created_at"`
	Total      string                 `json:"total"`
	MemberName string                 `json:"member_name"`
	HasMember  bool                   `json:"has_member"` // dana kembali boleh ke deposit member hanya bila nota ber-member
	Receivable string                 `json:"receivable"`
	Lines      []SaleReturnSourceLine `json:"lines"`
}

type SaleReturnSourceLine struct {
	Position    int       `json:"position"`
	ItemID      uuid.UUID `json:"item_id"`
	SKU         string    `json:"sku"`
	Name        string    `json:"name"`
	Unit        string    `json:"unit"`
	Qty         string    `json:"qty"`
	Returned    string    `json:"returned"`
	Returnable  string    `json:"returnable"`
	UnitPrice   string    `json:"unit_price"`
	ReturnStock string    `json:"return_stock"`
}

func (s *Service) SaleReturnSource(ctx context.Context, a authz.Actor, saleID uuid.UUID) (SaleReturnSource, error) {
	var out SaleReturnSource
	err := db.WithTenant(ctx, s.pool, a.TenantID, func(tx pgx.Tx) error {
		sale, lines, balance, hasReceivable, err := s.loadSaleReturnSource(ctx, tx, a, saleID, false)
		if err != nil {
			return err
		}
		var createdAt pgtype.Timestamptz
		var total dec
		if err := tx.QueryRow(ctx, `SELECT created_at, total FROM sales WHERE tenant_id = $1 AND id = $2`, a.TenantID, saleID).Scan(&createdAt, &total); err != nil {
			return err
		}
		out = SaleReturnSource{SaleID: sale.id, DocNo: sale.docNo, OutletName: sale.outletName, CreatedAt: createdAt.Time,
			Total: total.StringFixed(2), MemberName: sale.memberName, HasMember: sale.hasMember, Receivable: "0.00", Lines: []SaleReturnSourceLine{}}
		if hasReceivable {
			out.Receivable = balance.StringFixed(2)
		}
		for _, line := range lines {
			unitPrice := decimal.Zero
			if line.qty.IsPositive() {
				unitPrice = line.lineTotal.Div(line.qty)
			}
			out.Lines = append(out.Lines, SaleReturnSourceLine{Position: line.position, ItemID: line.itemID, SKU: line.sku, Name: line.name,
				Unit: line.unit, Qty: line.qty.String(), Returned: line.returnedQty.String(), Returnable: line.qty.Sub(line.returnedQty).String(),
				UnitPrice: unitPrice.StringFixed(2), ReturnStock: line.returnStock.String()})
		}
		return nil
	})
	return out, err
}

type SaleReturnQuoteLine struct {
	Position int       `json:"position"`
	ItemID   uuid.UUID `json:"item_id"`
	SKU      string    `json:"sku"`
	Name     string    `json:"name"`
	Unit     string    `json:"unit"`
	Qty      string    `json:"qty"`
	Value    string    `json:"value"`
}

type SaleReturnQuote struct {
	Lines                  []SaleReturnQuoteLine `json:"lines"`
	Subtotal               string                `json:"subtotal"`
	Discount               string                `json:"discount"`
	TaxAmount              string                `json:"tax_amount"`
	Surcharge              string                `json:"surcharge"`
	Total                  string                `json:"total"`
	ReceivableBalance      string                `json:"receivable_balance"`
	ReceivableCut          string                `json:"receivable_cut"`
	Refund                 string                `json:"refund"`
	PointsEarnedReversed   int                   `json:"points_earned_reversed"`
	PointsRedeemedRestored int                   `json:"points_redeemed_restored"`
}

type saleReturnCalcLine struct {
	source              saleReturnSourceLine
	qty, baseQty, value dec
	discount, tax       dec
}

type saleReturnCalc struct {
	lines                              []saleReturnCalcLine
	subtotal, discount, tax, surcharge dec
	total, receivableCut, refund       dec
	receivableBalance                  dec
	pointsEarned, pointsRedeemed       int
}

func cumulativeAmount(total, cumulative, whole dec, already dec) dec {
	if !whole.IsPositive() || !cumulative.IsPositive() {
		return decimal.Zero
	}
	target := total
	if cumulative.LessThan(whole) {
		target = total.Mul(cumulative).Div(whole).Round(2)
	}
	return target.Sub(already)
}

func allocateReturnAmount(total dec, lines []saleReturnCalcLine) []dec {
	out := make([]dec, len(lines))
	if len(lines) == 0 {
		return out
	}
	remaining := total
	basis := decimal.Zero
	for _, line := range lines {
		basis = basis.Add(line.value)
	}
	for i, line := range lines {
		if i == len(lines)-1 {
			out[i] = remaining
		} else if basis.IsPositive() {
			out[i] = total.Mul(line.value).Div(basis).Round(2)
			remaining = remaining.Sub(out[i])
		}
	}
	return out
}

func calcSaleReturn(ctx context.Context, tx pgx.Tx, a authz.Actor, sale saleReturnSale, source []saleReturnSourceLine, receivableBalance dec, hasReceivable bool, n saleReturnNorm) (saleReturnCalc, FieldErrors, error) {
	byPosition := make(map[int]saleReturnSourceLine, len(source))
	for _, line := range source {
		byPosition[line.position] = line
	}
	fields := FieldErrors{}
	calc := saleReturnCalc{receivableBalance: receivableBalance}
	for i, input := range n.lines {
		key := "lines." + strconv.Itoa(i)
		line, ok := byPosition[input.position]
		if !ok {
			fields[key+".position"] = sanitize.Invalid
			continue
		}
		remaining := line.qty.Sub(line.returnedQty)
		if input.qty.GreaterThan(remaining) {
			fields[key+".qty"] = "TOO_HIGH"
			continue
		}
		left := line.lineTotal.Sub(line.returnedValue)
		value := left
		if input.qty.LessThan(remaining) {
			value = line.lineTotal.Mul(input.qty).Div(line.qty).Round(2)
			if value.GreaterThan(left) {
				value = left
			}
		}
		calc.lines = append(calc.lines, saleReturnCalcLine{source: line, qty: input.qty, baseQty: input.qty.Mul(line.factor), value: value})
		calc.subtotal = calc.subtotal.Add(value)
	}
	if len(fields) > 0 {
		return saleReturnCalc{}, fields, nil
	}
	if len(calc.lines) == 0 {
		return saleReturnCalc{}, FieldErrors{"lines": sanitize.Required}, nil
	}

	cumulative := sale.returnedSubtotal.Add(calc.subtotal)
	calc.discount = cumulativeAmount(sale.discount, cumulative, sale.subtotal, sale.returnedDiscount)
	netReturned := cumulative.Sub(sale.returnedDiscount).Sub(calc.discount)
	netSale := sale.subtotal.Sub(sale.discount)
	calc.tax = cumulativeAmount(sale.taxStore.Add(sale.taxGov), netReturned, netSale, sale.returnedTax)
	// Biaya metode yang ditagihkan ke pelanggan (surcharge) dan biaya lain-lain nota TIDAK dikembalikan (keputusan pengguna
	// 2026-10-10): MDR sudah dibayar toko ke penyedia pembayaran. Kolom sales_returns.surcharge tetap ada dan selalu 0.
	calc.surcharge = decimal.Zero
	for _, amount := range []*dec{&calc.discount, &calc.tax} {
		if amount.IsNegative() {
			*amount = decimal.Zero
		}
	}
	discounts := allocateReturnAmount(calc.discount, calc.lines)
	taxes := allocateReturnAmount(calc.tax, calc.lines)
	for i := range calc.lines {
		calc.lines[i].discount, calc.lines[i].tax = discounts[i], taxes[i]
	}
	calc.total = calc.subtotal.Sub(calc.discount).Add(calc.tax).Add(calc.surcharge)
	if calc.total.IsNegative() {
		calc.total = decimal.Zero
	}
	if hasReceivable && receivableBalance.IsPositive() {
		calc.receivableCut = decimal.Min(receivableBalance, calc.total)
	}
	calc.refund = calc.total.Sub(calc.receivableCut)
	calc.pointsEarned = member.ReturnedPoints(sale.pointsEarned, netReturned, netSale, sale.returnedEarned)
	calc.pointsRedeemed = member.ReturnedPoints(sale.pointsRedeemed, cumulative, sale.subtotal, sale.returnedRedeemed)
	return calc, nil, nil
}

func (c saleReturnCalc) quote() SaleReturnQuote {
	out := SaleReturnQuote{Lines: []SaleReturnQuoteLine{}, Subtotal: c.subtotal.StringFixed(2), Discount: c.discount.StringFixed(2),
		TaxAmount: c.tax.StringFixed(2), Surcharge: c.surcharge.StringFixed(2), Total: c.total.StringFixed(2),
		ReceivableBalance: c.receivableBalance.StringFixed(2), ReceivableCut: c.receivableCut.StringFixed(2), Refund: c.refund.StringFixed(2),
		PointsEarnedReversed: c.pointsEarned, PointsRedeemedRestored: c.pointsRedeemed}
	for _, line := range c.lines {
		q := SaleReturnQuoteLine{Position: line.source.position, ItemID: line.source.itemID, SKU: line.source.sku, Name: line.source.name,
			Unit: line.source.unit, Qty: line.qty.String(), Value: line.value.Sub(line.discount).Add(line.tax).StringFixed(2)}
		out.Lines = append(out.Lines, q)
	}
	return out
}

func (s *Service) QuoteSaleReturn(ctx context.Context, a authz.Actor, in SaleReturnRequest) (SaleReturnQuote, error) {
	n, fields := normalizeSaleReturn(in, false)
	if len(fields) > 0 {
		return SaleReturnQuote{}, fields
	}
	var out SaleReturnQuote
	err := db.WithTenant(ctx, s.pool, a.TenantID, func(tx pgx.Tx) error {
		sale, lines, balance, hasReceivable, err := s.loadSaleReturnSource(ctx, tx, a, n.sale, false)
		if err != nil {
			return err
		}
		calc, f, err := calcSaleReturn(ctx, tx, a, sale, lines, balance, hasReceivable, n)
		if err != nil {
			return err
		}
		if len(f) > 0 {
			return f
		}
		out = calc.quote()
		return nil
	})
	return out, err
}

type SaleReturn struct {
	ID                     uuid.UUID              `json:"id"`
	DocNo                  string                 `json:"doc_no"`
	OutletID               uuid.UUID              `json:"outlet_id"`
	OutletName             string                 `json:"outlet_name"`
	SaleID                 uuid.UUID              `json:"sale_id"`
	SaleDocNo              string                 `json:"sale_doc_no"`
	MemberName             string                 `json:"member_name"`
	ReturnDate             string                 `json:"return_date"`
	Note                   string                 `json:"note"`
	Subtotal               string                 `json:"subtotal"`
	Discount               string                 `json:"discount"`
	TaxAmount              string                 `json:"tax_amount"`
	Surcharge              string                 `json:"surcharge"`
	Total                  string                 `json:"total"`
	ReceivableCut          string                 `json:"receivable_cut"`
	Refund                 string                 `json:"refund"`
	RefundMethod           string                 `json:"refund_method"` // jenis metode dana kembali ("" = tanpa dana kembali)
	RefundMethodName       string                 `json:"refund_method_name"`
	RefundRef              string                 `json:"refund_ref"`
	PointsEarnedReversed   int                    `json:"points_earned_reversed"`
	PointsRedeemedRestored int                    `json:"points_redeemed_restored"`
	CreatedAt              time.Time              `json:"created_at"`
	CreatedBy              string                 `json:"created_by"`
	Status                 string                 `json:"status"` // completed | void
	VoidReason             string                 `json:"void_reason"`
	VoidedAt               *time.Time             `json:"voided_at"`
	VoidedBy               string                 `json:"voided_by"`
	Lines                  []SaleReturnDetailLine `json:"lines"`
}

type SaleReturnDetailLine struct {
	Position     int       `json:"position"`
	SalePosition int       `json:"sale_position"`
	ItemID       uuid.UUID `json:"item_id"`
	SKU          string    `json:"sku"`
	Name         string    `json:"name"`
	Unit         string    `json:"unit"`
	Qty          string    `json:"qty"`
	Value        string    `json:"value"`
	Discount     string    `json:"discount"`
	TaxAmount    string    `json:"tax_amount"`
	UnitCost     string    `json:"unit_cost"`
}

type saleReturnReplay struct{ id uuid.UUID }

func (saleReturnReplay) Error() string { return "replay" }

func checkSaleReturnKey(ctx context.Context, tx pgx.Tx, tenant uuid.UUID, key, hash string) error {
	var id uuid.UUID
	var stored string
	err := tx.QueryRow(ctx, `SELECT id, request_hash FROM sales_returns WHERE tenant_id = $1 AND idempotency_key = $2`, tenant, key).Scan(&id, &stored)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	if stored != hash {
		return ErrKeyMismatch
	}
	return saleReturnReplay{id: id}
}

func optionalUUID(id uuid.UUID) pgtype.UUID {
	return pgtype.UUID{Bytes: id, Valid: id != uuid.Nil}
}

func (s *Service) CreateSaleReturn(ctx context.Context, a authz.Actor, key string, in SaleReturnRequest) (result SaleReturn, replayed bool, err error) {
	if !idemKeyPattern.MatchString(key) {
		return SaleReturn{}, false, ErrKeyRequired
	}
	n, fields := normalizeSaleReturn(in, true)
	if len(fields) > 0 {
		return SaleReturn{}, false, fields
	}
	hash := n.hash()
	var returnID uuid.UUID
	err = db.WithTenant(ctx, s.pool, a.TenantID, func(tx pgx.Tx) error {
		if err := checkSaleReturnKey(ctx, tx, a.TenantID, key, hash); err != nil {
			return err
		}
		sale, source, balance, hasReceivable, err := s.loadSaleReturnSource(ctx, tx, a, n.sale, true)
		if err != nil {
			return err
		}
		// Recheck after waiting on the sale row so concurrent retries see the committed idempotency record.
		if err := checkSaleReturnKey(ctx, tx, a.TenantID, key, hash); err != nil {
			return err
		}
		calc, f, err := calcSaleReturn(ctx, tx, a, sale, source, balance, hasReceivable, n)
		if err != nil {
			return err
		}
		if len(f) > 0 {
			return f
		}
		var methodID pgtype.UUID
		var methodKind, methodName, refundRef string
		if calc.refund.IsPositive() {
			if n.method == uuid.Nil {
				return FieldErrors{"refund_method_id": sanitize.Required}
			}
			var active bool
			err := tx.QueryRow(ctx, `SELECT kind, name, active FROM payment_methods WHERE tenant_id = $1 AND id = $2`, a.TenantID, n.method).
				Scan(&methodKind, &methodName, &active)
			if errors.Is(err, pgx.ErrNoRows) {
				return FieldErrors{"refund_method_id": sanitize.Invalid}
			}
			if err != nil {
				return err
			}
			switch {
			case !active:
				return FieldErrors{"refund_method_id": "METHOD_INACTIVE"}
			case methodKind == wallet.KindSupplierCredit:
				return FieldErrors{"refund_method_id": sanitize.Invalid}
			case methodKind == wallet.KindDeposit && !sale.hasMember:
				return FieldErrors{"refund_method_id": codeMemberRequired} // deposit hanya untuk nota ber-member
			case methodKind != "cash" && methodKind != wallet.KindDeposit && strings.TrimSpace(n.ref) == "":
				return FieldErrors{"refund_ref": sanitize.Required}
			}
			methodID = optionalUUID(n.method)
			refundRef = n.ref
		}
		var no int64
		if err := tx.QueryRow(ctx, `INSERT INTO sales_return_counters (tenant_id, outlet_id, day, last_no) VALUES ($1, $2, $3, 1)
			ON CONFLICT (tenant_id, outlet_id, day) DO UPDATE SET last_no = sales_return_counters.last_no + 1 RETURNING last_no`,
			a.TenantID, sale.outletID, sale.today).Scan(&no); err != nil {
			return err
		}
		docNo := fmt.Sprintf("RJ-%s-%s-%04d", strings.ToUpper(sale.outletCode), sale.today.Time.Format("060102"), no)
		returnID = uuid.New()
		_, err = tx.Exec(ctx, `INSERT INTO sales_returns (id, tenant_id, outlet_id, sale_id, member_id, doc_no, idempotency_key, request_hash,
			return_date, note, subtotal, discount, tax_amount, surcharge, total, receivable_cut, refund, refund_method, refund_method_id,
			refund_method_name, refund_ref, points_earned_reversed, points_redeemed_restored, created_by)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20,$21,$22,$23,$24)`,
			returnID, a.TenantID, sale.outletID, sale.id, optionalUUID(sale.memberID), docNo, key, hash, sale.today, n.note, calc.subtotal,
			calc.discount, calc.tax, calc.surcharge, calc.total, calc.receivableCut, calc.refund, nullableString(methodKind), methodID,
			methodName, refundRef, calc.pointsEarned, calc.pointsRedeemed, optionalUUID(a.UserID))
		if err != nil {
			return err
		}
		for i, line := range calc.lines {
			if _, err := tx.Exec(ctx, `INSERT INTO sales_return_lines (tenant_id, return_id, position, sale_position, item_id, item_sku, item_name,
				unit_name, factor, qty, base_qty, value, discount, tax_amount, unit_cost)
				VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15)`, a.TenantID, returnID, i, line.source.position,
				line.source.itemID, line.source.sku, line.source.name, line.source.unit, line.source.factor, line.qty, line.baseQty,
				line.value, line.discount, line.tax, line.source.unitCost); err != nil {
				return err
			}
		}
		if err := applySaleReturnStock(ctx, tx, a, sale.outletID, returnID, docNo, calc.lines, false); err != nil {
			return err
		}
		if sale.hasMember {
			if err := member.ApplySaleReturn(ctx, tx, a, sale.memberID, sale.id, returnID, docNo, calc.pointsEarned, calc.pointsRedeemed); err != nil {
				return err
			}
		}
		// Dana kembali ke deposit member: saldo deposit bertambah (uang tidak keluar dari laci).
		if methodKind == wallet.KindDeposit {
			if _, err := wallet.MemberDeposit.Apply(ctx, tx, wallet.Move{TenantID: a.TenantID, OwnerID: sale.memberID, OutletID: sale.outletID,
				Kind: wallet.DepSaleReturn, Amount: calc.refund, RefID: returnID, DocNo: docNo, ActorID: a.UserID}); err != nil {
				return err
			}
		}
		return audit.Record(ctx, tx, audit.FromActor(a), audit.Entry{Action: audit.ActionSaleReturnCreate, Entity: audit.EntitySaleReturn,
			EntityID: returnID.String(), Details: map[string]any{"doc_no": docNo, "sale_id": sale.id.String(), "sale_doc_no": sale.docNo,
				"total": calc.total.StringFixed(2), "receivable_cut": calc.receivableCut.StringFixed(2), "refund": calc.refund.StringFixed(2),
				"points_earned_reversed": calc.pointsEarned, "points_redeemed_restored": calc.pointsRedeemed, "lines": len(calc.lines)}})
	})
	var replay saleReturnReplay
	if errors.As(err, &replay) {
		result, err = s.GetSaleReturn(ctx, a, replay.id)
		return result, true, err
	}
	if err != nil {
		return SaleReturn{}, false, err
	}
	result, err = s.GetSaleReturn(ctx, a, returnID)
	return result, false, err
}

func nullableString(s string) pgtype.Text {
	return pgtype.Text{String: s, Valid: s != ""}
}

// applySaleReturnStock memasukkan barang retur ke bucket Retur outlet nota (void=false) atau mengeluarkannya lagi saat retur
// dibatalkan (void=true), lalu menghitung HPP rata-rata cabang maju/mundur dengan HPP baris nota. Barang dikunci terurut id
// (FOR NO KEY UPDATE) dan stok dibaca SETELAH ApplyAll (baris saldo sudah terkunci; pelajaran HPP basi, AGENTS.md §11).
func applySaleReturnStock(ctx context.Context, tx pgx.Tx, a authz.Actor, outlet uuid.UUID, returnID uuid.UUID, docNo string, lines []saleReturnCalcLine, void bool) error {
	type aggregate struct{ qty, value dec }
	aggs := map[uuid.UUID]*aggregate{}
	for _, line := range lines {
		if line.source.kind != "goods" {
			continue
		}
		agg := aggs[line.source.itemID]
		if agg == nil {
			agg = &aggregate{}
			aggs[line.source.itemID] = agg
		}
		agg.qty = agg.qty.Add(line.baseQty)
		agg.value = agg.value.Add(line.qty.Mul(line.source.unitCost))
	}
	ids := make([]uuid.UUID, 0, len(aggs))
	for id := range aggs {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return strings.Compare(ids[i].String(), ids[j].String()) < 0 })
	q := gen.New(tx)
	type costState struct{ avg, last dec }
	states := make(map[uuid.UUID]costState, len(ids))
	moves := make([]stock.Movement, 0, len(ids))
	for _, id := range ids {
		if _, err := q.PurchaseItemLock(ctx, gen.PurchaseItemLockParams{TenantID: a.TenantID, OutletID: outlet, ID: id}); err != nil {
			return err
		}
		state, err := q.PurchaseItemState(ctx, gen.PurchaseItemStateParams{TenantID: a.TenantID, OutletID: outlet, ID: id})
		if err != nil {
			return err
		}
		states[id] = costState{avg: state.AvgCost, last: state.LastCost}
		delta := aggs[id].qty
		if void {
			delta = delta.Neg()
		}
		moves = append(moves, stock.Movement{TenantID: a.TenantID, OutletID: outlet, ItemID: id, Bucket: stock.BucketReturns,
			Delta: delta, RefType: stock.RefSaleReturn, RefID: returnID, Note: docNo, ActorID: a.UserID})
	}
	if len(moves) == 0 {
		return nil
	}
	if _, err := stock.ApplyAll(ctx, tx, moves); err != nil {
		return err
	}
	for _, id := range ids {
		currentQty, err := q.StockOutletQty(ctx, gen.StockOutletQtyParams{TenantID: a.TenantID, OutletID: outlet, ItemID: id})
		if err != nil {
			return err
		}
		g, old := aggs[id], states[id]
		avg := old.avg
		if void {
			// Stok sebelum pembatalan = stok sekarang + qty yang keluar; HPP mundur (dibiarkan bila sisa ≤ 0).
			before := currentQty.Add(g.qty)
			if rem := before.Sub(g.qty); rem.IsPositive() {
				avg = before.Mul(old.avg).Sub(g.value).DivRound(rem, 2)
				if avg.IsNegative() {
					avg = decimal.Zero
				}
			}
		} else {
			before := currentQty.Sub(g.qty)
			if !before.IsPositive() || !old.avg.IsPositive() {
				avg = g.value.Div(g.qty).Round(2)
			} else {
				avg = before.Mul(old.avg).Add(g.value).Div(before.Add(g.qty)).Round(2)
			}
		}
		if err := q.StockSetCost(ctx, gen.StockSetCostParams{TenantID: a.TenantID, OutletID: outlet, ItemID: id, AvgCost: avg, LastCost: old.last}); err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) SalesReturnChoices(ctx context.Context, a authz.Actor, query string) ([]SaleReturnChoice, error) {
	if len([]rune(query)) > 100 {
		query = string([]rune(query)[:100])
	}
	pattern := ""
	if query != "" {
		pattern = "%" + escapeLike(query) + "%"
	}
	out := []SaleReturnChoice{}
	err := db.WithTenant(ctx, s.pool, a.TenantID, func(tx pgx.Tx) error {
		// Kandidat ber-indeks (sale_search_ids, 00052); pola ILIKE hanya dipakai bila kandidat melebihi batas.
		var ids []uuid.UUID
		byIDs := false
		if pattern != "" {
			var err error
			if ids, byIDs, err = db.SearchCandidates(ctx, tx, `SELECT sale_search_ids(ARRAY[$1::uuid], NULL, NULL, $2, false, $3)`,
				a.OutletID, pattern, db.SearchCap+1); err != nil {
				return err
			}
			if byIDs {
				pattern = ""
			}
		}
		rows, err := tx.Query(ctx, `SELECT s.id, s.doc_no, s.created_at, s.total, coalesce(m.name, ''),
			coalesce(r.balance, 0), coalesce(r.has_receivable, false)
			FROM sales s LEFT JOIN members m ON m.tenant_id = s.tenant_id AND m.id = s.member_id
			LEFT JOIN LATERAL (SELECT greatest(0, x.amount - coalesce((SELECT sum(p.amount) FROM receivable_payments p WHERE p.tenant_id = x.tenant_id AND p.receivable_id = x.id),0)
				- coalesce((SELECT sum(sr.receivable_cut) FROM sales_returns sr WHERE sr.tenant_id=x.tenant_id AND sr.sale_id=x.sale_id AND sr.status='completed'),0)) AS balance, true AS has_receivable
				FROM receivables x WHERE x.tenant_id=s.tenant_id AND x.sale_id=s.id) r ON true
			WHERE s.tenant_id=$1 AND s.outlet_id=$2 AND s.status='completed'
			AND EXISTS (SELECT 1 FROM sale_lines l WHERE l.tenant_id=s.tenant_id AND l.sale_id=s.id
				AND l.qty > coalesce((SELECT sum(rl.qty) FROM sales_return_lines rl JOIN sales_returns sr ON sr.tenant_id=rl.tenant_id AND sr.id=rl.return_id
					WHERE rl.tenant_id=s.tenant_id AND sr.sale_id=s.id AND rl.sale_position=l.position AND sr.status='completed'),0))
			AND ($3::text='' OR s.doc_no ILIKE $3 OR m.name ILIKE $3 OR EXISTS (SELECT 1 FROM users u WHERE u.tenant_id=s.tenant_id AND u.id=s.cashier_id AND u.name ILIKE $3))
			AND (NOT $4::bool OR s.id = ANY($5::uuid[]))
			ORDER BY s.created_at DESC, s.id DESC LIMIT 20`, a.TenantID, a.OutletID, pattern, byIDs, ids)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var choice SaleReturnChoice
			var at pgtype.Timestamptz
			var total, balance dec
			var hasReceivable bool
			if err := rows.Scan(&choice.SaleID, &choice.DocNo, &at, &total, &choice.MemberName, &balance, &hasReceivable); err != nil {
				return err
			}
			choice.CreatedAt, choice.Total = at.Time, total.StringFixed(2)
			if hasReceivable {
				choice.Receivable = balance.StringFixed(2)
			} else {
				choice.Receivable = "0.00"
			}
			out = append(out, choice)
		}
		return rows.Err()
	})
	return out, err
}

type SaleReturnChoice struct {
	SaleID     uuid.UUID `json:"sale_id"`
	DocNo      string    `json:"doc_no"`
	CreatedAt  time.Time `json:"created_at"`
	Total      string    `json:"total"`
	MemberName string    `json:"member_name"`
	Receivable string    `json:"receivable"`
}

func parseSaleReturnDate(v string) (time.Time, bool) {
	d, err := time.Parse("2006-01-02", v)
	return d, err == nil
}

func saleReturnPGDate(value time.Time) pgtype.Date {
	return pgtype.Date{Time: value, Valid: true}
}

func encodeSaleReturnCursor(date string, at time.Time, id uuid.UUID) string {
	return base64.RawURLEncoding.EncodeToString([]byte(date + "|" + at.UTC().Format(time.RFC3339Nano) + "|" + id.String()))
}

func decodeSaleReturnCursor(value string) (time.Time, time.Time, uuid.UUID, bool) {
	raw, err := base64.RawURLEncoding.DecodeString(value)
	if err != nil {
		return time.Time{}, time.Time{}, uuid.Nil, false
	}
	parts := strings.Split(string(raw), "|")
	if len(parts) != 3 {
		return time.Time{}, time.Time{}, uuid.Nil, false
	}
	date, err1 := time.Parse("2006-01-02", parts[0])
	at, err2 := time.Parse(time.RFC3339Nano, parts[1])
	id, err3 := uuid.Parse(parts[2])
	return date, at, id, err1 == nil && err2 == nil && err3 == nil
}

type SaleReturnListParams struct {
	From, To, Query, Cursor string
	Status                  string // "" | completed | void
	Limit                   int
}

type SaleReturnRow struct {
	ID            uuid.UUID `json:"id"`
	DocNo         string    `json:"doc_no"`
	SaleID        uuid.UUID `json:"sale_id"`
	SaleDocNo     string    `json:"sale_doc_no"`
	MemberName    string    `json:"member_name"`
	ReturnDate    string    `json:"return_date"`
	Status        string    `json:"status"`
	Total         string    `json:"total"`
	Refund        string    `json:"refund"`
	ReceivableCut string    `json:"receivable_cut"`
	Lines         int       `json:"lines"`
	CreatedAt     time.Time `json:"created_at"`
	CreatedBy     string    `json:"created_by"`
}

type SaleReturnListResult struct {
	Data       []SaleReturnRow `json:"data"`
	HasMore    bool            `json:"has_more"`
	NextCursor string          `json:"next_cursor"`
}

func (s *Service) ListSaleReturns(ctx context.Context, a authz.Actor, p SaleReturnListParams) (SaleReturnListResult, error) {
	now := time.Now().UTC()
	from, to := now.AddDate(0, 0, -30), now.AddDate(0, 0, 1)
	if p.From != "" || p.To != "" {
		var ok bool
		if from, ok = parseSaleReturnDate(p.From); !ok {
			return SaleReturnListResult{}, FieldErrors{"from": sanitize.Invalid}
		}
		if to, ok = parseSaleReturnDate(p.To); !ok {
			return SaleReturnListResult{}, FieldErrors{"to": sanitize.Invalid}
		}
		to = to.AddDate(0, 0, 1)
	}
	if to.Before(from) || to.Sub(from) > 367*24*time.Hour {
		return SaleReturnListResult{}, FieldErrors{"to": sanitize.Invalid}
	}
	if len([]rune(p.Query)) > 100 {
		p.Query = string([]rune(p.Query)[:100])
	}
	if p.Status != "" && p.Status != "completed" && p.Status != "void" {
		return SaleReturnListResult{}, FieldErrors{"status": sanitize.Invalid}
	}
	limit := p.Limit
	if limit < 1 || limit > 100 {
		limit = 50
	}
	pattern := ""
	if p.Query != "" {
		pattern = "%" + escapeLike(p.Query) + "%"
	}
	var cursorDate time.Time
	var cursorAt time.Time
	var cursorID uuid.UUID
	cursorOn := p.Cursor != ""
	if cursorOn {
		var ok bool
		cursorDate, cursorAt, cursorID, ok = decodeSaleReturnCursor(p.Cursor)
		if !ok {
			return SaleReturnListResult{}, FieldErrors{"cursor": sanitize.Invalid}
		}
	}
	result := SaleReturnListResult{Data: []SaleReturnRow{}}
	err := db.WithTenant(ctx, s.pool, a.TenantID, func(tx pgx.Tx) error {
		var ids []uuid.UUID
		byIDs := false
		if pattern != "" {
			var err error
			if ids, byIDs, err = db.SearchCandidates(ctx, tx, `SELECT sales_return_search_ids($1, $2, $3, $4, $5)`,
				a.OutletID, saleReturnPGDate(from), saleReturnPGDate(to), pattern, db.SearchCap+1); err != nil {
				return err
			}
			if byIDs {
				pattern = ""
			}
		}
		rows, err := tx.Query(ctx, `SELECT r.id, r.doc_no, r.sale_id, s.doc_no, coalesce(m.name,''), r.return_date, r.status, r.total,
			r.refund, r.receivable_cut, (SELECT count(*) FROM sales_return_lines l WHERE l.tenant_id=r.tenant_id AND l.return_id=r.id),
			r.created_at, coalesce(u.name,'')
			FROM sales_returns r JOIN sales s ON s.tenant_id=r.tenant_id AND s.id=r.sale_id
			LEFT JOIN members m ON m.tenant_id=s.tenant_id AND m.id=s.member_id
			LEFT JOIN users u ON u.tenant_id=r.tenant_id AND u.id=r.created_by
			WHERE r.tenant_id=$1 AND r.outlet_id=$2 AND r.return_date >= $3 AND r.return_date < $4
			AND ($5::text='' OR r.doc_no ILIKE $5 OR s.doc_no ILIKE $5 OR m.name ILIKE $5)
			AND (NOT $6::bool OR (r.return_date,r.created_at,r.id) < ($7::date,$8::timestamptz,$9::uuid))
			AND ($11::text = '' OR r.status = $11)
			AND (NOT $12::bool OR r.id = ANY($13::uuid[]))
			ORDER BY r.return_date DESC,r.created_at DESC,r.id DESC LIMIT $10`, a.TenantID, a.OutletID, saleReturnPGDate(from), saleReturnPGDate(to), pattern,
			cursorOn, saleReturnPGDate(cursorDate), pgtype.Timestamptz{Time: cursorAt, Valid: cursorOn}, cursorID, limit+1, p.Status, byIDs, ids)
		if err != nil {
			return err
		}
		for rows.Next() {
			var row SaleReturnRow
			var date pgtype.Date
			var total, refund, cut dec
			var count int64
			var at pgtype.Timestamptz
			if err := rows.Scan(&row.ID, &row.DocNo, &row.SaleID, &row.SaleDocNo, &row.MemberName, &date, &row.Status, &total, &refund, &cut, &count, &at, &row.CreatedBy); err != nil {
				rows.Close()
				return err
			}
			row.ReturnDate, row.Total, row.Refund, row.ReceivableCut = date.Time.Format("2006-01-02"), total.StringFixed(2), refund.StringFixed(2), cut.StringFixed(2)
			row.Lines, row.CreatedAt = int(count), at.Time
			result.Data = append(result.Data, row)
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return err
		}
		if len(result.Data) > limit {
			result.Data = result.Data[:limit]
			result.HasMore = true
			last := result.Data[len(result.Data)-1]
			result.NextCursor = encodeSaleReturnCursor(last.ReturnDate, last.CreatedAt, last.ID)
		}
		return nil
	})
	return result, err
}

func (s *Service) GetSaleReturn(ctx context.Context, a authz.Actor, id uuid.UUID) (SaleReturn, error) {
	var out SaleReturn
	err := db.WithTenant(ctx, s.pool, a.TenantID, func(tx pgx.Tx) error {
		var date pgtype.Date
		var createdAt, voidedAt pgtype.Timestamptz
		var sub, disc, tax, sur, total, cut, refund dec
		err := tx.QueryRow(ctx, `SELECT r.id,r.doc_no,r.outlet_id,o.name,r.sale_id,s.doc_no,coalesce(m.name,''),r.return_date,r.note,
			r.subtotal,r.discount,r.tax_amount,r.surcharge,r.total,r.receivable_cut,r.refund,r.refund_method_name,r.refund_ref,
			r.points_earned_reversed,r.points_redeemed_restored,r.created_at,coalesce(u.name,''),
			r.status,coalesce(r.refund_method,''),r.void_reason,r.voided_at,coalesce(vu.name,'')
			FROM sales_returns r JOIN outlets o ON o.tenant_id=r.tenant_id AND o.id=r.outlet_id
			JOIN sales s ON s.tenant_id=r.tenant_id AND s.id=r.sale_id LEFT JOIN members m ON m.tenant_id=s.tenant_id AND m.id=s.member_id
			LEFT JOIN users u ON u.tenant_id=r.tenant_id AND u.id=r.created_by
			LEFT JOIN users vu ON vu.tenant_id=r.tenant_id AND vu.id=r.voided_by WHERE r.tenant_id=$1 AND r.id=$2`, a.TenantID, id).Scan(
			&out.ID, &out.DocNo, &out.OutletID, &out.OutletName, &out.SaleID, &out.SaleDocNo, &out.MemberName, &date, &out.Note,
			&sub, &disc, &tax, &sur, &total, &cut, &refund,
			&out.RefundMethodName, &out.RefundRef, &out.PointsEarnedReversed, &out.PointsRedeemedRestored, &createdAt, &out.CreatedBy,
			&out.Status, &out.RefundMethod, &out.VoidReason, &voidedAt, &out.VoidedBy)
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNotFound
		}
		if err != nil {
			return err
		}
		if out.OutletID != a.OutletID && !a.Outlets[out.OutletID] {
			return ErrOutletForbidden
		}
		out.ReturnDate, out.CreatedAt = date.Time.Format("2006-01-02"), createdAt.Time
		out.Subtotal, out.Discount, out.TaxAmount, out.Surcharge = sub.StringFixed(2), disc.StringFixed(2), tax.StringFixed(2), sur.StringFixed(2)
		out.Total, out.ReceivableCut, out.Refund = total.StringFixed(2), cut.StringFixed(2), refund.StringFixed(2)
		if voidedAt.Valid {
			t := voidedAt.Time
			out.VoidedAt = &t
		}
		out.Lines = []SaleReturnDetailLine{}
		rows, err := tx.Query(ctx, `SELECT position,sale_position,item_id,item_sku,item_name,unit_name,qty,value,discount,tax_amount,unit_cost
			FROM sales_return_lines WHERE tenant_id=$1 AND return_id=$2 ORDER BY position`, a.TenantID, id)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var line SaleReturnDetailLine
			var pos, sourcePos int32
			var qty, value, discount, tax, cost dec
			if err := rows.Scan(&pos, &sourcePos, &line.ItemID, &line.SKU, &line.Name, &line.Unit, &qty, &value, &discount, &tax, &cost); err != nil {
				return err
			}
			line.Position, line.SalePosition = int(pos), int(sourcePos)
			line.Qty, line.Value, line.Discount, line.TaxAmount, line.UnitCost = qty.String(), value.StringFixed(2), discount.StringFixed(2), tax.StringFixed(2), cost.StringFixed(2)
			out.Lines = append(out.Lines, line)
		}
		return rows.Err()
	})
	return out, err
}

func (r SaleReturn) String() string { return fmt.Sprintf("%s (%s)", r.DocNo, r.Total) }
