package live

import (
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"aciraba/internal/authz"
	pauth "aciraba/internal/platform/auth"
	"aciraba/internal/platform/httpx"
)

// Izin: sama dengan Daftar Penjualan (omzet seluruh kasir outlet).
const (
	module = "sales_list"

	heartbeat = 20 * time.Second
	// Aliran ditutup berkala: izin/sesi dicek ulang saat klien menyambung lagi (token akses hidup 15 menit).
	maxStream = 10 * time.Minute
)

type Handler struct {
	svc      *Service
	hub      *Hub
	resolver *authz.Resolver
	tokens   *pauth.TokenIssuer
	log      *slog.Logger
}

func NewHandler(svc *Service, hub *Hub, resolver *authz.Resolver, tokens *pauth.TokenIssuer, log *slog.Logger) *Handler {
	return &Handler{svc: svc, hub: hub, resolver: resolver, tokens: tokens, log: log}
}

func (h *Handler) Routes(r chi.Router) {
	r.Route("/live", func(r chi.Router) {
		r.Use(httpx.RequireAuth(h.tokens), h.resolver.Authenticate, authz.Require(module, authz.ActView))
		r.Get("/sales/today", h.Today)
		r.Get("/sales/stream", h.Stream)
	})
}

func (h *Handler) Today(w http.ResponseWriter, r *http.Request) {
	a, _ := authz.ActorFrom(r.Context())
	res, err := h.svc.Today(r.Context(), a)
	if err != nil {
		h.log.Error("live today", "err", err)
		httpx.Error(w, http.StatusInternalServerError, "INTERNAL", "Terjadi kesalahan pada server.")
		return
	}
	httpx.JSON(w, http.StatusOK, res)
}

// Stream = Server-Sent Events. Event `sales` (data kosong) = "ada perubahan, ambil ulang ringkasan"; baris komentar
// `: ping` menjaga koneksi tetap hidup lewat proxy.
func (h *Handler) Stream(w http.ResponseWriter, r *http.Request) {
	a, _ := authz.ActorFrom(r.Context())
	rc := http.NewResponseController(w)
	// WriteTimeout server (30 dtk) tidak berlaku untuk aliran panjang.
	if err := rc.SetWriteDeadline(time.Time{}); err != nil {
		httpx.Error(w, http.StatusInternalServerError, "INTERNAL", "Aliran tidak didukung.")
		return
	}
	hd := w.Header()
	hd.Set("Content-Type", "text/event-stream")
	hd.Set("Cache-Control", "no-cache, no-transform")
	hd.Set("X-Accel-Buffering", "no") // nginx: jangan menahan aliran
	w.WriteHeader(http.StatusOK)

	sig, stop := h.hub.subscribe(a.TenantID, a.OutletID)
	defer stop()
	write := func(s string) bool {
		if _, err := fmt.Fprint(w, s); err != nil {
			return false
		}
		return rc.Flush() == nil
	}
	if !write("retry: 3000\nevent: ready\ndata: {}\n\n") {
		return
	}
	hb := time.NewTicker(heartbeat)
	defer hb.Stop()
	end := time.NewTimer(maxStream)
	defer end.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case <-end.C:
			return
		case <-h.hub.done:
			return
		case <-sig:
			if !write("event: sales\ndata: {}\n\n") {
				return
			}
		case <-hb.C:
			if !write(": ping\n\n") {
				return
			}
		}
	}
}
