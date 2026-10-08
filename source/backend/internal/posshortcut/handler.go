package posshortcut

import (
	"errors"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

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
	r.Route("/pos/shortcuts", func(r chi.Router) {
		r.Use(httpx.RequireAuth(h.tokens), h.resolver.Authenticate)
		// Milik kasir sendiri; cukup izin membuat nota.
		r.With(authz.Require("sales_orders", authz.ActCreate)).Get("/", h.List)
		r.With(authz.Require("sales_orders", authz.ActCreate)).Put("/{slot}", h.Set)
		r.With(authz.Require("sales_orders", authz.ActCreate)).Delete("/{slot}", h.Clear)
	})
}

func actor(r *http.Request) authz.Actor {
	a, _ := authz.ActorFrom(r.Context())
	return a
}

func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	list, err := h.svc.List(r.Context(), actor(r))
	if err != nil {
		h.fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"data": list, "slots": Slots})
}

func (h *Handler) Set(w http.ResponseWriter, r *http.Request) {
	slot, _ := strconv.Atoi(chi.URLParam(r, "slot"))
	var req struct {
		ItemID uuid.UUID `json:"item_id"`
	}
	if !httpx.DecodeJSON(w, r, &req) {
		return
	}
	if err := h.svc.Set(r.Context(), actor(r), slot, req.ItemID); err != nil {
		h.fail(w, r, err)
		return
	}
	h.List(w, r)
}

func (h *Handler) Clear(w http.ResponseWriter, r *http.Request) {
	slot, _ := strconv.Atoi(chi.URLParam(r, "slot"))
	if err := h.svc.Clear(r.Context(), actor(r), slot); err != nil {
		h.fail(w, r, err)
		return
	}
	h.List(w, r)
}

func (h *Handler) fail(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, ErrBadSlot):
		httpx.ValidationError(w, map[string]string{"slot": "INVALID"})
	case errors.Is(err, ErrNoItem):
		httpx.ValidationError(w, map[string]string{"item_id": "INVALID"})
	default:
		h.log.ErrorContext(r.Context(), "posshortcut", "err", err, "path", r.URL.Path)
		httpx.Error(w, http.StatusInternalServerError, "INTERNAL", "Terjadi kesalahan pada server.")
	}
}
