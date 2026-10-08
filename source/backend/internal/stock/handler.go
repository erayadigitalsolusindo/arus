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

// ConversionModule = modul izin Pecah Satuan: view = lihat riwayat, create = membuat dokumen.
const ConversionModule = "stock_conversion"

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
	r.Route("/stock/card", func(r chi.Router) {
		r.Use(httpx.RequireAuth(h.tokens), h.resolver.Authenticate, authz.Require(CardModule, authz.ActView))
		r.Get("/", h.Card)
		r.Get("/items", h.CardItems)
	})
	r.Route("/stock/conversions", func(r chi.Router) {
		r.Use(httpx.RequireAuth(h.tokens), h.resolver.Authenticate)
		r.With(authz.Require(ConversionModule, authz.ActView)).Get("/", h.ListConversions)
		r.With(authz.Require(ConversionModule, authz.ActView)).Get("/items", h.List)
		r.With(authz.Require(ConversionModule, authz.ActCreate)).Post("/", h.Convert)
		r.With(authz.Require(ConversionModule, authz.ActView)).Get("/{id}", h.GetConversion)
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
	case errors.Is(err, ErrInsufficient):
		httpx.Error(w, http.StatusConflict, "STOCK_INSUFFICIENT", "Stok barang asal tidak mencukupi.")
	case errors.Is(err, ErrKeyRequired):
		httpx.Error(w, http.StatusBadRequest, "IDEMPOTENCY_KEY_REQUIRED", "Header Idempotency-Key wajib (8–100 karakter).")
	case errors.Is(err, ErrKeyMismatch):
		httpx.Error(w, http.StatusUnprocessableEntity, "IDEMPOTENCY_MISMATCH", "Idempotency-Key sudah dipakai untuk permintaan yang berbeda.")
	case errors.Is(err, ErrOutletInactive):
		httpx.Error(w, http.StatusConflict, "OUTLET_NOT_FOUND", "Outlet aktif tidak ditemukan atau tidak aktif.")
	case errors.Is(err, ErrOpeningLocked):
		httpx.Error(w, http.StatusConflict, "OPENING_LOCKED", "Saldo awal sudah dikunci; gunakan stok opname.")
	case errors.Is(err, ErrAlreadyLocked):
		httpx.Error(w, http.StatusConflict, "ALREADY_LOCKED", "Tanggal mulai operasional sudah dikunci.")
	default:
		h.log.ErrorContext(r.Context(), "stock opening", "err", err, "path", r.URL.Path)
		httpx.Error(w, http.StatusInternalServerError, "INTERNAL", "Terjadi kesalahan pada server.")
	}
}

// Convert: POST /stock/conversions — header `Idempotency-Key` wajib. 201 = dokumen baru; 200 + `Idempotent-Replay: true` = kunci sama.
func (h *Handler) Convert(w http.ResponseWriter, r *http.Request) {
	var req ConvertInput
	if !httpx.DecodeJSON(w, r, &req) {
		return
	}
	conv, replayed, err := h.svc.Convert(r.Context(), actor(r), r.Header.Get("Idempotency-Key"), req)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	status := http.StatusCreated
	if replayed {
		w.Header().Set("Idempotent-Replay", "true")
		status = http.StatusOK
	}
	httpx.JSON(w, status, conv)
}

// ListConversions: ?limit=&offset=
func (h *Handler) ListConversions(w http.ResponseWriter, r *http.Request) {
	qs := r.URL.Query()
	limit, _ := strconv.Atoi(qs.Get("limit"))
	offset, _ := strconv.Atoi(qs.Get("offset"))
	rows, total, err := h.svc.ListConversions(r.Context(), actor(r), limit, offset)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"data": rows, "total": total})
}

func (h *Handler) GetConversion(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		httpx.Error(w, http.StatusNotFound, "NOT_FOUND", "Data tidak ditemukan.")
		return
	}
	conv, err := h.svc.GetConversion(r.Context(), actor(r), id)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, conv)
}
