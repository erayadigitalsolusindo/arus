package auth

import (
	"errors"
	"net/http"

	"github.com/google/uuid"

	"aciraba/internal/approval"
	"aciraba/internal/platform/httpx"
)

type switchRequest struct {
	OutletID string `json:"outlet_id"`
	// RefreshToken hanya dipakai klien native (X-Client: mobile); klien web memakai cookie.
	RefreshToken string `json:"refresh_token"`
	// POS=true: dari layar kasir → wajib persetujuan PIN Owner/Supervisor (kecuali pelaku sendiri penyetuju).
	POS      bool        `json:"pos"`
	Approval *ApprovalIn `json:"approval"`
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
	token := req.RefreshToken
	if !isNative(r) {
		if c, cerr := r.Cookie(refreshCookie); cerr == nil {
			token = c.Value
		} else {
			token = ""
		}
	}
	if token == "" {
		httpx.Error(w, http.StatusUnauthorized, "SESSION_INVALID", "Sesi tidak ditemukan.")
		return
	}
	sess, err := h.svc.SwitchOutletWith(r.Context(), actor(r), token, outletID, SwitchOpts{POS: req.POS, Approval: req.Approval})
	if approval.Fail(w, err) {
		return
	}
	switch {
	case errors.Is(err, ErrInvalidSession):
		httpx.Error(w, http.StatusUnauthorized, "SESSION_INVALID", "Sesi tidak valid. Silakan masuk kembali.")
	case errors.Is(err, ErrOutletForbidden):
		httpx.Error(w, http.StatusForbidden, "OUTLET_FORBIDDEN", "Anda tidak memiliki akses ke outlet ini.")
	case err != nil:
		h.internal(w, r, "pindah outlet gagal", err)
	default:
		h.writeSession(w, r, http.StatusOK, sess)
	}
}
