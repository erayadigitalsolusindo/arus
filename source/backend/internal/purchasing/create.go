package purchasing

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"

	"aciraba/internal/audit"
	"aciraba/internal/authz"
	gen "aciraba/internal/gen"
	"aciraba/internal/platform/sanitize"
	"aciraba/internal/stock"
)

type replay struct{ id uuid.UUID }

func (replay) Error() string { return "replay" }

const invoiceUniqueIdx = "purchases_supplier_invoice_key"

func pgDate(t time.Time) pgtype.Date { return pgtype.Date{Time: t, Valid: true} }

func nullUUID(id uuid.UUID) pgtype.UUID { return pgtype.UUID{Bytes: id, Valid: id != uuid.Nil} }

// Create menyimpan satu nota pembelian. replayed=true bila Idempotency-Key yang sama sudah pernah menyimpan nota
// (nota yang sama dikembalikan; tidak ada nota/stok/HPP ganda).
func (s *Service) Create(ctx context.Context, a authz.Actor, key string, in Request) (p Purchase, replayed bool, err error) {
	if !idemKeyPattern.MatchString(key) {
		return Purchase{}, false, ErrKeyRequired
	}
	n, f := normalize(in, true)
	if len(f) > 0 {
		return Purchase{}, false, f
	}
	am, f := computeAmounts(n)
	if len(f) > 0 {
		return Purchase{}, false, f
	}
	h := n.hash()

	var id uuid.UUID
	err = s.tx(ctx, a, func(tx pgx.Tx) error {
		var e error
		id, e = s.save(ctx, tx, a, n, am, key, h, nil)
		return e
	})
	var rp replay
	if errors.As(err, &rp) {
		p, err = s.Get(ctx, a, rp.id)
		return p, true, err
	}
	if err != nil {
		return Purchase{}, false, err
	}
	p, err = s.Get(ctx, a, id)
	return p, false, err
}

// lockItems mengunci barang-barang nota (terurut id) dan memvalidasinya. Penguncian ini membuat dua pembelian barang yang
// sama antre, sehingga HPP rata-rata tidak saling menimpa.
func lockItems(ctx context.Context, q *gen.Queries, a authz.Actor, n norm) (map[uuid.UUID]gen.PurchaseItemLockRow, FieldErrors, error) {
	ids, at := distinctItems(n)
	rows := make(map[uuid.UUID]gen.PurchaseItemLockRow, len(ids))
	f := FieldErrors{}
	for _, id := range ids {
		row, err := q.PurchaseItemLock(ctx, gen.PurchaseItemLockParams{TenantID: a.TenantID, OutletID: a.OutletID, ID: id})
		if errors.Is(err, pgx.ErrNoRows) {
			checkItem(at[id], false, false, "", f)
			continue
		}
		if err != nil {
			return nil, nil, err
		}
		// Baca ulang HPP dengan pernyataan BARU setelah kunci didapat: pembacaan di PurchaseItemLock bisa memakai versi
		// item_outlet_costs sebelum pembelian lain commit (EvalPlanQual hanya memperbarui baris yang dikunci).
		cur, err := q.PurchaseItemState(ctx, gen.PurchaseItemStateParams{TenantID: a.TenantID, OutletID: a.OutletID, ID: id})
		if err != nil {
			return nil, nil, err
		}
		row.AvgCost = cur.AvgCost
		rows[id] = row
		checkItem(at[id], true, row.Active, row.Kind, f)
	}
	return rows, f, nil
}

