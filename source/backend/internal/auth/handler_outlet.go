package auth

import (
	"errors"
	"net/http"

	"github.com/google/uuid"

	"aciraba/internal/platform/httpx"
)

type switchRequest struct {
	OutletID string `json:"outlet_id"`
}

// SwitchOutlet memindahkan sesi ke outlet lain dan mengembalikan token akses baru (cookie refresh tidak berubah).
func (h *Handler) SwitchOutlet(w http.ResponseWriter, r *http.Request) {
	var req switchRequest
	if !httpx.DecodeJSON(w, r, &req) {
		return
	}
	outletID, err := uuid.Parse(req.OutletID)
	if err != nil {
		httpx.ValidationError(w, map[string]string{"outlet_id": "INVALID"})
		return
	}
	c, err := r.Cookie(refreshCookie)
	if err != nil || c.Value == "" {
		httpx.Error(w, http.StatusUnauthorized, "SESSION_INVALID", "Sesi tidak ditemukan.")
		return
	}
	sess, err := h.svc.SwitchOutlet(r.Context(), actor(r), c.Value, outletID)
	switch {
	case errors.Is(err, ErrInvalidSession):
		httpx.Error(w, http.StatusUnauthorized, "SESSION_INVALID", "Sesi tidak valid. Silakan masuk kembali.")
	case errors.Is(err, ErrOutletForbidden):
		httpx.Error(w, http.StatusForbidden, "OUTLET_FORBIDDEN", "Anda tidak memiliki akses ke outlet ini.")
	case err != nil:
		h.internal(w, r, "pindah outlet gagal", err)
	default:
		httpx.JSON(w, http.StatusOK, sess)
	}
}
