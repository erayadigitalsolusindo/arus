package platformadmin

import (
	"errors"
	"net/http"
	"strings"

	"github.com/google/uuid"

	"aciraba/internal/platform/httpx"
	"aciraba/internal/storefront"
)

// Pustaka template landing page: Platform Admin membuat/ubah/hapus template yang bisa dipakai semua tenant sebagai titik awal.
// Isi disaring dengan penyaring yang sama dengan halaman tenant (tanpa script, CSS tanpa url()/@import).

const (
	ActionStorefrontTemplateSave   = "platform.storefront_template_save"
	ActionStorefrontTemplateDelete = "platform.storefront_template_delete"
)

type storefrontTemplateRequest struct {
	Code        string `json:"code"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Category    string `json:"category"`
	HTML        string `json:"html"`
	CSS         string `json:"css"`
	Active      bool   `json:"active"`
	SortOrder   int    `json:"sort_order"`
}

func (h *Handler) templatesErr(w http.ResponseWriter, r *http.Request, msg string, err error) {
	var fe storefront.FieldErrors
	switch {
	case errors.As(err, &fe):
		httpx.ValidationError(w, fe)
	case errors.Is(err, storefront.ErrTemplateCodeTaken):
		httpx.Error(w, http.StatusConflict, "CODE_TAKEN", "Kode template sudah terpakai.")
	case errors.Is(err, storefront.ErrNotFound):
		httpx.Error(w, http.StatusNotFound, "NOT_FOUND", "Template tidak ditemukan.")
	case errors.Is(err, storefront.ErrInvalidAdmin):
		httpx.Error(w, http.StatusUnauthorized, "UNAUTHORIZED", "Sesi tidak valid.")
	default:
		h.internal(w, r, msg, err)
	}
}

func (h *Handler) templatesOn(w http.ResponseWriter) bool {
	if h.Storefront == nil {
		httpx.Error(w, http.StatusServiceUnavailable, "UNAVAILABLE", "Fitur template belum diaktifkan di server.")
		return false
	}
	return true
}

func (h *Handler) ListStorefrontTemplates(w http.ResponseWriter, r *http.Request) {
	if !h.templatesOn(w) {
		return
	}
	list, err := h.Storefront.ListTemplates(r.Context(), false, false)
	if err != nil {
		h.templatesErr(w, r, "daftar template gagal", err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"items": list})
}

func (h *Handler) GetStorefrontTemplate(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok || !h.templatesOn(w) {
		return
	}
	t, err := h.Storefront.GetTemplate(r.Context(), id, false)
	if err != nil {
		h.templatesErr(w, r, "baca template gagal", err)
		return
	}
	httpx.JSON(w, http.StatusOK, t)
}

func (h *Handler) saveStorefrontTemplate(w http.ResponseWriter, r *http.Request, id *uuid.UUID) {
	if !h.templatesOn(w) {
		return
	}
	var req storefrontTemplateRequest
	if !httpx.DecodeJSONLimit(w, r, &req, 200_000) {
		return
	}
	a := actor(r)
	tid, warnings, err := h.Storefront.SaveTemplate(r.Context(), a.ID, storefront.TemplateInput{
		ID: id, Code: strings.ToLower(strings.TrimSpace(req.Code)), Name: strings.TrimSpace(req.Name), Description: strings.TrimSpace(req.Description),
		Category: strings.TrimSpace(req.Category), HTML: req.HTML, CSS: req.CSS, Active: req.Active, Sort: req.SortOrder,
	})
	if err != nil {
		h.templatesErr(w, r, "simpan template gagal", err)
		return
	}
	h.svc.record(r.Context(), a, ActionStorefrontTemplateSave, uuid.Nil, "", map[string]any{"template_id": tid.String(), "code": req.Code, "name": req.Name, "active": req.Active, "html_bytes": len(req.HTML), "css_bytes": len(req.CSS), "create": id == nil})
	if warnings == nil {
		warnings = []string{}
	}
	status := http.StatusOK
	if id == nil {
		status = http.StatusCreated
	}
	httpx.JSON(w, status, map[string]any{"id": tid.String(), "warnings": warnings})
}

func (h *Handler) CreateStorefrontTemplate(w http.ResponseWriter, r *http.Request) {
	h.saveStorefrontTemplate(w, r, nil)
}

func (h *Handler) UpdateStorefrontTemplate(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	h.saveStorefrontTemplate(w, r, &id)
}

func (h *Handler) DeleteStorefrontTemplate(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok || !h.templatesOn(w) {
		return
	}
	a := actor(r)
	if err := h.Storefront.DeleteTemplate(r.Context(), a.ID, id); err != nil {
		h.templatesErr(w, r, "hapus template gagal", err)
		return
	}
	h.svc.record(r.Context(), a, ActionStorefrontTemplateDelete, uuid.Nil, "", map[string]any{"template_id": id.String()})
	w.WriteHeader(http.StatusNoContent)
}

// PreviewStorefrontTemplate merender templat dengan data contoh untuk iframe srcdoc ber-sandbox di panel.
func (h *Handler) PreviewStorefrontTemplate(w http.ResponseWriter, r *http.Request) {
	if !h.templatesOn(w) {
		return
	}
	var req storefrontTemplateRequest
	if !httpx.DecodeJSONLimit(w, r, &req, 200_000) {
		return
	}
	scheme := "http"
	if r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https" {
		scheme = "https"
	}
	doc, err := h.Storefront.PreviewSample(r.Context(), req.HTML, req.CSS, scheme+"://"+r.Host)
	if err != nil {
		h.templatesErr(w, r, "pratinjau template gagal", err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]string{"html": doc})
}
