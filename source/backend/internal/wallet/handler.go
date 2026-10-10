package wallet

import (
	"errors"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/google/uuid"

	"aciraba/internal/authz"
	pauth "aciraba/internal/platform/auth"
	"aciraba/internal/platform/httpx"
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
//	GET  /member-deposits/{id}?before=        saldo + riwayat deposit member (member_deposits.view; juga kasir untuk saldo)
//	POST /member-deposits/{id}/topup          top-up (member_deposits.create, Idempotency-Key)
//	POST /member-deposits/{id}/withdraw       tarik deposit (member_deposits.create, Idempotency-Key)
//	GET  /supplier-credits?q=&positive=&cursor=  daftar pemasok berkredit (supplier_credits.view)
//	GET  /supplier-credits/{id}?before=       saldo + riwayat kredit pemasok (supplier_credits.view, juga pembayar hutang)
//	POST /supplier-credits/{id}/cash-out      pencairan kredit oleh pemasok (supplier_credits.create, Idempotency-Key)
func (h *Handler) Routes(r chi.Router) {
	r.Route("/member-deposits", func(r chi.Router) {
		r.Use(httpx.RequireAuth(h.tokens), h.resolver.Authenticate)
		r.With(authz.RequireAny([2]string{ModuleDeposits, authz.ActView}, [2]string{"sales_orders", authz.ActCreate},
			[2]string{"member_receivables", authz.ActCreate}, [2]string{"sales_returns", authz.ActCreate})).Get("/{id}", h.depositAccount)
		r.With(authz.Require(ModuleDeposits, authz.ActCreate)).Post("/{id}/topup", h.cash(MemberDeposit, DepTopup))
		r.With(authz.Require(ModuleDeposits, authz.ActCreate)).Post("/{id}/withdraw", h.cash(MemberDeposit, DepWithdraw))
	})
	r.Route("/supplier-credits", func(r chi.Router) {
		r.Use(httpx.RequireAuth(h.tokens), h.resolver.Authenticate)
		r.With(authz.Require(ModuleCredits, authz.ActView)).Get("/", h.creditList)
		r.With(authz.RequireAny([2]string{ModuleCredits, authz.ActView}, [2]string{"supplier_payables", authz.ActCreate})).Get("/{id}", h.creditAccount)
		r.With(authz.Require(ModuleCredits, authz.ActCreate)).Post("/{id}/cash-out", h.cash(SupplierCredit, CrCashOut))
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

func (h *Handler) fail(w http.ResponseWriter, r *http.Request, err error) {
	var fields FieldErrors
	switch {
	case errors.As(err, &fields):
		httpx.ValidationError(w, fields)
	case errors.Is(err, ErrNotFound):
		httpx.Error(w, http.StatusNotFound, "NOT_FOUND", "Data tidak ditemukan.")
	case errors.Is(err, ErrKeyRequired):
		httpx.Error(w, http.StatusBadRequest, "IDEMPOTENCY_KEY_REQUIRED", "Header Idempotency-Key wajib diisi.")
	case errors.Is(err, ErrKeyMismatch):
		httpx.Error(w, http.StatusUnprocessableEntity, "IDEMPOTENCY_MISMATCH", "Idempotency-Key sudah dipakai untuk isi yang berbeda.")
	case errors.Is(err, ErrOutletGone):
		httpx.Error(w, http.StatusConflict, "OUTLET_INACTIVE", "Outlet aktif tidak tersedia.")
	default:
		h.log.Error("saldo titipan gagal", "err", err, "req_id", middleware.GetReqID(r.Context()))
		httpx.Error(w, http.StatusInternalServerError, "INTERNAL", "Terjadi kesalahan pada server.")
	}
}

func (h *Handler) account(w http.ResponseWriter, r *http.Request, l Ledger) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	before, _ := strconv.ParseInt(r.URL.Query().Get("before"), 10, 64)
	acc, err := h.svc.Account(r.Context(), actor(r), l, id, max(before, 0))
	if err != nil {
		h.fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, acc)
}

func (h *Handler) depositAccount(w http.ResponseWriter, r *http.Request) {
	h.account(w, r, MemberDeposit)
}
func (h *Handler) creditAccount(w http.ResponseWriter, r *http.Request) {
	h.account(w, r, SupplierCredit)
}

func (h *Handler) cash(l Ledger, kind string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, ok := pathID(w, r)
		if !ok {
			return
		}
		var in CashInput
		if !httpx.DecodeJSON(w, r, &in) {
			return
		}
		acc, replayed, err := h.svc.Cash(r.Context(), actor(r), l, kind, id, r.Header.Get("Idempotency-Key"), in)
		if err != nil {
			h.fail(w, r, err)
			return
		}
		status := http.StatusCreated
		if replayed {
			w.Header().Set("Idempotent-Replay", "true")
			status = http.StatusOK
		}
		httpx.JSON(w, status, acc)
	}
}

func (h *Handler) creditList(w http.ResponseWriter, r *http.Request) {
	qs := r.URL.Query()
	limit, _ := strconv.Atoi(qs.Get("limit"))
	rows, next, err := h.svc.CreditList(r.Context(), actor(r), qs.Get("q"), qs.Get("positive") == "true", qs.Get("cursor"), limit)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"data": rows, "next_cursor": next, "has_more": next != ""})
}
