package purchasing

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"aciraba/internal/platform/db"
)

// Pencarian daftar pembelian, retur pembelian, dan pemilih nota retur harus memberi hasil yang sama lewat jalur kandidat
// (purchase_search_ids/purchase_return_search_ids, 00052) maupun jalur ILIKE lama (kandidat melebihi db.SearchCap).
func TestSearchCandidatesMatchFallback(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	a := e.actor(e.outlet)
	m := e.method(t)
	it := e.item(t, "goods", "1000", 10)
	sup2 := uuid.New()
	if _, err := e.admin.Exec(ctx, `INSERT INTO suppliers (id, tenant_id, name) VALUES ($1, $2, 'CV Langka_Abadi')`, sup2, e.tenant); err != nil {
		t.Fatal(err)
	}
	mk := func(sup uuid.UUID, inv string) Purchase {
		t.Helper()
		r := e.req(line(it, "2", "0", "1000"))
		r.SupplierID, r.SupplierInvoiceNo = sup, inv
		p, _, err := e.svc.Create(ctx, a, key(), r)
		if err != nil {
			t.Fatal(err)
		}
		return p
	}
	p1 := mk(e.supplier, "FKT-777")
	p2 := mk(sup2, "FKT-888")
	e.toReturns(t, it, "1")
	req := retReq(p2.ID, rl(0, "1"))
	req.RefundMethodID = &m
	ret, _, err := e.svc.CreateReturn(ctx, a, key(), req)
	if err != nil {
		t.Fatal(err)
	}

	orig := db.SearchCap
	t.Cleanup(func() { db.SearchCap = orig })
	for _, cap := range []int{orig, -1} { // -1: setiap hasil kandidat dianggap melebihi batas → jalur ILIKE
		db.SearchCap = cap
		for q, want := range map[string]uuid.UUID{"fkt-777": p1.ID, "langka_a": p2.ID, p2.DocNo: p2.ID} {
			res, err := e.svc.List(ctx, a, ListParams{Q: q})
			if err != nil || len(res.Data) != 1 || res.Data[0].ID != want || res.Summary.Count != 1 {
				t.Errorf("cap=%d daftar q=%q: %v %+v", cap, q, err, res)
			}
		}
		if res, err := e.svc.List(ctx, a, ListParams{Q: "zzqx"}); err != nil || len(res.Data) != 0 || res.Summary.Count != 0 {
			t.Errorf("cap=%d daftar tanpa hasil: %v %+v", cap, err, res)
		}
		// Underscore literal: "a_A" tidak boleh cocok "...gka Abadi" atau nama lain.
		if res, _ := e.svc.List(ctx, a, ListParams{Q: "a_A"}); res.Summary.Count != 1 {
			t.Errorf("cap=%d escape LIKE: %d", cap, res.Summary.Count)
		}
		for _, q := range []string{ret.DocNo, p2.DocNo, "fkt-888", "langka"} {
			res, err := e.svc.ListReturns(ctx, a, ReturnListParams{Q: q})
			if err != nil || len(res.Data) != 1 || res.Data[0].ID != ret.ID || res.Summary.Count != 1 {
				t.Errorf("cap=%d retur q=%q: %v %+v", cap, q, err, res)
			}
		}
		if res, err := e.svc.ListReturns(ctx, a, ReturnListParams{Q: "fkt-777"}); err != nil || len(res.Data) != 0 {
			t.Errorf("cap=%d retur nota lain: %v %+v", cap, err, res)
		}
		if list, err := e.svc.ReturnablePurchases(ctx, a, "FKT-7"); err != nil || len(list) != 1 || list[0].ID != p1.ID {
			t.Errorf("cap=%d pemilih nota: %v %+v", cap, err, list)
		}
		// Cabang lain tidak melihat nota cabang utama, juga lewat kandidat.
		if res, _ := e.svc.List(ctx, e.actor(e.outlet2), ListParams{Q: "fkt"}); res.Summary.Count != 0 {
			t.Errorf("cap=%d cabang 2: %d", cap, res.Summary.Count)
		}
	}
}

// Fungsi SECURITY DEFINER melewati RLS, jadi tenant WAJIB diambil dari app_tenant_id(): id outlet tenant lain yang
// dikirim pemanggil tidak boleh membuka data, dan tanpa tenant hasilnya kosong.
func TestSearchFunctionsStayInTenant(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	a := e.actor(e.outlet)
	it := e.item(t, "goods", "1000", 10)
	r := e.req(line(it, "1", "0", "1000"))
	r.SupplierInvoiceNo = "ISO-RLS-1"
	if _, _, err := e.svc.Create(ctx, a, key(), r); err != nil {
		t.Fatal(err)
	}
	var sku string
	if err := e.admin.QueryRow(ctx, `SELECT sku FROM items WHERE id = $1`, it).Scan(&sku); err != nil {
		t.Fatal(err)
	}
	type res struct {
		purchases, returns int
		skuTaken           bool
	}
	probe := func(q interface {
		QueryRow(context.Context, string, ...any) pgx.Row
	}) (out res) {
		t.Helper()
		if err := q.QueryRow(ctx, `SELECT (SELECT count(*) FROM purchase_search_ids($1, NULL, NULL, '%iso-rls%', 10)),
			(SELECT count(*) FROM purchase_return_search_ids($1, NULL, NULL, '%%', 10)), item_sku_taken($2)`, e.outlet, sku).
			Scan(&out.purchases, &out.returns, &out.skuTaken); err != nil {
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
	if got := in(e.tenant); got.purchases != 1 || !got.skuTaken {
		t.Fatalf("tenant sendiri: %+v", got)
	}
	if got := in(e.other); got != (res{}) {
		t.Errorf("tenant lain melihat data lewat fungsi: %+v", got)
	}
	if got := probe(e.app); got != (res{}) {
		t.Errorf("tanpa tenant melihat data lewat fungsi: %+v", got)
	}
}
