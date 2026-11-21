package dangerzone

import (
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/redis/go-redis/v9"

	"aciraba/internal/authz"
	pauth "aciraba/internal/platform/auth"
	"aciraba/internal/platform/httpx"
)

type Handler struct {
	svc      *Service
	resolver *authz.Resolver
	tokens   *pauth.TokenIssuer
	rdb      *redis.Client
	log      *slog.Logger
}

// Batas percobaan per pengguna (bukan per IP): menahan tebakan kata sandi lewat sesi yang tercuri.
const (
	maxAttempts = 5
	window      = 15 * time.Minute
)

func NewHandler(svc *Service, resolver *authz.Resolver, tokens *pauth.TokenIssuer, rdb *redis.Client, log *slog.Logger) *Handler {
	return &Handler{svc: svc, resolver: resolver, tokens: tokens, rdb: rdb, log: log}
}

func (h *Handler) Routes(r chi.Router) {
	r.Route("/danger-zone", func(r chi.Router) {
		r.Use(httpx.RequireAuth(h.tokens), h.resolver.Authenticate)
		r.Post("/{scope}", h.clear)
	})
}

func (h *Handler) clear(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Password string `json:"password"`
		Confirm  string `json:"confirm"`
	}
	if !httpx.DecodeJSONLimit(w, r, &in, 4096) {
		return
	}
	a, _ := authz.ActorFrom(r.Context())
	if h.rdb != nil {
		ok, wait, err := httpx.Allow(r.Context(), h.rdb, "rl:danger-zone:"+a.TenantID.String()+":"+a.UserID.String(), maxAttempts, window)
		switch {
		case err != nil:
			httpx.Error(w, http.StatusServiceUnavailable, "UNAVAILABLE", "Layanan sementara tidak tersedia.")
			return
		case !ok:
			httpx.TooManyRequests(w, wait)
			return
		}
	}
	counts, err := h.svc.Clear(r.Context(), a, chi.URLParam(r, "scope"), in.Password, in.Confirm)
	switch {
	case err == nil:
		httpx.JSON(w, http.StatusOK, map[string]any{"deleted": counts})
	case errors.Is(err, ErrScope):
		httpx.Error(w, http.StatusNotFound, "NOT_FOUND", "Data tidak ditemukan.")
	case errors.Is(err, ErrImpersonating):
		httpx.Error(w, http.StatusForbidden, "IMPERSONATING", "Tidak tersedia pada sesi masuk-sebagai.")
	case errors.Is(err, ErrOwnerOnly):
		httpx.Error(w, http.StatusForbidden, "DANGER_OWNER_REQUIRED", "Hanya Owner toko yang dapat memakai Danger Zone.")
	case errors.Is(err, ErrConfirm):
		httpx.ValidationError(w, map[string]string{"confirm": "INVALID"})
	case errors.Is(err, ErrPassword):
		httpx.ValidationError(w, map[string]string{"password": "INVALID"})
	default:
		h.log.Error("danger zone gagal", "err", err, "req_id", middleware.GetReqID(r.Context()))
		httpx.Error(w, http.StatusInternalServerError, "INTERNAL", "Terjadi kesalahan pada server.")
	}
}
