package platformadmin

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"aciraba/internal/authz"
	"aciraba/internal/platform/httpx"
	"aciraba/internal/platform/sanitize"
)

// Aksi audit role Platform Admin.
const (
	ActionRoleCreate    = "platform.role_create"
	ActionRoleUpdate    = "platform.role_update"
	ActionRoleDelete    = "platform.role_delete"
	ActionAdminRole     = "platform.admin_role"
	ActionTenantProfile = "platform.tenant_profile"

	maxRoleName = 50
)

var (
	ErrEscalation    = errors.New("izin melebihi izin Anda")
	ErrSystemRole    = errors.New("role sistem tidak dapat diubah")
	ErrRoleInUse     = errors.New("role masih dipakai admin")
	ErrRoleNotFound  = errors.New("role tidak ditemukan")
	ErrRoleNameTaken = errors.New("nama role sudah dipakai")
)

type Role struct {
	ID          uuid.UUID         `json:"id"`
	Name        string            `json:"name"`
	Permissions authz.Permissions `json:"permissions"`
	IsSystem    bool              `json:"is_system"`
	AdminCount  int               `json:"admin_count"`
}

// rolePerms = role & izin seorang admin. Tanpa role = tanpa izin (tertutup).
func (s *Service) rolePerms(ctx context.Context, adminID uuid.UUID) (uuid.UUID, string, authz.Permissions, error) {
	var (
		id   uuid.UUID
		name string
		raw  []byte
	)
	err := s.Pool.QueryRow(ctx, `SELECT r.id, r.name, r.permissions FROM platform_admin_roles ar
		JOIN platform_roles r ON r.id = ar.role_id WHERE ar.admin_id = $1`, adminID).Scan(&id, &name, &raw)
	if errors.Is(err, pgx.ErrNoRows) {
		return uuid.Nil, "", authz.Permissions{}, nil
	}
	if err != nil {
		return uuid.Nil, "", authz.Permissions{}, err
	}
	return id, name, authz.ParseStoredIn(PlatformModules, raw), nil
}

func (s *Service) roleByID(ctx context.Context, id uuid.UUID) (*Role, error) {
	var (
		r   Role
		raw []byte
	)
	err := s.Pool.QueryRow(ctx, `SELECT id, name, permissions, is_system FROM platform_roles WHERE id = $1`, id).Scan(&r.ID, &r.Name, &raw, &r.IsSystem)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrRoleNotFound
	}
	if err != nil {
		return nil, err
	}
	r.Permissions = authz.ParseStoredIn(PlatformModules, raw)
	return &r, nil
}

