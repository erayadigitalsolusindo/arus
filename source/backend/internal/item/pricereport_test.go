package item

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
)

func TestPriceReport(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	unit := e.master(t, "units", e.a.TenantID, "Pcs", true)
	kopi, err := e.svc.Create(ctx, e.a, Input{Name: "Kopi", UnitID: unit.String(), SellPrice: "1500"})
	if err != nil {
		t.Fatal(err)
	}
	teh, err := e.svc.Create(ctx, e.a, Input{Name: "Teh", UnitID: unit.String(), SellPrice: "800"})
	if err != nil {
		t.Fatal(err)
	}
	// Kopi: default 1500 → 1750; Cabang 2 dibuat 1800. Teh: default 800 → 900.
	if _, err = e.svc.Update(ctx, e.a, kopi.ID, Input{Name: "Kopi", SKU: kopi.SKU, UnitID: unit.String(), SellPrice: "1750",
		OutletPrices: &[]OutletPriceInput{{OutletID: e.outlet2.String(), SellPrice: "1800"}}}); err != nil {
		t.Fatal(err)
	}
	if _, err = e.svc.Update(ctx, e.a, teh.ID, Input{Name: "Teh", SKU: teh.SKU, UnitID: unit.String(), SellPrice: "900"}); err != nil {
		t.Fatal(err)
	}

	all, err := e.svc.PriceReport(ctx, e.a, PriceReportParams{})
	if err != nil {
		t.Fatal(err)
	}
	// Terbaru dulu: Teh→900, Kopi (default + Cabang 2), harga awal Teh, harga awal Kopi.
	if len(all.Items) != 4 || all.From == "" || all.To == "" {
		t.Fatalf("semua = %d kejadian, want 4: %+v", len(all.Items), all.Items)
	}
	if all.Items[0].Name != "Teh" || all.Items[0].SKU != teh.SKU || all.Items[0].ItemID != teh.ID || all.Items[0].Kind != HistoryPrice {
		t.Errorf("kejadian terbaru: %+v", all.Items[0])
	}

	// Pencarian nama / kode (wildcard di-escape).
	if r, _ := e.svc.PriceReport(ctx, e.a, PriceReportParams{Q: "kop"}); len(r.Items) != 2 {
		t.Errorf("cari 'kop' = %d, want 2", len(r.Items))
	}
	if r, _ := e.svc.PriceReport(ctx, e.a, PriceReportParams{Q: kopi.SKU}); len(r.Items) != 2 {
		t.Errorf("cari kode = %d, want 2", len(r.Items))
	}
	if r, _ := e.svc.PriceReport(ctx, e.a, PriceReportParams{Q: "%"}); len(r.Items) != 0 {
		t.Errorf("cari '%%' tidak boleh jadi wildcard: %d", len(r.Items))
	}

	// Filter cabang: Cabang 2 hanya Kopi; default termasuk harga awal.
	if r, _ := e.svc.PriceReport(ctx, e.a, PriceReportParams{Outlet: e.outlet2.String()}); len(r.Items) != 1 || r.Items[0].Name != "Kopi" {
		t.Errorf("filter Cabang 2: %+v", r.Items)
	}
	if r, _ := e.svc.PriceReport(ctx, e.a, PriceReportParams{Outlet: "default"}); len(r.Items) != 4 {
		t.Errorf("filter default = %d, want 4", len(r.Items))
	}

	// Pemanggil tanpa akses Cabang 2: filter ditolak, dan perubahan Cabang 2 tidak bocor.
	limited := e.a
	limited.Outlets = map[uuid.UUID]bool{e.a.OutletID: true}
	var fe FieldErrors
	if _, err = e.svc.PriceReport(ctx, limited, PriceReportParams{Outlet: e.outlet2.String()}); !errors.As(err, &fe) || fe["outlet"] == "" {
		t.Errorf("filter cabang terlarang: err = %v", err)
	}
	lr, err := e.svc.PriceReport(ctx, limited, PriceReportParams{})
	if err != nil {
		t.Fatal(err)
	}
	for _, ev := range lr.Items {
		for _, c := range ev.Changes {
			if c.OutletID != nil {
				t.Errorf("perubahan cabang di luar akses bocor: %+v", c)
			}
		}
	}

	// Paginasi per 1 kejadian mengumpulkan semuanya tanpa duplikat.
	seen := map[int64]bool{}
	cur := ""
	for i := 0; i < 10; i++ {
		pg, err := e.svc.PriceReport(ctx, e.a, PriceReportParams{Limit: 1, Cursor: cur})
		if err != nil {
			t.Fatal(err)
		}
		for _, ev := range pg.Items {
			if seen[ev.ID] {
				t.Errorf("kejadian %d duplikat", ev.ID)
			}
			seen[ev.ID] = true
		}
		if cur = pg.NextCursor; cur == "" {
			break
		}
	}
	if len(seen) != 4 {
		t.Errorf("paginasi mengumpulkan %d kejadian, want 4", len(seen))
	}

	// Validasi periode + isolasi tenant.
	for name, p := range map[string]PriceReportParams{
		"tanggal rusak":   {From: "2026-13-01"},
		"terbalik":        {From: "2026-02-02", To: "2026-01-01"},
		"terlalu panjang": {From: "2024-01-01", To: "2026-01-01"},
		"kursor rusak":    {Cursor: "!!"},
	} {
		if _, err := e.svc.PriceReport(ctx, e.a, p); !errors.As(err, &fe) {
			t.Errorf("%s: err = %v, want FieldErrors", name, err)
		}
	}
	if r, err := e.svc.PriceReport(ctx, e.b, PriceReportParams{}); err != nil || len(r.Items) != 0 {
		t.Errorf("tenant lain melihat %d kejadian (%v)", len(r.Items), err)
	}
}
