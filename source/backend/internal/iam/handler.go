package iam

import (
	"errors"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/google/uuid"

	"aciraba/internal/authz"
	"aciraba/internal/plan"
	pauth "aciraba/internal/platform/auth"
	"aciraba/internal/platform/httpx"
	"aciraba/internal/usermfa"
)

type Handler struct {
	svc      *Service
	resolver *authz.Resolver
	tokens   *pauth.TokenIssuer
	log      *slog.Logger
	mfa      *usermfa.Service // nil = reset 2FA oleh admin tidak tersedia
}

// WithMFA mengaktifkan DELETE /iam/users/{id}/2fa (admin mereset 2FA pengguna yang kehilangan perangkat).
func (h *Handler) WithMFA(m *usermfa.Service) *Handler {
	h.mfa = m
	return h
}

func NewHandler(svc *Service, resolver *authz.Resolver, tokens *pauth.TokenIssuer, log *slog.Logger) *Handler {
	return &Handler{svc: svc, resolver: resolver, tokens: tokens, log: log}
}

// Routes: semua endpoint butuh token akses sah + akun aktif; izin per aksi dicek dari DB (authz.Resolver).
func (h *Handler) Routes(r chi.Router) {
	r.Route("/iam", func(r chi.Router) {
		r.Use(httpx.RequireAuth(h.tokens), h.resolver.Authenticate)
		r.Get("/registry", h.Registry)

		r.With(authz.Require("roles", authz.ActView)).Get("/roles", h.ListRoles)
		r.With(authz.Require("roles", authz.ActCreate)).Post("/roles", h.CreateRole)
		r.With(authz.Require("roles", authz.ActUpdate)).Put("/roles/{id}", h.UpdateRole)
		r.With(authz.Require("roles", authz.ActDelete)).Delete("/roles/{id}", h.DeleteRole)

		r.With(authz.Require("users", authz.ActView)).Get("/users", h.ListUsers)
		r.With(authz.Require("users", authz.ActView)).Get("/assignable-roles", h.AssignableRoles)
		r.With(authz.Require("users", authz.ActCreate)).Post("/users", h.CreateUser)
		r.With(authz.Require("users", authz.ActUpdate)).Put("/users/{id}", h.UpdateUser)
		r.With(authz.Require("users", authz.ActUpdate)).Put("/users/{id}/password", h.ResetPassword)
		r.With(authz.Require("users", authz.ActUpdate)).Put("/users/{id}/verify-email", h.VerifyEmail)
		if h.mfa != nil {
			r.With(authz.Require("users", authz.ActUpdate)).Delete("/users/{id}/2fa", h.ResetMFA)
		}
	})
}

func actor(r *http.Request) authz.Actor {
	a, _ := authz.ActorFrom(r.Context())
	return a
}

func (h *Handler) Registry(w http.ResponseWriter, _ *http.Request) {
	httpx.JSON(w, http.StatusOK, map[string]any{"modules": authz.Modules})
}

func parseUUID(s string) uuid.UUID {
	id, _ := uuid.Parse(s) // tidak valid → uuid.Nil → ditolak validasi sebagai REQUIRED
	return id
}

func parseUUIDs(in []string) []uuid.UUID {
	out := make([]uuid.UUID, 0, len(in))
	for _, s := range in {
		out = append(out, parseUUID(s)) // yang tidak valid menjadi uuid.Nil dan ditolak checkOutlets
	}
	return out
}

func pathID(w http.ResponseWriter, r *http.Request) (uuid.UUID, bool) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		httpx.Error(w, http.StatusNotFound, "NOT_FOUND", "Data tidak ditemukan.")
		return uuid.Nil, false
	}
	return id, true
}

// fail memetakan error layanan ke respons API dengan kode stabil (diterjemahkan klien lewat errors.<CODE>).
func (h *Handler) fail(w http.ResponseWriter, r *http.Request, err error) {
	var fields FieldErrors
	switch {
	case errors.As(err, &fields):
		httpx.ValidationError(w, fields)
	case errors.Is(err, ErrNotFound):
		httpx.Error(w, http.StatusNotFound, "NOT_FOUND", "Data tidak ditemukan.")
	case errors.Is(err, ErrEscalation):
		httpx.Error(w, http.StatusForbidden, "PERMISSION_ESCALATION", "Tidak boleh memberi izin melebihi izin Anda sendiri.")
	case errors.Is(err, ErrOutletForbidden):
		httpx.Error(w, http.StatusForbidden, "OUTLET_FORBIDDEN", "Anda tidak memiliki akses ke outlet yang dipilih.")
	case errors.Is(err, ErrSystemRole):
		httpx.Error(w, http.StatusConflict, "SYSTEM_ROLE", "Role sistem tidak dapat diubah atau dihapus.")
	case errors.Is(err, ErrRoleInUse):
		httpx.Error(w, http.StatusConflict, "ROLE_IN_USE", "Role masih dipakai pengguna.")
	case errors.Is(err, ErrNameTaken):
		httpx.Error(w, http.StatusConflict, "NAME_TAKEN", "Nama role sudah dipakai.")
	case errors.Is(err, plan.ErrUserLimit):
		httpx.Error(w, http.StatusPaymentRequired, "PLAN_USER_LIMIT_REACHED", "Batas jumlah pengguna paket tercapai. Upgrade paket untuk menambah pengguna.")
	case errors.Is(err, ErrEmailTaken):
		httpx.Error(w, http.StatusConflict, "EMAIL_TAKEN", "Email sudah terdaftar.")
	case errors.Is(err, ErrLastOwner):
		httpx.Error(w, http.StatusConflict, "LAST_OWNER", "Pemilik aktif terakhir tidak boleh dinonaktifkan atau diganti rolenya.")
	case errors.Is(err, ErrSelfVerify):
		httpx.Error(w, http.StatusConflict, "SELF_VERIFY", "Tidak boleh memverifikasi email akun sendiri.")
	case errors.Is(err, ErrSelfChange):
		httpx.Error(w, http.StatusConflict, "SELF_CHANGE", "Tidak boleh menonaktifkan atau mengganti role akun sendiri.")
	default:
		h.log.Error("iam gagal", "err", err, "req_id", middleware.GetReqID(r.Context()))
		httpx.Error(w, http.StatusInternalServerError, "INTERNAL", "Terjadi kesalahan pada server.")
	}
}

// ResetMFA menghapus 2FA pengguna lain (perangkat hilang + kode pemulihan habis) dan mencabut semua sesinya.
func (h *Handler) ResetMFA(w http.ResponseWriter, r *http.Request) {
	err := h.mfa.Reset(r.Context(), actor(r), parseUUID(chi.URLParam(r, "id")))
	switch {
	case errors.Is(err, usermfa.ErrForbidden):
		httpx.Error(w, http.StatusForbidden, "FORBIDDEN", "Anda tidak memiliki izin untuk tindakan ini.")
	case errors.Is(err, usermfa.ErrUserNotFound):
		httpx.Error(w, http.StatusNotFound, "NOT_FOUND", "Data tidak ditemukan.")
	case err != nil:
		h.log.Error("reset 2fa gagal", "err", err)
		httpx.Error(w, http.StatusInternalServerError, "INTERNAL", "Terjadi kesalahan pada server.")
	default:
		w.WriteHeader(http.StatusNoContent)
	}
}
