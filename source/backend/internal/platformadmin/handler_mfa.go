package platformadmin

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net/http"

	"aciraba/internal/platform/httpx"
)

// requireMFA menolak (403 MFA_ENROLL_REQUIRED) admin yang belum mengaktifkan 2FA: mereka hanya boleh ke halaman keamanan.
func requireMFA(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if a, ok := ActorFrom(r.Context()); !ok || !a.MFA {
			httpx.Error(w, http.StatusForbidden, "MFA_ENROLL_REQUIRED", "Aktifkan 2FA terlebih dahulu.")
			return
		}
		next.ServeHTTP(w, r)
	})
}

// attempt membatasi percobaan kode/password per (IP, kunci). Bila terkunci, jawaban sudah ditulis dan ok=false.
// Pemanggil memanggil fail() saat salah dan done() saat benar.
func (h *Handler) attempt(w http.ResponseWriter, r *http.Request, key string) (fail func() bool, done func(), ok bool) {
	sum := sha256.Sum256([]byte(key))
	keyHash, ip := hex.EncodeToString(sum[:]), httpx.ClientIP(r)
	locked, err := h.Lockout.Check(r.Context(), ip, keyHash)
	if err != nil {
		httpx.Error(w, http.StatusServiceUnavailable, "UNAVAILABLE", "Layanan sementara tidak tersedia.")
		return nil, nil, false
	}
	if locked > 0 {
		httpx.Retry(w, "ACCOUNT_LOCKED", "Terlalu banyak percobaan gagal. Coba lagi nanti.", locked)
		return nil, nil, false
	}
	fail = func() bool { // true = jawaban sudah ditulis (terkunci)
		res, err := h.Lockout.RecordFailure(r.Context(), ip, keyHash)
		if err != nil {
			h.Log.Error("catat gagal 2FA", "err", err)
			return false
		}
		if res.LockedFor > 0 {
			httpx.Retry(w, "ACCOUNT_LOCKED", "Terlalu banyak percobaan gagal. Coba lagi nanti.", res.LockedFor)
			return true
		}
		return false
	}
	done = func() { _ = h.Lockout.Reset(r.Context(), ip, keyHash) }
	return fail, done, true
}

func (h *Handler) invalidCode(w http.ResponseWriter, fail func() bool) {
	if fail() {
		return
	}
	httpx.Error(w, http.StatusUnauthorized, "INVALID_CODE", "Kode 2FA salah.")
}

type mfaLoginRequest struct {
	MFAToken string `json:"mfa_token"`
	Code     string `json:"code"`
}

func (h *Handler) LoginMFA(w http.ResponseWriter, r *http.Request) {
	var req mfaLoginRequest
	if !httpx.DecodeJSON(w, r, &req) {
		return
	}
	if req.Code == "" || len(req.Code) > 32 || len(req.MFAToken) > 128 {
		httpx.Error(w, http.StatusUnauthorized, "INVALID_CODE", "Kode 2FA salah.")
		return
	}
	adminID, err := h.svc.MFAChallengeAdmin(r.Context(), req.MFAToken)
	if errors.Is(err, ErrInvalidMFAToken) {
		httpx.Error(w, http.StatusUnauthorized, "MFA_TOKEN_INVALID", "Sesi masuk berakhir. Masuk ulang.")
		return
	}
	if err != nil {
		h.internal(w, r, "tantangan 2FA gagal", err)
		return
	}
	fail, done, ok := h.attempt(w, r, "platform-mfa:"+adminID.String())
	if !ok {
		return
	}
	sess, err := h.svc.LoginMFA(r.Context(), req.MFAToken, req.Code)
	switch {
	case errors.Is(err, ErrInvalidCode):
		h.invalidCode(w, fail)
	case errors.Is(err, ErrInvalidMFAToken):
		httpx.Error(w, http.StatusUnauthorized, "MFA_TOKEN_INVALID", "Sesi masuk berakhir. Masuk ulang.")
	case err != nil:
		h.internal(w, r, "login 2FA gagal", err)
	default:
		done()
		h.setRefreshCookie(w, sess)
		httpx.JSON(w, http.StatusOK, sess)
	}
}

func (h *Handler) MFAStatus(w http.ResponseWriter, r *http.Request) {
	st, err := h.svc.MFAStatus(r.Context(), actor(r))
	if err != nil {
		h.internal(w, r, "status 2FA gagal", err)
		return
	}
	httpx.JSON(w, http.StatusOK, st)
}

