package sales

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/shopspring/decimal"

	"aciraba/internal/authz"
	gen "aciraba/internal/gen"
	"aciraba/internal/platform/db"
	"aciraba/internal/platform/httpx"
)

// DetailLine = satu baris nota lengkap: harga daftar → harga jual, potongan, dan (bila ada izin sales_cost) HPP & margin.
type DetailLine struct {
	Position      int     `json:"position"`
	ItemID        string  `json:"item_id"`
	SKU           string  `json:"sku"`
	Name          string  `json:"name"`
	Unit          string  `json:"unit"`
	Factor        string  `json:"factor"`   // satuan dasar per 1 satuan jual
	Qty           string  `json:"qty"`      // dalam satuan jual
	BaseQty       string  `json:"base_qty"` // Qty × Factor (yang memengaruhi stok)
	ListPrice     string  `json:"list_price"`
	UnitPrice     string  `json:"unit_price"`
	PriceOverride bool    `json:"price_override"`
	Discount      string  `json:"discount"` // potongan baris (rupiah, seluruh baris)
	LineTotal     string  `json:"line_total"`
	Note          string  `json:"note"`
	ReturnedQty   string  `json:"returned_qty"` // qty (satuan jual) yang sudah diretur lewat retur aktif; nota sendiri tidak berubah
	UnitCost      *string `json:"unit_cost,omitempty"`
	LineCost      *string `json:"line_cost,omitempty"`
	Profit        *string `json:"profit,omitempty"`
}

// StockMove = satu gerakan stok milik nota (ledger stock_movements).
type StockMove struct {
	ID           int64     `json:"id"`
	At           time.Time `json:"at"`
	Type         string    `json:"type"` // SALE | SALE_VOID | SALE_RETURN
	Bucket       string    `json:"bucket"`
	ItemID       string    `json:"item_id"`
	SKU          string    `json:"sku"`
	Name         string    `json:"name"`
	Unit         string    `json:"unit"`
	Delta        string    `json:"delta"`
	BalanceAfter string    `json:"balance_after"`
	Actor        string    `json:"actor"`
}

// Event = satu catatan audit nota (dibuat, harga/potongan disetujui PIN; edit/void nanti ikut di sini).
type Event struct {
	Action  string          `json:"action"`
	Actor   string          `json:"actor"`
	At      time.Time       `json:"at"`
	Details json.RawMessage `json:"details"`
}

// RevisionInfo = satu versi dalam rantai edit nota (asli + revisi).
type RevisionInfo struct {
	ID        uuid.UUID  `json:"id"`
	DocNo     string     `json:"doc_no"`
	Revision  int        `json:"revision"`
	Status    string     `json:"status"`
	CreatedAt time.Time  `json:"created_at"`
	RevisedAt *time.Time `json:"revised_at,omitempty"`
	Total     string     `json:"total"`
	Reason    string     `json:"reason,omitempty"`
}

// ReturnRef = satu dokumen retur penjualan yang merujuk nota ini (aktif maupun batal).
type ReturnRef struct {
	ID            uuid.UUID       `json:"id"`
	DocNo         string          `json:"doc_no"`
	ReturnDate    string          `json:"return_date"`
	CreatedAt     time.Time       `json:"created_at"`
	CreatedBy     string          `json:"created_by"`
	Status        string          `json:"status"` // completed | void
	VoidReason    string          `json:"void_reason,omitempty"`
	Total         string          `json:"total"`
	ReceivableCut string          `json:"receivable_cut"`
	Refund        string          `json:"refund"`
	RefundMethod  string          `json:"refund_method,omitempty"`
	Lines         []ReturnRefLine `json:"lines"`
}

type ReturnRefLine struct {
	SalePosition int    `json:"sale_position"`
	Name         string `json:"name"`
	Unit         string `json:"unit"`
	Qty          string `json:"qty"`
}

