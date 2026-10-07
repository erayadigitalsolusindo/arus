package iam

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

const goodPassword = "sandi-aman-123"

// Integrasi: TEST_DATABASE_URL = role aplikasi (RLS berlaku), TEST_ADMIN_DATABASE_URL = pemilik skema (siapkan/bersihkan data).
type env struct {
	svc      *Service
	res      *Resolver
	admin    *pgxpool.Pool
	tenant   uuid.UUID
	owner    Actor // pemilik pertama
	ownerRol uuid.UUID
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

	e := &env{admin: admin, res: NewResolver(app), tenant: uuid.New(), ownerRol: uuid.New()}
	e.svc = NewService(app, e.res)
	suffix := fmt.Sprintf("%d-%s", os.Getpid(), e.tenant.String()[:8])
	owner := uuid.New()
	for _, q := range []struct {
		sql  string
		args []any
	}{
		{`INSERT INTO tenants (id, code, name) VALUES ($1, $2, $3)`, []any{e.tenant, "iam-" + suffix, "UJI-IAM-" + suffix}},
		{`INSERT INTO roles (id, tenant_id, name, permissions, is_system) VALUES ($1, $2, 'Owner', '{"*":true}', true)`, []any{e.ownerRol, e.tenant}},
		{`INSERT INTO users (id, tenant_id, role_id, email, name, password_hash) VALUES ($1, $2, $3, $4, 'Pemilik', 'x')`, []any{owner, e.tenant, e.ownerRol, "owner-" + suffix + "@iam.test"}},
	} {
		if _, err := admin.Exec(ctx, q.sql, q.args...); err != nil {
			t.Fatalf("siapkan data: %v", err)
		}
	}
	t.Cleanup(func() {
		for _, tbl := range []string{"users", "roles", "outlets"} {
			_, _ = admin.Exec(ctx, `DELETE FROM `+tbl+` WHERE tenant_id = $1`, e.tenant)
		}
		_, _ = admin.Exec(ctx, `DELETE FROM tenants WHERE id = $1`, e.tenant)
	})
	e.owner = Actor{TenantID: e.tenant, UserID: owner, Perms: Permissions{All: true}}
	return e
}

func (e *env) role(t *testing.T, actor Actor, name string, perms map[string][]string) *Role {
	t.Helper()
	r, err := e.svc.CreateRole(context.Background(), actor, name, perms)
	if err != nil {
		t.Fatalf("buat role %s: %v", name, err)
	}
	return r
}

func (e *env) user(t *testing.T, actor Actor, email string, roleID uuid.UUID) *User {
	t.Helper()
	u, err := e.svc.CreateUser(context.Background(), actor, CreateUserInput{Name: "Pegawai", Email: email, Password: goodPassword, RoleID: roleID})
	if err != nil {
		t.Fatalf("buat user %s: %v", email, err)
	}
	return u
}

