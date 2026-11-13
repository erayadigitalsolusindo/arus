package auth

import (
	"errors"
	"net/http"

	"aciraba/internal/platform/httpx"
)

type erayaExchangeRequest struct {
	Code         string `json:"code"`
	CodeVerifier string `json:"code_verifier"`
	RedirectURI  string `json:"redirect_uri"`
}

// ErayaExchange menukar code SSO Eraya dengan id_token (browser tidak bisa memanggil token endpoint lintas origin).
// id_token lalu dipakai ke /auth/eraya/login atau /auth/eraya/register, diverifikasi ulang oleh server.
func (h *Handler) ErayaExchange(w http.ResponseWriter, r *http.Request) {
	var req erayaExchangeRequest
	if !httpx.DecodeJSON(w, r, &req) {
		return
	}
	if h.svc.Eraya == nil {
		httpx.Error(w, http.StatusServiceUnavailable, "ERAYA_DISABLED", "Login dengan Eraya belum diaktifkan.")
		return
	}
	if req.Code == "" || len(req.Code) > 512 || len(req.CodeVerifier) < 43 || len(req.CodeVerifier) > 128 || req.RedirectURI == "" || len(req.RedirectURI) > 512 {
		httpx.Error(w, http.StatusUnauthorized, "ERAYA_INVALID_TOKEN", "Verifikasi Eraya gagal atau kedaluwarsa. Coba lagi.")
		return
	}
	idToken, err := h.svc.Eraya.Exchange(r.Context(), req.Code, req.CodeVerifier, req.RedirectURI)
	switch {
	case errors.Is(err, ErrGoogleInvalidToken):
		httpx.Error(w, http.StatusUnauthorized, "ERAYA_INVALID_TOKEN", "Verifikasi Eraya gagal atau kedaluwarsa. Coba lagi.")
	case err != nil:
		h.internal(w, r, "tukar code eraya gagal", err)
	default:
		httpx.JSON(w, http.StatusOK, map[string]string{"id_token": idToken})
	}
}

func (h *Handler) erayaClaims(w http.ResponseWriter, r *http.Request, idToken string) (GoogleClaims, bool) {
	if h.svc.Eraya == nil {
		httpx.Error(w, http.StatusServiceUnavailable, "ERAYA_DISABLED", "Login dengan Eraya belum diaktifkan.")
		return GoogleClaims{}, false
	}
	c, err := h.svc.Eraya.Verifier.Verify(r.Context(), idToken)
	switch {
	case errors.Is(err, ErrGoogleEmailUnverified):
		httpx.Error(w, http.StatusForbidden, "ERAYA_EMAIL_UNVERIFIED", "Email akun Eraya belum terverifikasi.")
		return GoogleClaims{}, false
	case errors.Is(err, ErrGoogleInvalidToken):
		httpx.Error(w, http.StatusUnauthorized, "ERAYA_INVALID_TOKEN", "Verifikasi Eraya gagal atau kedaluwarsa. Coba lagi.")
		return GoogleClaims{}, false
	case err != nil:
		h.internal(w, r, "verifikasi token eraya gagal", err)
		return GoogleClaims{}, false
	}
	return c, true
}

func (h *Handler) ErayaLogin(w http.ResponseWriter, r *http.Request) {
	var req googleLoginRequest
	if !httpx.DecodeJSON(w, r, &req) {
		return
	}
	c, ok := h.erayaClaims(w, r, req.IDToken)
	if !ok {
		return
	}
	sess, err := h.svc.IdentityLogin(r.Context(), erayaProviderID, c, req.Remember)
	switch {
	case errors.Is(err, ErrGoogleNotRegistered):
		httpx.Error(w, http.StatusNotFound, "ERAYA_NOT_REGISTERED", "Akun Eraya ini belum terdaftar. Silakan daftar terlebih dahulu.")
		return
	case errors.Is(err, ErrGoogleLinked):
		httpx.Error(w, http.StatusConflict, "ERAYA_LINKED", "Akun ini sudah tertaut ke akun Eraya lain.")
		return
	case errors.Is(err, ErrAccountDisabled):
		httpx.Error(w, http.StatusForbidden, "ACCOUNT_DISABLED", "Akun dinonaktifkan. Hubungi administrator.")
		return
	case errors.Is(err, ErrNoOutlet):
		httpx.Error(w, http.StatusForbidden, "NO_OUTLET", "Akun belum memiliki outlet aktif. Hubungi administrator.")
		return
	case writeMFARequired(w, err):
		return
	case err != nil:
		h.internal(w, r, "login eraya gagal", err)
		return
	}
	h.writeSession(w, r, http.StatusOK, sess)
}

func (h *Handler) ErayaRegister(w http.ResponseWriter, r *http.Request) {
	var req googleRegisterRequest
	if !httpx.DecodeJSON(w, r, &req) {
		return
	}
	c, ok := h.erayaClaims(w, r, req.IDToken)
	if !ok {
		return
	}
	clean, fields := ValidateIdentityRegister(erayaProviderID, GoogleRegisterInput{
		BusinessName: req.BusinessName, OutletName: req.OutletName, Phone: req.Phone, AcceptTerms: req.AcceptTerms,
	}, c)
	if fields != nil {
		httpx.ValidationError(w, fields)
		return
	}
	sess, err := h.svc.GoogleRegister(r.Context(), clean, lang(r))
	switch {
	case errors.Is(err, ErrEmailTaken):
		httpx.Error(w, http.StatusConflict, "EMAIL_TAKEN", "Email sudah terdaftar.")
		return
	case err != nil:
		h.internal(w, r, "register eraya gagal", err)
		return
	}
	h.writeSession(w, r, http.StatusCreated, sess)
}
