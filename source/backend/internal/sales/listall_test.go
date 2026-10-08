package sales

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/shopspring/decimal"

	"aciraba/internal/authz"
	"aciraba/internal/platform/db"
	"aciraba/internal/stock"
)

// Daftar penjualan lengkap: semua kasir, pemisahan per cabang & tenant, rincian potongan, HPP hanya untuk pemegang sales_cost.
func TestListAllScopesDiscountsAndCost(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	it := e.item(t, "goods", "100000", "30000", 20, false)
	e.newVoucher(t, "HEMAT10", "percent", "10", "15000", "0", "NULL", "NULL", nil, true)
	e.newVoucher(t, "POTONG5", "amount", "5000", "", "100000", "NULL", "NULL", nil, true)

	// Cabang kedua + stok di sana + kasir kedua.
	outlet2, userB := uuid.New(), uuid.New()
	var role uuid.UUID
	if err := e.admin.QueryRow(ctx, `SELECT id FROM roles WHERE tenant_id = $1 LIMIT 1`, e.tenant).Scan(&role); err != nil {
		t.Fatal(err)
	}
	e.exec(t, `INSERT INTO outlets (id, tenant_id, code, name) VALUES ($1, $2, 'cab2', 'Cabang Dua')`, outlet2, e.tenant)
	e.exec(t, `INSERT INTO users (id, tenant_id, role_id, email, name, password_hash) VALUES ($1, $2, $3, $4, 'Kasir B', 'x')`, userB, e.tenant, role, "b-"+userB.String()+"@example.test")
	err := db.WithTenant(ctx, e.app, e.tenant, func(tx pgx.Tx) error {
		_, err := stock.Apply(ctx, tx, stock.Movement{TenantID: e.tenant, OutletID: outlet2, ItemID: it, Bucket: stock.BucketDisplay,
			Delta: decimal.NewFromInt(5), RefType: stock.RefOpening})
		return err
	})
	if err != nil {
		t.Fatal(err)
	}

	a := e.actor(e.tenant)
	a.Outlets = map[uuid.UUID]bool{e.outlet: true, outlet2: true}
	b := a
	b.UserID = userB
	a2 := a
	a2.OutletID = outlet2

	if _, _, err := e.svc.Create(ctx, a, key(), Request{Lines: []LineIn{line(it, "2")}, VoucherCodes: []string{"HEMAT10", "POTONG5"}, Payments: []PaymentIn{pay("cash", "200000")}}); err != nil {
		t.Fatal(err)
	}
	if _, _, err := e.svc.Create(ctx, b, key(), Request{Lines: []LineIn{line(it, "1")}, Discount: "1000", Payments: []PaymentIn{pay("debit", "99000")}}); err != nil {
		t.Fatal(err)
	}
	if _, _, err := e.svc.Create(ctx, a2, key(), Request{Lines: []LineIn{line(it, "1")}, Payments: []PaymentIn{pay("cash", "100000")}}); err != nil {
		t.Fatal(err)
	}

	// Tanpa izin HPP: semua kasir terlihat di cabang aktif, kolom modal tidak ada.
	res, err := e.svc.ListAll(ctx, a, AllParams{})
	if err != nil || len(res.Data) != 2 || res.Summary.Count != 2 || res.Summary.Cost != nil || res.Summary.Profit != nil {
		t.Fatalf("cabang aktif: %+v %v", res, err)
	}
	for _, r := range res.Data {
		if r.Cost != nil || r.Profit != nil || r.Outlet.ID != e.outlet {
			t.Fatalf("HPP bocor atau cabang salah: %+v", r)
		}
	}
	// Rincian potongan nota pertama (urutan terbaru dulu → baris kedua).
	first, second := res.Data[1], res.Data[0]
	if first.Discount != "20000.00" || first.VoucherAmount != "20000.00" || first.ManualDiscount != "0.00" || len(first.VoucherCodes) != 2 ||
		first.Cashier != "Kasir Uji" || first.Subtotal != "200000.00" || first.Methods["cash"] != "180000.00" {
		t.Fatalf("nota kupon: %+v", first)
	}
	if second.Discount != "1000.00" || second.ManualDiscount != "1000.00" || second.VoucherAmount != "0.00" || second.Cashier != "Kasir B" || second.Methods["debit"] != "99000.00" {
		t.Fatalf("nota potongan manual: %+v", second)
	}
	if res.Summary.Total != "279000.00" || res.Summary.Discount != "21000.00" || res.Summary.Methods["cash"] != "180000.00" {
		t.Fatalf("ringkasan: %+v", res.Summary)
	}

	// Dengan izin HPP: modal & laba = subtotal − potongan − HPP.
	withCost := a
	withCost.Perms = authz.Permissions{Grants: map[string][]string{"sales_cost": {"view"}}}
	rc, err := e.svc.ListAll(ctx, withCost, AllParams{})
	if err != nil || rc.Data[1].Cost == nil || *rc.Data[1].Cost != "60000.00" || *rc.Data[1].Profit != "120000.00" ||
		rc.Summary.Cost == nil || *rc.Summary.Cost != "90000.00" || *rc.Summary.Profit != "189000.00" {
		t.Fatalf("hpp/laba: %+v %v", rc, err)
	}

	// Semua cabang = hanya cabang yang boleh diakses pemanggil.
	all, err := e.svc.ListAll(ctx, a, AllParams{AllOutlets: true})
	if err != nil || len(all.Data) != 3 || all.Summary.Count != 3 || !all.AllOutlets {
		t.Fatalf("semua cabang: %+v %v", all, err)
	}
	limited := a
	limited.Outlets = map[uuid.UUID]bool{e.outlet: true}
	if lim, err := e.svc.ListAll(ctx, limited, AllParams{AllOutlets: true}); err != nil || len(lim.Data) != 2 {
		t.Fatalf("cabang di luar akses harus tak terlihat: %+v %v", lim, err)
	}
	if o2, err := e.svc.ListAll(ctx, a2, AllParams{}); err != nil || len(o2.Data) != 1 || o2.Data[0].Outlet.Name != "Cabang Dua" {
		t.Fatalf("cabang kedua: %+v %v", o2, err)
	}

	// Filter kasir / metode / cari / status.
	if r, err := e.svc.ListAll(ctx, a, AllParams{CashierID: userB.String()}); err != nil || len(r.Data) != 1 || r.Data[0].Cashier != "Kasir B" {
		t.Fatalf("filter kasir: %+v %v", r, err)
	}
	if r, err := e.svc.ListAll(ctx, a, AllParams{Method: "debit"}); err != nil || len(r.Data) != 1 || r.Summary.Total != "99000.00" {
		t.Fatalf("filter metode: %+v %v", r, err)
	}
	if r, err := e.svc.ListAll(ctx, a, AllParams{Q: "kasir b"}); err != nil || len(r.Data) != 1 {
		t.Fatalf("cari kasir: %+v %v", r, err)
	}
	if r, err := e.svc.ListAll(ctx, a, AllParams{Q: "%"}); err != nil || len(r.Data) != 0 {
		t.Fatalf("wildcard harus di-escape: %+v %v", r, err)
	}
	if r, err := e.svc.ListAll(ctx, a, AllParams{Status: "void"}); err != nil || len(r.Data) != 0 || r.Summary.Count != 0 {
		t.Fatalf("filter status: %+v %v", r, err)
	}

	// Paginasi keyset: 3 nota, 2 per halaman, tanpa duplikat.
	p1, err := e.svc.ListAll(ctx, a, AllParams{AllOutlets: true, Limit: 2})
	if err != nil || len(p1.Data) != 2 || p1.NextCursor == "" || p1.Summary.Count != 3 {
		t.Fatalf("halaman 1: %+v %v", p1, err)
	}
	p2, err := e.svc.ListAll(ctx, a, AllParams{AllOutlets: true, Limit: 2, Cursor: p1.NextCursor})
	if err != nil || len(p2.Data) != 1 || p2.NextCursor != "" || p2.Data[0].ID == p1.Data[0].ID || p2.Data[0].ID == p1.Data[1].ID {
		t.Fatalf("halaman 2: %+v %v", p2, err)
	}

	// Validasi & tenant lain.
	var fe FieldErrors
	for _, p := range []AllParams{{Status: "x"}, {Method: "x"}, {CashierID: "x"}, {Cursor: "x"}, {From: "2026-13-40"}, {From: "2020-01-01", To: "2026-12-31"}} {
		if _, err := e.svc.ListAll(ctx, a, p); !errors.As(err, &fe) {
			t.Fatalf("param %+v harus ditolak: %v", p, err)
		}
	}
	if _, err := e.svc.ListAll(ctx, e.actor(e.other), AllParams{AllOutlets: true}); !errors.Is(err, ErrOutletInactive) {
		t.Fatalf("tenant lain: %v", err)
	}
}

