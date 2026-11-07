package storefront

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/redis/go-redis/v9"
	"log/slog"
)

// Halaman publik lewat router asli: header keamanan, isi tersaring, cache Redis, 404 untuk halaman nonaktif/tak dikenal.
func TestPublicPageHTTP(t *testing.T) {
	e := newEnv(t)
	url := os.Getenv("TEST_REDIS_URL")
	if url == "" {
		t.Skip("TEST_REDIS_URL tidak di-set")
	}
	opt, err := redis.ParseURL(url)
	if err != nil {
		t.Fatal(err)
	}
	rdb := redis.NewClient(opt)
	t.Cleanup(func() { _ = rdb.Close() })
	ctx := context.Background()

	slug := "http-" + e.tenants[0].String()[:8]
	t.Cleanup(func() { rdb.Del(ctx, "sf:page:"+slug) })
	h := NewHandler(e.svc, nil, nil, rdb, nil, slog.New(slog.NewTextHandler(io.Discard, nil)))
	r := chi.NewRouter()
	h.Routes(r)

	get := func(path string) *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, path, nil)
		req.RemoteAddr = "10.9.8.7:1234"
		r.ServeHTTP(w, req)
		return w
	}

	// Nonaktif / tak ada -> 404 berbentuk halaman, tetap ber-CSP.
	if w := get("/public/s/" + slug); w.Code != http.StatusNotFound || !strings.Contains(w.Header().Get("Content-Security-Policy"), "script-src") && !strings.Contains(w.Header().Get("Content-Security-Policy"), "default-src 'none'") {
		t.Fatalf("nonaktif: %d %v", w.Code, w.Header())
	}

	_, _, err = e.svc.Save(ctx, e.a, SaveInput{Slug: slug, Enabled: true, Title: `Toko <i>A</i>`,
		HTML: `<h1 onclick="alert(1)">{namatoko}</h1><script>alert(1)</script><a href="javascript:alert(1)">x</a><img src="https://evil.test/p.png" onerror="alert(1)">`,
		CSS:  `h1{color:#c00}`})
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"/public/s/" + slug, "/s/" + slug} {
		w := get(path)
		body := w.Body.String()
		if w.Code != 200 {
			t.Fatalf("%s: status %d: %s", path, w.Code, body)
		}
		csp := w.Header().Get("Content-Security-Policy")
		for _, must := range []string{"default-src 'none'", "style-src 'self'", "base-uri 'none'", "form-action 'none'", "frame-ancestors 'none'", "sandbox"} {
			if !strings.Contains(csp, must) {
				t.Errorf("%s: CSP tak memuat %q: %s", path, must, csp)
			}
		}
		if strings.Contains(csp, "script-src") && !strings.Contains(csp, "default-src 'none'") {
			t.Errorf("CSP mengizinkan script: %s", csp)
		}
		if strings.Contains(csp, "unsafe-inline") || strings.Contains(csp, "unsafe-eval") {
			t.Errorf("CSP tak boleh unsafe-*: %s", csp)
		}
		if w.Header().Get("X-Content-Type-Options") != "nosniff" || !strings.HasPrefix(w.Header().Get("Content-Type"), "text/html") {
			t.Errorf("header: %v", w.Header())
		}
		lower := strings.ToLower(body)
		for _, bad := range []string{"<script", "onclick", "javascript:", "onerror", "<style"} {
			if strings.Contains(lower, bad) {
				t.Errorf("%s: halaman memuat %q:\n%s", path, bad, body)
			}
		}
		for _, want := range []string{"<title>Toko &lt;i&gt;A&lt;/i&gt;</title>", "<h1>UJI-SF-", `href="/public/s-assets/tailwind.css?v=`, `href="/public/s/` + slug + `/style.css?v=1"`, `src="https://evil.test/p.png"`} {
			if !strings.Contains(body, want) {
				t.Errorf("%s: tak memuat %q:\n%s", path, want, body)
			}
		}
	}
	// Disajikan dari cache Redis pada permintaan berikutnya, dan cache dibuang saat simpan ulang.
	if n, _ := rdb.Exists(ctx, "sf:page:"+slug).Result(); n != 1 {
		t.Fatal("halaman harus tersimpan di cache")
	}

	if w := get("/public/s/" + slug + "/style.css"); w.Code != 200 || w.Body.String() != "h1{color:#c00}" || !strings.HasPrefix(w.Header().Get("Content-Type"), "text/css") {
		t.Fatalf("style.css: %d %q %v", w.Code, w.Body.String(), w.Header())
	}
	// CSS berbahaya yang (hipotetis) sudah telanjur tersimpan tidak ikut dilayani.
	e.sql(t, `UPDATE storefronts SET css = '@import url(https://evil.test/x.css);' WHERE tenant_id = $1`, e.tenants[0])
	if w := get("/public/s/" + slug + "/style.css"); w.Body.Len() != 0 {
		t.Fatalf("CSS berbahaya tersimpan harus dikosongkan: %q", w.Body.String())
	}
	if w := get("/public/s-assets/tailwind.css"); w.Code != 200 || w.Body.Len() < 100 {
		t.Fatalf("tailwind.css: %d len=%d", w.Code, w.Body.Len())
	}
	// Slug asing / bentuk tak sah / gambar tak dikenal -> 404.
	for _, p := range []string{"/public/s/tidak-ada-" + slug, "/s/AB", "/public/s/" + slug + "/img/bukan-uuid", "/public/s/" + slug + "/img/00000000-0000-0000-0000-000000000000"} {
		if w := get(p); w.Code != http.StatusNotFound {
			t.Errorf("%s: %d, want 404", p, w.Code)
		}
	}
}
