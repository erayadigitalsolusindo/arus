package platformadmin

import (
	"context"
	"errors"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"

	"aciraba/internal/audit"
	"aciraba/internal/auth"
	"aciraba/internal/authz"
	gen "aciraba/internal/gen"
	pauth "aciraba/internal/platform/auth"
	"aciraba/internal/platform/db"
	"aciraba/internal/platform/sanitize"
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
	PlanID      uuid.UUID  `json:"plan_id"`
	PlanName    string     `json:"plan_name"`
	PlanExpires *time.Time `json:"plan_expires_at"`
	TenantType  string     `json:"tenant_type"`
	Location    string     `json:"location"`
}

// TenantFilter = filter lanjutan daftar tenant; nilai kosong/nil = tidak memfilter.
type TenantFilter struct {
	Q, Status, Login, Type, Location string
	PlanID           *uuid.UUID
	From, To         *time.Time
}

type TenantPage struct {
	Items  []TenantRow `json:"items"`
	Total  int64       `json:"total"`
	Limit  int         `json:"limit"`
	Offset int         `json:"offset"`
}

// ListTenants memanggil fungsi DB yang melewati RLS (hanya untuk Platform Admin aktif; diverifikasi di DB).
func (s *Service) ListTenants(ctx context.Context, a Actor, f TenantFilter, limit, offset int) (*TenantPage, error) {
	if limit <= 0 {
		limit = defaultPage
	}
	limit, offset = min(limit, maxPage), max(offset, 0)
	rows, err := s.Pool.Query(ctx,
		`SELECT id, code, name, active, created_at, outlet_count, user_count, last_login_at, owner_email, plan_id, plan_name, plan_expires_at, tenant_type, location, total
		   FROM platform_list_tenants($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)`, a.ID, f.Q, limit, offset, f.Status, f.PlanID, f.Login, f.From, f.To, f.Type, f.Location)
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
			pexp  pgtype.Timestamptz
		)
		if err := rows.Scan(&r.ID, &r.Code, &r.Name, &r.Active, &r.CreatedAt, &r.OutletCount, &r.UserCount, &last, &owner, &r.PlanID, &r.PlanName, &pexp, &r.TenantType, &r.Location, &page.Total); err != nil {
			return nil, err
		}
		if last.Valid {
			t := last.Time
			r.LastLoginAt = &t
		}
		r.OwnerEmail = owner.String
		if pexp.Valid {
			t := pexp.Time
			r.PlanExpires = &t
		}
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
	TenantType         string      `json:"tenant_type"`
	Location           string      `json:"location"`
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
		var tType, tLoc string
		if err := tx.QueryRow(ctx, `SELECT tenant_type, location FROM tenants WHERE id = $1`, id).Scan(&tType, &tLoc); err != nil {
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
		out = &TenantDetail{ID: t.ID, Code: t.Code, Name: t.Name, Active: t.Active, CreatedAt: t.CreatedAt.Time, SaleEditWindowDays: int(t.SaleEditWindowDays), TenantType: tType, Location: tLoc, Outlets: []OutletRow{}, Users: []UserRow{}}
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
		if active { // mengaktifkan kembali = membatalkan jadwal hapus (bila ada)
			_, err = s.Pool.Exec(ctx, `SELECT platform_tenant_cancel_delete($1, $2)`, a.ID, tenantID)
		}
	}
	return err
}

