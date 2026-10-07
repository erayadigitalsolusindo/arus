package auth

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/redis/go-redis/v9"

	pauth "aciraba/internal/platform/auth"
	"aciraba/internal/platform/httpx"
	"aciraba/internal/platform/sanitize"
)

const (
	refreshCookie = "refresh_token"
	maxPassword   = 128

	loginIPLimit = 60 // per IP, semua percobaan: longgar karena toko berbagi satu IP; kunci gagal-login ada di Lockout
	loginWindow  = 15 * time.Minute
)

type Handler struct {
	svc          *Service
	log          *slog.Logger
	rdb          *redis.Client
	tokens       *pauth.TokenIssuer
	lockout      *Lockout
	origins      []string
	secureCookie bool
}

func NewHandler(svc *Service, log *slog.Logger, rdb *redis.Client, lockout *Lockout, tokens *pauth.TokenIssuer, origins []string, secureCookie bool) *Handler {
	return &Handler{svc: svc, log: log, rdb: rdb, lockout: lockout, tokens: tokens, origins: origins, secureCookie: secureCookie}
}

// Routes mendaftarkan endpoint auth. Rate limit dipasang sebelum pekerjaan mahal (hash argon2id).
// Endpoint ber-cookie (refresh, logout) dilindungi CSRFGuard.
func (h *Handler) Routes(r chi.Router) {
	csrf := httpx.CSRFGuard(h.origins)
	r.With(httpx.RateLimit(h.rdb, "register", 10, time.Hour)).Post("/auth/register", h.Register)
	r.With(httpx.RateLimit(h.rdb, "login", loginIPLimit, loginWindow)).Post("/auth/login", h.Login)
	r.With(csrf, httpx.RateLimit(h.rdb, "refresh", 600, time.Hour)).Post("/auth/refresh", h.Refresh)
	r.With(csrf).Post("/auth/logout", h.Logout)
	r.With(httpx.RequireAuth(h.tokens)).Get("/auth/me", h.Me)
}

// registerRequest memakai tipe string saja; tipe lain (angka/objek) ditolak oleh decoder JSON.
type registerRequest struct {
	BusinessName string `json:"business_name"`
	OwnerName    string `json:"owner_name"`
	Email        string `json:"email"`
	Phone        string `json:"phone"`
	OutletName   string `json:"outlet_name"`
	Password     string `json:"password"`
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

	sess, err := h.svc.Register(r.Context(), clean)
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
	locked, err := h.lockout.Check(r.Context(), ip, emailHash)
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
		if d, lerr := h.lockout.RecordFailure(r.Context(), ip, emailHash); lerr != nil {
			h.log.Error("catat gagal login", "err", lerr, "req_id", middleware.GetReqID(r.Context()))
		} else if d > 0 {
			h.locked(w, d)
			return
		}
		httpx.Error(w, http.StatusUnauthorized, "INVALID_CREDENTIALS", "Email atau password salah.")
		return
	case errors.Is(err, ErrAccountDisabled):
		httpx.Error(w, http.StatusForbidden, "ACCOUNT_DISABLED", "Akun dinonaktifkan. Hubungi administrator.")
		return
	case err != nil:
		h.internal(w, r, "login gagal", err)
		return
	}
	if err := h.lockout.Reset(r.Context(), ip, emailHash); err != nil {
		h.log.Error("reset kunci login", "err", err)
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
	h.setRefreshCookie(w, sess) // tidak melakukan apa pun bila RefreshToken kosong (grace)
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
	claims, _ := httpx.ClaimsFrom(r.Context())
	p, err := h.svc.Me(r.Context(), claims.Subject, claims.TenantID)
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

func (h *Handler) internal(w http.ResponseWriter, r *http.Request, msg string, err error) {
	h.log.Error(msg, "err", err, "req_id", middleware.GetReqID(r.Context()))
	httpx.Error(w, http.StatusInternalServerError, "INTERNAL", "Terjadi kesalahan pada server.")
}

// setRefreshCookie: "tetap masuk" = cookie persisten; selain itu cookie sesi (hilang saat browser ditutup).
func (h *Handler) setRefreshCookie(w http.ResponseWriter, s *Session) {
	if s.RefreshToken == "" {
		return
	}
	maxAge := 0
	if s.Remember {
		maxAge = int(pauth.RefreshTTLRemember.Seconds())
	}
	http.SetCookie(w, &http.Cookie{
		Name: refreshCookie, Value: s.RefreshToken, Path: "/auth", MaxAge: maxAge,
		HttpOnly: true, Secure: h.secureCookie, SameSite: http.SameSiteLaxMode,
	})
}

func (h *Handler) clearRefreshCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name: refreshCookie, Value: "", Path: "/auth", MaxAge: -1,
		HttpOnly: true, Secure: h.secureCookie, SameSite: http.SameSiteLaxMode,
	})
}
