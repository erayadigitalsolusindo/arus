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
	out := Detail{Sale: sale, Lines: []DetailLine{}, Stock: []StockMove{}, Events: []Event{}, Revisions: []RevisionInfo{}}
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
		ms, err := q.SalesStockMovements(ctx, gen.SalesStockMovementsParams{TenantID: a.TenantID, SaleIds: ids})
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
