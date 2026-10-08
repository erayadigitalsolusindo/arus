package auth

import (
	"context"
	"errors"
	"fmt"
	"os"
	"testing"

	"github.com/google/uuid"

	"aciraba/internal/approval"
	pauth "aciraba/internal/platform/auth"
)

// addOutlet menambah outlet aktif ke tenant lewat pool admin.
func addOutlet(t *testing.T, h *harness, tenant, name string) uuid.UUID {
	t.Helper()
	id := uuid.New()
	if _, err := h.admin.Exec(context.Background(), `INSERT INTO outlets (id, tenant_id, code, name) VALUES ($1, $2, $3, $4)`, id, tenant, id.String()[:8], name); err != nil {
		t.Fatal(err)
	}
	return id
}

func TestSwitchOutletPersistsAcrossRefreshAndEnforcesAccess(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	email, sess := registered(t, h, "Pindah")
	out2 := addOutlet(t, h, sess.Tenant.ID, "Cabang 2")

	login, err := h.svc.Login(ctx, LoginInput{Email: email, Password: "sandi-aman-123", Remember: true})
	if err != nil {
		t.Fatal(err)
	}
	a := actorOf(t, h, login)
	if !a.Outlets[out2] {
		t.Fatal("pemilik harus punya akses ke outlet baru")
	}

	switched, err := h.svc.SwitchOutlet(ctx, a, login.RefreshToken, out2)
	if err != nil {
		t.Fatal(err)
	}
	if switched.Outlet.ID != out2.String() || switched.RefreshToken != "" {
		t.Fatalf("pindah outlet: %+v (refresh token tidak boleh dirotasi)", switched.Outlet)
	}
	claims, _ := h.svc.Tokens.Parse(switched.AccessToken)
	if claims.OutletID != out2.String() {
		t.Errorf("klaim oid = %s, want %s", claims.OutletID, out2)
	}

	// Refresh mempertahankan outlet pilihan sesi.
	ref, err := h.svc.Refresh(ctx, login.RefreshToken)
	if err != nil {
		t.Fatal(err)
	}
	if ref.Outlet.ID != out2.String() {
		t.Errorf("setelah refresh outlet = %s, want %s (pilihan sesi hilang)", ref.Outlet.ID, out2)
	}
	// Login baru (sesi lain) kembali ke outlet pertama: pilihan outlet per sesi, bukan global.
	other, _ := h.svc.Login(ctx, LoginInput{Email: email, Password: "sandi-aman-123"})
	if other.Outlet.ID != sess.Outlet.ID {
		t.Errorf("sesi lain outlet = %s, want %s", other.Outlet.ID, sess.Outlet.ID)
	}

	// Outlet milik tenant lain / tidak ada ditolak.
	if _, err := h.svc.SwitchOutlet(ctx, a, ref.RefreshToken, uuid.New()); !errors.Is(err, ErrOutletForbidden) {
		t.Errorf("outlet asing: err = %v, want ErrOutletForbidden", err)
	}
	// Refresh token milik pengguna lain tidak boleh dipakai bersama token akses ini.
	_, otherSess := registered(t, h, "Tetangga")
	if _, err := h.svc.SwitchOutlet(ctx, a, otherSess.RefreshToken, out2); !errors.Is(err, ErrInvalidSession) {
		t.Errorf("cookie milik orang lain: err = %v, want ErrInvalidSession", err)
	}
	if n := count(t, h.admin, `SELECT count(*) FROM audit_log WHERE tenant_id = $1 AND action = 'auth.outlet_switch'`, sess.Tenant.ID); n != 1 {
		t.Errorf("audit pindah outlet = %d, want 1", n)
	}
}

