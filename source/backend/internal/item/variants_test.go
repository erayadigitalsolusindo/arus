package item

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

func ti(pairs ...string) []TierInput {
	out := []TierInput{}
	for i := 0; i+1 < len(pairs); i += 2 {
		out = append(out, TierInput{MinQty: pairs[i], Price: pairs[i+1]})
	}
	return out
}

func tierStrings(ts []Tier) string { return tierString(ts) }

func TestValidateVariants(t *testing.T) {
	unit, alt := uuid.New().String(), uuid.New().String()
	base := Input{Name: "X", UnitID: unit}

	in := base
	in.Wholesale = &WholesaleInput{Default: ti("11", "8300", "2", "9500", "6", "8500")}
	c, f := validate(in, true)
	if f != nil || c.wholesale == nil || tierStrings(c.wholesale.def) != "2:9500.00,6:8500.00,11:8300.00" {
		t.Fatalf("tier valid (tak terurut diurutkan): %+v %v", c.wholesale, f)
	}
	in = base
	in.Units = &[]UnitInput{{UnitID: alt, Factor: "12", Barcode: "8991", SellPrice: "100000"}}
	if c, f = validate(in, true); f != nil || c.units == nil || len(*c.units) != 1 || (*c.units)[0].factor.String() != "12" || (*c.units)[0].price == nil {
		t.Fatalf("satuan valid: %+v %v", c.units, f)
	}

	badWholesale := map[string]*WholesaleInput{
		"qty ganda":                    {Default: ti("2", "9500", "2", "9000")},
		"harga naik":                   {Default: ti("2", "9500", "6", "9600")},
		"qty nol":                      {Default: ti("0", "9500")},
		"qty kosong":                   {Default: []TierInput{{MinQty: "", Price: "100"}}},
		"harga kosong":                 {Default: []TierInput{{MinQty: "2", Price: ""}}},
		"harga 3 desimal":              {Default: ti("2", "1.005")},
		"qty 4 desimal":                {Default: ti("1.2345", "100")},
		"qty negatif":                  {Default: ti("-1", "100")},
		"outlet bukan uuid":            {Outlets: []OutletTiersInput{{OutletID: "x", Tiers: ti("2", "1")}}},
		"outlet ganda":                 {Outlets: []OutletTiersInput{{OutletID: uuid.NewString()}, {OutletID: uuid.NewString()}}[:1]},
		"tier cabang tidak urut harga": {Outlets: []OutletTiersInput{{OutletID: uuid.NewString(), Tiers: ti("2", "1", "3", "2")}}},
	}
	// "outlet ganda" sungguhan: id yang sama dua kali.
	same := uuid.NewString()
	badWholesale["outlet ganda"] = &WholesaleInput{Outlets: []OutletTiersInput{{OutletID: same, Tiers: ti("2", "1")}, {OutletID: same, Tiers: ti("3", "1")}}}
	for name, w := range badWholesale {
		in := base
		in.Wholesale = w
		if _, f := validate(in, true); f["wholesale"] == "" {
			t.Errorf("grosir %s: seharusnya ditolak", name)
		}
	}
	many := []TierInput{}
	for i := 1; i <= maxTiers+1; i++ {
		many = append(many, TierInput{MinQty: decimal.NewFromInt(int64(i)).String(), Price: decimal.NewFromInt(int64(1000 - i)).String()})
	}
	in = base
	in.Wholesale = &WholesaleInput{Default: many}
	if _, f := validate(in, true); f["wholesale"] != codeTooMany {
		t.Errorf("tier terlalu banyak: %v", f)
	}

	badUnits := map[string][]UnitInput{
		"faktor nol":             {{UnitID: alt, Factor: "0"}},
		"faktor kosong":          {{UnitID: alt, Factor: ""}},
		"faktor negatif":         {{UnitID: alt, Factor: "-2"}},
		"faktor 7 desimal":       {{UnitID: alt, Factor: "1.1234567"}},
		"satuan = satuan dasar":  {{UnitID: unit, Factor: "2"}},
		"satuan bukan uuid":      {{UnitID: "x", Factor: "2"}},
		"satuan ganda":           {{UnitID: alt, Factor: "2"}, {UnitID: alt, Factor: "3"}},
		"barcode ganda":          {{UnitID: alt, Factor: "2", Barcode: "B1"}, {UnitID: uuid.NewString(), Factor: "3", Barcode: "B1"}},
		"barcode = barcode item": {{UnitID: alt, Factor: "2", Barcode: "ITEMBC"}},
		"harga negatif":          {{UnitID: alt, Factor: "2", SellPrice: "-1"}},
		"barcode kontrol":        {{UnitID: alt, Factor: "2", Barcode: "a\x00b"}},
	}
	for name, u := range badUnits {
		in := base
		in.Barcode = "ITEMBC"
		in.Units = &u
		if _, f := validate(in, true); f["units"] == "" {
			t.Errorf("satuan %s: seharusnya ditolak", name)
		}
	}
	tooMany := make([]UnitInput, 0, maxAltUnits+1)
	for range maxAltUnits + 1 {
		tooMany = append(tooMany, UnitInput{UnitID: uuid.NewString(), Factor: "2"})
	}
	in = base
	in.Units = &tooMany
	if _, f := validate(in, true); f["units"] != codeTooMany {
		t.Errorf("satuan terlalu banyak: %v", f)
	}
}

