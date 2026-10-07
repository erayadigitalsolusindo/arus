package item

import (
	"encoding/json"
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

// ModuleID = modul izin Daftar Item (authz.Modules).
const ModuleID = "items"

// maxBody: keterangan markdown sampai 5.000 karakter (hingga ~20 KB dalam UTF-8) + harga per cabang.
const maxBody = 64 << 10

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
	r.Route("/items", func(r chi.Router) {
		r.Use(httpx.RequireAuth(h.tokens), h.resolver.Authenticate)
		r.With(authz.Require(ModuleID, authz.ActView)).Get("/", h.List)
		r.With(authz.Require(ModuleID, authz.ActView)).Get("/{id}", h.Get)
		r.With(authz.Require(ModuleID, authz.ActCreate)).Post("/", h.Create)
		r.With(authz.Require(ModuleID, authz.ActUpdate)).Put("/{id}", h.Update)
		r.With(authz.Require(ModuleID, authz.ActUpdate)).Put("/{id}/active", h.SetActive)
	})
}

type outletPriceRequest struct {
	OutletID  string      `json:"outlet_id"`
	SellPrice json.Number `json:"sell_price"`
}

// request: angka sebagai json.Number agar presisi desimal terjaga (tidak lewat float64); id master "" = tidak diisi.
type request struct {
	SKU                string                `json:"sku"`
	Barcode            string                `json:"barcode"`
	Name               string                `json:"name"`
	WeightGrams        json.Number           `json:"weight_grams"`
	Cost               json.Number           `json:"cost"`
	SellPrice          json.Number           `json:"sell_price"`
	UnitID             string                `json:"unit_id"`
	CategoryID         string                `json:"category_id"`
	BrandID            string                `json:"brand_id"`
	PrincipalID        string                `json:"principal_id"`
	SupplierID         string                `json:"supplier_id"`
	Kind               string                `json:"kind"`
	AllowNegativeStock bool                  `json:"allow_negative_stock"`
	SellBelowCost      bool                  `json:"sell_below_cost"`
	Description        string                `json:"description"`
	OutletPrices       *[]outletPriceRequest `json:"outlet_prices"`
}

func (q request) input() Input {
	in := Input{
		SKU: q.SKU, Barcode: q.Barcode, Name: q.Name, Weight: q.WeightGrams.String(), Cost: q.Cost.String(), SellPrice: q.SellPrice.String(),
		UnitID: q.UnitID, CategoryID: q.CategoryID, BrandID: q.BrandID, PrincipalID: q.PrincipalID, SupplierID: q.SupplierID,
		Kind: q.Kind, AllowNegativeStock: q.AllowNegativeStock, SellBelowCost: q.SellBelowCost, Description: q.Description,
	}
	if q.OutletPrices != nil {
		list := make([]OutletPriceInput, 0, len(*q.OutletPrices))
		for _, p := range *q.OutletPrices {
			list = append(list, OutletPriceInput{OutletID: p.OutletID, SellPrice: p.SellPrice.String()})
		}
		in.OutletPrices = &list
	}
	return in
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

// List: ?q=&active=true|false&category_id=&limit=&offset= (active kosong = semua).
func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	qs := r.URL.Query()
	p := ListParams{Q: qs.Get("q"), CategoryID: qs.Get("category_id")}
	p.Limit, _ = strconv.Atoi(qs.Get("limit"))
	p.Offset, _ = strconv.Atoi(qs.Get("offset"))
	switch qs.Get("active") {
	case "true":
		t := true
		p.Active = &t
	case "false":
		f := false
		p.Active = &f
	}
	list, total, err := h.svc.List(r.Context(), actor(r), p)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"data": list, "total": total})
}

func (h *Handler) Get(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	it, err := h.svc.Get(r.Context(), actor(r), id)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, it)
}

func (h *Handler) Create(w http.ResponseWriter, r *http.Request) {
	var req request
	if !httpx.DecodeJSONLimit(w, r, &req, maxBody) {
		return
	}
	it, err := h.svc.Create(r.Context(), actor(r), req.input())
	if err != nil {
		h.fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusCreated, it)
}

func (h *Handler) Update(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	var req request
	if !httpx.DecodeJSONLimit(w, r, &req, maxBody) {
		return
	}
	it, err := h.svc.Update(r.Context(), actor(r), id, req.input())
	if err != nil {
		h.fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, it)
}

func (h *Handler) SetActive(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	var req struct {
		Active *bool `json:"active"`
	}
	if !httpx.DecodeJSON(w, r, &req) {
		return
	}
	if req.Active == nil {
		httpx.ValidationError(w, map[string]string{"active": "REQUIRED"})
		return
	}
	it, err := h.svc.SetActive(r.Context(), actor(r), id, *req.Active)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, it)
}

func (h *Handler) fail(w http.ResponseWriter, r *http.Request, err error) {
	var fields FieldErrors
	switch {
	case errors.As(err, &fields):
		httpx.ValidationError(w, fields)
	case errors.Is(err, ErrNotFound):
		httpx.Error(w, http.StatusNotFound, "NOT_FOUND", "Item tidak ditemukan.")
	case errors.Is(err, ErrCodeTaken):
		httpx.Error(w, http.StatusConflict, "CODE_TAKEN", "Kode barang sudah dipakai.")
	case errors.Is(err, ErrBarcodeTaken):
		httpx.Error(w, http.StatusConflict, "BARCODE_TAKEN", "Barcode sudah dipakai barang lain.")
	case errors.Is(err, ErrOutletForbidden):
		httpx.Error(w, http.StatusForbidden, "OUTLET_FORBIDDEN", "Anda tidak memiliki akses ke outlet yang dipilih.")
	default:
		h.log.Error("item gagal", "err", err, "req_id", middleware.GetReqID(r.Context()))
		httpx.Error(w, http.StatusInternalServerError, "INTERNAL", "Terjadi kesalahan pada server.")
	}
}
