package sales

import (
	"context"
	"encoding/base64"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/shopspring/decimal"

	"aciraba/internal/authz"
	gen "aciraba/internal/gen"
	"aciraba/internal/platform/db"
	"aciraba/internal/platform/httpx"
)

const (
	// ModuleList = izin menu Daftar Penjualan (semua kasir, outlet yang boleh diakses pemanggil).
	ModuleList = "sales_list"
	// ModuleCost = izin melihat HPP & laba penjualan. Tanpa izin ini kolom cost/profit TIDAK dikirim sama sekali.
	ModuleCost = "sales_cost"

	allDefaultLimit = 50
	allMaxLimit     = 200
	allMaxDays      = 366
)

var payMethods = map[string]bool{"cash": true, "debit": true, "credit_card": true, "ewallet": true, "transfer": true}

// AllParams = filter daftar penjualan lengkap. Semua string kosong = tidak difilter.
type AllParams struct {
	AllOutlets bool // true = semua outlet yang boleh diakses pemanggil; false = outlet aktif saja
	From, To   string
	Q          string
	Status     string // completed | void
	Method     string
	CashierID  string
	Cursor     string
	Limit      int
}

type OutletRef struct {
	ID   uuid.UUID `json:"id"`
	Code string    `json:"code"`
	Name string    `json:"name"`
}

// AllRow = satu nota pada daftar penjualan. Cost/Profit nil (tidak ikut JSON) bila pemanggil tak punya izin sales_cost.
type AllRow struct {
	ID        uuid.UUID `json:"id"`
	DocNo     string    `json:"doc_no"`
	Status    string    `json:"status"`
	CreatedAt time.Time `json:"created_at"`
	Outlet    OutletRef `json:"outlet"`
	Cashier   string    `json:"cashier"`
	Member    string    `json:"member,omitempty"`
	Salesman  string    `json:"salesperson,omitempty"`
	LineCount int       `json:"line_count"`
	Revision  int       `json:"revision"` // 1 = asli; >1 = hasil edit (nomor berakhiran -R<n>)

	Subtotal string `json:"subtotal"` // Σ baris setelah potongan baris
	// Rincian potongan: LineDiscount (per baris, sudah ada di Subtotal) + Discount (potongan di tingkat nota) =
	// ManualDiscount + VoucherAmount + RedeemAmount.
	LineDiscount   string   `json:"line_discount"`
	Discount       string   `json:"discount"`
	ManualDiscount string   `json:"manual_discount"`
	VoucherAmount  string   `json:"voucher_amount"`
	VoucherCodes   []string `json:"voucher_codes"`
	RedeemAmount   string   `json:"redeem_amount"`
	PointsRedeemed int      `json:"points_redeemed"`
	PointsEarned   int      `json:"points_earned"`
	PriceOverrides int      `json:"price_overrides"` // jumlah baris yang harganya diubah (disetujui PIN)

	TaxStore  string            `json:"tax_store"`
	TaxGov    string            `json:"tax_gov"`
	OtherCost string            `json:"other_cost"`
	Total     string            `json:"total"`
	Methods   map[string]string `json:"methods"`

	Cost   *string `json:"cost,omitempty"`   // Σ HPP × qty (snapshot saat transaksi)
	Profit *string `json:"profit,omitempty"` // Subtotal − Discount − Cost (sebelum pajak & biaya lain)
}

// AllSummary = ringkasan atas SELURUH hasil filter. Uang hanya dari nota completed.
type AllSummary struct {
	Count          int               `json:"count"`
	CompletedCount int               `json:"completed_count"`
	Total          string            `json:"total"`
	Discount       string            `json:"discount"` // potongan baris + potongan nota
	Methods        map[string]string `json:"methods"`
	// ByMethod = rincian per metode (master), Tunai dulu lalu menurut nama.
	ByMethod []MethodTotal `json:"by_method"`
	Cost     *string       `json:"cost,omitempty"`
	Profit   *string       `json:"profit,omitempty"`
}

// MethodTotal = jumlah satu metode pembayaran pada ringkasan (tunai sudah bersih dari kembalian).
type MethodTotal struct {
	MethodID  uuid.UUID `json:"method_id"`
	Name      string    `json:"name"`
	Kind      string    `json:"kind"`
	Amount    string    `json:"amount"`
	Fee       string    `json:"fee"`       // biaya metode (MDR) yang DITANGGUNG TOKO; bersih = Amount - Fee
	Surcharge string    `json:"surcharge"` // biaya yang ditagihkan ke PELANGGAN (tambahan di atas total nota)
}

type AllResult struct {
	Data       []AllRow   `json:"data"`
	Summary    AllSummary `json:"summary"`
	From       string     `json:"from"`
	To         string     `json:"to"`
	AllOutlets bool       `json:"all_outlets"`
	NextCursor string     `json:"next_cursor,omitempty"`
}