func TestWholesaleLifecycle(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	unit := e.master(t, "units", e.a.TenantID, "Pcs", true)
	it, err := e.svc.Create(ctx, e.a, Input{Name: "Kopi", UnitID: unit.String(), SellPrice: "10000",
		Wholesale: &WholesaleInput{
			Default: ti("2", "9500", "6", "8500", "11", "8300"),
			Outlets: []OutletTiersInput{{OutletID: e.outlet2.String(), Tiers: ti("3", "9000")}},
		}})
	if err != nil {
		t.Fatal(err)
	}
	if tierStrings(it.Wholesale.Default) != "2:9500.00,6:8500.00,11:8300.00" || len(it.Wholesale.Outlets) != 1 || it.Wholesale.Outlets[0].OutletID != e.outlet2 || it.Wholesale.Outlets[0].OutletName != "Cabang 2" {
		t.Fatalf("grosir saat membuat: %+v", it.Wholesale)
	}

	// Harga yang berlaku (contoh spesifikasi) memakai set default di outlet utama dan set cabang di Cabang 2.
	price := func(outlet uuid.UUID, qty string) string {
		def := it.Wholesale.Default
		var own []Tier
		for _, o := range it.Wholesale.Outlets {
			if o.OutletID == outlet {
				own = o.Tiers
			}
		}
		p, err := PriceFor(decimal.RequireFromString("10000"), ChooseTiers(def, own), decimal.RequireFromString(qty))
		if err != nil {
			t.Fatal(err)
		}
		return p.StringFixed(0)
	}
	for qty, want := range map[string]string{"1": "10000", "2": "9500", "5": "9500", "6": "8500", "10": "8500", "11": "8300", "500": "8300"} {
		if got := price(e.a.OutletID, qty); got != want {
			t.Errorf("outlet utama qty %s = %s, want %s", qty, got, want)
		}
	}
	for qty, want := range map[string]string{"1": "10000", "2": "10000", "3": "9000", "500": "9000"} { // set cabang menggantikan penuh
		if got := price(e.outlet2, qty); got != want {
			t.Errorf("cabang 2 qty %s = %s, want %s", qty, got, want)
		}
	}

	in := func(w *WholesaleInput) Input {
		return Input{Name: "Kopi", SKU: it.SKU, UnitID: unit.String(), SellPrice: "10000", Wholesale: w}
	}
	// Tanpa field grosir → tidak disentuh.
	up, err := e.svc.Update(ctx, e.a, it.ID, in(nil))
	if err != nil || len(up.Wholesale.Default) != 3 || len(up.Wholesale.Outlets) != 1 {
		t.Fatalf("grosir harus tetap: %+v %v", up.Wholesale, err)
	}
	// Ganti default; cabang yang tidak tercantum kembali ke default (set cabangnya hilang).
	up, err = e.svc.Update(ctx, e.a, it.ID, in(&WholesaleInput{Default: ti("5", "9000")}))
	if err != nil || tierStrings(up.Wholesale.Default) != "5:9000.00" || len(up.Wholesale.Outlets) != 0 {
		t.Fatalf("ganti default: %+v %v", up.Wholesale, err)
	}
	// Set kosong menghapus semua.
	up, err = e.svc.Update(ctx, e.a, it.ID, in(&WholesaleInput{}))
	if err != nil || len(up.Wholesale.Default) != 0 {
		t.Fatalf("hapus grosir: %+v %v", up.Wholesale, err)
	}
	if n := e.auditCount(t, e.a.TenantID, "item.price"); n < 3 {
		t.Errorf("perubahan grosir harus tercatat sebagai item.price, got %d", n)
	}
	before := e.auditCount(t, e.a.TenantID, "item.price")
	if _, err := e.svc.Update(ctx, e.a, it.ID, in(&WholesaleInput{})); err != nil { // tanpa perubahan
		t.Fatal(err)
	}
	if e.auditCount(t, e.a.TenantID, "item.price") != before {
		t.Error("simpan tanpa perubahan grosir tidak boleh menambah audit")
	}

	// Validasi dan akses cabang.
	var fe FieldErrors
	if _, err := e.svc.Update(ctx, e.a, it.ID, in(&WholesaleInput{Default: ti("2", "9500", "6", "9600")})); !errors.As(err, &fe) || fe["wholesale"] != codeNotDecreasing {
		t.Errorf("harga naik: %v", err)
	}
	limited := e.a
	limited.Outlets = map[uuid.UUID]bool{e.a.OutletID: true}
	if _, err := e.svc.Update(ctx, limited, it.ID, in(&WholesaleInput{Outlets: []OutletTiersInput{{OutletID: e.outlet2.String(), Tiers: ti("2", "1")}}})); !errors.Is(err, ErrOutletForbidden) {
		t.Errorf("set grosir cabang di luar akses: %v", err)
	}
	// Pemanggil terbatas tidak menghapus/melihat set cabang yang tak dapat diaksesnya.
	if _, err := e.svc.Update(ctx, e.a, it.ID, in(&WholesaleInput{Outlets: []OutletTiersInput{{OutletID: e.outlet2.String(), Tiers: ti("4", "8000")}}})); err != nil {
		t.Fatal(err)
	}
	up, err = e.svc.Update(ctx, limited, it.ID, in(&WholesaleInput{Default: ti("2", "9500")}))
	if err != nil || len(up.Wholesale.Outlets) != 0 {
		t.Fatalf("pemanggil terbatas tidak melihat set cabang lain: %+v %v", up.Wholesale, err)
	}
	if full, _ := e.svc.Get(ctx, e.a, it.ID); len(full.Wholesale.Outlets) != 1 || full.Wholesale.Outlets[0].Tiers[0].MinQty.String() != "4" {
		t.Errorf("set cabang yang tak dapat diakses pemanggil terbatas harus tetap: %+v", full.Wholesale)
	}
}

