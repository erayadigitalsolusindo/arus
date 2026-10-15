package platformadmin

import (
	"errors"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"aciraba/internal/extapi"
	"aciraba/internal/platform/httpx"
)

const (
	ActionExtCredCreate = "platform.ext_cred_create"
	ActionExtCredUpdate = "platform.ext_cred_update"
	ActionExtCredRotate = "platform.ext_cred_rotate"
	ActionExtCredDelete = "platform.ext_cred_delete"
	ActionExtBrandSave  = "platform.ext_brand_save"
	ActionExtBrandDel   = "platform.ext_brand_delete"
)

const maxBrandBody = 400 << 10 // logo data URI sampai ±300 KB

func (h *Handler) extEnabled(w http.ResponseWriter) bool {
	if h.Ext == nil {
		httpx.Error(w, http.StatusNotFound, "NOT_FOUND", "Fitur API eksternal tidak aktif.")
		return false
	}
	return true
}

func (h *Handler) extErr(w http.ResponseWriter, r *http.Request, msg string, err error) {
	switch {
	case errors.Is(err, extapi.ErrNotFound):
		httpx.Error(w, http.StatusNotFound, "NOT_FOUND", "Tidak ditemukan.")
	case errors.Is(err, extapi.ErrInvalid):
		httpx.Error(w, http.StatusUnprocessableEntity, "VALIDATION", "Data tidak valid (periksa IP/CIDR, akun layanan, dan outlet).")
	case errors.Is(err, extapi.ErrTaken):
		httpx.Error(w, http.StatusConflict, "TAKEN", "Slug atau domain sudah dipakai profil lain.")
	case errors.Is(err, extapi.ErrDefault):
		httpx.Error(w, http.StatusConflict, "PROTECTED", "Profil bawaan tidak dapat dihapus.")
	default:
		h.internal(w, r, msg, err)
	}
}

// ---- kredensial ----

func (h *Handler) ExtListCredentials(w http.ResponseWriter, r *http.Request) {
	if !h.extEnabled(w) {
		return
	}
	var tenant *uuid.UUID
	if q := r.URL.Query().Get("tenant_id"); q != "" {
		id, err := uuid.Parse(q)
		if err != nil {
			httpx.ValidationError(w, map[string]string{"tenant_id": "INVALID"})
			return
		}
		tenant = &id
	}
	list, err := h.Ext.ListCredentials(r.Context(), actor(r).ID, tenant)
	if err != nil {
		h.extErr(w, r, "daftar kredensial gagal", err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"items": list})
}

