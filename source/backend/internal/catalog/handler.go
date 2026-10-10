package catalog

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

// SupplierModule = id modul izin supplier (authz.Modules); master sederhana memakai Kind.Path.
const SupplierModule = "suppliers"

// ItemsModule = id modul izin item; pemegang izin lihat item boleh memakai rute /catalog/lookup.
const ItemsModule = "items"

type Handler struct {
	svc      *Service
	resolver *authz.Resolver
	tokens   *pauth.TokenIssuer
	log      *slog.Logger
}

func NewHandler(svc *Service, resolver *authz.Resolver, tokens *pauth.TokenIssuer, log *slog.Logger) *Handler {
	return &Handler{svc: svc, resolver: resolver, tokens: tokens, log: log}
}

// Routes: /catalog/<master>  GET (daftar/cari), POST, PUT /{id} (nama), PUT /{id}/active (arsip/aktifkan).
func (h *Handler) Routes(r chi.Router) {
	r.Route("/catalog", func(r chi.Router) {
		r.Use(httpx.RequireAuth(h.tokens), h.resolver.Authenticate)
		for _, k := range Kinds {
			r.Route("/"+k.Path, func(r chi.Router) {
				r.With(authz.Require(k.Path, authz.ActView)).Get("/", h.listSimple(k))
				r.With(authz.Require(k.Path, authz.ActCreate)).Post("/", h.createSimple(k))
				r.With(authz.Require(k.Path, authz.ActUpdate)).Put("/{id}", h.renameSimple(k))
				r.With(authz.Require(k.Path, authz.ActUpdate)).Put("/{id}/active", h.activeSimple(k))
			})
		}
		// Pilihan untuk combobox di form lain (mis. item): hanya id + nama, boleh dibaca pemegang izin master
		// itu atau izin lihat item — pegawai yang mengelola item tidak otomatis boleh membuka halaman masternya.
		r.Route("/lookup", func(r chi.Router) {
			for _, k := range Kinds {
				perms := [][2]string{{k.Path, authz.ActView}, {ItemsModule, authz.ActView}, {"stock_opname", authz.ActCreate}}
				if k.Path == "categories" {
					// Kasir memfilter katalog per kategori tanpa izin halaman master.
					perms = append(perms, [2]string{"sales_orders", authz.ActCreate})
				}
				r.With(authz.RequireAny(perms...)).Get("/"+k.Path+"/", h.listSimple(k))
			}
			r.With(authz.RequireAny([2]string{SupplierModule, authz.ActView}, [2]string{ItemsModule, authz.ActView}, [2]string{"purchase_invoices", authz.ActCreate})).Get("/suppliers/", h.lookupSuppliers)
			// Kasir memilih salesman di nota tanpa izin halaman master.
			r.With(authz.RequireAny([2]string{SalespersonModule, authz.ActView}, [2]string{"sales_orders", authz.ActCreate})).Get("/salespeople/", h.lookupSalespeople)
		})
		r.Route("/salespeople", func(r chi.Router) {
			r.With(authz.Require(SalespersonModule, authz.ActView)).Get("/", h.listSalespeople)
			r.With(authz.Require(SalespersonModule, authz.ActCreate)).Post("/", h.createSalesperson)
			r.With(authz.Require(SalespersonModule, authz.ActUpdate)).Put("/{id}", h.updateSalesperson)
			r.With(authz.Require(SalespersonModule, authz.ActUpdate)).Put("/{id}/active", h.activeSalesperson)
		})
		r.Route("/suppliers", func(r chi.Router) {
			r.With(authz.Require(SupplierModule, authz.ActView)).Get("/", h.listSuppliers)
			r.With(authz.Require(SupplierModule, authz.ActCreate)).Post("/", h.createSupplier)
			r.With(authz.Require(SupplierModule, authz.ActUpdate)).Put("/{id}", h.updateSupplier)
			r.With(authz.Require(SupplierModule, authz.ActUpdate)).Put("/{id}/active", h.activeSupplier)
		})
	})
}

