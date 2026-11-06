package profile

import (
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"

	"aciraba/internal/authz"
	"aciraba/internal/item"
	pauth "aciraba/internal/platform/auth"
	"aciraba/internal/platform/httpx"
	"aciraba/internal/usermfa"
)

type Handler struct {
	svc      *Service
	resolver *authz.Resolver
	tokens   *pauth.TokenIssuer
	rdb      *redis.Client
	mfa      *usermfa.Service
	log      *slog.Logger
}

func NewHandler(svc *Service, resolver *authz.Resolver, tokens *pauth.TokenIssuer, rdb *redis.Client, mfa *usermfa.Service, log *slog.Logger) *Handler {
	return &Handler{svc: svc, resolver: resolver, tokens: tokens, rdb: rdb, mfa: mfa, log: log}
}

// Routes: semua endpoint hanya butuh login (tanpa izin modul) karena selalu bekerja pada akun pemanggil sendiri.
func (h *Handler) Routes(r chi.Router) {
	r.Route("/profile", func(r chi.Router) {
		r.Use(httpx.RequireAuth(h.tokens), h.resolver.Authenticate)
		r.Get("/", h.Get)
		r.Put("/", h.Update)
		r.With(httpx.RateLimit(h.rdb, "profile-password", 10, time.Hour)).Post("/password", h.ChangePassword)
		r.Get("/activity", h.Activity)
		// 2FA TOTP: percobaan kode dibatasi per IP; password/kode salah juga dibatasi di sini karena tanpa Lockout login.
		r.Route("/2fa", func(r chi.Router) {
			r.Use(httpx.RateLimit(h.rdb, "profile-2fa", 30, 15*time.Minute))
			r.Get("/", h.MFAStatus)
			r.Post("/begin", h.MFABegin)
			r.Post("/enable", h.MFAEnable)
			r.Post("/recovery-codes", h.MFARecovery)
			r.Post("/disable", h.MFADisable)
		})
		r.Post("/avatar", h.upload(Avatar))
		r.Delete("/avatar", h.remove(Avatar))
		r.Post("/banner", h.upload(Banner))
		r.Delete("/banner", h.remove(Banner))
		// Berkas gambar milik pengguna lain di tenant yang sama (mis. daftar pengguna): /profile/{id}/avatar.
		r.Get("/{id}/avatar", h.file(Avatar))
		r.Get("/{id}/banner", h.file(Banner))
	})
}

func actor(r *http.Request) authz.Actor {
	a, _ := authz.ActorFrom(r.Context())
	return a
}

func (h *Handler) Get(w http.ResponseWriter, r *http.Request) {
	p, err := h.svc.Get(r.Context(), actor(r))
	if err != nil {
		h.fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, p)
}

func (h *Handler) Update(w http.ResponseWriter, r *http.Request) {
	var in UpdateInput
	if !httpx.DecodeJSON(w, r, &in) {
		return
	}
	p, err := h.svc.Update(r.Context(), actor(r), in)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, p)
}

func (h *Handler) ChangePassword(w http.ResponseWriter, r *http.Request) {
	var in struct {
		CurrentPassword string `json:"current_password"`
		Password        string `json:"password"`
	}
	if !httpx.DecodeJSON(w, r, &in) {
		return
	}
	if err := h.svc.ChangePassword(r.Context(), actor(r), in.CurrentPassword, in.Password); err != nil {
		h.fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) Activity(w http.ResponseWriter, r *http.Request) {
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	p, err := h.svc.ActivityList(r.Context(), actor(r), limit, r.URL.Query().Get("cursor"))
	if err != nil {
		h.fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, p)
}

func (h *Handler) MFAStatus(w http.ResponseWriter, r *http.Request) {
	st, err := h.mfa.Status(r.Context(), actor(r))
	if err != nil {
		h.fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, st)
}

func (h *Handler) MFABegin(w http.ResponseWriter, r *http.Request) {
	setup, err := h.mfa.Begin(r.Context(), actor(r))
	if err != nil {
		h.fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, setup)
}

type codeRequest struct {
	Code     string `json:"code"`
	Password string `json:"password"`
}

func (h *Handler) MFAEnable(w http.ResponseWriter, r *http.Request) {
	var in codeRequest
	if !httpx.DecodeJSON(w, r, &in) {
		return
	}
	codes, err := h.mfa.Enable(r.Context(), actor(r), in.Code)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"recovery_codes": codes})
}

func (h *Handler) MFARecovery(w http.ResponseWriter, r *http.Request) {
	var in codeRequest
	if !httpx.DecodeJSON(w, r, &in) {
		return
	}
	codes, err := h.mfa.Regenerate(r.Context(), actor(r), in.Code)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"recovery_codes": codes})
}

