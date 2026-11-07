package storefront

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
)

// Query sumber data tambahan terhadap DB sungguhan: kupon hanya yang bersaklar+berlaku+berkuota, stok, penghitung member,
// dan isolasi antar tenant (RLS).
func TestExtraSourcesAgainstDB(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	tid, oid := e.a.TenantID, e.a.OutletID
	t.Cleanup(func() {
		for _, tbl := range []string{"stock_balances", "vouchers", "payment_methods", "member_levels", "member_counters"} {
			_, _ = e.admin.Exec(ctx, `DELETE FROM `+tbl+` WHERE tenant_id = $1`, tid)
		}
	})
	exec := func(sql string, args ...any) {
		t.Helper()
		if _, err := e.admin.Exec(ctx, sql, args...); err != nil {
			t.Fatal(err)
		}
	}
	unit, ada, habis := uuid.New(), uuid.New(), uuid.New()
	exec(`INSERT INTO units (id, tenant_id, name) VALUES ($1, $2, 'Pcs')`, unit, tid)
	exec(`INSERT INTO items (id, tenant_id, sku, name, unit_id, sell_price) VALUES ($1, $2, 'ADA', 'Ada Stok', $3, 1000), ($4, $2, 'HBS', 'Habis Stok', $3, 2000)`, ada, tid, unit, habis)
	exec(`INSERT INTO stock_balances (tenant_id, outlet_id, item_id, bucket, qty) VALUES ($1, $2, $3, 'display', 5), ($1, $2, $4, 'returns', 9)`, tid, oid, ada, habis)

	today := time.Date(2026, 11, 7, 0, 0, 0, 0, time.UTC)
	exec(`INSERT INTO vouchers (tenant_id, code, name, kind, value, show_public) VALUES ($1, 'PUBLIK1', 'Publik', 'percent', 10, true)`, tid)
	exec(`INSERT INTO vouchers (tenant_id, code, name, kind, value, show_public) VALUES ($1, 'PRIVAT1', 'Privat', 'percent', 10, false)`, tid)
	exec(`INSERT INTO vouchers (tenant_id, code, name, kind, value, show_public, ends_on) VALUES ($1, 'LEWAT1', 'Lewat', 'amount', 5000, true, '2026-11-01')`, tid)
	exec(`INSERT INTO vouchers (tenant_id, code, name, kind, value, show_public, max_uses, used_count) VALUES ($1, 'HABIS1', 'Habis', 'amount', 5000, true, 1, 1)`, tid)
	exec(`INSERT INTO vouchers (tenant_id, code, name, kind, value, show_public, active) VALUES ($1, 'NONAKTIF1', 'Nonaktif', 'amount', 5000, true, false)`, tid)
	exec(`INSERT INTO member_levels (tenant_id, name, min_points, spend_per_point, point_value) VALUES ($1, 'Reguler', 0, 10000, 100)`, tid)
	exec(`INSERT INTO member_counters (tenant_id, last_no) VALUES ($1, 42)`, tid)
	exec(`INSERT INTO payment_methods (tenant_id, name, kind) VALUES ($1, 'Tunai', 'cash')`, tid)

	src := e.svc.source(tid)

	cps, err := src.Coupons(ctx, today, 10)
	if err != nil || len(cps) != 1 || cps[0].Code != "PUBLIK1" {
		t.Fatalf("kupon = %+v err=%v, want hanya PUBLIK1", cps, err)
	}
	items, err := src.Items(ctx, ItemQuery{Limit: 10, Order: "nama"})
	if err != nil || len(items) != 2 {
		t.Fatalf("items = %+v err=%v", items, err)
	}
	for _, it := range items {
		if it.Unit != "Pcs" {
			t.Fatalf("satuan = %q", it.Unit)
		}
		if got, want := it.InStock, it.SKU == "ADA"; got != want { // stok 'returns' tidak dihitung
			t.Fatalf("%s InStock = %v, want %v", it.SKU, got, want)
		}
	}
	if n, err := src.MemberTotal(ctx); err != nil || n != 42 {
		t.Fatalf("member total = %d err=%v", n, err)
	}
	if l, err := src.Levels(ctx, 10); err != nil || len(l) != 1 || l[0].Name != "Reguler" {
		t.Fatalf("level = %+v err=%v", l, err)
	}
	if p, err := src.PayMethods(ctx, 10); err != nil || len(p) != 1 {
		t.Fatalf("metode bayar = %+v err=%v", p, err)
	}
	if o, err := src.Outlets(ctx, 10); err != nil || len(o) != 1 || o[0].Name != "Pusat" {
		t.Fatalf("outlet = %+v err=%v", o, err)
	}

	// Tenant lain tidak melihat apa pun milik tenant A.
	other := e.svc.source(e.b.TenantID)
	if c, _ := other.Coupons(ctx, today, 10); len(c) != 0 {
		t.Fatalf("kupon bocor lintas tenant: %+v", c)
	}
	if n, _ := other.MemberTotal(ctx); n != 0 {
		t.Fatalf("penghitung member bocor lintas tenant: %d", n)
	}
	if l, _ := other.Levels(ctx, 10); len(l) != 0 {
		t.Fatalf("level bocor lintas tenant: %+v", l)
	}
}
