package auth

import (
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

func TestPasswordHashRoundTrip(t *testing.T) {
	h, err := HashPassword("kata sandi 123")
	if err != nil {
		t.Fatal(err)
	}
	if ok, err := VerifyPassword("kata sandi 123", h); err != nil || !ok {
		t.Fatalf("password benar harus lolos: ok=%v err=%v", ok, err)
	}
	if ok, _ := VerifyPassword("salah", h); ok {
		t.Fatal("password salah tidak boleh lolos")
	}
	h2, _ := HashPassword("kata sandi 123")
	if h == h2 {
		t.Fatal("salt harus acak")
	}
	if _, err := VerifyPassword("x", "bukan-hash"); err == nil {
		t.Fatal("hash rusak harus error")
	}
}

func TestTokenIssueParse(t *testing.T) {
	iss := NewTokenIssuer("rahasia-uji-rahasia-uji-rahasia-uji-123")
	now := time.Now()
	tok, err := iss.Issue("u1", "t1", "o1", "Owner", now)
	if err != nil {
		t.Fatal(err)
	}
	c, err := iss.Parse(tok)
	if err != nil || c.Subject != "u1" || c.TenantID != "t1" || c.OutletID != "o1" || c.Role != "Owner" {
		t.Fatalf("claims salah: %+v err=%v", c, err)
	}

	// Kedaluwarsa
	old, _ := iss.Issue("u1", "t1", "o1", "Owner", now.Add(-time.Hour))
	if _, err := iss.Parse(old); err == nil {
		t.Fatal("token kedaluwarsa harus ditolak")
	}
	// Secret lain
	other, _ := NewTokenIssuer("secret-lain-secret-lain-secret-lain-1").Issue("u1", "t1", "o1", "Owner", now)
	if _, err := iss.Parse(other); err == nil {
		t.Fatal("tanda tangan salah harus ditolak")
	}
	// alg=none
	none, _ := jwt.NewWithClaims(jwt.SigningMethodNone, Claims{TenantID: "t1", RegisteredClaims: jwt.RegisteredClaims{
		Issuer: issuer, Subject: "u1", ExpiresAt: jwt.NewNumericDate(now.Add(time.Minute)),
	}}).SignedString(jwt.UnsafeAllowNoneSignatureType)
	if _, err := iss.Parse(none); err == nil {
		t.Fatal("alg=none harus ditolak")
	}
}
