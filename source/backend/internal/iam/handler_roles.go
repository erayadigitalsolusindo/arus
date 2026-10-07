package iam

import (
	"net/http"

	"aciraba/internal/platform/httpx"
)

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

func (h *Handler) AssignableRoles(w http.ResponseWriter, r *http.Request) {
	roles, err := h.svc.AssignableRoles(r.Context(), actor(r))
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
