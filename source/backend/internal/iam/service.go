package iam

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	gen "aciraba/internal/gen"
	pauth "aciraba/internal/platform/auth"
	"aciraba/internal/platform/db"
	"aciraba/internal/platform/sanitize"
)

var (
	ErrNotFound   = errors.New("data tidak ditemukan")
	ErrSystemRole = errors.New("role sistem tidak dapat diubah atau dihapus")
	ErrRoleInUse  = errors.New("role masih dipakai pengguna")
	ErrNameTaken  = errors.New("nama role sudah dipakai")
	ErrEmailTaken = errors.New("email sudah terdaftar")
	ErrEscalation = errors.New("tidak boleh memberi izin melebihi izin sendiri")
	ErrLastOwner  = errors.New("pemilik aktif terakhir tidak boleh dinonaktifkan atau diganti rolenya")
	ErrSelfChange = errors.New("tidak boleh menonaktifkan atau mengganti role akun sendiri")
)

const (
	maxName           = 100
	maxRoleName       = 50
	maxConcurrentHash = 4
)

// FieldErrors = kode galat per field (REQUIRED/INVALID/TOO_LONG/TOO_SHORT/WEAK), diterjemahkan klien.
type FieldErrors map[string]string

func (f FieldErrors) Error() string { return "input tidak valid" }

type Service struct {
	pool     *pgxpool.Pool
	resolver *Resolver
	hashSem  chan struct{}
}

func NewService(pool *pgxpool.Pool, resolver *Resolver) *Service {
	return &Service{pool: pool, resolver: resolver, hashSem: make(chan struct{}, maxConcurrentHash)}
}

type Role struct {
	ID          uuid.UUID   `json:"id"`
	Name        string      `json:"name"`
	Permissions Permissions `json:"permissions"`
	IsSystem    bool        `json:"is_system"`
	UserCount   int         `json:"user_count"`
}

type User struct {
	ID          uuid.UUID  `json:"id"`
	Name        string     `json:"name"`
	Email       string     `json:"email"`
	Phone       string     `json:"phone"`
	Active      bool       `json:"active"`
	RoleID      uuid.UUID  `json:"role_id"`
	RoleName    string     `json:"role_name"`
	LastLoginAt *time.Time `json:"last_login_at"`
	CreatedAt   time.Time  `json:"created_at"`
}

func userOf(id uuid.UUID, name, email string, phone pgtype.Text, active bool, roleID uuid.UUID, roleName string, last, created pgtype.Timestamptz) User {
	u := User{ID: id, Name: name, Email: email, Phone: phone.String, Active: active, RoleID: roleID, RoleName: roleName, CreatedAt: created.Time}
	if last.Valid {
		t := last.Time
		u.LastLoginAt = &t
	}
	return u
}

// ---- Role ----

func (s *Service) ListRoles(ctx context.Context, tenantID uuid.UUID) ([]Role, error) {
	out := []Role{}
	err := db.WithTenant(ctx, s.pool, tenantID, func(tx pgx.Tx) error {
		rows, err := gen.New(tx).IamListRoles(ctx, tenantID)
		for _, r := range rows {
			out = append(out, Role{ID: r.ID, Name: r.Name, Permissions: ParseStored(r.Permissions), IsSystem: r.IsSystem, UserCount: int(r.UserCount)})
		}
		return err
	})
	return out, err
}

func validRoleInput(name string, perms map[string][]string) (string, Permissions, FieldErrors) {
	f := FieldErrors{}
	n, code := sanitize.Name(name, maxRoleName)
	if code != "" {
		f["name"] = code
	}
	p, err := Normalize(perms)
	if err != nil {
		f["permissions"] = "INVALID"
	}
	if len(f) > 0 {
		return "", Permissions{}, f
	}
	return n, p, nil
}