// save menulis nota beserta stok, HPP, hutang, dan audit dalam transaksi pemanggil. rev = nil untuk pembelian baru;
// berisi untuk revisi (edit): nomor, id, dan tautan rantai diberikan pemanggil.
func (s *Service) save(ctx context.Context, tx pgx.Tx, a authz.Actor, n norm, am amounts, key, h string, rev *revision) (uuid.UUID, error) {
	q := gen.New(tx)
	if ex, err := q.PurchaseByIdemKey(ctx, gen.PurchaseByIdemKeyParams{TenantID: a.TenantID, IdempotencyKey: key}); err == nil {
		if ex.RequestHash != h {
			return uuid.Nil, ErrKeyMismatch
		}
		return uuid.Nil, replay{ex.ID}
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return uuid.Nil, err
	}

	out, err := q.PurchaseOutletInfo(ctx, gen.PurchaseOutletInfoParams{TenantID: a.TenantID, ID: a.OutletID})
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && !out.Active) {
		return uuid.Nil, ErrOutletInactive
	}
	if err != nil {
		return uuid.Nil, err
	}
	sup, err := q.PurchaseSupplierState(ctx, gen.PurchaseSupplierStateParams{TenantID: a.TenantID, ID: n.supplier})
	if errors.Is(err, pgx.ErrNoRows) {
		return uuid.Nil, FieldErrors{"supplier_id": sanitize.Invalid}
	}
	if err != nil {
		return uuid.Nil, err
	}
	if !sup.Active {
		return uuid.Nil, FieldErrors{"supplier_id": codeSupplierInact}
	}
	purchaseDate, due, fe := resolveDates(n, out.LocalDay.Time)
	if len(fe) > 0 {
		return uuid.Nil, fe
	}
	items, fe, err := lockItems(ctx, q, a, n)
	if err != nil {
		return uuid.Nil, err
	}
	if len(fe) > 0 {
		return uuid.Nil, fe
	}

	pid := uuid.New()
	docNo := ""
	if rev != nil {
		pid, docNo = rev.id, rev.docNo
	} else {
		no, err := q.PurchaseNextNo(ctx, gen.PurchaseNextNoParams{TenantID: a.TenantID, OutletID: a.OutletID, Day: out.LocalDay})
		if err != nil {
			return uuid.Nil, err
		}
		docNo = fmt.Sprintf("PB-%s-%s-%04d", strings.ToUpper(out.Code), out.LocalDay.Time.Format("060102"), no)
	}
	rootID, supersedes, number, reason := pgtype.UUID{}, pgtype.UUID{}, int32(1), ""
	if rev != nil {
		rootID, supersedes, number, reason = nullUUID(rev.rootID), nullUUID(rev.supersedes), rev.number, rev.reason
	}
	payType := "cash"
	if n.credit {
		payType = "credit"
	}
	var dueArg pgtype.Date
	if due != nil {
		dueArg = pgDate(*due)
	}
	_, err = q.PurchaseInsert(ctx, gen.PurchaseInsertParams{
		ID: pid, TenantID: a.TenantID, OutletID: a.OutletID, DocNo: docNo, IdempotencyKey: key, RequestHash: h,
		SupplierID: n.supplier, SupplierInvoiceNo: n.invoice, PurchaseDate: pgDate(purchaseDate), PaymentType: payType, DueDate: dueArg,
		Note: n.note, Subtotal: am.subtotal, TaxPct: am.taxPct, TaxAmount: am.tax, OtherCost: am.other, Total: am.total,
		CreatedBy: nullUUID(a.UserID),
		RootID:    rootID, Revision: number, SupersedesID: supersedes, RevisionReason: reason,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		// Pengiriman ganda bersamaan: yang lain menang. Batalkan transaksi ini (nomor tak terpakai) dan kembalikan nota itu.
		ex, e2 := q.PurchaseByIdemKey(ctx, gen.PurchaseByIdemKeyParams{TenantID: a.TenantID, IdempotencyKey: key})
		if e2 != nil {
			return uuid.Nil, e2
		}
		if ex.RequestHash != h {
			return uuid.Nil, ErrKeyMismatch
		}
		return uuid.Nil, replay{ex.ID}
	}
	var pe *pgconn.PgError
	if errors.As(err, &pe) && pe.Code == "23505" && pe.ConstraintName == invoiceUniqueIdx {
		return uuid.Nil, FieldErrors{"supplier_invoice_no": codeInvoiceDup}
	}
	if err != nil {
		return uuid.Nil, err
	}

	// Stok masuk (ledger). Satu movement per barang+bucket; ApplyAll mengunci terurut dan menjaga atomik.
	type bk struct {
		item   uuid.UUID
		bucket string
	}
	sums := map[bk]dec{}
	var order []bk
	add := func(item uuid.UUID, bucket string, qty dec) {
		if !qty.IsPositive() {
			return
		}
		k := bk{item, bucket}
		if _, ok := sums[k]; !ok {
			order = append(order, k)
		}
		sums[k] = sums[k].Add(qty)
	}
	totalQty := map[uuid.UUID]dec{}
	for _, l := range n.lines {
		add(l.itemID, stock.BucketDisplay, l.qtyDisp)
		add(l.itemID, stock.BucketWarehouse, l.qtyWare)
		totalQty[l.itemID] = totalQty[l.itemID].Add(l.qty)
	}
	moves := make([]stock.Movement, 0, len(order))
	for _, k := range order {
		moves = append(moves, stock.Movement{TenantID: a.TenantID, OutletID: a.OutletID, ItemID: k.item, Bucket: k.bucket, Delta: sums[k],
			RefType: stock.RefPurchase, RefID: pid, Note: docNo, ActorID: a.UserID})
	}
	if _, err := stock.ApplyAll(ctx, tx, moves); err != nil {
		return uuid.Nil, err
	}

	// HPP rata-rata tertimbang per cabang. Stok sebelum = stok cabang (semua bucket) SESUDAH movement dikurangi qty nota ini
	// (dibaca setelah baris saldo terkunci oleh movement; penguncian barang menyerialkan pembelian lain atas barang yang sama).
	state := make(map[uuid.UUID]itemCost, len(items))
	for id, row := range items {
		after, err := q.StockOutletQty(ctx, gen.StockOutletQtyParams{TenantID: a.TenantID, OutletID: a.OutletID, ItemID: id})
		if err != nil {
			return uuid.Nil, err
		}
		state[id] = itemCost{stock0: after.Sub(totalQty[id]), avg0: row.AvgCost}
	}
	steps, finalAvg, lastCost := runAverages(n, am, state)

	for i, l := range n.lines {
		row := items[l.itemID]
		if err := q.PurchaseLineInsert(ctx, gen.PurchaseLineInsertParams{
			TenantID: a.TenantID, PurchaseID: pid, Position: int32(i), ItemID: l.itemID, ItemSku: row.Sku, ItemName: row.Name, UnitName: row.UnitName,
			QtyDisplay: l.qtyDisp, QtyWarehouse: l.qtyWare, Qty: l.qty, UnitPrice: l.price,
			Disc1: l.disc[0], Disc2: l.disc[1], Disc3: l.disc[2], Disc4: l.disc[3],
			LineTotal: am.lines[i].total, CostAlloc: am.lines[i].alloc, UnitCost: am.lines[i].unitCost,
			StockBefore: steps[i].stockBefore, AvgBefore: steps[i].avgBefore, AvgAfter: steps[i].avgAfter,
		}); err != nil {
			return uuid.Nil, err
		}
	}
	for i, c := range n.costs {
		if err := q.PurchaseCostInsert(ctx, gen.PurchaseCostInsertParams{TenantID: a.TenantID, PurchaseID: pid, Position: int32(i), Name: c.name, Amount: c.amount}); err != nil {
			return uuid.Nil, err
		}
	}
	for id := range items {
		if err := q.StockSetCost(ctx, gen.StockSetCostParams{TenantID: a.TenantID, OutletID: a.OutletID, ItemID: id, AvgCost: finalAvg[id], LastCost: lastCost[id]}); err != nil {
			return uuid.Nil, err
		}
	}
	if n.credit && am.total.IsPositive() {
		if err := q.PayableInsert(ctx, gen.PayableInsertParams{TenantID: a.TenantID, OutletID: a.OutletID, SupplierID: n.supplier,
			PurchaseID: pid, Amount: am.total, DueDate: dueArg}); err != nil {
			return uuid.Nil, err
		}
	}
	action := audit.ActionPurchaseCreate
	details := map[string]any{"doc_no": docNo, "outlet_id": a.OutletID.String(), "supplier": sup.Name, "payment_type": payType,
		"lines": len(n.lines), "subtotal": am.subtotal.String(), "tax": am.tax.String(), "other_cost": am.other.String(), "total": am.total.String()}
	if rev != nil {
		action = audit.ActionPurchaseEdit
		details["revision"] = number
		details["supersedes_id"] = rev.supersedes.String()
		details["reason"] = rev.reason
	}
	err = audit.Record(ctx, tx, audit.FromActor(a), audit.Entry{
		Action: action, Entity: audit.EntityPurchase, EntityID: pid.String(), Details: details,
	})
	return pid, err
}