func TestRoleLifecycleAndEscalation(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()

	kasir := e.role(t, e.owner, "Kasir", map[string][]string{"items": {"view"}, "sales_orders": {"create"}})
	if !kasir.Permissions.Has("sales_orders", "view") {
		t.Error("view harus otomatis menyertai aksi lain")
	}
	if _, err := e.svc.CreateRole(ctx, e.owner, "Kasir", nil); !errors.Is(err, ErrNameTaken) {
		t.Errorf("nama ganda err = %v, want ErrNameTaken", err)
	}
	var fe FieldErrors
	if _, err := e.svc.CreateRole(ctx, e.owner, "X", map[string][]string{"*": {"view"}}); !errors.As(err, &fe) || fe["permissions"] == "" {
		t.Errorf("wildcard lewat API harus ditolak sebagai VALIDATION, err = %v", err)
	}
	if _, err := e.svc.CreateRole(ctx, e.owner, "", nil); !errors.As(err, &fe) || fe["name"] == "" {
		t.Errorf("nama kosong harus ditolak, err = %v", err)
	}

	// Admin (bukan Owner) hanya boleh memberi izin yang ia miliki.
	adminRole := e.role(t, e.owner, "Admin", map[string][]string{"items": {"view", "create"}, "roles": {"create", "update", "delete"}, "users": {"create", "update"}})
	admin := Actor{TenantID: e.tenant, UserID: uuid.New(), Perms: adminRole.Permissions}
	if _, err := e.svc.CreateRole(ctx, admin, "Curang", map[string][]string{"items": {"delete"}}); !errors.Is(err, ErrEscalation) {
		t.Errorf("admin memberi izin di luar miliknya: err = %v, want ErrEscalation", err)
	}
	magang := e.role(t, e.owner, "Magang", map[string][]string{"items": {"view"}})
	if _, err := e.svc.UpdateRole(ctx, admin, magang.ID, "Magang", map[string][]string{"items": {"view", "create"}}); err != nil {
		t.Errorf("admin mengubah role dalam jangkauannya: %v", err)
	}
	// Kasir punya izin yang tidak dimiliki admin (sales_orders): di luar jangkauan.
	if _, err := e.svc.UpdateRole(ctx, admin, kasir.ID, "Kasir", nil); !errors.Is(err, ErrEscalation) {
		t.Errorf("mengubah role di luar jangkauan: err = %v, want ErrEscalation", err)
	}
	// Admin tidak boleh menyentuh role yang melebihi izinnya.
	super := e.role(t, e.owner, "Super", map[string][]string{"items": {"delete"}})
	if _, err := e.svc.UpdateRole(ctx, admin, super.ID, "Super", map[string][]string{}); !errors.Is(err, ErrEscalation) {
		t.Errorf("mengubah role atasan: err = %v, want ErrEscalation", err)
	}
	if err := e.svc.DeleteRole(ctx, admin, super.ID); !errors.Is(err, ErrEscalation) {
		t.Errorf("menghapus role atasan: err = %v, want ErrEscalation", err)
	}

	// Role sistem Owner kebal.
	if _, err := e.svc.UpdateRole(ctx, e.owner, e.ownerRol, "Owner", map[string][]string{"items": {"view"}}); !errors.Is(err, ErrSystemRole) {
		t.Errorf("ubah Owner: err = %v, want ErrSystemRole", err)
	}
	if err := e.svc.DeleteRole(ctx, e.owner, e.ownerRol); !errors.Is(err, ErrSystemRole) {
		t.Errorf("hapus Owner: err = %v, want ErrSystemRole", err)
	}

	// Role yang dipakai tidak bisa dihapus; yang kosong bisa.
	e.user(t, e.owner, fmt.Sprintf("kasir-%d@iam.test", os.Getpid()), kasir.ID)
	if err := e.svc.DeleteRole(ctx, e.owner, kasir.ID); !errors.Is(err, ErrRoleInUse) {
		t.Errorf("hapus role terpakai: err = %v, want ErrRoleInUse", err)
	}
	if err := e.svc.DeleteRole(ctx, e.owner, super.ID); err != nil {
		t.Errorf("hapus role kosong: %v", err)
	}
	if err := e.svc.DeleteRole(ctx, e.owner, super.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("hapus dua kali: err = %v, want ErrNotFound", err)
	}

	roles, err := e.svc.ListRoles(ctx, e.tenant)
	if err != nil || len(roles) != 4 || !roles[0].IsSystem || roles[0].UserCount != 1 {
		t.Errorf("daftar role: %+v %v (Owner harus pertama dengan 1 pengguna)", roles, err)
	}
}

