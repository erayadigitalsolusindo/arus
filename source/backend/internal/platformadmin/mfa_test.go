package platformadmin

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/google/uuid"

	pauth "aciraba/internal/platform/auth"
)

// Admin tanpa 2FA: hanya halaman keamanan yang terbuka sampai 2FA aktif; sesudahnya login butuh kode.
func TestMFAEnrollmentGateAndLogin(t *testing.T) {
	e := newEnv(t, "")
	ctx := context.Background()
	id, email, pw := uuid.New(), "mfa-"+uuid.NewString()[:8]+"@pa.test", "sandi-aman-123"
	hash, _ := pauth.HashPassword(pw)
	if _, err := e.admin.Exec(ctx, `INSERT INTO platform_admins (id, email, name, password_hash) VALUES ($1, $2, 'Belum 2FA', $3)`, id, email, hash); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { e.dropAdmin(id) })
	a := Actor{ID: id, Name: "Belum 2FA"}

	// Belum ada 2FA: login langsung terbit sesi, tetapi rute data ditolak sampai mendaftar.
	tok := e.loginToken(t, email, pw)
	if rec := e.do("GET", "/platform/tenants", tok, ""); rec.Code != 403 || !strings.Contains(rec.Body.String(), "MFA_ENROLL_REQUIRED") {
		t.Fatalf("tanpa 2FA: %d %s", rec.Code, rec.Body)
	}
	if rec := e.do("GET", "/platform/security", tok, ""); rec.Code != 200 {
		t.Fatalf("halaman keamanan harus terbuka: %d %s", rec.Code, rec.Body)
	}

	// Pendaftaran: kode salah ditolak; kode benar mengaktifkan dan memberi 10 kode pemulihan.
	setup, err := e.svc.BeginMFA(ctx, a)
	if err != nil || !strings.HasPrefix(setup.URL, "otpauth://totp/") {
		t.Fatalf("begin: %+v %v", setup, err)
	}
	if _, err := e.svc.EnableMFA(ctx, a, "000000"); !errors.Is(err, ErrInvalidCode) {
		t.Fatalf("kode salah: %v", err)
	}
	good, _ := pauth.TOTPCode(setup.Secret, e.clock)
	codes, err := e.svc.EnableMFA(ctx, a, good)
	if err != nil || len(codes) != 10 {
		t.Fatalf("enable: %d kode, err %v", len(codes), err)
	}
	e.secrets[email] = setup.Secret
	if _, err := e.svc.BeginMFA(ctx, a); !errors.Is(err, ErrMFAEnabled) {
		t.Errorf("begin saat sudah aktif: %v", err)
	}
	// Rahasia di DB terenkripsi, bukan teks asli.
	var enc string
	_ = e.admin.QueryRow(ctx, `SELECT totp_secret_enc FROM platform_admins WHERE id = $1`, id).Scan(&enc)
	if enc == "" || strings.Contains(enc, setup.Secret) {
		t.Error("rahasia TOTP harus tersimpan terenkripsi")
	}

	// Login sekarang butuh faktor kedua: password saja tidak menerbitkan sesi.
	sess, mfaTok, err := e.svc.Login(ctx, email, pw, false)
	if err != nil || sess != nil || mfaTok == "" {
		t.Fatalf("login dengan 2FA: sesi=%v token=%q err=%v", sess, mfaTok, err)
	}
	if _, err := e.svc.LoginMFA(ctx, mfaTok, "123456"); !errors.Is(err, ErrInvalidCode) {
		t.Errorf("kode salah: %v", err)
	}
	code := e.nextCode(email)
	sess, err = e.svc.LoginMFA(ctx, mfaTok, code)
	if err != nil || sess == nil {
		t.Fatalf("kode benar: %v", err)
	}
	if rec := e.do("GET", "/platform/tenants", sess.AccessToken, ""); rec.Code != 200 {
		t.Errorf("setelah 2FA: %d %s", rec.Code, rec.Body)
	}
	// Tantangan sekali pakai; kode yang sama (periode sama) tidak bisa dipakai lagi (replay).
	if _, err := e.svc.LoginMFA(ctx, mfaTok, code); !errors.Is(err, ErrInvalidMFAToken) {
		t.Errorf("tantangan dipakai ulang: %v", err)
	}
	_, mfaTok2, _ := e.svc.Login(ctx, email, pw, false)
	if _, err := e.svc.LoginMFA(ctx, mfaTok2, code); !errors.Is(err, ErrInvalidCode) {
		t.Errorf("replay kode TOTP: %v, want ErrInvalidCode", err)
	}

	// Kode pemulihan: sekali pakai.
	if s, err := e.svc.LoginMFA(ctx, mfaTok2, strings.ToLower(codes[0])); err != nil || s == nil {
		t.Fatalf("kode pemulihan: %v", err)
	}
	_, mfaTok3, _ := e.svc.Login(ctx, email, pw, false)
	if _, err := e.svc.LoginMFA(ctx, mfaTok3, codes[0]); !errors.Is(err, ErrInvalidCode) {
		t.Errorf("kode pemulihan dipakai ulang: %v", err)
	}
	st, _ := e.svc.MFAStatus(ctx, a)
	if !st.Enabled || st.RecoveryRemaining != 9 {
		t.Errorf("status: %+v", st)
	}

	// Ganti kode pemulihan: yang lama hangus.
	newCodes, err := e.svc.RegenerateRecovery(ctx, a, e.nextCode(email))
	if err != nil || len(newCodes) != 10 {
		t.Fatalf("regenerate: %v", err)
	}
	if s, _ := e.svc.MFAStatus(ctx, a); s.RecoveryRemaining != 10 {
		t.Errorf("sisa kode pemulihan: %d", s.RecoveryRemaining)
	}
	if _, err := e.svc.LoginMFA(ctx, mfaTok3, codes[1]); !errors.Is(err, ErrInvalidCode) {
		t.Errorf("kode pemulihan lama harus hangus: %v", err)
	}
}