// ---- Quote (pratinjau, tanpa menyimpan apa pun) ----

type QuoteLine struct {
	ItemID      uuid.UUID `json:"item_id"`
	Qty         string    `json:"qty"`
	LineTotal   string    `json:"line_total"`
	CostAlloc   string    `json:"cost_alloc"`
	UnitCost    string    `json:"unit_cost"`
	StockBefore string    `json:"stock_before"`
	AvgBefore   string    `json:"avg_before"`
	AvgAfter    string    `json:"avg_after"`
}

type Quote struct {
	Lines    []QuoteLine `json:"lines"`
	Subtotal string      `json:"subtotal"`
	TaxPct   string      `json:"tax_pct"`
	Tax      string      `json:"tax_amount"`
	Other    string      `json:"other_cost"`
	Total    string      `json:"total"`
}

// Quote menghitung total, alokasi biaya lain, HPP baris dan HPP rata-rata baru dengan aturan yang sama dengan Create.
// Tidak mengunci/menulis apa pun; stok yang dipakai = stok cabang saat ini.
func (s *Service) Quote(ctx context.Context, a authz.Actor, in Request) (Quote, error) {
	n, f := normalize(in, false)
	if len(f) > 0 {
		return Quote{}, f
	}
	if len(n.lines) == 0 {
		return Quote{Lines: []QuoteLine{}, Subtotal: "0.00", TaxPct: n.taxPct.StringFixed(2), Tax: "0.00", Other: n.other.StringFixed(2), Total: n.other.StringFixed(2)}, nil
	}
	am, f := computeAmounts(n)
	if len(f) > 0 {
		return Quote{}, f
	}
	var qt Quote
	err := s.tx(ctx, a, func(tx pgx.Tx) error {
		q := gen.New(tx)
		ids, at := distinctItems(n)
		state := make(map[uuid.UUID]itemCost, len(ids))
		fe := FieldErrors{}
		for _, id := range ids {
			row, err := q.PurchaseItemState(ctx, gen.PurchaseItemStateParams{TenantID: a.TenantID, OutletID: a.OutletID, ID: id})
			if errors.Is(err, pgx.ErrNoRows) {
				checkItem(at[id], false, false, "", fe)
				continue
			}
			if err != nil {
				return err
			}
			checkItem(at[id], true, row.Active, row.Kind, fe)
			cur, err := q.StockOutletQty(ctx, gen.StockOutletQtyParams{TenantID: a.TenantID, OutletID: a.OutletID, ItemID: id})
			if err != nil {
				return err
			}
			state[id] = itemCost{stock0: cur, avg0: row.AvgCost}
		}
		if len(fe) > 0 {
			return fe
		}
		steps, _, _ := runAverages(n, am, state)
		qt = Quote{Subtotal: am.subtotal.StringFixed(2), TaxPct: am.taxPct.StringFixed(2), Tax: am.tax.StringFixed(2),
			Other: am.other.StringFixed(2), Total: am.total.StringFixed(2), Lines: make([]QuoteLine, len(n.lines))}
		for i, l := range n.lines {
			qt.Lines[i] = QuoteLine{ItemID: l.itemID, Qty: l.qty.String(), LineTotal: am.lines[i].total.StringFixed(2),
				CostAlloc: am.lines[i].alloc.StringFixed(2), UnitCost: am.lines[i].unitCost.StringFixed(2),
				StockBefore: steps[i].stockBefore.String(), AvgBefore: steps[i].avgBefore.StringFixed(2), AvgAfter: steps[i].avgAfter.StringFixed(2)}
		}
		return nil
	})
	return qt, err
}
