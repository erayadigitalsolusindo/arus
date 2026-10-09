package paymentmethod

import (
	"errors"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
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
	r.Route("/payment-methods", func(r chi.Router) {
		r.Use(httpx.RequireAuth(h.tokens), h.resolver.Authenticate)
		// Kasir hanya butuh daftar aktif (id, nama, jenis); tak perlu izin halaman master.
		r.With(authz.RequireAny([2]string{ModuleID, authz.ActView}, [2]string{"sales_orders", authz.ActCreate}, [2]string{"member_receivables", authz.ActCreate}, [2]string{"supplier_payables", authz.ActCreate}, [2]string{"purchase_returns", authz.ActCreate})).Get("/lookup", h.lookup)
		r.With(authz.Require(ModuleID, authz.ActView)).Get("/", h.list)
		r.With(authz.Require(ModuleID, authz.ActCreate)).Post("/", h.create)
		r.With(authz.Require(ModuleID, authz.ActUpdate)).Put("/{id}", h.update)
		r.With(authz.Require(ModuleID, authz.ActUpdate)).Put("/{id}/active", h.active)
	})
}

func actor(r *http.Request) authz.Actor {
	a, _ := authz.ActorFrom(r.Context())
	return a
}

func pathID(w http.ResponseWriter, r *http.Request) (uuid.UUID, bool) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		httpx.Error(w, http.StatusNotFound, "NOT_FOUND", "Data tidak ditemukan.")
		return uuid.Nil, false
	}
	return id, true
}

func (h *Handler) fail(w http.ResponseWriter, r *http.Request, err error) {
	var fields FieldErrors
	switch {
	case errors.As(err, &fields):
		httpx.ValidationError(w, fields)
	case errors.Is(err, ErrNotFound):
		httpx.Error(w, http.StatusNotFound, "NOT_FOUND", "Data tidak ditemukan.")
	case errors.Is(err, ErrNameTaken):
		httpx.Error(w, http.StatusConflict, "NAME_TAKEN", "Nama sudah dipakai.")
	case errors.Is(err, ErrLocked):
		httpx.Error(w, http.StatusConflict, "METHOD_LOCKED", "Metode bawaan tidak dapat diarsipkan.")
	case errors.Is(err, ErrTooMany):
		httpx.Error(w, http.StatusConflict, "METHOD_LIMIT", "Jumlah metode pembayaran sudah mencapai batas.")
	default:
		h.log.Error("metode pembayaran gagal", "err", err, "req_id", middleware.GetReqID(r.Context()))
		httpx.Error(w, http.StatusInternalServerError, "INTERNAL", "Terjadi kesalahan pada server.")
	}
}

func (h *Handler) list(w http.ResponseWriter, r *http.Request) {
	qs := r.URL.Query()
	p := ListParams{Q: qs.Get("q")}
	p.Limit, _ = strconv.Atoi(qs.Get("limit"))
	p.Offset, _ = strconv.Atoi(qs.Get("offset"))
	switch qs.Get("active") {
	case "true":
		t := true
		p.Active = &t
	case "false":
		f := false
		p.Active = &f
	}
	list, total, err := h.svc.List(r.Context(), actor(r), p)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"data": list, "total": total})
}

func (h *Handler) lookup(w http.ResponseWriter, r *http.Request) {
	list, err := h.svc.Lookup(r.Context(), actor(r))
	if err != nil {
		h.fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"data": list})
}

func (h *Handler) create(w http.ResponseWriter, r *http.Request) {
	var in Input
	if !httpx.DecodeJSON(w, r, &in) {
		return
	}
	m, err := h.svc.Create(r.Context(), actor(r), in)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusCreated, m)
}

func (h *Handler) update(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	var in Input
	if !httpx.DecodeJSON(w, r, &in) {
		return
	}
	m, err := h.svc.Update(r.Context(), actor(r), id, in)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, m)
}

func (h *Handler) active(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	var req struct {
		Active *bool `json:"active"`
	}
	if !httpx.DecodeJSON(w, r, &req) {
		return
	}
	if req.Active == nil {
		httpx.ValidationError(w, map[string]string{"active": "REQUIRED"})
		return
	}
	m, err := h.svc.SetActive(r.Context(), actor(r), id, *req.Active)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, m)
}
