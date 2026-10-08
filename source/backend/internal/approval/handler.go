package approval

import (
	"errors"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"
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

func (h *Handler) Routes(r chi.Router) {
	r.Route("/approvals", func(r chi.Router) {
		r.Use(httpx.RequireAuth(h.tokens), h.resolver.Authenticate)
		// Kasir memilih penyetuju dan memeriksa PIN sebelum menyimpan nota.
		r.With(authz.Require("sales_orders", authz.ActCreate)).Get("/approvers", h.Approvers)
		r.With(authz.Require("sales_orders", authz.ActCreate)).Post("/check", h.Check)
		// PIN milik sendiri (hanya pemegang izin penyetuju).
		anyApprove := authz.RequireAny([2]string{Module, authz.ActApprove}, [2]string{ModuleOutletSwitch, authz.ActApprove}, [2]string{ModuleSaleEdit, authz.ActApprove}, [2]string{ModuleCreditLimit, authz.ActApprove})
		r.With(anyApprove).Get("/pin", h.PinStatus)
		r.With(anyApprove).Put("/pin", h.SetPin)
	})
}

func actor(r *http.Request) authz.Actor {
	a, _ := authz.ActorFrom(r.Context())
	return a
}

func (h *Handler) Approvers(w http.ResponseWriter, r *http.Request) {
	a := actor(r)
	module, outletID := Module, a.OutletID
	// ?for=outlet_switch&outlet_id=<tujuan>: penyetuju pindah outlet, dicari di outlet tujuan (yang boleh diakses pemanggil).
	if m := r.URL.Query().Get("for"); m != "" {
		if !ValidModule(m) {
			httpx.ValidationError(w, map[string]string{"for": "INVALID"})
			return
		}
		module = m
	}
	if s := r.URL.Query().Get("outlet_id"); s != "" {
		id, err := uuid.Parse(s)
		if err != nil || !a.Outlets[id] {
			httpx.ValidationError(w, map[string]string{"outlet_id": "INVALID"})
			return
		}
		outletID = id
	}
	list, err := h.svc.ApproversFor(r.Context(), a, module, outletID)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"approvers": list})
}

func (h *Handler) Check(w http.ResponseWriter, r *http.Request) {
	var req struct {
		UserID uuid.UUID `json:"user_id"`
		PIN    string    `json:"pin"`
	}
	if !httpx.DecodeJSON(w, r, &req) {
		return
	}
	ap, err := h.svc.Check(r.Context(), actor(r), req.UserID, req.PIN)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, ap)
}

func (h *Handler) PinStatus(w http.ResponseWriter, r *http.Request) {
	has, can, err := h.svc.PinStatus(r.Context(), actor(r))
	if err != nil {
		h.fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"has_pin": has, "can_approve": can})
}

func (h *Handler) SetPin(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Password string `json:"password"`
		PIN      string `json:"pin"`
	}
	if !httpx.DecodeJSON(w, r, &req) {
		return
	}
	if err := h.svc.SetPin(r.Context(), actor(r), req.Password, req.PIN); err != nil {
		h.fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"has_pin": true})
}

// Fail memetakan galat persetujuan ke respons HTTP; dipakai juga oleh modul penjualan.
func Fail(w http.ResponseWriter, err error) bool {
	var lk *LockedError
	switch {
	case errors.As(err, &lk):
		httpx.Retry(w, "PIN_LOCKED", "Terlalu banyak percobaan PIN. Coba lagi nanti.", lk.RetryAfter)
	case errors.Is(err, ErrPinRequired):
		httpx.Error(w, http.StatusForbidden, "PIN_REQUIRED", "Persetujuan Owner/Supervisor (PIN) wajib.")
	case errors.Is(err, ErrInvalidPin):
		httpx.Error(w, http.StatusForbidden, "INVALID_PIN", "Penyetuju atau PIN salah.")
	case errors.Is(err, ErrPinWeak):
		httpx.ValidationError(w, map[string]string{"pin": "PIN_WEAK"})
	case errors.Is(err, ErrPinFormat):
		httpx.ValidationError(w, map[string]string{"pin": "PIN_FORMAT"})
	case errors.Is(err, ErrBadPassword):
		httpx.ValidationError(w, map[string]string{"password": "PASSWORD_WRONG"})
	case errors.Is(err, ErrForbidden):
		httpx.Error(w, http.StatusForbidden, "FORBIDDEN", "Anda tidak memiliki izin untuk tindakan ini.")
	default:
		return false
	}
	return true
}

func (h *Handler) fail(w http.ResponseWriter, r *http.Request, err error) {
	if Fail(w, err) {
		return
	}
	h.log.ErrorContext(r.Context(), "approval", "err", err, "path", r.URL.Path)
	httpx.Error(w, http.StatusInternalServerError, "INTERNAL", "Terjadi kesalahan pada server.")
}
