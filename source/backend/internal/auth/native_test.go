package auth

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
)

// Klien native (X-Client: mobile): refresh token lewat badan JSON, tanpa cookie dan tanpa CSRFGuard.
func TestNativeClientSessionFlow(t *testing.T) {
	h := newHarness(t)
	email, _ := registered(t, h, "Native")
	api := NewHandler(HandlerDeps{
		Service: h.svc, Log: slog.New(slog.NewTextHandler(io.Discard, nil)), Redis: h.rdb,
		Lockout: NewLockout(h.rdb, NewPolicyLoader(h.admin, slog.New(slog.NewTextHandler(io.Discard, nil)))),
		Tokens:  h.svc.Tokens, Perms: h.svc.Perms, Origins: []string{"https://app.test"},
	})
	router := chi.NewRouter()
	api.Routes(router)
	post := func(path, body string, native bool, bearer string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		if native {
			req.Header.Set(ClientHeader, clientMobile)
		}
		if bearer != "" {
			req.Header.Set("Authorization", "Bearer "+bearer)
		}
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		return rec
	}
	type resp struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
	}
	decode := func(rec *httptest.ResponseRecorder) resp {
		var r resp
		if err := json.Unmarshal(rec.Body.Bytes(), &r); err != nil {
			t.Fatal(err)
		}
		return r
	}

	// login web: refresh token hanya di cookie, tidak di badan.
	web := post("/auth/login", `{"email":"`+email+`","password":"sandi-aman-123"}`, false, "")
	if web.Code != http.StatusOK || decode(web).RefreshToken != "" || len(web.Result().Cookies()) == 0 {
		t.Fatalf("login web: status=%d body=%s cookies=%d", web.Code, web.Body.String(), len(web.Result().Cookies()))
	}

	// login native: refresh token di badan, tanpa cookie.
	rec := post("/auth/login", `{"email":"`+email+`","password":"sandi-aman-123","remember":true}`, true, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("login native: %d %s", rec.Code, rec.Body.String())
	}
	s1 := decode(rec)
	if s1.AccessToken == "" || s1.RefreshToken == "" || len(rec.Result().Cookies()) != 0 {
		t.Fatalf("login native harus memberi refresh token di badan tanpa cookie: %+v cookies=%d", s1, len(rec.Result().Cookies()))
	}

	// refresh native tanpa Origin/CSRF header lolos, dirotasi; token lama tidak berlaku lagi di luar jendela grace.
	rec = post("/auth/refresh", `{"refresh_token":"`+s1.RefreshToken+`"}`, true, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("refresh native: %d %s", rec.Code, rec.Body.String())
	}
	s2 := decode(rec)
	if s2.RefreshToken == "" || s2.RefreshToken == s1.RefreshToken {
		t.Fatalf("refresh token harus dirotasi: %+v", s2)
	}

	// tanpa X-Client, endpoint refresh tetap dijaga CSRFGuard.
	if rec = post("/auth/refresh", `{"refresh_token":"`+s2.RefreshToken+`"}`, false, ""); rec.Code != http.StatusForbidden {
		t.Fatalf("refresh web tanpa Origin harus 403, dapat %d", rec.Code)
	}
	// token kosong / rusak ditolak.
	if rec = post("/auth/refresh", `{"refresh_token":""}`, true, ""); rec.Code != http.StatusUnauthorized {
		t.Fatalf("refresh kosong harus 401, dapat %d", rec.Code)
	}
	if rec = post("/auth/refresh", `{"refresh_token":"bukan-token"}`, true, ""); rec.Code != http.StatusUnauthorized {
		t.Fatalf("refresh rusak harus 401, dapat %d", rec.Code)
	}

	// logout native mencabut sesi.
	if rec = post("/auth/logout", `{"refresh_token":"`+s2.RefreshToken+`"}`, true, ""); rec.Code != http.StatusNoContent {
		t.Fatalf("logout native: %d", rec.Code)
	}
	if rec = post("/auth/refresh", `{"refresh_token":"`+s2.RefreshToken+`"}`, true, ""); rec.Code != http.StatusUnauthorized {
		t.Fatalf("refresh setelah logout harus 401, dapat %d", rec.Code)
	}
}
