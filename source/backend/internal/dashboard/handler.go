package dashboard

import (
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"

	"aciraba/internal/authz"
	pauth "aciraba/internal/platform/auth"
	"aciraba/internal/platform/httpx"
)

type Handler struct {
	svc      *Service
	resolver *authz.Resolver
	tokens   *pauth.TokenIssuer
	log      *slog.Logger
}

func NewHandler(svc *Service, resolver *authz.Resolver, tokens *pauth.TokenIssuer, log *slog.Logger) *Handler {
	return &Handler{svc: svc, resolver: resolver, tokens: tokens, log: log}
}

// Routes: dasbor terbuka bagi semua pengguna yang masuk; bagian yang dikirim mengikuti izin pemanggil (lihat Overview).
func (h *Handler) Routes(r chi.Router) {
	r.Route("/dashboard", func(r chi.Router) {
		r.Use(httpx.RequireAuth(h.tokens), h.resolver.Authenticate)
		r.Get("/overview", h.Overview)
	})
}

// Overview: GET /dashboard/overview?scope=all
func (h *Handler) Overview(w http.ResponseWriter, r *http.Request) {
	a, _ := authz.ActorFrom(r.Context())
	res, err := h.svc.Overview(r.Context(), a, r.URL.Query().Get("scope") == "all")
	if err != nil {
		h.log.ErrorContext(r.Context(), "dashboard", "err", err)
		httpx.Error(w, http.StatusInternalServerError, "INTERNAL", "Terjadi kesalahan pada server.")
		return
	}
	httpx.JSON(w, http.StatusOK, res)
}
