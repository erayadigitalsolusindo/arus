package stock

import (
	"errors"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"aciraba/internal/authz"
	pauth "aciraba/internal/platform/auth"
	"aciraba/internal/platform/httpx"
)

// Handler HTTP untuk laporan persedian.
type Handler struct {
	svc      *Service
	resolver *authz.Resolver
	tokens   *pauth.TokenIssuer
	log      *slog.Logger
}

func NewHandler(svc *Service, resolver *authz.Resolver, tokens *pauth.TokenIssuer, log *slog.Logger) *Handler {
	return &Handler{svc: svc, resolver: resolver, tokens: tokens, log: log}
}

// Routes: GET /reports/stock/valuation.
func (h *Handler) Routes(r chi.Router) {
	r.Route("/reports/stock", func(r chi.Router) {
		r.Use(httpx.RequireAuth(h.tokens), h.resolver.Authenticate)
		r.With(authz.Require(ModuleStockReports, authz.ActView)).Get("/valuation", h.Valuation)
	})
}

// Valuation: GET /reports/stock/valuation?outlet_id=&category_id=&from=&to=&limit=&cursor=
func (h *Handler) Valuation(w http.ResponseWriter, r *http.Request) {
	qs := r.URL.Query()
	limit, _ := strconv.Atoi(qs.Get("limit"))
	p := ValuationParams{
		OutletID:   qs.Get("outlet_id"),
		CategoryID: qs.Get("category_id"),
		From:       qs.Get("from"),
		To:         qs.Get("to"),
		Limit:      limit,
		Cursor:     qs.Get("cursor"),
	}
	result, err := h.svc.Valuation(r.Context(), actor(r), p)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, result)
}

func (h *Handler) fail(w http.ResponseWriter, r *http.Request, err error) {
	if errors.Is(err, ErrOutletForbidden) {
		httpx.Error(w, http.StatusForbidden, "OUTLET_FORBIDDEN", err.Error())
		return
	}
	if fe, ok := err.(FieldErrors); ok {
		httpx.Error(w, http.StatusBadRequest, "VALIDATION_ERROR", fe.Error())
		return
	}
	httpx.Error(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
}

// actor extracts authz.Actor from request context.
func actor(r *http.Request) authz.Actor {
	a, _ := authz.ActorFrom(r.Context())
	return a
}
