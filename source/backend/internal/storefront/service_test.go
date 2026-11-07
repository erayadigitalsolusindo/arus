package storefront

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"aciraba/internal/authz"
)

type env struct {
	svc     *Service
	admin   *pgxpool.Pool
	a, b    authz.Actor
	tenants [2]uuid.UUID
}

func newEnv(t *testing.T) *env {
	t.Helper()
	appURL, adminURL := os.Getenv("TEST_DATABASE_URL"), os.Getenv("TEST_ADMIN_DATABASE_URL")
	if appURL == "" || adminURL == "" {
		t.Skip("TEST_DATABASE_URL / TEST_ADMIN_DATABASE_URL tidak di-set")
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
	e := &env{svc: NewService(app), admin: admin}
	actors := make([]authz.Actor, 2)
	for i := range actors {
		tid, uid, oid, role := uuid.New(), uuid.New(), uuid.New(), uuid.New()
		e.tenants[i] = tid
		sfx := tid.String()[:8]
		for _, q := range []struct {
			sql  string
			args []any
		}{
			{`INSERT INTO tenants (id, code, name) VALUES ($1, $2, $3)`, []any{tid, "sf-" + sfx, "UJI-SF-" + sfx}},
			{`INSERT INTO outlets (id, tenant_id, code, name) VALUES ($1, $2, 'main', 'Pusat')`, []any{oid, tid}},
			{`INSERT INTO roles (id, tenant_id, name, permissions, is_system) VALUES ($1, $2, 'Owner', '{"*":true}', true)`, []any{role, tid}},
			{`INSERT INTO users (id, tenant_id, role_id, email, name, password_hash) VALUES ($1, $2, $3, $4, 'Pemilik', 'x')`, []any{uid, tid, role, "sf-" + sfx + "@it.test"}},
		} {
			if _, err := admin.Exec(ctx, q.sql, q.args...); err != nil {
				t.Fatal(err)
			}
		}
		actors[i] = authz.Actor{TenantID: tid, UserID: uid, OutletID: oid, Name: "Pemilik", Perms: authz.Permissions{All: true}, Outlets: map[uuid.UUID]bool{oid: true}}
	}
	e.a, e.b = actors[0], actors[1]
	t.Cleanup(func() {
		for _, tid := range e.tenants {
			for _, tbl := range []string{"audit_log", "storefront_versions", "storefronts", "item_images", "items", "units", "categories", "user_outlets", "users", "roles", "outlets"} {
				_, _ = admin.Exec(ctx, `DELETE FROM `+tbl+` WHERE tenant_id = $1`, tid)
			}
			_, _ = admin.Exec(ctx, `DELETE FROM tenants WHERE id = $1`, tid)
		}
	})
	return e
}

func (e *env) sql(t *testing.T, q string, args ...any) {
	t.Helper()
	if _, err := e.admin.Exec(context.Background(), q, args...); err != nil {
		t.Fatal(err)
	}
}

func TestStorefrontLifecycle(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	slugA := "toko-a-" + e.tenants[0].String()[:8]
	slugB := "toko-b-" + e.tenants[1].String()[:8]

	// Belum pernah disimpan.
	if c, err := e.svc.Get(ctx, e.a); err != nil || c != nil {
		t.Fatalf("Get awal = %v %v, want nil", c, err)
	}
	// Validasi.
	if _, _, err := e.svc.Save(ctx, e.a, SaveInput{Slug: "A_B", HTML: "x"}); err == nil {
		t.Fatal("slug tak valid harus ditolak")
	}
	if _, _, err := e.svc.Save(ctx, e.a, SaveInput{Slug: slugA, HTML: "x", CSS: `.a{background:url(https://evil.test/x)}`}); err == nil {
		t.Fatal("CSS berbahaya harus ditolak")
	}

	// Simpan (nonaktif) -> tidak bisa dilihat publik; peringatan menyebut tag yang dibuang.
	c, warn, err := e.svc.Save(ctx, e.a, SaveInput{Slug: slugA, Title: "Toko A", HTML: `<h1>{namatoko}</h1><script>alert(1)</script>`, CSS: `h1{color:red}`})
	if err != nil || c.Version != 1 || len(warn) == 0 {
		t.Fatalf("simpan: %+v %v warn=%v", c, err, warn)
	}
	if _, err := e.svc.Lookup(ctx, slugA); !errors.Is(err, ErrNotFound) {
		t.Fatalf("halaman nonaktif tak boleh publik: %v", err)
	}
	c, _, err = e.svc.Save(ctx, e.a, SaveInput{Slug: slugA, Enabled: true, Title: "Toko A", HTML: `<h1>{namatoko}</h1>{#barang limit=5}<p>{namabarang} {harga}</p>{/barang}`})
	if err != nil || c.Version != 2 {
		t.Fatalf("simpan ke-2: %+v %v", c, err)
	}
	p, err := e.svc.Lookup(ctx, slugA)
	if err != nil || p.TenantID != e.tenants[0] || p.Version != 2 {
		t.Fatalf("lookup: %+v %v", p, err)
	}

	// Slug unik global; tenant lain tak bisa merebut.
	if _, _, err := e.svc.Save(ctx, e.b, SaveInput{Slug: slugA, HTML: "x"}); !errors.Is(err, ErrSlugTaken) {
		t.Fatalf("slug bentrok = %v, want ErrSlugTaken", err)
	}
	if _, _, err := e.svc.Save(ctx, e.b, SaveInput{Slug: slugB, Enabled: true, HTML: "<p>B</p>"}); err != nil {
		t.Fatal(err)
	}
	// Tenant A tak melihat konfigurasi B (RLS) dan sebaliknya.
	if cb, _ := e.svc.Get(ctx, e.b); cb == nil || cb.Slug != slugB {
		t.Fatalf("Get B = %+v", cb)
	}

	// Riwayat: 2 versi; versi lama bisa dibaca.
	vs, err := e.svc.Versions(ctx, e.a)
	if err != nil || len(vs) != 2 || vs[0].Version != 2 {
		t.Fatalf("versions: %+v %v", vs, err)
	}
	old, err := e.svc.GetVersion(ctx, e.a, 1)
	if err != nil || !strings.Contains(old.HTML, "<script>") {
		t.Fatalf("versi 1 (teks asli disimpan apa adanya): %+v %v", old, err)
	}
	if vb, err := e.svc.GetVersion(ctx, e.b, 1); err != nil || vb.HTML != "<p>B</p>" {
		t.Fatalf("versi 1 tenant B harus miliknya sendiri: %+v %v", vb, err)
	}
	if _, err := e.svc.GetVersion(ctx, e.b, 2); !errors.Is(err, ErrNotFound) {
		t.Fatalf("versi 2 hanya milik A: %v", err)
	}

	// Data barang: hanya aktif, hanya milik tenant itu, urut nama, kategori terfilter.
	unit, cat, catB := uuid.New(), uuid.New(), uuid.New()
	e.sql(t, `INSERT INTO units (id, tenant_id, name) VALUES ($1, $2, 'Pcs'), ($3, $4, 'Pcs')`, unit, e.tenants[0], uuid.New(), e.tenants[1])
	e.sql(t, `INSERT INTO categories (id, tenant_id, name) VALUES ($1, $2, 'Minuman'), ($3, $4, 'Rahasia B')`, cat, e.tenants[0], catB, e.tenants[1])
	e.sql(t, `INSERT INTO items (tenant_id, sku, name, unit_id, category_id, sell_price, active) VALUES
		($1, 'K1', 'Kopi', $2, $3, 18500, true), ($1, 'T1', 'Teh', $2, $3, 5000, true), ($1, 'X1', 'Arsip', $2, $3, 100, false)`, e.tenants[0], unit, cat)
	var unitB uuid.UUID
	if err := e.admin.QueryRow(ctx, `SELECT id FROM units WHERE tenant_id = $1`, e.tenants[1]).Scan(&unitB); err != nil {
		t.Fatal(err)
	}
	e.sql(t, `INSERT INTO items (tenant_id, sku, name, unit_id, category_id, sell_price) VALUES ($1, 'B1', 'Barang Tenant B', $2, $3, 999)`, e.tenants[1], unitB, catB)

	body, err := e.svc.Body(ctx, e.tenants[0], "Toko A", slugA, p.HTML, "")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"<h1>Toko A</h1>", "<p>Kopi Rp18.500</p>", "<p>Teh Rp5.000</p>"} {
		if !strings.Contains(body, want) {
			t.Errorf("body tak memuat %q: %s", want, body)
		}
	}
	for _, bad := range []string{"Arsip", "Barang Tenant B", "Rahasia B"} {
		if strings.Contains(body, bad) {
			t.Errorf("body bocor %q: %s", bad, body)
		}
	}
	body, _ = e.svc.Body(ctx, e.tenants[0], "Toko A", slugA, `{#kategori}{namakategori}:{#barang kategori=ini}{namabarang};{/barang}{/kategori}|{kategori1}|{namabarang}`, "")
	if body != "Minuman:Kopi;Teh;|Minuman|Kopi" {
		t.Fatalf("kategori/indeks: %q", body)
	}

	// Pemangkasan riwayat: hanya keepVersions terbaru yang tersisa.
	for i := 0; i < keepVersions+2; i++ {
		if _, _, err := e.svc.Save(ctx, e.a, SaveInput{Slug: slugA, Enabled: true, HTML: "<p>v</p>"}); err != nil {
			t.Fatal(err)
		}
	}
	if vs, _ := e.svc.Versions(ctx, e.a); len(vs) != keepVersions {
		t.Fatalf("riwayat setelah pemangkasan = %d, want %d", len(vs), keepVersions)
	}
}
