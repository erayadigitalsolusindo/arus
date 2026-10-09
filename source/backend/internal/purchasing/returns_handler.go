package purchasing

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"aciraba/internal/authz"
	"aciraba/internal/platform/httpx"
	"aciraba/internal/stock"
)

// ReturnRoutes: /purchase-returns (modul izin purchase_returns: view = lihat, create = buat, approve = batalkan).
func (h *Handler) ReturnRoutes(r chi.Router) {
	r.Route("/purchase-returns", func(r chi.Router) {
		r.Use(httpx.RequireAuth(h.tokens), h.resolver.Authenticate)
		view := authz.Require(ModuleReturns, authz.ActView)
		create := authz.Require(ModuleReturns, authz.ActCreate)
		r.With(view).Get("/", h.ListReturns)
		r.With(create).Post("/", h.CreateReturn)
		r.With(create).Post("/quote", h.QuoteReturn)
		r.With(create).Get("/purchases", h.ReturnablePurchases)
		r.With(create).Get("/source/{id}", h.ReturnSource)
		r.With(view).Get("/{id}", h.GetReturn)
		r.With(authz.Require(ModuleReturns, authz.ActApprove)).Post("/{id}/void", h.VoidReturn)
	})
}

func pathID(w http.ResponseWriter, r *http.Request) (uuid.UUID, bool) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		httpx.Error(w, http.StatusNotFound, "NOT_FOUND", "Data tidak ditemukan.")
		return uuid.Nil, false
	}
	return id, true
}

// CreateReturn: POST /purchase-returns (header Idempotency-Key wajib). 201 = baru; 200 + Idempotent-Replay = kunci sama.
func (h *Handler) CreateReturn(w http.ResponseWriter, r *http.Request) {
	var req ReturnRequest
	if !httpx.DecodeJSONLimit(w, r, &req, 128<<10) {
		return
	}
	ret, replayed, err := h.svc.CreateReturn(r.Context(), actor(r), r.Header.Get("Idempotency-Key"), req)
	if err != nil {
		h.failReturn(w, r, err)
		return
	}
	status := http.StatusCreated
	if replayed {
		w.Header().Set("Idempotent-Replay", "true")
		status = http.StatusOK
	}
	httpx.JSON(w, status, ret)
}

func (h *Handler) QuoteReturn(w http.ResponseWriter, r *http.Request) {
	var req ReturnRequest
	if !httpx.DecodeJSONLimit(w, r, &req, 128<<10) {
		return
	}
	q, err := h.svc.QuoteReturn(r.Context(), actor(r), req)
	if err != nil {
		h.failReturn(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, q)
}

func (h *Handler) ReturnSource(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	src, err := h.svc.ReturnSource(r.Context(), actor(r), id)
	if err != nil {
		h.failReturn(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, src)
}

func (h *Handler) ReturnablePurchases(w http.ResponseWriter, r *http.Request) {
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	if len([]rune(q)) > 100 {
		q = string([]rune(q)[:100])
	}
	list, err := h.svc.ReturnablePurchases(r.Context(), actor(r), q)
	if err != nil {
		h.failReturn(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"data": list})
}

func (h *Handler) GetReturn(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	ret, err := h.svc.GetReturn(r.Context(), actor(r), id)
	if err != nil {
		h.failReturn(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, ret)
}

// ListReturns: GET /purchase-returns?from=&to=&status=&q=&limit=&cursor=
func (h *Handler) ListReturns(w http.ResponseWriter, r *http.Request) {
	qs := r.URL.Query()
	p := ReturnListParams{From: qs.Get("from"), To: qs.Get("to"), Status: qs.Get("status"), Q: strings.TrimSpace(qs.Get("q")), Cursor: qs.Get("cursor")}
	if len([]rune(p.Q)) > 100 {
		p.Q = string([]rune(p.Q)[:100])
	}
	p.Limit, _ = strconv.Atoi(qs.Get("limit"))
	res, err := h.svc.ListReturns(r.Context(), actor(r), p)
	if err != nil {
		h.failReturn(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, res)
}

// VoidReturn: POST /purchase-returns/{id}/void {reason}.
func (h *Handler) VoidReturn(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	var req struct {
		Reason string `json:"reason"`
	}
	if !httpx.DecodeJSONLimit(w, r, &req, 16<<10) {
		return
	}
	ret, err := h.svc.VoidReturn(r.Context(), actor(r), id, req.Reason)
	if err != nil {
		h.failReturn(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, ret)
}

func (h *Handler) failReturn(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, ErrNotReturnable):
		httpx.Error(w, http.StatusConflict, "PURCHASE_NOT_RETURNABLE", "Nota ini tidak dapat diretur (sudah dibatalkan atau digantikan revisi).")
	case errors.Is(err, ErrReturnNotActive):
		httpx.Error(w, http.StatusConflict, "RETURN_NOT_ACTIVE", "Retur ini sudah dibatalkan.")
	case errors.Is(err, ErrReturnLocked):
		httpx.Error(w, http.StatusConflict, "RETURN_LOCKED", "Hutang nota sudah dibayar setelah retur dibuat; retur tidak dapat dibatalkan.")
	case errors.Is(err, stock.ErrInsufficient):
		httpx.Error(w, http.StatusConflict, "RETURN_STOCK_INSUFFICIENT", "Stok di bucket Retur tidak cukup. Mutasi barang ke bucket Retur dulu.")
	default:
		h.fail(w, r, err)
	}
}
