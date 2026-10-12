package erapay

import (
	"errors"
	"log/slog"
	"net/http"
	"regexp"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"aciraba/internal/authz"
	pauth "aciraba/internal/platform/auth"
	"aciraba/internal/platform/httpx"
)

var keyPattern = regexp.MustCompile(`^[A-Za-z0-9._:-]{8,100}$`)

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
	r.Route("/payments/erapay", func(r chi.Router) {
		r.Use(httpx.RequireAuth(h.tokens), h.resolver.Authenticate)
		sale := authz.Require("sales_orders", authz.ActCreate)
		r.With(sale).Get("/config", h.config)
		r.With(sale).Post("/charges", h.create)
		r.With(sale).Get("/charges/{id}", h.get)
		pm := authz.Require("payment_methods", authz.ActUpdate)
		r.With(pm).Put("/key", h.saveKey)
		r.With(pm).Delete("/key", h.removeKey)
	})
}

func actor(r *http.Request) authz.Actor {
	a, _ := authz.ActorFrom(r.Context())
	return a
}

func (h *Handler) fail(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, ErrDisabled):
		httpx.Error(w, http.StatusConflict, "ERAPAY_DISABLED", "Integrasi EraPay belum diaktifkan di server.")
	case errors.Is(err, ErrNotConfigured):
		httpx.Error(w, http.StatusConflict, "ERAPAY_NOT_CONFIGURED", "API key EraPay belum dipasang untuk toko ini.")
	case errors.Is(err, ErrNotFound):
		httpx.Error(w, http.StatusNotFound, "NOT_FOUND", "Data tidak ditemukan.")
	case errors.Is(err, ErrAmount):
		httpx.ValidationError(w, map[string]string{"amount": "INVALID"})
	case errors.Is(err, ErrKeyConflict):
		httpx.Error(w, http.StatusConflict, "IDEMPOTENCY_CONFLICT", "Kunci sudah dipakai untuk nominal lain.")
	case errors.Is(err, ErrRejected):
		httpx.Error(w, http.StatusUnprocessableEntity, "ERAPAY_REJECTED", "EraPay menolak tagihan QRIS ini.")
	case errors.Is(err, ErrBadKey):
		httpx.ValidationError(w, map[string]string{"api_key": "INVALID"})
	case errors.Is(err, ErrKeyInvalid):
		httpx.Error(w, http.StatusUnprocessableEntity, "ERAPAY_KEY_INVALID", "API key ditolak EraPay (tidak dikenal, dicabut, atau kedaluwarsa).")
	case errors.Is(err, ErrKeyScope):
		httpx.Error(w, http.StatusUnprocessableEntity, "ERAPAY_KEY_SCOPE", "API key harus punya izin payment:create dan payment:read.")
	case errors.Is(err, ErrKeyEnvironment):
		httpx.Error(w, http.StatusUnprocessableEntity, "ERAPAY_KEY_SANDBOX", "API key sandbox tidak boleh dipakai di server production.")
	case errors.Is(err, ErrImpersonating):
		httpx.Error(w, http.StatusForbidden, "IMPERSONATING", "Tidak tersedia pada sesi masuk-sebagai.")
	case errors.Is(err, ErrUnavailable):
		httpx.Error(w, http.StatusBadGateway, "ERAPAY_UNAVAILABLE", "EraPay tidak dapat dihubungi, coba lagi.")
	default:
		h.log.Error("erapay gagal", "err", err, "req_id", middleware.GetReqID(r.Context()))
		httpx.Error(w, http.StatusInternalServerError, "INTERNAL", "Terjadi kesalahan pada server.")
	}
}

func (h *Handler) config(w http.ResponseWriter, r *http.Request) {
	cfg, err := h.svc.Config(r.Context(), actor(r))
	if err != nil {
		h.fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, cfg)
}

func (h *Handler) create(w http.ResponseWriter, r *http.Request) {
	key := r.Header.Get("Idempotency-Key")
	if !keyPattern.MatchString(key) {
		httpx.Error(w, http.StatusBadRequest, "IDEMPOTENCY_KEY_REQUIRED", "Header Idempotency-Key wajib (8–100 karakter).")
		return
	}
	var in struct {
		Amount string `json:"amount"`
	}
	if !httpx.DecodeJSON(w, r, &in) {
		return
	}
	amount, err := decimal.NewFromString(in.Amount)
	if err != nil {
		httpx.ValidationError(w, map[string]string{"amount": "INVALID"})
		return
	}
	c, err := h.svc.CreateCharge(r.Context(), actor(r), key, amount)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, c)
}

func (h *Handler) get(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		httpx.Error(w, http.StatusNotFound, "NOT_FOUND", "Data tidak ditemukan.")
		return
	}
	c, err := h.svc.GetCharge(r.Context(), actor(r), id)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, c)
}

func (h *Handler) saveKey(w http.ResponseWriter, r *http.Request) {
	var in struct {
		APIKey string `json:"api_key"`
	}
	if !httpx.DecodeJSONLimit(w, r, &in, 4<<10) {
		return
	}
	cfg, err := h.svc.SaveKey(r.Context(), actor(r), in.APIKey)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, cfg)
}

func (h *Handler) removeKey(w http.ResponseWriter, r *http.Request) {
	if err := h.svc.RemoveKey(r.Context(), actor(r)); err != nil {
		h.fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
