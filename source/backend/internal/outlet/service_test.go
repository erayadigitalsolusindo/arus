package outlet

import (
	"context"
	"errors"
	"os"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"aciraba/internal/authz"
)

func TestValidate(t *testing.T) {
	good, f := validate(Input{Code: " Cabang-2 ", Name: "  Cabang   Dua ", Timezone: "Asia/Makassar", TaxStorePct: "11", TaxGovPct: "0.5"}, true)
	if f != nil || good.code != "cabang-2" || good.name != "Cabang Dua" || good.tz != "Asia/Makassar" || good.taxStore.String() != "11" {
		t.Fatalf("input valid: %+v %v", good, f)
	}
	if c, f := validate(Input{Code: "x", Name: "N"}, false); f != nil || c.tz != defaultTimezone || !c.taxStore.IsZero() {
		t.Errorf("kode tidak divalidasi saat ubah; zona waktu bawaan; pajak kosong = 0: %+v %v", c, f)
	}
	for name, in := range map[string]Input{
		"kode terlalu pendek": {Code: "a", Name: "N"},
		"kode berspasi":       {Code: "a b", Name: "N"},
		"nama kosong":         {Code: "ab", Name: " "},
		"zona waktu palsu":    {Code: "ab", Name: "N", Timezone: "Mars/Olympus"},
		"zona Local":          {Code: "ab", Name: "N", Timezone: "Local"},
		"pajak negatif":       {Code: "ab", Name: "N", TaxStorePct: "-1"},
		"pajak >100":          {Code: "ab", Name: "N", TaxGovPct: "100.01"},
		"pajak 3 desimal":     {Code: "ab", Name: "N", TaxStorePct: "10.123"},
		"pajak bukan angka":   {Code: "ab", Name: "N", TaxStorePct: "sepuluh"},
	} {
		if _, f := validate(in, true); f == nil {
			t.Errorf("%s: seharusnya ditolak", name)
		}
	}
}

type env struct {
	svc    *Service
	admin  *pgxpool.Pool
	tenant uuid.UUID
	first  uuid.UUID
	owner  authz.Actor
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

	e := &env{admin: admin, svc: NewService(app, authz.NewResolver(app)), tenant: uuid.New(), first: uuid.New()}
	roleID, userID := uuid.New(), uuid.New()
	sfx := e.tenant.String()[:8]
	for _, q := range []struct {
		sql  string
		args []any
	}{
		{`INSERT INTO tenants (id, code, name) VALUES ($1, $2, $3)`, []any{e.tenant, "ot-" + sfx, "UJI-OUTLET-" + sfx}},
		{`INSERT INTO outlets (id, tenant_id, code, name) VALUES ($1, $2, 'main', 'Pusat')`, []any{e.first, e.tenant}},
		{`INSERT INTO roles (id, tenant_id, name, permissions, is_system) VALUES ($1, $2, 'Owner', '{"*":true}', true)`, []any{roleID, e.tenant}},
		{`INSERT INTO users (id, tenant_id, role_id, email, name, password_hash) VALUES ($1, $2, $3, $4, 'Pemilik', 'x')`, []any{userID, e.tenant, roleID, "ot-" + sfx + "@ot.test"}},
	} {
		if _, err := admin.Exec(ctx, q.sql, q.args...); err != nil {
			t.Fatal(err)
		}
	}
	t.Cleanup(func() {
		for _, tbl := range []string{"audit_log", "user_outlets", "users", "roles", "outlets"} {
			_, _ = admin.Exec(ctx, `DELETE FROM `+tbl+` WHERE tenant_id = $1`, e.tenant)
		}
		_, _ = admin.Exec(ctx, `DELETE FROM tenants WHERE id = $1`, e.tenant)
	})
	e.owner = authz.Actor{TenantID: e.tenant, UserID: userID, OutletID: e.first, Name: "Pemilik", Perms: authz.Permissions{All: true}, Outlets: map[uuid.UUID]bool{e.first: true}}
	return e
}

