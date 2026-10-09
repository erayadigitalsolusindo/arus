package purchasing

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"aciraba/internal/authz"
	gen "aciraba/internal/gen"
)

type Line struct {
	ItemID       uuid.UUID `json:"item_id"`
	SKU          string    `json:"sku"`
	Name         string    `json:"name"`
	Unit         string    `json:"unit"`
	QtyDisplay   string    `json:"qty_display"`
	QtyWarehouse string    `json:"qty_warehouse"`
	Qty          string    `json:"qty"`
	UnitPrice    string    `json:"unit_price"`
	Discounts    []string  `json:"discounts"` // hanya tingkat yang terisi
	LineTotal    string    `json:"line_total"`
	CostAlloc    string    `json:"cost_alloc"`
	UnitCost     string    `json:"unit_cost"` // HPP baris per satuan dasar
	StockBefore  string    `json:"stock_before"`
	AvgBefore    string    `json:"avg_before"`
	AvgAfter     string    `json:"avg_after"`
}

type Cost struct {
	Name   string `json:"name"`
	Amount string `json:"amount"`
}

// PayableInfo = hutang yang lahir dari nota kredit (saldo = amount − pembayaran, Fase 6.3).
type PayableInfo struct {
	ID       uuid.UUID `json:"id"`
	Amount   string    `json:"amount"`
	DueDate  *string   `json:"due_date"`
	Paid     string    `json:"paid"`
	Returned string    `json:"returned"` // dipotong retur pembelian aktif
	Balance  string    `json:"balance"`
}

// Event = satu catatan audit nota (siapa, kapan, dan rinciannya).
type Event struct {
	Action  string          `json:"action"`
	Actor   string          `json:"actor"`
	At      time.Time       `json:"at"`
	Details json.RawMessage `json:"details"`
}

type Purchase struct {
	ID                uuid.UUID    `json:"id"`
	DocNo             string       `json:"doc_no"`
	OutletID          uuid.UUID    `json:"outlet_id"`
	OutletCode        string       `json:"outlet_code"`
	OutletName        string       `json:"outlet_name"`
	SupplierID        uuid.UUID    `json:"supplier_id"`
	SupplierName      string       `json:"supplier_name"`
	SupplierInvoiceNo string       `json:"supplier_invoice_no"`
	PurchaseDate      string       `json:"purchase_date"`
	PaymentType       string       `json:"payment_type"`
	DueDate           *string      `json:"due_date"`
	Status            string       `json:"status"`
	Note              string       `json:"note"`
	Subtotal          string       `json:"subtotal"`
	TaxPct            string       `json:"tax_pct"`
	TaxAmount         string       `json:"tax_amount"`
	OtherCost         string       `json:"other_cost"`
	Total             string       `json:"total"`
	CreatedAt         time.Time    `json:"created_at"`
	CreatedBy         string       `json:"created_by"`
	Lines             []Line       `json:"lines"`
	Costs             []Cost       `json:"costs"`
	Payable           *PayableInfo `json:"payable"`
	Events            []Event      `json:"events"`
	Revision          int          `json:"revision"`
	RevisionReason    string       `json:"revision_reason"`
	RevisedAt         *time.Time   `json:"revised_at"`
	SupersededBy      *uuid.UUID   `json:"superseded_by"`
	SupersedesID      *uuid.UUID   `json:"supersedes_id"`
	VoidReason        string       `json:"void_reason"`
	VoidedAt          *time.Time   `json:"voided_at"`
	Returns           []ReturnRef  `json:"returns"`
}

// ReturnRef = dokumen retur pembelian atas nota ini (aktif maupun batal).
type ReturnRef struct {
	ID        uuid.UUID `json:"id"`
	DocNo     string    `json:"doc_no"`
	Status    string    `json:"status"`
	Total     string    `json:"total"`
	CreatedAt time.Time `json:"created_at"`
}

func timePtr(t pgtype.Timestamptz) *time.Time {
	if !t.Valid {
		return nil
	}
	v := t.Time
	return &v
}

func uuidPtr(u pgtype.UUID) *uuid.UUID {
	if !u.Valid {
		return nil
	}
	v := uuid.UUID(u.Bytes)
	return &v
}

func dateStr(d pgtype.Date) *string {
	if !d.Valid {
		return nil
	}
	s := d.Time.Format("2006-01-02")
	return &s
}

func canAccessOutlet(a authz.Actor, outlet uuid.UUID) bool {
	return a.OutletID == outlet || a.Outlets[outlet]
}

