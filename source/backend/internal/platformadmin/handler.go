package platformadmin

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"

	"aciraba/internal/audit"
	"aciraba/internal/auth"
	pauth "aciraba/internal/platform/auth"
	"aciraba/internal/platform/httpx"
	"aciraba/internal/platform/sanitize"
	"aciraba/internal/platform/storage"
)

const (
	refreshCookie = "platform_refresh"
	cookiePath    = "/platform/auth"
	maxPassword   = 128
	maxQuery      = 100
)

// HandlerDeps = ketergantungan HTTP.
type HandlerDeps struct {
	Service      *Service
	Log          *slog.Logger
	Redis        *redis.Client
	Lockout      *auth.Lockout // dipakai bersama login tenant; kunci hitungannya diberi awalan "platform:"
	Origins      []string      // untuk CSRFGuard (endpoint ber-cookie)
	SecureCookie bool
	APK          *storage.Local // penyimpanan APK Kasir; nil = fitur unggah APK nonaktif
}

type Handler struct {
	HandlerDeps
	svc *Service
}

func NewHandler(d HandlerDeps) *Handler { return &Handler{HandlerDeps: d, svc: d.Service} }

func (h *Handler) Routes(r chi.Router) {
	csrf := httpx.CSRFGuard(h.Origins)
	authed := httpx.RequireAuth(h.svc.PTokens)

	r.Get("/platform/setup/status", h.SetupStatus)

	// APK Kasir: unduh publik (halaman login web); unggah/hapus khusus Platform Admin (di bawah).
	r.With(httpx.RateLimit(h.Redis, "public-apk-info", 600, time.Hour)).Get("/public/mobile/apk/info", h.APKInfoPublic)
	r.With(httpx.RateLimit(h.Redis, "public-apk", 60, time.Hour)).Get("/public/mobile/apk", h.APKDownload)
	r.With(httpx.RateLimit(h.Redis, "platform-setup", 10, time.Hour)).Post("/platform/setup", h.Setup)
	r.With(httpx.RateLimit(h.Redis, "platform-login", 30, 15*time.Minute)).Post("/platform/auth/login", h.Login)
	r.With(httpx.RateLimit(h.Redis, "platform-login-mfa", 30, 15*time.Minute)).Post("/platform/auth/login/mfa", h.LoginMFA)
	r.With(csrf, httpx.RateLimit(h.Redis, "platform-refresh", 600, time.Hour)).Post("/platform/auth/refresh", h.Refresh)
	r.With(csrf).Post("/platform/auth/logout", h.Logout)

	r.Group(func(r chi.Router) {
		r.Use(authed, h.svc.Authenticate)
		r.Get("/platform/auth/me", h.Me)

		// Keamanan akun (2FA): satu-satunya area yang terbuka bagi admin yang belum mendaftarkan 2FA.
		r.Get("/platform/security", h.MFAStatus)
		r.Post("/platform/security/totp/begin", h.MFABegin)
		r.Post("/platform/security/totp/enable", h.MFAEnable)
		r.Post("/platform/security/totp/disable", h.MFADisable)
		r.Post("/platform/security/recovery-codes", h.MFARecovery)

		// Selebihnya wajib 2FA aktif (Platform Admin = akses ke semua data pelanggan).
		r.Group(func(r chi.Router) {
			r.Use(requireMFA)
			r.Get("/platform/tenants", h.ListTenants)
			r.Get("/platform/tenants/{id}", h.GetTenant)
			r.Patch("/platform/tenants/{id}", h.PatchTenant)
			r.Get("/platform/tenants/{id}/audit", h.TenantAudit)
			r.Post("/platform/tenants/{id}/impersonate", h.Impersonate)

			r.Get("/platform/admins", h.ListAdmins)
			r.Post("/platform/admins", h.CreateAdmin)
			r.Patch("/platform/admins/{id}", h.PatchAdmin)
			r.Put("/platform/admins/{id}/password", h.SetPassword)

			r.Get("/platform/audit", h.Audit)
			r.Post("/platform/admins/{id}/reset-2fa", h.ResetMFA)

			r.Put("/platform/mobile/apk", h.APKUpload)
			r.Delete("/platform/mobile/apk", h.APKDelete)
		})
	})
}

