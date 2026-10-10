package sales

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"sort"
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
	listLimit   = 500
	maxListDays = 62
)

// ListRow = ringkasan satu nota pada daftar penjualan kasir.
type ListRow struct {
	ID         uuid.UUID         `json:"id"`
	DocNo      string            `json:"doc_no"`
	Status     string            `json:"status"`
	CreatedAt  time.Time         `json:"created_at"`
	Cashier    string            `json:"cashier"`
	Member     string            `json:"member,omitempty"`
	LineCount  int               `json:"line_count"`
	Total      string            `json:"total"`
	Surcharge  string            `json:"surcharge"`  // biaya metode yang ditagihkan ke pelanggan; ditagih = total + surcharge
	Receivable string            `json:"receivable"` // bagian nota yang dikreditkan (piutang member); bukan uang di laci
	Methods    map[string]string `json:"methods"`    // jenis → jumlah (tunai sudah bersih dari kembalian)
	Pays       []MethodAmount    `json:"pays"`       // per metode (id + nama sekarang)
}

// MethodAmount = jumlah per metode pembayaran (tunai bersih dari kembalian).
type MethodAmount struct {
	MethodID uuid.UUID `json:"method_id"`
	Name     string    `json:"name"`
	Kind     string    `json:"kind"`
	Amount   string    `json:"amount"`
}

// ListResult: Totals = jumlah per metode atas SELURUH baris yang dikembalikan; Truncated bila terpotong batas.
type ListResult struct {
	Data      []ListRow         `json:"data"`
	Total     string            `json:"total"`     // Σ total nota = omzet (tanpa biaya metode)
	Surcharge string            `json:"surcharge"` // Σ biaya metode yang ditagihkan ke pelanggan (bukan pendapatan)
	Received  string            `json:"received"`  // Total + Surcharge = Σ per metode (cocokkan dengan EDC/QRIS/laci)
	Totals    map[string]string `json:"totals"`    // per jenis
	ByMethod  []MethodAmount    `json:"by_method"` // per metode: Tunai dulu, lalu menurut nama
	// Flows = uang lain yang masuk/keluar lewat petugas ini di outlet aktif pada rentang yang sama (bayar piutang, top-up/tarik
	// deposit, dana kembali retur, bayar hutang, pencairan kredit pemasok), per sumber + metode; Amount bertanda (+ masuk).
	Flows []Flow `json:"flows"`
	// Drawer = uang per metode yang seharusnya ada (penjualan + Flows), tanpa saldo titipan (deposit/kredit pemasok).
	Drawer    []MethodAmount `json:"drawer"`
	From      string         `json:"from"`
	To        string         `json:"to"`
	Truncated bool           `json:"truncated"`
}

// Flow = jumlah satu sumber uang lain per metode.
type Flow struct {
	Source   string    `json:"source"` // receivable_payment | deposit_topup | deposit_withdraw | sale_return | payable_payment | purchase_return | supplier_credit_cashout
	MethodID uuid.UUID `json:"method_id"`
	Name     string    `json:"name"`
	Kind     string    `json:"kind"`
	Amount   string    `json:"amount"`
	Count    int       `json:"count"`
}

// flowsSQL: $1 tenant, $2 outlet, $3 semua petugas?, $4 petugas, $5..$6 rentang hari (zona waktu outlet). Metode saldo titipan
// (deposit/kredit pemasok) tidak dihitung karena bukan uang di laci.
const flowsSQL = `
WITH b AS (
  SELECT ($5::date)::timestamp AT TIME ZONE o.timezone AS t0, ($6::date + 1)::timestamp AT TIME ZONE o.timezone AS t1
  FROM outlets o WHERE o.tenant_id = $1 AND o.id = $2
), x AS (
  SELECT 'receivable_payment' AS src, rp.method_id, rp.method AS kind,
         rp.amount + CASE WHEN rp.fee_bearer = 'customer' THEN rp.fee_amount ELSE 0 END AS amt
  FROM receivable_payments rp, b
  WHERE rp.tenant_id = $1 AND rp.outlet_id = $2 AND ($3::bool OR rp.received_by = $4) AND rp.created_at >= b.t0 AND rp.created_at < b.t1
  UNION ALL
  SELECT CASE d.kind WHEN 'TOPUP' THEN 'deposit_topup' ELSE 'deposit_withdraw' END, d.method_id, d.method, d.amount
  FROM member_deposit_movements d, b
  WHERE d.tenant_id = $1 AND d.outlet_id = $2 AND d.method_id IS NOT NULL AND ($3::bool OR d.actor_id = $4) AND d.created_at >= b.t0 AND d.created_at < b.t1
  UNION ALL
  SELECT 'sale_return', r.refund_method_id, r.refund_method, -r.refund
  FROM sales_returns r, b
  WHERE r.tenant_id = $1 AND r.outlet_id = $2 AND r.status = 'completed' AND r.refund > 0 AND ($3::bool OR r.created_by = $4)
    AND r.created_at >= b.t0 AND r.created_at < b.t1
  UNION ALL
  SELECT 'payable_payment', pp.method_id, pp.method, -pp.amount
  FROM payable_payments pp, b
  WHERE pp.tenant_id = $1 AND pp.outlet_id = $2 AND ($3::bool OR pp.paid_by = $4) AND pp.created_at >= b.t0 AND pp.created_at < b.t1
  UNION ALL
  SELECT 'purchase_return', r.refund_method_id, r.refund_method, r.refund
  FROM purchase_returns r, b
  WHERE r.tenant_id = $1 AND r.outlet_id = $2 AND r.status = 'completed' AND r.refund > 0 AND ($3::bool OR r.created_by = $4)
    AND r.created_at >= b.t0 AND r.created_at < b.t1
  UNION ALL
  SELECT 'supplier_credit_cashout', c.method_id, c.method, -c.amount
  FROM supplier_credit_movements c, b
  WHERE c.tenant_id = $1 AND c.outlet_id = $2 AND c.kind = 'CASH_OUT' AND ($3::bool OR c.actor_id = $4) AND c.created_at >= b.t0 AND c.created_at < b.t1
)
SELECT x.src, x.method_id, pm.name, x.kind, sum(x.amt), count(*)
FROM x JOIN payment_methods pm ON pm.tenant_id = $1 AND pm.id = x.method_id
WHERE x.kind NOT IN ('deposit', 'supplier_credit')
GROUP BY x.src, x.method_id, pm.name, x.kind
ORDER BY x.src, (x.kind <> 'cash'), pm.name`

