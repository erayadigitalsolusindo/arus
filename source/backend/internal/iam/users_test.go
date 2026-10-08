package iam

import (
	"context"
	"errors"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	"aciraba/internal/authz"
	pauth "aciraba/internal/platform/auth"
)

func TestUserRulesAndResolver(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()

	kasirRole := e.role(t, e.owner, "Kasir", map[string][]string{"items": {"view"}})
	adminRole := e.role(t, e.owner, "Admin", map[string][]string{"items": {"view", "create"}, "users": {"create", "update"}})

	// Validasi dan keunikan.
	var fe FieldErrors
	_, err := e.svc.CreateUser(ctx, e.owner, CreateUserInput{Name: "A", Email: "bukan-email", Password: "lemah", RoleID: kasirRole.ID, OutletIDs: []uuid.UUID{e.outlet}})
	if !errors.As(err, &fe) || fe["email"] == "" || fe["password"] == "" {
		t.Errorf("validasi: err = %v", err)
	}
	kasir := e.user(t, e.owner, e.mail("kasir"), kasirRole.ID)
	if _, err := e.svc.CreateUser(ctx, e.owner, CreateUserInput{Name: "Dobel", Email: e.mail("kasir"), Password: goodPassword, RoleID: kasirRole.ID, OutletIDs: []uuid.UUID{e.outlet}}); !errors.Is(err, ErrEmailTaken) {
		t.Errorf("email ganda: err = %v", err)
	}
	if _, err := e.svc.CreateUser(ctx, e.owner, CreateUserInput{Name: "X", Email: e.mail("x"), Password: goodPassword, RoleID: uuid.New(), OutletIDs: []uuid.UUID{e.outlet}}); !errors.As(err, &fe) || fe["role_id"] != "INVALID" {
		t.Errorf("role tak ada: err = %v", err)
	}
	// Pengguna non-pemilik wajib punya minimal satu outlet.
	if _, err := e.svc.CreateUser(ctx, e.owner, CreateUserInput{Name: "X", Email: e.mail("noout"), Password: goodPassword, RoleID: kasirRole.ID}); !errors.As(err, &fe) || fe["outlet_ids"] != "REQUIRED" {
		t.Errorf("tanpa outlet: err = %v", err)
	}

	// Izin efektif: dari DB, berubah segera setelah role diubah, dan hilang saat akun dinonaktifkan.
	acc, err := e.res.For(ctx, e.tenant, kasir.ID)
	if err != nil || !acc.Perms.Has("items", "view") || acc.Perms.Has("items", "create") || !acc.Outlets[e.outlet] {
		t.Fatalf("hak akses kasir: %+v %v", acc, err)
	}
	if _, err := e.svc.UpdateRole(ctx, e.owner, kasirRole.ID, "Kasir", map[string][]string{"items": {"view", "create"}}); err != nil {
		t.Fatal(err)
	}
	if acc, _ = e.res.For(ctx, e.tenant, kasir.ID); !acc.Perms.Has("items", "create") {
		t.Error("perubahan role harus langsung terlihat (cache tenant dibuang)")
	}
	if _, err := e.svc.UpdateUser(ctx, e.owner, kasir.ID, UpdateUserInput{Name: "Pegawai", RoleID: kasirRole.ID, Active: false, OutletIDs: []uuid.UUID{e.outlet}}); err != nil {
		t.Fatal(err)
	}
	if _, err := e.res.For(ctx, e.tenant, kasir.ID); !errors.Is(err, authz.ErrInactive) {
		t.Errorf("akun nonaktif: err = %v, want ErrInactive", err)
	}

	// Admin tidak boleh memberi role Owner, menyentuh pemilik, atau mengganti akunnya sendiri.
	adm := e.user(t, e.owner, e.mail("admin"), adminRole.ID)
	admin := e.actor(adm.ID, adminRole.Permissions)
	if _, err := e.svc.CreateUser(ctx, admin, CreateUserInput{Name: "Bos", Email: e.mail("bos"), Password: goodPassword, RoleID: e.ownerRol}); !errors.Is(err, ErrEscalation) {
		t.Errorf("admin memberi role Owner: err = %v", err)
	}
	if _, err := e.svc.UpdateUser(ctx, admin, e.owner.UserID, UpdateUserInput{Name: "Pemilik", RoleID: e.ownerRol, Active: false}); !errors.Is(err, ErrEscalation) {
		t.Errorf("admin menonaktifkan pemilik: err = %v", err)
	}
	if err := e.svc.ResetPassword(ctx, admin, e.owner.UserID, goodPassword); !errors.Is(err, ErrEscalation) {
		t.Errorf("admin reset password pemilik: err = %v", err)
	}
	if _, err := e.svc.UpdateUser(ctx, admin, adm.ID, UpdateUserInput{Name: "Admin", RoleID: adminRole.ID, Active: false, OutletIDs: []uuid.UUID{e.outlet}}); !errors.Is(err, ErrSelfChange) {
		t.Errorf("menonaktifkan diri sendiri: err = %v", err)
	}

	// Pemilik aktif terakhir dilindungi.
	other := e.user(t, e.owner, e.mail("kasir2"), kasirRole.ID)
	if _, err := e.svc.UpdateUser(ctx, e.owner, e.owner.UserID, UpdateUserInput{Name: "Pemilik", RoleID: e.ownerRol, Active: true}); err != nil {
		t.Errorf("mengubah nama sendiri harus boleh: %v", err)
	}
	if _, err := e.svc.UpdateUser(ctx, e.owner, other.ID, UpdateUserInput{Name: "P", RoleID: e.ownerRol, Active: true}); err != nil {
		t.Fatalf("menjadikan pemilik kedua: %v", err)
	}
	second := e.actor(other.ID, authz.Permissions{All: true})
	if _, err := e.svc.UpdateUser(ctx, second, e.owner.UserID, UpdateUserInput{Name: "Pemilik", RoleID: kasirRole.ID, Active: true, OutletIDs: []uuid.UUID{e.outlet}}); err != nil {
		t.Fatalf("menurunkan pemilik pertama (masih ada pemilik lain): %v", err)
	}
	if _, err := e.svc.UpdateUser(ctx, e.owner, other.ID, UpdateUserInput{Name: "P", RoleID: kasirRole.ID, Active: true, OutletIDs: []uuid.UUID{e.outlet}}); !errors.Is(err, ErrLastOwner) && !errors.Is(err, ErrSelfChange) {
		t.Errorf("pemilik terakhir: err = %v, want ErrLastOwner/ErrSelfChange", err)
	}
}

