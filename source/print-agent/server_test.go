package main

import (
	"bytes"
	"encoding/base64"
	"errors"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

type fakePrinter struct {
	mu   sync.Mutex
	jobs [][]byte
	fail error
}

func (f *fakePrinter) Name() string { return "Uji" }
func (f *fakePrinter) Print(_ string, data []byte) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.fail != nil {
		return f.fail
	}
	f.jobs = append(f.jobs, append([]byte(nil), data...))
	return nil
}

const app = "https://arus.contoh.id"

func newTestServer(p Printer) http.Handler {
	s := &server{cfg: Config{Listen: "127.0.0.1:9100", Origins: []string{app}}, printer: p, log: log.New(io.Discard, "", 0)}
	return s.routes()
}

func do(h http.Handler, method, path, origin, host, ctype, body string, hdr ...string) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, "http://"+host+path, strings.NewReader(body))
	r.Host = host
	if origin != "" {
		r.Header.Set("Origin", origin)
	}
	if ctype != "" {
		r.Header.Set("Content-Type", ctype)
	}
	for i := 0; i+1 < len(hdr); i += 2 {
		r.Header.Set(hdr[i], hdr[i+1])
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}

func job(data string, copies int) string {
	b64 := base64.StdEncoding.EncodeToString([]byte(data))
	if copies == 0 {
		return `{"data":"` + b64 + `"}`
	}
	return `{"data":"` + b64 + `","copies":` + string(rune('0'+copies)) + `}`
}

func TestPrintPassesBytesThroughUnchanged(t *testing.T) {
	p := &fakePrinter{}
	h := newTestServer(p)
	payload := "\x1b@HALO\x1dV\x42\x00\xff"
	w := do(h, "POST", "/print", app, "127.0.0.1:9100", "application/json", job(payload, 2))
	if w.Code != 200 || w.Header().Get("Access-Control-Allow-Origin") != app {
		t.Fatalf("status %d %s", w.Code, w.Body)
	}
	if len(p.jobs) != 2 || !bytes.Equal(p.jobs[0], []byte(payload)) {
		t.Fatalf("byte harus diteruskan apa adanya, %d salinan: %q", len(p.jobs), p.jobs)
	}
}

func TestGuardsRejectForeignCallers(t *testing.T) {
	p := &fakePrinter{}
	h := newTestServer(p)
	body := job("x", 0)
	cases := []struct {
		name, origin, host, ctype string
		want                      int
	}{
		{"situs lain", "https://jahat.example", "127.0.0.1:9100", "application/json", 403},
		{"tanpa origin", "", "127.0.0.1:9100", "application/json", 403},
		{"origin mirip", app + ".jahat.example", "127.0.0.1:9100", "application/json", 403},
		{"dns rebinding", app, "jahat.example:9100", "application/json", 403},
		{"form tanpa preflight", app, "127.0.0.1:9100", "text/plain", 415},
		{"localhost boleh", app, "localhost:9100", "application/json", 200},
	}
	for _, c := range cases {
		if w := do(h, "POST", "/print", c.origin, c.host, c.ctype, body); w.Code != c.want {
			t.Errorf("%s: status %d, mau %d", c.name, w.Code, c.want)
		}
	}
	if len(p.jobs) != 1 {
		t.Fatalf("hanya permintaan sah yang tercetak: %d", len(p.jobs))
	}
}

func TestPreflightAllowsPrivateNetwork(t *testing.T) {
	h := newTestServer(&fakePrinter{})
	w := do(h, "OPTIONS", "/print", app, "127.0.0.1:9100", "", "", "Access-Control-Request-Private-Network", "true", "Access-Control-Request-Method", "POST")
	if w.Code != 204 || w.Header().Get("Access-Control-Allow-Private-Network") != "true" || w.Header().Get("Access-Control-Allow-Origin") != app {
		t.Fatalf("preflight: %d %v", w.Code, w.Header())
	}
	if w := do(h, "OPTIONS", "/print", "https://jahat.example", "127.0.0.1:9100", "", ""); w.Code != 403 {
		t.Fatalf("preflight origin asing harus ditolak: %d", w.Code)
	}
}

func TestBadRequestsAndPrinterErrors(t *testing.T) {
	p := &fakePrinter{}
	h := newTestServer(p)
	for _, body := range []string{`{"data":"bukan base64!"}`, `{"data":""}`, `{"data":"QQ==","lain":1}`, `bukan json`, `{"data":"` + strings.Repeat("A", maxBody) + `"}`} {
		if w := do(h, "POST", "/print", app, "127.0.0.1:9100", "application/json", body); w.Code != 400 {
			t.Errorf("%.30q: status %d", body, w.Code)
		}
	}
	p.fail = errors.New("printer offline")
	if w := do(h, "POST", "/print", app, "127.0.0.1:9100", "application/json", job("x", 0)); w.Code != 502 || !strings.Contains(w.Body.String(), "printer offline") {
		t.Fatalf("galat printer: %d %s", w.Code, w.Body)
	}
	if w := do(h, "GET", "/print", app, "127.0.0.1:9100", "", ""); w.Code != 405 {
		t.Fatalf("GET /print: %d", w.Code)
	}
	if w := do(h, "GET", "/status", app, "127.0.0.1:9100", "", ""); w.Code != 200 || !strings.Contains(w.Body.String(), `"printer":"Uji"`) {
		t.Fatalf("status: %d %s", w.Code, w.Body)
	}
}

func TestConfigDefaultsAndValidation(t *testing.T) {
	path := filepath.Join(t.TempDir(), "print-agent.json")
	cfg, err := loadConfig(path)
	if err != nil || cfg.Listen != "127.0.0.1:9100" {
		t.Fatalf("bawaan: %+v %v", cfg, err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatal("berkas konfigurasi bawaan harus dibuat")
	}
	for _, bad := range []string{
		`{"listen":"0.0.0.0:9100"}`,
		`{"origins":["arus.contoh.id"]}`,
		`{"origins":["https://arus.contoh.id/kasir"]}`,
		`{"origins":[]}`,
	} {
		_ = os.WriteFile(path, []byte(bad), 0o644)
		if _, err := loadConfig(path); err == nil {
			t.Errorf("%s harus ditolak", bad)
		}
	}
	_ = os.WriteFile(path, []byte(`{"origins":["https://arus.contoh.id/"],"printer":"POS-58"}`), 0o644)
	if cfg, err := loadConfig(path); err != nil || cfg.Origins[0] != "https://arus.contoh.id" || cfg.Printer != "POS-58" {
		t.Fatalf("garis miring akhir dibuang: %+v %v", cfg, err)
	}
}