func escapeLike(s string) string {
	return strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(s)
}

func encodeCursor(t time.Time, id uuid.UUID) string {
	return base64.RawURLEncoding.EncodeToString([]byte(strconv.FormatInt(t.UnixMicro(), 10) + "|" + id.String()))
}

func decodeCursor(c string) (time.Time, uuid.UUID, bool) {
	raw, err := base64.RawURLEncoding.DecodeString(c)
	if err != nil {
		return time.Time{}, uuid.Nil, false
	}
	us, ids, ok := strings.Cut(string(raw), "|")
	if !ok {
		return time.Time{}, uuid.Nil, false
	}
	n, err := strconv.ParseInt(us, 10, 64)
	id, err2 := uuid.Parse(ids)
	if err != nil || err2 != nil {
		return time.Time{}, uuid.Nil, false
	}
	return time.UnixMicro(n).UTC(), id, true
}

// ListAll = nota SEMUA kasir pada outlet aktif (atau semua outlet yang boleh diakses), terbaru dulu, dengan rincian potongan.
// Tenant selalu dari token (+RLS); outlet dibatasi ke a.Outlets sehingga cabang di luar akses tidak pernah terbaca.
func (s *Service) ListAll(ctx context.Context, a authz.Actor, p AllParams) (AllResult, error) {
	res := AllResult{Data: []AllRow{}, AllOutlets: p.AllOutlets}
	canCost := a.Perms.Has(ModuleCost, authz.ActView)

	fe := FieldErrors{}
	status := strings.TrimSpace(p.Status)
	if status != "" && status != "completed" && status != "void" {
		fe["status"] = "INVALID"
	}
	method := strings.TrimSpace(p.Method)
	if method != "" && !payMethods[method] {
		fe["method"] = "INVALID"
	}
	cashier := uuid.Nil
	if p.CashierID != "" {
		id, err := uuid.Parse(p.CashierID)
		if err != nil || id == uuid.Nil {
			fe["cashier_id"] = "INVALID"
		}
		cashier = id
	}
	var cursorAt time.Time
	var cursorID uuid.UUID
	if p.Cursor != "" {
		var ok bool
		if cursorAt, cursorID, ok = decodeCursor(p.Cursor); !ok {
			fe["cursor"] = "INVALID"
		}
	}
	q := strings.TrimSpace(p.Q)
	if len(q) > 60 {
		q = q[:60]
	}
	limit := p.Limit
	if limit <= 0 {
		limit = allDefaultLimit
	}
	limit = min(limit, allMaxLimit)

	err := db.WithTenant(ctx, s.pool, a.TenantID, func(tx pgx.Tx) error {
		qr := gen.New(tx)
		o, err := qr.SalesOutletInfo(ctx, gen.SalesOutletInfoParams{TenantID: a.TenantID, ID: a.OutletID})
		if errors.Is(err, pgx.ErrNoRows) || (err == nil && !o.Active) {
			return ErrOutletInactive
		}
		if err != nil {
			return err
		}
		today := o.LocalDay.Time
		fd, td := today, today
		if p.From != "" {
			if fd, err = time.Parse("2006-01-02", p.From); err != nil {
				fe["from"] = "INVALID"
			}
		}
		if p.To != "" {
			if td, err = time.Parse("2006-01-02", p.To); err != nil {
				fe["to"] = "INVALID"
			}
		}
		if len(fe) == 0 && (td.Before(fd) || td.Sub(fd) > allMaxDays*24*time.Hour) {
			fe["to"] = "INVALID"
		}
		if len(fe) > 0 {
			return fe
		}

		outlets := []uuid.UUID{a.OutletID}
		if p.AllOutlets && len(a.Outlets) > 0 {
			outlets = outlets[:0]
			for id := range a.Outlets {
				outlets = append(outlets, id)
			}
		}
		from, to := pgtype.Date{Time: fd, Valid: true}, pgtype.Date{Time: td, Valid: true}
		like := escapeLike(q)
		res.From, res.To = fd.Format("2006-01-02"), td.Format("2006-01-02")

		rows, err := qr.SalesListAll(ctx, gen.SalesListAllParams{TenantID: a.TenantID, OutletIds: outlets, FromDay: from, ToDay: to,
			Status: status, CashierID: cashier, Method: method, Q: like,
			HasCursor: p.Cursor != "", CursorAt: pgtype.Timestamptz{Time: cursorAt, Valid: p.Cursor != ""}, CursorID: cursorID,
			PageLimit: int32(limit + 1)})
		if err != nil {
			return err
		}
		if len(rows) > limit {
			rows = rows[:limit]
			last := rows[len(rows)-1]
			res.NextCursor = encodeCursor(last.CreatedAt.Time, last.ID)
		}
		for _, r := range rows {
			res.Data = append(res.Data, allRow(r, canCost))
		}

		sum, err := qr.SalesListAllSummary(ctx, gen.SalesListAllSummaryParams{TenantID: a.TenantID, OutletIds: outlets, FromDay: from, ToDay: to,
			Status: status, CashierID: cashier, Method: method, Q: like})
		if err != nil {
			return err
		}
		res.Summary = AllSummary{Count: int(sum.SaleCount), CompletedCount: int(sum.CompletedCount), Total: sum.Total.StringFixed(2),
			Discount: sum.Discount.StringFixed(2), Methods: map[string]string{}, ByMethod: []MethodTotal{}}
		if canCost {
			cost, profit := sum.Cost.StringFixed(2), sum.NetSales.Sub(sum.Cost).StringFixed(2)
			res.Summary.Cost, res.Summary.Profit = &cost, &profit
		}
		ms, err := qr.SalesListAllMethodTotals(ctx, gen.SalesListAllMethodTotalsParams{TenantID: a.TenantID, OutletIds: outlets, FromDay: from, ToDay: to,
			CashierID: cashier, Method: method, Q: like})
		if err != nil {
			return err
		}
		for _, m := range ms {
			res.Summary.Methods[m.Method] = m.Amount.StringFixed(2)
		}
		bm, err := qr.SalesListAllMethodBreakdown(ctx, gen.SalesListAllMethodBreakdownParams{TenantID: a.TenantID, OutletIds: outlets, FromDay: from, ToDay: to,
			CashierID: cashier, Method: method, Q: like})
		if err != nil {
			return err
		}
		for _, m := range bm {
			res.Summary.ByMethod = append(res.Summary.ByMethod, MethodTotal{MethodID: m.MethodID, Name: m.Name, Kind: m.Kind, Amount: m.Amount.StringFixed(2), Fee: m.Fee.StringFixed(2), Surcharge: m.Surcharge.StringFixed(2)})
		}
		return nil
	})
	return res, err
}