func TestAltUnitsAndBarcodes(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	pcs := e.master(t, "units", e.a.TenantID, "Pcs", true)
	dus := e.master(t, "units", e.a.TenantID, "Dus", true)
	box := e.master(t, "units", e.a.TenantID, "Box", true)
	old := e.master(t, "units", e.a.TenantID, "Lusin", false) // terarsip
	foreign := e.master(t, "units", e.b.TenantID, "Milik B", true)

	it, err := e.svc.Create(ctx, e.a, Input{Name: "Kopi", UnitID: pcs.String(), SellPrice: "1000", Barcode: "BC-PCS",
		Units: &[]UnitInput{{UnitID: dus.String(), Factor: "12", Barcode: "BC-DUS", SellPrice: "11000"}, {UnitID: box.String(), Factor: "6.5"}}})
	if err != nil {
		t.Fatal(err)
	}
	if len(it.Units) != 2 || it.Units[0].UnitName != "Dus" || it.Units[0].Factor.String() != "12" || it.Units[0].Barcode != "BC-DUS" ||
		it.Units[0].SellPrice == nil || it.Units[0].SellPrice.String() != "11000" || it.Units[1].SellPrice != nil || it.Units[1].Factor.String() != "6.5" {
		t.Fatalf("satuan tambahan: %+v", it.Units)
	}

	// Barcode unik lintas items dan item_units (kedua arah) per tenant.
	other, err := e.svc.Create(ctx, e.a, Input{Name: "Teh", UnitID: pcs.String()})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := e.svc.Create(ctx, e.a, Input{Name: "Gula", UnitID: pcs.String(), Barcode: "BC-DUS"}); !errors.Is(err, ErrBarcodeTaken) {
		t.Errorf("barcode item = barcode satuan item lain: %v", err)
	}
	if _, err := e.svc.Update(ctx, e.a, other.ID, Input{Name: "Teh", SKU: other.SKU, UnitID: pcs.String(),
		Units: &[]UnitInput{{UnitID: dus.String(), Factor: "10", Barcode: "BC-PCS"}}}); !errors.Is(err, ErrBarcodeTaken) {
		t.Errorf("barcode satuan = barcode item lain: %v", err)
	}
	if _, err := e.svc.Update(ctx, e.a, other.ID, Input{Name: "Teh", SKU: other.SKU, UnitID: pcs.String(),
		Units: &[]UnitInput{{UnitID: dus.String(), Factor: "10", Barcode: "BC-DUS"}}}); !errors.Is(err, ErrBarcodeTaken) {
		t.Errorf("barcode satuan ganda antar item: %v", err)
	}
	// Item yang sama boleh menyimpan ulang barcode miliknya sendiri; tenant lain bebas memakai nilai yang sama.
	if _, err := e.svc.Update(ctx, e.a, it.ID, Input{Name: "Kopi", SKU: it.SKU, UnitID: pcs.String(), SellPrice: "1000", Barcode: "BC-PCS",
		Units: &[]UnitInput{{UnitID: dus.String(), Factor: "12", Barcode: "BC-DUS"}, {UnitID: box.String(), Factor: "6.5"}}}); err != nil {
		t.Errorf("simpan ulang barcode sendiri: %v", err)
	}
	unitB := e.master(t, "units", e.b.TenantID, "Pcs", true)
	if _, err := e.svc.Create(ctx, e.b, Input{Name: "Kopi", UnitID: unitB.String(), Barcode: "BC-DUS"}); err != nil {
		t.Errorf("tenant lain memakai barcode sama: %v", err)
	}

	// Referensi satuan: terarsip/milik tenant lain ditolak untuk satuan baru.
	var fe FieldErrors
	for name, u := range map[string]uuid.UUID{"terarsip": old, "tenant lain": foreign, "tak ada": uuid.New()} {
		if _, err := e.svc.Update(ctx, e.a, other.ID, Input{Name: "Teh", SKU: other.SKU, UnitID: pcs.String(), Units: &[]UnitInput{{UnitID: u.String(), Factor: "2"}}}); !errors.As(err, &fe) || fe["units"] == "" {
			t.Errorf("satuan %s: %v", name, err)
		}
	}
	// Satuan tambahan yang sudah terpasang boleh tetap walau kemudian diarsipkan.
	if _, err := e.admin.Exec(ctx, `UPDATE units SET active = false WHERE id = $1`, box); err != nil {
		t.Fatal(err)
	}
	if up, err := e.svc.Update(ctx, e.a, it.ID, Input{Name: "Kopi", SKU: it.SKU, UnitID: pcs.String(),
		Units: &[]UnitInput{{UnitID: dus.String(), Factor: "12"}, {UnitID: box.String(), Factor: "7"}}}); err != nil || len(up.Units) != 2 {
		t.Errorf("satuan terpasang yang diarsipkan: %+v %v", up, err)
	}

	// Satuan dasar tidak boleh menjadi salah satu satuan tambahan (lewat ubah satuan dasar tanpa mengirim daftar).
	if _, err := e.svc.Update(ctx, e.a, it.ID, Input{Name: "Kopi", SKU: it.SKU, UnitID: dus.String()}); !errors.As(err, &fe) || fe["unit_id"] == "" {
		t.Errorf("ubah satuan dasar menjadi satuan tambahan: %v", err)
	}
	// Tanpa field satuan → tidak disentuh; daftar kosong → semua dihapus.
	if up, err := e.svc.Update(ctx, e.a, it.ID, Input{Name: "Kopi", SKU: it.SKU, UnitID: pcs.String()}); err != nil || len(up.Units) != 2 {
		t.Errorf("satuan harus tetap: %+v %v", up, err)
	}
	if up, err := e.svc.Update(ctx, e.a, it.ID, Input{Name: "Kopi", SKU: it.SKU, UnitID: pcs.String(), Units: &[]UnitInput{}}); err != nil || len(up.Units) != 0 {
		t.Errorf("hapus semua satuan tambahan: %+v %v", up, err)
	}
	if e.auditCount(t, e.a.TenantID, "item.update") < 2 {
		t.Error("perubahan satuan tambahan harus tercatat")
	}
}

func TestConcurrentBarcodeAcrossTables(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	pcs := e.master(t, "units", e.a.TenantID, "Pcs", true)
	dus := e.master(t, "units", e.a.TenantID, "Dus", true)
	var wg sync.WaitGroup
	res := make(chan error, 10)
	for i := range 10 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			in := Input{Name: "Paralel", UnitID: pcs.String()}
			if i%2 == 0 { // separuh memakai kode itu sebagai barcode item, separuh sebagai barcode satuan
				in.Barcode = "SAMA"
			} else {
				in.Units = &[]UnitInput{{UnitID: dus.String(), Factor: "2", Barcode: "SAMA"}}
			}
			_, err := e.svc.Create(ctx, e.a, in)
			res <- err
		}()
	}
	wg.Wait()
	close(res)
	ok, taken := 0, 0
	for err := range res {
		switch {
		case err == nil:
			ok++
		case errors.Is(err, ErrBarcodeTaken):
			taken++
		default:
			t.Errorf("error tak terduga: %v", err)
		}
	}
	if ok != 1 || taken != 9 {
		t.Errorf("menang=%d bentrok=%d, want 1 dan 9 (barcode harus unik lintas items/item_units)", ok, taken)
	}
}
