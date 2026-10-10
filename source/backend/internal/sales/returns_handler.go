package sales

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"aciraba/internal/authz"
	"aciraba/internal/platform/httpx"
)

// ReturnRoutes mounts the sales-return workflow under the dedicated sales_returns permission.
func (h *Handler) ReturnRoutes(r chi.Router) {
	r.Route("/sales-returns", func(r chi.Router) {
		r.Use(httpx.RequireAuth(h.tokens), h.resolver.Authenticate)
		view := authz.Require(ModuleReturns, authz.ActView)
		create := authz.Require(ModuleReturns, authz.ActCreate)
		r.With(view).Get("/", h.ListSaleReturns)
		r.With(create).Get("/sales", h.SaleReturnChoices)
		r.With(create).Get("/source/{id}", h.SaleReturnSource)
		r.With(create).Post("/quote", h.QuoteSaleReturn)
		r.With(create).Post("/", h.CreateSaleReturn)
		r.With(view).Get("/{id}", h.GetSaleReturn)
		r.With(authz.Require(ModuleReturns, authz.ActApprove)).Post("/{id}/void", h.VoidSaleReturn)
	})
}

func saleReturnPathID(w http.ResponseWriter, r *http.Request) (uuid.UUID, bool) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		httpx.Error(w, http.StatusNotFound, "NOT_FOUND", "Data tidak ditemukan.")
		return uuid.Nil, false
	}
	return id, true
}

func (h *Handler) CreateSaleReturn(w http.ResponseWriter, r *http.Request) {
	var req SaleReturnRequest
	if !httpx.DecodeJSONLimit(w, r, &req, 128<<10) {
		return
	}
	ret, replayed, err := h.svc.CreateSaleReturn(r.Context(), actor(r), r.Header.Get("Idempotency-Key"), req)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	status := http.StatusCreated
	if replayed {
		status = http.StatusOK
		w.Header().Set("Idempotent-Replay", "true")
	}
	httpx.JSON(w, status, ret)
}

func (h *Handler) QuoteSaleReturn(w http.ResponseWriter, r *http.Request) {
	var req SaleReturnRequest
	if !httpx.DecodeJSONLimit(w, r, &req, 128<<10) {
		return
	}
	quote, err := h.svc.QuoteSaleReturn(r.Context(), actor(r), req)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, quote)
}

func (h *Handler) SaleReturnSource(w http.ResponseWriter, r *http.Request) {
	id, ok := saleReturnPathID(w, r)
	if !ok {
		return
	}
	source, err := h.svc.SaleReturnSource(r.Context(), actor(r), id)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, source)
}

func (h *Handler) SaleReturnChoices(w http.ResponseWriter, r *http.Request) {
	choices, err := h.svc.SalesReturnChoices(r.Context(), actor(r), strings.TrimSpace(r.URL.Query().Get("q")))
	if err != nil {
		h.fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"data": choices})
}

func (h *Handler) GetSaleReturn(w http.ResponseWriter, r *http.Request) {
	id, ok := saleReturnPathID(w, r)
	if !ok {
		return
	}
	ret, err := h.svc.GetSaleReturn(r.Context(), actor(r), id)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, ret)
}

func (h *Handler) ListSaleReturns(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()
	limit, _ := strconv.Atoi(query.Get("limit"))
	result, err := h.svc.ListSaleReturns(r.Context(), actor(r), SaleReturnListParams{
		From: query.Get("from"), To: query.Get("to"), Query: strings.TrimSpace(query.Get("q")), Cursor: query.Get("cursor"), Limit: limit,
		Status: query.Get("status"),
	})
	if err != nil {
		h.fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, result)
}

// VoidSaleReturn: POST /sales-returns/{id}/void {reason} (izin sales_returns.approve).
func (h *Handler) VoidSaleReturn(w http.ResponseWriter, r *http.Request) {
	id, ok := saleReturnPathID(w, r)
	if !ok {
		return
	}
	var req struct {
		Reason string `json:"reason"`
	}
	if !httpx.DecodeJSONLimit(w, r, &req, 16<<10) {
		return
	}
	ret, err := h.svc.VoidSaleReturn(r.Context(), actor(r), id, req.Reason)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, ret)
}
