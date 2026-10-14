package sales

import (
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"

	"aciraba/internal/authz"
	pauth "aciraba/internal/platform/auth"
	"aciraba/internal/platform/httpx"
)

const (
	ModuleReports     = "sales_reports"
	ModuleReportsCost = "sales_reports_cost"
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

// Routes: GET /reports/sales/by-date (butuh login + izin sales_reports.view; tenant/outlet dari token).
func (h *Handler) Routes(r chi.Router) {
	r.Route("/reports/sales", func(r chi.Router) {
		r.Use(httpx.RequireAuth(h.tokens), h.resolver.Authenticate)
		r.With(authz.Require(ModuleReports, authz.ActView)).Get("/by-date", h.ByDate)
	})
}

// ByDate: GET /reports/sales/by-date?from=&to=&outlet_id=&cashier_id=&all_outlets=&hide_empty=
func (h *Handler) ByDate(w http.ResponseWriter, r *http.Request) {
	qs := r.URL.Query()
	p := ByDateParams{
		AllOutlets: qs.Get("all_outlets") == "true",
		OutletID:   qs.Get("outlet_id"),
		From:       qs.Get("from"),
		To:         qs.Get("to"),
		CashierID:  qs.Get("cashier_id"),
		HideEmpty:  qs.Get("hide_empty") == "true",
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