func TestOutletAssignmentRules(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	out2, out3 := uuid.New(), uuid.New()
	for _, o := range []uuid.UUID{out2, out3} {
		if _, err := e.admin.Exec(ctx, `INSERT INTO outlets (id, tenant_id, code, name) VALUES ($1, $2, $3, 'Cabang')`, o, e.tenant, o.String()[:8]); err != nil {
			t.Fatal(err)
		}
	}
	role := e.role(t, e.owner, "Kasir", map[string][]string{"items": {"view"}})
	managerRole := e.role(t, e.owner, "Manajer", map[string][]string{"users": {"view", "create", "update"}, "items": {"view"}})

	// Manajer hanya memegang outlet utama + cabang 2: tidak boleh menugaskan cabang 3.
	mgr := e.user(t, e.owner, e.mail("mgr"), managerRole.ID)
	if _, err := e.svc.UpdateUser(ctx, e.owner, mgr.ID, UpdateUserInput{Name: "Manajer", RoleID: managerRole.ID, Active: true, OutletIDs: []uuid.UUID{e.outlet, out2}}); err != nil {
		t.Fatal(err)
	}
	manager := authz.Actor{TenantID: e.tenant, UserID: mgr.ID, OutletID: e.outlet, Name: "Manajer", Perms: managerRole.Permissions, Outlets: map[uuid.UUID]bool{e.outlet: true, out2: true}}
	if _, err := e.svc.CreateUser(ctx, manager, CreateUserInput{Name: "K", Email: e.mail("k3"), Password: goodPassword, RoleID: role.ID, OutletIDs: []uuid.UUID{out3}}); !errors.Is(err, ErrOutletForbidden) {
		t.Errorf("menugaskan outlet di luar jangkauan: err = %v, want ErrOutletForbidden", err)
	}
	k, err := e.svc.CreateUser(ctx, manager, CreateUserInput{Name: "K", Email: e.mail("k2"), Password: goodPassword, RoleID: role.ID, OutletIDs: []uuid.UUID{out2}})
	if err != nil {
		t.Fatal(err)
	}
	if len(k.OutletIDs) != 1 || k.OutletIDs[0] != out2 {
		t.Errorf("outlet kasir = %v, want [cabang 2]", k.OutletIDs)
	}

	// Pemilik menambah cabang 3; manajer kemudian mengubah kasir tanpa menghapus penugasan di luar jangkauannya.
	if _, err := e.svc.UpdateUser(ctx, e.owner, k.ID, UpdateUserInput{Name: "K", RoleID: role.ID, Active: true, OutletIDs: []uuid.UUID{out2, out3}}); err != nil {
		t.Fatal(err)
	}
	got, err := e.svc.UpdateUser(ctx, manager, k.ID, UpdateUserInput{Name: "K2", RoleID: role.ID, Active: true, OutletIDs: []uuid.UUID{e.outlet}})
	if err != nil {
		t.Fatal(err)
	}
	if len(got.OutletIDs) != 2 || !slices.Contains(got.OutletIDs, e.outlet) || !slices.Contains(got.OutletIDs, out3) || slices.Contains(got.OutletIDs, out2) {
		t.Errorf("outlet kasir = %v, want [utama, cabang 3]: manajer mengganti yang ia kelola (cabang 2 -> utama) dan cabang 3 di luar jangkauannya dipertahankan", got.OutletIDs)
	}
	acc, _ := e.res.For(ctx, e.tenant, k.ID)
	if len(acc.Outlets) == 0 {
		t.Error("hak akses outlet kasir kosong")
	}

	// Outlet yang dinonaktifkan hilang dari hak akses.
	if _, err := e.admin.Exec(ctx, `UPDATE outlets SET active = false WHERE id = $1`, out3); err != nil {
		t.Fatal(err)
	}
	e.res.InvalidateTenant(e.tenant)
	if acc, _ = e.res.For(ctx, e.tenant, k.ID); acc.Outlets[out3] {
		t.Error("outlet nonaktif tidak boleh bisa diakses")
	}
}

