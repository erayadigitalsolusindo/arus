package auth

import (
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/redis/go-redis/v9"

	"aciraba/internal/platform/httpx"
)

const refreshCookie = "refresh_token"

type Handler struct {
	svc          *Service
	log          *slog.Logger
	secureCookie bool
}

func NewHandler(svc *Service, log *slog.Logger, secureCookie bool) *Handler {
	return &Handler{svc: svc, log: log, secureCookie: secureCookie}
}

// Routes mendaftarkan endpoint publik auth. Rate limit dipasang sebelum pekerjaan mahal (hash argon2id).
func (h *Handler) Routes(r chi.Router, rdb *redis.Client) {
	r.With(httpx.RateLimit(rdb, "register", 10, time.Hour)).Post("/auth/register", h.Register)
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
		h.log.Error("register gagal", "err", err, "req_id", middleware.GetReqID(r.Context()))
		httpx.Error(w, http.StatusInternalServerError, "INTERNAL", "Terjadi kesalahan pada server.")
		return
	}

	http.SetCookie(w, &http.Cookie{
		Name: refreshCookie, Value: sess.RefreshToken, Path: "/auth", MaxAge: int(sess.RefreshTTL.Seconds()),
		HttpOnly: true, Secure: h.secureCookie, SameSite: http.SameSiteLaxMode,
	})
	httpx.JSON(w, http.StatusCreated, sess)
}
