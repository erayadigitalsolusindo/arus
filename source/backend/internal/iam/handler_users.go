package iam

import (
	"net/http"

	"aciraba/internal/platform/httpx"
)

type createUserRequest struct {
	Name      string   `json:"name"`
	Email     string   `json:"email"`
	Phone     string   `json:"phone"`
	Password  string   `json:"password"`
	RoleID    string   `json:"role_id"`
	OutletIDs []string `json:"outlet_ids"`
}

type updateUserRequest struct {
	Name   string `json:"name"`
	Phone  string `json:"phone"`
	RoleID string `json:"role_id"`
	// Pointer: field yang hilang harus ditolak, bukan dianggap false (menonaktifkan akun tanpa sengaja).
	Active    *bool    `json:"active"`
	OutletIDs []string `json:"outlet_ids"`
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
		Name: req.Name, Email: req.Email, Phone: req.Phone, Password: req.Password,
		RoleID: parseUUID(req.RoleID), OutletIDs: parseUUIDs(req.OutletIDs),
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
		Name: req.Name, Phone: req.Phone, RoleID: parseUUID(req.RoleID), Active: *req.Active, OutletIDs: parseUUIDs(req.OutletIDs),
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

// VerifyEmail menandai email pengguna lain sebagai terverifikasi (manual, mis. saat email macet atau tanpa internet).
func (h *Handler) VerifyEmail(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	if err := h.svc.VerifyEmail(r.Context(), actor(r), id); err != nil {
		h.fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
