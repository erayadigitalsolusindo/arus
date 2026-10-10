package outlet

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/google/uuid"

	"aciraba/internal/authz"
	pauth "aciraba/internal/platform/auth"
	"aciraba/internal/platform/httpx"
)

// ModuleID = modul izin pengelolaan outlet (authz.Modules).
const ModuleID = "outlets"

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
	r.Route("/outlets", func(r chi.Router) {
		r.Use(httpx.RequireAuth(h.tokens), h.resolver.Authenticate)
		// Pemilih outlet: semua pengguna terautentikasi, tanpa izin modul.
		r.Get("/accessible", h.Accessible)

		r.With(authz.Require(ModuleID, authz.ActView)).Get("/", h.List)
		r.With(authz.Require(ModuleID, authz.ActCreate)).Post("/", h.Create)
		r.With(authz.Require(ModuleID, authz.ActUpdate)).Put("/{id}", h.Update)
	})
}

// Pct menerima angka JSON; kosong/hilang dianggap 0.
type request struct {
	Code        string      `json:"code"`
	Name        string      `json:"name"`
	Timezone    string      `json:"timezone"`
	TaxStorePct json.Number `json:"tax_store_pct"`
	TaxGovPct   json.Number `json:"tax_gov_pct"`
	Active      *bool       `json:"active"`
	// Data struk (opsional; kosong = tidak dicetak).
	Address       string `json:"address"`
	Phone         string `json:"phone"`
	ReceiptHeader string `json:"receipt_header"`
	ReceiptFooter string `json:"receipt_footer"`
}

func (r request) input(creating bool) (Input, bool) {
	in := Input{Code: r.Code, Name: r.Name, Timezone: r.Timezone, TaxStorePct: r.TaxStorePct.String(), TaxGovPct: r.TaxGovPct.String(), Active: true,
		Address: r.Address, Phone: r.Phone, ReceiptHeader: r.ReceiptHeader, ReceiptFooter: r.ReceiptFooter}
	if !creating {
		if r.Active == nil {
			return in, false
		}
		in.Active = *r.Active
	}
	return in, true
}

func actor(r *http.Request) authz.Actor {
	a, _ := authz.ActorFrom(r.Context())
	return a
}

func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	list, err := h.svc.List(r.Context(), actor(r))
	if err != nil {
		h.fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"outlets": list})
}

func (h *Handler) Accessible(w http.ResponseWriter, r *http.Request) {
	list, err := h.svc.Accessible(r.Context(), actor(r))
	if err != nil {
		h.fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"outlets": list, "current_id": actor(r).OutletID})
}

func (h *Handler) Create(w http.ResponseWriter, r *http.Request) {
	var req request
	if !httpx.DecodeJSON(w, r, &req) {
		return
	}
	in, _ := req.input(true)
	o, err := h.svc.Create(r.Context(), actor(r), in)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusCreated, o)
}

func (h *Handler) Update(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		httpx.Error(w, http.StatusNotFound, "NOT_FOUND", "Data tidak ditemukan.")
		return
	}
	var req request
	if !httpx.DecodeJSON(w, r, &req) {
		return
	}
	in, ok := req.input(false)
	if !ok {
		httpx.ValidationError(w, map[string]string{"active": "REQUIRED"})
		return
	}
	o, err := h.svc.Update(r.Context(), actor(r), id, in)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, o)
}

func (h *Handler) fail(w http.ResponseWriter, r *http.Request, err error) {
	var fields FieldErrors
	switch {
	case errors.As(err, &fields):
		httpx.ValidationError(w, fields)
	case errors.Is(err, ErrNotFound):
		httpx.Error(w, http.StatusNotFound, "NOT_FOUND", "Outlet tidak ditemukan.")
	case errors.Is(err, ErrNotAccessible):
		httpx.Error(w, http.StatusForbidden, "OUTLET_FORBIDDEN", "Anda tidak memiliki akses ke outlet ini.")
	case errors.Is(err, ErrCodeTaken):
		httpx.Error(w, http.StatusConflict, "OUTLET_CODE_TAKEN", "Kode outlet sudah dipakai.")
	case errors.Is(err, ErrLastOutlet):
		httpx.Error(w, http.StatusConflict, "LAST_OUTLET", "Outlet aktif terakhir tidak boleh dinonaktifkan.")
	case errors.Is(err, ErrCurrentOutlet):
		httpx.Error(w, http.StatusConflict, "CURRENT_OUTLET", "Outlet yang sedang dipakai tidak boleh dinonaktifkan. Pindah outlet dulu.")
	default:
		h.log.Error("outlet gagal", "err", err, "req_id", middleware.GetReqID(r.Context()))
		httpx.Error(w, http.StatusInternalServerError, "INTERNAL", "Terjadi kesalahan pada server.")
	}
}
