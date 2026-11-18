package httpx

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestTurnstileGuard(t *testing.T) {
	cf := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		if r.Form.Get("secret") != "s3cret" {
			t.Errorf("secret tidak terkirim")
		}
		switch r.Form.Get("response") {
		case "good":
			_, _ = w.Write([]byte(`{"success":true}`))
		case "boom":
			w.WriteHeader(http.StatusInternalServerError)
		default:
			_, _ = w.Write([]byte(`{"success":false}`))
		}
	}))
	defer cf.Close()
	ts := NewTurnstile("s3cret")
	ts.url = cf.URL
	h := ts.Guard(func(r *http.Request) bool { return r.Header.Get("X-Client") == "mobile" })(
		http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) }))
	do := func(token, client string) int {
		req := httptest.NewRequest(http.MethodPost, "/auth/login", strings.NewReader(""))
		if token != "" {
			req.Header.Set(TurnstileHeader, token)
		}
		if client != "" {
			req.Header.Set("X-Client", client)
		}
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		return rec.Code
	}
	for _, c := range []struct {
		token, client string
		want          int
	}{
		{"good", "", http.StatusNoContent},
		{"", "", http.StatusForbidden},
		{"bad", "", http.StatusForbidden},
		{"boom", "", http.StatusServiceUnavailable},
		{"", "mobile", http.StatusNoContent},
	} {
		if got := do(c.token, c.client); got != c.want {
			t.Errorf("token=%q client=%q: got %d want %d", c.token, c.client, got, c.want)
		}
	}
	var off *Turnstile
	if rec := httptest.NewRecorder(); func() int {
		off.Guard(nil)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) })).
			ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/", nil))
		return rec.Code
	}() != http.StatusNoContent {
		t.Errorf("tanpa secret harus meneruskan")
	}
}
