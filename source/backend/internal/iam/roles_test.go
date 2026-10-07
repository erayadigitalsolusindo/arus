package iam

import (
	"context"
	"errors"
	"fmt"
	"os"
	"testing"

	"github.com/google/uuid"

	"aciraba/internal/authz"
)

func TestRoleLifecycleAndEscalation(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()

	kasir := e.role(t, e.owner, "Kasir", map[string][]string{"items": {"view"}, "sales_orders": {"create"}})
	if !kasir.Permissions.Has("sales_orders", "view") {
		t.Error("view harus otomatis menyertai aksi lain")
	}
	if _, err := e.svc.CreateRole(ctx, e.owner, "Kasir", nil, false); !errors.Is(err, ErrNameTaken) {
		t.Errorf("nama ganda err = %v, want ErrNameTaken", err)
	}
	var fe FieldErrors
	if _, err := e.svc.CreateRole(ctx, e.owner, "X", map[string][]string{"*": {"view"}}, false); !errors.As(err, &fe) || fe["permissions"] == "" {
		t.Errorf("wildcard lewat API harus ditolak sebagai VALIDATION, err = %v", err)
	}
	if _, err := e.svc.CreateRole(ctx, e.owner, "", nil, false); !errors.As(err, &fe) || fe["name"] == "" {
		t.Errorf("nama kosong harus ditolak, err = %v", err)
	}

	// Admin (bukan Owner) hanya boleh memberi izin yang ia miliki.
	adminRole := e.role(t, e.owner, "Admin", map[string][]string{"items": {"view", "create"}, "roles": {"create", "update", "delete"}, "users": {"create", "update"}})
	admin := e.actor(uuid.New(), adminRole.Permissions)
	if _, err := e.svc.CreateRole(ctx, admin, "Curang", map[string][]string{"items": {"delete"}}, false); !errors.Is(err, ErrEscalation) {
		t.Errorf("admin memberi izin di luar miliknya: err = %v, want ErrEscalation", err)
	}
	magang := e.role(t, e.owner, "Magang", map[string][]string{"items": {"view"}})
	if _, err := e.svc.UpdateRole(ctx, admin, magang.ID, "Magang", map[string][]string{"items": {"view", "create"}}, false); err != nil {
		t.Errorf("admin mengubah role dalam jangkauannya: %v", err)
	}
	// Kasir punya izin yang tidak dimiliki admin (sales_orders): di luar jangkauan.
	if _, err := e.svc.UpdateRole(ctx, admin, kasir.ID, "Kasir", nil, false); !errors.Is(err, ErrEscalation) {
		t.Errorf("mengubah role di luar jangkauan: err = %v, want ErrEscalation", err)
	}
	super := e.role(t, e.owner, "Super", map[string][]string{"items": {"delete"}})
	if _, err := e.svc.UpdateRole(ctx, admin, super.ID, "Super", map[string][]string{}, false); !errors.Is(err, ErrEscalation) {
		t.Errorf("mengubah role atasan: err = %v, want ErrEscalation", err)
	}
	if err := e.svc.DeleteRole(ctx, admin, super.ID); !errors.Is(err, ErrEscalation) {
		t.Errorf("menghapus role atasan: err = %v, want ErrEscalation", err)
	}

	// Role sistem Owner kebal.
	if _, err := e.svc.UpdateRole(ctx, e.owner, e.ownerRol, "Owner", map[string][]string{"items": {"view"}}, false); !errors.Is(err, ErrSystemRole) {
		t.Errorf("ubah Owner: err = %v, want ErrSystemRole", err)
	}
	if err := e.svc.DeleteRole(ctx, e.owner, e.ownerRol); !errors.Is(err, ErrSystemRole) {
		t.Errorf("hapus Owner: err = %v, want ErrSystemRole", err)
	}

	// Role yang dipakai tidak bisa dihapus; yang kosong bisa.
	e.user(t, e.owner, e.mail("kasir"), kasir.ID)
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

	// Setiap perubahan tercatat di audit log dalam transaksi yang sama.
	for action, want := range map[string]int{"role.create": 4, "role.update": 1, "role.delete": 1} {
		if n := e.count(t, `SELECT count(*) FROM audit_log WHERE tenant_id = $1 AND action = $2`, e.tenant, action); n != want {
			t.Errorf("audit %s = %d, want %d", action, n, want)
		}
	}
}

