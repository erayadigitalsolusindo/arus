package iam

import (
	"context"
	"errors"
	"slices"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"aciraba/internal/audit"
	"aciraba/internal/authz"
	gen "aciraba/internal/gen"
	"aciraba/internal/platform/db"
	"aciraba/internal/platform/sanitize"
)

type User struct {
	ID            uuid.UUID `json:"id"`
	Name          string    `json:"name"`
	Email         string    `json:"email"`
	Phone         string    `json:"phone"`
	Active        bool      `json:"active"`
	EmailVerified bool      `json:"email_verified"`
	RoleID        uuid.UUID `json:"role_id"`
	RoleName      string    `json:"role_name"`
	RoleIsSystem  bool      `json:"role_is_system"`
	// AllOutlets: pemilik (role sistem) otomatis mengakses semua outlet; OutletIDs kosong dalam kasus itu.
	AllOutlets  bool        `json:"all_outlets"`
	OutletIDs   []uuid.UUID `json:"outlet_ids"`
	LastLoginAt *time.Time  `json:"last_login_at"`
	CreatedAt   time.Time   `json:"created_at"`
}

type userRow struct {
	id                 uuid.UUID
	name, email        string
	phone              pgtype.Text
	active             bool
	roleID             uuid.UUID
	roleName           string
	roleIsSystem       bool
	lastLogin, created pgtype.Timestamptz
	emailVerified      pgtype.Timestamptz
	rolePerms          []byte
}

func (r userRow) user(outlets []uuid.UUID) User {
	u := User{
		ID: r.id, Name: r.name, Email: r.email, Phone: r.phone.String, Active: r.active, EmailVerified: r.emailVerified.Valid,
		RoleID: r.roleID, RoleName: r.roleName, RoleIsSystem: r.roleIsSystem, AllOutlets: authz.ParseStored(r.rolePerms).All,
		OutletIDs: []uuid.UUID{}, CreatedAt: r.created.Time,
	}
	if !u.AllOutlets && outlets != nil {
		u.OutletIDs = outlets
	}
	if r.lastLogin.Valid {
		t := r.lastLogin.Time
		u.LastLoginAt = &t
	}
	return u
}

func (s *Service) ListUsers(ctx context.Context, tenantID uuid.UUID) ([]User, error) {
	out := []User{}
	err := db.WithTenant(ctx, s.pool, tenantID, func(tx pgx.Tx) error {
		q := gen.New(tx)
		rows, err := q.IamListUsers(ctx, tenantID)
		if err != nil {
			return err
		}
		links, err := q.IamListUserOutlets(ctx, tenantID)
		if err != nil {
			return err
		}
		byUser := map[uuid.UUID][]uuid.UUID{}
		for _, l := range links {
			byUser[l.UserID] = append(byUser[l.UserID], l.OutletID)
		}
		for _, r := range rows {
			out = append(out, userRow{r.ID, r.Name, r.Email, r.Phone, r.Active, r.RoleID, r.RoleName, r.RoleIsSystem, r.LastLoginAt, r.CreatedAt, r.EmailVerifiedAt, r.RolePermissions}.user(byUser[r.ID]))
		}
		return nil
	})
	return out, err
}

