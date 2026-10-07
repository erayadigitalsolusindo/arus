package auth

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net/http"
	"time"

	"aciraba/internal/platform/httpx"
	"aciraba/internal/platform/sanitize"
)

// registerRequest memakai tipe string/bool saja; tipe lain (angka/objek) ditolak oleh decoder JSON.
type registerRequest struct {
	BusinessName string `json:"business_name"`
	OwnerName    string `json:"owner_name"`
	Email        string `json:"email"`
	Phone        string `json:"phone"`
	OutletName   string `json:"outlet_name"`
	Password     string `json:"password"`
	AcceptTerms  bool   `json:"accept_terms"`
}

func (h *Handler) Register(w http.ResponseWriter, r *http.Request) {
	var req registerRequest
	if !httpx.DecodeJSON(w, r, &req) {
		return
	}
	clean, fields := ValidateRegister(RegisterInput(req))
	if fields != nil {
		httpx.ValidationError(w, fields)
		return
	}

	sess, err := h.svc.Register(r.Context(), clean, lang(r))
	switch {
	case errors.Is(err, ErrEmailTaken):
		httpx.Error(w, http.StatusConflict, "EMAIL_TAKEN", "Email sudah terdaftar.")
		return
	case err != nil:
		h.internal(w, r, "register gagal", err)
		return
	}
	h.setRefreshCookie(w, sess)
	httpx.JSON(w, http.StatusCreated, sess)
}

type loginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
	Remember bool   `json:"remember"`
}

func (h *Handler) Login(w http.ResponseWriter, r *http.Request) {
	var req loginRequest
	if !httpx.DecodeJSON(w, r, &req) {
		return
	}
	// Input yang mustahil valid dijawab sama dengan kredensial salah: tidak membocorkan aturan format.
	email, code := sanitize.Email(req.Email)
	if code != "" || req.Password == "" || len(req.Password) > maxPassword {
		httpx.Error(w, http.StatusUnauthorized, "INVALID_CREDENTIALS", "Email atau password salah.")
		return
	}

	sum := sha256.Sum256([]byte(email))
	emailHash, ip := hex.EncodeToString(sum[:]), httpx.ClientIP(r)
	locked, err := h.Lockout.Check(r.Context(), ip, emailHash)
	if err != nil {
		httpx.Error(w, http.StatusServiceUnavailable, "UNAVAILABLE", "Layanan sementara tidak tersedia.")
		return
	}
	if locked > 0 {
		h.locked(w, locked)
		return
	}

	sess, err := h.svc.Login(r.Context(), LoginInput{Email: email, Password: req.Password, Remember: req.Remember})
	switch {
	case errors.Is(err, ErrInvalidCredentials):
		// Gagal dihitung; bila percobaan ini memicu kunci, jawab langsung dengan sisa waktunya.
		res, lerr := h.Lockout.RecordFailure(r.Context(), ip, emailHash)
		if lerr != nil {
			h.Log.Error("catat gagal login", "err", lerr)
		} else if res.LockedFor > 0 {
			h.locked(w, res.LockedFor)
			return
		}
		// AttemptsLeft memberi tahu pengguna sisa percobaan sebelum akun dikunci (0 = tidak diketahui, tidak dikirim).
		httpx.ErrorAttempts(w, http.StatusUnauthorized, "INVALID_CREDENTIALS", "Email atau password salah.", res.AttemptsLeft)
		return
	case errors.Is(err, ErrAccountDisabled):
		httpx.Error(w, http.StatusForbidden, "ACCOUNT_DISABLED", "Akun dinonaktifkan. Hubungi administrator.")
		return
	case errors.Is(err, ErrNoOutlet):
		httpx.Error(w, http.StatusForbidden, "NO_OUTLET", "Akun belum memiliki outlet aktif. Hubungi administrator.")
		return
	case err != nil:
		h.internal(w, r, "login gagal", err)
		return
	}
	if err := h.Lockout.Reset(r.Context(), ip, emailHash); err != nil {
		h.Log.Error("reset kunci login", "err", err)
	}
	h.setRefreshCookie(w, sess)
	httpx.JSON(w, http.StatusOK, sess)
}

func (h *Handler) locked(w http.ResponseWriter, d time.Duration) {
	httpx.Retry(w, "ACCOUNT_LOCKED", "Terlalu banyak percobaan gagal. Coba lagi nanti.", d)
}

func (h *Handler) Refresh(w http.ResponseWriter, r *http.Request) {
	c, err := r.Cookie(refreshCookie)
	if err != nil || c.Value == "" {
		httpx.Error(w, http.StatusUnauthorized, "SESSION_INVALID", "Sesi tidak ditemukan.")
		return
	}
	sess, err := h.svc.Refresh(r.Context(), c.Value)
	switch {
	case errors.Is(err, ErrInvalidSession):
		h.clearRefreshCookie(w)
		httpx.Error(w, http.StatusUnauthorized, "SESSION_INVALID", "Sesi berakhir. Silakan masuk kembali.")
		return
	case err != nil:
		h.internal(w, r, "refresh gagal", err)
		return
	}
	h.setRefreshCookie(w, sess)
	httpx.JSON(w, http.StatusOK, sess)
}

func (h *Handler) Logout(w http.ResponseWriter, r *http.Request) {
	h.clearRefreshCookie(w) // sebelum menulis status: header Set-Cookie harus terkirim bersama respons
	if c, err := r.Cookie(refreshCookie); err == nil && c.Value != "" {
		if err := h.svc.Logout(r.Context(), c.Value); err != nil {
			h.internal(w, r, "logout gagal", err)
			return
		}
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) Me(w http.ResponseWriter, r *http.Request) {
	p, err := h.svc.Me(r.Context(), actor(r))
	switch {
	case errors.Is(err, ErrInvalidSession):
		httpx.Error(w, http.StatusUnauthorized, "UNAUTHORIZED", "Sesi tidak valid.")
		return
	case err != nil:
		h.internal(w, r, "me gagal", err)
		return
	}
	httpx.JSON(w, http.StatusOK, p)
}