func TestAssignableRolesAreLimitedToActorsPermissions(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	e.role(t, e.owner, "Kasir", map[string][]string{"items": {"view"}})
	adminRole := e.role(t, e.owner, "Admin", map[string][]string{"items": {"view", "create"}, "users": {"view", "create"}})
	e.role(t, e.owner, "Super", map[string][]string{"items": {"delete"}})

	roles, err := e.svc.AssignableRoles(ctx, e.actor(uuid.New(), adminRole.Permissions))
	if err != nil || len(roles) != 2 {
		t.Errorf("role yang dapat diberikan admin = %d (%v), want 2 (Kasir dan Admin; bukan Owner/Super)", len(roles), err)
	}
}

func TestTenantIsolationOfIAM(t *testing.T) {
	a, b := newEnv(t), newEnv(t)
	ctx := context.Background()
	roleB := b.role(t, b.owner, "Rahasia", map[string][]string{"items": {"view"}})
	userB := b.user(t, b.owner, fmt.Sprintf("b-%d-%s@iam.test", os.Getpid(), b.tenant.String()[:6]), roleB.ID)

	// Pemilik tenant A tidak dapat melihat, mengubah, atau memakai data tenant B walaupun mengetahui id-nya.
	if _, err := a.svc.UpdateRole(ctx, a.owner, roleB.ID, "Diretas", nil, false); !errors.Is(err, ErrNotFound) {
		t.Errorf("update role tenant lain: err = %v, want ErrNotFound", err)
	}
	if err := a.svc.DeleteRole(ctx, a.owner, roleB.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("hapus role tenant lain: err = %v, want ErrNotFound", err)
	}
	if _, err := a.svc.UpdateUser(ctx, a.owner, userB.ID, UpdateUserInput{Name: "X", RoleID: a.ownerRol, Active: false, OutletIDs: []uuid.UUID{a.outlet}}); !errors.Is(err, ErrNotFound) {
		t.Errorf("update user tenant lain: err = %v, want ErrNotFound", err)
	}
	var fe FieldErrors
	_, err := a.svc.CreateUser(ctx, a.owner, CreateUserInput{Name: "X", Email: fmt.Sprintf("c-%d@iam.test", os.Getpid()), Password: goodPassword, RoleID: roleB.ID, OutletIDs: []uuid.UUID{a.outlet}})
	if !errors.As(err, &fe) || fe["role_id"] != "INVALID" {
		t.Errorf("memakai role tenant lain: err = %v, want role_id INVALID", err)
	}
	if err := a.svc.ResetPassword(ctx, a.owner, userB.ID, goodPassword); !errors.Is(err, ErrNotFound) {
		t.Errorf("reset password tenant lain: err = %v, want ErrNotFound", err)
	}
	if users, _ := a.svc.ListUsers(ctx, a.tenant); len(users) != 1 {
		t.Errorf("daftar user tenant A = %d, want 1", len(users))
	}
	if _, err := a.res.For(ctx, a.tenant, userB.ID); !errors.Is(err, authz.ErrInactive) {
		t.Errorf("resolver lintas tenant: err = %v, want ErrInactive", err)
	}
	// Menugaskan outlet milik tenant lain ditolak (aplikasi dan FK komposit).
	if _, err := a.svc.CreateUser(ctx, a.owner, CreateUserInput{Name: "X", Email: fmt.Sprintf("d-%d@iam.test", os.Getpid()), Password: goodPassword, RoleID: a.role(t, a.owner, "K", nil).ID, OutletIDs: []uuid.UUID{b.outlet}}); err == nil {
		t.Error("outlet tenant lain seharusnya ditolak")
	}
}