// Get membaca satu nota lengkap. Nota di cabang di luar akses pemanggil ditolak (ErrOutletForbidden); lintas tenant = tidak ada (RLS).
func (s *Service) Get(ctx context.Context, a authz.Actor, id uuid.UUID) (Purchase, error) {
	var p Purchase
	err := s.tx(ctx, a, func(tx pgx.Tx) error {
		q := gen.New(tx)
		r, err := q.PurchaseGet(ctx, gen.PurchaseGetParams{TenantID: a.TenantID, ID: id})
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNotFound
		}
		if err != nil {
			return err
		}
		if !canAccessOutlet(a, r.OutletID) {
			return ErrOutletForbidden
		}
		p = Purchase{ID: r.ID, DocNo: r.DocNo, OutletID: r.OutletID, OutletCode: r.OutletCode, OutletName: r.OutletName,
			SupplierID: r.SupplierID, SupplierName: r.SupplierName, SupplierInvoiceNo: r.SupplierInvoiceNo,
			PurchaseDate: r.PurchaseDate.Time.Format("2006-01-02"), PaymentType: r.PaymentType, DueDate: dateStr(r.DueDate),
			Status: r.Status, Note: r.Note, Subtotal: r.Subtotal.StringFixed(2), TaxPct: r.TaxPct.StringFixed(2),
			TaxAmount: r.TaxAmount.StringFixed(2), OtherCost: r.OtherCost.StringFixed(2), Total: r.Total.StringFixed(2),
			CreatedAt: r.CreatedAt.Time, CreatedBy: r.CreatedName, Lines: []Line{}, Costs: []Cost{}, Events: []Event{},
			Revision: int(r.Revision), RevisionReason: r.RevisionReason, RevisedAt: timePtr(r.RevisedAt), SupersededBy: uuidPtr(r.SupersededBy),
			SupersedesID: uuidPtr(r.SupersedesID), VoidReason: r.VoidReason, VoidedAt: timePtr(r.VoidedAt)}
		lines, err := q.PurchaseLines(ctx, gen.PurchaseLinesParams{TenantID: a.TenantID, PurchaseID: id})
		if err != nil {
			return err
		}
		for _, l := range lines {
			ds := []string{}
			for _, d := range []dec{l.Disc1, l.Disc2, l.Disc3, l.Disc4} {
				if d.IsPositive() {
					ds = append(ds, d.StringFixed(2))
				}
			}
			p.Lines = append(p.Lines, Line{ItemID: l.ItemID, SKU: l.ItemSku, Name: l.ItemName, Unit: l.UnitName,
				QtyDisplay: l.QtyDisplay.String(), QtyWarehouse: l.QtyWarehouse.String(), Qty: l.Qty.String(),
				UnitPrice: priceString(l.UnitPrice), Discounts: ds, LineTotal: l.LineTotal.StringFixed(2),
				CostAlloc: l.CostAlloc.StringFixed(2), UnitCost: l.UnitCost.StringFixed(2),
				StockBefore: l.StockBefore.String(), AvgBefore: l.AvgBefore.StringFixed(2), AvgAfter: l.AvgAfter.StringFixed(2)})
		}
		costs, err := q.PurchaseCosts(ctx, gen.PurchaseCostsParams{TenantID: a.TenantID, PurchaseID: id})
		if err != nil {
			return err
		}
		for _, c := range costs {
			p.Costs = append(p.Costs, Cost{Name: c.Name, Amount: c.Amount.StringFixed(2)})
		}
		pay, err := q.PurchasePayable(ctx, gen.PurchasePayableParams{TenantID: a.TenantID, PurchaseID: id})
		if err == nil {
			var paid, returned dec
			if err := tx.QueryRow(ctx, `SELECT coalesce(sum(amount), 0) FROM payable_payments WHERE tenant_id = $1 AND payable_id = $2`, a.TenantID, pay.ID).Scan(&paid); err != nil {
				return err
			}
			if err := tx.QueryRow(ctx, `SELECT coalesce(sum(payable_cut), 0) FROM purchase_returns WHERE tenant_id = $1 AND purchase_id = $2 AND status = 'completed'`, a.TenantID, id).Scan(&returned); err != nil {
				return err
			}
			p.Payable = &PayableInfo{ID: pay.ID, Amount: pay.Amount.StringFixed(2), DueDate: dateStr(pay.DueDate), Paid: paid.StringFixed(2),
				Returned: returned.StringFixed(2), Balance: pay.Amount.Sub(paid).Sub(returned).StringFixed(2)}
		} else if !errors.Is(err, pgx.ErrNoRows) {
			return err
		}
		p.Returns = []ReturnRef{}
		rrows, err := tx.Query(ctx, `SELECT id, doc_no, status, total, created_at FROM purchase_returns
			WHERE tenant_id = $1 AND purchase_id = $2 ORDER BY created_at, id`, a.TenantID, id)
		if err != nil {
			return err
		}
		for rrows.Next() {
			var r ReturnRef
			var total dec
			if err := rrows.Scan(&r.ID, &r.DocNo, &r.Status, &total, &r.CreatedAt); err != nil {
				rrows.Close()
				return err
			}
			r.Total = total.StringFixed(2)
			p.Returns = append(p.Returns, r)
		}
		rrows.Close()
		if err := rrows.Err(); err != nil {
			return err
		}
		evs, err := q.PurchaseAuditEvents(ctx, gen.PurchaseAuditEventsParams{TenantID: a.TenantID, PurchaseID: id})
		if err != nil {
			return err
		}
		for _, e := range evs {
			p.Events = append(p.Events, Event{Action: e.Action, Actor: e.ActorName, At: e.CreatedAt.Time, Details: json.RawMessage(e.Details)})
		}
		return nil
	})
	return p, err
}