// Detail nota: baris lengkap (HPP hanya dengan izin), kupon, gerakan stok, riwayat audit, dan batas cabang/tenant.
func TestDetailLinesStockEventsAndScope(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	it := e.item(t, "goods", "100000", "30000", 20, false)
	e.newVoucher(t, "HEMAT10", "percent", "10", "15000", "0", "NULL", "NULL", nil, true)
	a := e.actor(e.tenant)
	a.Outlets = map[uuid.UUID]bool{e.outlet: true}
	s, _, err := e.svc.Create(ctx, a, key(), Request{Lines: []LineIn{line(it, "2")}, VoucherCodes: []string{"HEMAT10"}, Payments: []PaymentIn{pay("cash", "200000")}})
	if err != nil {
		t.Fatal(err)
	}

	d, err := e.svc.Detail(ctx, a, s.ID)
	if err != nil || len(d.Lines) != 1 || d.Lines[0].BaseQty != "2" || d.Lines[0].ListPrice != "100000.00" || d.Lines[0].LineTotal != "200000.00" {
		t.Fatalf("baris: %+v %v", d.Lines, err)
	}
	if d.Cost != nil || d.Profit != nil || d.Lines[0].UnitCost != nil || d.Lines[0].Profit != nil {
		t.Fatalf("HPP bocor tanpa izin: %+v", d)
	}
	if d.VoucherAmount != "15000.00" || d.ManualDiscount != "0.00" || d.LineDiscount != "0.00" || len(d.Vouchers) != 1 || d.Outlet.Code != "main" {
		t.Fatalf("potongan: %+v", d)
	}
	if len(d.Stock) != 1 || d.Stock[0].Type != "SALE" || d.Stock[0].Delta != "-2" || d.Stock[0].BalanceAfter != "18" || d.Stock[0].Bucket != "display" {
		t.Fatalf("stok: %+v", d.Stock)
	}
	if len(d.Events) != 1 || d.Events[0].Action != "sale.create" {
		t.Fatalf("riwayat: %+v", d.Events)
	}

	withCost := a
	withCost.Perms = authz.Permissions{Grants: map[string][]string{"sales_cost": {"view"}}}
	dc, err := e.svc.Detail(ctx, withCost, s.ID)
	if err != nil || dc.Cost == nil || *dc.Cost != "60000.00" || *dc.Profit != "125000.00" || *dc.Lines[0].LineCost != "60000.00" || *dc.Lines[0].Profit != "140000.00" {
		t.Fatalf("hpp: %+v %v", dc, err)
	}

	// Cabang di luar akses & tenant lain = tidak ditemukan.
	other := a
	other.OutletID = uuid.New()
	other.Outlets = map[uuid.UUID]bool{other.OutletID: true}
	if _, err := e.svc.Detail(ctx, other, s.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("cabang lain: %v", err)
	}
	if _, err := e.svc.Detail(ctx, e.actor(e.other), s.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("tenant lain: %v", err)
	}
	if _, err := e.svc.Detail(ctx, a, uuid.New()); !errors.Is(err, ErrNotFound) {
		t.Fatalf("tak ada: %v", err)
	}
}

