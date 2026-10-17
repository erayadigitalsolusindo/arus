package iam

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"

	"aciraba/internal/plan"
)

// Batas pengguna paket: pengguna ke-(max+1) ditolak, mengaktifkan kembali pengguna nonaktif juga memakai slot,
// dan pengguna nonaktif tidak dihitung.
func TestPlanUserLimit(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	if _, err := e.admin.Exec(ctx, `UPDATE plans SET max_users = 2 WHERE id = $1`, e.plan); err != nil {
		t.Fatal(err)
	} // owner + 1
	role := e.role(t, e.owner, "Kasir", map[string][]string{"items": {"view"}})
	kasir := e.user(t, e.owner, e.mail("kasir"), role.ID)

	_, err := e.svc.CreateUser(ctx, e.owner, CreateUserInput{Name: "Lagi", Email: e.mail("lagi"), Password: goodPassword, RoleID: role.ID, OutletIDs: []uuid.UUID{e.outlet}})
	if !errors.Is(err, plan.ErrUserLimit) {
		t.Fatalf("pengguna ke-3 = %v, want ErrUserLimit", err)
	}
	// Nonaktifkan kasir -> slot bebas -> bisa tambah satu; mengaktifkan kembali kasir lama kini melewati batas.
	if _, err := e.svc.UpdateUser(ctx, e.owner, kasir.ID, UpdateUserInput{Name: "Pegawai", RoleID: role.ID, Active: false, OutletIDs: []uuid.UUID{e.outlet}}); err != nil {
		t.Fatal(err)
	}
	e.user(t, e.owner, e.mail("baru"), role.ID)
	_, err = e.svc.UpdateUser(ctx, e.owner, kasir.ID, UpdateUserInput{Name: "Pegawai", RoleID: role.ID, Active: true, OutletIDs: []uuid.UUID{e.outlet}})
	if !errors.Is(err, plan.ErrUserLimit) {
		t.Fatalf("aktifkan kembali saat penuh = %v, want ErrUserLimit", err)
	}
}
