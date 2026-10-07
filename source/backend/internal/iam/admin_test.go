package iam

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"

	"aciraba/internal/authz"
)

// Administrator = sifat role (izin "*"), bukan role sistem: dapat dibuat/diubah/dihapus secara dinamis, memberi akses
// semua menu/aksi dan semua outlet tanpa penugasan outlet.
func TestAdministratorRoleIsDynamicAndUnfiltered(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()

	adminRole, err := e.svc.CreateRole(ctx, e.owner, "Administrator", nil, true)
	if err != nil {
		t.Fatal(err)
	}
	if !adminRole.Permissions.All || adminRole.IsSystem {
		t.Fatalf("role administrator: %+v (harus akses penuh dan BUKAN role sistem)", adminRole)
	}
	// Perms yang dikirim bersama flag diabaikan; wildcard lewat peta izin tetap ditolak.
	if r, err := e.svc.CreateRole(ctx, e.owner, "Admin Dua", map[string][]string{"items": {"view"}}, true); err != nil || !r.Permissions.All {
		t.Errorf("flag administrator harus menang atas peta izin: %+v %v", r, err)
	}
	var fe FieldErrors
	if _, err := e.svc.CreateRole(ctx, e.owner, "Curang", map[string][]string{"*": {"view"}}, false); !errors.As(err, &fe) {
		t.Errorf("wildcard lewat peta izin harus tetap ditolak: %v", err)
	}

	// Pengguna Administrator tidak wajib punya penugasan outlet dan otomatis melihat semua outlet aktif (termasuk yang baru).
	adm, err := e.svc.CreateUser(ctx, e.owner, CreateUserInput{Name: "Admin", Email: e.mail("adm"), Password: goodPassword, RoleID: adminRole.ID})
	if err != nil {
		t.Fatalf("administrator tanpa outlet: %v", err)
	}
	if !adm.AllOutlets || len(adm.OutletIDs) != 0 {
		t.Errorf("AllOutlets=%v outlet_ids=%v, want true dan kosong", adm.AllOutlets, adm.OutletIDs)
	}
	newOutlet := uuid.New()
	if _, err := e.admin.Exec(ctx, `INSERT INTO outlets (id, tenant_id, code, name) VALUES ($1, $2, 'baru', 'Cabang Baru')`, newOutlet, e.tenant); err != nil {
		t.Fatal(err)
	}
	e.res.InvalidateTenant(e.tenant)
	acc, err := e.res.For(ctx, e.tenant, adm.ID)
	if err != nil || !acc.Perms.Has("modul_apa_saja", "aksi_apa_saja") || !acc.Outlets[e.outlet] || !acc.Outlets[newOutlet] {
		t.Fatalf("hak akses administrator: %+v %v", acc, err)
	}

	// Administrator boleh melakukan semua hal (tanpa filter): membuat role akses penuh, memberi role apa pun, dan
	// menugaskan outlet mana pun.
	actor := authz.Actor{TenantID: e.tenant, UserID: adm.ID, OutletID: e.outlet, Name: "Admin", Perms: acc.Perms, Outlets: acc.Outlets}
	if _, err := e.svc.CreateRole(ctx, actor, "Admin Tiga", nil, true); err != nil {
		t.Errorf("administrator membuat role akses penuh: %v", err)
	}
	kasir := e.role(t, e.owner, "Kasir", map[string][]string{"items": {"view"}})
	if _, err := e.svc.CreateUser(ctx, actor, CreateUserInput{Name: "K", Email: e.mail("k"), Password: goodPassword, RoleID: kasir.ID, OutletIDs: []uuid.UUID{newOutlet}}); err != nil {
		t.Errorf("administrator menugaskan outlet baru: %v", err)
	}

	// Non-administrator tidak bisa membuat, memberikan, atau menyentuh role akses penuh (anti-eskalasi).
	managerRole := e.role(t, e.owner, "Manajer", map[string][]string{"roles": {"view", "create", "update", "delete"}, "users": {"view", "create", "update"}})
	manager := e.actor(uuid.New(), managerRole.Permissions)
	if _, err := e.svc.CreateRole(ctx, manager, "Mau Admin", nil, true); !errors.Is(err, ErrEscalation) {
		t.Errorf("non-administrator membuat role akses penuh: err = %v, want ErrEscalation", err)
	}
	if _, err := e.svc.UpdateRole(ctx, manager, adminRole.ID, "Administrator", map[string][]string{"items": {"view"}}, false); !errors.Is(err, ErrEscalation) {
		t.Errorf("non-administrator menurunkan role administrator: err = %v, want ErrEscalation", err)
	}
	if err := e.svc.DeleteRole(ctx, manager, adminRole.ID); !errors.Is(err, ErrEscalation) {
		t.Errorf("non-administrator menghapus role administrator: err = %v, want ErrEscalation", err)
	}
	if _, err := e.svc.CreateUser(ctx, manager, CreateUserInput{Name: "X", Email: e.mail("x"), Password: goodPassword, RoleID: adminRole.ID}); !errors.Is(err, ErrEscalation) {
		t.Errorf("non-administrator memberi role administrator: err = %v, want ErrEscalation", err)
	}
	if _, err := e.svc.UpdateUser(ctx, manager, adm.ID, UpdateUserInput{Name: "Admin", RoleID: adminRole.ID, Active: false}); !errors.Is(err, ErrEscalation) {
		t.Errorf("non-administrator menonaktifkan administrator: err = %v, want ErrEscalation", err)
	}

	// Dinamis: menurunkan role menjadi biasa langsung mencabut akses semua outlet (pengguna kini butuh penugasan).
	if _, err := e.svc.UpdateRole(ctx, e.owner, adminRole.ID, "Administrator", map[string][]string{"items": {"view"}}, false); err != nil {
		t.Fatal(err)
	}
	acc, _ = e.res.For(ctx, e.tenant, adm.ID)
	if acc.Perms.All || len(acc.Outlets) != 0 {
		t.Errorf("setelah diturunkan: All=%v outlet=%d, want false dan 0", acc.Perms.All, len(acc.Outlets))
	}
	// ...dan role-nya sendiri dapat dihapus setelah tidak dipakai (bukan role sistem).
	if _, err := e.svc.UpdateUser(ctx, e.owner, adm.ID, UpdateUserInput{Name: "Admin", RoleID: kasir.ID, Active: true, OutletIDs: []uuid.UUID{e.outlet}}); err != nil {
		t.Fatal(err)
	}
	if err := e.svc.DeleteRole(ctx, e.owner, adminRole.ID); err != nil {
		t.Errorf("hapus role administrator yang tak dipakai: %v", err)
	}
}
