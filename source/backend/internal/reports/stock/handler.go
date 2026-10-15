package stock

import (
	"errors"
	"log/slog"
	"mime"
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
	// Laporan master (member, supplier, salesman) memakai izin yang sama dengan laporan persediaan.
	r.Route("/reports/master", func(r chi.Router) {
		r.Use(httpx.RequireAuth(h.tokens), h.resolver.Authenticate, authz.Require(ModuleStockReports, authz.ActView))
		r.Get("/members", h.master(MasterKindMembers))
		r.Get("/suppliers", h.master(MasterKindSuppliers))
		r.Get("/salespeople", h.master(MasterKindSalespeople))
	})
}

// master: GET ...?q=&status=active|inactive&limit=&cursor=&format=xlsx|pdf
// Tanpa format = JSON berhalaman; xlsx/pdf = unduh berkas (seluruh hasil sampai batas ekspor).
func (h *Handler) master(kind string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		qs := r.URL.Query()
		limit, _ := strconv.Atoi(qs.Get("limit"))
		p := MasterParams{Q: qs.Get("q"), Status: qs.Get("status"), Limit: limit, Cursor: qs.Get("cursor")}
		a := actor(r)

		if format := qs.Get("format"); format != "" {
			if format != "xlsx" && format != "pdf" {
				h.fail(w, r, FieldErrors{"format": "INVALID"})
				return
			}
			p.Cursor = ""
			tb, err := h.svc.MasterExport(r.Context(), a, kind, format, p)
			if err != nil {
				h.fail(w, r, err)
				return
			}
			var (
				b  []byte
				ct string
			)
			if format == "xlsx" {
				b, err = tb.ToXLSX()
				ct = "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"
			} else {
				b, err = tb.ToPDF()
				ct = "application/pdf"
			}
			if err != nil {
				h.fail(w, r, err)
				return
			}
			w.Header().Set("Content-Type", ct)
			w.Header().Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": tb.FileName(format)}))
			w.Header().Set("X-Content-Type-Options", "nosniff")
			w.Header().Set("Cache-Control", "no-cache")
			w.Header().Set("Content-Length", strconv.Itoa(len(b)))
			if _, err := w.Write(b); err != nil {
				h.log.Error("tulis berkas ekspor laporan master", "kind", kind, "err", err)
			}
			return
		}

		var (
			res any
			err error
		)
		switch kind {
		case MasterKindMembers:
			res, err = h.svc.MasterMembers(r.Context(), a, p)
		case MasterKindSuppliers:
			res, err = h.svc.MasterSuppliers(r.Context(), a, p)
		default:
			res, err = h.svc.MasterSalespeople(r.Context(), a, p)
		}
		if err != nil {
			h.fail(w, r, err)
			return
		}
		httpx.JSON(w, http.StatusOK, res)
	}
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