func actor(r *http.Request) Actor {
	a, _ := ActorFrom(r.Context())
	return a
}

func (h *Handler) internal(w http.ResponseWriter, r *http.Request, msg string, err error) {
	h.Log.Error(msg, "err", err, "req_id", middleware.GetReqID(r.Context()))
	httpx.Error(w, http.StatusInternalServerError, "INTERNAL", "Terjadi kesalahan pada server.")
}

func (h *Handler) setRefreshCookie(w http.ResponseWriter, s *Session) {
	if s.RefreshToken == "" {
		return
	}
	maxAge := 0
	if s.Remember {
		maxAge = int(pauth.RefreshTTLRemember.Seconds())
	}
	http.SetCookie(w, &http.Cookie{
		Name: refreshCookie, Value: s.RefreshToken, Path: cookiePath, MaxAge: maxAge,
		HttpOnly: true, Secure: h.SecureCookie, SameSite: http.SameSiteLaxMode,
	})
}

func (h *Handler) clearRefreshCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name: refreshCookie, Value: "", Path: cookiePath, MaxAge: -1,
		HttpOnly: true, Secure: h.SecureCookie, SameSite: http.SameSiteLaxMode,
	})
}

func pathID(w http.ResponseWriter, r *http.Request) (uuid.UUID, bool) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		httpx.Error(w, http.StatusNotFound, "NOT_FOUND", "Tidak ditemukan.")
		return uuid.Nil, false
	}
	return id, true
}

// ---- setup & sesi ----

func (h *Handler) SetupStatus(w http.ResponseWriter, r *http.Request) {
	required, err := h.svc.SetupRequired(r.Context())
	if err != nil {
		h.internal(w, r, "status setup gagal", err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]bool{"setup_required": required})
}

type setupRequest struct {
	SetupToken string `json:"setup_token"`
	Name       string `json:"name"`
	Email      string `json:"email"`
	Password   string `json:"password"`
}

func validateAdmin(name, email, password string) (string, string, map[string]string) {
	f := map[string]string{}
	n, code := sanitize.Name(name, maxName)
	if code != "" {
		f["name"] = code
	}
	e, code := sanitize.Email(email)
	if code != "" {
		f["email"] = code
	}
	if code := sanitize.Password(password, e); code != "" {
		f["password"] = code
	}
	return n, e, f
}

