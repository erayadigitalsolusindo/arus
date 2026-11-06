package auth

import (
	"context"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"

	"aciraba/internal/authz"
	pauth "aciraba/internal/platform/auth"
	"aciraba/internal/usermfa"
)

// Alur 2FA end-to-end: daftar → aktifkan → login tertahan tantangan → kode salah/ulang ditolak → kode benar →
// kode pemulihan sekali pakai → nonaktifkan → login biasa lagi. Juga isolasi: tantangan tak bisa dipakai akun lain.
func TestLoginWithMFA(t *testing.T) {
	h := newHarness(t)
	svc, pool := h.svc, h.admin
	ctx := context.Background()
	cleanup(t, pool, "%@mfa.test")

	box, err := pauth.NewUserTOTPBox("rahasia-uji-rahasia-uji-rahasia-uji-123")
	if err != nil {
		t.Fatal(err)
	}
	mfa := usermfa.New(svc.Pool, box, svc.Sessions)
	svc.MFA = mfa

	email := fmt.Sprintf("mfa-%d@mfa.test", os.Getpid())
	if _, err := svc.Register(ctx, input("Mfa", email), "id"); err != nil {
		t.Fatal(err)
	}
	sess, err := svc.Login(ctx, LoginInput{Email: email, Password: testPassword})
	if err != nil {
		t.Fatalf("login tanpa 2FA harus langsung berhasil: %v", err)
	}
	uid, tid := uuid.MustParse(sess.User.ID), uuid.MustParse(sess.Tenant.ID)
	a := authz.Actor{TenantID: tid, UserID: uid, OutletID: uuid.MustParse(sess.Outlet.ID)}

	setup, err := mfa.Begin(ctx, a)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := mfa.Enable(ctx, a, "000000"); !errors.Is(err, usermfa.ErrInvalidCode) {
		t.Fatalf("kode salah saat aktivasi: %v", err)
	}
	now := time.Now()
	code, _ := pauth.TOTPCode(setup.Secret, now)
	recovery, err := mfa.Enable(ctx, a, code)
	if err != nil || len(recovery) != 10 {
		t.Fatalf("aktivasi: %v (%d kode)", err, len(recovery))
	}
	if _, err := mfa.Begin(ctx, a); !errors.Is(err, usermfa.ErrEnabled) {
		t.Fatalf("daftar ulang saat aktif harus ditolak: %v", err)
	}

	// Login: password benar → tantangan, bukan sesi.
	var mr *MFARequired
	if _, err := svc.Login(ctx, LoginInput{Email: email, Password: testPassword, Remember: true}); !errors.As(err, &mr) || mr.Token == "" {
		t.Fatalf("login dengan 2FA harus MFARequired: %v", err)
	}
	if _, err := svc.LoginMFA(ctx, mr.Token, "000000"); !errors.Is(err, ErrInvalidMFACode) {
		t.Fatalf("kode salah: %v", err)
	}
	// Kode TOTP yang sudah dipakai saat aktivasi (periode sama) tidak boleh dipakai ulang; periode berikutnya sah.
	if _, err := svc.LoginMFA(ctx, mr.Token, code); !errors.Is(err, ErrInvalidMFACode) {
		t.Fatalf("replay kode aktivasi harus ditolak: %v", err)
	}
	next, _ := pauth.TOTPCode(setup.Secret, now.Add(30*time.Second))
	got, err := svc.LoginMFA(ctx, mr.Token, next)
	if err != nil || got.AccessToken == "" || !got.Remember || got.User.Email != email {
		t.Fatalf("login 2FA: %+v %v", got, err)
	}
	if _, err := svc.LoginMFA(ctx, mr.Token, next); !errors.Is(err, ErrInvalidMFAToken) {
		t.Fatalf("tantangan sekali pakai: %v", err)
	}

	// Kode pemulihan: sekali pakai.
	if _, err := svc.Login(ctx, LoginInput{Email: email, Password: testPassword}); !errors.As(err, &mr) {
		t.Fatalf("login ke-2: %v", err)
	}
	if _, err := svc.LoginMFA(ctx, mr.Token, recovery[0]); err != nil {
		t.Fatalf("kode pemulihan: %v", err)
	}
	if _, err := svc.Login(ctx, LoginInput{Email: email, Password: testPassword}); !errors.As(err, &mr) {
		t.Fatalf("login ke-3: %v", err)
	}
	if _, err := svc.LoginMFA(ctx, mr.Token, recovery[0]); !errors.Is(err, ErrInvalidMFACode) {
		t.Fatalf("kode pemulihan terpakai harus ditolak: %v", err)
	}
	if st, _ := mfa.Status(ctx, a); st == nil || !st.Enabled || st.RecoveryRemaining != 9 {
		t.Fatalf("status: %+v", st)
	}

	// Nonaktifkan: wajib password + kode.
	if err := mfa.Disable(ctx, a, "salah-salah-123", recovery[1]); !errors.Is(err, usermfa.ErrBadPassword) {
		t.Fatalf("password salah: %v", err)
	}
	if err := mfa.Disable(ctx, a, testPassword, recovery[1]); err != nil {
		t.Fatal(err)
	}
	if s, err := svc.Login(ctx, LoginInput{Email: email, Password: testPassword}); err != nil || s.AccessToken == "" {
		t.Fatalf("setelah nonaktif login biasa lagi: %v", err)
	}
}