func (h *Handler) ExtTenantActors(w http.ResponseWriter, r *http.Request) {
	if !h.extEnabled(w) {
		return
	}
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	raw, err := h.Ext.TenantActors(r.Context(), actor(r).ID, id)
	if err != nil {
		h.extErr(w, r, "pilihan akun layanan gagal", err)
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_, _ = w.Write(raw)
}

type extCredCreateRequest struct {
	TenantID    uuid.UUID  `json:"tenant_id"`
	UserID      uuid.UUID  `json:"user_id"`
	OutletID    uuid.UUID  `json:"outlet_id"`
	Label       string     `json:"label"`
	IPAllowlist []string   `json:"ip_allowlist"`
	RateLimit   int        `json:"rate_limit_min"`
	ExpiresAt   *time.Time `json:"expires_at"`
}

func (h *Handler) ExtCreateCredential(w http.ResponseWriter, r *http.Request) {
	if !h.extEnabled(w) {
		return
	}
	var req extCredCreateRequest
	if !httpx.DecodeJSON(w, r, &req) {
		return
	}
	if req.RateLimit == 0 {
		req.RateLimit = 120
	}
	if req.ExpiresAt != nil && !req.ExpiresAt.After(time.Now()) {
		httpx.ValidationError(w, map[string]string{"expires_at": "PAST"})
		return
	}
	a := actor(r)
	created, err := h.Ext.CreateCredential(r.Context(), a.ID, extapi.CredentialInput{
		TenantID: req.TenantID, UserID: req.UserID, OutletID: req.OutletID, Label: req.Label,
		IPAllowlist: req.IPAllowlist, RateLimit: req.RateLimit, ExpiresAt: req.ExpiresAt,
	})
	if err != nil {
		h.extErr(w, r, "buat kredensial gagal", err)
		return
	}
	h.svc.record(r.Context(), a, ActionExtCredCreate, req.TenantID, "", map[string]any{
		"key_id": created.KeyID, "label": req.Label, "ip_allowlist": req.IPAllowlist, "user_id": req.UserID, "outlet_id": req.OutletID,
	})
	httpx.JSON(w, http.StatusCreated, created) // satu-satunya kali secret terlihat
}

type extCredUpdateRequest struct {
	Label       string     `json:"label"`
	IPAllowlist []string   `json:"ip_allowlist"`
	RateLimit   int        `json:"rate_limit_min"`
	Active      bool       `json:"active"`
	ExpiresAt   *time.Time `json:"expires_at"`
}

func (h *Handler) ExtUpdateCredential(w http.ResponseWriter, r *http.Request) {
	if !h.extEnabled(w) {
		return
	}
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	var req extCredUpdateRequest
	if !httpx.DecodeJSON(w, r, &req) {
		return
	}
	a := actor(r)
	if err := h.Ext.UpdateCredential(r.Context(), a.ID, id, extapi.CredentialUpdate{
		Label: req.Label, IPAllowlist: req.IPAllowlist, RateLimit: req.RateLimit, Active: req.Active, ExpiresAt: req.ExpiresAt,
	}); err != nil {
		h.extErr(w, r, "ubah kredensial gagal", err)
		return
	}
	h.svc.record(r.Context(), a, ActionExtCredUpdate, uuid.Nil, "", map[string]any{
		"id": id, "label": req.Label, "ip_allowlist": req.IPAllowlist, "active": req.Active, "rate_limit_min": req.RateLimit,
	})
	w.WriteHeader(http.StatusNoContent)
}

type extRotateRequest struct {
	GraceMinutes int `json:"grace_minutes"` // 0 = secret lama langsung mati
}

func (h *Handler) ExtRotateCredential(w http.ResponseWriter, r *http.Request) {
	if !h.extEnabled(w) {
		return
	}
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	var req extRotateRequest
	if !httpx.DecodeJSON(w, r, &req) {
		return
	}
	if req.GraceMinutes < 0 || req.GraceMinutes > 7*24*60 {
		httpx.ValidationError(w, map[string]string{"grace_minutes": "INVALID"})
		return
	}
	a := actor(r)
	list, err := h.Ext.ListCredentials(r.Context(), a.ID, nil)
	if err != nil {
		h.extErr(w, r, "rotasi kredensial gagal", err)
		return
	}
	keyID := ""
	for _, c := range list {
		if c.ID == id {
			keyID = c.KeyID
		}
	}
	if keyID == "" {
		httpx.Error(w, http.StatusNotFound, "NOT_FOUND", "Tidak ditemukan.")
		return
	}
	created, err := h.Ext.RotateCredential(r.Context(), a.ID, id, keyID, time.Duration(req.GraceMinutes)*time.Minute)
	if err != nil {
		h.extErr(w, r, "rotasi kredensial gagal", err)
		return
	}
	h.svc.record(r.Context(), a, ActionExtCredRotate, uuid.Nil, "", map[string]any{"id": id, "key_id": keyID, "grace_minutes": req.GraceMinutes})
	httpx.JSON(w, http.StatusOK, created)
}

func (h *Handler) ExtDeleteCredential(w http.ResponseWriter, r *http.Request) {
	if !h.extEnabled(w) {
		return
	}
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	a := actor(r)
	if err := h.Ext.DeleteCredential(r.Context(), a.ID, id); err != nil {
		h.extErr(w, r, "hapus kredensial gagal", err)
		return
	}
	h.svc.record(r.Context(), a, ActionExtCredDelete, uuid.Nil, "", map[string]any{"id": id})
	w.WriteHeader(http.StatusNoContent)
}

// ---- white label ----

func (h *Handler) ExtListBrands(w http.ResponseWriter, r *http.Request) {
	if !h.extEnabled(w) {
		return
	}
	list, err := h.Ext.ListBrands(r.Context(), actor(r).ID)
	if err != nil {
		h.extErr(w, r, "daftar profil gagal", err)
		return
	}
	groups := []extapi.Group{}
	if h.ExtCatalog != nil {
		groups = h.ExtCatalog.Get().Groups
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"items": list, "groups": groups})
}