func TestCreateUpdateAndGuards(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()

	o, err := e.svc.Create(ctx, e.owner, Input{Code: "cab2", Name: "Cabang 2", TaxStorePct: "11"})
	if err != nil {
		t.Fatal(err)
	}
	if o.Code != "cab2" || o.TaxStorePct.String() != "11" || o.Timezone != defaultTimezone || !o.Active {
		t.Errorf("outlet baru: %+v", o)
	}
	if _, err := e.svc.Create(ctx, e.owner, Input{Code: "cab2", Name: "Lain"}); !errors.Is(err, ErrCodeTaken) {
		t.Errorf("kode ganda: err = %v, want ErrCodeTaken", err)
	}
	var fe FieldErrors
	if _, err := e.svc.Create(ctx, e.owner, Input{Code: "x", Name: ""}); !errors.As(err, &fe) || fe["code"] == "" || fe["name"] == "" {
		t.Errorf("validasi: err = %v", err)
	}

	// Ubah: kode tidak berubah; audit mencatat sebelum/sesudah.
	upd, err := e.svc.Update(ctx, e.owner, o.ID, Input{Name: "Cabang Dua", Timezone: "Asia/Jayapura", TaxStorePct: "10", Active: true})
	if err != nil || upd.Code != "cab2" || upd.Name != "Cabang Dua" || upd.Timezone != "Asia/Jayapura" {
		t.Fatalf("ubah: %+v %v", upd, err)
	}
	var audits int
	_ = e.admin.QueryRow(ctx, `SELECT count(*) FROM audit_log WHERE tenant_id = $1 AND entity = 'outlet'`, e.tenant).Scan(&audits)
	if audits != 2 {
		t.Errorf("audit outlet = %d, want 2 (buat + ubah)", audits)
	}

	// Outlet yang sedang dipakai tidak boleh dinonaktifkan; yang lain boleh selama masih ada outlet aktif lain.
	if _, err := e.svc.Update(ctx, e.owner, e.first, Input{Name: "Pusat", Active: false}); !errors.Is(err, ErrCurrentOutlet) {
		t.Errorf("nonaktifkan outlet aktif sesi: err = %v, want ErrCurrentOutlet", err)
	}
	if _, err := e.svc.Update(ctx, e.owner, o.ID, Input{Name: "Cabang Dua", Active: false}); err != nil {
		t.Errorf("nonaktifkan outlet lain: %v", err)
	}
	// Outlet aktif terakhir dilindungi (pemanggil di outlet lain, mis. sesi yang sudah berpindah).
	other := e.owner
	other.OutletID = o.ID
	if _, err := e.svc.Update(ctx, other, e.first, Input{Name: "Pusat", Active: false}); !errors.Is(err, ErrLastOutlet) {
		t.Errorf("outlet aktif terakhir: err = %v, want ErrLastOutlet", err)
	}
	if _, err := e.svc.Update(ctx, e.owner, uuid.New(), Input{Name: "X", Active: true}); !errors.Is(err, ErrNotFound) {
		t.Errorf("outlet tak ada: err = %v, want ErrNotFound", err)
	}

	// Pengguna non-pemilik: hanya melihat/mengubah outlet yang ditugaskan; pembuat otomatis mendapat akses ke outlet buatannya.
	mgr := authz.Actor{TenantID: e.tenant, UserID: e.owner.UserID, OutletID: e.first, Name: "Manajer",
		Perms: authz.Permissions{Grants: map[string][]string{"outlets": {"view", "create", "update"}}}, Outlets: map[uuid.UUID]bool{e.first: true}}
	if list, _ := e.svc.List(ctx, mgr); len(list) != 1 || list[0].ID != e.first {
		t.Errorf("manajer melihat %d outlet, want hanya outlet miliknya", len(list))
	}
	if _, err := e.svc.Update(ctx, mgr, o.ID, Input{Name: "Curang", Active: true}); !errors.Is(err, ErrNotAccessible) {
		t.Errorf("ubah outlet di luar akses: err = %v, want ErrNotAccessible", err)
	}
	made, err := e.svc.Create(ctx, mgr, Input{Code: "mgr-cab", Name: "Cabang Manajer"})
	if err != nil {
		t.Fatal(err)
	}
	var assigned int
	_ = e.admin.QueryRow(ctx, `SELECT count(*) FROM user_outlets WHERE tenant_id = $1 AND user_id = $2 AND outlet_id = $3`, e.tenant, mgr.UserID, made.ID).Scan(&assigned)
	if assigned != 1 {
		t.Error("pembuat non-pemilik harus otomatis ditugaskan ke outlet buatannya")
	}
}
