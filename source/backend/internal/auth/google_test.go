package auth

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

func jwksServer(t *testing.T, key *rsa.PublicKey) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"keys": []map[string]string{{
			"kid": "k1", "kty": "RSA",
			"n": base64.RawURLEncoding.EncodeToString(key.N.Bytes()),
			"e": base64.RawURLEncoding.EncodeToString(big.NewInt(int64(key.E)).Bytes()),
		}}})
	}))
	t.Cleanup(srv.Close)
	return srv
}

func signGoogle(t *testing.T, key *rsa.PrivateKey, mod func(jwt.MapClaims)) string {
	t.Helper()
	c := jwt.MapClaims{
		"iss": "https://accounts.google.com", "aud": "client-1", "sub": "sub-123", "email": "orang@gmail.test",
		"email_verified": true, "name": "Orang Uji", "exp": time.Now().Add(time.Hour).Unix(),
	}
	if mod != nil {
		mod(c)
	}
	tok := jwt.NewWithClaims(jwt.SigningMethodRS256, c)
	tok.Header["kid"] = "k1"
	s, err := tok.SignedString(key)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func TestGoogleVerifier(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	other, _ := rsa.GenerateKey(rand.Reader, 2048)
	srv := jwksServer(t, &key.PublicKey)
	v := NewGoogleVerifier([]string{"client-1", "client-2"}).(*googleVerifier)
	v.url = srv.URL

	c, err := v.Verify(context.Background(), signGoogle(t, key, nil))
	if err != nil || c.Subject != "sub-123" || c.Email != "orang@gmail.test" || c.Name != "Orang Uji" {
		t.Fatalf("token sah ditolak: %+v %v", c, err)
	}
	cases := map[string]struct {
		tok  string
		want error
	}{
		"aud salah":      {signGoogle(t, key, func(c jwt.MapClaims) { c["aud"] = "lain" }), ErrGoogleInvalidToken},
		"iss salah":      {signGoogle(t, key, func(c jwt.MapClaims) { c["iss"] = "https://evil.test" }), ErrGoogleInvalidToken},
		"kedaluwarsa":    {signGoogle(t, key, func(c jwt.MapClaims) { c["exp"] = time.Now().Add(-time.Hour).Unix() }), ErrGoogleInvalidToken},
		"tanda tangan":   {signGoogle(t, other, nil), ErrGoogleInvalidToken},
		"email belum ok": {signGoogle(t, key, func(c jwt.MapClaims) { c["email_verified"] = false }), ErrGoogleEmailUnverified},
		"tanpa sub":      {signGoogle(t, key, func(c jwt.MapClaims) { delete(c, "sub") }), ErrGoogleInvalidToken},
		"sampah":         {"bukan.jwt.sama-sekali", ErrGoogleInvalidToken},
		"kosong":         {"", ErrGoogleInvalidToken},
	}
	for name, tc := range cases {
		if _, err := v.Verify(context.Background(), tc.tok); !errors.Is(err, tc.want) {
			t.Errorf("%s: err = %v, want %v", name, err, tc.want)
		}
	}
	if NewGoogleVerifier(nil) != nil || NewGoogleVerifier([]string{" "}) != nil {
		t.Error("tanpa client ID verifier harus nil")
	}
}

func TestValidateGoogleRegister(t *testing.T) {
	c := GoogleClaims{Subject: "s1", Email: "Orang@Gmail.test", Name: "Orang"}
	ok, f := ValidateGoogleRegister(GoogleRegisterInput{BusinessName: "Toko A", OutletName: "Pusat", Phone: "0812 3456 7890", AcceptTerms: true}, c)
	if f != nil || ok.Email != "orang@gmail.test" || ok.OwnerName != "Orang" || ok.GoogleSub != "s1" {
		t.Fatalf("valid ditolak: %+v %v", ok, f)
	}
	_, f = ValidateGoogleRegister(GoogleRegisterInput{}, c)
	for _, k := range []string{"business_name", "outlet_name", "phone", "accept_terms"} {
		if f[k] == "" {
			t.Errorf("field %s harus ditolak", k)
		}
	}
}

func TestGoogleRegisterLoginAndLink(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	cleanup(t, h.admin, "%@google.test")
	pid := os.Getpid()
	email := fmt.Sprintf("g-%d@google.test", pid)
	claims := GoogleClaims{Subject: fmt.Sprintf("sub-%d", pid), Email: email, Name: "Google User"}

	// Belum terdaftar.
	if _, err := h.svc.GoogleLogin(ctx, claims, true); !errors.Is(err, ErrGoogleNotRegistered) {
		t.Fatalf("err = %v, want ErrGoogleNotRegistered", err)
	}
	clean, f := ValidateGoogleRegister(GoogleRegisterInput{BusinessName: "UJI-Google", OutletName: "Pusat", Phone: "+6281234567890", AcceptTerms: true}, claims)
	if f != nil {
		t.Fatal(f)
	}
	sess, err := h.svc.GoogleRegister(ctx, clean, "id")
	if err != nil {
		t.Fatal(err)
	}
	if !sess.EmailVerified || sess.AccessToken == "" {
		t.Fatalf("sesi google: %+v", sess)
	}
	if n := count(t, h.admin, `SELECT count(*) FROM users WHERE email = $1 AND email_verified_at IS NOT NULL`, email); n != 1 {
		t.Fatalf("email harus terverifikasi, n = %d", n)
	}
	if h.mail.count(email) != 0 {
		t.Fatal("tidak boleh kirim email verifikasi untuk akun Google")
	}

	// Daftar ulang dengan identitas sama / email sama ditolak.
	if _, err := h.svc.GoogleRegister(ctx, clean, "id"); !errors.Is(err, ErrEmailTaken) {
		t.Fatalf("daftar ulang err = %v, want ErrEmailTaken", err)
	}

	// Login lewat identitas tertaut, bahkan bila email Google berubah.
	moved := GoogleClaims{Subject: claims.Subject, Email: "lain@google.test", Name: "X"}
	if _, err := h.svc.GoogleLogin(ctx, moved, false); err != nil {
		t.Fatalf("login via identitas: %v", err)
	}

	// Akun password biasa dengan email sama tertaut otomatis saat login Google pertama.
	pwEmail := fmt.Sprintf("pw-%d@google.test", pid)
	if _, err := h.svc.Register(ctx, input("Pw", pwEmail), "id"); err != nil {
		t.Fatal(err)
	}
	link := GoogleClaims{Subject: fmt.Sprintf("sub-pw-%d", pid), Email: pwEmail}
	s2, err := h.svc.GoogleLogin(ctx, link, true)
	if err != nil || !s2.EmailVerified {
		t.Fatalf("tautan otomatis: %v %+v", err, s2)
	}
	if n := count(t, h.admin, `SELECT count(*) FROM user_identities WHERE subject = $1`, link.Subject); n != 1 {
		t.Fatalf("identitas tertaut = %d, want 1", n)
	}
	// Identitas Google lain untuk email yang sudah punya identitas → ditolak.
	if _, err := h.svc.GoogleLogin(ctx, GoogleClaims{Subject: "sub-lain", Email: pwEmail}, true); !errors.Is(err, ErrGoogleLinked) {
		t.Fatalf("err = %v, want ErrGoogleLinked", err)
	}
}
