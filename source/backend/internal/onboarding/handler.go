package onboarding

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

// Routes: status boleh dibaca semua pengguna masuk; menutup wizard hanya untuk pemilik bagan akun (accounts.create),
// orang yang sama yang bisa memasang template.
func (h *Handler) Routes(r chi.Router) {
	r.Route("/onboarding", func(r chi.Router) {
		r.Use(httpx.RequireAuth(h.tokens), h.resolver.Authenticate)
		r.Get("/", h.Get)
		r.With(authz.Require("accounts", authz.ActCreate)).Post("/complete", h.Complete)
	})
}

func actor(r *http.Request) authz.Actor {
	a, _ := authz.ActorFrom(r.Context())
	return a
}

func (h *Handler) Get(w http.ResponseWriter, r *http.Request) {
	st, err := h.svc.Get(r.Context(), actor(r))
	if err != nil {
		h.internal(w, r, "status onboarding gagal", err)
		return
	}
	httpx.JSON(w, http.StatusOK, st)
}

func (h *Handler) Complete(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Skipped bool `json:"skipped"`
	}
	if !httpx.DecodeJSONLimit(w, r, &in, 1<<10) {
		return
	}
	if err := h.svc.Complete(r.Context(), actor(r), in.Skipped); err != nil {
		h.internal(w, r, "tutup onboarding gagal", err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) internal(w http.ResponseWriter, r *http.Request, msg string, err error) {
	h.log.Error(msg, "err", err, "path", r.URL.Path)
	httpx.Error(w, http.StatusInternalServerError, "INTERNAL", "Terjadi kesalahan pada server.")
}