// Menonaktifkan/mereset 2FA mencabut sesi dan mewajibkan pendaftaran ulang; DB menolak penulisan langsung dari aplikasi.
func TestMFADisableResetAndDBGuards(t *testing.T) {
	e := newEnv(t, "")
	ctx := context.Background()
	id1, email1, pw := e.newAdmin(t, "Satu")
	id2, email2, _ := e.newAdmin(t, "Dua")
	a1, a2 := Actor{ID: id1, Name: "Satu", MFA: true}, Actor{ID: id2, Name: "Dua", MFA: true}
	tok2 := e.loginToken(t, email2, "sandi-aman-123")
	_ = email1

	// Nonaktifkan sendiri: wajib password benar + kode.
	if err := e.svc.DisableOwnMFA(ctx, a1, "salah-salah-1", e.nextCode(email1)); !errors.Is(err, ErrInvalidCredentials) {
		t.Errorf("password salah: %v", err)
	}
	if err := e.svc.DisableOwnMFA(ctx, a1, pw, "000000"); !errors.Is(err, ErrInvalidCode) {
		t.Errorf("kode salah: %v", err)
	}

	// Admin 1 mereset 2FA admin 2 (perangkat hilang): token lama admin 2 mati, ia harus daftar ulang.
	if err := e.svc.ResetAdminMFA(ctx, a1, id1); !errors.Is(err, ErrSelf) {
		t.Errorf("reset diri sendiri lewat jalur admin lain: %v", err)
	}
	if rec := e.do("POST", "/platform/admins/"+id2.String()+"/reset-2fa", mustLogin(t, e, email1, pw), ""); rec.Code != 204 {
		t.Fatalf("reset-2fa: %d %s", rec.Code, rec.Body)
	}
	if rec := e.do("GET", "/platform/auth/me", tok2, ""); rec.Code != 401 {
		t.Errorf("token lama admin yang di-reset: %d, want 401", rec.Code)
	}
	if _, mfa, err := e.svc.Login(ctx, email2, "sandi-aman-123", false); err != nil || mfa != "" {
		t.Errorf("setelah reset login tanpa tantangan 2FA: mfa=%q err=%v", mfa, err)
	}
	if st, _ := e.svc.MFAStatus(ctx, a2); st.Enabled || st.RecoveryRemaining != 0 {
		t.Errorf("status setelah reset: %+v", st)
	}

	// Penjaga DB: aplikasi tidak bisa menulis kolom/ tabel 2FA langsung, dan fungsi menolak pelaku bukan admin.
	if _, err := e.app.Exec(ctx, `UPDATE platform_admins SET totp_enabled_at = NULL`); err == nil {
		t.Error("UPDATE langsung ke platform_admins harus ditolak")
	}
	if _, err := e.app.Exec(ctx, `INSERT INTO platform_recovery_codes (admin_id, code_hash) VALUES ($1, 'x')`, id1); err == nil {
		t.Error("INSERT langsung ke platform_recovery_codes harus ditolak")
	}
	if _, err := e.app.Exec(ctx, `SELECT platform_admin_totp_reset($1, $2, now())`, uuid.New(), id1); err == nil {
		t.Error("reset 2FA oleh pelaku yang bukan Platform Admin harus ditolak")
	}
}

func mustLogin(t *testing.T, e *env, email, pw string) string {
	t.Helper()
	return e.loginToken(t, email, pw)
}
