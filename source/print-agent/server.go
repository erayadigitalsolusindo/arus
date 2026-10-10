package main

import (
	"encoding/base64"
	"encoding/json"
	"log"
	"net"
	"net/http"
	"slices"
	"strings"
	"sync"
	"time"
)

const (
	version  = "1.0.0"
	maxBody  = 512 << 10 // satu struk ESC/POS hanya beberapa KB; batas mencegah penyalahgunaan memori
	maxCopy  = 5
	jobLabel = "ARUS struk"
)

// server meneruskan byte ESC/POS dari aplikasi ARUS ke printer. Pengamanan:
//   - hanya mendengar di loopback;
//   - Host harus localhost/127.0.0.1 (menolak serangan DNS rebinding);
//   - Origin wajib ada dan termasuk daftar aplikasi ARUS (situs lain yang dibuka di PC kasir tidak bisa mencetak);
//   - POST wajib application/json, sehingga browser selalu melakukan preflight CORS lebih dulu.
type server struct {
	cfg     Config
	printer Printer
	mu      sync.Mutex // satu pekerjaan cetak sekali jalan: struk tidak saling bercampur
	log     *log.Logger
}

func (s *server) routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/status", s.status)
	mux.HandleFunc("/print", s.print)
	return s.guard(mux)
}

func (s *server) guard(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		host, _, err := net.SplitHostPort(r.Host)
		if err != nil {
			host = r.Host
		}
		if host != "127.0.0.1" && host != "localhost" && host != "[::1]" && host != "::1" {
			http.Error(w, "host tidak diizinkan", http.StatusForbidden)
			return
		}
		origin := r.Header.Get("Origin")
		if !slices.Contains(s.cfg.Origins, origin) {
			s.log.Printf("tolak origin %q %s %s", origin, r.Method, r.URL.Path)
			http.Error(w, "origin tidak diizinkan", http.StatusForbidden)
			return
		}
		h := w.Header()
		h.Set("Access-Control-Allow-Origin", origin)
		h.Set("Vary", "Origin")
		h.Set("Access-Control-Allow-Methods", "GET, POST")
		h.Set("Access-Control-Allow-Headers", "Content-Type")
		h.Set("Access-Control-Max-Age", "600")
		// Chrome (Private/Local Network Access): halaman publik yang memanggil 127.0.0.1 butuh izin eksplisit ini.
		if r.Header.Get("Access-Control-Request-Private-Network") == "true" {
			h.Set("Access-Control-Allow-Private-Network", "true")
		}
		h.Set("Cache-Control", "no-store")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func (s *server) status(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "metode tidak didukung", http.StatusMethodNotAllowed)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "version": version, "printer": s.printer.Name()})
}

type printRequest struct {
	// Data = byte ESC/POS (base64) yang disusun aplikasi; agent tidak mengubah isinya.
	Data   string `json:"data"`
	Copies int    `json:"copies"`
}

func (s *server) print(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "metode tidak didukung", http.StatusMethodNotAllowed)
		return
	}
	if ct := r.Header.Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		writeJSON(w, http.StatusUnsupportedMediaType, map[string]any{"ok": false, "error": "Content-Type harus application/json"})
		return
	}
	var req printRequest
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBody))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "isi permintaan tidak valid"})
		return
	}
	data, err := base64.StdEncoding.DecodeString(req.Data)
	if err != nil || len(data) == 0 {
		writeJSON(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "data cetak kosong atau tidak valid"})
		return
	}
	copies := min(max(req.Copies, 1), maxCopy)

	s.mu.Lock()
	defer s.mu.Unlock()
	start := time.Now()
	for range copies {
		if err := s.printer.Print(jobLabel, data); err != nil {
			s.log.Printf("gagal cetak ke %s: %v", s.printer.Name(), err)
			writeJSON(w, http.StatusBadGateway, map[string]any{"ok": false, "error": err.Error(), "printer": s.printer.Name()})
			return
		}
	}
	s.log.Printf("cetak %d byte ×%d ke %s (%s)", len(data), copies, s.printer.Name(), time.Since(start).Round(time.Millisecond))
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "printer": s.printer.Name()})
}
