package storefront

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"

	"aciraba/internal/authz"
	"aciraba/internal/platform/httpx"
)

// Host milik tenant: domain didaftarkan Platform Admin; tenant hanya memilih peran tiap host (aplikasi / landing / keduanya)
// dan menambah sub-host di bawah domain aktifnya. Pendaftaran domain induk + verifikasi DNS tetap di Platform Admin.

var (
	ErrDomainBase  = errors.New("domain induk tidak sah")
	ErrDomainLabel = errors.New("nama sub-domain tidak valid")
	ErrDomainLimit = errors.New("batas sub-domain tercapai")
	ErrDomainTaken = errors.New("host sudah dipakai")
	ErrDomainRole  = errors.New("host harus melayani aplikasi, landing page, atau keduanya")
)

type Domain struct {
	ID         uuid.UUID `json:"id"`
	Host       string    `json:"host"`
	Status     string    `json:"status"`
	App        bool      `json:"app"`
	Landing    bool      `json:"landing"`
	TenantMade bool      `json:"tenant_made"`
}

func (s *Service) Domains(ctx context.Context, a authz.Actor) ([]Domain, error) {
	rows, err := s.pool.Query(ctx, `SELECT id, host, status, app_enabled, landing, tenant_made FROM tenant_domain_list($1)`, a.TenantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Domain{}
	for rows.Next() {
		var d Domain
		if err := rows.Scan(&d.ID, &d.Host, &d.Status, &d.App, &d.Landing, &d.TenantMade); err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

func (s *Service) AddSubdomain(ctx context.Context, a authz.Actor, base, label string) (uuid.UUID, error) {
	var id uuid.UUID
	err := s.pool.QueryRow(ctx, `SELECT tenant_domain_add_sub($1, $2, $3)`, a.TenantID, strings.ToLower(strings.TrimSpace(base)), strings.ToLower(strings.TrimSpace(label))).Scan(&id)
	var pe *pgconn.PgError
	if errors.As(err, &pe) {
		switch {
		case pe.Code == "23505":
			return uuid.Nil, ErrDomainTaken
		case pe.Message == "DOMAIN_LABEL":
			return uuid.Nil, ErrDomainLabel
		case pe.Message == "DOMAIN_BASE":
			return uuid.Nil, ErrDomainBase
		case pe.Message == "DOMAIN_LIMIT":
			return uuid.Nil, ErrDomainLimit
		}
	}
	return id, err
}

func (s *Service) SetDomainRole(ctx context.Context, a authz.Actor, id uuid.UUID, app, landing bool) error {
	var ok bool
	err := s.pool.QueryRow(ctx, `SELECT tenant_domain_set_role($1, $2, $3, $4)`, a.TenantID, id, app, landing).Scan(&ok)
	var pe *pgconn.PgError
	if errors.As(err, &pe) && pe.Code == "23514" {
		return ErrDomainRole
	}
	if err == nil && !ok {
		return ErrNotFound
	}
	return err
}

func (s *Service) DeleteSubdomain(ctx context.Context, a authz.Actor, id uuid.UUID) error {
	var ok bool
	if err := s.pool.QueryRow(ctx, `SELECT tenant_domain_delete_sub($1, $2)`, a.TenantID, id).Scan(&ok); err != nil {
		return err
	}
	if !ok {
		return ErrNotFound
	}
	return nil
}

// ---- HTTP ----

func (h *Handler) domainsRoutes(r chi.Router) {
	r.With(authz.Require("storefront", authz.ActView)).Get("/domains", h.domainList)
	r.With(authz.Require("storefront", authz.ActUpdate)).Post("/domains", h.domainAdd)
	r.With(authz.Require("storefront", authz.ActUpdate)).Put("/domains/{id}", h.domainRole)
	r.With(authz.Require("storefront", authz.ActUpdate)).Delete("/domains/{id}", h.domainDelete)
}

func (h *Handler) domainFail(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, ErrDomainLabel):
		httpx.Error(w, http.StatusUnprocessableEntity, "DOMAIN_LABEL_INVALID", "Nama sub-domain hanya huruf kecil, angka, dan strip (maks 63 karakter).")
	case errors.Is(err, ErrDomainBase):
		httpx.Error(w, http.StatusUnprocessableEntity, "DOMAIN_BASE_INVALID", "Domain induk belum terdaftar/aktif. Minta admin ARUS mendaftarkan dan mengaktifkan domain Anda.")
	case errors.Is(err, ErrDomainLimit):
		httpx.Error(w, http.StatusUnprocessableEntity, "DOMAIN_LIMIT", "Batas 10 sub-domain buatan sendiri tercapai; hapus salah satu dulu.")
	case errors.Is(err, ErrDomainTaken):
		httpx.Error(w, http.StatusConflict, "DOMAIN_TAKEN", "Host ini sudah dipakai.")
	case errors.Is(err, ErrDomainRole):
		httpx.Error(w, http.StatusUnprocessableEntity, "DOMAIN_ROLE_INVALID", "Host harus melayani aplikasi, landing page, atau keduanya.")
	case errors.Is(err, ErrNotFound):
		httpx.Error(w, http.StatusNotFound, "NOT_FOUND", "Host tidak ditemukan atau belum aktif.")
	default:
		h.log.ErrorContext(r.Context(), "storefront: domain gagal", "err", err)
		httpx.Error(w, http.StatusInternalServerError, "INTERNAL", "Terjadi kesalahan.")
	}
}

func (h *Handler) domainList(w http.ResponseWriter, r *http.Request) {
	ds, err := h.svc.Domains(r.Context(), actor(r))
	if err != nil {
		h.domainFail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"data": ds, "target": h.domainTarget})
}

func (h *Handler) domainAdd(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Base  string `json:"base"`
		Label string `json:"label"`
	}
	if !httpx.DecodeJSON(w, r, &req) {
		return
	}
	id, err := h.svc.AddSubdomain(r.Context(), actor(r), req.Base, req.Label)
	if err != nil {
		h.domainFail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusCreated, map[string]any{"id": id})
}

func (h *Handler) domainRole(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		h.domainFail(w, r, ErrNotFound)
		return
	}
	var req struct {
		Mode string `json:"mode"` // app | landing | both
	}
	if !httpx.DecodeJSON(w, r, &req) {
		return
	}
	if req.Mode != "app" && req.Mode != "landing" && req.Mode != "both" {
		h.domainFail(w, r, ErrDomainRole)
		return
	}
	if err := h.svc.SetDomainRole(r.Context(), actor(r), id, req.Mode != "landing", req.Mode != "app"); err != nil {
		h.domainFail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) domainDelete(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		h.domainFail(w, r, ErrNotFound)
		return
	}
	if err := h.svc.DeleteSubdomain(r.Context(), actor(r), id); err != nil {
		h.domainFail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
