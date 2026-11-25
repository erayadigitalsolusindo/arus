package platformadmin

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"

	gen "aciraba/internal/gen"
)

type AdminRow struct {
	ID          uuid.UUID  `json:"id"`
	Email       string     `json:"email"`
	Name        string     `json:"name"`
	Active      bool       `json:"active"`
	LastLoginAt *time.Time `json:"last_login_at"`
	CreatedAt   time.Time  `json:"created_at"`
	MFAEnabled  bool       `json:"mfa_enabled"`
	RoleID      *uuid.UUID `json:"role_id"`
	RoleName    string     `json:"role_name"`
	// TenantCount = jumlah tenant yang boleh dilihat; nil = semua tenant.
	TenantCount *int `json:"tenant_count"`
}

func (s *Service) ListAdmins(ctx context.Context) ([]AdminRow, error) {
	rows, err := s.Pool.Query(ctx, `SELECT a.id, a.email, a.name, a.active, a.last_login_at, a.created_at,
		(a.totp_enabled_at IS NOT NULL), r.id, r.name, cardinality(a.tenant_scope)
		FROM platform_admins a
		LEFT JOIN platform_admin_roles ar ON ar.admin_id = a.id
		LEFT JOIN platform_roles r ON r.id = ar.role_id
		ORDER BY a.created_at, a.id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []AdminRow{}
	for rows.Next() {
		var (
			a    AdminRow
			last pgtype.Timestamptz
			rid  pgtype.UUID
			rn   pgtype.Text
			tc   pgtype.Int4
		)
		if err := rows.Scan(&a.ID, &a.Email, &a.Name, &a.Active, &last, &a.CreatedAt, &a.MFAEnabled, &rid, &rn, &tc); err != nil {
			return nil, err
		}
		if last.Valid {
			t := last.Time
			a.LastLoginAt = &t
		}
		if rid.Valid {
			id := uuid.UUID(rid.Bytes)
			a.RoleID = &id
		}
		a.RoleName = rn.String
		if tc.Valid {
			n := int(tc.Int32)
			a.TenantCount = &n
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

// pgCode mengembalikan SQLSTATE galat Postgres ("" bila bukan galat Postgres).
func pgCode(err error) string {
	var pe *pgconn.PgError
	if errors.As(err, &pe) {
		return pe.Code
	}
	return ""
}

// CreateAdmin menambah Platform Admin. Penulisan lewat fungsi DB yang memverifikasi pelaku adalah admin aktif.
func (s *Service) CreateAdmin(ctx context.Context, a Actor, name, email, password string, roleID uuid.UUID) (uuid.UUID, error) {
	role, err := s.roleByID(ctx, roleID)
	if err != nil {
		return uuid.Nil, err
	}
	if !a.Perms.Covers(role.Permissions) {
		return uuid.Nil, ErrEscalation
	}
	hash, err := s.hash(ctx, password)
	if err != nil {
		return uuid.Nil, err
	}
	var id pgtype.UUID
	err = s.Pool.QueryRow(ctx, `SELECT platform_admin_create($1, $2, $3, $4, $5)`, a.ID, email, name, hash, roleID).Scan(&id)
	switch pgCode(err) {
	case "23505":
		return uuid.Nil, ErrEmailTaken
	case "42501":
		return uuid.Nil, ErrInvalidSession
	}
	if err != nil {
		return uuid.Nil, err
	}
	s.record(ctx, a, ActionAdminCreate, uuid.Nil, "", map[string]any{"target_id": uuid.UUID(id.Bytes).String(), "email": email, "role": role.Name})
	return id.Bytes, nil
}

// SetAdminActive mengaktifkan/menonaktifkan admin. Penonaktifan mencabut token akses (tokens_valid_after) dan semua sesi.
func (s *Service) SetAdminActive(ctx context.Context, a Actor, target uuid.UUID, active bool) error {
	var ok bool
	err := s.Pool.QueryRow(ctx, `SELECT platform_admin_set_active($1, $2, $3, $4)`, a.ID, target, active, s.now()).Scan(&ok)
	switch {
	case pgCode(err) == "42501":
		return ErrInvalidSession
	case pgCode(err) == "P0001" && strings.Contains(err.Error(), "diri sendiri"):
		return ErrSelf
	case pgCode(err) == "P0001":
		return ErrLastAdmin
	case err != nil:
		return err
	case !ok:
		return ErrNotFound
	}
	s.forget(target)
	if !active {
		if err := s.Sessions.RevokeUser(ctx, target.String()); err != nil {
			return err
		}
	}
	s.record(ctx, a, ActionAdminStatus, uuid.Nil, "", map[string]any{"target_id": target.String(), "active": active})
	return nil
}

var ErrAdminHasHistory = errors.New("admin punya riwayat checklist QA; nonaktifkan saja")

// DeleteAdmin menghapus admin permanen (lihat fungsi DB untuk aturannya). Sesi dicabut sebelum baris hilang.
func (s *Service) DeleteAdmin(ctx context.Context, a Actor, target uuid.UUID) error {
	var ok bool
	err := s.Pool.QueryRow(ctx, `SELECT platform_admin_delete($1, $2)`, a.ID, target).Scan(&ok)
	switch {
	case pgCode(err) == "42501":
		return ErrInvalidSession
	case pgCode(err) == "23503":
		return ErrAdminHasHistory
	case pgCode(err) == "P0001" && strings.Contains(err.Error(), "diri sendiri"):
		return ErrSelf
	case pgCode(err) == "P0001":
		return ErrLastAdmin
	case err != nil:
		return err
	case !ok:
		return ErrNotFound
	}
	s.forget(target)
	if err := s.Sessions.RevokeUser(ctx, target.String()); err != nil {
		return err
	}
	s.record(ctx, a, ActionAdminDelete, uuid.Nil, "", map[string]any{"target_id": target.String()})
	return nil
}

// SetAdminPassword mengganti password admin (termasuk diri sendiri) dan mencabut semua sesinya.
func (s *Service) SetAdminPassword(ctx context.Context, a Actor, target uuid.UUID, password string) error {
	hash, err := s.hash(ctx, password)
	if err != nil {
		return err
	}
	var ok bool
	err = s.Pool.QueryRow(ctx, `SELECT platform_admin_set_password($1, $2, $3, $4)`, a.ID, target, hash, s.now()).Scan(&ok)
	switch {
	case pgCode(err) == "42501":
		return ErrInvalidSession
	case err != nil:
		return err
	case !ok:
		return ErrNotFound
	}
	s.forget(target)
	if err := s.Sessions.RevokeUser(ctx, target.String()); err != nil {
		return err
	}
	s.record(ctx, a, ActionAdminPassword, uuid.Nil, "", map[string]any{"target_id": target.String()})
	return nil
}

// ---- audit platform ----

type AuditFilter struct {
	TenantID     uuid.UUID
	AdminID      uuid.UUID
	ActionPrefix string
	BeforeID     int64
	Limit        int
}

type AuditItem struct {
	ID         int64          `json:"id"`
	AdminID    *uuid.UUID     `json:"admin_id"`
	AdminName  string         `json:"admin_name"`
	Action     string         `json:"action"`
	TenantID   *uuid.UUID     `json:"tenant_id"`
	TenantName string         `json:"tenant_name"`
	Details    map[string]any `json:"details"`
	IP         string         `json:"ip"`
	CreatedAt  time.Time      `json:"created_at"`
}

type AuditPage struct {
	Items      []AuditItem `json:"items"`
	NextBefore int64       `json:"next_before,omitempty"`
}

func (s *Service) Audit(ctx context.Context, f AuditFilter) (*AuditPage, error) {
	limit := f.Limit
	if limit <= 0 {
		limit = 50
	}
	limit = min(limit, 200)
	p := gen.PlatformAuditListParams{MaxRows: int32(limit + 1)}
	if f.TenantID != uuid.Nil {
		p.TenantID = pgtype.UUID{Bytes: f.TenantID, Valid: true}
	}
	if f.AdminID != uuid.Nil {
		p.AdminID = pgtype.UUID{Bytes: f.AdminID, Valid: true}
	}
	if f.ActionPrefix != "" {
		r := strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`)
		p.ActionPrefix = pgtype.Text{String: r.Replace(f.ActionPrefix), Valid: true}
	}
	if f.BeforeID > 0 {
		p.BeforeID = pgtype.Int8{Int64: f.BeforeID, Valid: true}
	}
	rows, err := gen.New(s.Pool).PlatformAuditList(ctx, p)
	if err != nil {
		return nil, err
	}
	page := &AuditPage{Items: make([]AuditItem, 0, min(len(rows), limit))}
	for i, r := range rows {
		if i == limit {
			page.NextBefore = page.Items[limit-1].ID
			break
		}
		it := AuditItem{ID: r.ID, AdminName: r.AdminName, Action: r.Action, TenantName: r.TenantName, IP: r.Ip, CreatedAt: r.CreatedAt.Time, Details: map[string]any{}}
		if r.AdminID.Valid {
			id := uuid.UUID(r.AdminID.Bytes)
			it.AdminID = &id
		}
		if r.TenantID.Valid {
			id := uuid.UUID(r.TenantID.Bytes)
			it.TenantID = &id
		}
		_ = json.Unmarshal(r.Details, &it.Details)
		page.Items = append(page.Items, it)
	}
	return page, nil
}
