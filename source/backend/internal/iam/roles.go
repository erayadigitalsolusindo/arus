package iam

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"aciraba/internal/audit"
	"aciraba/internal/authz"
	gen "aciraba/internal/gen"
	"aciraba/internal/platform/db"
	"aciraba/internal/platform/sanitize"
)

type Role struct {
	ID          uuid.UUID         `json:"id"`
	Name        string            `json:"name"`
	Permissions authz.Permissions `json:"permissions"`
	IsSystem    bool              `json:"is_system"`
	UserCount   int               `json:"user_count"`
}

func (s *Service) ListRoles(ctx context.Context, tenantID uuid.UUID) ([]Role, error) {
	out := []Role{}
	err := db.WithTenant(ctx, s.pool, tenantID, func(tx pgx.Tx) error {
		rows, err := gen.New(tx).IamListRoles(ctx, tenantID)
		for _, r := range rows {
			out = append(out, Role{ID: r.ID, Name: r.Name, Permissions: authz.ParseStored(r.Permissions), IsSystem: r.IsSystem, UserCount: int(r.UserCount)})
		}
		return err
	})
	return out, err
}

// AssignableRoles = role yang boleh diberikan pemanggil ke pengguna lain (izinnya tidak melebihi izin pemanggil).
// Dipakai dropdown role di halaman Pengguna, sehingga pemegang `users` tidak perlu izin `roles`.
func (s *Service) AssignableRoles(ctx context.Context, actor authz.Actor) ([]Role, error) {
	all, err := s.ListRoles(ctx, actor.TenantID)
	if err != nil {
		return nil, err
	}
	out := all[:0]
	for _, r := range all {
		if actor.Perms.Covers(r.Permissions) {
			out = append(out, r)
		}
	}
	return out, nil
}

// validRoleInput memvalidasi input role. Wildcard ("*", akses penuh) tidak pernah bisa dibuat lewat API: hanya role sistem
// Owner yang memilikinya (Normalize menolak "*"; DB menjaga dengan CHECK roles_wildcard_only_system).
func validRoleInput(name string, perms map[string][]string) (string, authz.Permissions, FieldErrors) {
	f := FieldErrors{}
	n, code := sanitize.Name(name, maxRoleName)
	setCode(f, "name", code)
	p, err := authz.Normalize(perms)
	if err != nil {
		f["permissions"] = "INVALID"
	}
	if len(f) > 0 {
		return "", authz.Permissions{}, f
	}
	return n, p, nil
}

func (s *Service) CreateRole(ctx context.Context, actor authz.Actor, name string, perms map[string][]string) (*Role, error) {
	n, p, f := validRoleInput(name, perms)
	if f != nil {
		return nil, f
	}
	if !actor.Perms.Covers(p) {
		return nil, ErrEscalation
	}
	raw, _ := json.Marshal(p)
	var row gen.IamCreateRoleRow
	err := db.WithTenant(ctx, s.pool, actor.TenantID, func(tx pgx.Tx) error {
		var qerr error
		row, qerr = gen.New(tx).IamCreateRole(ctx, gen.IamCreateRoleParams{TenantID: actor.TenantID, Name: n, Permissions: raw})
		if qerr != nil {
			return qerr
		}
		return audit.Record(ctx, tx, audit.FromActor(actor), audit.Entry{
			Action: audit.ActionRoleCreate, Entity: audit.EntityRole, EntityID: row.ID.String(),
			Details: map[string]any{"name": n, "permissions": p.Grants},
		})
	})
	if isUnique(err, "roles_tenant_name_key") {
		return nil, ErrNameTaken
	}
	if err != nil {
		return nil, err
	}
	s.resolver.InvalidateTenant(actor.TenantID)
	return &Role{ID: row.ID, Name: row.Name, Permissions: p, IsSystem: row.IsSystem}, nil
}

func (s *Service) UpdateRole(ctx context.Context, actor authz.Actor, id uuid.UUID, name string, perms map[string][]string) (*Role, error) {
	n, p, f := validRoleInput(name, perms)
	if f != nil {
		return nil, f
	}
	if !actor.Perms.Covers(p) {
		return nil, ErrEscalation
	}
	raw, _ := json.Marshal(p)
	var row gen.IamUpdateRoleRow
	err := db.WithTenant(ctx, s.pool, actor.TenantID, func(tx pgx.Tx) error {
		q := gen.New(tx)
		cur, err := q.IamGetRole(ctx, gen.IamGetRoleParams{TenantID: actor.TenantID, ID: id})
		if err != nil {
			return err
		}
		if cur.IsSystem {
			return ErrSystemRole
		}
		// Role yang melebihi izin pemanggil tidak boleh disentuh (mencegah menurunkan/mengambil alih role atasan).
		curPerms := authz.ParseStored(cur.Permissions)
		if !actor.Perms.Covers(curPerms) {
			return ErrEscalation
		}
		row, err = q.IamUpdateRole(ctx, gen.IamUpdateRoleParams{TenantID: actor.TenantID, ID: id, Name: n, Permissions: raw})
		if err != nil {
			return err
		}
		return audit.Record(ctx, tx, audit.FromActor(actor), audit.Entry{
			Action: audit.ActionRoleUpdate, Entity: audit.EntityRole, EntityID: id.String(),
			Details: map[string]any{
				"before": map[string]any{"name": cur.Name, "permissions": curPerms.Grants},
				"after":  map[string]any{"name": n, "permissions": p.Grants},
			},
		})
	})
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		return nil, ErrNotFound
	case isUnique(err, "roles_tenant_name_key"):
		return nil, ErrNameTaken
	case err != nil:
		return nil, err
	}
	s.resolver.InvalidateTenant(actor.TenantID)
	return &Role{ID: row.ID, Name: row.Name, Permissions: p, IsSystem: row.IsSystem}, nil
}

func (s *Service) DeleteRole(ctx context.Context, actor authz.Actor, id uuid.UUID) error {
	err := db.WithTenant(ctx, s.pool, actor.TenantID, func(tx pgx.Tx) error {
		q := gen.New(tx)
		cur, err := q.IamGetRole(ctx, gen.IamGetRoleParams{TenantID: actor.TenantID, ID: id})
		if err != nil {
			return err
		}
		if cur.IsSystem {
			return ErrSystemRole
		}
		if !actor.Perms.Covers(authz.ParseStored(cur.Permissions)) {
			return ErrEscalation
		}
		if _, err = q.IamDeleteRole(ctx, gen.IamDeleteRoleParams{TenantID: actor.TenantID, ID: id}); err != nil {
			return err
		}
		return audit.Record(ctx, tx, audit.FromActor(actor), audit.Entry{
			Action: audit.ActionRoleDelete, Entity: audit.EntityRole, EntityID: id.String(),
			Details: map[string]any{"name": cur.Name},
		})
	})
	var pgErr *pgconn.PgError
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		return ErrNotFound
	case errors.As(err, &pgErr) && (pgErr.Code == "23503" || pgErr.Code == "23001"): // users_role_fk (RESTRICT)
		return ErrRoleInUse
	case err != nil:
		return err
	}
	s.resolver.InvalidateTenant(actor.TenantID)
	return nil
}
