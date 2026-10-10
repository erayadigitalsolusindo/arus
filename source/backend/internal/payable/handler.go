package payable

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
	r.Route("/payables", func(r chi.Router) {
		r.Use(httpx.RequireAuth(h.tokens), h.resolver.Authenticate)
		r.With(authz.Require(Module, authz.ActView)).Get("/", h.List)
		r.With(authz.Require(Module, authz.ActCreate)).Post("/settlements/quote", h.SettleQuote)
		r.With(authz.Require(Module, authz.ActCreate)).Post("/settlements", h.Settle)
		r.With(authz.Require(Module, authz.ActView)).Get("/settlements/{id}", h.GetSettlement)
		r.With(authz.Require(Module, authz.ActView)).Get("/{id}", h.Get)
		r.With(authz.Require(Module, authz.ActCreate)).Post("/{id}/payments", h.Pay)
	})
}

func actor(r *http.Request) authz.Actor {
	a, _ := authz.ActorFrom(r.Context())
	return a
}

// List: GET /payables?supplier_id=&status=open|overdue|paid|all&q=&limit=&cursor=  (paginasi keyset)
func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	qs := r.URL.Query()
	p := ListParams{Status: qs.Get("status"), Q: qs.Get("q")}
	if s := qs.Get("supplier_id"); s != "" {
		id, err := uuid.Parse(s)
		if err != nil {
			httpx.ValidationError(w, map[string]string{"supplier_id": "INVALID"})
			return
		}
		p.SupplierID = &id
	}
	p.Limit, _ = strconv.Atoi(qs.Get("limit"))
	p.Cursor = qs.Get("cursor")
	res, err := h.svc.List(r.Context(), actor(r), p)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, res)
}

func (h *Handler) Get(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		httpx.Error(w, http.StatusNotFound, "NOT_FOUND", "Data tidak ditemukan.")
		return
	}
	d, err := h.svc.Get(r.Context(), actor(r), id)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, d)
}

// Pay: POST /payables/{id}/payments (header Idempotency-Key wajib). 201 = pembayaran baru; 200 + Idempotent-Replay = kunci sama.
func (h *Handler) Pay(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		httpx.Error(w, http.StatusNotFound, "NOT_FOUND", "Data tidak ditemukan.")
		return
	}
	var req PayInput
	if !httpx.DecodeJSONLimit(w, r, &req, 16<<10) {
		return
	}
	d, replayed, err := h.svc.Pay(r.Context(), actor(r), id, r.Header.Get("Idempotency-Key"), req)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	status := http.StatusCreated
	if replayed {
		w.Header().Set("Idempotent-Replay", "true")
		status = http.StatusOK
	}
	httpx.JSON(w, status, d)
}

// SettleQuote: POST /payables/settlements/quote — pratinjau pembagian uang.
func (h *Handler) SettleQuote(w http.ResponseWriter, r *http.Request) {
	var req SettleInput
	if !httpx.DecodeJSONLimit(w, r, &req, 64<<10) {
		return
	}
	p, err := h.svc.SettleQuote(r.Context(), actor(r), req)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, p)
}

// Settle: POST /payables/settlements (header Idempotency-Key wajib). 201 = baru; 200 + Idempotent-Replay = kunci sama.
func (h *Handler) Settle(w http.ResponseWriter, r *http.Request) {
	var req SettleInput
	if !httpx.DecodeJSONLimit(w, r, &req, 64<<10) {
		return
	}
	d, replayed, err := h.svc.Settle(r.Context(), actor(r), r.Header.Get("Idempotency-Key"), req)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	status := http.StatusCreated
	if replayed {
		w.Header().Set("Idempotent-Replay", "true")
		status = http.StatusOK
	}
	httpx.JSON(w, status, d)
}

func (h *Handler) GetSettlement(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		httpx.Error(w, http.StatusNotFound, "NOT_FOUND", "Data tidak ditemukan.")
		return
	}
	d, err := h.svc.GetSettlement(r.Context(), actor(r), id)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, d)
}

func (h *Handler) fail(w http.ResponseWriter, r *http.Request, err error) {
	var fields FieldErrors
	switch {
	case errors.As(err, &fields):
		httpx.ValidationError(w, fields)
	case errors.Is(err, ErrKeyRequired):
		httpx.Error(w, http.StatusBadRequest, "IDEMPOTENCY_KEY_REQUIRED", "Header Idempotency-Key wajib (8–100 karakter).")
	case errors.Is(err, ErrKeyMismatch):
		httpx.Error(w, http.StatusUnprocessableEntity, "IDEMPOTENCY_MISMATCH", "Idempotency-Key sudah dipakai untuk permintaan yang berbeda.")
	case errors.Is(err, ErrOutletGone):
		httpx.Error(w, http.StatusConflict, "OUTLET_NOT_FOUND", "Outlet aktif tidak ditemukan atau tidak aktif.")
	case errors.Is(err, ErrNotFound):
		httpx.Error(w, http.StatusNotFound, "NOT_FOUND", "Data tidak ditemukan.")
	default:
		h.log.ErrorContext(r.Context(), "payable", "err", err, "path", r.URL.Path)
		httpx.Error(w, http.StatusInternalServerError, "INTERNAL", "Terjadi kesalahan pada server.")
	}
}