func TestUserWithoutAssignedOutletCannotSwitchOrLogin(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	_, owner := registered(t, h, "Cabang")
	out2 := addOutlet(t, h, owner.Tenant.ID, "Cabang 2")

	// Kasir hanya ditugaskan ke outlet utama.
	hash, err := pauth.HashPassword("kasir-pass-123")
	if err != nil {
		t.Fatal(err)
	}
	roleID, userID := uuid.New(), uuid.New()
	kasirEmail := fmt.Sprintf("kasir-%d@rec.test", os.Getpid())
	for _, q := range []struct {
		sql  string
		args []any
	}{
		{`INSERT INTO roles (id, tenant_id, name, permissions) VALUES ($1, $2, 'Kasir', '{}')`, []any{roleID, owner.Tenant.ID}},
		{`INSERT INTO users (id, tenant_id, role_id, email, name, password_hash) VALUES ($1, $2, $3, $4, 'Kasir', $5)`, []any{userID, owner.Tenant.ID, roleID, kasirEmail, hash}},
		{`INSERT INTO user_outlets (tenant_id, user_id, outlet_id) VALUES ($1, $2, $3)`, []any{owner.Tenant.ID, userID, owner.Outlet.ID}},
	} {
		if _, err := h.admin.Exec(ctx, q.sql, q.args...); err != nil {
			t.Fatal(err)
		}
	}
	login, err := h.svc.Login(ctx, LoginInput{Email: kasirEmail, Password: "kasir-pass-123"})
	if err != nil {
		t.Fatal(err)
	}
	if login.Outlet.ID != owner.Outlet.ID {
		t.Fatalf("outlet kasir = %s, want outlet utama", login.Outlet.ID)
	}
	a := actorOf(t, h, login)
	if _, err := h.svc.SwitchOutlet(ctx, a, login.RefreshToken, out2); !errors.Is(err, ErrOutletForbidden) {
		t.Errorf("kasir pindah ke outlet yang bukan haknya: err = %v, want ErrOutletForbidden", err)
	}

	// Kehilangan semua outlet aktif: login ditolak dengan alasan khusus (setelah password terbukti), dan refresh gagal.
	if _, err := h.admin.Exec(ctx, `DELETE FROM user_outlets WHERE user_id = $1`, userID); err != nil {
		t.Fatal(err)
	}
	if _, err := h.svc.Login(ctx, LoginInput{Email: kasirEmail, Password: "kasir-pass-123"}); !errors.Is(err, ErrNoOutlet) {
		t.Errorf("tanpa outlet: err = %v, want ErrNoOutlet", err)
	}
	if _, err := h.svc.Login(ctx, LoginInput{Email: kasirEmail, Password: "salah-salah-123"}); !errors.Is(err, ErrInvalidCredentials) {
		t.Errorf("password salah tanpa outlet harus tetap INVALID_CREDENTIALS, got %v", err)
	}
	if _, err := h.svc.Refresh(ctx, login.RefreshToken); !errors.Is(err, ErrInvalidSession) {
		t.Errorf("refresh tanpa outlet: err = %v, want ErrInvalidSession", err)
	}
}

// Owner (role sistem, izin "*") mengakses semua outlet aktif tanpa penugasan user_outlets, termasuk outlet yang baru ditambah.
func TestOwnerAccessesEveryOutletWithoutAssignment(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	_, owner := registered(t, h, "Admin")
	out2 := addOutlet(t, h, owner.Tenant.ID, "Cabang 2")

	a := actorOf(t, h, owner)
	if !a.Perms.All || !a.Outlets[out2] || !a.Outlets[uuid.MustParse(owner.Outlet.ID)] {
		t.Fatalf("akses owner: all=%v outlets=%v", a.Perms.All, a.Outlets)
	}
	if sw, err := h.svc.SwitchOutlet(ctx, a, owner.RefreshToken, out2); err != nil || sw.Outlet.ID != out2.String() {
		t.Errorf("owner pindah ke outlet mana pun: %v", err)
	}
}

// mkUser membuat pengguna ber-role `perms` yang ditugaskan ke `outlets`; mengembalikan sesi hasil login.
func mkUser(t *testing.T, h *harness, tenant, name, perms string, outlets ...string) *Session {
	t.Helper()
	ctx := context.Background()
	pw := "uji-pass-12345"
	hash, err := pauth.HashPassword(pw)
	if err != nil {
		t.Fatal(err)
	}
	roleID, userID := uuid.New(), uuid.New()
	email := fmt.Sprintf("%s-%d@rec.test", name, os.Getpid())
	if _, err := h.admin.Exec(ctx, `INSERT INTO roles (id, tenant_id, name, permissions) VALUES ($1, $2, $3, $4::jsonb)`, roleID, tenant, name, perms); err != nil {
		t.Fatal(err)
	}
	if _, err := h.admin.Exec(ctx, `INSERT INTO users (id, tenant_id, role_id, email, name, password_hash) VALUES ($1, $2, $3, $4, $5, $6)`, userID, tenant, roleID, email, name, hash); err != nil {
		t.Fatal(err)
	}
	for _, o := range outlets {
		if _, err := h.admin.Exec(ctx, `INSERT INTO user_outlets (tenant_id, user_id, outlet_id) VALUES ($1, $2, $3)`, tenant, userID, o); err != nil {
			t.Fatal(err)
		}
	}
	sess, err := h.svc.Login(ctx, LoginInput{Email: email, Password: pw})
	if err != nil {
		t.Fatal(err)
	}
	return sess
}

