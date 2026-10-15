package extapi

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/go-chi/chi/v5"

	"aciraba/internal/authz"
)

// Vektor uji yang sama dipajang di portal dokumentasi: developer memverifikasi implementasinya terhadap nilai ini.
const (
	vecSecret = "sk_test_0123456789abcdef"
	vecSig    = "4bc13ded8954dee32457b668cb562c1939966f9196610b8034fbb1e7642a1366" // JANGAN diubah tanpa memperbarui portal dokumentasi
)

func vector() (string, string) {
	q := url.Values{"to": {"2026-10-31"}, "from": {"2026-10-01"}, "outlet_id": {"a b"}}
	sts := StringToSign("get", "/ext/v1/accounting/journals", q, "1760000000", "nonce-0123456789abcdef", BodyHash(nil))
	return sts, Sign(vecSecret, sts)
}

func TestCanonicalString(t *testing.T) {
	sts, _ := vector()
	want := "HMAC-SHA256\nGET\n/ext/v1/accounting/journals\nfrom=2026-10-01&outlet_id=a+b&to=2026-10-31\n1760000000\nnonce-0123456789abcdef\n" +
		"e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"
	if sts != want {
		t.Fatalf("string kanonik:\n%q\nwant\n%q", sts, want)
	}
}

func TestSignDeterministic(t *testing.T) {
	_, a := vector()
	_, b := vector()
	if a != b || len(a) != 64 {
		t.Fatalf("tanda tangan tidak stabil: %q vs %q", a, b)
	}
	if !Equal(a, a) || Equal(a, Sign("lain", "x")) {
		t.Fatal("Equal salah")
	}
	if a != vecSig {
		t.Fatalf("vektor uji berubah: %s (portal dokumentasi memakai %s)", a, vecSig)
	}
}

func TestIPAllowed(t *testing.T) {
	list, err := NormalizeIPs([]string{"203.0.113.7", "198.51.100.0/24", "2001:db8::/32", "203.0.113.7", " 10.0.0.5/8 "})
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 4 || list[3] != "10.0.0.0/8" {
		t.Fatalf("normalisasi: %v", list)
	}
	for ip, want := range map[string]bool{
		"203.0.113.7": true, "203.0.113.8": false, "198.51.100.200": true, "198.51.101.1": false,
		"2001:db8::1": true, "2001:dead::1": false, "10.9.9.9": true, "::ffff:203.0.113.7": true, "bukan-ip": false,
	} {
		if got := IPAllowed(list, ip); got != want {
			t.Errorf("IPAllowed(%q) = %v, want %v", ip, got, want)
		}
	}
	if IPAllowed(nil, "203.0.113.7") {
		t.Error("daftar kosong harus menolak")
	}
	for _, bad := range [][]string{nil, {}, {"300.1.1.1"}, {"10.0.0.0/33"}, {"x"}} {
		if _, err := NormalizeIPs(bad); err == nil {
			t.Errorf("NormalizeIPs(%v) harus galat", bad)
		}
	}
}

func TestBlocked(t *testing.T) {
	for p, want := range map[string]bool{
		"/auth/login": true, "/platform/tenants": true, "/public/x": true, "/mcp": true, "/oauth/token": true, "/ext/v1/x": true,
		"/accounting/journals": false, "/authors": false, "/items": false,
	} {
		if Blocked(p) != want {
			t.Errorf("Blocked(%q) != %v", p, want)
		}
	}
}

func TestBrandValidate(t *testing.T) {
	ok := BrandInput{Slug: "Mitra-1", Domain: "Docs.Mitra.COM", Name: " Mitra API ", Accent: "#06b6d4", BaseURL: "https://api.mitra.com/"}
	if f := ok.Validate(); len(f) != 0 || ok.Slug != "mitra-1" || ok.Domain != "docs.mitra.com" || ok.BaseURL != "https://api.mitra.com" {
		t.Fatalf("valid ditolak/tidak dinormalkan: %v %+v", f, ok)
	}
	bad := BrandInput{Slug: "-x", Domain: "bad domain", Name: "", Accent: "red", BaseURL: "ftp://x", LogoURL: "javascript:alert(1)", HiddenModules: []string{"Ok", "bad mod"}}
	f := bad.Validate()
	for _, k := range []string{"slug", "domain", "name", "accent", "base_url", "logo_url", "hidden_modules"} {
		if f[k] == "" {
			t.Errorf("field %s harus galat; got %v", k, f)
		}
	}
}

func TestAutoSummaryAndGroup(t *testing.T) {
	for _, c := range []struct{ m, p, want string }{
		{"GET", "/accounting/journals", "Daftar accounting journals"},
		{"GET", "/accounting/journals/{id}", "Detail journals"},
		{"POST", "/accounting/journals/{id}/post", "Posting journals"},
		{"GET", "/reports/sales/by-date", "Laporan sales by-date"},
		{"DELETE", "/items/{id}", "Hapus items"},
	} {
		if got := autoSummary(c.m, c.p); got != c.want {
			t.Errorf("autoSummary(%s %s) = %q, want %q", c.m, c.p, got, c.want)
		}
	}
	if groupOf("/reports/sales/by-date") != "reports-sales" || groupOf("/items/{id}") != "items" || groupOf("/reports/{x}") != "reports" {
		t.Error("groupOf salah")
	}
}

// Izin ditemukan dari middleware tanpa mengubah handler: Require, RequireAny, dan group yang hanya butuh login.
func TestCatalogDiscoversPermissions(t *testing.T) {
	r := chi.NewRouter()
	ok := http.HandlerFunc(func(http.ResponseWriter, *http.Request) {})
	r.With(authz.Require("journals", authz.ActCreate)).Post("/accounting/journals", ok)
	r.With(authz.RequireAny([2]string{"items", authz.ActView}, [2]string{"stock_card", authz.ActView})).Get("/items/search", ok)
	r.Get("/open/thing", ok)
	r.Get("/auth/me", ok)
	r.Get("/platform/tenants", ok)

	cat := NewCatalogBuilder(r).Get()
	by := map[string]Endpoint{}
	for _, e := range cat.Endpoints {
		by[e.Method+" "+e.Path] = e
	}
	if _, found := by["GET /platform/tenants"]; found {
		t.Error("rute /platform tidak boleh masuk katalog")
	}
	if e := by["POST /accounting/journals"]; len(e.Permissions) != 1 || e.Permissions[0] != (PermRef{"journals", "create"}) || e.Auth != "hmac" {
		t.Errorf("Require: %+v", e)
	}
	if e := by["GET /items/search"]; len(e.Permissions) != 2 {
		t.Errorf("RequireAny: %+v", e.Permissions)
	}
	if e := by["GET /open/thing"]; len(e.Permissions) != 0 {
		t.Errorf("tanpa izin: %+v", e.Permissions)
	}
	if e := by["GET /auth/me"]; e.Auth != "jwt" {
		t.Errorf("auth harus jwt: %+v", e)
	}
	if len(cat.Filter([]string{"accounting"}).Endpoints) != len(cat.Endpoints)-1 {
		t.Error("Filter tidak menyembunyikan grup")
	}
	_ = httptest.NewRecorder
}

func slogDiscard() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }
