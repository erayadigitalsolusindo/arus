package item

import (
	"context"
	"testing"
)

// HPP per cabang: baris item_outlet_costs menimpa HPP awal barang hanya di cabangnya; cabang lain tetap memakai HPP awal.
func TestOutletCosts(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	unit := e.master(t, "units", e.a.TenantID, "Pcs", true)
	it, err := e.svc.Create(ctx, e.a, Input{Name: "Kopi", UnitID: unit.String(), SellPrice: "1500", Cost: "1000"})
	if err != nil {
		t.Fatal(err)
	}
	if it.AvgCost != "1000.00" || it.LastCost != "1000.00" {
		t.Fatalf("HPP awal: %s/%s", it.AvgCost, it.LastCost)
	}
	if _, err := e.admin.Exec(ctx, `INSERT INTO item_outlet_costs (tenant_id, outlet_id, item_id, avg_cost, last_cost) VALUES ($1, $2, $3, 1500, 1400)`,
		e.a.TenantID, e.outlet2, it.ID); err != nil {
		t.Fatal(err)
	}

	other := e.a
	other.OutletID = e.outlet2
	if rows, _, _ := e.svc.List(ctx, e.a, ListParams{}); rows[0].AvgCost != "1000.00" || rows[0].LastCost != "1000.00" {
		t.Errorf("daftar outlet utama: %+v", rows[0])
	}
	if rows, _, _ := e.svc.List(ctx, other, ListParams{}); rows[0].AvgCost != "1500.00" || rows[0].LastCost != "1400.00" {
		t.Errorf("daftar cabang 2: %+v", rows[0])
	}
	if g, _ := e.svc.Get(ctx, e.a, it.ID); g.AvgCost != "1000.00" {
		t.Errorf("detail outlet utama: %s", g.AvgCost)
	}
	if g, _ := e.svc.Get(ctx, other, it.ID); g.AvgCost != "1500.00" || g.LastCost != "1400.00" {
		t.Errorf("detail cabang 2: %s/%s", g.AvgCost, g.LastCost)
	}

	// Mode semua cabang: rincian memuat HPP tiap cabang.
	rows, _, _ := e.svc.List(ctx, e.a, ListParams{AllOutlets: true})
	got := map[string]string{}
	for _, o := range rows[0].Outlets {
		got[o.OutletID.String()] = o.AvgCost
	}
	if len(got) != 2 || got[e.a.OutletID.String()] != "1000.00" || got[e.outlet2.String()] != "1500.00" {
		t.Errorf("rincian HPP per cabang: %+v", got)
	}

	// Ubah barang tidak menyentuh HPP cabang.
	if _, err := e.svc.Update(ctx, e.a, it.ID, Input{Name: "Kopi Baru", SKU: it.SKU, UnitID: unit.String(), SellPrice: "1600"}); err != nil {
		t.Fatal(err)
	}
	if g, _ := e.svc.Get(ctx, other, it.ID); g.AvgCost != "1500.00" {
		t.Errorf("HPP cabang berubah saat ubah barang: %s", g.AvgCost)
	}
}