func (s *Service) ListRoles(ctx context.Context) ([]Role, error) {
	rows, err := s.Pool.Query(ctx, `SELECT r.id, r.name, r.permissions, r.is_system,
		(SELECT count(*) FROM platform_admin_roles ar WHERE ar.role_id = r.id)
		FROM platform_roles r ORDER BY r.is_system DESC, lower(r.name)`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Role{}
	for rows.Next() {
		var (
			r   Role
			raw []byte
			n   int64
		)
		if err := rows.Scan(&r.ID, &r.Name, &raw, &r.IsSystem, &n); err != nil {
			return nil, err
		}
		r.Permissions, r.AdminCount = authz.ParseStoredIn(PlatformModules, raw), int(n)
		out = append(out, r)
	}
	return out, rows.Err()
}

// AssignableRoles = role yang boleh diberikan pemanggil (izinnya tidak melebihi izin pemanggil).
func (s *Service) AssignableRoles(ctx context.Context, a Actor) ([]Role, error) {
	all, err := s.ListRoles(ctx)
	if err != nil {
		return nil, err
	}
	out := all[:0]
	for _, r := range all {
		if a.Perms.Covers(r.Permissions) {
			out = append(out, r)
		}
	}
	return out, nil
}

// validRole memvalidasi input. Wildcard tidak pernah bisa dibuat lewat API (Normalize menolak; DB menjaga dengan CHECK).
func validRole(name string, perms map[string][]string) (string, authz.Permissions, map[string]string) {
	f := map[string]string{}
	n, code := sanitize.Name(name, maxRoleName)
	if code != "" {
		f["name"] = code
	}
	p, err := authz.NormalizeIn(PlatformModules, perms)
	if err != nil {
		f["permissions"] = "INVALID"
	}
	if len(f) > 0 {
		return "", authz.Permissions{}, f
	}
	return n, p, nil
}

func (s *Service) SaveRole(ctx context.Context, a Actor, id uuid.UUID, name string, perms map[string][]string) (*Role, error) {
	n, p, f := validRole(name, perms)
	if f != nil {
		return nil, FieldErrors(f)
	}
	if !a.Perms.Covers(p) {
		return nil, ErrEscalation
	}
	var before *Role
	if id != uuid.Nil {
		cur, err := s.roleByID(ctx, id)
		if err != nil {
			return nil, err
		}
		if cur.IsSystem {
			return nil, ErrSystemRole
		}
		// Role yang melebihi izin pemanggil tidak boleh disentuh (mencegah menurunkan/mengambil alih role atasan).
		if !a.Perms.Covers(cur.Permissions) {
			return nil, ErrEscalation
		}
		before = cur
	}
	raw, _ := json.Marshal(p)
	var arg pgtype.UUID
	if id != uuid.Nil {
		arg = pgtype.UUID{Bytes: id, Valid: true}
	}
	var out pgtype.UUID
	err := s.Pool.QueryRow(ctx, `SELECT platform_role_save($1, $2, $3, $4)`, a.ID, arg, n, raw).Scan(&out)
	switch {
	case pgCode(err) == "23505":
		return nil, ErrRoleNameTaken
	case pgCode(err) == "P0001":
		return nil, ErrSystemRole
	case pgCode(err) == "42501":
		return nil, ErrInvalidSession
	case err != nil:
		return nil, err
	case !out.Valid:
		return nil, ErrRoleNotFound
	}
	s.forgetAll() // izin semua pemegang role berubah
	rid := uuid.UUID(out.Bytes)
	d := map[string]any{"role_id": rid.String(), "name": n, "permissions": p.Grants}
	action := ActionRoleCreate
	if before != nil {
		action = ActionRoleUpdate
		d["before"] = map[string]any{"name": before.Name, "permissions": before.Permissions.Grants}
	}
	s.record(ctx, a, action, uuid.Nil, "", d)
	return &Role{ID: rid, Name: n, Permissions: p}, nil
}

func (s *Service) DeleteRole(ctx context.Context, a Actor, id uuid.UUID) error {
	cur, err := s.roleByID(ctx, id)
	if err != nil {
		return err
	}
	if cur.IsSystem {
		return ErrSystemRole
	}
	if !a.Perms.Covers(cur.Permissions) {
		return ErrEscalation
	}
	var ok bool
	err = s.Pool.QueryRow(ctx, `SELECT platform_role_delete($1, $2)`, a.ID, id).Scan(&ok)
	switch {
	case pgCode(err) == "23503":
		return ErrRoleInUse
	case pgCode(err) == "P0001":
		return ErrSystemRole
	case pgCode(err) == "42501":
		return ErrInvalidSession
	case err != nil:
		return err
	case !ok:
		return ErrRoleNotFound
	}
	s.record(ctx, a, ActionRoleDelete, uuid.Nil, "", map[string]any{"role_id": id.String(), "name": cur.Name})
	return nil
}

// canManage: pemanggil hanya boleh mengelola (ubah role/password/2FA/status/hapus) admin yang izinnya tercakup izinnya sendiri.
// Tanpa ini pemegang izin "admins" bisa mereset password Owner lalu mengambil alih akunnya.
func (s *Service) canManage(ctx context.Context, a Actor, target uuid.UUID) error {
	if a.ID == target {
		return nil
	}
	_, _, tp, err := s.rolePerms(ctx, target)
	if err != nil {
		return err
	}
	if !a.Perms.Covers(tp) {
		return ErrEscalation
	}
	return nil
}

// SetAdminRole mengganti role admin lain. Role diri sendiri tidak boleh diubah.
func (s *Service) SetAdminRole(ctx context.Context, a Actor, target, roleID uuid.UUID) error {
	if a.ID == target {
		return ErrSelf
	}
	if err := s.canManage(ctx, a, target); err != nil {
		return err
	}
	role, err := s.roleByID(ctx, roleID)
	if err != nil {
		return err
	}
	if !a.Perms.Covers(role.Permissions) {
		return ErrEscalation
	}
	var ok bool
	err = s.Pool.QueryRow(ctx, `SELECT platform_admin_set_role($1, $2, $3)`, a.ID, target, roleID).Scan(&ok)
	switch {
	case pgCode(err) == "42501":
		return ErrInvalidSession
	case pgCode(err) == "23503":
		return ErrRoleNotFound
	case pgCode(err) == "P0001":
		return ErrLastAdmin
	case err != nil:
		return err
	case !ok:
		return ErrNotFound
	}
	s.forget(target)
	s.record(ctx, a, ActionAdminRole, uuid.Nil, "", map[string]any{"target_id": target.String(), "role_id": roleID.String(), "role": role.Name})
	return nil
}

// ---- HTTP ----

type roleRequest struct {
	Name        string              `json:"name"`
	Permissions map[string][]string `json:"permissions"`
}

func (h *Handler) roleFail(w http.ResponseWriter, r *http.Request, err error) {
	var fe FieldErrors
	switch {
	case errors.As(err, &fe):
		httpx.ValidationError(w, fe)
	case errors.Is(err, ErrEscalation):
		httpx.Error(w, http.StatusForbidden, "PERMISSION_ESCALATION", "Izin melebihi izin Anda sendiri.")
	case errors.Is(err, ErrSystemRole):
		httpx.Error(w, http.StatusConflict, "SYSTEM_ROLE", "Role sistem tidak dapat diubah atau dihapus.")
	case errors.Is(err, ErrRoleInUse):
		httpx.Error(w, http.StatusConflict, "ROLE_IN_USE", "Role masih dipakai admin. Pindahkan adminnya dulu.")
	case errors.Is(err, ErrRoleNameTaken):
		httpx.Error(w, http.StatusConflict, "NAME_TAKEN", "Nama role sudah dipakai.")
	case errors.Is(err, ErrRoleNotFound):
		httpx.Error(w, http.StatusNotFound, "NOT_FOUND", "Role tidak ditemukan.")
	case errors.Is(err, ErrInvalidSession):
		httpx.Error(w, http.StatusUnauthorized, "UNAUTHORIZED", "Sesi tidak valid.")
	default:
		h.internal(w, r, "role platform gagal", err)
	}
}

func (h *Handler) ListRoles(w http.ResponseWriter, r *http.Request) {
	roles, err := h.svc.ListRoles(r.Context())
	if err != nil {
		h.roleFail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"roles": roles})
}