func (s *Service) GetUser(ctx context.Context, tenantID, id uuid.UUID) (*User, error) {
	var u User
	err := db.WithTenant(ctx, s.pool, tenantID, func(tx pgx.Tx) error {
		q := gen.New(tx)
		r, err := q.IamGetUser(ctx, gen.IamGetUserParams{TenantID: tenantID, ID: id})
		if err != nil {
			return err
		}
		outlets, err := q.IamGetUserOutlets(ctx, gen.IamGetUserOutletsParams{TenantID: tenantID, UserID: id})
		if err != nil {
			return err
		}
		u = userRow{r.ID, r.Name, r.Email, r.Phone, r.Active, r.RoleID, r.RoleName, r.RoleIsSystem, r.LastLoginAt, r.CreatedAt, r.EmailVerifiedAt, r.RolePermissions}.user(outlets)
		return nil
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	return &u, err
}

// optionalPhone: nomor HP boleh kosong pada pegawai.
func optionalPhone(raw string, f FieldErrors) pgtype.Text {
	if raw == "" {
		return pgtype.Text{}
	}
	v, code := sanitize.Phone(raw)
	if code != "" {
		f["phone"] = code
		return pgtype.Text{}
	}
	return pgtype.Text{String: v, Valid: true}
}

// checkOutlets memastikan pemanggil hanya menugaskan outlet yang ia sendiri boleh akses.
func checkOutlets(actor authz.Actor, ids []uuid.UUID) ([]uuid.UUID, error) {
	out := make([]uuid.UUID, 0, len(ids))
	for _, id := range ids {
		if id == uuid.Nil {
			return nil, FieldErrors{"outlet_ids": "INVALID"}
		}
		if !actor.Perms.All && !actor.Outlets[id] {
			return nil, ErrOutletForbidden
		}
		if !slices.Contains(out, id) {
			out = append(out, id)
		}
	}
	return out, nil
}

func replaceOutlets(ctx context.Context, q *gen.Queries, tenantID, userID uuid.UUID, ids []uuid.UUID) error {
	if err := q.IamDeleteUserOutlets(ctx, gen.IamDeleteUserOutletsParams{TenantID: tenantID, UserID: userID}); err != nil {
		return err
	}
	for _, id := range ids {
		if err := q.IamAssignUserOutlet(ctx, gen.IamAssignUserOutletParams{TenantID: tenantID, UserID: userID, OutletID: id}); err != nil {
			return err
		}
	}
	return nil
}

type CreateUserInput struct {
	Name, Email, Phone, Password string
	RoleID                       uuid.UUID
	OutletIDs                    []uuid.UUID
}

func (s *Service) CreateUser(ctx context.Context, actor authz.Actor, in CreateUserInput) (*User, error) {
	f := FieldErrors{}
	name, code := sanitize.Name(in.Name, maxName)
	setCode(f, "name", code)
	email, code := sanitize.Email(in.Email)
	setCode(f, "email", code)
	phone := optionalPhone(in.Phone, f)
	setCode(f, "password", sanitize.Password(in.Password, email))
	if in.RoleID == uuid.Nil {
		f["role_id"] = "REQUIRED"
	}
	if len(f) > 0 {
		return nil, f
	}
	outlets, err := checkOutlets(actor, in.OutletIDs)
	if err != nil {
		return nil, err
	}
	hash, err := s.hash(ctx, in.Password)
	if err != nil {
		return nil, err
	}

	var id uuid.UUID
	err = db.WithTenant(ctx, s.pool, actor.TenantID, func(tx pgx.Tx) error {
		q := gen.New(tx)
		role, err := q.IamGetRole(ctx, gen.IamGetRoleParams{TenantID: actor.TenantID, ID: in.RoleID})
		if errors.Is(err, pgx.ErrNoRows) {
			return FieldErrors{"role_id": "INVALID"}
		}
		if err != nil {
			return err
		}
		if !actor.Perms.Covers(authz.ParseStored(role.Permissions)) {
			return ErrEscalation
		}
		if !authz.ParseStored(role.Permissions).All && len(outlets) == 0 {
			return FieldErrors{"outlet_ids": "REQUIRED"} // pengguna non-pemilik wajib punya minimal satu outlet
		}
		id, err = q.IamCreateUser(ctx, gen.IamCreateUserParams{
			TenantID: actor.TenantID, RoleID: in.RoleID, Email: email, Name: name, Phone: phone, PasswordHash: hash,
		})
		if err != nil {
			return err
		}
		if !authz.ParseStored(role.Permissions).All {
			if err := replaceOutlets(ctx, q, actor.TenantID, id, outlets); err != nil {
				return err // FK komposit menolak outlet milik tenant lain
			}
		}
		return audit.Record(ctx, tx, audit.FromActor(actor), audit.Entry{
			Action: audit.ActionUserCreate, Entity: audit.EntityUser, EntityID: id.String(),
			Details: map[string]any{"name": name, "email": email, "role": role.Name, "outlet_ids": outlets},
		})
	})
	if isUnique(err, "users_email_key") {
		return nil, ErrEmailTaken
	}
	if err != nil {
		return nil, err
	}
	s.resolver.InvalidateTenant(actor.TenantID)
	return s.GetUser(ctx, actor.TenantID, id)
}

type UpdateUserInput struct {
	Name, Phone string
	RoleID      uuid.UUID
	Active      bool
	OutletIDs   []uuid.UUID
}

func (s *Service) UpdateUser(ctx context.Context, actor authz.Actor, id uuid.UUID, in UpdateUserInput) (*User, error) {
	f := FieldErrors{}
	name, code := sanitize.Name(in.Name, maxName)
	setCode(f, "name", code)
	phone := optionalPhone(in.Phone, f)
	if in.RoleID == uuid.Nil {
		f["role_id"] = "REQUIRED"
	}
	if len(f) > 0 {
		return nil, f
	}
	requested, err := checkOutlets(actor, in.OutletIDs)
	if err != nil {
		return nil, err
	}

	deactivated := false
	err = db.WithTenant(ctx, s.pool, actor.TenantID, func(tx pgx.Tx) error {
		q := gen.New(tx)
		// Kunci pemilik aktif lebih dulu: dua perubahan bersamaan tidak bisa sama-sama menyisakan nol pemilik.
		owners, err := q.IamLockActiveOwners(ctx, actor.TenantID)
		if err != nil {
			return err
		}
		target, err := q.IamGetUser(ctx, gen.IamGetUserParams{TenantID: actor.TenantID, ID: id})
		if err != nil {
			return err
		}
		newRole, err := q.IamGetRole(ctx, gen.IamGetRoleParams{TenantID: actor.TenantID, ID: in.RoleID})
		if errors.Is(err, pgx.ErrNoRows) {
			return FieldErrors{"role_id": "INVALID"}
		}
		if err != nil {
			return err
		}
		oldRole, err := q.IamGetRole(ctx, gen.IamGetRoleParams{TenantID: actor.TenantID, ID: target.RoleID})
		if err != nil {
			return err
		}
		current, err := q.IamGetUserOutlets(ctx, gen.IamGetUserOutletsParams{TenantID: actor.TenantID, UserID: id})
		if err != nil {
			return err
		}

		roleChanged := in.RoleID != target.RoleID
		if id == actor.UserID && (roleChanged || !in.Active) {
			return ErrSelfChange
		}
		// Pemanggil tidak boleh menyentuh akun yang izinnya melebihi izinnya, atau memberi role yang melebihi.
		if !actor.Perms.Covers(authz.ParseStored(oldRole.Permissions)) || !actor.Perms.Covers(authz.ParseStored(newRole.Permissions)) {
			return ErrEscalation
		}
		if oldRole.IsSystem && target.Active && (roleChanged && !newRole.IsSystem || !in.Active) && len(owners) <= 1 && slices.Contains(owners, id) {
			return ErrLastOwner
		}

		// Outlet: pemanggil hanya mengatur outlet yang ia akses; penugasan di luar jangkauannya dipertahankan.
		final := slices.Clone(requested)
		for _, o := range current {
			if !actor.Perms.All && !actor.Outlets[o] && !slices.Contains(final, o) {
				final = append(final, o)
			}
		}
		if !authz.ParseStored(newRole.Permissions).All && len(final) == 0 {
			return FieldErrors{"outlet_ids": "REQUIRED"}
		}
		if id == actor.UserID && !sameSet(current, final) && !authz.ParseStored(newRole.Permissions).All {
			return ErrSelfChange // tidak mencabut/menambah outlet akun sendiri (bisa mengunci diri dari outlet aktif)
		}

		n, err := q.IamUpdateUser(ctx, gen.IamUpdateUserParams{TenantID: actor.TenantID, ID: id, Name: name, Phone: phone, RoleID: in.RoleID, Active: in.Active, ValidAfter: now()})
		if err != nil {
			return err
		}
		if n == 0 {
			return pgx.ErrNoRows
		}
		if !sameSet(current, final) {
			if err := replaceOutlets(ctx, q, actor.TenantID, id, final); err != nil {
				return err
			}
		}
		deactivated = target.Active && !in.Active
		return audit.Record(ctx, tx, audit.FromActor(actor), audit.Entry{
			Action: audit.ActionUserUpdate, Entity: audit.EntityUser, EntityID: id.String(),
			Details: map[string]any{
				"before": map[string]any{"name": target.Name, "role": oldRole.Name, "active": target.Active, "outlet_ids": current},
				"after":  map[string]any{"name": name, "role": newRole.Name, "active": in.Active, "outlet_ids": final},
			},
		})
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	s.resolver.InvalidateTenant(actor.TenantID)
	if deactivated {
		// Token akses sudah dicabut lewat tokens_valid_after; cabut juga semua sesi refresh di semua perangkat.
		if err := s.sessions.RevokeUser(ctx, id.String()); err != nil {
			return nil, err
		}
	}
	return s.GetUser(ctx, actor.TenantID, id)
}

// ResetPassword mengganti password pengguna lain oleh admin. Semua sesi pengguna itu dicabut (refresh token di semua
// perangkat dan token akses yang masih hidup), sehingga ia wajib masuk ulang dengan password baru.
func (s *Service) ResetPassword(ctx context.Context, actor authz.Actor, id uuid.UUID, password string) error {
	err := db.WithTenant(ctx, s.pool, actor.TenantID, func(tx pgx.Tx) error {
		q := gen.New(tx)
		target, err := q.IamGetUser(ctx, gen.IamGetUserParams{TenantID: actor.TenantID, ID: id})
		if err != nil {
			return err
		}
		if code := sanitize.Password(password, target.Email); code != "" {
			return FieldErrors{"password": code}
		}
		role, err := q.IamGetRole(ctx, gen.IamGetRoleParams{TenantID: actor.TenantID, ID: target.RoleID})
		if err != nil {
			return err
		}
		if !actor.Perms.Covers(authz.ParseStored(role.Permissions)) {
			return ErrEscalation
		}
		hash, err := s.hash(ctx, password)
		if err != nil {
			return err
		}
		if _, err = q.IamSetPassword(ctx, gen.IamSetPasswordParams{TenantID: actor.TenantID, ID: id, PasswordHash: hash, ValidAfter: now()}); err != nil {
			return err
		}
		return audit.Record(ctx, tx, audit.FromActor(actor), audit.Entry{
			Action: audit.ActionUserPasswordReset, Entity: audit.EntityUser, EntityID: id.String(),
			Details: map[string]any{"email": target.Email},
		})
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	s.resolver.InvalidateTenant(actor.TenantID)
	return s.sessions.RevokeUser(ctx, id.String())
}

// now = jam aplikasi untuk tokens_valid_after (sama sumbernya dengan iat token akses).
func now() pgtype.Timestamptz { return pgtype.Timestamptz{Time: time.Now(), Valid: true} }

func sameSet(a, b []uuid.UUID) bool {
	if len(a) != len(b) {
		return false
	}
	for _, x := range a {
		if !slices.Contains(b, x) {
			return false
		}
	}
	return true
}

// VerifyEmail menandai email pengguna lain sebagai terverifikasi secara manual. Penanda harus punya izin yang
// mencakup role target (anti-eskalasi, seperti ResetPassword) dan tidak boleh memverifikasi akunnya sendiri.
// Tercatat di audit sebagai tindakan manual; memanggil ulang pada akun yang sudah terverifikasi tidak berefek.
func (s *Service) VerifyEmail(ctx context.Context, actor authz.Actor, id uuid.UUID) error {
	if id == actor.UserID {
		return ErrSelfVerify
	}
	err := db.WithTenant(ctx, s.pool, actor.TenantID, func(tx pgx.Tx) error {
		q := gen.New(tx)
		target, err := q.IamGetUser(ctx, gen.IamGetUserParams{TenantID: actor.TenantID, ID: id})
		if err != nil {
			return err
		}
		if !actor.Perms.Covers(authz.ParseStored(target.RolePermissions)) {
			return ErrEscalation
		}
		n, err := q.IamMarkEmailVerified(ctx, gen.IamMarkEmailVerifiedParams{TenantID: actor.TenantID, ID: id})
		if err != nil || n == 0 {
			return err
		}
		return audit.Record(ctx, tx, audit.FromActor(actor), audit.Entry{
			Action: audit.ActionUserEmailVerify, Entity: audit.EntityUser, EntityID: id.String(),
			Details: map[string]any{"email": target.Email, "manual": true},
		})
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	return err
}