func allRow(r gen.SalesListAllRow, canCost bool) AllRow {
	methods := map[string]string{}
	for _, part := range strings.Split(r.PayAmounts, ",") {
		m, v, ok := strings.Cut(part, ":")
		if !ok {
			continue
		}
		if d, err := decimal.NewFromString(v); err == nil {
			methods[m] = d.StringFixed(2)
		}
	}
	codes := []string{}
	if r.VoucherCodes != "" {
		codes = strings.Split(r.VoucherCodes, ",")
	}
	// Potongan nota = manual + kupon + tukar poin (ketiganya disimpan menyatu di sales.discount).
	manual := r.Discount.Sub(r.VoucherAmount).Sub(r.RedeemAmount)
	if manual.IsNegative() {
		manual = decimal.Zero
	}
	row := AllRow{ID: r.ID, DocNo: r.DocNo, Status: r.Status, CreatedAt: r.CreatedAt.Time,
		Outlet:  OutletRef{ID: r.OutletID, Code: r.OutletCode, Name: r.OutletName},
		Cashier: r.CashierName, Member: r.MemberName, Salesman: r.SalespersonName, LineCount: int(r.LineCount), Revision: int(r.Revision),
		Subtotal: r.Subtotal.StringFixed(2), LineDiscount: r.LineDiscount.StringFixed(2), Discount: r.Discount.StringFixed(2),
		ManualDiscount: manual.StringFixed(2), VoucherAmount: r.VoucherAmount.StringFixed(2), VoucherCodes: codes,
		RedeemAmount: r.RedeemAmount.StringFixed(2), PointsRedeemed: int(r.PointsRedeemed), PointsEarned: int(r.PointsEarned),
		PriceOverrides: int(r.OverrideCount),
		TaxStore:       r.TaxStore.StringFixed(2), TaxGov: r.TaxGov.StringFixed(2), OtherCost: r.OtherCost.StringFixed(2),
		Total: r.Total.StringFixed(2), Methods: methods}
	if canCost {
		cost, profit := r.Cost.StringFixed(2), r.Subtotal.Sub(r.Discount).Sub(r.Cost).StringFixed(2)
		row.Cost, row.Profit = &cost, &profit
	}
	return row
}

// ListAll: GET /sales/all?scope=all&from=&to=&q=&status=&method=&cashier_id=&cursor=&limit=
func (h *Handler) ListAll(w http.ResponseWriter, r *http.Request) {
	qs := r.URL.Query()
	limit, _ := strconv.Atoi(qs.Get("limit"))
	res, err := h.svc.ListAll(r.Context(), actor(r), AllParams{AllOutlets: qs.Get("scope") == "all", From: qs.Get("from"), To: qs.Get("to"),
		Q: qs.Get("q"), Status: qs.Get("status"), Method: qs.Get("method"), CashierID: qs.Get("cashier_id"), Cursor: qs.Get("cursor"), Limit: limit})
	if err != nil {
		h.fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, res)
}