func actor(r *http.Request) authz.Actor {
	a, _ := authz.ActorFrom(r.Context())
	return a
}

type nameRequest struct {
	Name string `json:"name"`
}

type activeRequest struct {
	Active *bool `json:"active"`
}

type supplierRequest struct {
	Code        string `json:"code"`
	Name        string `json:"name"`
	ContactName string `json:"contact_name"`
	Phone       string `json:"phone"`
	Email       string `json:"email"`
	Address     string `json:"address"`
	Note        string `json:"note"`
}

func (s supplierRequest) input() SupplierInput {
	return SupplierInput{Code: s.Code, Name: s.Name, ContactName: s.ContactName, Phone: s.Phone, Email: s.Email, Address: s.Address, Note: s.Note}
}

// listParams membaca ?q=&active=true|false&limit=&offset= (active kosong/"all" = semua). Angka rusak diabaikan.
func listParams(r *http.Request) ListParams {
	qs := r.URL.Query()
	p := ListParams{Q: qs.Get("q")}
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
	return p
}

func pathID(w http.ResponseWriter, r *http.Request) (uuid.UUID, bool) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		httpx.Error(w, http.StatusNotFound, "NOT_FOUND", "Data tidak ditemukan.")
		return uuid.Nil, false
	}
	return id, true
}

func (h *Handler) listSimple(k Kind) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		list, total, err := h.svc.List(r.Context(), actor(r), k, listParams(r))
		if err != nil {
			h.fail(w, r, err)
			return
		}
		httpx.JSON(w, http.StatusOK, map[string]any{"data": list, "total": total})
	}
}

func (h *Handler) createSimple(k Kind) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req nameRequest
		if !httpx.DecodeJSON(w, r, &req) {
			return
		}
		e, err := h.svc.Create(r.Context(), actor(r), k, req.Name)
		if err != nil {
			h.fail(w, r, err)
			return
		}
		httpx.JSON(w, http.StatusCreated, e)
	}
}

func (h *Handler) renameSimple(k Kind) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, ok := pathID(w, r)
		if !ok {
			return
		}
		var req nameRequest
		if !httpx.DecodeJSON(w, r, &req) {
			return
		}
		e, err := h.svc.Rename(r.Context(), actor(r), k, id, req.Name)
		if err != nil {
			h.fail(w, r, err)
			return
		}
		httpx.JSON(w, http.StatusOK, e)
	}
}

func (h *Handler) activeSimple(k Kind) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, ok := pathID(w, r)
		if !ok {
			return
		}
		var req activeRequest
		if !httpx.DecodeJSON(w, r, &req) {
			return
		}
		if req.Active == nil {
			httpx.ValidationError(w, map[string]string{"active": "REQUIRED"})
			return
		}
		e, err := h.svc.SetActive(r.Context(), actor(r), k, id, *req.Active)
		if err != nil {
			h.fail(w, r, err)
			return
		}
		httpx.JSON(w, http.StatusOK, e)
	}
}

func (h *Handler) listSuppliers(w http.ResponseWriter, r *http.Request) {
	list, total, err := h.svc.ListSuppliers(r.Context(), actor(r), listParams(r))
	if err != nil {
		h.fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"data": list, "total": total})
}

// lookupSuppliers = daftar supplier ringkas (tanpa telepon/alamat/catatan) untuk combobox.
func (h *Handler) lookupSuppliers(w http.ResponseWriter, r *http.Request) {
	list, total, err := h.svc.ListSuppliers(r.Context(), actor(r), listParams(r))
	if err != nil {
		h.fail(w, r, err)
		return
	}
	out := make([]Entry, 0, len(list))
	for _, s := range list {
		out = append(out, Entry{ID: s.ID, Name: s.Name, Active: s.Active, CreatedAt: s.CreatedAt})
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"data": out, "total": total})
}

func (h *Handler) createSupplier(w http.ResponseWriter, r *http.Request) {
	var req supplierRequest
	if !httpx.DecodeJSON(w, r, &req) {
		return
	}
	s, err := h.svc.CreateSupplier(r.Context(), actor(r), req.input())
	if err != nil {
		h.fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusCreated, s)
}