func (h *Handler) Setup(w http.ResponseWriter, r *http.Request) {
	var req setupRequest
	if !httpx.DecodeJSON(w, r, &req) {
		return
	}
	name, email, fields := validateAdmin(req.Name, req.Email, req.Password)
	if len(req.SetupToken) > 256 {
		fields["setup_token"] = "TOO_LONG"
	}
	if req.SetupToken == "" {
		fields["setup_token"] = "REQUIRED"
	}
	if len(fields) > 0 {
		httpx.ValidationError(w, fields)
		return
	}
	sess, err := h.svc.Setup(r.Context(), req.SetupToken, name, email, req.Password)
	switch {
	case errors.Is(err, ErrSetupUnavailable):
		httpx.Error(w, http.StatusNotFound, "NOT_FOUND", "Endpoint tidak ditemukan.")
		return
	case errors.Is(err, ErrBadSetupToken):
		httpx.Error(w, http.StatusForbidden, "INVALID_SETUP_TOKEN", "Token setup salah.")
		return
	case err != nil:
		h.internal(w, r, "setup gagal", err)
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
	email, code := sanitize.Email(req.Email)
	if code != "" || req.Password == "" || len(req.Password) > maxPassword {
		httpx.Error(w, http.StatusUnauthorized, "INVALID_CREDENTIALS", "Email atau password salah.")
		return
	}
	sum := sha256.Sum256([]byte("platform:" + email))
	emailHash, ip := hex.EncodeToString(sum[:]), httpx.ClientIP(r)
	locked, err := h.Lockout.Check(r.Context(), ip, emailHash)
	if err != nil {
		httpx.Error(w, http.StatusServiceUnavailable, "UNAVAILABLE", "Layanan sementara tidak tersedia.")
		return
	}
	if locked > 0 {
		httpx.Retry(w, "ACCOUNT_LOCKED", "Terlalu banyak percobaan gagal. Coba lagi nanti.", locked)
		return
	}
	sess, mfaToken, err := h.svc.Login(r.Context(), email, req.Password, req.Remember)
	switch {
	case errors.Is(err, ErrInvalidCredentials):
		res, lerr := h.Lockout.RecordFailure(r.Context(), ip, emailHash)
		if lerr != nil {
			h.Log.Error("catat gagal login platform", "err", lerr)
		} else if res.LockedFor > 0 {
			httpx.Retry(w, "ACCOUNT_LOCKED", "Terlalu banyak percobaan gagal. Coba lagi nanti.", res.LockedFor)
			return
		}
		httpx.ErrorAttempts(w, http.StatusUnauthorized, "INVALID_CREDENTIALS", "Email atau password salah.", res.AttemptsLeft)
		return
	case errors.Is(err, ErrAccountDisabled):
		httpx.Error(w, http.StatusForbidden, "ACCOUNT_DISABLED", "Akun dinonaktifkan.")
		return
	case err != nil:
		h.internal(w, r, "login platform gagal", err)
		return
	}
	if err := h.Lockout.Reset(r.Context(), ip, emailHash); err != nil {
		h.Log.Error("reset kunci login platform", "err", err)
	}
	if mfaToken != "" {
		// Password benar, 2FA aktif: belum ada sesi. Klien melanjutkan ke /platform/auth/login/mfa.
		httpx.JSON(w, http.StatusOK, map[string]any{"mfa_required": true, "mfa_token": mfaToken})
		return
	}
	h.setRefreshCookie(w, sess)
	httpx.JSON(w, http.StatusOK, sess)
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
		h.internal(w, r, "refresh platform gagal", err)
		return
	}
	h.setRefreshCookie(w, sess)
	httpx.JSON(w, http.StatusOK, sess)
}

