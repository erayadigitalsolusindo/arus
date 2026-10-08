package item

import (
	"context"
	"errors"
	"os"
	"strings"
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"aciraba/internal/authz"
	"aciraba/internal/platform/storage"
)

func TestValidate(t *testing.T) {
	unit := uuid.New().String()
	good, f := validate(Input{SKU: " ", Barcode: " 899 123 ", Name: "  Kopi   Susu ", Weight: "250.5", Cost: "1000", SellPrice: "1500.50", UnitID: unit,
		Description: "# Judul\r\n\r\n- satu\n- dua\t\n"}, true)
	if f != nil || good.sku != "" || good.barcode != "899 123" || good.name != "Kopi Susu" || good.weight.String() != "250.5" ||
		good.sell.String() != "1500.5" || good.kind != kindGoods || good.desc != "# Judul\n\n- satu\n- dua" {
		t.Fatalf("input valid: %+v %v", good, f)
	}
	if _, f := validate(Input{SKU: "", Name: "X", UnitID: unit}, false); f["sku"] != "REQUIRED" {
		t.Errorf("kode kosong saat mengubah harus ditolak: %v", f)
	}

	bad := map[string]Input{
		"nama kosong":           {Name: " ", UnitID: unit},
		"nama markup":           {Name: "<b>", UnitID: unit},
		"nama 201 karakter":     {Name: strings.Repeat("a", 201), UnitID: unit},
		"sku berspasi":          {Name: "X", SKU: "A B", UnitID: unit},
		"sku 41 karakter":       {Name: "X", SKU: strings.Repeat("a", 41), UnitID: unit},
		"barcode kontrol":       {Name: "X", Barcode: "12\x003", UnitID: unit},
		"barcode panjang":       {Name: "X", Barcode: strings.Repeat("1", 201), UnitID: unit},
		"berat negatif":         {Name: "X", Weight: "-1", UnitID: unit},
		"berat 4 desimal":       {Name: "X", Weight: "1.2345", UnitID: unit},
		"harga negatif":         {Name: "X", SellPrice: "-0.01", UnitID: unit},
		"harga 3 desimal":       {Name: "X", SellPrice: "1.005", UnitID: unit},
		"harga bukan angka":     {Name: "X", SellPrice: "seribu", UnitID: unit},
		"harga terlalu besar":   {Name: "X", SellPrice: "1000000000001", UnitID: unit},
		"hpp negatif":           {Name: "X", Cost: "-5", UnitID: unit},
		"satuan kosong":         {Name: "X"},
		"satuan bukan uuid":     {Name: "X", UnitID: "abc"},
		"kategori bukan uuid":   {Name: "X", UnitID: unit, CategoryID: "abc"},
		"jenis asing":           {Name: "X", UnitID: unit, Kind: "bundle"},
		"keterangan kontrol":    {Name: "X", UnitID: unit, Description: "a\x07b"},
		"keterangan zero-width": {Name: "X", UnitID: unit, Description: "a​b"},
		"keterangan 5001":       {Name: "X", UnitID: unit, Description: strings.Repeat("a", maxDesc+1)},
		"harga cabang kosong":   {Name: "X", UnitID: unit, OutletPrices: &[]OutletPriceInput{{OutletID: uuid.NewString(), SellPrice: ""}}},
		"harga cabang ganda": {Name: "X", UnitID: unit, OutletPrices: &[]OutletPriceInput{
			{OutletID: "00000000-0000-0000-0000-000000000001", SellPrice: "1"}, {OutletID: "00000000-0000-0000-0000-000000000001", SellPrice: "2"}}},
	}
	for name, in := range bad {
		if _, f := validate(in, true); f == nil {
			t.Errorf("%s: seharusnya ditolak", name)
		}
	}
	// Markdown dengan HTML tetap tersimpan apa adanya (disaring saat dirender, bukan dibuang di server).
	if c, f := validate(Input{Name: "X", UnitID: unit, Description: "<b>tebal</b> **md**"}, true); f != nil || c.desc != "<b>tebal</b> **md**" {
		t.Errorf("keterangan markdown: %+v %v", c, f)
	}
}