func (h *Handler) updateSupplier(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	var req supplierRequest
	if !httpx.DecodeJSON(w, r, &req) {
		return
	}
	s, err := h.svc.UpdateSupplier(r.Context(), actor(r), id, req.input())
	if err != nil {
		h.fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, s)
}

func (h *Handler) activeSupplier(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	var req activeRequest
	if !httpx.DecodeJSON(w, r, &req) {
		return
	}
	if req.Active == nil {
		httpx.ValidationError(w, map[string]string{"active": "REQUIRED"})
		return
	}
	s, err := h.svc.SetSupplierActive(r.Context(), actor(r), id, *req.Active)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, s)
}

func (h *Handler) fail(w http.ResponseWriter, r *http.Request, err error) {
	var fields FieldErrors
	switch {
	case errors.As(err, &fields):
		httpx.ValidationError(w, fields)
	case errors.Is(err, ErrNotFound):
		httpx.Error(w, http.StatusNotFound, "NOT_FOUND", "Data tidak ditemukan.")
	case errors.Is(err, ErrNameTaken):
		httpx.Error(w, http.StatusConflict, "NAME_TAKEN", "Nama sudah dipakai.")
	case errors.Is(err, ErrCodeTaken):
		httpx.Error(w, http.StatusConflict, "CODE_TAKEN", "Kode sudah dipakai.")
	default:
		h.log.Error("katalog gagal", "err", err, "req_id", middleware.GetReqID(r.Context()))
		httpx.Error(w, http.StatusInternalServerError, "INTERNAL", "Terjadi kesalahan pada server.")
	}
}

type salespersonRequest struct {
	Code          string      `json:"code"`
	Name          string      `json:"name"`
	Phone         string      `json:"phone"`
	Note          string      `json:"note"`
	CommissionPct json.Number `json:"commission_pct"`
}

func (s salespersonRequest) input() SalespersonInput {
	return SalespersonInput{Code: s.Code, Name: s.Name, Phone: s.Phone, Note: s.Note, CommissionPct: s.CommissionPct}
}

func (h *Handler) listSalespeople(w http.ResponseWriter, r *http.Request) {
	list, total, err := h.svc.ListSalespeople(r.Context(), actor(r), listParams(r))
	if err != nil {
		h.fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"data": list, "total": total})
}

// lookupSalespeople = id + nama saja (tanpa telepon/komisi) untuk pemilih di kasir.
func (h *Handler) lookupSalespeople(w http.ResponseWriter, r *http.Request) {
	list, total, err := h.svc.ListSalespeople(r.Context(), actor(r), listParams(r))
	if err != nil {
		h.fail(w, r, err)
		return
	}
	out := make([]Entry, 0, len(list))
	for _, s := range list {
		out = append(out, Entry{ID: s.ID, Name: s.Name, Active: s.Active, CreatedAt: s.CreatedAt})
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"data": out, "total": total})
}

func (h *Handler) createSalesperson(w http.ResponseWriter, r *http.Request) {
	var req salespersonRequest
	if !httpx.DecodeJSON(w, r, &req) {
		return
	}
	s, err := h.svc.CreateSalesperson(r.Context(), actor(r), req.input())
	if err != nil {
		h.fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusCreated, s)
}

func (h *Handler) updateSalesperson(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	var req salespersonRequest
	if !httpx.DecodeJSON(w, r, &req) {
		return
	}
	s, err := h.svc.UpdateSalesperson(r.Context(), actor(r), id, req.input())
	if err != nil {
		h.fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, s)
}

func (h *Handler) activeSalesperson(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	var req activeRequest
	if !httpx.DecodeJSON(w, r, &req) {
		return
	}
	if req.Active == nil {
		httpx.ValidationError(w, map[string]string{"active": "REQUIRED"})
		return
	}
	s, err := h.svc.SetSalespersonActive(r.Context(), actor(r), id, *req.Active)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, s)
}
