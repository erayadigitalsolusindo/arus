package sales

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"aciraba/internal/platform/db"
)

// Pencarian daftar penjualan, daftar retur penjualan, dan pemilih nota retur memberi hasil yang sama lewat jalur kandidat
// (sale_search_ids/sales_return_search_ids, 00052) maupun jalur ILIKE lama (kandidat melebihi db.SearchCap).
func TestSearchCandidatesMatchFallback(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	a := e.actor(e.tenant)
	it := e.item(t, "goods", "1000", "500", 10, false)
	mem := e.newMember(t, 0, true)
	e.exec(t, `UPDATE members SET name = 'Ibu Langka_Sari', code = 'LSR-01' WHERE id = $1`, mem)
	var cashier string
	if err := e.admin.QueryRow(ctx, `SELECT name FROM users WHERE id = $1`, e.user).Scan(&cashier); err != nil {
		t.Fatal(err)
	}
	s1, _, err := e.svc.Create(ctx, a, key(), Request{MemberID: &mem, Lines: []LineIn{line(it, "2")}, Payments: []PaymentIn{pay("cash", "2000")}})
	if err != nil {
		t.Fatal(err)
	}
	s2, _, err := e.svc.Create(ctx, a, key(), Request{Lines: []LineIn{line(it, "1")}, Payments: []PaymentIn{pay("cash", "1000")}})
	if err != nil {
		t.Fatal(err)
	}
	cash := saleRefundMethod(t, e, "cash")
	ret, _, err := e.svc.CreateSaleReturn(ctx, a, key(), saleReturnRequest(s1.ID, 1, "1", &cash))
	if err != nil {
		t.Fatal(err)
	}

	ids := func(rows []AllRow) map[uuid.UUID]bool {
		out := map[uuid.UUID]bool{}
		for _, r := range rows {
			out[r.ID] = true
		}
		return out
	}
	orig := db.SearchCap
	t.Cleanup(func() { db.SearchCap = orig })
	for _, cap := range []int{orig, -1} { // -1: setiap hasil kandidat dianggap melebihi batas → jalur ILIKE
		db.SearchCap = cap
		for q, want := range map[string][]uuid.UUID{s2.DocNo: {s2.ID}, "langka_s": {s1.ID}, "lsr-01": {s1.ID}, cashier: {s1.ID, s2.ID}, "zzqx": nil} {
			res, err := e.svc.ListAll(ctx, a, AllParams{Q: q})
			got := ids(res.Data)
			if err != nil || len(got) != len(want) || res.Summary.Count != len(want) {
				t.Errorf("cap=%d daftar q=%q: %v %+v", cap, q, err, res)
				continue
			}
			for _, id := range want {
				if !got[id] {
					t.Errorf("cap=%d daftar q=%q: %s tidak ada", cap, q, id)
				}
			}
		}
		for _, q := range []string{ret.DocNo, s1.DocNo, "langka"} {
			res, err := e.svc.ListSaleReturns(ctx, a, SaleReturnListParams{Query: q})
			if err != nil || len(res.Data) != 1 || res.Data[0].ID != ret.ID {
				t.Errorf("cap=%d retur q=%q: %v %+v", cap, q, err, res)
			}
		}
		if res, err := e.svc.ListSaleReturns(ctx, a, SaleReturnListParams{Query: s2.DocNo}); err != nil || len(res.Data) != 0 {
			t.Errorf("cap=%d retur nota lain: %v %+v", cap, err, res)
		}
		for q, want := range map[string]int{s2.DocNo: 1, "langka": 1, cashier: 2, "lsr-01": 0} {
			if list, err := e.svc.SalesReturnChoices(ctx, a, q); err != nil || len(list) != want {
				t.Errorf("cap=%d pemilih q=%q: %v %+v", cap, q, err, list)
			}
		}
	}
}

// Fungsi SECURITY DEFINER melewati RLS: tenant hanya dari app_tenant_id(), bukan dari id outlet yang dikirim.
func TestSearchFunctionsStayInTenant(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	a := e.actor(e.tenant)
	it := e.item(t, "goods", "1000", "500", 10, false)
	mem := e.newMember(t, 0, true)
	sale, _, err := e.svc.Create(ctx, a, key(), Request{MemberID: &mem, Lines: []LineIn{line(it, "2")}, Payments: []PaymentIn{pay("cash", "2000")}})
	if err != nil {
		t.Fatal(err)
	}
	cash := saleRefundMethod(t, e, "cash")
	if _, _, err := e.svc.CreateSaleReturn(ctx, a, key(), saleReturnRequest(sale.ID, 1, "1", &cash)); err != nil {
		t.Fatal(err)
	}
	var code string
	if err := e.admin.QueryRow(ctx, `SELECT code FROM members WHERE id = $1`, mem).Scan(&code); err != nil {
		t.Fatal(err)
	}
	type res struct {
		sales, returns int
		codeTaken      bool
	}
	probe := func(q interface {
		QueryRow(context.Context, string, ...any) pgx.Row
	}) (out res) {
		t.Helper()
		if err := q.QueryRow(ctx, `SELECT (SELECT count(*) FROM sale_search_ids(ARRAY[$1::uuid], NULL, NULL, $2, true, 10)),
			(SELECT count(*) FROM sales_return_search_ids($1, NULL, NULL, $2, 10)), member_code_taken($3)`, e.outlet, "%"+sale.DocNo+"%", code).
			Scan(&out.sales, &out.returns, &out.codeTaken); err != nil {
			t.Fatal(err)
		}
		return out
	}
	in := func(tenant uuid.UUID) (out res) {
		if err := db.WithTenant(ctx, e.app, tenant, func(tx pgx.Tx) error { out = probe(tx); return nil }); err != nil {
			t.Fatal(err)
		}
		return out
	}
	if got := in(e.tenant); got.sales != 1 || got.returns != 1 || !got.codeTaken {
		t.Fatalf("tenant sendiri: %+v", got)
	}
	if got := in(e.other); got != (res{}) {
		t.Errorf("tenant lain melihat data lewat fungsi: %+v", got)
	}
	if got := probe(e.app); got != (res{}) {
		t.Errorf("tanpa tenant melihat data lewat fungsi: %+v", got)
	}
}