func TestPasswordResetAndDeactivationRevokeEverySession(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	role := e.role(t, e.owner, "Kasir", map[string][]string{"items": {"view"}})
	u := e.user(t, e.owner, e.mail("sesi"), role.ID)

	// Dua perangkat masuk: dua rantai sesi.
	t1, err := e.sessions.Create(ctx, u.ID.String(), true)
	if err != nil {
		t.Fatal(err)
	}
	t2, err := e.sessions.Create(ctx, u.ID.String(), false)
	if err != nil {
		t.Fatal(err)
	}
	// Salah satu sudah dirotasi (token lama masih dalam jendela grace): tidak boleh hidup lagi setelah pencabutan.
	rot, err := e.sessions.Rotate(ctx, t1)
	if err != nil || rot.Status != pauth.RotateOK {
		t.Fatalf("rotasi: %+v %v", rot, err)
	}

	before := time.Now().Add(-time.Second)
	if err := e.svc.ResetPassword(ctx, e.owner, u.ID, "password-baru-456"); err != nil {
		t.Fatal(err)
	}
	for name, tok := range map[string]string{"perangkat 1 (terbaru)": rot.Token, "perangkat 1 (token lama, grace)": t1, "perangkat 2": t2} {
		res, err := e.sessions.Rotate(ctx, tok)
		if err != nil || res.Status != pauth.RotateInvalid {
			t.Errorf("%s: status %v (%v), want RotateInvalid setelah reset password", name, res.Status, err)
		}
	}
	// Token akses lama dicabut lewat tokens_valid_after.
	acc, err := e.res.For(ctx, e.tenant, u.ID)
	if err != nil || !acc.ValidAfter.After(before) {
		t.Errorf("ValidAfter = %v (%v), want sesudah %v", acc.ValidAfter, err, before)
	}
}