func (h *Handler) RoleModules(w http.ResponseWriter, r *http.Request) {
	httpx.JSON(w, http.StatusOK, map[string]any{"modules": PlatformModules})
}

func (h *Handler) AssignableRoles(w http.ResponseWriter, r *http.Request) {
	roles, err := h.svc.AssignableRoles(r.Context(), actor(r))
	if err != nil {
		h.roleFail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"roles": roles})
}

func (h *Handler) CreateRole(w http.ResponseWriter, r *http.Request) {
	var req roleRequest
	if !httpx.DecodeJSON(w, r, &req) {
		return
	}
	role, err := h.svc.SaveRole(r.Context(), actor(r), uuid.Nil, req.Name, req.Permissions)
	if err != nil {
		h.roleFail(w, r, err)
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
	role, err := h.svc.SaveRole(r.Context(), actor(r), id, req.Name, req.Permissions)
	if err != nil {
		h.roleFail(w, r, err)
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
		h.roleFail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) SetAdminRole(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	var req struct {
		RoleID string `json:"role_id"`
	}
	if !httpx.DecodeJSON(w, r, &req) {
		return
	}
	rid, err := uuid.Parse(req.RoleID)
	if err != nil {
		httpx.ValidationError(w, map[string]string{"role_id": "INVALID"})
		return
	}
	switch err := h.svc.SetAdminRole(r.Context(), actor(r), id, rid); {
	case errors.Is(err, ErrNotFound):
		httpx.Error(w, http.StatusNotFound, "NOT_FOUND", "Admin tidak ditemukan.")
	case errors.Is(err, ErrSelf):
		httpx.Error(w, http.StatusConflict, "CANNOT_CHANGE_SELF", "Role akun sendiri tidak dapat diubah.")
	case errors.Is(err, ErrLastAdmin):
		httpx.Error(w, http.StatusConflict, "LAST_OWNER", "Harus tersisa minimal satu Owner aktif.")
	case err != nil:
		h.roleFail(w, r, err)
	default:
		w.WriteHeader(http.StatusNoContent)
	}
}

// guardTarget: admin yang dituju {id} harus tercakup izin pemanggil (lihat canManage). Dipasang di rute yang mengubah admin lain.
func (h *Handler) guardTarget(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id, err := uuid.Parse(chi.URLParam(r, "id"))
		if err != nil {
			httpx.Error(w, http.StatusNotFound, "NOT_FOUND", "Tidak ditemukan.")
			return
		}
		switch err := h.svc.canManage(r.Context(), actor(r), id); {
		case errors.Is(err, ErrEscalation):
			httpx.Error(w, http.StatusForbidden, "PERMISSION_ESCALATION", "Admin ini memiliki izin melebihi izin Anda.")
		case err != nil:
			h.internal(w, r, "periksa target admin gagal", err)
		default:
			next.ServeHTTP(w, r)
		}
	})
}
