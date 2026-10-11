package shift

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"aciraba/internal/platform/httpx"
)

// AddMovement: POST /shifts/petty-cash (Idempotency-Key wajib) {direction, amount, category, note, approval}.
// Mengembalikan catatan + shift terbaru (rekap berjalan) agar klien langsung menyegarkan tampilan.
func (h *Handler) AddMovement(w http.ResponseWriter, r *http.Request) {
	var in MovementInput
	if !httpx.DecodeJSONLimit(w, r, &in, 8<<10) {
		return
	}
	m, err := h.svc.AddMovement(r.Context(), actor(r), r.Header.Get("Idempotency-Key"), in)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusCreated, m)
}

// VoidMovement: POST /shifts/petty-cash/{id}/void.
func (h *Handler) VoidMovement(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		httpx.Error(w, http.StatusNotFound, "NOT_FOUND", "Data tidak ditemukan.")
		return
	}
	if err := h.svc.VoidMovement(r.Context(), actor(r), id); err != nil {
		h.fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"ok": true})
}

// SetPettyCashLimit: PUT /shifts/petty-cash/limit {limit}.
func (h *Handler) SetPettyCashLimit(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Limit json.Number `json:"limit"`
	}
	if !httpx.DecodeJSONLimit(w, r, &in, 1<<10) {
		return
	}
	v, err := h.svc.SetPettyCashLimit(r.Context(), actor(r), in.Limit)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"limit": v})
}