var (
	ErrInvalidTenantName = errors.New("nama toko tidak valid")
	ErrInvalidTenantCode = errors.New("kode toko tidak valid")
	ErrTenantCodeTaken   = errors.New("kode toko sudah dipakai")

	tenantCodeRe = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{1,31}$`)
)

// RenameTenant mengganti nama dan kode (slug) tenant. Kode hanya pengenal tampilan/dukungan (login memakai email),
// jadi aman diganti; keunikan dijaga UNIQUE tenants_code_key. Tercatat di audit platform dan audit tenant.
func (s *Service) RenameTenant(ctx context.Context, a Actor, tenantID uuid.UUID, name, code string) error {
	name, code = strings.TrimSpace(name), strings.ToLower(strings.TrimSpace(code))
	if n := len([]rune(name)); n < 1 || n > 100 {
		return ErrInvalidTenantName
	}
	if !tenantCodeRe.MatchString(code) {
		return ErrInvalidTenantCode
	}
	err := db.WithTenant(ctx, s.Pool, tenantID, func(tx pgx.Tx) error {
		q := gen.New(tx)
		t, err := q.PlatformTenantGet(ctx, tenantID)
		if err != nil {
			return err
		}
		if n, err := q.PlatformTenantRename(ctx, gen.PlatformTenantRenameParams{ID: tenantID, Name: name, Code: code}); err != nil || n == 0 {
			if err == nil {
				err = pgx.ErrNoRows
			}
			return err
		}
		details := map[string]any{"name": name, "code": code, "previous_name": t.Name, "previous_code": t.Code}
		if err := s.recordTx(ctx, tx, a, ActionTenantRename, tenantID, name, details); err != nil {
			return err
		}
		return audit.Record(ctx, tx, audit.Actor{TenantID: tenantID, Name: "Platform: " + a.Name}, audit.Entry{
			Action: audit.ActionPlatformTenantRename, Entity: audit.EntityTenant, EntityID: tenantID.String(),
			Details: map[string]any{"name": name, "code": code, "previous_name": t.Name, "previous_code": t.Code, "admin_id": a.ID.String()},
		})
	})
	var pe *pgconn.PgError
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		return ErrNotFound
	case errors.As(err, &pe) && pe.Code == "23505" && pe.ConstraintName == "tenants_code_key":
		return ErrTenantCodeTaken
	}
	return err
}

var ErrInvalidEmail = errors.New("email tidak valid")

// ChangeUserEmail mengganti email login pengguna tenant. Transaksi merujuk pengguna lewat id, bukan email, jadi riwayat
// tidak terpengaruh. Email baru berstatus belum terverifikasi; token akses dan semua sesi refresh dicabut; tautan Google
// lama dilepas (identitas itu milik pemilik email lama). Email bentrok = ErrEmailTaken. Tercatat di audit platform dan tenant.
func (s *Service) ChangeUserEmail(ctx context.Context, a Actor, tenantID, userID uuid.UUID, rawEmail string) error {
	email, code := sanitize.Email(rawEmail)
	if code != "" {
		return ErrInvalidEmail
	}
	err := db.WithTenant(ctx, s.Pool, tenantID, func(tx pgx.Tx) error {
		q := gen.New(tx)
		t, err := q.PlatformTenantGet(ctx, tenantID)
		if err != nil {
			return err
		}
		u, err := q.PlatformUserGet(ctx, gen.PlatformUserGetParams{TenantID: tenantID, ID: userID})
		if err != nil {
			return err
		}
		if strings.EqualFold(u.Email, email) {
			return ErrEmailUnchanged
		}
		if n, err := q.PlatformUserSetEmail(ctx, gen.PlatformUserSetEmailParams{TenantID: tenantID, ID: userID, Email: email, ValidAfter: pgtype.Timestamptz{Time: s.now(), Valid: true}}); err != nil || n == 0 {
			if err == nil {
				err = pgx.ErrNoRows
			}
			return err
		}
		unlinked, err := q.PlatformUserUnlinkIdentities(ctx, gen.PlatformUserUnlinkIdentitiesParams{ActorID: a.ID, TenantID: tenantID, UserID: userID})
		if err != nil {
			return err
		}
		details := map[string]any{"user_id": userID.String(), "user": u.Name, "email": email, "previous": u.Email, "google_unlinked": unlinked > 0}
		if err := s.recordTx(ctx, tx, a, ActionUserEmail, tenantID, t.Name, details); err != nil {
			return err
		}
		return audit.Record(ctx, tx, audit.Actor{TenantID: tenantID, Name: "Platform: " + a.Name}, audit.Entry{
			Action: audit.ActionPlatformUserEmail, Entity: audit.EntityUser, EntityID: userID.String(),
			Details: map[string]any{"email": email, "previous": u.Email, "google_unlinked": unlinked > 0, "admin_id": a.ID.String()},
		})
	})
	var pe *pgconn.PgError
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		return ErrNotFound
	case errors.As(err, &pe) && pe.Code == "23505" && pe.ConstraintName == "users_email_key":
		return ErrEmailTaken
	}
	if err != nil {
		return err
	}
	s.Perms.InvalidateTenant(tenantID)
	return s.Sessions.RevokeUser(ctx, userID.String())
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
			Details: map[string]any{"admin_id": a.ID.String(), "outlet": outlet.Name, "read_only": false},
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

var ErrInvalidTenantProfile = errors.New("jenis/lokasi tenant tidak valid")

const (
	maxTenantType     = 50
	maxTenantLocation = 100
)

// cleanLabel menormalkan isian penanda opsional (boleh kosong), menolak karakter kontrol/markup dan teks terlalu panjang.
func cleanLabel(raw string, max int) (string, bool) {
	v, ok := sanitize.Text(raw)
	return v, ok && !strings.ContainsAny(v, "<>") && len([]rune(v)) <= max
}

// SetTenantProfile mengisi penanda Jenis Tenant (mis. UMKM) dan Lokasi Tenant. Keduanya hanya label untuk operator:
// tidak mempengaruhi paket, izin, maupun data bisnis tenant.
func (s *Service) SetTenantProfile(ctx context.Context, a Actor, tenantID uuid.UUID, tenantType, location string) error {
	tt, ok1 := cleanLabel(tenantType, maxTenantType)
	loc, ok2 := cleanLabel(location, maxTenantLocation)
	if !ok1 || !ok2 {
		return ErrInvalidTenantProfile
	}
	err := db.WithTenant(ctx, s.Pool, tenantID, func(tx pgx.Tx) error {
		var name, prevType, prevLoc string
		if err := tx.QueryRow(ctx, `SELECT name, tenant_type, location FROM tenants WHERE id = $1`, tenantID).Scan(&name, &prevType, &prevLoc); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `UPDATE tenants SET tenant_type = $2, location = $3 WHERE id = $1`, tenantID, tt, loc); err != nil {
			return err
		}
		return s.recordTx(ctx, tx, a, ActionTenantProfile, tenantID, name, map[string]any{
			"tenant_type": tt, "location": loc, "previous_tenant_type": prevType, "previous_location": prevLoc})
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	return err
}

type TenantTypeCount struct {
	Type  string `json:"type"`
	Count int64  `json:"count"`
}

// TenantTypes = jenis tenant yang sudah dipakai (maks 200, terbanyak dulu): bahan saran isian dan filter.
func (s *Service) TenantTypes(ctx context.Context, a Actor) ([]TenantTypeCount, error) {
	rows, err := s.Pool.Query(ctx, `SELECT tenant_type, n FROM platform_tenant_types($1)`, a.ID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []TenantTypeCount{}
	for rows.Next() {
		var t TenantTypeCount
		if err := rows.Scan(&t.Type, &t.Count); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}
