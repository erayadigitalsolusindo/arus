package item

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
)

func TestPriceHistory(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	unit := e.master(t, "units", e.a.TenantID, "Pcs", true)
	it, err := e.svc.Create(ctx, e.a, Input{Name: "Kopi", UnitID: unit.String(), SellPrice: "1500"})
	if err != nil {
		t.Fatal(err)
	}
	base := Input{Name: "Kopi", SKU: it.SKU, UnitID: unit.String()}

	// 1) default 1500 → 1750 dan harga khusus Cabang 2 dibuat; 2) Cabang 2 → 2000; 3) grosir; 4) tanpa perubahan harga.
	in := base
	in.SellPrice = "1750"
	in.OutletPrices = &[]OutletPriceInput{{OutletID: e.outlet2.String(), SellPrice: "1800"}}
	if _, err = e.svc.Update(ctx, e.a, it.ID, in); err != nil {
		t.Fatal(err)
	}
	in.OutletPrices = &[]OutletPriceInput{{OutletID: e.outlet2.String(), SellPrice: "2000"}}
	if _, err = e.svc.Update(ctx, e.a, it.ID, in); err != nil {
		t.Fatal(err)
	}
	in.Wholesale = &WholesaleInput{Default: []TierInput{{MinQty: "3", Price: "1600"}}}
	if _, err = e.svc.Update(ctx, e.a, it.ID, in); err != nil {
		t.Fatal(err)
	}
	in.Name = "Kopi Susu" // bukan perubahan harga
	if _, err = e.svc.Update(ctx, e.a, it.ID, in); err != nil {
		t.Fatal(err)
	}

	h, err := e.svc.PriceHistory(ctx, e.a, it.ID, "", 0)
	if err != nil {
		t.Fatal(err)
	}
	// Terbaru dulu: grosir, Cabang 2 → 2000, default+Cabang 2 dibuat, harga awal.
	kinds := []string{HistoryWholesale, HistoryPrice, HistoryPrice, HistoryInitial}
	if len(h.Items) != len(kinds) {
		t.Fatalf("jumlah kejadian = %d, want %d: %+v", len(h.Items), len(kinds), h.Items)
	}
	for i, k := range kinds {
		if h.Items[i].Kind != k {
			t.Errorf("kejadian %d kind = %s, want %s", i, h.Items[i].Kind, k)
		}
	}
	if c := h.Items[1].Changes; len(c) != 1 || c[0].OutletID == nil || *c[0].OutletID != e.outlet2 || *c[0].Before != "1800.00" || *c[0].After != "2000.00" {
		t.Errorf("perubahan Cabang 2: %+v", c)
	}
	if c := h.Items[2].Changes; len(c) != 2 {
		t.Fatalf("kejadian pertama harus punya 2 perubahan (default + Cabang 2): %+v", c)
	}
	if c := h.Items[3].Changes; len(c) != 1 || c[0].Before != nil || *c[0].After != "1500.00" {
		t.Errorf("harga awal: %+v", c)
	}

	// Pemanggil yang hanya punya akses outlet utama tidak melihat perubahan Cabang 2.
	limited := e.a
	limited.Outlets = map[uuid.UUID]bool{e.a.OutletID: true}
	lh, err := e.svc.PriceHistory(ctx, limited, it.ID, "", 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(lh.Items) != 3 { // kejadian "Cabang 2 → 2000" hilang seluruhnya
		t.Fatalf("riwayat akses terbatas = %d kejadian, want 3: %+v", len(lh.Items), lh.Items)
	}
	for _, ev := range lh.Items {
		for _, c := range ev.Changes {
			if c.OutletID != nil {
				t.Errorf("perubahan cabang di luar akses bocor: %+v", c)
			}
		}
	}

	// Paginasi: halaman 1 kejadian; harga awal hanya di halaman terakhir.
	p1, err := e.svc.PriceHistory(ctx, e.a, it.ID, "", 1)
	if err != nil || len(p1.Items) != 1 || p1.NextCursor == "" || p1.Items[0].Kind != HistoryWholesale {
		t.Fatalf("halaman 1: %+v %v", p1, err)
	}
	var seen int
	for cur := p1.NextCursor; cur != ""; {
		pg, err := e.svc.PriceHistory(ctx, e.a, it.ID, cur, 1)
		if err != nil {
			t.Fatal(err)
		}
		seen += len(pg.Items)
		cur = pg.NextCursor
	}
	if seen != 3 {
		t.Errorf("sisa halaman = %d kejadian, want 3", seen)
	}

	// Lintas tenant / tak ada = tidak ditemukan.
	if _, err = e.svc.PriceHistory(ctx, e.b, it.ID, "", 0); !errors.Is(err, ErrNotFound) {
		t.Errorf("lintas tenant: err = %v, want ErrNotFound", err)
	}
	if _, err = e.svc.PriceHistory(ctx, e.a, uuid.New(), "", 0); !errors.Is(err, ErrNotFound) {
		t.Errorf("tak ada: err = %v, want ErrNotFound", err)
	}
}
