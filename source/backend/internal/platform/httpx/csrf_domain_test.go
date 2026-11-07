package httpx

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestCSRFGuardWithDynamicOrigin(t *testing.T) {
	extra := func(_ context.Context, origin string) bool { return origin == "https://arus.klien.id" }
	h := CSRFGuardWith([]string{"https://app.example"}, extra)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	cases := []struct {
		origin, csrf string
		want         int
	}{
		{"https://app.example", "1", http.StatusNoContent},
		{"https://arus.klien.id", "1", http.StatusNoContent},
		{"https://arus.klien.id", "", http.StatusForbidden},
		{"https://evil.example", "1", http.StatusForbidden},
		{"", "1", http.StatusForbidden},
	}
	for _, c := range cases {
		r := httptest.NewRequest(http.MethodPost, "/auth/refresh", nil)
		if c.origin != "" {
			r.Header.Set("Origin", c.origin)
		}
		if c.csrf != "" {
			r.Header.Set(CSRFHeader, c.csrf)
		}
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if w.Code != c.want {
			t.Errorf("origin=%q csrf=%q: got %d, want %d", c.origin, c.csrf, w.Code, c.want)
		}
	}
}
