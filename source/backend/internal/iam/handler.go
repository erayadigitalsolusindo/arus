package iam

import (
	"errors"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/google/uuid"

	pauth "aciraba/internal/platform/auth"
	"aciraba/internal/platform/httpx"
)

type Handler struct {
	svc      *Service
	resolver *Resolver
	tokens   *pauth.TokenIssuer
	log      *slog.Logger
}

func NewHandler(svc *Service, resolver *Resolver, tokens *pauth.TokenIssuer, log *slog.Logger) *Handler {
	return &Handler{svc: svc, resolver: resolver, tokens: tokens, log: log}
}

// Routes: semua endpoint butuh token akses sah + akun aktif; izin per aksi dicek dari DB (Resolver).
func (h *Handler) Routes(r chi.Router) {
	r.Route("/iam", func(r chi.Router) {
		r.Use(httpx.RequireAuth(h.tokens), h.resolver.Authenticate)
		r.Get("/registry", h.Registry)

		r.With(Require("roles", ActView)).Get("/roles", h.ListRoles)
		r.With(Require("roles", ActCreate)).Post("/roles", h.CreateRole)
		r.With(Require("roles", ActUpdate)).Put("/roles/{id}", h.UpdateRole)
		r.With(Require("roles", ActDelete)).Delete("/roles/{id}", h.DeleteRole)

		r.With(Require("users", ActView)).Get("/users", h.ListUsers)
		r.With(Require("users", ActView)).Get("/assignable-roles", h.AssignableRoles)
		r.With(Require("users", ActCreate)).Post("/users", h.CreateUser)
		r.With(Require("users", ActUpdate)).Put("/users/{id}", h.UpdateUser)
		r.With(Require("users", ActUpdate)).Put("/users/{id}/password", h.ResetPassword)
	})
}

func actor(r *http.Request) Actor {
	a, _ := ActorFrom(r.Context())
	return a
}

func (h *Handler) Registry(w http.ResponseWriter, _ *http.Request) {
	httpx.JSON(w, http.StatusOK, map[string]any{"modules": Modules})
}

// ---- roles ----

type roleRequest struct {
	Name        string              `json:"name"`
	Permissions map[string][]string `json:"permissions"`
}

func (h *Handler) ListRoles(w http.ResponseWriter, r *http.Request) {
	roles, err := h.svc.ListRoles(r.Context(), actor(r).TenantID)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"roles": roles})
}

func (h *Handler) CreateRole(w http.ResponseWriter, r *http.Request) {
	var req roleRequest
	if !httpx.DecodeJSON(w, r, &req) {
		return
	}
	role, err := h.svc.CreateRole(r.Context(), actor(r), req.Name, req.Permissions)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusCreated, role)
}

func (h *Handler) UpdateRole(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	var req roleRequest
	if !httpx.DecodeJSON(w, r, &req) {
		return
	}
	role, err := h.svc.UpdateRole(r.Context(), actor(r), id, req.Name, req.Permissions)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, role)
}

func (h *Handler) DeleteRole(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	if err := h.svc.DeleteRole(r.Context(), actor(r), id); err != nil {
		h.fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ---- users ----

func (h *Handler) AssignableRoles(w http.ResponseWriter, r *http.Request) {
	roles, err := h.svc.AssignableRoles(r.Context(), actor(r))
	if err != nil {
		h.fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"roles": roles})
}

type createUserRequest struct {
	Name     string `json:"name"`
	Email    string `json:"email"`
	Phone    string `json:"phone"`
	Password string `json:"password"`
	RoleID   string `json:"role_id"`
}

type updateUserRequest struct {
	Name   string `json:"name"`
	Phone  string `json:"phone"`
	RoleID string `json:"role_id"`
	// Pointer: field yang hilang harus ditolak, bukan dianggap false (menonaktifkan akun tanpa sengaja).
	Active *bool `json:"active"`
}

type passwordRequest struct {
	Password string `json:"password"`
}

func (h *Handler) ListUsers(w http.ResponseWriter, r *http.Request) {
	users, err := h.svc.ListUsers(r.Context(), actor(r).TenantID)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"users": users})
}

func (h *Handler) CreateUser(w http.ResponseWriter, r *http.Request) {
	var req createUserRequest
	if !httpx.DecodeJSON(w, r, &req) {
		return
	}
	user, err := h.svc.CreateUser(r.Context(), actor(r), CreateUserInput{
		Name: req.Name, Email: req.Email, Phone: req.Phone, Password: req.Password, RoleID: parseUUID(req.RoleID),
	})
	if err != nil {
		h.fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusCreated, user)
}

func (h *Handler) UpdateUser(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	var req updateUserRequest
	if !httpx.DecodeJSON(w, r, &req) {
		return
	}
	if req.Active == nil {
		httpx.ValidationError(w, map[string]string{"active": "REQUIRED"})
		return
	}
	user, err := h.svc.UpdateUser(r.Context(), actor(r), id, UpdateUserInput{
		Name: req.Name, Phone: req.Phone, RoleID: parseUUID(req.RoleID), Active: *req.Active,
	})
	if err != nil {
		h.fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, user)
}

func (h *Handler) ResetPassword(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	var req passwordRequest
	if !httpx.DecodeJSON(w, r, &req) {
		return
	}
	if err := h.svc.ResetPassword(r.Context(), actor(r), id, req.Password); err != nil {
		h.fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ---- util ----

func parseUUID(s string) uuid.UUID {
	id, _ := uuid.Parse(s) // tidak valid → uuid.Nil → ditolak validasi sebagai REQUIRED
	return id
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
	case errors.Is(err, ErrSystemRole):
		httpx.Error(w, http.StatusConflict, "SYSTEM_ROLE", "Role sistem tidak dapat diubah atau dihapus.")
	case errors.Is(err, ErrRoleInUse):
		httpx.Error(w, http.StatusConflict, "ROLE_IN_USE", "Role masih dipakai pengguna.")
	case errors.Is(err, ErrNameTaken):
		httpx.Error(w, http.StatusConflict, "NAME_TAKEN", "Nama role sudah dipakai.")
	case errors.Is(err, ErrEmailTaken):
		httpx.Error(w, http.StatusConflict, "EMAIL_TAKEN", "Email sudah terdaftar.")
	case errors.Is(err, ErrLastOwner):
		httpx.Error(w, http.StatusConflict, "LAST_OWNER", "Pemilik aktif terakhir tidak boleh dinonaktifkan atau diganti rolenya.")
	case errors.Is(err, ErrSelfChange):
		httpx.Error(w, http.StatusConflict, "SELF_CHANGE", "Tidak boleh menonaktifkan atau mengganti role akun sendiri.")
	default:
		h.log.Error("iam gagal", "err", err, "req_id", middleware.GetReqID(r.Context()))
		httpx.Error(w, http.StatusInternalServerError, "INTERNAL", "Terjadi kesalahan pada server.")
	}
}