// ---- Daftar ----

type ListParams struct {
	From, To    string // YYYY-MM-DD menurut tanggal pembelian; kosong = 30 hari terakhir
	SupplierID  *uuid.UUID
	PaymentType string
	Q           string
	Limit       int
	Cursor      string // dari next_cursor halaman sebelumnya; kosong = halaman pertama
}

type Row struct {
	ID                uuid.UUID `json:"id"`
	DocNo             string    `json:"doc_no"`
	SupplierID        uuid.UUID `json:"supplier_id"`
	SupplierName      string    `json:"supplier_name"`
	SupplierInvoiceNo string    `json:"supplier_invoice_no"`
	PurchaseDate      string    `json:"purchase_date"`
	PaymentType       string    `json:"payment_type"`
	DueDate           *string   `json:"due_date"`
	Status            string    `json:"status"`
	Total             string    `json:"total"`
	Lines             int       `json:"lines"`
	CreatedAt         time.Time `json:"created_at"`
	CreatedBy         string    `json:"created_by"`
}

type Summary struct {
	Count       int    `json:"count"`
	Total       string `json:"total"`
	CreditTotal string `json:"credit_total"`
}

type ListResult struct {
	Data       []Row   `json:"data"`
	HasMore    bool    `json:"has_more"`
	NextCursor string  `json:"next_cursor"`
	Summary    Summary `json:"summary"`
}

// cursor = base64url("YYYY-MM-DD|RFC3339Nano|uuid") dari baris terakhir halaman. Format tak valid = ditolak (INVALID).
func encodeCursor(date string, at time.Time, id uuid.UUID) string {
	raw := date + "|" + at.UTC().Format(time.RFC3339Nano) + "|" + id.String()
	return base64.RawURLEncoding.EncodeToString([]byte(raw))
}

func decodeCursor(c string) (date time.Time, at time.Time, id uuid.UUID, ok bool) {
	b, err := base64.RawURLEncoding.DecodeString(c)
	if err != nil {
		return time.Time{}, time.Time{}, uuid.Nil, false
	}
	parts := strings.Split(string(b), "|")
	if len(parts) != 3 {
		return time.Time{}, time.Time{}, uuid.Nil, false
	}
	if date, err = time.Parse("2006-01-02", parts[0]); err != nil {
		return time.Time{}, time.Time{}, uuid.Nil, false
	}
	if at, err = time.Parse(time.RFC3339Nano, parts[1]); err != nil {
		return time.Time{}, time.Time{}, uuid.Nil, false
	}
	if id, err = uuid.Parse(parts[2]); err != nil {
		return time.Time{}, time.Time{}, uuid.Nil, false
	}
	return date, at, id, true
}

const maxRangeDays = 366

func escapeLike(s string) string {
	out := make([]rune, 0, len(s))
	for _, r := range s {
		if r == '\\' || r == '%' || r == '_' {
			out = append(out, '\\')
		}
		out = append(out, r)
	}
	return string(out)
}

