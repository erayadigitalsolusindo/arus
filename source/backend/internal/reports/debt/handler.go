package debt

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
	// Izin yang sudah ada: laporan hutang mengikuti hak lihat hutang pemasok, laporan piutang
	// hak lihat piutang member, saldo di pemasok hak lihat kredit pemasok.
	ModulePayable    = "supplier_payables"
	ModuleReceivable = "member_receivables"
	ModuleCredit     = "supplier_credits"

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

// Routes:
//
//	GET /reports/payables/{outstanding,payments,balances,ledger}     (supplier_payables.view)
//	GET /reports/receivables/{outstanding,payments,balances,ledger}  (member_receivables.view)
//	GET /reports/supplier-credits                                    (supplier_credits.view)
//
// Tenant/outlet dari token. Query `format=xlsx|pdf` = unduh berkas, bukan JSON.
func (h *Handler) Routes(r chi.Router) {
	mount := func(path, module string, k Kind) {
		r.Route(path, func(r chi.Router) {
			r.Use(httpx.RequireAuth(h.tokens), h.resolver.Authenticate)
			view := authz.Require(module, authz.ActView)
			r.With(view).Get("/outstanding", h.outstanding(k))
			r.With(view).Get("/payments", h.payments(k))
			r.With(view).Get("/balances", h.balances(k))
			r.With(view).Get("/ledger", h.ledger(k))
		})
	}
	mount("/reports/payables", ModulePayable, KindPayable)
	mount("/reports/receivables", ModuleReceivable, KindReceivable)
	r.Route("/reports/supplier-credits", func(r chi.Router) {
		r.Use(httpx.RequireAuth(h.tokens), h.resolver.Authenticate)
		r.With(authz.Require(ModuleCredit, authz.ActView)).Get("/", h.credit)
	})
}

func isExport(format string) bool { return format == "xlsx" || format == "pdf" }

// params membaca query: ?as_of=&from=&to=&outlet_id=&party_id=&method_id=&q=&all_outlets=&cursor=&limit=&format=
func params(qs url.Values) Params {
	limit, _ := strconv.Atoi(qs.Get("limit"))
	format := qs.Get("format")
	p := Params{
		AllOutlets: qs.Get("all_outlets") == "true",
		OutletID:   qs.Get("outlet_id"),
		AsOf:       qs.Get("as_of"),
		From:       qs.Get("from"),
		To:         qs.Get("to"),
		PartyID:    qs.Get("party_id"),
		MethodID:   qs.Get("method_id"),
		Search:     qs.Get("q"),
		Cursor:     qs.Get("cursor"),
		Limit:      limit,
		ForExport:  isExport(format),
	}
	if format == "pdf" {
		p.ExportMax = maxPDFRows
	}
	return p
}

func (h *Handler) outstanding(k Kind) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		qs := r.URL.Query()
		res, err := h.svc.Outstanding(r.Context(), actor(r), k, params(qs))
		if err != nil {
			h.fail(w, err)
			return
		}
		if !isExport(qs.Get("format")) {
			httpx.JSON(w, http.StatusOK, res)
			return
		}
		h.writeTable(w, OutstandingTable(k, res), qs.Get("format"), FileName(k, ViewOutstanding, res.AsOf, qs.Get("format")))
	}
}

func (h *Handler) balances(k Kind) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		qs := r.URL.Query()
		res, err := h.svc.Balances(r.Context(), actor(r), k, params(qs))
		if err != nil {
			h.fail(w, err)
			return
		}
		if !isExport(qs.Get("format")) {
			httpx.JSON(w, http.StatusOK, res)
			return
		}
		h.writeTable(w, BalanceTable(k, res), qs.Get("format"), FileName(k, ViewBalances, res.AsOf, qs.Get("format")))
	}
}

func (h *Handler) payments(k Kind) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		qs := r.URL.Query()
		res, err := h.svc.Payments(r.Context(), actor(r), k, params(qs))
		if err != nil {
			h.fail(w, err)
			return
		}
		if !isExport(qs.Get("format")) {
			httpx.JSON(w, http.StatusOK, res)
			return
		}
		h.writeTable(w, PaymentTable(k, res), qs.Get("format"), FileName(k, ViewPayments, res.From+"_"+res.To, qs.Get("format")))
	}
}

func (h *Handler) ledger(k Kind) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		qs := r.URL.Query()
		res, err := h.svc.Ledger(r.Context(), actor(r), k, params(qs))
		if err != nil {
			h.fail(w, err)
			return
		}
		if !isExport(qs.Get("format")) {
			httpx.JSON(w, http.StatusOK, res)
			return
		}
		h.writeTable(w, LedgerTable(k, res), qs.Get("format"), FileName(k, ViewLedger, res.From+"_"+res.To, qs.Get("format")))
	}
}

func (h *Handler) credit(w http.ResponseWriter, r *http.Request) {
	qs := r.URL.Query()
	res, err := h.svc.Credit(r.Context(), actor(r), params(qs))
	if err != nil {
		h.fail(w, err)
		return
	}
	if !isExport(qs.Get("format")) {
		httpx.JSON(w, http.StatusOK, res)
		return
	}
	h.writeTable(w, CreditTable(res), qs.Get("format"), FileName(KindPayable, ViewCredit, res.From+"_"+res.To, qs.Get("format")))
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
		h.log.Error("tulis berkas ekspor laporan hutang/piutang", "file", name, "err", err)
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
	h.log.Error("laporan hutang/piutang", "err", err)
	httpx.Error(w, http.StatusInternalServerError, "INTERNAL_ERROR", "internal error")
}

func actor(r *http.Request) authz.Actor {
	a, _ := authz.ActorFrom(r.Context())
	return a
}