// Detail = nota lengkap untuk panel detail Daftar Penjualan. `Lines` di tingkat ini menggantikan Sale.Lines pada JSON
// (field paling dangkal menang), sehingga klien hanya melihat satu `lines` yang kaya.
type Detail struct {
	Sale
	Lines          []DetailLine   `json:"lines"`
	Outlet         OutletRef      `json:"outlet"`
	LineDiscount   string         `json:"line_discount"`
	ManualDiscount string         `json:"manual_discount"`
	VoucherAmount  string         `json:"voucher_amount"`
	BaseQtyTotal   string         `json:"base_qty_total"`
	Revisions      []RevisionInfo `json:"revisions"` // seluruh versi nota (asli + revisi), urut revisi
	Stock          []StockMove    `json:"stock"`
	Events         []Event        `json:"events"`
	Returns        []ReturnRef    `json:"returns"`        // dokumen retur yang merujuk nota ini, terbaru dulu
	ReturnedTotal  string         `json:"returned_total"` // Σ nilai retur aktif
	NetTotal       string         `json:"net_total"`      // total nota − retur aktif
	Cost           *string        `json:"cost,omitempty"`
	Profit         *string        `json:"profit,omitempty"` // Subtotal − Discount − HPP (sebelum pajak & biaya lain)
}

