package auth

import (
	"context"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"aciraba/internal/audit"
	gen "aciraba/internal/gen"
	"aciraba/internal/platform/db"
	"aciraba/internal/platform/sanitize"
)

// linkGoogle menautkan identitas Google ke pengguna (di dalam transaksi pemanggil).
func linkGoogle(ctx context.Context, tx pgx.Tx, subject string, tenantID, userID uuid.UUID) error {
	var ok bool
	if err := tx.QueryRow(ctx, "SELECT auth_identity_link($1, $2, $3, $4)", googleProviderID, subject, tenantID, userID).Scan(&ok); err != nil {
		return err
	}
	if !ok {
		return ErrGoogleLinked
	}
	return nil
}

// GoogleLogin masuk dengan ID token Google yang sudah diverifikasi. Urutan: identitas tertaut → email yang sama
// (Google sudah memverifikasi email, jadi tertaut otomatis) → ErrGoogleNotRegistered.
func (s *Service) GoogleLogin(ctx context.Context, c GoogleClaims, remember bool) (*Session, error) {
	email, code := sanitize.Email(c.Email)
	if code != "" {
		return nil, ErrGoogleInvalidToken
	}
	var uid uuid.UUID
	err := s.Pool.QueryRow(ctx, "SELECT auth_identity_user($1, $2)", googleProviderID, c.Subject).Scan(&uid)
	if err != nil {
		return nil, err
	}

	var acc account
	if uid != uuid.Nil {
		acc, err = accountByUserID(ctx, s.Pool, uid, uuid.Nil)
	} else {
		acc, err = accountByEmail(ctx, s.Pool, email)
	}
	if isNoAccount(err) {
		return nil, ErrGoogleNotRegistered
	}
	if err != nil {
		return nil, err
	}
	if !acc.UserActive || !acc.TenantActive {
		return nil, ErrAccountDisabled
	}

	if uid == uuid.Nil {
		err = db.WithTenant(ctx, s.Pool, acc.TenantID, func(tx pgx.Tx) error {
			if err := linkGoogle(ctx, tx, c.Subject, acc.TenantID, acc.UserID); err != nil {
				return err
			}
			return audit.Record(ctx, tx, auditActor(acc), audit.Entry{
				Action: audit.ActionLogin, Entity: audit.EntityUser, EntityID: acc.UserID.String(),
				Details: map[string]any{"google_linked": true},
			})
		})
		if err != nil {
			return nil, err
		}
	}
	if !acc.EmailVerified && strings.EqualFold(acc.Email, email) {
		// Google membuktikan pemilik email ini.
		if err := db.WithTenant(ctx, s.Pool, acc.TenantID, func(tx pgx.Tx) error {
			_, err := gen.New(tx).AuthMarkEmailVerified(ctx, gen.AuthMarkEmailVerifiedParams{TenantID: acc.TenantID, ID: acc.UserID})
			return err
		}); err != nil {
			return nil, err
		}
		acc.EmailVerified = true
	}
	return s.gate(ctx, acc, remember)
}

// GoogleRegisterInput = isian tambahan saat mendaftar lewat Google (nama pemilik dan email dari token).
type GoogleRegisterInput struct {
	BusinessName string
	OutletName   string
	Phone        string
	AcceptTerms  bool
}

// ValidateGoogleRegister menormalkan isian dan menggabungkannya dengan klaim Google.
func ValidateGoogleRegister(in GoogleRegisterInput, c GoogleClaims) (CleanRegister, map[string]string) {
	var out CleanRegister
	f := map[string]string{}
	set := func(field, code string) {
		if code != "" {
			f[field] = code
		}
	}
	var code string
	out.BusinessName, code = sanitize.Name(in.BusinessName, maxName)
	set("business_name", code)
	out.OutletName, code = sanitize.Name(in.OutletName, maxName)
	set("outlet_name", code)
	out.Phone, code = sanitize.Phone(in.Phone)
	set("phone", code)
	out.Email, code = sanitize.Email(c.Email)
	set("email", code)
	owner := strings.TrimSpace(c.Name)
	if owner == "" {
		owner, _, _ = strings.Cut(c.Email, "@")
	}
	if name, code := sanitize.Name(owner, maxName); code == "" {
		out.OwnerName = name
	} else {
		set("owner_name", code)
	}
	if !in.AcceptTerms {
		f["accept_terms"] = "REQUIRED"
	}
	if len(f) > 0 {
		return CleanRegister{}, f
	}
	out.GoogleSub = c.Subject
	return out, nil
}

// GoogleRegister membuat tenant baru untuk identitas Google. ErrEmailTaken bila email sudah dipakai akun lain.
func (s *Service) GoogleRegister(ctx context.Context, in CleanRegister, lang string) (*Session, error) {
	var uid uuid.UUID
	if err := s.Pool.QueryRow(ctx, "SELECT auth_identity_user($1, $2)", googleProviderID, in.GoogleSub).Scan(&uid); err != nil {
		return nil, err
	}
	if uid != uuid.Nil {
		return nil, ErrEmailTaken
	}
	return s.Register(ctx, in, lang)
}
