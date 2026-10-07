package auth

import (
	"errors"
	"net/http"
	"time"

	pauth "aciraba/internal/platform/auth"
	"aciraba/internal/platform/httpx"
)

type verifyRequest struct {
	Token string `json:"token"`
}

func (h *Handler) VerifyEmail(w http.ResponseWriter, r *http.Request) {
	var req verifyRequest
	if !httpx.DecodeJSON(w, r, &req) {
		return
	}
	err := h.svc.VerifyEmail(r.Context(), req.Token)
	switch {
	case errors.Is(err, pauth.ErrInvalidToken):
		httpx.Error(w, http.StatusBadRequest, "INVALID_TOKEN", "Tautan tidak valid atau sudah kedaluwarsa.")
	case err != nil:
		h.internal(w, r, "verifikasi email gagal", err)
	default:
		w.WriteHeader(http.StatusNoContent)
	}
}

func (h *Handler) ResendVerification(w http.ResponseWriter, r *http.Request) {
	a := actor(r)
	ok, wait, err := httpx.Allow(r.Context(), h.Redis, "rl:resend-verify:"+a.UserID.String(), h.RateLimits.Get(r.Context()).ResendVerificationPer, time.Hour)
	if err != nil {
		httpx.Error(w, http.StatusServiceUnavailable, "UNAVAILABLE", "Layanan sementara tidak tersedia.")
		return
	}
	if !ok {
		httpx.TooManyRequests(w, wait)
		return
	}
	switch err := h.svc.ResendVerification(r.Context(), a, lang(r)); {
	case errors.Is(err, ErrInvalidSession):
		httpx.Error(w, http.StatusUnauthorized, "UNAUTHORIZED", "Sesi tidak valid.")
	case err != nil:
		h.internal(w, r, "kirim ulang verifikasi gagal", err)
	default:
		w.WriteHeader(http.StatusAccepted)
	}
}