func TestUserRulesAndResolver(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	mail := func(n string) string { return fmt.Sprintf("%s-%d-%s@iam.test", n, os.Getpid(), e.tenant.String()[:6]) }

	kasirRole := e.role(t, e.owner, "Kasir", map[string][]string{"items": {"view"}})
	adminRole := e.role(t, e.owner, "Admin", map[string][]string{"items": {"view", "create"}, "users": {"create", "update"}})

	// Validasi dan keunikan.
	var fe FieldErrors
	_, err := e.svc.CreateUser(ctx, e.owner, CreateUserInput{Name: "A", Email: "bukan-email", Password: "lemah", RoleID: kasirRole.ID})
	if !errors.As(err, &fe) || fe["email"] == "" || fe["password"] == "" {
		t.Errorf("validasi: err = %v", err)
	}
	kasir := e.user(t, e.owner, mail("kasir"), kasirRole.ID)
	if _, err := e.svc.CreateUser(ctx, e.owner, CreateUserInput{Name: "Dobel", Email: mail("kasir"), Password: goodPassword, RoleID: kasirRole.ID}); !errors.Is(err, ErrEmailTaken) {
		t.Errorf("email ganda: err = %v", err)
	}
	if _, err := e.svc.CreateUser(ctx, e.owner, CreateUserInput{Name: "X", Email: mail("x"), Password: goodPassword, RoleID: uuid.New()}); !errors.As(err, &fe) || fe["role_id"] != "INVALID" {
		t.Errorf("role tak ada: err = %v", err)
	}

	// Izin efektif: dari DB, berubah segera setelah role diubah, dan hilang saat akun dinonaktifkan.
	perms, err := e.res.For(ctx, e.tenant, kasir.ID)
	if err != nil || !perms.Has("items", "view") || perms.Has("items", "create") {
		t.Fatalf("izin kasir: %+v %v", perms, err)
	}
	if _, err := e.svc.UpdateRole(ctx, e.owner, kasirRole.ID, "Kasir", map[string][]string{"items": {"view", "create"}}); err != nil {
		t.Fatal(err)
	}
	if perms, _ = e.res.For(ctx, e.tenant, kasir.ID); !perms.Has("items", "create") {
		t.Error("perubahan role harus langsung terlihat (cache tenant dibuang)")
	}
	if _, err := e.svc.UpdateUser(ctx, e.owner, kasir.ID, UpdateUserInput{Name: "Pegawai", RoleID: kasirRole.ID, Active: false}); err != nil {
		t.Fatal(err)
	}
	if _, err := e.res.For(ctx, e.tenant, kasir.ID); !errors.Is(err, ErrInactive) {
		t.Errorf("akun nonaktif: err = %v, want ErrInactive", err)
	}

	if roles, err := e.svc.AssignableRoles(ctx, Actor{TenantID: e.tenant, UserID: uuid.New(), Perms: adminRole.Permissions}); err != nil || len(roles) != 2 {
		t.Errorf("role yang dapat diberikan admin = %d (%v), want 2 (Kasir dan Admin; bukan Owner)", len(roles), err)
	}
	// Admin tidak boleh memberi role Owner, menyentuh pemilik, atau mengganti akunnya sendiri.
	adm := e.user(t, e.owner, mail("admin"), adminRole.ID)
	admin := Actor{TenantID: e.tenant, UserID: adm.ID, Perms: adminRole.Permissions}
	if _, err := e.svc.CreateUser(ctx, admin, CreateUserInput{Name: "Bos", Email: mail("bos"), Password: goodPassword, RoleID: e.ownerRol}); !errors.Is(err, ErrEscalation) {
		t.Errorf("admin memberi role Owner: err = %v", err)
	}
	if _, err := e.svc.UpdateUser(ctx, admin, e.owner.UserID, UpdateUserInput{Name: "Pemilik", RoleID: e.ownerRol, Active: false}); !errors.Is(err, ErrEscalation) {
		t.Errorf("admin menonaktifkan pemilik: err = %v", err)
	}
	if err := e.svc.ResetPassword(ctx, admin, e.owner.UserID, goodPassword); !errors.Is(err, ErrEscalation) {
		t.Errorf("admin reset password pemilik: err = %v", err)
	}
	if _, err := e.svc.UpdateUser(ctx, admin, adm.ID, UpdateUserInput{Name: "Admin", RoleID: adminRole.ID, Active: false}); !errors.Is(err, ErrSelfChange) {
		t.Errorf("menonaktifkan diri sendiri: err = %v", err)
	}

	// Pemilik aktif terakhir dilindungi.
	other := e.user(t, e.owner, mail("kasir2"), kasirRole.ID)
	if _, err := e.svc.UpdateUser(ctx, e.owner, e.owner.UserID, UpdateUserInput{Name: "Pemilik", RoleID: e.ownerRol, Active: true}); err != nil {
		t.Errorf("mengubah nama sendiri harus boleh: %v", err)
	}
	if _, err := e.svc.UpdateUser(ctx, e.owner, other.ID, UpdateUserInput{Name: "P", RoleID: e.ownerRol, Active: true}); err != nil {
		t.Fatalf("menjadikan pemilik kedua: %v", err)
	}
	if _, err := e.svc.UpdateUser(ctx, Actor{TenantID: e.tenant, UserID: other.ID, Perms: Permissions{All: true}}, e.owner.UserID, UpdateUserInput{Name: "Pemilik", RoleID: kasirRole.ID, Active: true}); err != nil {
		t.Fatalf("menurunkan pemilik pertama (masih ada pemilik lain): %v", err)
	}
	if _, err := e.svc.UpdateUser(ctx, e.owner, other.ID, UpdateUserInput{Name: "P", RoleID: kasirRole.ID, Active: true}); !errors.Is(err, ErrLastOwner) && !errors.Is(err, ErrSelfChange) {
		t.Errorf("pemilik terakhir: err = %v, want ErrLastOwner/ErrSelfChange", err)
	}
}