// List = nota pembelian di cabang aktif sesi pada rentang tanggal pembelian.
func (s *Service) List(ctx context.Context, a authz.Actor, p ListParams) (ListResult, error) {
	f := FieldErrors{}
	var from, to time.Time
	var ok bool
	if p.To == "" && p.From == "" {
		now := time.Now().UTC()
		to = time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
		from = to.AddDate(0, 0, -30)
		to = to.AddDate(0, 0, 1) // toleransi hari lokal yang mendahului UTC
	} else {
		if from, ok = parseDate(p.From); !ok {
			f["from"] = "INVALID"
		}
		if to, ok = parseDate(p.To); !ok {
			f["to"] = "INVALID"
		}
	}
	if len(f) == 0 && (to.Before(from) || to.Sub(from) > maxRangeDays*24*time.Hour) {
		f["to"] = "INVALID"
	}
	if p.PaymentType != "" && p.PaymentType != "cash" && p.PaymentType != "credit" {
		f["payment_type"] = "INVALID"
	}
	var cur struct {
		on   bool
		date time.Time
		at   time.Time
		id   uuid.UUID
	}
	if p.Cursor != "" {
		d, at, id, ok := decodeCursor(p.Cursor)
		if !ok {
			f["cursor"] = "INVALID"
		}
		cur.on, cur.date, cur.at, cur.id = true, d, at, id
	}
	if len(f) > 0 {
		return ListResult{}, f
	}
	limit := p.Limit
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	res := ListResult{Data: []Row{}}
	err := s.tx(ctx, a, func(tx pgx.Tx) error {
		q := gen.New(tx)
		var sup pgtype.UUID
		if p.SupplierID != nil {
			sup = pgtype.UUID{Bytes: *p.SupplierID, Valid: true}
		}
		qs := escapeLike(p.Q)
		params := gen.PurchaseListParams{TenantID: a.TenantID, OutletID: a.OutletID, FromDate: pgDate(from), ToDate: pgDate(to),
			SupplierID: sup, PaymentType: p.PaymentType, Q: qs, PageLimit: int32(limit + 1)}
		if cur.on {
			params.HasCursor, params.CurDate, params.CurAt, params.CurID = true, pgDate(cur.date),
				pgtype.Timestamptz{Time: cur.at, Valid: true}, cur.id
		}
		rows, err := q.PurchaseList(ctx, params)
		if err != nil {
			return err
		}
		// Satu baris ekstra menandakan masih ada halaman berikut; baris itu tidak ditampilkan.
		if len(rows) > limit {
			rows = rows[:limit]
			res.HasMore = true
		}
		for _, r := range rows {
			res.Data = append(res.Data, Row{ID: r.ID, DocNo: r.DocNo, SupplierID: r.SupplierID, SupplierName: r.SupplierName,
				SupplierInvoiceNo: r.SupplierInvoiceNo, PurchaseDate: r.PurchaseDate.Time.Format("2006-01-02"), PaymentType: r.PaymentType,
				DueDate: dateStr(r.DueDate), Status: r.Status, Total: r.Total.StringFixed(2), Lines: int(r.LineCount),
				CreatedAt: r.CreatedAt.Time, CreatedBy: r.CreatedName})
		}
		if res.HasMore {
			last := rows[len(rows)-1]
			res.NextCursor = encodeCursor(last.PurchaseDate.Time.Format("2006-01-02"), last.CreatedAt.Time, last.ID)
		}
		sm, err := q.PurchaseListSummary(ctx, gen.PurchaseListSummaryParams{TenantID: a.TenantID, OutletID: a.OutletID, FromDate: pgDate(from), ToDate: pgDate(to),
			SupplierID: sup, PaymentType: p.PaymentType, Q: qs})
		if err != nil {
			return err
		}
		res.Summary = Summary{Count: int(sm.Cnt), Total: sm.Total.StringFixed(2), CreditTotal: sm.CreditTotal.StringFixed(2)}
		return nil
	})
	return res, err
}

// ---- Pemilih barang ----

type ItemChoice struct {
	ID         uuid.UUID `json:"id"`
	SKU        string    `json:"sku"`
	Barcode    string    `json:"barcode"`
	Name       string    `json:"name"`
	Unit       string    `json:"unit"`
	AvgCost    string    `json:"avg_cost"`
	LastCost   string    `json:"last_cost"`
	StockTotal string    `json:"stock_total"`
}

// Items = pencarian barang berstok aktif untuk form pembelian: stok total dan HPP di cabang aktif (maks 20).
func (s *Service) Items(ctx context.Context, a authz.Actor, q string) ([]ItemChoice, error) {
	out := []ItemChoice{}
	err := s.tx(ctx, a, func(tx pgx.Tx) error {
		rows, err := gen.New(tx).PurchaseItemSearch(ctx, gen.PurchaseItemSearchParams{TenantID: a.TenantID, OutletID: a.OutletID, Q: escapeLike(q), PageLimit: 20})
		if err != nil {
			return err
		}
		for _, r := range rows {
			out = append(out, ItemChoice{ID: r.ID, SKU: r.Sku, Barcode: r.Barcode, Name: r.Name, Unit: r.UnitName,
				AvgCost: r.AvgCost.StringFixed(2), LastCost: r.LastCost.StringFixed(2), StockTotal: r.StockTotal.String()})
		}
		return nil
	})
	return out, err
}

// priceString: harga beli dengan minimal 2 desimal dan maksimal 4 (angka nol di ujung setelah desimal ke-2 dibuang).
func priceString(d dec) string {
	s := d.StringFixed(4)
	for strings.HasSuffix(s, "0") && len(s)-strings.Index(s, ".") > 3 {
		s = s[:len(s)-1]
	}
	return s
}
