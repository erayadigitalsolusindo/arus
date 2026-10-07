package httpx

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	pauth "aciraba/internal/platform/auth"
)

func TestRequireAuth(t *testing.T) {
	iss := pauth.NewTokenIssuer("rahasia-uji-rahasia-uji-rahasia-uji-123")
	var gotSub string
	h := RequireAuth(iss)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, _ := ClaimsFrom(r.Context())
		gotSub = c.Subject
	}))
	valid, _ := iss.Issue("u1", "t1", "o1", "Owner", time.Now())
	expired, _ := iss.Issue("u1", "t1", "o1", "Owner", time.Now().Add(-time.Hour))

	cases := []struct {
		name, header string
		status       int
	}{
		{"tanpa header", "", 401},
		{"skema salah", "Basic " + valid, 401},
		{"sampah", "Bearer abc", 401},
		{"kedaluwarsa", "Bearer " + expired, 401},
		{"valid", "Bearer " + valid, 200},
	}
	for _, c := range cases {
		req := httptest.NewRequest("GET", "/", nil)
		if c.header != "" {
			req.Header.Set("Authorization", c.header)
		}
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Code != c.status {
			t.Errorf("%s: status %d, want %d", c.name, rec.Code, c.status)
		}
		if c.name == "kedaluwarsa" && !strings.Contains(rec.Body.String(), "TOKEN_EXPIRED") {
			t.Errorf("kedaluwarsa harus TOKEN_EXPIRED: %s", rec.Body.String())
		}
	}
	if gotSub != "u1" {
		t.Errorf("claims tidak terpasang di context: %q", gotSub)
	}
}

func TestCSRFGuard(t *testing.T) {
	h := CSRFGuard([]string{"http://localhost:5173"})(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	cases := []struct {
		name, origin, header string
		status               int
	}{
		{"tanpa origin", "", "x", 403},
		{"origin asing", "https://jahat.example", "x", 403},
		{"tanpa header kustom", "http://localhost:5173", "", 403},
		{"sah", "http://localhost:5173", "aciraba", 200},
	}
	for _, c := range cases {
		req := httptest.NewRequest("POST", "/", nil)
		if c.origin != "" {
			req.Header.Set("Origin", c.origin)
		}
		if c.header != "" {
			req.Header.Set(CSRFHeader, c.header)
		}
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Code != c.status {
			t.Errorf("%s: status %d, want %d", c.name, rec.Code, c.status)
		}
	}
}