func TestMFAResetByAdminAndIsolation(t *testing.T) {
	h := newHarness(t)
	svc, pool := h.svc, h.admin
	ctx := context.Background()
	cleanup(t, pool, "%@mfa2.test")
	box, _ := pauth.NewUserTOTPBox("rahasia-uji-rahasia-uji-rahasia-uji-123")
	mfa := usermfa.New(svc.Pool, box, svc.Sessions)
	svc.MFA = mfa

	mk := func(tag string) (authz.Actor, string) {
		email := fmt.Sprintf("%s-%d@mfa2.test", tag, os.Getpid())
		if _, err := svc.Register(ctx, input(tag, email), "id"); err != nil {
			t.Fatal(err)
		}
		s, err := svc.Login(ctx, LoginInput{Email: email, Password: testPassword})
		if err != nil {
			t.Fatal(err)
		}
		return authz.Actor{TenantID: uuid.MustParse(s.Tenant.ID), UserID: uuid.MustParse(s.User.ID), OutletID: uuid.MustParse(s.Outlet.ID)}, email
	}
	a, email := mk("a")
	b, _ := mk("b")

	setup, _ := mfa.Begin(ctx, a)
	code, _ := pauth.TOTPCode(setup.Secret, time.Now())
	if _, err := mfa.Enable(ctx, a, code); err != nil {
		t.Fatal(err)
	}

	// Tenant lain tidak bisa mereset 2FA pengguna ini (RLS: dianggap tidak ada).
	if err := mfa.Reset(ctx, b, a.UserID); !errors.Is(err, usermfa.ErrUserNotFound) {
		t.Fatalf("reset lintas tenant: %v", err)
	}
	if err := mfa.Reset(ctx, a, a.UserID); !errors.Is(err, usermfa.ErrForbidden) {
		t.Fatalf("reset diri sendiri harus ditolak: %v", err)
	}
	// Pengguna lain di tenant yang sama (admin) boleh mereset.
	admin := authz.Actor{TenantID: a.TenantID, UserID: uuid.New(), OutletID: a.OutletID}
	if err := mfa.Reset(ctx, admin, a.UserID); err != nil {
		t.Fatal(err)
	}
	if on, _ := mfa.Enabled(ctx, a.TenantID, a.UserID); on {
		t.Fatal("2FA seharusnya mati setelah reset")
	}
	if _, err := svc.Login(ctx, LoginInput{Email: email, Password: testPassword}); err != nil {
		t.Fatalf("login setelah reset: %v", err)
	}
}