// Detail membaca satu nota lengkap. Tenant dari token (+RLS); nota di cabang yang tidak boleh diakses pemanggil = tidak ditemukan.
func (s *Service) Detail(ctx context.Context, a authz.Actor, id uuid.UUID) (Detail, error) {
	sale, err := s.Get(ctx, a, id)
	if err != nil {
		return Detail{}, err
	}
	if sale.OutletID != a.OutletID && !a.Outlets[sale.OutletID] {
		return Detail{}, ErrOutletForbidden
	}
	canCost := a.Perms.Has(ModuleCost, authz.ActView)
	out := Detail{Sale: sale, Lines: []DetailLine{}, Stock: []StockMove{}, Events: []Event{}, Revisions: []RevisionInfo{}, Returns: []ReturnRef{}}
	out.Sale.Lines = nil
	err = db.WithTenant(ctx, s.pool, a.TenantID, func(tx pgx.Tx) error {
		q := gen.New(tx)
		o, err := q.SalesOutletInfo(ctx, gen.SalesOutletInfoParams{TenantID: a.TenantID, ID: sale.OutletID})
		if err != nil {
			return err
		}
		out.Outlet = OutletRef{ID: sale.OutletID, Code: o.Code, Name: o.Name}

		ls, err := q.SalesLines(ctx, gen.SalesLinesParams{TenantID: a.TenantID, SaleID: id})
		if err != nil {
			return err
		}
		lineDisc, baseTotal, cost := decimal.Zero, decimal.Zero, decimal.Zero
		for i, l := range ls {
			base := l.Qty.Mul(l.Factor)
			lineCost := l.UnitCost.Mul(l.Qty)
			lineDisc, baseTotal, cost = lineDisc.Add(l.Discount), baseTotal.Add(base), cost.Add(lineCost)
			dl := DetailLine{Position: i + 1, ItemID: l.ItemID.String(), SKU: l.Sku, Name: l.Name, Unit: l.UnitName, Factor: l.Factor.String(),
				Qty: l.Qty.String(), BaseQty: base.String(), ListPrice: l.ListPrice.StringFixed(2), UnitPrice: l.UnitPrice.StringFixed(2),
				PriceOverride: l.PriceOverride, Discount: l.Discount.StringFixed(2), LineTotal: l.LineTotal.StringFixed(2), Note: l.Note}
			if canCost {
				uc, lc, pf := l.UnitCost.StringFixed(2), lineCost.StringFixed(2), l.LineTotal.Sub(lineCost).StringFixed(2)
				dl.UnitCost, dl.LineCost, dl.Profit = &uc, &lc, &pf
			}
			out.Lines = append(out.Lines, dl)
		}
		out.LineDiscount, out.BaseQtyTotal = lineDisc.StringFixed(2), baseTotal.String()

		returnIDs, err := loadDetailReturns(ctx, tx, a.TenantID, id, &out)
		if err != nil {
			return err
		}

		voucher := decimal.Zero
		for _, v := range sale.Vouchers {
			if d, err := decimal.NewFromString(v.Amount); err == nil {
				voucher = voucher.Add(d)
			}
		}
		discount, _ := decimal.NewFromString(sale.Discount)
		redeem, _ := decimal.NewFromString(sale.RedeemAmount)
		manual := discount.Sub(voucher).Sub(redeem)
		if manual.IsNegative() {
			manual = decimal.Zero
		}
		out.VoucherAmount, out.ManualDiscount = voucher.StringFixed(2), manual.StringFixed(2)
		if canCost {
			subtotal, _ := decimal.NewFromString(sale.Subtotal)
			c, p := cost.StringFixed(2), subtotal.Sub(discount).Sub(cost).StringFixed(2)
			out.Cost, out.Profit = &c, &p
		}

		chain, err := q.SalesRevisionChain(ctx, gen.SalesRevisionChainParams{TenantID: a.TenantID, RootID: pgtype.UUID{Bytes: sale.RootID, Valid: true}})
		if err != nil {
			return err
		}
		ids, sids := []uuid.UUID{id}, []string{id.String()}
		if len(chain) > 0 {
			ids, sids = ids[:0], sids[:0]
		}
		for _, c := range chain {
			ids, sids = append(ids, c.ID), append(sids, c.ID.String())
			ri := RevisionInfo{ID: c.ID, DocNo: c.DocNo, Revision: int(c.Revision), Status: c.Status, CreatedAt: c.CreatedAt.Time, Total: c.Total.StringFixed(2),
				Reason: c.RevisionReason.String}
			if c.VoidReason.Valid {
				ri.Reason = c.VoidReason.String
			}
			if c.RevisedAt.Valid {
				t := c.RevisedAt.Time
				ri.RevisedAt = &t
			}
			out.Revisions = append(out.Revisions, ri)
		}
		// Gerakan stok retur ber-ref_id dokumen retur (bukan nota), jadi ikut dicari agar tab Stok lengkap.
		ms, err := q.SalesStockMovements(ctx, gen.SalesStockMovementsParams{TenantID: a.TenantID, SaleIds: append(ids, returnIDs...)})
		if err != nil {
			return err
		}
		for _, m := range ms {
			out.Stock = append(out.Stock, StockMove{ID: m.ID, At: m.CreatedAt.Time, Type: m.RefType, Bucket: m.Bucket, ItemID: m.ItemID.String(), SKU: m.Sku,
				Name: m.Name, Unit: m.UnitName, Delta: m.QtyDelta.String(), BalanceAfter: m.BalanceAfter.String(), Actor: m.ActorName})
		}
		es, err := q.SalesAuditEvents(ctx, gen.SalesAuditEventsParams{TenantID: a.TenantID, SaleIds: sids})
		if err != nil {
			return err
		}
		for _, e := range es {
			out.Events = append(out.Events, Event{Action: e.Action, Actor: e.ActorName, At: e.CreatedAt.Time, Details: json.RawMessage(e.Details)})
		}
		return nil
	})
	return out, err
}