func TestLastOwnerRaceBetweenTwoOwners(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	second := e.user(t, e.owner, fmt.Sprintf("owner2-%d-%s@iam.test", os.Getpid(), e.tenant.String()[:6]), e.ownerRol)
	a1 := e.owner
	a2 := Actor{TenantID: e.tenant, UserID: second.ID, Perms: Permissions{All: true}}

	// Dua pemilik saling menonaktifkan bersamaan: tepat satu yang boleh berhasil.
	var wg sync.WaitGroup
	res := make(chan error, 2)
	for _, c := range []struct {
		by     Actor
		target uuid.UUID
	}{{a1, second.ID}, {a2, a1.UserID}} {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := e.svc.UpdateUser(ctx, c.by, c.target, UpdateUserInput{Name: "Pemilik", RoleID: e.ownerRol, Active: false})
			res <- err
		}()
	}
	wg.Wait()
	close(res)
	ok, blocked := 0, 0
	for err := range res {
		switch {
		case err == nil:
			ok++
		case errors.Is(err, ErrLastOwner):
			blocked++
		default:
			t.Errorf("error tak terduga: %v", err)
		}
	}
	if ok != 1 || blocked != 1 {
		t.Fatalf("ok=%d blocked=%d, want 1 dan 1", ok, blocked)
	}
	var active int
	if err := e.admin.QueryRow(ctx, `SELECT count(*) FROM users u JOIN roles r ON r.id = u.role_id WHERE u.tenant_id = $1 AND u.active AND r.is_system`, e.tenant).Scan(&active); err != nil || active != 1 {
		t.Fatalf("pemilik aktif = %d (%v), want 1", active, err)
	}
}

func TestTenantIsolationOfIAM(t *testing.T) {
	a, b := newEnv(t), newEnv(t)
	ctx := context.Background()
	roleB := b.role(t, b.owner, "Rahasia", map[string][]string{"items": {"view"}})
	userB := b.user(t, b.owner, fmt.Sprintf("b-%d-%s@iam.test", os.Getpid(), b.tenant.String()[:6]), roleB.ID)

	// Pemilik tenant A tidak dapat melihat, mengubah, atau memakai data tenant B walaupun mengetahui id-nya.
	if _, err := a.svc.UpdateRole(ctx, a.owner, roleB.ID, "Diretas", nil); !errors.Is(err, ErrNotFound) {
		t.Errorf("update role tenant lain: err = %v, want ErrNotFound", err)
	}
	if err := a.svc.DeleteRole(ctx, a.owner, roleB.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("hapus role tenant lain: err = %v, want ErrNotFound", err)
	}
	if _, err := a.svc.UpdateUser(ctx, a.owner, userB.ID, UpdateUserInput{Name: "X", RoleID: a.ownerRol, Active: false}); !errors.Is(err, ErrNotFound) {
		t.Errorf("update user tenant lain: err = %v, want ErrNotFound", err)
	}
	var fe FieldErrors
	if _, err := a.svc.CreateUser(ctx, a.owner, CreateUserInput{Name: "X", Email: fmt.Sprintf("c-%d@iam.test", os.Getpid()), Password: goodPassword, RoleID: roleB.ID}); !errors.As(err, &fe) || fe["role_id"] != "INVALID" {
		t.Errorf("memakai role tenant lain: err = %v, want role_id INVALID", err)
	}
	if err := a.svc.ResetPassword(ctx, a.owner, userB.ID, goodPassword); !errors.Is(err, ErrNotFound) {
		t.Errorf("reset password tenant lain: err = %v, want ErrNotFound", err)
	}
	if users, _ := a.svc.ListUsers(ctx, a.tenant); len(users) != 1 {
		t.Errorf("daftar user tenant A = %d, want 1", len(users))
	}
	// Dan resolver tenant A tidak mengenal user tenant B.
	if _, err := a.res.For(ctx, a.tenant, userB.ID); !errors.Is(err, ErrInactive) {
		t.Errorf("resolver lintas tenant: err = %v, want ErrInactive", err)
	}
}
