package auth

import (
	"context"
	"errors"
	"fmt"
	"os"
	"testing"

	"aciraba/internal/authz"
	pauth "aciraba/internal/platform/auth"
)

func registered(t *testing.T, h *harness, name string) (email string, sess *Session) {
	t.Helper()
	email = fmt.Sprintf("%s-%d@rec.test", name, os.Getpid())
	cleanup(t, h.admin, "%@rec.test")
	sess, err := h.svc.Register(context.Background(), input(name, email), "id")
	if err != nil {
		t.Fatal(err)
	}
	return email, sess
}

func TestPasswordResetFlow(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	email, sess := registered(t, h, "Lupa")
	h.jobs.Wait() // email verifikasi dari register

	// Email tidak dikenal: tidak ada email, tidak ada error (anti-enumerasi).
	h.svc.RequestPasswordReset("tidak-ada@rec.test", "id")
	h.jobs.Wait()
	if h.mail.count("tidak-ada@rec.test") != 0 {
		t.Fatal("email tidak boleh dikirim ke alamat yang tidak terdaftar")
	}

	h.svc.RequestPasswordReset(email, "id")
	h.jobs.Wait()
	first := h.mail.lastToken(email)
	if first == "" {
		t.Fatal("email reset tidak terkirim")
	}
	// Permintaan kedua mencabut tautan pertama (satu token aktif per pengguna).
	h.svc.RequestPasswordReset(email, "id")
	h.jobs.Wait()
	second := h.mail.lastToken(email)
	if second == "" || second == first {
		t.Fatal("token kedua harus berbeda")
	}
	if err := h.svc.ResetPassword(ctx, first, "password-baru-456"); !errors.Is(err, pauth.ErrInvalidToken) {
		t.Fatalf("token lama err = %v, want ErrInvalidToken", err)
	}

	// Password lemah ditolak TANPA membakar token.
	var fe FieldErrors
	if err := h.svc.ResetPassword(ctx, second, "lemah"); !errors.As(err, &fe) || fe["password"] == "" {
		t.Fatalf("password lemah: err = %v", err)
	}

	// Sebelum reset: ada sesi refresh dan token akses yang hidup.
	tokensBefore := h.svc.now()
	if err := h.svc.ResetPassword(ctx, second, "password-baru-456"); err != nil {
		t.Fatal(err)
	}
	if err := h.svc.ResetPassword(ctx, second, "password-baru-789"); !errors.Is(err, pauth.ErrInvalidToken) {
		t.Fatalf("token dipakai dua kali: err = %v, want ErrInvalidToken", err)
	}

	// Semua sesi lama dicabut: refresh token register tidak berlaku, token akses lama kedaluwarsa lewat ValidAfter.
	if _, err := h.svc.Refresh(ctx, sess.RefreshToken); !errors.Is(err, ErrInvalidSession) {
		t.Errorf("refresh token lama: err = %v, want ErrInvalidSession", err)
	}
	acc, err := accountByEmail(ctx, h.svc.Pool, email)
	if err != nil {
		t.Fatal(err)
	}
	access, err := h.svc.Perms.For(ctx, acc.TenantID, acc.UserID)
	if err != nil || !access.ValidAfter.After(tokensBefore) {
		t.Errorf("ValidAfter = %v (%v), want setelah %v", access.ValidAfter, err, tokensBefore)
	}

	// Password lama gagal, baru berhasil; email terbukti milik pengguna → terverifikasi.
	if _, err := h.svc.Login(ctx, LoginInput{Email: email, Password: "sandi-aman-123"}); !errors.Is(err, ErrInvalidCredentials) {
		t.Errorf("password lama: err = %v", err)
	}
	login, err := h.svc.Login(ctx, LoginInput{Email: email, Password: "password-baru-456"})
	if err != nil {
		t.Fatal(err)
	}
	if !login.EmailVerified {
		t.Error("email harus terverifikasi setelah reset lewat tautan email")
	}
	if n := count(t, h.admin, `SELECT count(*) FROM audit_log WHERE tenant_id = $1 AND action = 'auth.password_reset'`, sess.Tenant.ID); n != 1 {
		t.Errorf("audit reset password = %d, want 1", n)
	}
}

func TestPasswordResetIgnoresDisabledAccounts(t *testing.T) {
	h := newHarness(t)
	email, _ := registered(t, h, "Nonaktif")
	h.jobs.Wait()
	before := h.mail.count(email)
	if _, err := h.admin.Exec(context.Background(), `UPDATE users SET active = false WHERE email = $1`, email); err != nil {
		t.Fatal(err)
	}
	h.svc.RequestPasswordReset(email, "id")
	h.jobs.Wait()
	if h.mail.count(email) != before {
		t.Error("akun nonaktif tidak boleh menerima email reset")
	}
}

// actorOf membangun authz.Actor seperti yang dipasang middleware Authenticate untuk sesi ini.
func actorOf(t *testing.T, h *harness, sess *Session) authz.Actor {
	t.Helper()
	claims, err := h.svc.Tokens.Parse(sess.AccessToken)
	if err != nil {
		t.Fatal(err)
	}
	a := authzActor(t, h, claims.TenantID, claims.Subject, claims.OutletID)
	return a
}