// Rincian biaya lain-lain: total = Σ rincian, tersimpan terstruktur, tampil di nota/detail, divalidasi, ikut idempotensi.
func TestOtherCostsBreakdown(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	it := e.item(t, "goods", "100000", "30000", 10, false)
	a := e.actor(e.tenant)
	a.Outlets = map[uuid.UUID]bool{e.outlet: true}
	costs := []CostIn{{Name: "Ongkir", Amount: "5000"}, {Name: " Bungkus ", Amount: "1500.50"}}

	q, err := e.svc.Quote(ctx, a, Request{Lines: []LineIn{line(it, "1")}, OtherCosts: costs})
	if err != nil || q.OtherCost != "6500.50" || q.Total != "106500.50" {
		t.Fatalf("quote: %+v %v", q, err)
	}
	k := key()
	s, _, err := e.svc.Create(ctx, a, k, Request{Lines: []LineIn{line(it, "1")}, OtherCosts: costs, OtherCost: "6500.50", Payments: []PaymentIn{pay("cash", "110000")}})
	if err != nil || s.OtherCost != "6500.50" || len(s.OtherCosts) != 2 || s.OtherCosts[1].Name != "Bungkus" || s.OtherCosts[1].Amount != "1500.50" {
		t.Fatalf("simpan: %+v %v", s, err)
	}
	d, err := e.svc.Detail(ctx, a, s.ID)
	if err != nil || len(d.OtherCosts) != 2 || d.OtherCosts[0].Name != "Ongkir" || d.OtherCost != "6500.50" {
		t.Fatalf("detail: %+v %v", d.OtherCosts, err)
	}
	if rep, replayed, err := e.svc.Create(ctx, a, k, Request{Lines: []LineIn{line(it, "1")}, OtherCosts: costs, OtherCost: "6500.50", Payments: []PaymentIn{pay("cash", "110000")}}); err != nil || !replayed || rep.ID != s.ID {
		t.Fatalf("replay: %v %v", replayed, err)
	}
	if _, _, err := e.svc.Create(ctx, a, k, Request{Lines: []LineIn{line(it, "1")}, OtherCosts: costs[:1], Payments: []PaymentIn{pay("cash", "110000")}}); !errors.Is(err, ErrKeyMismatch) {
		t.Fatalf("rincian beda harus ditolak: %v", err)
	}

	// Validasi: total terpisah harus sama, jumlah > 0, nama wajar, batas banyaknya.
	base := Request{Lines: []LineIn{line(it, "1")}}
	for name, tc := range map[string]struct {
		costs []CostIn
		other string
		field string
	}{
		"total beda":   {costs, "1", "other_cost"},
		"nol":          {[]CostIn{{Name: "x", Amount: "0"}}, "", "other_costs.0.amount"},
		"negatif":      {[]CostIn{{Name: "x", Amount: "-5"}}, "", "other_costs.0.amount"},
		"nama panjang": {[]CostIn{{Name: strings.Repeat("a", 81), Amount: "1"}}, "", "other_costs.0.name"},
	} {
		r := base
		r.OtherCosts, r.OtherCost = tc.costs, json.Number(tc.other)
		if _, err := e.svc.Quote(ctx, a, r); err == nil {
			t.Fatalf("%s: harus ditolak", name)
		} else {
			var fe FieldErrors
			if !errors.As(err, &fe) || fe[tc.field] == "" {
				t.Fatalf("%s: galat %v, ingin field %s", name, err, tc.field)
			}
		}
	}
	many := make([]CostIn, 21)
	for i := range many {
		many[i] = CostIn{Name: "b", Amount: "1"}
	}
	r := base
	r.OtherCosts = many
	fieldErr(t, func() error { _, err := e.svc.Quote(ctx, a, r); return err }(), "other_costs", codeTooMany)

	// Tanpa rincian tetap bisa (satu angka), dan tidak menghasilkan baris rincian.
	s2, _, err := e.svc.Create(ctx, a, key(), Request{Lines: []LineIn{line(it, "1")}, OtherCost: "2000", Payments: []PaymentIn{pay("cash", "102000")}})
	if err != nil || s2.OtherCost != "2000.00" || len(s2.OtherCosts) != 0 {
		t.Fatalf("tanpa rincian: %+v %v", s2, err)
	}
}
