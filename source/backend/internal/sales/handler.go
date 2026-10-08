package sales

import (
	"errors"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"aciraba/internal/approval"
	"aciraba/internal/authz"
	pauth "aciraba/internal/platform/auth"
	"aciraba/internal/platform/httpx"
	"aciraba/internal/stock"
)

// Module = modul izin penjualan (authz.Modules): view = lihat nota, create = simpan nota.
const Module = "sales_orders"

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
	r.Route("/sales", func(r chi.Router) {
		r.Use(httpx.RequireAuth(h.tokens), h.resolver.Authenticate)
		r.With(authz.Require(Module, authz.ActCreate)).Post("/", h.Create)
		r.With(authz.Require(Module, authz.ActCreate)).Post("/quote", h.Quote)
		r.With(authz.Require(Module, authz.ActView)).Get("/", h.List)
		r.With(authz.Require(ModuleList, authz.ActView)).Get("/all", h.ListAll)
		r.With(authz.Require(ModuleList, authz.ActView)).Get("/{id}/detail", h.Detail)
		r.With(authz.Require(Module, authz.ActView)).Get("/{id}", h.Get)
		r.With(authz.Require(Module, authz.ActUpdate)).Put("/{id}", h.Edit)
		r.With(authz.Require(Module, authz.ActUpdate)).Post("/{id}/quote", h.QuoteEdit)
		r.With(authz.Require(Module, authz.ActDelete)).Post("/{id}/void", h.Void)
	})
}

func actor(r *http.Request) authz.Actor {
	a, _ := authz.ActorFrom(r.Context())
	return a
}

// Create: header `Idempotency-Key` wajib (UUID/acak 8–100 karakter dibuat klien per percobaan bayar).
// 201 = nota baru; 200 + `Idempotent-Replay: true` = kunci yang sama sudah pernah menyimpan (nota yang sama dikembalikan).
func (h *Handler) Create(w http.ResponseWriter, r *http.Request) {
	var req Request
	if !httpx.DecodeJSONLimit(w, r, &req, 256<<10) {
		return
	}
	sale, replayed, err := h.svc.Create(r.Context(), actor(r), r.Header.Get("Idempotency-Key"), req)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	status := http.StatusCreated
	if replayed {
		w.Header().Set("Idempotent-Replay", "true")
		status = http.StatusOK
	}
	httpx.JSON(w, status, sale)
}

// Quote: hitung harga/total di server tanpa menyimpan (pratinjau kasir).
func (h *Handler) Quote(w http.ResponseWriter, r *http.Request) {
	var req Request
	if !httpx.DecodeJSONLimit(w, r, &req, 256<<10) {
		return
	}
	q, err := h.svc.Quote(r.Context(), actor(r), req)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, q)
}

func (h *Handler) Get(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		httpx.Error(w, http.StatusNotFound, "NOT_FOUND", "Data tidak ditemukan.")
		return
	}
	sale, err := h.svc.Get(r.Context(), actor(r), id)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, sale)
}

func (h *Handler) fail(w http.ResponseWriter, r *http.Request, err error) {
	var fields FieldErrors
	var se *StockError
	var cl *CreditLimitError
	if approval.Fail(w, err) {
		return
	}
	switch {
	case errors.As(err, &fields):
		httpx.ValidationError(w, fields)
	case errors.As(err, &se):
		httpx.Error(w, http.StatusConflict, "STOCK_INSUFFICIENT", "Stok "+se.Name+" tidak mencukupi.")
	case errors.Is(err, stock.ErrNotStocked):
		httpx.Error(w, http.StatusUnprocessableEntity, "NOT_STOCKED", "Barang ini tidak memiliki stok.")
	case errors.Is(err, ErrKeyRequired):
		httpx.Error(w, http.StatusBadRequest, "IDEMPOTENCY_KEY_REQUIRED", "Header Idempotency-Key wajib (8–100 karakter).")
	case errors.Is(err, ErrKeyMismatch):
		httpx.Error(w, http.StatusUnprocessableEntity, "IDEMPOTENCY_MISMATCH", "Idempotency-Key sudah dipakai untuk permintaan yang berbeda.")
	case errors.Is(err, ErrOutletInactive):
		httpx.Error(w, http.StatusConflict, "OUTLET_NOT_FOUND", "Outlet aktif tidak ditemukan atau tidak aktif.")
	case errors.As(err, &cl):
		httpx.Error(w, http.StatusForbidden, "CREDIT_LIMIT_EXCEEDED", "Piutang member melewati limit kredit. Butuh persetujuan Owner/Supervisor (PIN).")
	case errors.Is(err, ErrReceivablePaid):
		httpx.Error(w, http.StatusConflict, "RECEIVABLE_PAID", "Piutang nota ini sudah dibayar sebagian atau seluruhnya, sehingga nota tidak dapat diubah atau dibatalkan.")
	case errors.Is(err, ErrNotEditable):
		httpx.Error(w, http.StatusConflict, "SALE_NOT_EDITABLE", "Nota ini tidak dapat diubah lagi (sudah dibatalkan atau digantikan revisi).")
	case errors.Is(err, ErrEditWindow):
		httpx.Error(w, http.StatusForbidden, "EDIT_WINDOW_CLOSED", "Batas waktu edit nota sudah lewat.")
	case errors.Is(err, ErrOutletMismatch):
		httpx.Error(w, http.StatusConflict, "OUTLET_MISMATCH", "Nota ini milik outlet lain; pindah ke outlet nota itu dulu.")
	case errors.Is(err, ErrNotFound):
		httpx.Error(w, http.StatusNotFound, "NOT_FOUND", "Data tidak ditemukan.")
	default:
		h.log.ErrorContext(r.Context(), "sales", "err", err, "path", r.URL.Path)
		httpx.Error(w, http.StatusInternalServerError, "INTERNAL", "Terjadi kesalahan pada server.")
	}
}
