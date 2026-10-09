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
		r.With(authz.Require(ModuleInvoices, authz.ActUpdate)).Put("/{id}", h.Edit)
		r.With(authz.Require(ModuleInvoices, authz.ActDelete)).Post("/{id}/void", h.Void)
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

// editRequest = isi pembelian revisi + alasan perubahan.
type editRequest struct {
	Request
	Reason string `json:"reason"`
}

// Edit: PUT /purchases/{id} (header Idempotency-Key wajib). 201 = revisi baru; 200 + Idempotent-Replay = kunci sama.
func (h *Handler) Edit(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		httpx.Error(w, http.StatusNotFound, "NOT_FOUND", "Data tidak ditemukan.")
		return
	}
	var req editRequest
	if !httpx.DecodeJSONLimit(w, r, &req, 256<<10) {
		return
	}
	p, replayed, err := h.svc.Edit(r.Context(), actor(r), id, r.Header.Get("Idempotency-Key"), req.Request, req.Reason)
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

// Void: POST /purchases/{id}/void {reason}.
func (h *Handler) Void(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		httpx.Error(w, http.StatusNotFound, "NOT_FOUND", "Data tidak ditemukan.")
		return
	}
	var req struct {
		Reason string `json:"reason"`
	}
	if !httpx.DecodeJSONLimit(w, r, &req, 16<<10) {
		return
	}
	p, err := h.svc.Void(r.Context(), actor(r), id, req.Reason)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, p)
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

// List: GET /purchases?from=&to=&supplier_id=&payment_type=&q=&limit=&cursor=
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
	p.Cursor = qs.Get("cursor")
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
	case errors.Is(err, ErrNotEditable):
		httpx.Error(w, http.StatusConflict, "PURCHASE_NOT_EDITABLE", "Nota ini tidak dapat diubah lagi (sudah dibatalkan atau digantikan revisi).")
	case errors.Is(err, ErrEditWindowClosed):
		httpx.Error(w, http.StatusForbidden, "EDIT_WINDOW_CLOSED", "Batas waktu edit nota sudah lewat.")
	case errors.Is(err, ErrOutletMismatch):
		httpx.Error(w, http.StatusConflict, "OUTLET_MISMATCH", "Nota ini milik outlet lain; pindah ke outlet nota itu dulu.")
	case errors.Is(err, stock.ErrInsufficient):
		httpx.Error(w, http.StatusConflict, "STOCK_INSUFFICIENT", "Stok barang tidak mencukupi untuk membalik nota ini (sudah terjual sebagian).")
	case errors.Is(err, stock.ErrItemNotFound), errors.Is(err, stock.ErrNotStocked):
		httpx.Error(w, http.StatusUnprocessableEntity, "ITEM_UNAVAILABLE", "Barang tidak tersedia untuk pembelian.")
	default:
		h.log.ErrorContext(r.Context(), "purchasing", "err", err, "path", r.URL.Path)
		httpx.Error(w, http.StatusInternalServerError, "INTERNAL", "Terjadi kesalahan pada server.")
	}
}
