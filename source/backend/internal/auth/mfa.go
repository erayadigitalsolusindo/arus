package auth

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net/http"
	"strings"

	"github.com/google/uuid"

	pauth "aciraba/internal/platform/auth"
	"aciraba/internal/platform/httpx"
)

var (
	ErrInvalidMFAToken = errors.New("tantangan 2FA tidak valid atau kedaluwarsa")
	ErrInvalidMFACode  = errors.New("kode 2FA salah")
)

// MFARequired dikembalikan Login/GoogleLogin saat kredensial benar tetapi akun memakai 2FA: belum ada sesi, klien
// melanjutkan ke POST /auth/login/mfa dengan Token dan kode TOTP (atau kode pemulihan).
type MFARequired struct{ Token string }

func (*MFARequired) Error() string { return "kode 2FA diperlukan" }

// gate = pintu terakhir sebelum sesi terbit: akun beraktivasi 2FA ditahan di tantangan.
func (s *Service) gate(ctx context.Context, acc account, remember bool) (*Session, error) {
	if !acc.UserActive || !acc.TenantActive {
		return nil, ErrAccountDisabled
	}
	if acc.OutletID == uuid.Nil {
		return nil, ErrNoOutlet
	}
	if s.MFA != nil {
		on, err := s.MFA.Enabled(ctx, acc.TenantID, acc.UserID)
		if err != nil {
			return nil, err
		}
		if on {
			flag := "0"
			if remember {
				flag = "1"
			}
			tok, err := s.OneTime.Issue(ctx, pauth.PurposeUserMFA, acc.UserID.String()+"|"+flag, pauth.PlatformMFATTL)
			if err != nil {
				return nil, err
			}
			return nil, &MFARequired{Token: tok}
		}
	}
	return s.startSession(ctx, acc, remember)
}

func parseChallenge(subject string) (uuid.UUID, bool, error) {
	id, flag, _ := strings.Cut(subject, "|")
	uid, err := uuid.Parse(id)
	if err != nil {
		return uuid.Nil, false, ErrInvalidMFAToken
	}
	return uid, flag == "1", nil
}

// MFAChallengeUser membaca tantangan TANPA memakainya (salah kode tidak membakar tantangan; percobaan dibatasi Lockout).
func (s *Service) MFAChallengeUser(ctx context.Context, token string) (uuid.UUID, error) {
	subject, err := s.OneTime.Peek(ctx, pauth.PurposeUserMFA, token)
	if errors.Is(err, pauth.ErrInvalidToken) {
		return uuid.Nil, ErrInvalidMFAToken
	}
	if err != nil {
		return uuid.Nil, err
	}
	uid, _, err := parseChallenge(subject)
	return uid, err
}

// LoginMFA menyelesaikan login: kode benar → tantangan dipakai dan sesi terbit.
func (s *Service) LoginMFA(ctx context.Context, token, code string) (*Session, error) {
	if s.MFA == nil {
		return nil, ErrInvalidMFAToken
	}
	subject, err := s.OneTime.Peek(ctx, pauth.PurposeUserMFA, token)
	if errors.Is(err, pauth.ErrInvalidToken) {
		return nil, ErrInvalidMFAToken
	}
	if err != nil {
		return nil, err
	}
	uid, remember, err := parseChallenge(subject)
	if err != nil {
		return nil, err
	}
	acc, err := accountByUserID(ctx, s.Pool, uid, uuid.Nil)
	if isNoAccount(err) || (err == nil && (!acc.UserActive || !acc.TenantActive)) {
		return nil, ErrInvalidMFAToken
	}
	if err != nil {
		return nil, err
	}
	ok, err := s.MFA.Verify(ctx, acc.TenantID, acc.UserID, code)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, ErrInvalidMFACode
	}
	if _, err := s.OneTime.Consume(ctx, pauth.PurposeUserMFA, token); err != nil {
		return nil, ErrInvalidMFAToken // keduluan permintaan lain dengan tantangan yang sama
	}
	return s.startSession(ctx, acc, remember)
}

type mfaRequest struct {
	MFAToken string `json:"mfa_token"`
	Code     string `json:"code"`
}

// writeMFARequired menjawab login yang tertahan di tantangan 2FA. true bila err memang MFARequired.
func writeMFARequired(w http.ResponseWriter, err error) bool {
	var mr *MFARequired
	if !errors.As(err, &mr) {
		return false
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"mfa_required": true, "mfa_token": mr.Token})
	return true
}

// LoginMFA: langkah kedua login. Percobaan kode dibatasi Lockout per pengguna (selain rate limit per IP pada rute).
func (h *Handler) LoginMFA(w http.ResponseWriter, r *http.Request) {
	var req mfaRequest
	if !httpx.DecodeJSON(w, r, &req) {
		return
	}
	if req.MFAToken == "" || req.Code == "" || len(req.Code) > 32 {
		httpx.Error(w, http.StatusUnauthorized, "MFA_TOKEN_INVALID", "Tantangan 2FA tidak valid atau kedaluwarsa. Masuk ulang.")
		return
	}
	uid, err := h.svc.MFAChallengeUser(r.Context(), req.MFAToken)
	if errors.Is(err, ErrInvalidMFAToken) {
		httpx.Error(w, http.StatusUnauthorized, "MFA_TOKEN_INVALID", "Tantangan 2FA tidak valid atau kedaluwarsa. Masuk ulang.")
		return
	}
	if err != nil {
		h.internal(w, r, "tantangan 2fa gagal", err)
		return
	}
	sum := sha256.Sum256([]byte("user-mfa:" + uid.String()))
	keyHash, ip := hex.EncodeToString(sum[:]), httpx.ClientIP(r)
	locked, err := h.Lockout.Check(r.Context(), ip, keyHash)
	if err != nil {
		httpx.Error(w, http.StatusServiceUnavailable, "UNAVAILABLE", "Layanan sementara tidak tersedia.")
		return
	}
	if locked > 0 {
		h.locked(w, locked)
		return
	}
	sess, err := h.svc.LoginMFA(r.Context(), req.MFAToken, req.Code)
	switch {
	case errors.Is(err, ErrInvalidMFACode):
		res, lerr := h.Lockout.RecordFailure(r.Context(), ip, keyHash)
		if lerr != nil {
			h.Log.Error("catat gagal 2fa", "err", lerr)
		} else if res.LockedFor > 0 {
			h.locked(w, res.LockedFor)
			return
		}
		httpx.ErrorAttempts(w, http.StatusUnauthorized, "MFA_CODE_INVALID", "Kode 2FA salah.", res.AttemptsLeft)
		return
	case errors.Is(err, ErrInvalidMFAToken):
		httpx.Error(w, http.StatusUnauthorized, "MFA_TOKEN_INVALID", "Tantangan 2FA tidak valid atau kedaluwarsa. Masuk ulang.")
		return
	case errors.Is(err, ErrAccountDisabled):
		httpx.Error(w, http.StatusForbidden, "ACCOUNT_DISABLED", "Akun dinonaktifkan. Hubungi administrator.")
		return
	case errors.Is(err, ErrNoOutlet):
		httpx.Error(w, http.StatusForbidden, "NO_OUTLET", "Akun belum memiliki outlet aktif. Hubungi administrator.")
		return
	case err != nil:
		h.internal(w, r, "login 2fa gagal", err)
		return
	}
	if err := h.Lockout.Reset(r.Context(), ip, keyHash); err != nil {
		h.Log.Error("reset kunci 2fa", "err", err)
	}
	h.writeSession(w, r, http.StatusOK, sess)
}
