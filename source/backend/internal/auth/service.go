// Package auth: pendaftaran tenant baru, login, refresh, dan logout. Use-case = satu transaksi DB (AGENTS.md §3.2).
package auth

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	gen "aciraba/internal/gen"
	"aciraba/internal/iam"
	pauth "aciraba/internal/platform/auth"
	"aciraba/internal/platform/db"
	"aciraba/internal/platform/sanitize"
)

var (
	ErrEmailTaken = errors.New("email sudah terdaftar")

	ownerPermissions = []byte(`{"*":true}`)
)

const (
	maxName = 100
	// Batas hashing argon2id bersamaan (masing-masing ±64 MiB) agar lonjakan register tidak menghabiskan memori.
	maxConcurrentHashes = 4
	maxCodeAttempts     = 5
)

type Service struct {
	pool     *pgxpool.Pool
	tokens   *pauth.TokenIssuer
	sessions *pauth.Sessions
	perms    *iam.Resolver
	hashSem  chan struct{}
	now      func() time.Time
}

func NewService(pool *pgxpool.Pool, tokens *pauth.TokenIssuer, sessions *pauth.Sessions, perms *iam.Resolver) *Service {
	return &Service{pool: pool, tokens: tokens, sessions: sessions, perms: perms, hashSem: make(chan struct{}, maxConcurrentHashes), now: time.Now}
}

// RegisterInput = input mentah dari klien.
type RegisterInput struct {
	BusinessName string
	OwnerName    string
	Email        string
	Phone        string
	OutletName   string
	Password     string
}

// CleanRegister = input yang sudah divalidasi dan dinormalkan.
type CleanRegister struct {
	BusinessName, OwnerName, Email, Phone, OutletName, Password string
}

// ValidateRegister menormalkan semua field dan mengumpulkan kode galat per field.
func ValidateRegister(in RegisterInput) (CleanRegister, map[string]string) {
	var c CleanRegister
	f := map[string]string{}
	set := func(field, code string) {
		if code != "" {
			f[field] = code
		}
	}
	var code string
	c.BusinessName, code = sanitize.Name(in.BusinessName, maxName)
	set("business_name", code)
	c.OwnerName, code = sanitize.Name(in.OwnerName, maxName)
	set("owner_name", code)
	c.OutletName, code = sanitize.Name(in.OutletName, maxName)
	set("outlet_name", code)
	c.Email, code = sanitize.Email(in.Email)
	set("email", code)
	c.Phone, code = sanitize.Phone(in.Phone)
	set("phone", code)
	set("password", sanitize.Password(in.Password, c.Email))
	c.Password = in.Password
	if len(f) > 0 {
		return CleanRegister{}, f
	}
	return c, nil
}

type Identity struct {
	ID    string `json:"id"`
	Code  string `json:"code,omitempty"`
	Name  string `json:"name"`
	Email string `json:"email,omitempty"`
}

// Profile = identitas pengguna yang sedang masuk (dipakai klien untuk tampilan; bukan sumber otorisasi).
type Profile struct {
	// Permissions: izin efektif untuk menyaring menu/tombol di UI. Penegakan tetap di server (iam.Require).
	Permissions iam.Permissions `json:"permissions"`
	User        Identity        `json:"user"`
	Tenant      Identity        `json:"tenant"`
	Outlet      Identity        `json:"outlet"`
}

type Session struct {
	AccessToken string `json:"access_token"`
	ExpiresIn   int    `json:"expires_in"`
	Profile
	// RefreshToken kosong pada jendela grace (cookie baru sudah dikirim oleh permintaan lain).
	RefreshToken string `json:"-"`
	Remember     bool   `json:"-"`
}

// Register membuat tenant + outlet pertama + role Owner + user Owner dalam satu transaksi,
// lalu menerbitkan sesi. Input harus sudah lolos ValidateRegister.
func (s *Service) Register(ctx context.Context, in CleanRegister) (*Session, error) {
	hash, err := s.hash(ctx, in.Password)
	if err != nil {
		return nil, err
	}

	for range maxCodeAttempts {
		code, err := tenantCode(in.BusinessName)
		if err != nil {
			return nil, err
		}
		sess, err := s.registerTx(ctx, in, code, hash)
		if err == nil {
			return sess, nil
		}
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			switch pgErr.ConstraintName {
			case "users_email_key":
				return nil, ErrEmailTaken
			case "tenants_code_key":
				continue // bentrok kode tenant (sangat jarang): ulang dengan sufiks acak baru
			}
		}
		return nil, err
	}
	return nil, errors.New("gagal membuat kode tenant unik")
}

func (s *Service) registerTx(ctx context.Context, in CleanRegister, code, hash string) (*Session, error) {
	var (
		tenant gen.CreateTenantRow
		outlet gen.CreateOutletRow
		role   gen.CreateRoleRow
		user   gen.CreateUserRow
	)
	tenantID := uuid.New() // dibuat aplikasi agar seluruh transaksi bisa berjalan di bawah RLS tenant ini
	err := db.WithTenant(ctx, s.pool, tenantID, func(tx pgx.Tx) error {
		q := gen.New(tx)
		var err error
		if tenant, err = q.CreateTenant(ctx, gen.CreateTenantParams{ID: tenantID, Code: code, Name: in.BusinessName}); err != nil {
			return err
		}
		if outlet, err = q.CreateOutlet(ctx, gen.CreateOutletParams{TenantID: tenant.ID, Code: "main", Name: in.OutletName}); err != nil {
			return err
		}
		if role, err = q.CreateRole(ctx, gen.CreateRoleParams{TenantID: tenant.ID, Name: "Owner", Permissions: ownerPermissions}); err != nil {
			return err
		}
		user, err = q.CreateUser(ctx, gen.CreateUserParams{
			TenantID: tenant.ID, RoleID: role.ID, Email: in.Email, Name: in.OwnerName, Phone: pgtype.Text{String: in.Phone, Valid: true}, PasswordHash: hash,
		})
		return err
	})
	if err != nil {
		return nil, err
	}

	access, err := s.tokens.Issue(user.ID.String(), tenant.ID.String(), outlet.ID.String(), role.Name, s.now())
	if err != nil {
		return nil, err
	}
	refresh, err := s.sessions.Create(ctx, user.ID.String(), true)
	if err != nil {
		return nil, err
	}
	return buildSession(account{
		UserID: user.ID, UserName: user.Name, Email: user.Email, RoleName: role.Name,
		TenantID: tenant.ID, TenantCode: tenant.Code, TenantName: tenant.Name,
		OutletID: outlet.ID, OutletCode: outlet.Code, OutletName: outlet.Name,
	}, iam.Permissions{All: true}, access, refresh, true), nil
}

func (s *Service) acquire(ctx context.Context) error {
	select {
	case s.hashSem <- struct{}{}:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (s *Service) release() { <-s.hashSem }

func (s *Service) hash(ctx context.Context, password string) (string, error) {
	if err := s.acquire(ctx); err != nil {
		return "", err
	}
	defer s.release()
	return pauth.HashPassword(password)
}

// tenantCode = slug nama bisnis + sufiks acak 4 hex, cocok dengan CHECK tenants_code_format.
func tenantCode(name string) (string, error) {
	b := make([]byte, 2)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("acak: %w", err)
	}
	base := sanitize.Slug(name, 24)
	if base == "" {
		base = "toko"
	}
	return base + "-" + hex.EncodeToString(b), nil
}
