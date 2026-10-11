package ai

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

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

func (h *Handler) Routes(r chi.Router) {
	r.Route("/ai", func(r chi.Router) {
		r.Use(httpx.RequireAuth(h.tokens), h.resolver.Authenticate)
		r.With(authz.Require(Module, authz.ActView)).Get("/settings", h.GetSettings)
		r.With(authz.Require(Module, authz.ActView)).Post("/chat", h.Chat)
		r.With(authz.Require(Module, authz.ActView)).Get("/connections", h.Connections)
		r.With(authz.Require(Module, authz.ActView)).Delete("/connections/{id}", h.RevokeConnection)
		r.With(authz.Require(Module, authz.ActUpdate)).Put("/settings", h.PutSettings)
		r.With(authz.Require(Module, authz.ActUpdate)).Delete("/settings/key", h.DeleteKey)
	})
}

func (h *Handler) fail(w http.ResponseWriter, r *http.Request, err error) {
	var up *upstreamError
	switch {
	case errors.Is(err, ErrNotConfigured):
		httpx.Error(w, http.StatusConflict, "AI_NOT_CONFIGURED", "API key AI belum diatur.")
	case errors.Is(err, ErrBadKey):
		httpx.Error(w, http.StatusUnprocessableEntity, "AI_KEY_INVALID", "API key ditolak oleh Anthropic.")
	case errors.Is(err, ErrRateLimited):
		httpx.Error(w, http.StatusTooManyRequests, "AI_RATE_LIMITED", "Terlalu banyak pertanyaan; coba lagi beberapa menit lagi.")
	case errors.Is(err, ErrFreeQuota):
		httpx.Error(w, http.StatusTooManyRequests, "AI_FREE_QUOTA_EXHAUSTED", "Kuota akun gratis hari ini habis.")
	case errors.Is(err, ErrBadInput):
		httpx.Error(w, http.StatusUnprocessableEntity, "AI_BAD_INPUT", "Percakapan tidak valid.")
	case errors.Is(err, ErrTooManySteps):
		httpx.Error(w, http.StatusUnprocessableEntity, "AI_TOO_MANY_STEPS", "Analisis terlalu panjang; pecah menjadi pertanyaan yang lebih spesifik.")
	case errors.Is(err, ErrImpersonating):
		httpx.Error(w, http.StatusForbidden, "AI_FORBIDDEN", "Asisten AI tidak tersedia pada sesi masuk-sebagai.")
	case errors.As(err, &up):
		status := http.StatusBadGateway
		if up.Code == "AI_KEY_INVALID" || up.Code == "AI_NO_CREDIT" {
			status = http.StatusUnprocessableEntity
		}
		h.log.WarnContext(r.Context(), "ai upstream", "code", up.Code, "status", up.Status)
		httpx.Error(w, status, up.Code, "Layanan AI tidak dapat memproses permintaan.")
	case errors.Is(err, context.Canceled):
		// klien menutup koneksi; tidak perlu respons
	default:
		h.log.ErrorContext(r.Context(), "ai", "err", err)
		httpx.Error(w, http.StatusInternalServerError, "INTERNAL", "Terjadi kesalahan pada server.")
	}
}

func (h *Handler) GetSettings(w http.ResponseWriter, r *http.Request) {
	a, _ := authz.ActorFrom(r.Context())
	st, err := h.svc.Settings(r.Context(), a)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, st)
}

type putSettingsReq struct {
	APIKey string `json:"api_key"` // kosong = hanya ganti model
	Model  string `json:"model"`
}

func (h *Handler) PutSettings(w http.ResponseWriter, r *http.Request) {
	a, _ := authz.ActorFrom(r.Context())
	var in putSettingsReq
	if !httpx.DecodeJSONLimit(w, r, &in, 4<<10) {
		return
	}
	if err := h.svc.Save(r.Context(), a, in.APIKey, in.Model); err != nil {
		h.fail(w, r, err)
		return
	}
	st, err := h.svc.Settings(r.Context(), a)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, st)
}

func (h *Handler) DeleteKey(w http.ResponseWriter, r *http.Request) {
	a, _ := authz.ActorFrom(r.Context())
	if err := h.svc.RemoveKey(r.Context(), a); err != nil {
		h.fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

type chatReq struct {
	Messages []ChatMessage `json:"messages"`
}

// Chat: POST /ai/chat. Satu pertanyaan bisa memakan beberapa putaran tool (puluhan detik): batas tulis server (30 dtk) dilonggarkan
// khusus untuk request ini.
func (h *Handler) Chat(w http.ResponseWriter, r *http.Request) {
	a, _ := authz.ActorFrom(r.Context())
	var in chatReq
	if !httpx.DecodeJSONLimit(w, r, &in, 256<<10) {
		return
	}
	_ = http.NewResponseController(w).SetWriteDeadline(time.Now().Add(4 * time.Minute))
	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Minute)
	defer cancel()
	res, err := h.svc.Chat(ctx, a, in.Messages)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, res)
}

func (h *Handler) Connections(w http.ResponseWriter, r *http.Request) {
	a, _ := authz.ActorFrom(r.Context())
	list, err := h.svc.Connections(r.Context(), a)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"connections": list})
}

func (h *Handler) RevokeConnection(w http.ResponseWriter, r *http.Request) {
	a, _ := authz.ActorFrom(r.Context())
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		httpx.Error(w, http.StatusNotFound, "NOT_FOUND", "Tidak ditemukan.")
		return
	}
	if err := h.svc.RevokeConnection(r.Context(), a, id); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			httpx.Error(w, http.StatusNotFound, "NOT_FOUND", "Tidak ditemukan.")
			return
		}
		h.fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