// Pindah outlet dari kasir wajib disetujui penyetuju ber-PIN yang punya izin `outlet_switch.approve` dan akses ke
// outlet tujuan; pemegang izin itu sendiri tidak perlu PIN; pindah di luar kasir tidak diwajibkan.
func TestPosSwitchOutletNeedsApproval(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	_, owner := registered(t, h, "PosPindah")
	tid, o1 := owner.Tenant.ID, owner.Outlet.ID
	o2, o3 := addOutlet(t, h, tid, "Cabang 2").String(), addOutlet(t, h, tid, "Cabang 3").String()
	h.svc.Approvals = approval.NewService(h.svc.Pool, h.rdb, "uji-rahasia-uji-rahasia-uji-rahasia")

	kasir := mkUser(t, h, tid, "kasir", `{"sales_orders":["view","create"]}`, o1, o2, o3)
	spv := mkUser(t, h, tid, "spv", `{"outlet_switch":["view","approve"]}`, o2)
	spvActor := actorOf(t, h, spv)
	if err := h.svc.Approvals.SetPin(ctx, spvActor, "uji-pass-12345", "482915"); err != nil {
		t.Fatal(err)
	}
	spvID := spvActor.UserID
	ka := actorOf(t, h, kasir)
	to := func(id string, ap *ApprovalIn) error {
		_, err := h.svc.SwitchOutletWith(ctx, ka, kasir.RefreshToken, uuid.MustParse(id), SwitchOpts{POS: true, Approval: ap})
		return err
	}

	if err := to(o2, nil); !errors.Is(err, approval.ErrPinRequired) {
		t.Errorf("tanpa persetujuan: err = %v, want ErrPinRequired", err)
	}
	if err := to(o2, &ApprovalIn{UserID: spvID, PIN: "000111"}); !errors.Is(err, approval.ErrInvalidPin) {
		t.Errorf("PIN salah: err = %v, want ErrInvalidPin", err)
	}
	// Penyetuju tidak punya akses ke outlet tujuan (hanya o2) → ditolak walau PIN benar.
	if err := to(o3, &ApprovalIn{UserID: spvID, PIN: "482915"}); !errors.Is(err, approval.ErrInvalidPin) {
		t.Errorf("penyetuju tanpa akses outlet tujuan: err = %v, want ErrInvalidPin", err)
	}
	// Pelaku bukan penyetuju tidak bisa menyetujui dirinya sendiri.
	if err := to(o2, &ApprovalIn{UserID: ka.UserID, PIN: "482915"}); !errors.Is(err, approval.ErrInvalidPin) {
		t.Errorf("setuju diri sendiri: err = %v, want ErrInvalidPin", err)
	}
	if n := count(t, h.admin, `SELECT count(*) FROM audit_log WHERE tenant_id = $1 AND action = 'auth.outlet_switch'`, tid); n != 0 {
		t.Errorf("penolakan tidak boleh mencatat perpindahan, got %d", n)
	}
	if err := to(o2, &ApprovalIn{UserID: spvID, PIN: "482915"}); err != nil {
		t.Fatalf("persetujuan benar: %v", err)
	}
	if n := count(t, h.admin, `SELECT count(*) FROM audit_log WHERE tenant_id = $1 AND action = 'auth.outlet_switch' AND details->>'approved_by' = 'spv'`, tid); n != 1 {
		t.Errorf("audit dengan penyetuju = %d, want 1", n)
	}

	// Pindah di luar kasir (pemilih sidebar) tidak diwajibkan.
	if _, err := h.svc.SwitchOutletWith(ctx, ka, kasir.RefreshToken, uuid.MustParse(o3), SwitchOpts{}); err != nil {
		t.Errorf("non-kasir tanpa PIN: %v", err)
	}
	// Pemegang izin penyetuju memindahkan dirinya sendiri tanpa PIN.
	if _, err := h.svc.SwitchOutletWith(ctx, spvActor, spv.RefreshToken, uuid.MustParse(o2), SwitchOpts{POS: true}); err != nil {
		t.Errorf("penyetuju sendiri: %v", err)
	}
	if _, err := h.svc.SwitchOutletWith(ctx, actorOf(t, h, owner), owner.RefreshToken, uuid.MustParse(o3), SwitchOpts{POS: true}); err != nil {
		t.Errorf("owner: %v", err)
	}
}