func (h *Handler) MFABegin(w http.ResponseWriter, r *http.Request) {
	setup, err := h.svc.BeginMFA(r.Context(), actor(r))
	switch {
	case errors.Is(err, ErrMFAEnabled):
		httpx.Error(w, http.StatusConflict, "MFA_ALREADY_ENABLED", "2FA sudah aktif.")
	case err != nil:
		h.internal(w, r, "mulai 2FA gagal", err)
	default:
		httpx.JSON(w, http.StatusOK, setup)
	}
}

type codeRequest struct {
	Code string `json:"code"`
}

func (h *Handler) MFAEnable(w http.ResponseWriter, r *http.Request) {
	var req codeRequest
	if !httpx.DecodeJSON(w, r, &req) {
		return
	}
	a := actor(r)
	fail, done, ok := h.attempt(w, r, "platform-2fa:"+a.ID.String())
	if !ok {
		return
	}
	codes, err := h.svc.EnableMFA(r.Context(), a, req.Code)
	switch {
	case errors.Is(err, ErrInvalidCode):
		h.invalidCode(w, fail)
	case errors.Is(err, ErrMFAEnabled):
		httpx.Error(w, http.StatusConflict, "MFA_ALREADY_ENABLED", "2FA sudah aktif.")
	case errors.Is(err, ErrMFANotPending):
		httpx.Error(w, http.StatusConflict, "MFA_NOT_PENDING", "Mulai pendaftaran 2FA terlebih dahulu.")
	case err != nil:
		h.internal(w, r, "aktifkan 2FA gagal", err)
	default:
		done()
		httpx.JSON(w, http.StatusOK, map[string]any{"recovery_codes": codes})
	}
}

func (h *Handler) MFARecovery(w http.ResponseWriter, r *http.Request) {
	var req codeRequest
	if !httpx.DecodeJSON(w, r, &req) {
		return
	}
	a := actor(r)
	fail, done, ok := h.attempt(w, r, "platform-2fa:"+a.ID.String())
	if !ok {
		return
	}
	codes, err := h.svc.RegenerateRecovery(r.Context(), a, req.Code)
	switch {
	case errors.Is(err, ErrInvalidCode):
		h.invalidCode(w, fail)
	case err != nil:
		h.internal(w, r, "kode pemulihan gagal", err)
	default:
		done()
		httpx.JSON(w, http.StatusOK, map[string]any{"recovery_codes": codes})
	}
}

type disableMFARequest struct {
	Password string `json:"password"`
	Code     string `json:"code"`
}

func (h *Handler) MFADisable(w http.ResponseWriter, r *http.Request) {
	var req disableMFARequest
	if !httpx.DecodeJSON(w, r, &req) {
		return
	}
	a := actor(r)
	fail, done, ok := h.attempt(w, r, "platform-2fa:"+a.ID.String())
	if !ok {
		return
	}
	if req.Password == "" || len(req.Password) > maxPassword {
		h.invalidCode(w, fail)
		return
	}
	err := h.svc.DisableOwnMFA(r.Context(), a, req.Password, req.Code)
	switch {
	case errors.Is(err, ErrInvalidCode), errors.Is(err, ErrInvalidCredentials):
		h.invalidCode(w, fail)
	case err != nil:
		h.internal(w, r, "nonaktifkan 2FA gagal", err)
	default:
		done()
		h.clearRefreshCookie(w)
		w.WriteHeader(http.StatusNoContent)
	}
}

func (h *Handler) ResetMFA(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	switch err := h.svc.ResetAdminMFA(r.Context(), actor(r), id); {
	case errors.Is(err, ErrSelf):
		httpx.Error(w, http.StatusConflict, "CANNOT_RESET_SELF", "Gunakan menu keamanan untuk akun sendiri.")
	case errors.Is(err, ErrNotFound):
		httpx.Error(w, http.StatusNotFound, "NOT_FOUND", "Admin tidak ditemukan.")
	case errors.Is(err, ErrInvalidSession):
		httpx.Error(w, http.StatusUnauthorized, "UNAUTHORIZED", "Sesi tidak valid.")
	case err != nil:
		h.internal(w, r, "reset 2FA gagal", err)
	default:
		w.WriteHeader(http.StatusNoContent)
	}
}
