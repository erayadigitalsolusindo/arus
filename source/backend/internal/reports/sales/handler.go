package sales

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	"aciraba/internal/authz"
	"aciraba/internal/platform/httpx"
)

const (
	ModuleReports     = "sales_reports"
	ModuleReportsCost = "sales_reports_cost"
)

type Handler struct {
	svc *Service
}

func NewHandler(svc *Service) *Handler {
	return &Handler{svc: svc}
}

// Routes: GET /reports/sales/by-date
func (h *Handler) Routes(r chi.Router) {
	r.Get("/reports/sales/by-date", h.ByDate)
}

// ByDate: GET /reports/sales/by-date?from=&to=&outlet_id=&cashier_id=&all_outlets=
func (h *Handler) ByDate(w http.ResponseWriter, r *http.Request) {
	qs := r.URL.Query()
	p := ByDateParams{
		AllOutlets: qs.Get("all_outlets") == "true",
		OutletID:   qs.Get("outlet_id"),
		From:       qs.Get("from"),
		To:         qs.Get("to"),
		CashierID:  qs.Get("cashier_id"),
	}
	result, err := h.svc.ByDate(r.Context(), actor(r), p)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, result)
}

func (h *Handler) fail(w http.ResponseWriter, r *http.Request, err error) {
	if fe, ok := err.(FieldErrors); ok {
		httpx.Error(w, http.StatusBadRequest, "VALIDATION_ERROR", fe.Error())
		return
	}
	httpx.Error(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
}

// actor extracts authz.Actor from request context. Same pattern as sales/handler.go.
func actor(r *http.Request) authz.Actor {
	a, _ := authz.ActorFrom(r.Context())
	return a
}
