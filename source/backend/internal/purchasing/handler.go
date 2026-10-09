package purchasing

import (
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"aciraba/internal/authz"
	pauth "aciraba/internal/platform/auth"
	"aciraba/internal/platform/httpx"
	"aciraba/internal/stock"
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
	r.Route("/purchases", func(r chi.Router) {
		r.Use(httpx.RequireAuth(h.tokens), h.resolver.Authenticate)
		view := authz.RequireAny([2]string{ModuleList, authz.ActView}, [2]string{ModuleInvoices, authz.ActView})
		create := authz.Require(ModuleInvoices, authz.ActCreate)
		r.With(view).Get("/", h.List)
		r.With(create).Post("/", h.Create)
		r.With(create).Post("/quote", h.Quote)
		r.With(create).Get("/items", h.Items)
		r.With(view).Get("/{id}", h.Get)
	})
}

func actor(r *http.Request) authz.Actor {
	a, _ := authz.ActorFrom(r.Context())
	return a
}

// Create: POST /purchases (header Idempotency-Key wajib). 201 = nota baru; 200 + Idempotent-Replay = kunci sama.
func (h *Handler) Create(w http.ResponseWriter, r *http.Request) {
	var req Request
	if !httpx.DecodeJSONLimit(w, r, &req, 256<<10) {
		return
	}
	p, replayed, err := h.svc.Create(r.Context(), actor(r), r.Header.Get("Idempotency-Key"), req)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	status := http.StatusCreated
	if replayed {
		w.Header().Set("Idempotent-Replay", "true")
		status = http.StatusOK
	}
	httpx.JSON(w, status, p)
}

// Quote: POST /purchases/quote — pratinjau total, HPP baris dan HPP rata-rata baru; tidak menyimpan apa pun.
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
	p, err := h.svc.Get(r.Context(), actor(r), id)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, p)
}

// List: GET /purchases?from=&to=&supplier_id=&payment_type=&q=&limit=&offset=
func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	qs := r.URL.Query()
	p := ListParams{From: qs.Get("from"), To: qs.Get("to"), PaymentType: qs.Get("payment_type"), Q: strings.TrimSpace(qs.Get("q"))}
	if s := qs.Get("supplier_id"); s != "" {
		id, err := uuid.Parse(s)
		if err != nil {
			httpx.ValidationError(w, map[string]string{"supplier_id": "INVALID"})
			return
		}
		p.SupplierID = &id
	}
	p.Limit, _ = strconv.Atoi(qs.Get("limit"))
	p.Offset, _ = strconv.Atoi(qs.Get("offset"))
	res, err := h.svc.List(r.Context(), actor(r), p)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, res)
}

// Items: GET /purchases/items?q= — pemilih barang untuk form pembelian.
func (h *Handler) Items(w http.ResponseWriter, r *http.Request) {
	items, err := h.svc.Items(r.Context(), actor(r), strings.TrimSpace(r.URL.Query().Get("q")))
	if err != nil {
		h.fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"data": items})
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
	case errors.Is(err, ErrOutletInactive):
		httpx.Error(w, http.StatusConflict, "OUTLET_NOT_FOUND", "Outlet aktif tidak ditemukan atau tidak aktif.")
	case errors.Is(err, ErrOutletForbidden):
		httpx.Error(w, http.StatusForbidden, "OUTLET_FORBIDDEN", "Anda tidak memiliki akses ke outlet ini.")
	case errors.Is(err, ErrNotFound):
		httpx.Error(w, http.StatusNotFound, "NOT_FOUND", "Data tidak ditemukan.")
	case errors.Is(err, stock.ErrItemNotFound), errors.Is(err, stock.ErrNotStocked):
		httpx.Error(w, http.StatusUnprocessableEntity, "ITEM_UNAVAILABLE", "Barang tidak tersedia untuk pembelian.")
	default:
		h.log.ErrorContext(r.Context(), "purchasing", "err", err, "path", r.URL.Path)
		httpx.Error(w, http.StatusInternalServerError, "INTERNAL", "Terjadi kesalahan pada server.")
	}
}
