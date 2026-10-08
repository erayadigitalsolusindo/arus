package auth

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"

	"aciraba/internal/audit"
	"aciraba/internal/authz"
	gen "aciraba/internal/gen"
	"aciraba/internal/member"
	"aciraba/internal/paymentmethod"
	"aciraba/internal/platform/db"
	"aciraba/internal/platform/sanitize"
)

const maxCodeAttempts = 5

var ownerPermissions = []byte(`{"*":true}`)

// RegisterInput = input mentah dari klien.
type RegisterInput struct {
	BusinessName string
	OwnerName    string
	Email        string
	Phone        string
	OutletName   string
	Password     string
	AcceptTerms  bool
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
	if !in.AcceptTerms {
		f["accept_terms"] = "REQUIRED"
	}
	if len(f) > 0 {
		return CleanRegister{}, f
	}
	return c, nil
}

// Register membuat tenant + outlet pertama + role Owner + user Owner dalam satu transaksi, lalu menerbitkan sesi dan
// (setelah commit) mengirim email verifikasi. Input harus sudah lolos ValidateRegister. lang = bahasa email.
func (s *Service) Register(ctx context.Context, in CleanRegister, lang string) (*Session, error) {
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
			s.queueVerification(sess.User.ID, in.Email, in.OwnerName, lang)
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
	err := db.WithTenant(ctx, s.Pool, tenantID, func(tx pgx.Tx) error {
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
			TenantID: tenant.ID, RoleID: role.ID, Email: in.Email, Name: in.OwnerName,
			Phone: pgtype.Text{String: in.Phone, Valid: true}, PasswordHash: hash, TermsVersion: pgtype.Text{String: TermsVersion, Valid: true},
		})
		if err != nil {
			return err
		}
		if err := member.SeedDefaultLevels(ctx, tx, tenant.ID); err != nil { // level member bawaan
			return err
		}
		if err := paymentmethod.SeedDefaults(ctx, tx, tenant.ID); err != nil { // metode pembayaran bawaan
			return err
		}
		return audit.Record(ctx, tx, audit.Actor{TenantID: tenant.ID, UserID: user.ID, OutletID: outlet.ID, Name: user.Name}, audit.Entry{
			Action: audit.ActionRegister, Entity: audit.EntityUser, EntityID: user.ID.String(),
			Details: map[string]any{"tenant": tenant.Code, "terms_version": TermsVersion},
		})
	})
	if err != nil {
		return nil, err
	}

	acc := account{
		UserID: user.ID, UserName: user.Name, Email: user.Email, RoleName: role.Name, UserActive: true,
		TenantID: tenant.ID, TenantCode: tenant.Code, TenantName: tenant.Name, TenantActive: true,
		OutletID: outlet.ID, OutletCode: outlet.Code, OutletName: outlet.Name,
	}
	access, err := s.issueAccess(acc, s.now())
	if err != nil {
		return nil, err
	}
	refresh, err := s.Sessions.Create(ctx, user.ID.String(), true)
	if err != nil {
		return nil, err
	}
	return buildSession(acc, authz.Permissions{All: true}, access, refresh, true), nil
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
