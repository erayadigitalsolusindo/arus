package auth

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/redis/go-redis/v9"

	"aciraba/internal/authz"
	pauth "aciraba/internal/platform/auth"
	"aciraba/internal/platform/httpx"
)

const (
	refreshCookie = "refresh_token"
	maxPassword   = 128

	loginIPLimit = 60 // per IP, semua percobaan: longgar karena toko berbagi satu IP; kunci gagal-login ada di Lockout
	loginWindow  = 15 * time.Minute
)

// HandlerDeps = ketergantungan HTTP. Struct agar menambah dependensi tidak mengubah semua pemanggil.
type HandlerDeps struct {
	Service      *Service
	Log          *slog.Logger
	Redis        *redis.Client
	Lockout      *Lockout
	RateLimits   *RateLimitLoader // nil = batas bawaan
	Tokens       *pauth.TokenIssuer
	Perms        *authz.Resolver
	Origins      []string // dipakai CSRFGuard (endpoint ber-cookie)
	SecureCookie bool
}

type Handler struct {
	HandlerDeps
	svc *Service
}

func NewHandler(d HandlerDeps) *Handler { return &Handler{HandlerDeps: d, svc: d.Service} }

// Routes mendaftarkan endpoint auth. Rate limit dipasang sebelum pekerjaan mahal (hash argon2id).
// Endpoint ber-cookie (refresh, logout, pindah outlet) dilindungi CSRFGuard.
func (h *Handler) Routes(r chi.Router) {
	csrf := nativeOr(httpx.CSRFGuard(h.Origins))
	authed := []func(http.Handler) http.Handler{httpx.RequireAuth(h.Tokens), h.Perms.Authenticate}

	r.With(h.limit("register", func(p RateLimitPolicy) int { return p.RegisterPerIP })).Post("/auth/register", h.Register)
	r.With(httpx.RateLimit(h.Redis, "login", loginIPLimit, loginWindow)).Post("/auth/login", h.Login)
	r.With(csrf, httpx.RateLimit(h.Redis, "refresh", 600, time.Hour)).Post("/auth/refresh", h.Refresh)
	r.With(csrf).Post("/auth/logout", h.Logout)
	r.With(authed...).Get("/auth/me", h.Me)
	r.With(authed...).Get("/auth/feature-shortcuts", h.GetFeatureShortcuts)
	r.With(authed...).Put("/auth/feature-shortcuts", h.PutFeatureShortcuts)

	r.With(h.limit("forgot", func(p RateLimitPolicy) int { return p.ForgotPerIP })).Post("/auth/forgot-password", h.ForgotPassword)
	r.With(h.limit("reset", func(p RateLimitPolicy) int { return p.ResetPerIP })).Post("/auth/reset-password", h.ResetPassword)
	r.With(h.limit("verify", func(p RateLimitPolicy) int { return p.VerifyPerIP })).Post("/auth/verify-email", h.VerifyEmail)
	r.With(authed...).Post("/auth/resend-verification", h.ResendVerification)
	r.With(append([]func(http.Handler) http.Handler{csrf}, authed...)...).Post("/auth/switch-outlet", h.SwitchOutlet)
}

// limit = rate limit per IP berjendela satu jam dengan batas dari pengaturan (app_settings `auth.rate_limits`).
func (h *Handler) limit(name string, pick func(RateLimitPolicy) int) func(http.Handler) http.Handler {
	return httpx.RateLimitFunc(h.Redis, name, time.Hour, func(ctx context.Context) int { return pick(h.RateLimits.Get(ctx)) })
}

// ClientHeader menandai klien native (aplikasi mobile). Klien ini tidak memakai cookie: refresh token dikirim dan
// diterima lewat badan JSON, dan disimpan di penyimpanan aman perangkat. Karena tidak ada kredensial ambient yang
// dikirim browser otomatis, CSRFGuard (yang melindungi cookie) tidak relevan dan dilewati; Bearer/refresh token
// tetap wajib sah.
const (
	ClientHeader = "X-Client"
	clientMobile = "mobile"
)

func isNative(r *http.Request) bool { return r.Header.Get(ClientHeader) == clientMobile }

// nativeOr menjalankan guard hanya untuk klien web (cookie); klien native langsung diteruskan.
func nativeOr(guard func(http.Handler) http.Handler) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		guarded := guard(next)
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if isNative(r) {
				next.ServeHTTP(w, r)
				return
			}
			guarded.ServeHTTP(w, r)
		})
	}
}

// nativeSession = respons sesi untuk klien native: sama dengan Session ditambah refresh token di badan.
type nativeSession struct {
	*Session
	RefreshToken string `json:"refresh_token,omitempty"`
}

// writeSession: web → refresh token di cookie httpOnly; native → di badan respons, tanpa cookie.
func (h *Handler) writeSession(w http.ResponseWriter, r *http.Request, status int, s *Session) {
	if isNative(r) {
		httpx.JSON(w, status, nativeSession{Session: s, RefreshToken: s.RefreshToken})
		return
	}
	h.setRefreshCookie(w, s)
	httpx.JSON(w, status, s)
}

// refreshTokenOf mengambil refresh token: native dari badan {"refresh_token"}, web dari cookie.
func (h *Handler) refreshTokenOf(w http.ResponseWriter, r *http.Request) (string, bool) {
	if isNative(r) {
		var body struct {
			RefreshToken string `json:"refresh_token"`
		}
		if !httpx.DecodeJSON(w, r, &body) {
			return "", false
		}
		return body.RefreshToken, true
	}
	c, err := r.Cookie(refreshCookie)
	if err != nil {
		return "", true
	}
	return c.Value, true
}

func actor(r *http.Request) authz.Actor {
	a, _ := authz.ActorFrom(r.Context())
	return a
}

// lang = bahasa pilihan klien (untuk email); dinormalkan di paket mailer.
func lang(r *http.Request) string { return r.Header.Get("Accept-Language") }

func (h *Handler) internal(w http.ResponseWriter, r *http.Request, msg string, err error) {
	h.Log.Error(msg, "err", err, "req_id", middleware.GetReqID(r.Context()))
	httpx.Error(w, http.StatusInternalServerError, "INTERNAL", "Terjadi kesalahan pada server.")
}

// setRefreshCookie: "tetap masuk" = cookie persisten; selain itu cookie sesi (hilang saat browser ditutup).
// Tidak melakukan apa pun bila RefreshToken kosong (jendela grace, pindah outlet).
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
		HttpOnly: true, Secure: h.SecureCookie, SameSite: http.SameSiteLaxMode,
	})
}

func (h *Handler) clearRefreshCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name: refreshCookie, Value: "", Path: "/auth", MaxAge: -1,
		HttpOnly: true, Secure: h.SecureCookie, SameSite: http.SameSiteLaxMode,
	})
}
