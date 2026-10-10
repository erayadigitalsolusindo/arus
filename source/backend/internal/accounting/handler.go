package accounting

import (
	"errors"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"aciraba/internal/authz"
	pauth "aciraba/internal/platform/auth"
	"aciraba/internal/platform/httpx"
)

// Modul izin (authz.Modules). journals.approve = posting, jurnal balik, saldo awal; accounting_periods.update = tutup buku,
// accounting_periods.approve = buka kembali periode tertutup (izin khusus, tercatat di audit).
const (
	ModuleAccounts = "accounts"
	ModuleJournals = "journals"
	ModulePeriods  = "accounting_periods"
	ModuleLedger   = "general_ledger"
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

func (h *Handler) Routes(r chi.Router) {
	r.Route("/accounting", func(r chi.Router) {
		r.Use(httpx.RequireAuth(h.tokens), h.resolver.Authenticate)
		req := authz.Require
		r.With(req(ModuleAccounts, authz.ActView)).Get("/accounts", h.ListAccounts)
		r.With(req(ModuleAccounts, authz.ActCreate)).Post("/accounts", h.CreateAccount)
		r.With(req(ModuleAccounts, authz.ActCreate)).Post("/accounts/seed-retail", h.SeedRetail)
		r.With(req(ModuleAccounts, authz.ActUpdate)).Put("/accounts/{id}", h.UpdateAccount)
		r.With(req(ModuleAccounts, authz.ActDelete)).Delete("/accounts/{id}", h.DeleteAccount)

		r.With(req(ModulePeriods, authz.ActView)).Get("/periods", h.ListPeriods)
		r.With(req(ModulePeriods, authz.ActUpdate)).Post("/periods/{month}/close", h.ClosePeriod)
		r.With(req(ModulePeriods, authz.ActApprove)).Post("/periods/{month}/reopen", h.ReopenPeriod)

		r.With(req(ModuleJournals, authz.ActView)).Get("/journals", h.ListJournals)
		r.With(req(ModuleJournals, authz.ActView)).Get("/journals/{id}", h.GetJournal)
		r.With(req(ModuleJournals, authz.ActCreate)).Post("/journals", h.CreateJournal)
		r.With(req(ModuleJournals, authz.ActUpdate)).Put("/journals/{id}", h.UpdateJournal)
		r.With(req(ModuleJournals, authz.ActDelete)).Delete("/journals/{id}", h.DeleteJournal)
		r.With(req(ModuleJournals, authz.ActApprove)).Post("/journals/{id}/post", h.PostJournal)
		r.With(req(ModuleJournals, authz.ActApprove)).Post("/journals/{id}/reverse", h.ReverseJournal)
		r.With(req(ModuleJournals, authz.ActApprove)).Post("/opening", h.PostOpening)

		r.With(req(ModuleLedger, authz.ActView)).Get("/ledger", h.Ledger)
		// Laporan keuangan (fase B): memakai izin general_ledger.
		r.With(req(ModuleLedger, authz.ActView)).Get("/reports/trial-balance", h.TrialBalance)
		r.With(req(ModuleLedger, authz.ActView)).Get("/reports/income-statement", h.IncomeStatement)
		r.With(req(ModuleLedger, authz.ActView)).Get("/reports/balance-sheet", h.BalanceSheet)
		r.With(req(ModuleLedger, authz.ActView)).Get("/reports/cash-bank", h.CashBank)
		r.With(req(ModuleLedger, authz.ActView)).Get("/reports/general-journal", h.GeneralJournal)
	})
}

func actor(r *http.Request) authz.Actor {
	a, _ := authz.ActorFrom(r.Context())
	return a
}

func pathID(w http.ResponseWriter, r *http.Request) (uuid.UUID, bool) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		httpx.Error(w, http.StatusNotFound, "NOT_FOUND", "Data tidak ditemukan.")
		return uuid.Nil, false
	}
	return id, true
}

func queryID(w http.ResponseWriter, r *http.Request, key string) (uuid.UUID, bool) {
	s := r.URL.Query().Get(key)
	if s == "" {
		return uuid.Nil, true
	}
	id, err := uuid.Parse(s)
	if err != nil {
		httpx.ValidationError(w, map[string]string{key: "INVALID"})
		return uuid.Nil, false
	}
	return id, true
}

func (h *Handler) ListAccounts(w http.ResponseWriter, r *http.Request) {
	res, err := h.svc.ListAccounts(r.Context(), actor(r))
	if err != nil {
		h.fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"items": res})
}

func (h *Handler) CreateAccount(w http.ResponseWriter, r *http.Request) {
	var in AccountInput
	if !httpx.DecodeJSONLimit(w, r, &in, 8<<10) {
		return
	}
	res, err := h.svc.CreateAccount(r.Context(), actor(r), in)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusCreated, res)
}

func (h *Handler) SeedRetail(w http.ResponseWriter, r *http.Request) {
	res, err := h.svc.SeedRetailTemplate(r.Context(), actor(r))
	if err != nil {
		h.fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusCreated, map[string]any{"items": res})
}

