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
		r.With(authz.Require(ModuleReports, authz.ActView)).Get("/invoices", h.Invoices)
		r.With(authz.Require(ModuleReports, authz.ActView)).Get("/items", h.Items)
		r.With(authz.Require(ModuleReports, authz.ActView)).Get("/cashiers", h.Cashiers)
		r.With(authz.Require(ModuleReports, authz.ActView)).Get("/invoice-detail", h.InvoiceDetail)
		r.With(authz.Require(ModuleReports, authz.ActView)).Get("/customers", h.Customers)
		r.With(authz.Require(ModuleReports, authz.ActView)).Get("/customer-invoices", h.CustomerInvoices)
	})
}

// Cashiers: GET /reports/sales/cashiers?from=&to=&outlet_id=&cashier_id=&status=open|closed&diff_only=&all_outlets=&cursor=&limit=&format=xlsx|pdf
func (h *Handler) Cashiers(w http.ResponseWriter, r *http.Request) {
	qs := r.URL.Query()
	limit, _ := strconv.Atoi(qs.Get("limit"))
	format := qs.Get("format")
	p := CashierParams{
		AllOutlets: qs.Get("all_outlets") == "true",
		OutletID:   qs.Get("outlet_id"),
		From:       qs.Get("from"),
		To:         qs.Get("to"),
		CashierID:  qs.Get("cashier_id"),
		Status:     qs.Get("status"),
		DiffOnly:   qs.Get("diff_only") == "true",
		Cursor:     qs.Get("cursor"),
		Limit:      limit,
		ForExport:  format == "xlsx" || format == "pdf",
	}
	result, err := h.svc.Cashiers(r.Context(), actor(r), p)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	switch format {
	case "xlsx":
		b, err := CashierToXLSX(result)
		if err != nil {
			h.fail(w, r, err)
			return
		}
		h.writeFile(w, r, CashierFileName(result.From, result.To, "xlsx"), xlsxContentType, b)
	case "pdf":
		b, err := CashierToPDF(result)
		if err != nil {
			h.fail(w, r, err)
			return
		}
		h.writeFile(w, r, CashierFileName(result.From, result.To, "pdf"), pdfContentType, b)
	default:
		httpx.JSON(w, http.StatusOK, result)
	}
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

// Detail: GET /reports/sales/detail?from=&to=&outlet_id=&cashier_id=&category_id=&status=&all_outlets=&cursor=&limit=&format=
func (h *Handler) Detail(w http.ResponseWriter, r *http.Request) {
	qs := r.URL.Query()
	limit, _ := strconv.Atoi(qs.Get("limit"))
	p := DetailParams{
		AllOutlets: qs.Get("all_outlets") == "true",
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

// Invoices: GET /reports/sales/invoices?from=&to=&outlet_id=&cashier_id=&status=&q=&all_outlets=&cursor=&limit=&format=xlsx
func (h *Handler) Invoices(w http.ResponseWriter, r *http.Request) {
	qs := r.URL.Query()
	limit, _ := strconv.Atoi(qs.Get("limit"))
	p := InvoiceParams{
		AllOutlets: qs.Get("all_outlets") == "true",
		OutletID:   qs.Get("outlet_id"),
		From:       qs.Get("from"),
		To:         qs.Get("to"),
		CashierID:  qs.Get("cashier_id"),
		Status:     qs.Get("status"),
		Search:     qs.Get("q"),
		Cursor:     qs.Get("cursor"),
		Limit:      limit,
		ForExport:  qs.Get("format") == "xlsx" || qs.Get("format") == "pdf",
	}
	if qs.Get("format") == "pdf" {
		p.ExportMax = maxInvoicePDFRows
	}
	result, err := h.svc.Invoices(r.Context(), actor(r), p)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	switch qs.Get("format") {
	case "xlsx":
		b, err := InvoiceToXLSX(result)
		if err != nil {
			h.fail(w, r, err)
			return
		}
		h.writeFile(w, r, InvoiceFileName(result.From, result.To, "xlsx"), xlsxContentType, b)
	case "pdf":
		b, err := InvoiceToPDF(result)
		if err != nil {
			h.fail(w, r, err)
			return
		}
		h.writeFile(w, r, InvoiceFileName(result.From, result.To, "pdf"), pdfContentType, b)
	default:
		httpx.JSON(w, http.StatusOK, result)
	}
}

// Items: GET /reports/sales/items?from=&to=&outlet_id=&cashier_id=&category_id=&q=&sort=&all_outlets=&limit=&format=xlsx|pdf
func (h *Handler) Items(w http.ResponseWriter, r *http.Request) {
	qs := r.URL.Query()
	limit, _ := strconv.Atoi(qs.Get("limit"))
	p := ItemParams{
		AllOutlets: qs.Get("all_outlets") == "true",
		OutletID:   qs.Get("outlet_id"),
		From:       qs.Get("from"),
		To:         qs.Get("to"),
		CashierID:  qs.Get("cashier_id"),
		CategoryID: qs.Get("category_id"),
		Search:     qs.Get("q"),
		Sort:       qs.Get("sort"),
		Limit:      limit,
		ForExport:  qs.Get("format") == "xlsx" || qs.Get("format") == "pdf",
	}
	if qs.Get("format") == "pdf" {
		p.ExportMax = maxItemPDFRows
	}
	result, err := h.svc.Items(r.Context(), actor(r), p)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	switch qs.Get("format") {
	case "xlsx":
		b, err := ItemToXLSX(result)
		if err != nil {
			h.fail(w, r, err)
			return
		}
		h.writeFile(w, r, ItemFileName(result.From, result.To, "xlsx"), xlsxContentType, b)
	case "pdf":
		b, err := ItemToPDF(result)
		if err != nil {
			h.fail(w, r, err)
			return
		}
		h.writeFile(w, r, ItemFileName(result.From, result.To, "pdf"), pdfContentType, b)
	default:
		httpx.JSON(w, http.StatusOK, result)
	}
}

// InvoiceDetail: GET /reports/sales/invoice-detail?(sama dengan /detail)
// Data sama dengan laporan detail; tampilan & berkas ekspor datar per faktur.
func (h *Handler) InvoiceDetail(w http.ResponseWriter, r *http.Request) {
	qs := r.URL.Query()
	limit, _ := strconv.Atoi(qs.Get("limit"))
	format := qs.Get("format")
	p := DetailParams{
		AllOutlets: qs.Get("all_outlets") == "true",
		OutletID:   qs.Get("outlet_id"),
		From:       qs.Get("from"),
		To:         qs.Get("to"),
		CashierID:  qs.Get("cashier_id"),
		CategoryID: qs.Get("category_id"),
		Status:     qs.Get("status"),
		Cursor:     qs.Get("cursor"),
		Limit:      limit,
		ForExport:  format == "xlsx" || format == "pdf",
	}
	result, err := h.svc.Detail(r.Context(), actor(r), p)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	switch format {
	case "xlsx":
		b, err := DetailToXLSX(result)
		if err != nil {
			h.fail(w, r, err)
			return
		}
		h.writeFile(w, r, InvoiceDetailFileName(result.From, result.To, "xlsx"), xlsxContentType, b)
	case "pdf":
		b, err := InvoiceDetailToPDF(result)
		if err != nil {
			h.fail(w, r, err)
			return
		}
		h.writeFile(w, r, InvoiceDetailFileName(result.From, result.To, "pdf"), pdfContentType, b)
	default:
		httpx.JSON(w, http.StatusOK, result)
	}
}

// Customers: GET /reports/sales/customers?from=&to=&outlet_id=&cashier_id=&q=&sort=&all_outlets=&limit=&format=xlsx|pdf
func (h *Handler) Customers(w http.ResponseWriter, r *http.Request) {
	qs := r.URL.Query()
	limit, _ := strconv.Atoi(qs.Get("limit"))
	format := qs.Get("format")
	p := CustomerParams{
		AllOutlets: qs.Get("all_outlets") == "true",
		OutletID:   qs.Get("outlet_id"),
		From:       qs.Get("from"),
		To:         qs.Get("to"),
		CashierID:  qs.Get("cashier_id"),
		Search:     qs.Get("q"),
		Sort:       qs.Get("sort"),
		Limit:      limit,
		ForExport:  format == "xlsx" || format == "pdf",
	}
	if format == "pdf" {
		p.ExportMax = maxCustomerPDFRows
	}
	result, err := h.svc.Customers(r.Context(), actor(r), p)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	switch format {
	case "xlsx":
		b, err := CustomerToXLSX(result)
		if err != nil {
			h.fail(w, r, err)
			return
		}
		h.writeFile(w, r, CustomerFileName(result.From, result.To, "xlsx"), xlsxContentType, b)
	case "pdf":
		b, err := CustomerToPDF(result)
		if err != nil {
			h.fail(w, r, err)
			return
		}
		h.writeFile(w, r, CustomerFileName(result.From, result.To, "pdf"), pdfContentType, b)
	default:
		httpx.JSON(w, http.StatusOK, result)
	}
}

// CustomerInvoices: GET /reports/sales/customer-invoices?from=&to=&member_id=&with_items=&cursor=&limit=&format=xlsx|pdf
// Layar: member_id wajib (satu pelanggan, keyset). Ekspor: member_id kosong = semua
// pelanggan (hasil q/sort), faktur dikelompokkan per pelanggan.
func (h *Handler) CustomerInvoices(w http.ResponseWriter, r *http.Request) {
	qs := r.URL.Query()
	limit, _ := strconv.Atoi(qs.Get("limit"))
	format := qs.Get("format")
	p := CustInvParams{
		AllOutlets: qs.Get("all_outlets") == "true",
		OutletID:   qs.Get("outlet_id"),
		From:       qs.Get("from"),
		To:         qs.Get("to"),
		CashierID:  qs.Get("cashier_id"),
		MemberID:   qs.Get("member_id"),
		Search:     qs.Get("q"),
		Sort:       qs.Get("sort"),
		Cursor:     qs.Get("cursor"),
		Limit:      limit,
		WithItems:  qs.Get("with_items") == "true",
		ForExport:  format == "xlsx" || format == "pdf",
	}
	result, err := h.svc.CustomerInvoices(r.Context(), actor(r), p)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	switch format {
	case "xlsx":
		b, err := CustInvToXLSX(result)
		if err != nil {
			h.fail(w, r, err)
			return
		}
		h.writeFile(w, r, CustInvFileName(result.From, result.To, p.WithItems, "xlsx"), xlsxContentType, b)
	case "pdf":
		b, err := CustInvToPDF(result)
		if err != nil {
			h.fail(w, r, err)
			return
		}
		h.writeFile(w, r, CustInvFileName(result.From, result.To, p.WithItems, "pdf"), pdfContentType, b)
	default:
		httpx.JSON(w, http.StatusOK, result)
	}
}

const (
	xlsxContentType ="application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"
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
