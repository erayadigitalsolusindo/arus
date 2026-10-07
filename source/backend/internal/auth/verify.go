package auth

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"aciraba/internal/audit"
	"aciraba/internal/authz"
	gen "aciraba/internal/gen"
	pauth "aciraba/internal/platform/auth"
	"aciraba/internal/platform/db"
	"aciraba/internal/platform/mailer"
)

// queueVerification menerbitkan token verifikasi dan mengirim emailnya di latar belakang (setelah transaksi commit).
// Gagal kirim hanya dicatat; pengguna dapat meminta kirim ulang.
func (s *Service) queueVerification(userID, email, name, lang string) {
	s.Jobs.Go("verify-email", func(ctx context.Context) error {
		tok, err := s.OneTime.Issue(ctx, pauth.PurposeVerifyEmail, userID, pauth.VerifyEmailTTL)
		if err != nil {
			return err
		}
		msg, err := mailer.VerifyEmail(lang, email, name, s.BaseURL+"/verify-email#token="+tok)
		if err != nil {
			return err
		}
		return s.Mailer.Send(ctx, msg)
	})
}

// ResendVerification mengirim ulang email verifikasi untuk pemanggil. Email yang sudah terverifikasi: tidak melakukan apa pun.
func (s *Service) ResendVerification(ctx context.Context, a authz.Actor, lang string) error {
	acc, err := accountByUserID(ctx, s.Pool, a.UserID, a.OutletID)
	if isNoAccount(err) {
		return ErrInvalidSession
	}
	if err != nil {
		return err
	}
	if acc.EmailVerified {
		return nil
	}
	s.queueVerification(acc.UserID.String(), acc.Email, acc.UserName, lang)
	return nil
}

// VerifyEmail memakai token (sekali pakai) dan menandai email terverifikasi. Idempotent bila sudah terverifikasi.
func (s *Service) VerifyEmail(ctx context.Context, token string) error {
	subject, err := s.OneTime.Consume(ctx, pauth.PurposeVerifyEmail, token)
	if err != nil {
		return err
	}
	uid, err := uuid.Parse(subject)
	if err != nil {
		return pauth.ErrInvalidToken
	}
	acc, err := accountByUserID(ctx, s.Pool, uid, uuid.Nil)
	if isNoAccount(err) {
		return pauth.ErrInvalidToken
	}
	if err != nil {
		return err
	}
	err = db.WithTenant(ctx, s.Pool, acc.TenantID, func(tx pgx.Tx) error {
		n, err := gen.New(tx).AuthMarkEmailVerified(ctx, gen.AuthMarkEmailVerifiedParams{TenantID: acc.TenantID, ID: acc.UserID})
		if err != nil {
			return err
		}
		if n == 0 {
			return pgx.ErrNoRows
		}
		return audit.Record(ctx, tx, auditActor(acc), audit.Entry{
			Action: audit.ActionEmailVerified, Entity: audit.EntityUser, EntityID: acc.UserID.String(),
		})
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return pauth.ErrInvalidToken
	}
	return err
}
