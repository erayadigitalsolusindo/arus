package subscription

import (
	"errors"
	"log/slog"
	"net/http"
	"regexp"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/google/uuid"

	"aciraba/internal/authz"
	"aciraba/internal/erapay"
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
	r.Route("/subscription", func(r chi.Router) {
		r.Use(httpx.RequireAuth(h.tokens), h.resolver.Authenticate)
		view := authz.Require("subscription", authz.ActView)
		r.With(view).Get("/", h.status)
		r.With(view).Post("/checkout", h.checkout)
		r.With(view).Get("/payments/{id}", h.payment)
	})
}

func actor(r *http.Request) authz.Actor {
	a, _ := authz.ActorFrom(r.Context())
	return a
}

func (h *Handler) fail(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, ErrOwnerOnly):
		httpx.Error(w, http.StatusForbidden, "OWNER_REQUIRED", "Hanya Owner toko yang dapat membeli paket.")
	case errors.Is(err, ErrImpersonating):
		httpx.Error(w, http.StatusForbidden, "IMPERSONATING", "Tidak tersedia pada sesi masuk-sebagai.")
	case errors.Is(err, ErrDisabled):
		httpx.Error(w, http.StatusConflict, "PAYMENTS_DISABLED", "Pembayaran paket belum diaktifkan di server.")
	case errors.Is(err, ErrPlanNotFound):
		httpx.Error(w, http.StatusNotFound, "PLAN_NOT_FOUND", "Paket tidak tersedia.")
	case errors.Is(err, ErrNotFound):
		httpx.Error(w, http.StatusNotFound, "NOT_FOUND", "Data tidak ditemukan.")
	case errors.Is(err, ErrKeyConflict):
		httpx.Error(w, http.StatusConflict, "IDEMPOTENCY_CONFLICT", "Kunci sudah dipakai untuk paket lain.")
	case errors.Is(err, erapay.ErrRejected):
		httpx.Error(w, http.StatusUnprocessableEntity, "ERAPAY_REJECTED", "EraPay menolak tagihan QRIS ini.")
	case errors.Is(err, ErrUnavailable):
		httpx.Error(w, http.StatusBadGateway, "ERAPAY_UNAVAILABLE", "EraPay tidak dapat dihubungi, coba lagi.")
	default:
		h.log.Error("subscription gagal", "err", err, "req_id", middleware.GetReqID(r.Context()))
		httpx.Error(w, http.StatusInternalServerError, "INTERNAL", "Terjadi kesalahan pada server.")
	}
}

func (h *Handler) status(w http.ResponseWriter, r *http.Request) {
	st, err := h.svc.Status(r.Context(), actor(r))
	if err != nil {
		h.fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, st)
}

func (h *Handler) checkout(w http.ResponseWriter, r *http.Request) {
	key := r.Header.Get("Idempotency-Key")
	if !keyPattern.MatchString(key) {
		httpx.Error(w, http.StatusBadRequest, "IDEMPOTENCY_KEY_REQUIRED", "Header Idempotency-Key wajib (8–100 karakter).")
		return
	}
	var in struct {
		PlanID uuid.UUID `json:"plan_id"`
	}
	if !httpx.DecodeJSON(w, r, &in) {
		return
	}
	if in.PlanID == uuid.Nil {
		httpx.ValidationError(w, map[string]string{"plan_id": "REQUIRED"})
		return
	}
	p, err := h.svc.Checkout(r.Context(), actor(r), key, in.PlanID)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, p)
}

func (h *Handler) payment(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		httpx.Error(w, http.StatusNotFound, "NOT_FOUND", "Data tidak ditemukan.")
		return
	}
	p, err := h.svc.GetPayment(r.Context(), actor(r), id)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, p)
}
