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

// fakeEraya meniru website Eraya: JWKS + token endpoint (code "good" → id_token, selain itu invalid_grant).
func fakeEraya(t *testing.T, key *rsa.PrivateKey, mod func(jwt.MapClaims)) *httptest.Server {
	t.Helper()
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/.well-known/jwks.json":
			_ = json.NewEncoder(w).Encode(map[string]any{"keys": []map[string]string{{
				"kid": "k1", "kty": "RSA",
				"n": base64.RawURLEncoding.EncodeToString(key.N.Bytes()),
				"e": base64.RawURLEncoding.EncodeToString(big.NewInt(int64(key.E)).Bytes()),
			}}})
		case "/oauth/token":
			_ = r.ParseForm()
			if r.PostForm.Get("code") != "good" || r.PostForm.Get("client_id") != "arus" || r.PostForm.Get("code_verifier") == "" {
				w.WriteHeader(http.StatusBadRequest)
				_, _ = w.Write([]byte(`{"error":"invalid_grant"}`))
				return
			}
			c := jwt.MapClaims{"iss": srv.URL, "aud": "arus", "sub": "u-1", "email": "orang@eraya.test", "email_verified": true,
				"name": "Orang Eraya", "exp": time.Now().Add(time.Hour).Unix()}
			if mod != nil {
				mod(c)
			}
			tok := jwt.NewWithClaims(jwt.SigningMethodRS256, c)
			tok.Header["kid"] = "k1"
			s, _ := tok.SignedString(key)
			_ = json.NewEncoder(w).Encode(map[string]string{"id_token": s})
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestErayaExchangeAndVerify(t *testing.T) {
	key, _ := rsa.GenerateKey(rand.Reader, 2048)
	srv := fakeEraya(t, key, nil)
	e := NewEraya(srv.URL+"/", "arus", "")
	ctx := context.Background()
	verifier := string(make([]byte, 43))

	tok, err := e.Exchange(ctx, "good", verifier, "http://localhost/cb")
	if err != nil {
		t.Fatal(err)
	}
	c, err := e.Verifier.Verify(ctx, tok)
	if err != nil || c.Subject != "u-1" || c.Email != "orang@eraya.test" {
		t.Fatalf("klaim: %+v %v", c, err)
	}
	if _, err := e.Exchange(ctx, "bad", verifier, "http://localhost/cb"); !errors.Is(err, ErrGoogleInvalidToken) {
		t.Fatalf("code salah: err = %v", err)
	}

	// Token dari issuer/audience/kunci lain atau email belum terverifikasi ditolak.
	other, _ := rsa.GenerateKey(rand.Reader, 2048)
	for name, tc := range map[string]struct {
		srv  *httptest.Server
		want error
	}{
		"iss salah":      {fakeEraya(t, key, func(c jwt.MapClaims) { c["iss"] = "https://evil.test" }), ErrGoogleInvalidToken},
		"aud salah":      {fakeEraya(t, key, func(c jwt.MapClaims) { c["aud"] = "lain" }), ErrGoogleInvalidToken},
		"email belum ok": {fakeEraya(t, key, func(c jwt.MapClaims) { c["email_verified"] = false }), ErrGoogleEmailUnverified},
		"kunci lain":     {fakeEraya(t, other, nil), ErrGoogleInvalidToken},
	} {
		// Token berasal dari tc.srv tetapi diverifikasi terhadap JWKS `srv` (kecuali "iss"/"aud"/"email": kunci sama).
		ex := NewEraya(tc.srv.URL, "arus", "")
		tok, err := ex.Exchange(ctx, "good", verifier, "http://localhost/cb")
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if name == "kunci lain" {
			ex = e
		} else if name != "email belum ok" {
			ex = &Eraya{Verifier: e.Verifier}
		}
		if _, err := ex.Verifier.Verify(ctx, tok); !errors.Is(err, tc.want) {
			t.Errorf("%s: err = %v, want %v", name, err, tc.want)
		}
	}
	if NewEraya("", "arus", "") != nil || NewEraya("http://x", " ", "") != nil {
		t.Error("tanpa issuer/client ID harus nil")
	}
}

func TestErayaRegisterLoginAndLink(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	cleanup(t, h.admin, "%@eraya.test")
	pid := os.Getpid()
	email := fmt.Sprintf("e-%d@eraya.test", pid)
	claims := GoogleClaims{Subject: fmt.Sprintf("sub-%d", pid), Email: email, Name: "Eraya User"}

	if _, err := h.svc.IdentityLogin(ctx, erayaProviderID, claims, true); !errors.Is(err, ErrGoogleNotRegistered) {
		t.Fatalf("err = %v, want ErrGoogleNotRegistered", err)
	}
	clean, f := ValidateIdentityRegister(erayaProviderID, GoogleRegisterInput{BusinessName: "UJI-Eraya", OutletName: "Pusat", Phone: "+6281234567890", AcceptTerms: true}, claims)
	if f != nil {
		t.Fatal(f)
	}
	sess, err := h.svc.GoogleRegister(ctx, clean, "id")
	if err != nil || !sess.EmailVerified {
		t.Fatalf("daftar: %+v %v", sess, err)
	}
	if n := count(t, h.admin, `SELECT count(*) FROM user_identities WHERE provider = 'eraya' AND subject = $1`, claims.Subject); n != 1 {
		t.Fatalf("identitas eraya harus tertaut, n = %d", n)
	}
	if _, err := h.svc.GoogleRegister(ctx, clean, "id"); !errors.Is(err, ErrEmailTaken) {
		t.Fatalf("daftar ulang err = %v, want ErrEmailTaken", err)
	}
	// Login lewat identitas tertaut walau email di Eraya berubah; identitas Google tidak ikut tertaut.
	moved := GoogleClaims{Subject: claims.Subject, Email: "lain@eraya.test", Name: "X"}
	if _, err := h.svc.IdentityLogin(ctx, erayaProviderID, moved, false); err != nil {
		t.Fatalf("login tertaut: %v", err)
	}
	if _, err := h.svc.GoogleLogin(ctx, moved, false); !errors.Is(err, ErrGoogleNotRegistered) {
		t.Fatalf("subject yang sama di Google harus asing, err = %v", err)
	}
}