func (h *Handler) UpdateAccount(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	var in AccountInput
	if !httpx.DecodeJSONLimit(w, r, &in, 8<<10) {
		return
	}
	res, err := h.svc.UpdateAccount(r.Context(), actor(r), id, in)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, res)
}

func (h *Handler) DeleteAccount(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	if err := h.svc.DeleteAccount(r.Context(), actor(r), id); err != nil {
		h.fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) ListPeriods(w http.ResponseWriter, r *http.Request) {
	res, err := h.svc.ListPeriods(r.Context(), actor(r))
	if err != nil {
		h.fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"items": res})
}

func (h *Handler) ClosePeriod(w http.ResponseWriter, r *http.Request) {
	res, err := h.svc.ClosePeriod(r.Context(), actor(r), chi.URLParam(r, "month"))
	if err != nil {
		h.fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, res)
}

func (h *Handler) ReopenPeriod(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Reason string `json:"reason"`
	}
	if !httpx.DecodeJSONLimit(w, r, &in, 2<<10) {
		return
	}
	res, err := h.svc.ReopenPeriod(r.Context(), actor(r), chi.URLParam(r, "month"), in.Reason)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, res)
}

// ListJournals: GET /accounting/journals?from=&to=&status=&type=&outlet_id=&cursor=&limit=
func (h *Handler) ListJournals(w http.ResponseWriter, r *http.Request) {
	qs := r.URL.Query()
	outlet, ok := queryID(w, r, "outlet_id")
	if !ok {
		return
	}
	p := JournalListParams{OutletID: outlet, From: qs.Get("from"), To: qs.Get("to"), Status: qs.Get("status"), Type: qs.Get("type"), Cursor: qs.Get("cursor")}
	p.Limit, _ = strconv.Atoi(qs.Get("limit"))
	res, err := h.svc.ListJournals(r.Context(), actor(r), p)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, res)
}

func (h *Handler) GetJournal(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	res, err := h.svc.Get(r.Context(), actor(r), id)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, res)
}

func (h *Handler) CreateJournal(w http.ResponseWriter, r *http.Request) {
	var in JournalInput
	if !httpx.DecodeJSONLimit(w, r, &in, 128<<10) {
		return
	}
	res, err := h.svc.SaveDraft(r.Context(), actor(r), uuid.Nil, in)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusCreated, res)
}

func (h *Handler) UpdateJournal(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	var in JournalInput
	if !httpx.DecodeJSONLimit(w, r, &in, 128<<10) {
		return
	}
	res, err := h.svc.SaveDraft(r.Context(), actor(r), id, in)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, res)
}

func (h *Handler) DeleteJournal(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	if err := h.svc.DeleteDraft(r.Context(), actor(r), id); err != nil {
		h.fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) PostJournal(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	res, err := h.svc.Post(r.Context(), actor(r), id)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, res)
}

func (h *Handler) ReverseJournal(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	var in struct {
		Date      string `json:"date"`
		Narration string `json:"narration"`
	}
	if !httpx.DecodeJSONLimit(w, r, &in, 4<<10) {
		return
	}
	res, err := h.svc.Reverse(r.Context(), actor(r), id, in.Date, in.Narration)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusCreated, res)
}

func (h *Handler) PostOpening(w http.ResponseWriter, r *http.Request) {
	var in OpeningInput
	if !httpx.DecodeJSONLimit(w, r, &in, 128<<10) {
		return
	}
	res, err := h.svc.PostOpening(r.Context(), actor(r), in)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusCreated, res)
}

// Ledger: GET /accounting/ledger?account_id=&from=&to=&outlet_id=&cursor=&limit=
func (h *Handler) Ledger(w http.ResponseWriter, r *http.Request) {
	qs := r.URL.Query()
	acc, err := uuid.Parse(qs.Get("account_id"))
	if err != nil {
		httpx.ValidationError(w, map[string]string{"account_id": "INVALID"})
		return
	}
	outlet, ok := queryID(w, r, "outlet_id")
	if !ok {
		return
	}
	p := LedgerParams{AccountID: acc, OutletID: outlet, From: qs.Get("from"), To: qs.Get("to"), Cursor: qs.Get("cursor")}
	p.Limit, _ = strconv.Atoi(qs.Get("limit"))
	res, err := h.svc.Ledger(r.Context(), actor(r), p)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, res)
}

// report menjalankan laporan berparameter outlet_id + rentang (from/to) dan menulis hasilnya sebagai JSON.
func (h *Handler) report(w http.ResponseWriter, r *http.Request, run func(a authz.Actor, outlet uuid.UUID, qs url.Values) (any, error)) {
	outlet, ok := queryID(w, r, "outlet_id")
	if !ok {
		return
	}
	res, err := run(actor(r), outlet, r.URL.Query())
	if err != nil {
		h.fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, res)
}

func (h *Handler) TrialBalance(w http.ResponseWriter, r *http.Request) {
	h.report(w, r, func(a authz.Actor, o uuid.UUID, q url.Values) (any, error) {
		return h.svc.TrialBalance(r.Context(), a, o, q.Get("from"), q.Get("to"))
	})
}