func TestLikeEscape(t *testing.T) {
	if got := likeEscape(`50%_a\b`); got != `50\%\_a\\b` {
		t.Errorf("likeEscape = %q", got)
	}
}

type env struct {
	svc       *Service
	store     *storage.Local
	storeRoot string
	admin     *pgxpool.Pool
	a, b      authz.Actor
	outlet2   uuid.UUID // outlet kedua milik tenant A
	tenants   [2]uuid.UUID
}

func (e *env) master(t *testing.T, table string, tenant uuid.UUID, name string, active bool) uuid.UUID {
	t.Helper()
	id := uuid.New()
	if _, err := e.admin.Exec(context.Background(), `INSERT INTO `+table+` (id, tenant_id, name, active) VALUES ($1, $2, $3, $4)`, id, tenant, name, active); err != nil {
		t.Fatal(err)
	}
	return id
}

func newEnv(t *testing.T) *env {
	t.Helper()
	appURL, adminURL := os.Getenv("TEST_DATABASE_URL"), os.Getenv("TEST_ADMIN_DATABASE_URL")
	if appURL == "" || adminURL == "" {
		t.Skip("TEST_DATABASE_URL/TEST_ADMIN_DATABASE_URL tidak di-set")
	}
	ctx := context.Background()
	app, err := pgxpool.New(ctx, appURL)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(app.Close)
	admin, err := pgxpool.New(ctx, adminURL)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(admin.Close)

	storeRoot := t.TempDir()
	store, err := storage.NewLocal(storeRoot)
	if err != nil {
		t.Fatal(err)
	}
	e := &env{admin: admin, svc: NewService(app, store), store: store, storeRoot: storeRoot, outlet2: uuid.New()}
	actors := make([]authz.Actor, 2)
	for i := range actors {
		tid, uid, oid := uuid.New(), uuid.New(), uuid.New()
		e.tenants[i] = tid
		sfx := tid.String()[:8]
		roleID := uuid.New()
		stmts := []struct {
			sql  string
			args []any
		}{
			{`INSERT INTO tenants (id, code, name) VALUES ($1, $2, $3)`, []any{tid, "it-" + sfx, "UJI-ITEM-" + sfx}},
			{`INSERT INTO outlets (id, tenant_id, code, name) VALUES ($1, $2, 'main', 'Pusat')`, []any{oid, tid}},
			{`INSERT INTO roles (id, tenant_id, name, permissions, is_system) VALUES ($1, $2, 'Owner', '{"*":true}', true)`, []any{roleID, tid}},
			{`INSERT INTO users (id, tenant_id, role_id, email, name, password_hash) VALUES ($1, $2, $3, $4, 'Pemilik', 'x')`, []any{uid, tid, roleID, "it-" + sfx + "@it.test"}},
		}
		if i == 0 {
			stmts = append(stmts, struct {
				sql  string
				args []any
			}{`INSERT INTO outlets (id, tenant_id, code, name) VALUES ($1, $2, 'cab2', 'Cabang 2')`, []any{e.outlet2, tid}})
		}
		for _, q := range stmts {
			if _, err := admin.Exec(ctx, q.sql, q.args...); err != nil {
				t.Fatal(err)
			}
		}
		outlets := map[uuid.UUID]bool{oid: true}
		if i == 0 {
			outlets[e.outlet2] = true
		}
		actors[i] = authz.Actor{TenantID: tid, UserID: uid, OutletID: oid, Name: "Pemilik", Perms: authz.Permissions{All: true}, Outlets: outlets}
	}
	e.a, e.b = actors[0], actors[1]
	t.Cleanup(func() {
		for _, tid := range e.tenants {
			for _, tbl := range []string{"audit_log", "stock_balances", "item_outlet_prices", "item_counters", "items", "units", "categories", "brands", "principals", "suppliers", "user_outlets", "users", "roles", "outlets"} {
				_, _ = admin.Exec(ctx, `DELETE FROM `+tbl+` WHERE tenant_id = $1`, tid)
			}
			_, _ = admin.Exec(ctx, `DELETE FROM tenants WHERE id = $1`, tid)
		}
	})
	return e
}

