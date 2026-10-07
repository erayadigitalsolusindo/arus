package auth

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"aciraba/internal/audit"
	gen "aciraba/internal/gen"
	pauth "aciraba/internal/platform/auth"
	"aciraba/internal/platform/db"
	"aciraba/internal/platform/mailer"
	"aciraba/internal/platform/sanitize"
)

// RequestPasswordReset memproses permintaan "lupa password" di latar belakang dan kembali segera, sehingga waktu
// respons tidak membedakan email yang terdaftar dari yang tidak (anti-enumerasi). Email hanya dikirim bila akun
// ada dan aktif. email harus sudah dinormalkan (sanitize.Email).
func (s *Service) RequestPasswordReset(email, lang string) {
	s.Jobs.Go("password-reset", func(ctx context.Context) error {
		acc, err := accountByEmail(ctx, s.Pool, email)
		if isNoAccount(err) || (err == nil && (!acc.UserActive || !acc.TenantActive)) {
			return nil
		}
		if err != nil {
			return err
		}
		tok, err := s.OneTime.Issue(ctx, pauth.PurposePasswordReset, acc.UserID.String(), pauth.PasswordResetTTL)
		if err != nil {
			return err
		}
		msg, err := mailer.ResetPassword(lang, acc.Email, acc.UserName, s.BaseURL+"/reset-password#token="+tok)
		if err != nil {
			return err
		}
		return s.Mailer.Send(ctx, msg)
	})
}

// ResetPassword menetapkan password baru lewat token dari email. Urutan: baca token tanpa memakainya → validasi password
// (token tidak terbakar bila password lemah) → pakai token (atomik, sekali) → simpan hash, cabut token akses lama
// (tokens_valid_after), lalu cabut SEMUA sesi refresh pengguna di semua perangkat.
func (s *Service) ResetPassword(ctx context.Context, token, password string) error {
	subject, err := s.OneTime.Peek(ctx, pauth.PurposePasswordReset, token)
	if err != nil {
		return err
	}
	uid, err := uuid.Parse(subject)
	if err != nil {
		return pauth.ErrInvalidToken
	}
	acc, err := accountByUserID(ctx, s.Pool, uid, uuid.Nil)
	if isNoAccount(err) || (err == nil && (!acc.UserActive || !acc.TenantActive)) {
		return pauth.ErrInvalidToken
	}
	if err != nil {
		return err
	}
	if code := sanitize.Password(password, acc.Email); code != "" {
		return FieldErrors{"password": code}
	}
	if _, err := s.OneTime.Consume(ctx, pauth.PurposePasswordReset, token); err != nil {
		return err // dipakai permintaan lain di antara Peek dan Consume
	}
	hash, err := s.hash(ctx, password)
	if err != nil {
		return err
	}
	err = db.WithTenant(ctx, s.Pool, acc.TenantID, func(tx pgx.Tx) error {
		n, err := gen.New(tx).AuthSetPassword(ctx, gen.AuthSetPasswordParams{TenantID: acc.TenantID, ID: acc.UserID, PasswordHash: hash, ValidAfter: pgtype.Timestamptz{Time: s.now(), Valid: true}})
		if err != nil {
			return err
		}
		if n == 0 {
			return pgx.ErrNoRows
		}
		return audit.Record(ctx, tx, auditActor(acc), audit.Entry{
			Action: audit.ActionPasswordReset, Entity: audit.EntityUser, EntityID: acc.UserID.String(),
		})
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return pauth.ErrInvalidToken
	}
	if err != nil {
		return err
	}
	s.Perms.InvalidateTenant(acc.TenantID)
	return s.Sessions.RevokeUser(ctx, acc.UserID.String())
}