func (h *Handler) IncomeStatement(w http.ResponseWriter, r *http.Request) {
	h.report(w, r, func(a authz.Actor, o uuid.UUID, q url.Values) (any, error) {
		return h.svc.IncomeStatement(r.Context(), a, o, q.Get("from"), q.Get("to"))
	})
}

func (h *Handler) BalanceSheet(w http.ResponseWriter, r *http.Request) {
	h.report(w, r, func(a authz.Actor, o uuid.UUID, q url.Values) (any, error) {
		return h.svc.BalanceSheet(r.Context(), a, o, q.Get("as_of"))
	})
}

func (h *Handler) CashBank(w http.ResponseWriter, r *http.Request) {
	h.report(w, r, func(a authz.Actor, o uuid.UUID, q url.Values) (any, error) {
		return h.svc.CashBank(r.Context(), a, o, q.Get("from"), q.Get("to"))
	})
}

func (h *Handler) GeneralJournal(w http.ResponseWriter, r *http.Request) {
	h.report(w, r, func(a authz.Actor, o uuid.UUID, q url.Values) (any, error) {
		limit, _ := strconv.Atoi(q.Get("limit"))
		return h.svc.GeneralJournal(r.Context(), a, o, q.Get("from"), q.Get("to"), q.Get("cursor"), limit)
	})
}

func (h *Handler) fail(w http.ResponseWriter, r *http.Request, err error) {
	var fields FieldErrors
	e := func(status int, code, msg string) { httpx.Error(w, status, code, msg) }
	switch {
	case errors.As(err, &fields):
		httpx.ValidationError(w, fields)
	case errors.Is(err, ErrNotFound):
		e(http.StatusNotFound, "NOT_FOUND", "Data tidak ditemukan.")
	case errors.Is(err, ErrOutletForbidden):
		e(http.StatusForbidden, "OUTLET_FORBIDDEN", "Anda tidak berhak atas outlet ini.")
	case errors.Is(err, ErrJournalUnbalanced):
		e(http.StatusUnprocessableEntity, "JOURNAL_UNBALANCED", "Total debit dan kredit harus sama.")
	case errors.Is(err, ErrJournalShape):
		e(http.StatusUnprocessableEntity, "JOURNAL_SHAPE_INVALID", "Susunan baris tidak sesuai jenis jurnal (minimal 2 baris; KM/KK/TK mengikuti aturan kas/bank).")
	case errors.Is(err, ErrAccountInvalid):
		e(http.StatusUnprocessableEntity, "ACCOUNT_INVALID", "Akun jurnal harus akun buku yang aktif.")
	case errors.Is(err, ErrPeriodClosed):
		e(http.StatusConflict, "PERIOD_CLOSED", "Periode akuntansi sudah ditutup.")
	case errors.Is(err, ErrPeriodNotClosed):
		e(http.StatusConflict, "PERIOD_NOT_CLOSED", "Periode ini belum ditutup.")
	case errors.Is(err, ErrDraftsInPeriod):
		e(http.StatusConflict, "PERIOD_HAS_DRAFTS", "Masih ada jurnal draf di periode ini; posting atau hapus dulu.")
	case errors.Is(err, ErrJournalPosted):
		e(http.StatusConflict, "JOURNAL_POSTED", "Jurnal sudah diposting; koreksi lewat jurnal balik.")
	case errors.Is(err, ErrNotPosted):
		e(http.StatusConflict, "JOURNAL_NOT_POSTED", "Jurnal belum diposting.")
	case errors.Is(err, ErrAlreadyReversed):
		e(http.StatusConflict, "JOURNAL_ALREADY_REVERSED", "Jurnal ini sudah dibalik.")
	case errors.Is(err, ErrOpeningExists):
		e(http.StatusConflict, "OPENING_EXISTS", "Jurnal pembuka sudah ada; balik dulu bila ingin mencatat ulang.")
	case errors.Is(err, ErrCodeTaken):
		e(http.StatusConflict, "ACCOUNT_CODE_TAKEN", "Kode akun sudah dipakai.")
	case errors.Is(err, ErrParentInvalid):
		e(http.StatusUnprocessableEntity, "ACCOUNT_PARENT_INVALID", "Induk harus akun grup dengan kelas yang sama.")
	case errors.Is(err, ErrAccountInUse):
		e(http.StatusConflict, "ACCOUNT_IN_USE", "Akun masih dipakai (punya anak atau baris jurnal); nonaktifkan saja.")
	case errors.Is(err, ErrAccountSystem):
		e(http.StatusConflict, "ACCOUNT_SYSTEM", "Akun sistem dipakai otomatis oleh aplikasi; tidak boleh dihapus, dinonaktifkan, atau diubah kode/kelasnya.")
	case errors.Is(err, ErrCOAExists):
		e(http.StatusConflict, "COA_EXISTS", "Bagan akun sudah ada.")
	default:
		h.log.ErrorContext(r.Context(), "accounting", "err", err, "path", r.URL.Path)
		e(http.StatusInternalServerError, "INTERNAL", "Terjadi kesalahan pada server.")
	}
}
