package stock

import (
	"encoding/json"
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

// OpeningModule = modul izin Saldo Awal Stok (authz.Modules): view = lihat, create = isi/ubah, approve = kunci.
const OpeningModule = "stock_opening"

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
	r.Route("/stock/opening", func(r chi.Router) {
		r.Use(httpx.RequireAuth(h.tokens), h.resolver.Authenticate)
		r.With(authz.Require(OpeningModule, authz.ActView)).Get("/status", h.Status)
		r.With(authz.Require(OpeningModule, authz.ActView)).Get("/items", h.List)
		r.With(authz.Require(OpeningModule, authz.ActCreate)).Put("/items/{id}", h.Set)
		r.With(authz.Require(OpeningModule, authz.ActApprove)).Post("/lock", h.Lock)
	})
}

func actor(r *http.Request) authz.Actor {
	a, _ := authz.ActorFrom(r.Context())
	return a
}

func (h *Handler) Status(w http.ResponseWriter, r *http.Request) {
	st, err := h.svc.Status(r.Context(), actor(r))
	if err != nil {
		h.fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, st)
}

// List: ?q=&limit=&offset=
func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	qs := r.URL.Query()
	p := ListParams{Q: qs.Get("q")}
	p.Limit, _ = strconv.Atoi(qs.Get("limit"))
	p.Offset, _ = strconv.Atoi(qs.Get("offset"))
	rows, total, err := h.svc.List(r.Context(), actor(r), p)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"data": rows, "total": total})
}

func (h *Handler) Set(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		httpx.Error(w, http.StatusNotFound, "NOT_FOUND", "Data tidak ditemukan.")
		return
	}
	var req struct {
		Bucket string      `json:"bucket"`
		Qty    json.Number `json:"qty"`
	}
	if !httpx.DecodeJSON(w, r, &req) {
		return
	}
	bal, err := h.svc.SetOpening(r.Context(), actor(r), id, req.Bucket, req.Qty.String())
	if err != nil {
		h.fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"bucket": req.Bucket, "qty": bal.String()})
}

func (h *Handler) Lock(w http.ResponseWriter, r *http.Request) {
	var req struct {
		StartDate string `json:"start_date"`
	}
	if !httpx.DecodeJSON(w, r, &req) {
		return
	}
	st, err := h.svc.Lock(r.Context(), actor(r), req.StartDate)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, st)
}

func (h *Handler) fail(w http.ResponseWriter, r *http.Request, err error) {
	var fields FieldErrors
	switch {
	case errors.As(err, &fields):
		httpx.ValidationError(w, fields)
	case errors.Is(err, ErrNotFound), errors.Is(err, ErrItemNotFound):
		httpx.Error(w, http.StatusNotFound, "NOT_FOUND", "Data tidak ditemukan.")
	case errors.Is(err, ErrNotStocked):
		httpx.Error(w, http.StatusUnprocessableEntity, "NOT_STOCKED", "Barang ini tidak memiliki stok.")
	case errors.Is(err, ErrOpeningLocked):
		httpx.Error(w, http.StatusConflict, "OPENING_LOCKED", "Saldo awal sudah dikunci; gunakan stok opname.")
	case errors.Is(err, ErrAlreadyLocked):
		httpx.Error(w, http.StatusConflict, "ALREADY_LOCKED", "Tanggal mulai operasional sudah dikunci.")
	default:
		h.log.ErrorContext(r.Context(), "stock opening", "err", err, "path", r.URL.Path)
		httpx.Error(w, http.StatusInternalServerError, "INTERNAL", "Terjadi kesalahan pada server.")
	}
}