func TestDeactivationRevokesSessions(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	role := e.role(t, e.owner, "Kasir", map[string][]string{"items": {"view"}})
	u := e.user(t, e.owner, e.mail("nonaktif"), role.ID)
	tok, _ := e.sessions.Create(ctx, u.ID.String(), true)

	if _, err := e.svc.UpdateUser(ctx, e.owner, u.ID, UpdateUserInput{Name: "Pegawai", RoleID: role.ID, Active: false, OutletIDs: []uuid.UUID{e.outlet}}); err != nil {
		t.Fatal(err)
	}
	if res, _ := e.sessions.Rotate(ctx, tok); res.Status != pauth.RotateInvalid {
		t.Errorf("sesi pengguna nonaktif masih hidup: %v", res.Status)
	}
	if n := e.count(t, `SELECT count(*) FROM users WHERE id = $1 AND tokens_valid_after > now() - interval '5 seconds'`, u.ID); n != 1 {
		t.Error("tokens_valid_after harus diperbarui saat dinonaktifkan")
	}
	if n := e.count(t, `SELECT count(*) FROM audit_log WHERE tenant_id = $1 AND action = 'user.update' AND details->'after'->>'active' = 'false'`, e.tenant); n != 1 {
		t.Errorf("audit penonaktifan = %d, want 1", n)
	}
}

func TestLastOwnerRaceBetweenTwoOwners(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	second := e.user(t, e.owner, e.mail("owner2"), e.ownerRol)
	a1 := e.owner
	a2 := e.actor(second.ID, authz.Permissions{All: true})

	// Dua pemilik saling menonaktifkan bersamaan: tepat satu yang boleh berhasil.
	var wg sync.WaitGroup
	res := make(chan error, 2)
	for _, c := range []struct {
		by     authz.Actor
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
	if n := e.count(t, `SELECT count(*) FROM users u JOIN roles r ON r.id = u.role_id WHERE u.tenant_id = $1 AND u.active AND r.is_system`, e.tenant); n != 1 {
		t.Fatalf("pemilik aktif = %d, want 1", n)
	}
}

func TestManualEmailVerification(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	kasirRole := e.role(t, e.owner, "Kasir", map[string][]string{"items": {"view"}})
	adminRole := e.role(t, e.owner, "Admin", map[string][]string{"items": {"view"}, "users": {"view", "update"}})
	u := e.user(t, e.owner, e.mail("belum"), kasirRole.ID)
	adm := e.user(t, e.owner, e.mail("adm"), adminRole.ID)
	admin := e.actor(adm.ID, adminRole.Permissions)

	verified := func(id uuid.UUID) bool {
		var ok bool
		if err := e.admin.QueryRow(ctx, `SELECT email_verified_at IS NOT NULL FROM users WHERE id = $1`, id).Scan(&ok); err != nil {
			t.Fatal(err)
		}
		return ok
	}
	if verified(u.ID) {
		t.Fatal("pengguna baru seharusnya belum terverifikasi")
	}
	if err := e.svc.VerifyEmail(ctx, admin, adm.ID); !errors.Is(err, ErrSelfVerify) {
		t.Errorf("verifikasi akun sendiri: err = %v, want ErrSelfVerify", err)
	}
	if err := e.svc.VerifyEmail(ctx, admin, e.owner.UserID); !errors.Is(err, ErrEscalation) {
		t.Errorf("admin memverifikasi pemilik: err = %v, want ErrEscalation", err)
	}
	if err := e.svc.VerifyEmail(ctx, admin, uuid.New()); !errors.Is(err, ErrNotFound) {
		t.Errorf("pengguna tak ada: err = %v, want ErrNotFound", err)
	}
	if err := e.svc.VerifyEmail(ctx, admin, u.ID); err != nil {
		t.Fatal(err)
	}
	if !verified(u.ID) {
		t.Error("email seharusnya terverifikasi")
	}
	// Panggilan ulang tidak berefek dan tidak menggandakan audit.
	if err := e.svc.VerifyEmail(ctx, admin, u.ID); err != nil {
		t.Fatal(err)
	}
	var n int
	if err := e.admin.QueryRow(ctx, `SELECT count(*) FROM audit_log WHERE tenant_id = $1 AND action = 'user.email_verify' AND entity_id = $2 AND details->>'manual' = 'true'`, e.tenant, u.ID.String()).Scan(&n); err != nil || n != 1 {
		t.Errorf("audit verifikasi manual = %d (%v), want 1", n, err)
	}
}
