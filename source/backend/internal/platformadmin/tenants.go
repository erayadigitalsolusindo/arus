package platformadmin

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"aciraba/internal/audit"
	"aciraba/internal/auth"
	"aciraba/internal/authz"
	gen "aciraba/internal/gen"
	pauth "aciraba/internal/platform/auth"
	"aciraba/internal/platform/db"
)

const (
	defaultPage = 25
	maxPage     = 100
)

// TenantRow = satu baris daftar tenant.
type TenantRow struct {
	ID          uuid.UUID  `json:"id"`
	Code        string     `json:"code"`
	Name        string     `json:"name"`
	Active      bool       `json:"active"`
	CreatedAt   time.Time  `json:"created_at"`
	OutletCount int64      `json:"outlet_count"`
	UserCount   int64      `json:"user_count"`
	LastLoginAt *time.Time `json:"last_login_at"`
	OwnerEmail  string     `json:"owner_email"`
}

type TenantPage struct {
	Items  []TenantRow `json:"items"`
	Total  int64       `json:"total"`
	Limit  int         `json:"limit"`
	Offset int         `json:"offset"`
}

// ListTenants memanggil fungsi DB yang melewati RLS (hanya untuk Platform Admin aktif; diverifikasi di DB).
func (s *Service) ListTenants(ctx context.Context, a Actor, q string, limit, offset int) (*TenantPage, error) {
	if limit <= 0 {
		limit = defaultPage
	}
	limit, offset = min(limit, maxPage), max(offset, 0)
	rows, err := s.Pool.Query(ctx,
		`SELECT id, code, name, active, created_at, outlet_count, user_count, last_login_at, owner_email, total
		   FROM platform_list_tenants($1, $2, $3, $4)`, a.ID, q, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	page := &TenantPage{Items: []TenantRow{}, Limit: limit, Offset: offset}
	for rows.Next() {
		var (
			r     TenantRow
			last  pgtype.Timestamptz
			owner pgtype.Text
		)
		if err := rows.Scan(&r.ID, &r.Code, &r.Name, &r.Active, &r.CreatedAt, &r.OutletCount, &r.UserCount, &last, &owner, &page.Total); err != nil {
			return nil, err
		}
		if last.Valid {
			t := last.Time
			r.LastLoginAt = &t
		}
		r.OwnerEmail = owner.String
		page.Items = append(page.Items, r)
	}
	return page, rows.Err()
}

type OutletRow struct {
	ID     uuid.UUID `json:"id"`
	Code   string    `json:"code"`
	Name   string    `json:"name"`
	Active bool      `json:"active"`
}

type UserRow struct {
	ID            uuid.UUID  `json:"id"`
	Name          string     `json:"name"`
	Email         string     `json:"email"`
	Active        bool       `json:"active"`
	RoleName      string     `json:"role_name"`
	LastLoginAt   *time.Time `json:"last_login_at"`
	EmailVerified bool       `json:"email_verified"`
	CreatedAt     time.Time  `json:"created_at"`
}

type TenantDetail struct {
	ID        uuid.UUID `json:"id"`
	Code      string    `json:"code"`
	Name      string    `json:"name"`
	Active    bool      `json:"active"`
	CreatedAt time.Time `json:"created_at"`
	// SaleEditWindowDays = berapa hari sesudah hari nota dibuat nota masih boleh diedit/dibatalkan (0 = hanya hari itu). Hanya operator platform yang mengubahnya.
	SaleEditWindowDays int         `json:"sale_edit_window_days"`
	Outlets            []OutletRow `json:"outlets"`
	Users              []UserRow   `json:"users"`
}

// Tenant memuat detail satu tenant di bawah WithTenant (RLS membatasi ke tenant itu). tenantID berasal dari path rute
// Platform Admin yang sudah terautentikasi, bukan dari token tenant.
func (s *Service) Tenant(ctx context.Context, id uuid.UUID) (*TenantDetail, error) {
	var out *TenantDetail
	err := db.WithTenant(ctx, s.Pool, id, func(tx pgx.Tx) error {
		q := gen.New(tx)
		t, err := q.PlatformTenantGet(ctx, id)
		if err != nil {
			return err
		}
		outlets, err := q.PlatformTenantOutlets(ctx, id)
		if err != nil {
			return err
		}
		users, err := q.PlatformTenantUsers(ctx, id)
		if err != nil {
			return err
		}
		out = &TenantDetail{ID: t.ID, Code: t.Code, Name: t.Name, Active: t.Active, CreatedAt: t.CreatedAt.Time, SaleEditWindowDays: int(t.SaleEditWindowDays), Outlets: []OutletRow{}, Users: []UserRow{}}
		for _, o := range outlets {
			out.Outlets = append(out.Outlets, OutletRow{ID: o.ID, Code: o.Code, Name: o.Name, Active: o.Active})
		}
		for _, u := range users {
			row := UserRow{ID: u.ID, Name: u.Name, Email: u.Email, Active: u.Active, RoleName: u.RoleName,
				EmailVerified: u.EmailVerified, CreatedAt: u.CreatedAt.Time}
			if u.LastLoginAt.Valid {
				t := u.LastLoginAt.Time
				row.LastLoginAt = &t
			}
			out.Users = append(out.Users, row)
		}
		return nil
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	return out, err
}

// TenantAudit = audit log tenant itu (termasuk kejadian `platform.*` dari Platform Admin).
func (s *Service) TenantAudit(ctx context.Context, tenantID uuid.UUID, f audit.Filter) (*audit.Page, error) {
	return audit.NewService(s.Pool).List(ctx, tenantID, f)
}

// SetTenantActive mengaktifkan/menonaktifkan tenant. Pengguna tenant yang nonaktif langsung ditolak (cache izin dibuang).
func (s *Service) SetTenantActive(ctx context.Context, a Actor, tenantID uuid.UUID, active bool) error {
	err := db.WithTenant(ctx, s.Pool, tenantID, func(tx pgx.Tx) error {
		q := gen.New(tx)
		t, err := q.PlatformTenantGet(ctx, tenantID)
		if err != nil {
			return err
		}
		if n, err := q.PlatformTenantSetActive(ctx, gen.PlatformTenantSetActiveParams{ID: tenantID, Active: active}); err != nil || n == 0 {
			if err == nil {
				err = pgx.ErrNoRows
			}
			return err
		}
		details := map[string]any{"active": active, "previous": t.Active}
		if err := s.recordTx(ctx, tx, a, ActionTenantStatus, tenantID, t.Name, details); err != nil {
			return err
		}
		return audit.Record(ctx, tx, audit.Actor{TenantID: tenantID, Name: "Platform: " + a.Name}, audit.Entry{
			Action: audit.ActionPlatformTenantStatus, Entity: audit.EntityTenant, EntityID: tenantID.String(),
			Details: map[string]any{"active": active, "admin_id": a.ID.String()},
		})
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	if err == nil {
		s.Perms.InvalidateTenant(tenantID)
	}
	return err
}

// MaxEditWindowDays = batas atas pengaturan (3650 hari ≈ tanpa batas).
const MaxEditWindowDays = 3650

var ErrInvalidWindow = errors.New("batas hari edit nota tidak valid")

// SetSaleEditWindow mengatur batas hari edit/batal nota satu tenant. Sengaja HANYA lewat Platform Admin: pemilik tenant
// tidak bisa melonggarkan sendiri batas yang menjaga akurasi laporan hariannya. Perubahan tercatat di audit platform dan audit tenant.
func (s *Service) SetSaleEditWindow(ctx context.Context, a Actor, tenantID uuid.UUID, days int) error {
	if days < 0 || days > MaxEditWindowDays {
		return ErrInvalidWindow
	}
	err := db.WithTenant(ctx, s.Pool, tenantID, func(tx pgx.Tx) error {
		q := gen.New(tx)
		t, err := q.PlatformTenantGet(ctx, tenantID)
		if err != nil {
			return err
		}
		if n, err := q.PlatformTenantSetEditWindow(ctx, gen.PlatformTenantSetEditWindowParams{ID: tenantID, Days: int32(days)}); err != nil || n == 0 {
			if err == nil {
				err = pgx.ErrNoRows
			}
			return err
		}
		details := map[string]any{"days": days, "previous": t.SaleEditWindowDays}
		if err := s.recordTx(ctx, tx, a, ActionEditWindow, tenantID, t.Name, details); err != nil {
			return err
		}
		return audit.Record(ctx, tx, audit.Actor{TenantID: tenantID, Name: "Platform: " + a.Name}, audit.Entry{
			Action: audit.ActionPlatformEditWindow, Entity: audit.EntityTenant, EntityID: tenantID.String(),
			Details: map[string]any{"days": days, "previous": t.SaleEditWindowDays, "admin_id": a.ID.String()},
		})
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	return err
}

// Impersonation = respons "masuk sebagai": bentuknya sama dengan sesi tenant (agar SPA memakai UI yang sama),
// ditambah `impersonating`. Tanpa refresh token; umur token pauth.ImpersonationTTL.
type Impersonation struct {
	AccessToken   string `json:"access_token"`
	ExpiresIn     int    `json:"expires_in"`
	Impersonating bool   `json:"impersonating"`
	auth.Profile
}

// Impersonate menerbitkan token tenant hanya-baca untuk Platform Admin. outletID kosong = outlet aktif pertama.
// Tercatat di audit platform DAN audit tenant (terlihat oleh pemilik tenant) dalam satu transaksi dengan pembacaan.
func (s *Service) Impersonate(ctx context.Context, a Actor, tenantID, outletID uuid.UUID) (*Impersonation, error) {
	var (
		out    *Impersonation
		now    = s.now()
		tenant gen.PlatformTenantGetRow
		outlet gen.PlatformTenantOutletsRow
	)
	err := db.WithTenant(ctx, s.Pool, tenantID, func(tx pgx.Tx) error {
		q := gen.New(tx)
		var err error
		if tenant, err = q.PlatformTenantGet(ctx, tenantID); err != nil {
			return err
		}
		outlets, err := q.PlatformTenantOutlets(ctx, tenantID)
		if err != nil {
			return err
		}
		found := false
		for _, o := range outlets {
			if o.Active && (outletID == uuid.Nil || o.ID == outletID) {
				outlet, found = o, true
				break
			}
		}
		if !found {
			return ErrOutletNotFound
		}
		if err := s.recordTx(ctx, tx, a, ActionImpersonate, tenantID, tenant.Name, map[string]any{"outlet_id": outlet.ID.String(), "outlet": outlet.Name}); err != nil {
			return err
		}
		return audit.Record(ctx, tx, audit.Actor{TenantID: tenantID, OutletID: outlet.ID, Name: "Platform: " + a.Name}, audit.Entry{
			Action: audit.ActionPlatformImpersonate, Entity: audit.EntityTenant, EntityID: tenantID.String(),
			Details: map[string]any{"admin_id": a.ID.String(), "outlet": outlet.Name, "read_only": true},
		})
	})
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		return nil, ErrNotFound
	case err != nil:
		return nil, err
	}
	tok, err := s.Tokens.IssueImpersonation(a.ID.String(), tenantID.String(), outlet.ID.String(), now)
	if err != nil {
		return nil, err
	}
	out = &Impersonation{
		AccessToken: tok, ExpiresIn: int(pauth.ImpersonationTTL.Seconds()), Impersonating: true,
		Profile: auth.Profile{
			Permissions:   authz.Permissions{All: true},
			EmailVerified: true,
			User:          auth.Identity{ID: a.ID.String(), Name: "Platform: " + a.Name},
			Tenant:        auth.Identity{ID: tenant.ID.String(), Code: tenant.Code, Name: tenant.Name},
			Outlet:        auth.Identity{ID: outlet.ID.String(), Code: outlet.Code, Name: outlet.Name},
		},
	}
	return out, nil
}