func (h *Handler) Logout(w http.ResponseWriter, r *http.Request) {
	h.clearRefreshCookie(w)
	if c, err := r.Cookie(refreshCookie); err == nil && c.Value != "" {
		if err := h.svc.Logout(r.Context(), c.Value); err != nil {
			h.internal(w, r, "logout platform gagal", err)
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
	case err != nil:
		h.internal(w, r, "me platform gagal", err)
	default:
		httpx.JSON(w, http.StatusOK, p)
	}
}

// ---- tenant ----

func intParam(q map[string][]string, key string) (int, bool) {
	v := q[key]
	if len(v) == 0 || v[0] == "" {
		return 0, true
	}
	n, err := strconv.Atoi(v[0])
	return n, err == nil && n >= 0
}

func (h *Handler) ListTenants(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	limit, ok1 := intParam(q, "limit")
	offset, ok2 := intParam(q, "offset")
	search, _ := sanitize.Text(q.Get("q"))
	if !ok1 || !ok2 || len(search) > maxQuery {
		httpx.ValidationError(w, map[string]string{"q": "INVALID"})
		return
	}
	page, err := h.svc.ListTenants(r.Context(), actor(r), search, limit, offset)
	if err != nil {
		h.internal(w, r, "daftar tenant gagal", err)
		return
	}
	httpx.JSON(w, http.StatusOK, page)
}

func (h *Handler) GetTenant(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	t, err := h.svc.Tenant(r.Context(), id)
	switch {
	case errors.Is(err, ErrNotFound):
		httpx.Error(w, http.StatusNotFound, "NOT_FOUND", "Tenant tidak ditemukan.")
	case err != nil:
		h.internal(w, r, "detail tenant gagal", err)
	default:
		httpx.JSON(w, http.StatusOK, t)
	}
}

type activeRequest struct {
	Active *bool `json:"active"`
	// SaleEditWindowDays: batas hari edit/batal nota tenant (0–3650); boleh dikirim bersama atau tanpa active.
	SaleEditWindowDays *int `json:"sale_edit_window_days"`
}

func (h *Handler) PatchTenant(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	var req activeRequest
	if !httpx.DecodeJSON(w, r, &req) {
		return
	}
	if req.Active == nil && req.SaleEditWindowDays == nil {
		httpx.ValidationError(w, map[string]string{"active": "REQUIRED"})
		return
	}
	if req.SaleEditWindowDays != nil {
		switch err := h.svc.SetSaleEditWindow(r.Context(), actor(r), id, *req.SaleEditWindowDays); {
		case errors.Is(err, ErrInvalidWindow):
			httpx.ValidationError(w, map[string]string{"sale_edit_window_days": "INVALID"})
			return
		case errors.Is(err, ErrNotFound):
			httpx.Error(w, http.StatusNotFound, "NOT_FOUND", "Tenant tidak ditemukan.")
			return
		case err != nil:
			h.internal(w, r, "ubah batas edit nota gagal", err)
			return
		}
		if req.Active == nil {
			w.WriteHeader(http.StatusNoContent)
			return
		}
	}
	switch err := h.svc.SetTenantActive(r.Context(), actor(r), id, *req.Active); {
	case errors.Is(err, ErrNotFound):
		httpx.Error(w, http.StatusNotFound, "NOT_FOUND", "Tenant tidak ditemukan.")
	case err != nil:
		h.internal(w, r, "ubah status tenant gagal", err)
	default:
		w.WriteHeader(http.StatusNoContent)
	}
}

func (h *Handler) TenantAudit(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	q := r.URL.Query()
	limit, okL := intParam(q, "limit")
	if !okL {
		httpx.ValidationError(w, map[string]string{"limit": "INVALID"})
		return
	}
	page, err := h.svc.TenantAudit(r.Context(), id, audit.Filter{ActionPrefix: q.Get("action"), Cursor: q.Get("cursor"), Limit: limit})
	switch {
	case errors.Is(err, audit.ErrBadCursor):
		httpx.ValidationError(w, map[string]string{"cursor": "INVALID"})
	case err != nil:
		h.internal(w, r, "audit tenant gagal", err)
	default:
		httpx.JSON(w, http.StatusOK, page)
	}
}

type impersonateRequest struct {
	OutletID string `json:"outlet_id"`
}

func (h *Handler) Impersonate(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	var req impersonateRequest
	if !httpx.DecodeJSON(w, r, &req) {
		return
	}
	outlet := uuid.Nil
	if req.OutletID != "" {
		var err error
		if outlet, err = uuid.Parse(req.OutletID); err != nil {
			httpx.ValidationError(w, map[string]string{"outlet_id": "INVALID"})
			return
		}
	}
	res, err := h.svc.Impersonate(r.Context(), actor(r), id, outlet)
	switch {
	case errors.Is(err, ErrNotFound):
		httpx.Error(w, http.StatusNotFound, "NOT_FOUND", "Tenant tidak ditemukan.")
	case errors.Is(err, ErrOutletNotFound):
		httpx.Error(w, http.StatusNotFound, "OUTLET_NOT_FOUND", "Outlet aktif tidak ditemukan.")
	case err != nil:
		h.internal(w, r, "masuk sebagai gagal", err)
	default:
		httpx.JSON(w, http.StatusOK, res)
	}
}

// ---- admin ----

func (h *Handler) ListAdmins(w http.ResponseWriter, r *http.Request) {
	list, err := h.svc.ListAdmins(r.Context())
	if err != nil {
		h.internal(w, r, "daftar admin gagal", err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"items": list})
}

type createAdminRequest struct {
	Name     string `json:"name"`
	Email    string `json:"email"`
	Password string `json:"password"`
}

func (h *Handler) CreateAdmin(w http.ResponseWriter, r *http.Request) {
	var req createAdminRequest
	if !httpx.DecodeJSON(w, r, &req) {
		return
	}
	name, email, fields := validateAdmin(req.Name, req.Email, req.Password)
	if len(fields) > 0 {
		httpx.ValidationError(w, fields)
		return
	}
	id, err := h.svc.CreateAdmin(r.Context(), actor(r), name, email, req.Password)
	switch {
	case errors.Is(err, ErrEmailTaken):
		httpx.Error(w, http.StatusConflict, "EMAIL_TAKEN", "Email sudah terdaftar.")
	case errors.Is(err, ErrInvalidSession):
		httpx.Error(w, http.StatusUnauthorized, "UNAUTHORIZED", "Sesi tidak valid.")
	case err != nil:
		h.internal(w, r, "buat admin gagal", err)
	default:
		httpx.JSON(w, http.StatusCreated, map[string]string{"id": id.String()})
	}
}

func (h *Handler) PatchAdmin(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	var req activeRequest
	if !httpx.DecodeJSON(w, r, &req) {
		return
	}
	if req.Active == nil {
		httpx.ValidationError(w, map[string]string{"active": "REQUIRED"})
		return
	}
	switch err := h.svc.SetAdminActive(r.Context(), actor(r), id, *req.Active); {
	case errors.Is(err, ErrNotFound):
		httpx.Error(w, http.StatusNotFound, "NOT_FOUND", "Admin tidak ditemukan.")
	case errors.Is(err, ErrSelf):
		httpx.Error(w, http.StatusConflict, "CANNOT_DISABLE_SELF", "Tidak dapat menonaktifkan akun sendiri.")
	case errors.Is(err, ErrLastAdmin):
		httpx.Error(w, http.StatusConflict, "LAST_ADMIN", "Harus tersisa minimal satu Platform Admin aktif.")
	case errors.Is(err, ErrInvalidSession):
		httpx.Error(w, http.StatusUnauthorized, "UNAUTHORIZED", "Sesi tidak valid.")
	case err != nil:
		h.internal(w, r, "ubah status admin gagal", err)
	default:
		w.WriteHeader(http.StatusNoContent)
	}
}

type passwordRequest struct {
	Password string `json:"password"`
}

func (h *Handler) SetPassword(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	var req passwordRequest
	if !httpx.DecodeJSON(w, r, &req) {
		return
	}
	// Password tidak boleh sama dengan email target; email tidak dimuat di sini, cukup aturan umum (panjang/huruf+angka).
	if code := sanitize.Password(req.Password, ""); code != "" {
		httpx.ValidationError(w, map[string]string{"password": code})
		return
	}
	switch err := h.svc.SetAdminPassword(r.Context(), actor(r), id, req.Password); {
	case errors.Is(err, ErrNotFound):
		httpx.Error(w, http.StatusNotFound, "NOT_FOUND", "Admin tidak ditemukan.")
	case errors.Is(err, ErrInvalidSession):
		httpx.Error(w, http.StatusUnauthorized, "UNAUTHORIZED", "Sesi tidak valid.")
	case err != nil:
		h.internal(w, r, "ganti password admin gagal", err)
	default:
		w.WriteHeader(http.StatusNoContent)
	}
}

// ---- audit platform ----

func (h *Handler) Audit(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	bad := map[string]string{}
	f := AuditFilter{ActionPrefix: q.Get("action")}
	for _, p := range []struct {
		key string
		dst *uuid.UUID
	}{{"tenant_id", &f.TenantID}, {"admin_id", &f.AdminID}} {
		if v := q.Get(p.key); v != "" {
			id, err := uuid.Parse(v)
			if err != nil {
				bad[p.key] = "INVALID"
			}
			*p.dst = id
		}
	}
	if v := q.Get("before"); v != "" {
		n, err := strconv.ParseInt(v, 10, 64)
		if err != nil || n < 1 {
			bad["before"] = "INVALID"
		}
		f.BeforeID = n
	}
	if n, ok := intParam(q, "limit"); !ok {
		bad["limit"] = "INVALID"
	} else {
		f.Limit = n
	}
	if len(f.ActionPrefix) > 64 {
		bad["action"] = "INVALID"
	}
	if len(bad) > 0 {
		httpx.ValidationError(w, bad)
		return
	}
	page, err := h.svc.Audit(r.Context(), f)
	if err != nil {
		h.internal(w, r, "audit platform gagal", err)
		return
	}
	httpx.JSON(w, http.StatusOK, page)
}