func (e *env) auditCount(t *testing.T, tenant uuid.UUID, action string) int {
	t.Helper()
	var n int
	if err := e.admin.QueryRow(context.Background(), `SELECT count(*) FROM audit_log WHERE tenant_id = $1 AND action = $2`, tenant, action).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func TestCreateGetAndCodes(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	unit := e.master(t, "units", e.a.TenantID, "Pcs", true)

	// Kode otomatis berurutan; kode manual yang bentrok dengan nomor berikutnya dilewati.
	i1, err := e.svc.Create(ctx, e.a, Input{Name: "Kopi", UnitID: unit.String(), SellPrice: "1500", Cost: "1000", Weight: "250"})
	if err != nil || i1.SKU != "ITM-000001" || i1.LastCost != "1000.00" || i1.AvgCost != "1000.00" || i1.SellPrice != "1500.00" || i1.Unit.Name != "Pcs" || !i1.Active {
		t.Fatalf("item pertama: %+v %v", i1, err)
	}
	if _, err := e.svc.Create(ctx, e.a, Input{Name: "Manual", SKU: "itm-000002", UnitID: unit.String()}); err != nil {
		t.Fatal(err)
	}
	i3, err := e.svc.Create(ctx, e.a, Input{Name: "Otomatis lagi", UnitID: unit.String()})
	if err != nil || i3.SKU != "ITM-000003" {
		t.Fatalf("kode otomatis harus melewati ITM-000002 yang dipakai manual: %+v %v", i3, err)
	}

	// Kode barang unik per tenant (tanpa membedakan huruf besar/kecil); barcode boleh kembar.
	if _, err := e.svc.Create(ctx, e.a, Input{Name: "Dobel", SKU: "ITM-000001", UnitID: unit.String()}); !errors.Is(err, ErrCodeTaken) {
		t.Errorf("kode ganda: %v", err)
	}
	if _, err := e.svc.Create(ctx, e.a, Input{Name: "A", SKU: "BC-1", Barcode: "899001", UnitID: unit.String()}); err != nil {
		t.Fatal(err)
	}
	// Barcode BOLEH kembar antar barang (barang A negara CC dan B negara DD sama-sama 899001); pembeda opsional.
	if _, err := e.svc.Create(ctx, e.a, Input{Name: "A", SKU: "BC-1X", Origin: "CC", Barcode: "899001", UnitID: unit.String()}); err != nil {
		t.Errorf("barcode kembar harus diizinkan: %v", err)
	}
	if _, err := e.svc.Create(ctx, e.a, Input{Name: "B", SKU: "BC-2", Origin: "DD", Barcode: "899001", UnitID: unit.String()}); err != nil {
		t.Errorf("barcode kembar harus diizinkan: %v", err)
	}
	// Tenant lain boleh memakai kode yang sama.
	unitB := e.master(t, "units", e.b.TenantID, "Pcs", true)
	if _, err := e.svc.Create(ctx, e.b, Input{Name: "A", SKU: "BC-1", Barcode: "899001", UnitID: unitB.String()}); err != nil {
		t.Errorf("tenant lain memakai kode/barcode sama: %v", err)
	}

	// Barcode kosong boleh berulang.
	for _, sku := range []string{"NB-1", "NB-2"} {
		if _, err := e.svc.Create(ctx, e.a, Input{Name: sku, SKU: sku, UnitID: unit.String()}); err != nil {
			t.Errorf("tanpa barcode %s: %v", sku, err)
		}
	}

	// Isolasi tenant: item A tidak terbaca/terubah oleh B.
	if _, err := e.svc.Get(ctx, e.b, i1.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("get lintas tenant: %v", err)
	}
	if _, err := e.svc.Update(ctx, e.b, i1.ID, Input{Name: "Curang", SKU: "X", UnitID: unitB.String()}); !errors.Is(err, ErrNotFound) {
		t.Errorf("ubah lintas tenant: %v", err)
	}
	if _, err := e.svc.SetActive(ctx, e.b, i1.ID, false); !errors.Is(err, ErrNotFound) {
		t.Errorf("arsip lintas tenant: %v", err)
	}
	if e.auditCount(t, e.a.TenantID, "item.create") != 8 {
		t.Errorf("audit item.create = %d, want 8", e.auditCount(t, e.a.TenantID, "item.create"))
	}
}

func TestMasterReferences(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	unit := e.master(t, "units", e.a.TenantID, "Pcs", true)
	archived := e.master(t, "categories", e.a.TenantID, "Lama", false)
	cat := e.master(t, "categories", e.a.TenantID, "Minuman", true)
	brand := e.master(t, "brands", e.a.TenantID, "Kapal Api", true)
	foreignUnit := e.master(t, "units", e.b.TenantID, "Milik B", true)

	var fe FieldErrors
	if _, err := e.svc.Create(ctx, e.a, Input{Name: "X", UnitID: foreignUnit.String()}); !errors.As(err, &fe) || fe["unit_id"] != "INVALID" {
		t.Errorf("satuan milik tenant lain harus ditolak: %v", err)
	}
	if _, err := e.svc.Create(ctx, e.a, Input{Name: "X", UnitID: uuid.NewString()}); !errors.As(err, &fe) || fe["unit_id"] != "INVALID" {
		t.Errorf("satuan tak ada: %v", err)
	}
	if _, err := e.svc.Create(ctx, e.a, Input{Name: "X", UnitID: unit.String(), CategoryID: archived.String()}); !errors.As(err, &fe) || fe["category_id"] != "INVALID" {
		t.Errorf("master terarsip tak boleh dipilih untuk item baru: %v", err)
	}

	it, err := e.svc.Create(ctx, e.a, Input{Name: "Kopi", UnitID: unit.String(), CategoryID: cat.String(), BrandID: brand.String()})
	if err != nil || it.Category == nil || it.Category.Name != "Minuman" || it.Brand == nil || it.Principal != nil || it.Supplier != nil {
		t.Fatalf("item dengan master: %+v %v", it, err)
	}
	// Master yang kemudian diarsipkan tetap boleh dirujuk item yang sudah memakainya (tidak diubah).
	if _, err := e.admin.Exec(ctx, `UPDATE categories SET active = false WHERE id = $1`, cat); err != nil {
		t.Fatal(err)
	}
	if _, err := e.svc.Update(ctx, e.a, it.ID, Input{Name: "Kopi Baru", SKU: it.SKU, UnitID: unit.String(), CategoryID: cat.String(), BrandID: brand.String()}); err != nil {
		t.Errorf("mengubah item yang merujuk master terarsip: %v", err)
	}
	// Mengosongkan master opsional diperbolehkan; satuan tidak boleh dikosongkan.
	up, err := e.svc.Update(ctx, e.a, it.ID, Input{Name: "Kopi Baru", SKU: it.SKU, UnitID: unit.String()})
	if err != nil || up.Category != nil || up.Brand != nil {
		t.Errorf("kosongkan master opsional: %+v %v", up, err)
	}
	if _, err := e.svc.Update(ctx, e.a, it.ID, Input{Name: "Kopi Baru", SKU: it.SKU}); !errors.As(err, &fe) || fe["unit_id"] == "" {
		t.Errorf("satuan wajib: %v", err)
	}
}

func TestUpdateKeepsCostAndAudits(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	unit := e.master(t, "units", e.a.TenantID, "Pcs", true)
	it, err := e.svc.Create(ctx, e.a, Input{Name: "Kopi", UnitID: unit.String(), Cost: "1000", SellPrice: "1500"})
	if err != nil {
		t.Fatal(err)
	}
	// Biaya yang dikirim saat mengubah diabaikan: HPP hanya diubah transaksi.
	up, err := e.svc.Update(ctx, e.a, it.ID, Input{Name: "Kopi", SKU: it.SKU, UnitID: unit.String(), Cost: "9999", SellPrice: "1750", Barcode: "123"})
	if err != nil || up.LastCost != "1000.00" || up.AvgCost != "1000.00" || up.SellPrice != "1750.00" || up.Barcode != "123" {
		t.Fatalf("ubah: %+v %v", up, err)
	}
	if e.auditCount(t, e.a.TenantID, "item.update") != 1 || e.auditCount(t, e.a.TenantID, "item.price") != 1 {
		t.Errorf("audit: update=%d price=%d, want 1 dan 1", e.auditCount(t, e.a.TenantID, "item.update"), e.auditCount(t, e.a.TenantID, "item.price"))
	}
	// Simpan tanpa perubahan tidak menambah catatan audit.
	if _, err := e.svc.Update(ctx, e.a, it.ID, Input{Name: "Kopi", SKU: it.SKU, UnitID: unit.String(), SellPrice: "1750", Barcode: "123"}); err != nil {
		t.Fatal(err)
	}
	if e.auditCount(t, e.a.TenantID, "item.update") != 1 || e.auditCount(t, e.a.TenantID, "item.price") != 1 {
		t.Error("simpan tanpa perubahan tidak boleh menambah audit")
	}

	// Arsip / aktifkan kembali + filter daftar.
	if r, err := e.svc.SetActive(ctx, e.a, it.ID, false); err != nil || r.Active {
		t.Fatalf("arsip: %+v %v", r, err)
	}
	yes := true
	if _, total, _ := e.svc.List(ctx, e.a, ListParams{Active: &yes}); total != 0 {
		t.Errorf("aktif total = %d, want 0", total)
	}
	if rows, total, _ := e.svc.List(ctx, e.a, ListParams{Q: "123"}); total != 1 || rows[0].ID != it.ID {
		t.Errorf("cari berdasarkan barcode: %+v", rows)
	}
	if e.auditCount(t, e.a.TenantID, "item.active") != 1 {
		t.Error("arsip harus tercatat")
	}
}

func TestOutletPrices(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	unit := e.master(t, "units", e.a.TenantID, "Pcs", true)
	it, err := e.svc.Create(ctx, e.a, Input{Name: "Kopi", UnitID: unit.String(), SellPrice: "1500",
		OutletPrices: &[]OutletPriceInput{{OutletID: e.outlet2.String(), SellPrice: "1800"}}})
	if err != nil {
		t.Fatal(err)
	}
	prices := func(x *Item) map[uuid.UUID]string {
		m := map[uuid.UUID]string{}
		for _, p := range x.OutletPrices {
			if p.SellPrice != nil {
				m[p.OutletID] = p.SellPrice.StringFixed(2)
			}
		}
		return m
	}
	if len(it.OutletPrices) != 2 || prices(it)[e.outlet2] != "1800.00" || prices(it)[e.a.OutletID] != "" {
		t.Fatalf("harga cabang saat membuat: %+v", it.OutletPrices)
	}

	// Daftar menampilkan harga efektif untuk outlet aktif sesi: outlet utama = default, cabang 2 = harga cabang.
	rows, _, _ := e.svc.List(ctx, e.a, ListParams{})
	if len(rows) != 1 || rows[0].Price != "1500.00" || rows[0].PriceOverride {
		t.Errorf("harga efektif outlet utama: %+v", rows)
	}
	other := e.a
	other.OutletID = e.outlet2
	if rows, _, _ = e.svc.List(ctx, other, ListParams{}); rows[0].Price != "1800.00" || !rows[0].PriceOverride {
		t.Errorf("harga efektif cabang 2: %+v", rows)
	}

	// Daftar menampilkan stok outlet aktif per bucket; outlet lain tidak ikut terhitung. Tanpa saldo = 0.
	if rows[0].Stock.Total != "0" || rows[0].Stock.Display != "0" {
		t.Errorf("stok awal harus 0: %+v", rows[0].Stock)
	}
	for _, b := range []struct {
		outlet uuid.UUID
		bucket string
		qty    string
	}{{e.a.OutletID, "display", "7.5"}, {e.a.OutletID, "warehouse", "20"}, {e.outlet2, "display", "99"}} {
		if _, err := e.admin.Exec(ctx, `INSERT INTO stock_balances (tenant_id, outlet_id, item_id, bucket, qty) VALUES ($1, $2, $3, $4, $5)`,
			e.a.TenantID, b.outlet, it.ID, b.bucket, b.qty); err != nil {
			t.Fatal(err)
		}
	}
	if rows, _, _ = e.svc.List(ctx, e.a, ListParams{}); rows[0].Stock != (StockQty{Display: "7.5", Warehouse: "20", Returns: "0", Total: "27.5"}) {
		t.Errorf("stok outlet utama: %+v", rows[0].Stock)
	}
	if rows, _, _ = e.svc.List(ctx, other, ListParams{}); rows[0].Stock.Total != "99" {
		t.Errorf("stok cabang 2: %+v", rows[0].Stock)
	}

	// Mode semua cabang: stok dijumlahkan atas outlet yang boleh diakses, harga menjadi rentang, plus rincian per cabang.
	if rows, _, _ = e.svc.List(ctx, e.a, ListParams{AllOutlets: true}); len(rows) != 1 ||
		rows[0].Stock != (StockQty{Display: "106.5", Warehouse: "20", Returns: "0", Total: "126.5"}) ||
		rows[0].Price != "1500.00" || rows[0].PriceMax != "1800.00" || len(rows[0].Outlets) != 2 {
		t.Errorf("semua cabang: %+v", rows)
	}
	// Pemanggil dengan akses satu cabang hanya melihat cabang itu, walau meminta semua.
	narrow := e.a
	narrow.Outlets = map[uuid.UUID]bool{e.outlet2: true}
	if rows, _, _ = e.svc.List(ctx, narrow, ListParams{AllOutlets: true}); len(rows[0].Outlets) != 1 || rows[0].Stock.Total != "99" || rows[0].PriceMax != "1800.00" {
		t.Errorf("semua cabang harus dibatasi akses: %+v", rows[0])
	}
	if rows, _, _ = e.svc.List(ctx, other, ListParams{}); len(rows[0].Outlets) != 0 || rows[0].PriceMax != "" {
		t.Errorf("mode biasa tidak membawa rincian: %+v", rows[0])
	}

	// Mengubah tanpa menyertakan harga cabang tidak menyentuhnya.
	up, err := e.svc.Update(ctx, e.a, it.ID, Input{Name: "Kopi", SKU: it.SKU, UnitID: unit.String(), SellPrice: "1600"})
	if err != nil || prices(up)[e.outlet2] != "1800.00" {
		t.Fatalf("harga cabang harus tetap: %+v %v", up, err)
	}
	// Daftar kosong menghapus semua harga cabang (kembali ke default); daftar berisi menggantikan.
	up, err = e.svc.Update(ctx, e.a, it.ID, Input{Name: "Kopi", SKU: it.SKU, UnitID: unit.String(), SellPrice: "1600",
		OutletPrices: &[]OutletPriceInput{{OutletID: e.a.OutletID.String(), SellPrice: "1700"}}})
	if err != nil || len(prices(up)) != 1 || prices(up)[e.a.OutletID] != "1700.00" {
		t.Fatalf("ganti harga cabang: %+v %v", up.OutletPrices, err)
	}
	if up, err = e.svc.Update(ctx, e.a, it.ID, Input{Name: "Kopi", SKU: it.SKU, UnitID: unit.String(), SellPrice: "1600", OutletPrices: &[]OutletPriceInput{}}); err != nil || len(prices(up)) != 0 {
		t.Fatalf("hapus semua harga cabang: %+v %v", up.OutletPrices, err)
	}
	if e.auditCount(t, e.a.TenantID, "item.price") < 3 {
		t.Errorf("perubahan harga harus tercatat, got %d", e.auditCount(t, e.a.TenantID, "item.price"))
	}

	// Pemanggil hanya boleh mengatur harga outlet yang dapat diaksesnya; outlet lain tenant/tak dikenal ditolak.
	limited := e.a
	limited.Outlets = map[uuid.UUID]bool{e.a.OutletID: true}
	if _, err := e.svc.Update(ctx, limited, it.ID, Input{Name: "Kopi", SKU: it.SKU, UnitID: unit.String(),
		OutletPrices: &[]OutletPriceInput{{OutletID: e.outlet2.String(), SellPrice: "1"}}}); !errors.Is(err, ErrOutletForbidden) {
		t.Errorf("outlet di luar akses: %v", err)
	}
	if _, err := e.svc.Create(ctx, e.a, Input{Name: "Y", UnitID: unit.String(),
		OutletPrices: &[]OutletPriceInput{{OutletID: uuid.NewString(), SellPrice: "1"}}}); !errors.Is(err, ErrOutletForbidden) {
		t.Errorf("outlet tak dikenal: %v", err)
	}
	// Pemanggil terbatas tidak melihat dan tidak menghapus harga outlet yang tak dapat diaksesnya.
	if _, err := e.svc.Update(ctx, e.a, it.ID, Input{Name: "Kopi", SKU: it.SKU, UnitID: unit.String(),
		OutletPrices: &[]OutletPriceInput{{OutletID: e.outlet2.String(), SellPrice: "1900"}}}); err != nil {
		t.Fatal(err)
	}
	if up, err = e.svc.Update(ctx, limited, it.ID, Input{Name: "Kopi", SKU: it.SKU, UnitID: unit.String(), OutletPrices: &[]OutletPriceInput{}}); err != nil || len(up.OutletPrices) != 1 {
		t.Fatalf("pemanggil terbatas hanya melihat 1 outlet: %+v %v", up, err)
	}
	if full, _ := e.svc.Get(ctx, e.a, it.ID); prices(full)[e.outlet2] != "1900.00" {
		t.Errorf("harga outlet yang tak dapat diakses pemanggil terbatas harus tetap: %+v", full.OutletPrices)
	}
}

func TestConcurrentAutoCodesAreUnique(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	unit := e.master(t, "units", e.a.TenantID, "Pcs", true)
	var wg sync.WaitGroup
	res := make(chan string, 12)
	errs := make(chan error, 12)
	for range 12 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			it, err := e.svc.Create(ctx, e.a, Input{Name: "Paralel", UnitID: unit.String()})
			if err != nil {
				errs <- err
				return
			}
			res <- it.SKU
		}()
	}
	wg.Wait()
	close(res)
	close(errs)
	for err := range errs {
		t.Errorf("pembuatan bersamaan gagal: %v", err)
	}
	seen := map[string]bool{}
	for sku := range res {
		if seen[sku] {
			t.Errorf("kode ganda %s", sku)
		}
		seen[sku] = true
	}
	if len(seen) != 12 {
		t.Errorf("kode unik = %d, want 12", len(seen))
	}
}