func (h *Handler) MFADisable(w http.ResponseWriter, r *http.Request) {
	var in codeRequest
	if !httpx.DecodeJSON(w, r, &in) {
		return
	}
	if err := h.mfa.Disable(r.Context(), actor(r), in.Password, in.Code); err != nil {
		h.fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

const maxUploadBody = item.MaxUploadBytes + 1<<20

func (h *Handler) upload(k Kind) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		_ = http.NewResponseController(w).SetReadDeadline(time.Now().Add(2 * time.Minute))
		r.Body = http.MaxBytesReader(w, r.Body, maxUploadBody)
		mr, err := r.MultipartReader()
		if err != nil {
			httpx.Error(w, http.StatusUnsupportedMediaType, "UNSUPPORTED_MEDIA_TYPE", "Content-Type harus multipart/form-data.")
			return
		}
		for {
			part, err := mr.NextPart()
			if err != nil {
				var tooBig *http.MaxBytesError
				if errors.As(err, &tooBig) {
					h.fail(w, r, item.ErrImageTooLarge)
					return
				}
				httpx.ValidationError(w, map[string]string{"file": "REQUIRED"})
				return
			}
			if part.FormName() != "file" {
				continue
			}
			p, err := h.svc.SetImage(r.Context(), actor(r), k, part)
			if err != nil {
				h.fail(w, r, err)
				return
			}
			httpx.JSON(w, http.StatusOK, p)
			return
		}
	}
}

func (h *Handler) remove(k Kind) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		p, err := h.svc.RemoveImage(r.Context(), actor(r), k)
		if err != nil {
			h.fail(w, r, err)
			return
		}
		httpx.JSON(w, http.StatusOK, p)
	}
}

// file menyajikan gambar (?size=thumb|full). Id gambar berubah tiap ganti, jadi aman di-cache lama.
func (h *Handler) file(k Kind) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		owner, err := uuid.Parse(chi.URLParam(r, "id"))
		if err != nil {
			httpx.Error(w, http.StatusNotFound, "NOT_FOUND", "Data tidak ditemukan.")
			return
		}
		f, imgID, err := h.svc.OpenImage(r.Context(), actor(r), owner, k, r.URL.Query().Get("size") == "thumb")
		if err != nil {
			h.fail(w, r, err)
			return
		}
		defer f.Close()
		w.Header().Set("Content-Type", "image/jpeg")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Cache-Control", "private, max-age=31536000, immutable")
		w.Header().Set("ETag", `"`+imgID+`"`)
		http.ServeContent(w, r, "", time.Time{}, f)
	}
}

func (h *Handler) fail(w http.ResponseWriter, r *http.Request, err error) {
	var fields FieldErrors
	switch {
	case errors.As(err, &fields):
		httpx.ValidationError(w, fields)
	case errors.Is(err, ErrNotFound):
		httpx.Error(w, http.StatusNotFound, "NOT_FOUND", "Data tidak ditemukan.")
	case errors.Is(err, ErrForbidden):
		httpx.Error(w, http.StatusForbidden, "FORBIDDEN", "Anda tidak memiliki izin untuk tindakan ini.")
	case errors.Is(err, usermfa.ErrInvalidCode):
		httpx.ValidationError(w, map[string]string{"code": "CODE_INVALID"})
	case errors.Is(err, usermfa.ErrBadPassword):
		httpx.ValidationError(w, map[string]string{"password": "PASSWORD_WRONG"})
	case errors.Is(err, usermfa.ErrEnabled):
		httpx.Error(w, http.StatusConflict, "MFA_ALREADY_ENABLED", "2FA sudah aktif.")
	case errors.Is(err, usermfa.ErrNotPending):
		httpx.Error(w, http.StatusConflict, "MFA_NOT_PENDING", "Mulai pendaftaran 2FA terlebih dahulu.")
	case errors.Is(err, usermfa.ErrForbidden):
		httpx.Error(w, http.StatusForbidden, "FORBIDDEN", "Anda tidak memiliki izin untuk tindakan ini.")
	case errors.Is(err, usermfa.ErrUserNotFound):
		httpx.Error(w, http.StatusNotFound, "NOT_FOUND", "Data tidak ditemukan.")
	case errors.Is(err, ErrBadPassword):
		httpx.ValidationError(w, map[string]string{"current_password": "PASSWORD_WRONG"})
	case errors.Is(err, item.ErrImageTooLarge):
		httpx.Error(w, http.StatusRequestEntityTooLarge, "IMAGE_TOO_LARGE", "Ukuran gambar terlalu besar.")
	case errors.Is(err, item.ErrImageUnsupported):
		httpx.Error(w, http.StatusUnsupportedMediaType, "IMAGE_UNSUPPORTED", "Format gambar tidak didukung. Gunakan JPG, PNG, atau WebP.")
	case errors.Is(err, item.ErrImageDimensions):
		httpx.Error(w, http.StatusUnprocessableEntity, "IMAGE_DIMENSIONS", "Dimensi gambar terlalu besar.")
	case errors.Is(err, item.ErrImageCorrupt):
		httpx.Error(w, http.StatusUnprocessableEntity, "IMAGE_CORRUPT", "Gambar rusak atau tidak dapat dibaca.")
	case errors.Is(err, ErrNoStorage):
		httpx.Error(w, http.StatusServiceUnavailable, "UNAVAILABLE", "Penyimpanan gambar belum tersedia.")
	default:
		h.log.Error("profile gagal", "err", err, "req_id", middleware.GetReqID(r.Context()))
		httpx.Error(w, http.StatusInternalServerError, "INTERNAL", "Terjadi kesalahan pada server.")
	}
}
