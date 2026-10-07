package auth

import (
	"context"
	"errors"
	"testing"

	pauth "aciraba/internal/platform/auth"
)

func TestEmailVerificationFlow(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	email, sess := registered(t, h, "Verif")
	h.jobs.Wait()

	// Register mengirim email verifikasi (setelah commit).
	tok := h.mail.lastToken(email)
	if tok == "" || h.mail.count(email) != 1 {
		t.Fatalf("email verifikasi saat register: token=%q jumlah=%d", tok, h.mail.count(email))
	}
	if sess.EmailVerified {
		t.Fatal("sesi register harus belum terverifikasi")
	}

	// Kirim ulang: token baru mencabut yang lama.
	a := actorOf(t, h, sess)
	if err := h.svc.ResendVerification(ctx, a, "en"); err != nil {
		t.Fatal(err)
	}
	h.jobs.Wait()
	fresh := h.mail.lastToken(email)
	if fresh == "" || fresh == tok || h.mail.count(email) != 2 {
		t.Fatalf("kirim ulang: token baru=%q jumlah=%d", fresh, h.mail.count(email))
	}
	if err := h.svc.VerifyEmail(ctx, tok); !errors.Is(err, pauth.ErrInvalidToken) {
		t.Errorf("token lama: err = %v, want ErrInvalidToken", err)
	}

	// Token untuk tujuan lain tidak bisa dipakai di sini.
	if err := h.svc.VerifyEmail(ctx, "bukan-token-yang-valid-sama-sekali"); !errors.Is(err, pauth.ErrInvalidToken) {
		t.Errorf("token asing: err = %v", err)
	}

	if err := h.svc.VerifyEmail(ctx, fresh); err != nil {
		t.Fatal(err)
	}
	if err := h.svc.VerifyEmail(ctx, fresh); !errors.Is(err, pauth.ErrInvalidToken) {
		t.Errorf("pakai dua kali: err = %v, want ErrInvalidToken", err)
	}
	login, err := h.svc.Login(ctx, LoginInput{Email: email, Password: "sandi-aman-123"})
	if err != nil || !login.EmailVerified {
		t.Fatalf("setelah verifikasi: %+v %v", login, err)
	}

	// Email yang sudah terverifikasi tidak dikirimi lagi.
	if err := h.svc.ResendVerification(ctx, a, "id"); err != nil {
		t.Fatal(err)
	}
	h.jobs.Wait()
	if h.mail.count(email) != 2 {
		t.Errorf("jumlah email = %d, want tetap 2", h.mail.count(email))
	}
	if n := count(t, h.admin, `SELECT count(*) FROM audit_log WHERE tenant_id = $1 AND action = 'auth.email_verified'`, sess.Tenant.ID); n != 1 {
		t.Errorf("audit verifikasi = %d, want 1", n)
	}
}

func TestResetTokenCannotVerifyEmail(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	email, _ := registered(t, h, "Silang")
	h.jobs.Wait()
	h.svc.RequestPasswordReset(email, "id")
	h.jobs.Wait()
	resetTok := h.mail.lastToken(email)
	if err := h.svc.VerifyEmail(ctx, resetTok); !errors.Is(err, pauth.ErrInvalidToken) {
		t.Errorf("token reset dipakai untuk verifikasi: err = %v, want ErrInvalidToken", err)
	}
	// ...dan tetap berlaku untuk tujuannya sendiri.
	if err := h.svc.ResetPassword(ctx, resetTok, "password-baru-456"); err != nil {
		t.Errorf("token reset rusak setelah salah pakai: %v", err)
	}
}
