package auth

import (
	"encoding/base64"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
)

func TestValidateFeatureShortcuts(t *testing.T) {
	validPNG := "data:image/png;base64," + base64.StdEncoding.EncodeToString([]byte("\x89PNG\r\n\x1a\n\x00\x00\x00\rIHDR"))
	cases := []struct {
		name      string
		shortcuts []FeatureShortcut
		valid     bool
	}{
		{name: "internal path and uploaded icon", shortcuts: []FeatureShortcut{{ID: "pos", Title: "Kasir", URL: "/kasir", Icon: validPNG}}, valid: true},
		{name: "external https", shortcuts: []FeatureShortcut{{ID: "purchase", Title: "Pembelian", URL: "https://example.test/purchases"}}, valid: true},
		{name: "javascript rejected", shortcuts: []FeatureShortcut{{ID: "bad", Title: "Bad", URL: "javascript:alert(1)"}}},
		{name: "protocol relative rejected", shortcuts: []FeatureShortcut{{ID: "bad", Title: "Bad", URL: "//example.test"}}},
		{name: "duplicate id rejected", shortcuts: []FeatureShortcut{{ID: "same", Title: "A", URL: "/a"}, {ID: "same", Title: "B", URL: "/b"}}},
		{name: "invalid image rejected", shortcuts: []FeatureShortcut{{ID: "bad", Title: "Bad", URL: "/bad", Icon: "data:image/png;base64,PHNjcmlwdD4="}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := validateFeatureShortcuts(tc.shortcuts) == nil
			if got != tc.valid {
				t.Fatalf("valid = %v, want %v", got, tc.valid)
			}
		})
	}
}

func TestFeatureShortcutsRoundTrip(t *testing.T) {
	h := newHarness(t)
	_, sess := registered(t, h, "Shortcuts")
	api := NewHandler(HandlerDeps{
		Service: h.svc, Log: slog.New(slog.NewTextHandler(io.Discard, nil)), Redis: h.rdb,
		Tokens: h.svc.Tokens, Perms: h.svc.Perms,
	})
	router := chi.NewRouter()
	api.Routes(router)
	request := func(method, body string, token bool) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, "/auth/feature-shortcuts", strings.NewReader(body))
		if token {
			req.Header.Set("Authorization", "Bearer "+sess.AccessToken)
		}
		if body != "" {
			req.Header.Set("Content-Type", "application/json")
		}
		response := httptest.NewRecorder()
		router.ServeHTTP(response, req)
		return response
	}

	unauthorized := request(http.MethodGet, "", false)
	if unauthorized.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated GET status = %d, want 401", unauthorized.Code)
	}
	payload := `{"shortcuts":[{"id":"pos","title":"Kasir","url":"/kasir","icon":""}]}`
	stored := request(http.MethodPut, payload, true)
	if stored.Code != http.StatusOK {
		t.Fatalf("PUT status = %d, body=%s", stored.Code, stored.Body.String())
	}
	loaded := request(http.MethodGet, "", true)
	if loaded.Code != http.StatusOK {
		t.Fatalf("GET status = %d, body=%s", loaded.Code, loaded.Body.String())
	}
	var response featureShortcutsResponse
	if err := json.Unmarshal(loaded.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if len(response.Shortcuts) != 1 || response.Shortcuts[0].URL != "/kasir" {
		t.Fatalf("loaded shortcuts = %#v", response.Shortcuts)
	}
}