// cashierFlows menghitung Flows dan Drawer (penjualan per metode + arus lain).
func cashierFlows(ctx context.Context, tx pgx.Tx, a authz.Actor, fd, td time.Time, byMethod []MethodAmount) ([]Flow, []MethodAmount, error) {
	rows, err := tx.Query(ctx, flowsSQL, a.TenantID, a.OutletID, a.Impersonator != uuid.Nil, a.UserID,
		pgtype.Date{Time: fd, Valid: true}, pgtype.Date{Time: td, Valid: true})
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()
	flows := []Flow{}
	drawer := map[uuid.UUID]*MethodAmount{}
	amt := map[uuid.UUID]decimal.Decimal{}
	var order []uuid.UUID
	add := func(id uuid.UUID, name, kind string, v decimal.Decimal) {
		if kind == "deposit" || kind == "supplier_credit" {
			return
		}
		if _, ok := drawer[id]; !ok {
			drawer[id] = &MethodAmount{MethodID: id, Name: name, Kind: kind}
			order = append(order, id)
		}
		amt[id] = amt[id].Add(v)
	}
	for _, m := range byMethod {
		v, _ := decimal.NewFromString(m.Amount)
		add(m.MethodID, m.Name, m.Kind, v)
	}
	for rows.Next() {
		var f Flow
		var sum decimal.Decimal
		var n int64
		if err := rows.Scan(&f.Source, &f.MethodID, &f.Name, &f.Kind, &sum, &n); err != nil {
			return nil, nil, err
		}
		f.Amount, f.Count = sum.StringFixed(2), int(n)
		flows = append(flows, f)
		add(f.MethodID, f.Name, f.Kind, sum)
	}
	if err := rows.Err(); err != nil {
		return nil, nil, err
	}
	out := make([]MethodAmount, 0, len(order))
	for _, id := range order {
		m := drawer[id]
		m.Amount = amt[id].StringFixed(2)
		out = append(out, *m)
	}
	sort.SliceStable(out, func(i, j int) bool {
		ci, cj := out[i].Kind == "cash", out[j].Kind == "cash"
		if ci != cj {
			return ci
		}
		return strings.ToLower(out[i].Name) < strings.ToLower(out[j].Name)
	})
	return flows, out, nil
}

