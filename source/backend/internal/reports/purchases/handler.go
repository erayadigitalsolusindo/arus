package purchases

import (
	"errors"
	"log/slog"
	"mime"
	"net/http"
	"net/url"
	"strconv"

	"github.com/go-chi/chi/v5"

	"aciraba/internal/authz"
	pauth "aciraba/internal/platform/auth"
	"aciraba/internal/platform/httpx"
)

const (
	xlsxContentType = "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"
	pdfContentType  = "application/pdf"
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

// Routes: GET /reports/purchases/{invoices,invoice-detail,items,by-date}
// (login + izin purchase_reports.view; tenant/outlet dari token).
// Query `format=xlsx|pdf` = unduh berkas, bukan JSON.
func (h *Handler) Routes(r chi.Router) {
	r.Route("/reports/purchases", func(r chi.Router) {
		r.Use(httpx.RequireAuth(h.tokens), h.resolver.Authenticate)
		view := authz.Require(ModuleReports, authz.ActView)
		r.With(view).Get("/invoices", h.Invoices)
		r.With(view).Get("/invoice-detail", h.InvoiceDetail)
		r.With(view).Get("/items", h.Items)
		r.With(view).Get("/by-date", h.ByDate)
	})
}

func common(qs url.Values) Common {
	return Common{
		AllOutlets:  qs.Get("all_outlets") == "true",
		OutletID:    qs.Get("outlet_id"),
		From:        qs.Get("from"),
		To:          qs.Get("to"),
		SupplierID:  qs.Get("supplier_id"),
		PaymentType: qs.Get("payment_type"),
	}
}

func isExport(format string) bool { return format == "xlsx" || format == "pdf" }

func invoiceParams(qs url.Values, withLines bool) InvoiceParams {
	limit, _ := strconv.Atoi(qs.Get("limit"))
	format := qs.Get("format")
	p := InvoiceParams{
		Common:    common(qs),
		Status:    qs.Get("status"),
		Search:    qs.Get("q"),
		Cursor:    qs.Get("cursor"),
		Limit:     limit,
		ForExport: isExport(format),
		WithLines: withLines,
	}
	if format == "pdf" {
		p.ExportMax = maxPDFRows
	}
	return p
}

// Invoices: ?from=&to=&outlet_id=&supplier_id=&payment_type=&status=&q=&all_outlets=&cursor=&limit=&format=
func (h *Handler) Invoices(w http.ResponseWriter, r *http.Request) {
	qs := r.URL.Query()
	h.invoices(w, r, invoiceParams(qs, false), qs.Get("format"))
}

// InvoiceDetail: parameter sama dengan /invoices; tiap faktur membawa baris barang.
func (h *Handler) InvoiceDetail(w http.ResponseWriter, r *http.Request) {
	qs := r.URL.Query()
	h.invoices(w, r, invoiceParams(qs, true), qs.Get("format"))
}

func (h *Handler) invoices(w http.ResponseWriter, r *http.Request, p InvoiceParams, format string) {
	res, err := h.svc.Invoices(r.Context(), actor(r), p)
	if err != nil {
		h.fail(w, err)
		return
	}
	if !isExport(format) {
		httpx.JSON(w, http.StatusOK, res)
		return
	}
	t := InvoiceTable(res)
	if p.WithLines {
		t = InvoiceDetailTable(res)
	}
	h.writeTable(w, t, format, InvoiceFileName(res.From, res.To, p.WithLines, format))
}

// Items: ?from=&to=&outlet_id=&supplier_id=&payment_type=&category_id=&q=&sort=total|qty|name&all_outlets=&limit=&format=
func (h *Handler) Items(w http.ResponseWriter, r *http.Request) {
	qs := r.URL.Query()
	limit, _ := strconv.Atoi(qs.Get("limit"))
	format := qs.Get("format")
	p := ItemParams{
		Common:     common(qs),
		CategoryID: qs.Get("category_id"),
		Search:     qs.Get("q"),
		Sort:       qs.Get("sort"),
		Limit:      limit,
		ForExport:  isExport(format),
	}
	if format == "pdf" {
		p.ExportMax = maxPDFRows
	}
	res, err := h.svc.Items(r.Context(), actor(r), p)
	if err != nil {
		h.fail(w, err)
		return
	}
	if !isExport(format) {
		httpx.JSON(w, http.StatusOK, res)
		return
	}
	h.writeTable(w, ItemTable(res), format, ItemFileName(res.From, res.To, format))
}

// ByDate: ?from=&to=&outlet_id=&supplier_id=&payment_type=&all_outlets=&hide_empty=&format=
func (h *Handler) ByDate(w http.ResponseWriter, r *http.Request) {
	qs := r.URL.Query()
	format := qs.Get("format")
	res, err := h.svc.ByDate(r.Context(), actor(r), ByDateParams{
		Common:    common(qs),
		HideEmpty: qs.Get("hide_empty") == "true",
	})
	if err != nil {
		h.fail(w, err)
		return
	}
	if !isExport(format) {
		httpx.JSON(w, http.StatusOK, res)
		return
	}
	h.writeTable(w, ByDateTable(res), format, ByDateFileName(res.From, res.To, format))
}

func (h *Handler) writeTable(w http.ResponseWriter, t table, format, name string) {
	var (
		b   []byte
		err error
		ct  = xlsxContentType
	)
	if format == "pdf" {
		b, err = t.ToPDF()
		ct = pdfContentType
	} else {
		b, err = t.ToXLSX()
	}
	if err != nil {
		h.fail(w, err)
		return
	}
	w.Header().Set("Content-Type", ct)
	w.Header().Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": name}))
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Content-Length", strconv.Itoa(len(b)))
	if _, err := w.Write(b); err != nil {
		h.log.Error("tulis berkas ekspor laporan pembelian", "file", name, "err", err)
	}
}

func (h *Handler) fail(w http.ResponseWriter, err error) {
	if errors.Is(err, ErrOutletForbidden) {
		httpx.Error(w, http.StatusForbidden, "OUTLET_FORBIDDEN", err.Error())
		return
	}
	var fe FieldErrors
	if errors.As(err, &fe) {
		httpx.Error(w, http.StatusBadRequest, "VALIDATION_ERROR", fe.Error())
		return
	}
	h.log.Error("laporan pembelian", "err", err)
	httpx.Error(w, http.StatusInternalServerError, "INTERNAL_ERROR", "internal error")
}

func actor(r *http.Request) authz.Actor {
	a, _ := authz.ActorFrom(r.Context())
	return a
}
