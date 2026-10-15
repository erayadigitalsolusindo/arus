package sales

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

// Routes: GET /reports/sales/by-date dan GET /reports/sales/detail
// (butuh login + izin sales_reports.view; tenant/outlet dari token).
// Query param `format=xlsx|pdf` pada keduanya = unduh berkas, bukan JSON.
func (h *Handler) Routes(r chi.Router) {
	r.Route("/reports/sales", func(r chi.Router) {
		r.Use(httpx.RequireAuth(h.tokens), h.resolver.Authenticate)
		r.With(authz.Require(ModuleReports, authz.ActView)).Get("/by-date", h.ByDate)
		r.With(authz.Require(ModuleReports, authz.ActView)).Get("/detail", h.Detail)
	})
}

// ByDate: GET /reports/sales/by-date?from=&to=&outlet_id=&cashier_id=&category_id=&payment_method_id=&all_outlets=&hide_empty=&format=
func (h *Handler) ByDate(w http.ResponseWriter, r *http.Request) {
	qs := r.URL.Query()
	p := ByDateParams{
		AllOutlets: qs.Get("all_outlets") == "true",
		OutletID:   qs.Get("outlet_id"),
		From:       qs.Get("from"),
		To:         qs.Get("to"),
		CashierID:  qs.Get("cashier_id"),
		CategoryID: qs.Get("category_id"),
		MethodID:   qs.Get("payment_method_id"),
		HideEmpty:  qs.Get("hide_empty") == "true",
	}
	result, err := h.svc.ByDate(r.Context(), actor(r), p)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	switch qs.Get("format") {
	case "xlsx":
		b, err := ToXLSX(result)
		if err != nil {
			h.fail(w, r, err)
			return
		}
		h.writeFile(w, r, FileName(result.From, result.To, "xlsx"), xlsxContentType, b)
	case "pdf":
		b, err := ToPDF(result)
		if err != nil {
			h.fail(w, r, err)
			return
		}
		h.writeFile(w, r, FileName(result.From, result.To, "pdf"), pdfContentType, b)
	default:
		httpx.JSON(w, http.StatusOK, result)
	}
}

// Detail: GET /reports/sales/detail?from=&to=&outlet_id=&cashier_id=&category_id=&status=&cursor=&limit=&format=
func (h *Handler) Detail(w http.ResponseWriter, r *http.Request) {
	qs := r.URL.Query()
	limit, _ := strconv.Atoi(qs.Get("limit"))
	p := DetailParams{
		OutletID:   qs.Get("outlet_id"),
		From:       qs.Get("from"),
		To:         qs.Get("to"),
		CashierID:  qs.Get("cashier_id"),
		CategoryID: qs.Get("category_id"),
		Status:     qs.Get("status"),
		Cursor:     qs.Get("cursor"),
		Limit:      limit,
		ForExport:  qs.Get("format") == "xlsx" || qs.Get("format") == "pdf",
	}
	result, err := h.svc.Detail(r.Context(), actor(r), p)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	switch qs.Get("format") {
	case "xlsx":
		b, err := DetailToXLSX(result)
		if err != nil {
			h.fail(w, r, err)
			return
		}
		h.writeFile(w, r, DetailFileName(result.From, result.To, "xlsx"), xlsxContentType, b)
	case "pdf":
		b, err := DetailToPDF(result)
		if err != nil {
			h.fail(w, r, err)
			return
		}
		h.writeFile(w, r, DetailFileName(result.From, result.To, "pdf"), pdfContentType, b)
	default:
		httpx.JSON(w, http.StatusOK, result)
	}
}

const (
	xlsxContentType = "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"
	pdfContentType  = "application/pdf"
)

// writeFile mengirim berkas unduhan (header sama untuk semua ekspor laporan).
func (h *Handler) writeFile(w http.ResponseWriter, r *http.Request, name, contentType string, data []byte) {
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": name}))
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Content-Length", strconv.Itoa(len(data)))
	if _, err := w.Write(data); err != nil {
		h.log.Error("tulis berkas ekspor laporan penjualan", "file", name, "err", err)
	}
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

// actor extracts authz.Actor from request context. Same pattern as sales/handler.go.
func actor(r *http.Request) authz.Actor {
	a, _ := authz.ActorFrom(r.Context())
	return a
}