// List = nota outlet aktif milik KASIR YANG SEDANG LOGIN (untuk mencocokkan uang fisik di lacinya; kasir lain tak terlihat) pada rentang tanggal (zona waktu outlet; kosong = hari ini), terbaru dulu, opsional cari no. nota.
func (s *Service) List(ctx context.Context, a authz.Actor, from, to, q string) (ListResult, error) {
	res := ListResult{Data: []ListRow{}, Totals: map[string]string{}, ByMethod: []MethodAmount{}, Flows: []Flow{}, Drawer: []MethodAmount{}}
	err := db.WithTenant(ctx, s.pool, a.TenantID, func(tx pgx.Tx) error {
		qr := gen.New(tx)
		o, err := qr.SalesOutletInfo(ctx, gen.SalesOutletInfoParams{TenantID: a.TenantID, ID: a.OutletID})
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrOutletInactive
		}
		if err != nil {
			return err
		}
		today := o.LocalDay.Time
		fd, td := today, today
		fe := FieldErrors{}
		if from != "" {
			if fd, err = time.Parse("2006-01-02", from); err != nil {
				fe["from"] = "INVALID"
			}
		}
		if to != "" {
			if td, err = time.Parse("2006-01-02", to); err != nil {
				fe["to"] = "INVALID"
			}
		}
		if len(fe) == 0 && (td.Before(fd) || td.Sub(fd) > maxListDays*24*time.Hour) {
			fe["to"] = "INVALID"
		}
		if len(fe) > 0 {
			return fe
		}
		q = strings.TrimSpace(q)
		if len(q) > 60 {
			q = q[:60]
		}
		rows, err := qr.SalesList(ctx, gen.SalesListParams{TenantID: a.TenantID, OutletID: a.OutletID, CashierID: pgtype.UUID{Bytes: a.UserID, Valid: a.UserID != uuid.Nil}, AllCashiers: a.Impersonator != uuid.Nil, FromDay: pgtype.Date{Time: fd, Valid: true},
			ToDay: pgtype.Date{Time: td, Valid: true}, Q: q})
		if err != nil {
			return err
		}
		res.From, res.To = fd.Format("2006-01-02"), td.Format("2006-01-02")
		res.Truncated = len(rows) >= listLimit
		sums := map[string]decimal.Decimal{}
		perMethod := map[uuid.UUID]*MethodAmount{}
		perAmt := map[uuid.UUID]decimal.Decimal{}
		var order []uuid.UUID
		all, sur, credit := decimal.Zero, decimal.Zero, decimal.Zero
		for _, r := range rows {
			if r.Status != "completed" {
				// Nota batal tetap terlihat di daftar tapi tidak masuk hitungan uang (laci kasir).
				res.Data = append(res.Data, ListRow{ID: r.ID, DocNo: r.DocNo, Status: r.Status, CreatedAt: r.CreatedAt.Time, Cashier: r.CashierName,
					Member: r.MemberName, LineCount: int(r.LineCount), Total: r.Total.StringFixed(2), Surcharge: r.Surcharge.StringFixed(2), Receivable: r.Receivable.StringFixed(2), Methods: map[string]string{}, Pays: []MethodAmount{}})
				continue
			}
			methods := map[string]string{}
			pays := []MethodAmount{}
			var parts []struct {
				ID     uuid.UUID       `json:"id"`
				Name   string          `json:"name"`
				Kind   string          `json:"kind"`
				Amount decimal.Decimal `json:"amount"`
			}
			if err := json.Unmarshal([]byte(r.PayAmounts), &parts); err != nil {
				return err
			}
			for _, p := range parts {
				prev, _ := decimal.NewFromString(methods[p.Kind])
				methods[p.Kind] = prev.Add(p.Amount).StringFixed(2)
				sums[p.Kind] = sums[p.Kind].Add(p.Amount)
				pays = append(pays, MethodAmount{MethodID: p.ID, Name: p.Name, Kind: p.Kind, Amount: p.Amount.StringFixed(2)})
				if _, ok := perMethod[p.ID]; !ok {
					perMethod[p.ID] = &MethodAmount{MethodID: p.ID, Name: p.Name, Kind: p.Kind}
					order = append(order, p.ID)
				}
				perAmt[p.ID] = perAmt[p.ID].Add(p.Amount)
			}
			all, sur, credit = all.Add(r.Total), sur.Add(r.Surcharge), credit.Add(r.Receivable)
			res.Data = append(res.Data, ListRow{ID: r.ID, DocNo: r.DocNo, Status: r.Status, CreatedAt: r.CreatedAt.Time, Cashier: r.CashierName,
				Member: r.MemberName, LineCount: int(r.LineCount), Total: r.Total.StringFixed(2), Surcharge: r.Surcharge.StringFixed(2), Receivable: r.Receivable.StringFixed(2), Methods: methods, Pays: pays})
		}
		res.Total, res.Surcharge, res.Received = all.StringFixed(2), sur.StringFixed(2), all.Add(sur).StringFixed(2)
		for _, id := range order {
			m := perMethod[id]
			m.Amount = perAmt[id].StringFixed(2)
			res.ByMethod = append(res.ByMethod, *m)
		}
		sort.SliceStable(res.ByMethod, func(i, j int) bool {
			ci, cj := res.ByMethod[i].Kind == "cash", res.ByMethod[j].Kind == "cash"
			if ci != cj {
				return ci
			}
			return strings.ToLower(res.ByMethod[i].Name) < strings.ToLower(res.ByMethod[j].Name)
		})
		for m, v := range sums {
			res.Totals[m] = v.StringFixed(2)
		}
		if credit.IsPositive() {
			res.Totals["credit"] = credit.StringFixed(2) // piutang dari nota kredit (bukan metode bayar)
		}
		// Arus uang lain hanya relevan untuk laci, jadi tidak dihitung saat mencari nomor nota tertentu.
		if q == "" {
			res.Flows, res.Drawer, err = cashierFlows(ctx, tx, a, fd, td, res.ByMethod)
			return err
		}
		res.Drawer = res.ByMethod
		return nil
	})
	return res, err
}

// List: GET /sales/?from=YYYY-MM-DD&to=YYYY-MM-DD&q=<no. nota>.
func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	qs := r.URL.Query()
	res, err := h.svc.List(r.Context(), actor(r), qs.Get("from"), qs.Get("to"), qs.Get("q"))
	if err != nil {
		h.fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, res)
}
