package auth

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net/http"
	"time"

	pauth "aciraba/internal/platform/auth"
	"aciraba/internal/platform/httpx"
	"aciraba/internal/platform/sanitize"
)

type forgotRequest struct {
	Email string `json:"email"`
}

// ForgotPassword selalu menjawab 202 dengan isi sama, apa pun hasilnya (anti-enumerasi akun). Pekerjaan sebenarnya
// berjalan di latar belakang. Batas per email tidak mengubah respons; hanya mengabaikan permintaan berlebih.
func (h *Handler) ForgotPassword(w http.ResponseWriter, r *http.Request) {
	var req forgotRequest
	if !httpx.DecodeJSON(w, r, &req) {
		return
	}
	if email, code := sanitize.Email(req.Email); code == "" {
		sum := sha256.Sum256([]byte(email))
		ok, _, err := httpx.Allow(r.Context(), h.Redis, "rl:forgot-email:"+hex.EncodeToString(sum[:]), h.RateLimits.Get(r.Context()).ForgotPerEmail, time.Hour)
		if err == nil && ok {
			h.svc.RequestPasswordReset(email, lang(r))
		}
	}
	w.WriteHeader(http.StatusAccepted)
}

type resetRequest struct {
	Token    string `json:"token"`
	Password string `json:"password"`
}

func (h *Handler) ResetPassword(w http.ResponseWriter, r *http.Request) {
	var req resetRequest
	if !httpx.DecodeJSON(w, r, &req) {
		return
	}
	if len(req.Password) > maxPassword {
		httpx.ValidationError(w, map[string]string{"password": "TOO_LONG"})
		return
	}
	err := h.svc.ResetPassword(r.Context(), req.Token, req.Password)
	var fields FieldErrors
	switch {
	case errors.As(err, &fields):
		httpx.ValidationError(w, fields)
	case errors.Is(err, pauth.ErrInvalidToken):
		httpx.Error(w, http.StatusBadRequest, "INVALID_TOKEN", "Tautan tidak valid atau sudah kedaluwarsa.")
	case err != nil:
		h.internal(w, r, "reset password gagal", err)
	default:
		w.WriteHeader(http.StatusNoContent)
	}
}