func (s *Service) CreateRole(ctx context.Context, actor Actor, name string, perms map[string][]string) (*Role, error) {
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
		return qerr
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

func (s *Service) UpdateRole(ctx context.Context, actor Actor, id uuid.UUID, name string, perms map[string][]string) (*Role, error) {
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
		if !actor.Perms.Covers(ParseStored(cur.Permissions)) {
			return ErrEscalation
		}
		row, err = q.IamUpdateRole(ctx, gen.IamUpdateRoleParams{TenantID: actor.TenantID, ID: id, Name: n, Permissions: raw})
		return err
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

func (s *Service) DeleteRole(ctx context.Context, actor Actor, id uuid.UUID) error {
	err := db.WithTenant(ctx, s.pool, actor.TenantID, func(tx pgx.Tx) error {
		q := gen.New(tx)
		cur, err := q.IamGetRole(ctx, gen.IamGetRoleParams{TenantID: actor.TenantID, ID: id})
		if err != nil {
			return err
		}
		if cur.IsSystem {
			return ErrSystemRole
		}
		if !actor.Perms.Covers(ParseStored(cur.Permissions)) {
			return ErrEscalation
		}
		_, err = q.IamDeleteRole(ctx, gen.IamDeleteRoleParams{TenantID: actor.TenantID, ID: id})
		return err
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

// AssignableRoles = role yang boleh diberikan pemanggil ke pengguna lain (izinnya tidak melebihi izin pemanggil).
// Dipakai dropdown role di halaman Pengguna, sehingga pemegang `users` tidak perlu izin `roles`.
func (s *Service) AssignableRoles(ctx context.Context, actor Actor) ([]Role, error) {
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

// ---- Pengguna ----

func (s *Service) ListUsers(ctx context.Context, tenantID uuid.UUID) ([]User, error) {
	out := []User{}
	err := db.WithTenant(ctx, s.pool, tenantID, func(tx pgx.Tx) error {
		rows, err := gen.New(tx).IamListUsers(ctx, tenantID)
		for _, r := range rows {
			out = append(out, userOf(r.ID, r.Name, r.Email, r.Phone, r.Active, r.RoleID, r.RoleName, r.LastLoginAt, r.CreatedAt))
		}
		return err
	})
	return out, err
}

type CreateUserInput struct {
	Name, Email, Phone, Password string
	RoleID                       uuid.UUID
}

func (s *Service) CreateUser(ctx context.Context, actor Actor, in CreateUserInput) (*User, error) {
	f := FieldErrors{}
	name, code := sanitize.Name(in.Name, maxName)
	setCode(f, "name", code)
	email, code := sanitize.Email(in.Email)
	setCode(f, "email", code)
	phone, _ := optionalPhone(in.Phone, f)
	setCode(f, "password", sanitize.Password(in.Password, email))
	if in.RoleID == uuid.Nil {
		f["role_id"] = "REQUIRED"
	}
	if len(f) > 0 {
		return nil, f
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
		if !actor.Perms.Covers(ParseStored(role.Permissions)) {
			return ErrEscalation
		}
		id, err = q.IamCreateUser(ctx, gen.IamCreateUserParams{
			TenantID: actor.TenantID, RoleID: in.RoleID, Email: email, Name: name, Phone: phone, PasswordHash: hash,
		})
		return err
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

func (s *Service) GetUser(ctx context.Context, tenantID, id uuid.UUID) (*User, error) {
	var u User
	err := db.WithTenant(ctx, s.pool, tenantID, func(tx pgx.Tx) error {
		r, err := gen.New(tx).IamGetUser(ctx, gen.IamGetUserParams{TenantID: tenantID, ID: id})
		if err == nil {
			u = userOf(r.ID, r.Name, r.Email, r.Phone, r.Active, r.RoleID, r.RoleName, r.LastLoginAt, r.CreatedAt)
		}
		return err
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	return &u, err
}

type UpdateUserInput struct {
	Name, Phone string
	RoleID      uuid.UUID
	Active      bool
}

func (s *Service) UpdateUser(ctx context.Context, actor Actor, id uuid.UUID, in UpdateUserInput) (*User, error) {
	f := FieldErrors{}
	name, code := sanitize.Name(in.Name, maxName)
	setCode(f, "name", code)
	phone, _ := optionalPhone(in.Phone, f)
	if in.RoleID == uuid.Nil {
		f["role_id"] = "REQUIRED"
	}
	if len(f) > 0 {
		return nil, f
	}

	err := db.WithTenant(ctx, s.pool, actor.TenantID, func(tx pgx.Tx) error {
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

		roleChanged := in.RoleID != target.RoleID
		if id == actor.UserID && (roleChanged || !in.Active) {
			return ErrSelfChange
		}
		// Pemanggil tidak boleh menyentuh akun yang izinnya melebihi izinnya, atau memberi role yang melebihi.
		if !actor.Perms.Covers(ParseStored(oldRole.Permissions)) || !actor.Perms.Covers(ParseStored(newRole.Permissions)) {
			return ErrEscalation
		}
		if oldRole.IsSystem && target.Active && (roleChanged && !newRole.IsSystem || !in.Active) && len(owners) <= 1 && slices.Contains(owners, id) {
			return ErrLastOwner
		}
		n, err := q.IamUpdateUser(ctx, gen.IamUpdateUserParams{TenantID: actor.TenantID, ID: id, Name: name, Phone: phone, RoleID: in.RoleID, Active: in.Active})
		if err == nil && n == 0 {
			return pgx.ErrNoRows
		}
		return err
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	s.resolver.InvalidateTenant(actor.TenantID)
	return s.GetUser(ctx, actor.TenantID, id)
}

// ResetPassword mengganti password pengguna lain oleh admin. Sesi refresh yang sedang hidup belum dicabut
// (butuh indeks sesi per pengguna di Redis); akun yang dinonaktifkan sudah langsung kehilangan akses.
func (s *Service) ResetPassword(ctx context.Context, actor Actor, id uuid.UUID, password string) error {
	hash := ""
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
		if !actor.Perms.Covers(ParseStored(role.Permissions)) {
			return ErrEscalation
		}
		if hash, err = s.hash(ctx, password); err != nil {
			return err
		}
		_, err = q.IamSetPassword(ctx, gen.IamSetPasswordParams{TenantID: actor.TenantID, ID: id, PasswordHash: hash})
		return err
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	return err
}

// ---- util ----

func (s *Service) hash(ctx context.Context, password string) (string, error) {
	select {
	case s.hashSem <- struct{}{}:
		defer func() { <-s.hashSem }()
	case <-ctx.Done():
		return "", ctx.Err()
	}
	return pauth.HashPassword(password)
}

func setCode(f FieldErrors, field, code string) {
	if code != "" {
		f[field] = code
	}
}

// optionalPhone: nomor HP boleh kosong pada pegawai.
func optionalPhone(raw string, f FieldErrors) (pgtype.Text, bool) {
	if raw == "" {
		return pgtype.Text{}, true
	}
	v, code := sanitize.Phone(raw)
	if code != "" {
		f["phone"] = code
		return pgtype.Text{}, false
	}
	return pgtype.Text{String: v, Valid: true}, true
}

func isUnique(err error, constraint string) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505" && pgErr.ConstraintName == constraint
}