type extBrandRequest struct {
	Slug          string   `json:"slug"`
	Domain        string   `json:"domain"`
	IsDefault     bool     `json:"is_default"`
	Active        bool     `json:"active"`
	Name          string   `json:"name"`
	Tagline       string   `json:"tagline"`
	LogoURL       string   `json:"logo_url"`
	Accent        string   `json:"accent"`
	SupportEmail  string   `json:"support_email"`
	SupportURL    string   `json:"support_url"`
	Footer        string   `json:"footer"`
	BaseURL       string   `json:"base_url"`
	HiddenModules []string `json:"hidden_modules"`
	SandboxNote   string   `json:"sandbox_note"`
}

func (h *Handler) extSaveBrand(w http.ResponseWriter, r *http.Request, id *uuid.UUID) {
	if !h.extEnabled(w) {
		return
	}
	var req extBrandRequest
	if !httpx.DecodeJSONLimit(w, r, &req, maxBrandBody) {
		return
	}
	in := extapi.BrandInput{
		ID: id, Slug: req.Slug, Domain: req.Domain, IsDefault: req.IsDefault, Active: req.Active, Name: req.Name, Tagline: req.Tagline,
		LogoURL: req.LogoURL, Accent: req.Accent, SupportEmail: req.SupportEmail, SupportURL: req.SupportURL, Footer: req.Footer,
		BaseURL: req.BaseURL, HiddenModules: req.HiddenModules, SandboxNote: req.SandboxNote,
	}
	if f := in.Validate(); len(f) > 0 {
		httpx.ValidationError(w, f)
		return
	}
	if in.IsDefault && !in.Active {
		httpx.ValidationError(w, map[string]string{"active": "DEFAULT_MUST_BE_ACTIVE"})
		return
	}
	a := actor(r)
	saved, err := h.Ext.SaveBrand(r.Context(), a.ID, in)
	if err != nil {
		h.extErr(w, r, "simpan profil gagal", err)
		return
	}
	h.svc.record(r.Context(), a, ActionExtBrandSave, uuid.Nil, "", map[string]any{"id": saved, "slug": in.Slug, "domain": in.Domain, "hidden_modules": in.HiddenModules})
	status := http.StatusOK
	if id == nil {
		status = http.StatusCreated
	}
	httpx.JSON(w, status, map[string]any{"id": saved})
}

func (h *Handler) ExtCreateBrand(w http.ResponseWriter, r *http.Request) { h.extSaveBrand(w, r, nil) }

func (h *Handler) ExtUpdateBrand(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	h.extSaveBrand(w, r, &id)
}

func (h *Handler) ExtDeleteBrand(w http.ResponseWriter, r *http.Request) {
	if !h.extEnabled(w) {
		return
	}
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		httpx.Error(w, http.StatusNotFound, "NOT_FOUND", "Tidak ditemukan.")
		return
	}
	a := actor(r)
	if err := h.Ext.DeleteBrand(r.Context(), a.ID, id); err != nil {
		h.extErr(w, r, "hapus profil gagal", err)
		return
	}
	h.svc.record(r.Context(), a, ActionExtBrandDel, uuid.Nil, "", map[string]any{"id": id})
	w.WriteHeader(http.StatusNoContent)
}
