package posshortcut

import (
	"context"
	"errors"
	"os"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"aciraba/internal/authz"
)

type fx struct {
	svc             *Service
	admin           *pgxpool.Pool
	t1, t2          uuid.UUID
	outlet, outlet2 uuid.UUID
	userA, userB    uuid.UUID
	item1, item2    uuid.UUID
	otherItem       uuid.UUID
}

func setup(t *testing.T) *fx {
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
	f := &fx{svc: NewService(app), admin: admin, t1: uuid.New(), t2: uuid.New(), outlet: uuid.New(), outlet2: uuid.New(), userA: uuid.New(), userB: uuid.New()}
	exec := func(sql string, args ...any) {
		t.Helper()
		if _, err := admin.Exec(ctx, sql, args...); err != nil {
			t.Fatal(err)
		}
	}
	for _, tid := range []uuid.UUID{f.t1, f.t2} {
		exec(`INSERT INTO tenants (id, code, name) VALUES ($1, $2, 'UJI-PINTASAN')`, tid, "ps-"+tid.String()[:8])
	}
	exec(`INSERT INTO outlets (id, tenant_id, code, name) VALUES ($1, $2, 'main', 'Pusat')`, f.outlet, f.t1)
	exec(`INSERT INTO outlets (id, tenant_id, code, name) VALUES ($1, $2, 'cab2', 'Cabang 2')`, f.outlet2, f.t1)
	role := uuid.New()
	exec(`INSERT INTO roles (id, tenant_id, name) VALUES ($1, $2, 'r')`, role, f.t1)
	for _, u := range []uuid.UUID{f.userA, f.userB} {
		exec(`INSERT INTO users (id, tenant_id, role_id, email, name, password_hash) VALUES ($1, $2, $3, $4, 'Kasir', 'x')`, u, f.t1, role, u.String()+"@uji.test")
	}
	mk := func(tenant uuid.UUID, price string) uuid.UUID {
		unit, id := uuid.New(), uuid.New()
		exec(`INSERT INTO units (id, tenant_id, name) VALUES ($1, $2, $3)`, unit, tenant, "u-"+unit.String()[:8])
		exec(`INSERT INTO items (id, tenant_id, sku, name, unit_id, sell_price) VALUES ($1, $2, $3, 'Barang', $4, $5)`, id, tenant, "S-"+id.String()[:8], unit, price)
		return id
	}
	f.item1, f.item2, f.otherItem = mk(f.t1, "1000"), mk(f.t1, "2000"), mk(f.t2, "3000")
	t.Cleanup(func() {
		for _, tid := range []uuid.UUID{f.t1, f.t2} {
			for _, tbl := range []string{"pos_shortcuts", "item_outlet_prices", "items", "units", "users", "roles", "outlets"} {
				_, _ = admin.Exec(ctx, `DELETE FROM `+tbl+` WHERE tenant_id = $1`, tid)
			}
			_, _ = admin.Exec(ctx, `DELETE FROM tenants WHERE id = $1`, tid)
		}
	})
	return f
}

func (f *fx) actor(user, outlet uuid.UUID) authz.Actor {
	return authz.Actor{TenantID: f.t1, UserID: user, OutletID: outlet, Perms: authz.Permissions{All: true}}
}

func TestShortcuts(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	a, b := f.actor(f.userA, f.outlet), f.actor(f.userB, f.outlet)

	if l, err := f.svc.List(ctx, a); err != nil || len(l) != 0 {
		t.Fatalf("awal kosong: %v %v", l, err)
	}
	if err := f.svc.Set(ctx, a, 3, f.item1); err != nil {
		t.Fatal(err)
	}
	if err := f.svc.Set(ctx, a, 1, f.item2); err != nil {
		t.Fatal(err)
	}
	if err := f.svc.Set(ctx, a, 3, f.item2); err != nil { // mengganti isi slot
		t.Fatal(err)
	}
	l, _ := f.svc.List(ctx, a)
	if len(l) != 2 || l[0].Slot != 1 || l[1].Slot != 3 || l[1].ItemID != f.item2 || l[1].Price != "2000.00" {
		t.Fatalf("daftar: %+v", l)
	}
	// Harga cabang menggantikan harga default hanya di outlet itu.
	if _, err := f.admin.Exec(ctx, `INSERT INTO item_outlet_prices (tenant_id, item_id, outlet_id, sell_price) VALUES ($1, $2, $3, 2500)`, f.t1, f.item2, f.outlet2); err != nil {
		t.Fatal(err)
	}
	if l2, _ := f.svc.List(ctx, f.actor(f.userA, f.outlet2)); l2[0].Price != "2500.00" {
		t.Fatalf("harga cabang: %+v", l2)
	}
	// Kasir lain tidak melihat milik kasir A.
	if lb, _ := f.svc.List(ctx, b); len(lb) != 0 {
		t.Fatalf("bocor antar kasir: %+v", lb)
	}
	// Validasi: slot luar jangkauan, barang tenant lain.
	if err := f.svc.Set(ctx, a, 0, f.item1); !errors.Is(err, ErrBadSlot) {
		t.Fatalf("slot 0: %v", err)
	}
	if err := f.svc.Set(ctx, a, 17, f.item1); !errors.Is(err, ErrBadSlot) {
		t.Fatalf("slot 17: %v", err)
	}
	if err := f.svc.Set(ctx, a, 2, f.otherItem); !errors.Is(err, ErrNoItem) {
		t.Fatalf("barang tenant lain: %v", err)
	}
	if err := f.svc.Clear(ctx, a, 3); err != nil {
		t.Fatal(err)
	}
	if l, _ := f.svc.List(ctx, a); len(l) != 1 {
		t.Fatalf("setelah kosongkan: %+v", l)
	}
}
