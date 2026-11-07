package platformadmin

// "API ARUS" untuk akun gratis HalloArus: base URL + model + API key diatur Platform Admin (lihat internal/ai/platform_provider.go dan
// migrasi 00099). Key tidak pernah dikembalikan ke panel (hanya 4 karakter terakhir). Penulisan lewat fungsi DB yang memverifikasi admin.

import (
	"errors"
	"net/http"
	"net/url"

	"aciraba/internal/ai"
	"aciraba/internal/platform/httpx"
)

const (
	ActionAIProviderSave  = "platform.ai_provider_save"
	ActionAIProviderClear = "platform.ai_provider_clear"
)

func (h *Handler) aiProviderOff(w http.ResponseWriter) bool {
	if h.AI == nil {
		httpx.Error(w, http.StatusServiceUnavailable, "AI_UNAVAILABLE", "Pengaturan API ARUS tidak tersedia.")
		return true
	}
	return false
}

func (h *Handler) GetAIProvider(w http.ResponseWriter, r *http.Request) {
	if h.aiProviderOff(w) {
		return
	}
	p, err := h.AI.GetPlatformProvider(r.Context(), actor(r).ID)
	if err != nil {
		h.planErr(w, r, "baca API ARUS gagal", mapPolicyErr(err))
		return
	}
	httpx.JSON(w, http.StatusOK, p)
}

type aiProviderRequest struct {
	BaseURL string `json:"base_url"`
	Model   string `json:"model"`
	APIKey  string `json:"api_key"`
}

func (h *Handler) SaveAIProvider(w http.ResponseWriter, r *http.Request) {
	if h.aiProviderOff(w) {
		return
	}
	var req aiProviderRequest
	if !httpx.DecodeJSON(w, r, &req) {
		return
	}
	a := actor(r)
	err := h.AI.SavePlatformProvider(r.Context(), a.ID, ai.PlatformProviderInput{BaseURL: req.BaseURL, Model: req.Model, APIKey: req.APIKey})
	switch {
	case err == nil:
		host := ""
		if u, e := url.Parse(req.BaseURL); e == nil {
			host = u.Host
		}
		h.svc.record(r.Context(), a, ActionAIProviderSave, [16]byte{}, "", map[string]any{"base_host": host, "model": req.Model, "key_changed": req.APIKey != ""})
		httpx.JSON(w, http.StatusOK, map[string]bool{"ok": true})
	case errors.Is(err, ai.ErrBadBaseURL):
		httpx.Error(w, http.StatusUnprocessableEntity, "AI_BASE_URL_INVALID", "Base URL tidak diizinkan: gunakan alamat https publik.")
	case errors.Is(err, ai.ErrBadKey):
		httpx.Error(w, http.StatusUnprocessableEntity, "AI_KEY_INVALID", "API key ditolak oleh penyedia (atau wajib diisi saat base URL berubah).")
	case errors.Is(err, ai.ErrBadInput):
		httpx.Error(w, http.StatusUnprocessableEntity, "AI_BAD_INPUT", "Isian tidak valid.")
	default:
		if ai.IsUpstream(err) {
			httpx.Error(w, http.StatusBadGateway, "AI_UNAVAILABLE", "Gateway tidak dapat dihubungi atau menolak model/key.")
			return
		}
		h.planErr(w, r, "simpan API ARUS gagal", mapPolicyErr(err))
	}
}

func (h *Handler) DeleteAIProvider(w http.ResponseWriter, r *http.Request) {
	if h.aiProviderOff(w) {
		return
	}
	a := actor(r)
	found, err := h.AI.ClearPlatformProvider(r.Context(), a.ID)
	if err != nil {
		h.planErr(w, r, "hapus API ARUS gagal", mapPolicyErr(err))
		return
	}
	if found {
		h.svc.record(r.Context(), a, ActionAIProviderClear, [16]byte{}, "", nil)
	}
	w.WriteHeader(http.StatusNoContent)
}