// loadDetailReturns mengisi dokumen retur nota, qty diretur per baris (retur aktif saja), total retur, dan nilai bersih.
// Mengembalikan id semua dokumen retur (termasuk yang batal) untuk dicari gerakan stoknya.
func loadDetailReturns(ctx context.Context, tx pgx.Tx, tenant, saleID uuid.UUID, out *Detail) ([]uuid.UUID, error) {
	rows, err := tx.Query(ctx, `SELECT r.id, r.doc_no, r.return_date, r.created_at, coalesce(u.name, ''), r.status, coalesce(r.void_reason, ''),
		r.total, r.receivable_cut, r.refund, r.refund_method_name
		FROM sales_returns r LEFT JOIN users u ON u.tenant_id = r.tenant_id AND u.id = r.created_by
		WHERE r.tenant_id = $1 AND r.sale_id = $2 ORDER BY r.created_at DESC, r.id DESC`, tenant, saleID)
	if err != nil {
		return nil, err
	}
	var ids []uuid.UUID
	index := map[uuid.UUID]int{}
	returned := decimal.Zero
	for rows.Next() {
		var ref ReturnRef
		var date pgtype.Date
		var total, cut, refund decimal.Decimal
		if err := rows.Scan(&ref.ID, &ref.DocNo, &date, &ref.CreatedAt, &ref.CreatedBy, &ref.Status, &ref.VoidReason, &total, &cut, &refund, &ref.RefundMethod); err != nil {
			rows.Close()
			return nil, err
		}
		ref.ReturnDate = date.Time.Format("2006-01-02")
		ref.Total, ref.ReceivableCut, ref.Refund, ref.Lines = total.StringFixed(2), cut.StringFixed(2), refund.StringFixed(2), []ReturnRefLine{}
		if ref.Status == "completed" {
			returned = returned.Add(total)
		}
		index[ref.ID] = len(out.Returns)
		ids = append(ids, ref.ID)
		out.Returns = append(out.Returns, ref)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	saleTotal, _ := decimal.NewFromString(out.Total)
	out.ReturnedTotal, out.NetTotal = returned.StringFixed(2), saleTotal.Sub(returned).StringFixed(2)
	for i := range out.Lines {
		out.Lines[i].ReturnedQty = "0"
	}
	if len(ids) == 0 {
		return nil, nil
	}

	// Posisi baris nota (urutan sama dengan SalesLines) → indeks DetailLine.
	posRows, err := tx.Query(ctx, `SELECT position FROM sale_lines WHERE tenant_id = $1 AND sale_id = $2 ORDER BY position`, tenant, saleID)
	if err != nil {
		return nil, err
	}
	lineAt := map[int]int{}
	for i := 0; posRows.Next(); i++ {
		var pos int32
		if err := posRows.Scan(&pos); err != nil {
			posRows.Close()
			return nil, err
		}
		lineAt[int(pos)] = i
	}
	posRows.Close()
	if err := posRows.Err(); err != nil {
		return nil, err
	}

	lineRows, err := tx.Query(ctx, `SELECT return_id, sale_position, item_name, unit_name, qty FROM sales_return_lines
		WHERE tenant_id = $1 AND return_id = ANY($2) ORDER BY return_id, position`, tenant, ids)
	if err != nil {
		return nil, err
	}
	defer lineRows.Close()
	returnedQty := map[int]decimal.Decimal{}
	for lineRows.Next() {
		var rid uuid.UUID
		var pos int32
		var l ReturnRefLine
		var q decimal.Decimal
		if err := lineRows.Scan(&rid, &pos, &l.Name, &l.Unit, &q); err != nil {
			return nil, err
		}
		l.SalePosition, l.Qty = int(pos), q.String()
		ref := &out.Returns[index[rid]]
		ref.Lines = append(ref.Lines, l)
		if i, ok := lineAt[int(pos)]; ok && ref.Status == "completed" {
			returnedQty[i] = returnedQty[i].Add(q)
		}
	}
	if err := lineRows.Err(); err != nil {
		return nil, err
	}
	for i, q := range returnedQty {
		out.Lines[i].ReturnedQty = q.String()
	}
	// sale_position di respons = nomor baris tampilan (Position DetailLine), bukan posisi internal tabel.
	for r := range out.Returns {
		for j := range out.Returns[r].Lines {
			if i, ok := lineAt[out.Returns[r].Lines[j].SalePosition]; ok {
				out.Returns[r].Lines[j].SalePosition = out.Lines[i].Position
			}
		}
	}
	return ids, nil
}

// Detail: GET /sales/{id}/detail
func (h *Handler) Detail(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		httpx.Error(w, http.StatusNotFound, "NOT_FOUND", "Data tidak ditemukan.")
		return
	}
	d, err := h.svc.Detail(r.Context(), actor(r), id)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, d)
}
