// Package usermfa: 2FA TOTP opsional untuk pengguna tenant (daftar, aktifkan, kode pemulihan, nonaktifkan, verifikasi saat login).
// Dipakai bersama paket auth (langkah kedua login) dan profile (pengaturan di halaman profil).
package usermfa

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"aciraba/internal/audit"
	"aciraba/internal/authz"
	gen "aciraba/internal/gen"
	pauth "aciraba/internal/platform/auth"
	"aciraba/internal/platform/db"
)

const (
	issuer        = "ACIRABA"
	recoveryCount = 10
)

var (
	ErrInvalidCode  = errors.New("kode 2FA salah")
	ErrEnabled      = errors.New("2FA sudah aktif")
	ErrNotPending   = errors.New("pendaftaran 2FA belum dimulai")
	ErrBadPassword  = errors.New("password salah")
	ErrForbidden    = errors.New("tidak boleh mengubah 2FA ini")
	ErrUserNotFound = errors.New("pengguna tidak ditemukan")
)

type Service struct {
	pool     *pgxpool.Pool
	box      *pauth.TOTPBox
	sessions *pauth.Sessions
	now      func() time.Time
}

func New(pool *pgxpool.Pool, box *pauth.TOTPBox, sessions *pauth.Sessions) *Service {
	return &Service{pool: pool, box: box, sessions: sessions, now: time.Now}
}

// Enabled: apakah pengguna sudah mengaktifkan 2FA.
func (s *Service) Enabled(ctx context.Context, tenant, user uuid.UUID) (bool, error) {
	var on bool
	err := db.WithTenant(ctx, s.pool, tenant, func(tx pgx.Tx) (err error) {
		on, err = gen.New(tx).MfaEnabled(ctx, gen.MfaEnabledParams{TenantID: tenant, ID: user})
		return
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	return on, err
}

// check memeriksa kode TOTP (periodenya ditandai terpakai: tidak bisa diulang) atau, bila allowRecovery, kode pemulihan
// sekali pakai. Dijalankan di dalam transaksi pemanggil. false = salah / sudah dipakai / 2FA tidak aktif.
func (s *Service) check(ctx context.Context, q *gen.Queries, tenant, user uuid.UUID, row gen.MfaGetRow, code string, allowRecovery bool) (bool, error) {
	if !row.TotpEnabledAt.Valid || row.TotpSecretEnc.String == "" {
		return false, nil
	}
	if rec, ok := pauth.NormalizeRecovery(code); ok && allowRecovery { // 10 karakter ≠ 6 digit TOTP: tak ada tabrakan
		n, err := q.MfaRecoveryUse(ctx, gen.MfaRecoveryUseParams{TenantID: tenant, UserID: user, CodeHash: pauth.RecoveryHash(rec)})
		return n > 0, err
	}
	secret, err := s.box.Open(row.TotpSecretEnc.String, user.String())
	if err != nil {
		return false, err
	}
	step, ok := pauth.VerifyTOTP(secret, code, s.now())
	if !ok {
		return false, nil
	}
	n, err := q.MfaUseStep(ctx, gen.MfaUseStepParams{TenantID: tenant, ID: user, Step: step})
	return n > 0, err
}

// Verify = langkah kedua login: true bila kode TOTP/pemulihan benar.
func (s *Service) Verify(ctx context.Context, tenant, user uuid.UUID, code string) (bool, error) {
	var ok bool
	err := db.WithTenant(ctx, s.pool, tenant, func(tx pgx.Tx) error {
		q := gen.New(tx)
		row, err := q.MfaGet(ctx, gen.MfaGetParams{TenantID: tenant, ID: user})
		if err != nil {
			return err
		}
		ok, err = s.check(ctx, q, tenant, user, row, code, true)
		return err
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	return ok, err
}

type Status struct {
	Enabled           bool `json:"enabled"`
	Pending           bool `json:"pending"` // rahasia sudah dibuat tetapi belum dikonfirmasi
	RecoveryRemaining int  `json:"recovery_remaining"`
}

func (s *Service) Status(ctx context.Context, a authz.Actor) (*Status, error) {
	var st Status
	err := db.WithTenant(ctx, s.pool, a.TenantID, func(tx pgx.Tx) error {
		q := gen.New(tx)
		row, err := q.MfaGet(ctx, gen.MfaGetParams{TenantID: a.TenantID, ID: a.UserID})
		if err != nil {
			return err
		}
		n, err := q.MfaRecoveryRemaining(ctx, gen.MfaRecoveryRemainingParams{TenantID: a.TenantID, UserID: a.UserID})
		if err != nil {
			return err
		}
		st = Status{Enabled: row.TotpEnabledAt.Valid, Pending: !row.TotpEnabledAt.Valid && row.TotpSecretEnc.String != "", RecoveryRemaining: int(n)}
		return nil
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrUserNotFound
	}
	return &st, err
}

// Setup = rahasia untuk didaftarkan di aplikasi autentikator (QR dari URL, atau ketik rahasia manual).
type Setup struct {
	Secret string `json:"secret"`
	URL    string `json:"url"`
}

// Begin membuat rahasia baru (menimpa pendaftaran yang belum dikonfirmasi). Ditolak bila 2FA sudah aktif.
func (s *Service) Begin(ctx context.Context, a authz.Actor) (*Setup, error) {
	if a.Impersonator != uuid.Nil {
		return nil, ErrForbidden
	}
	secret, err := pauth.NewTOTPSecret()
	if err != nil {
		return nil, err
	}
	sealed, err := s.box.Seal(secret, a.UserID.String())
	if err != nil {
		return nil, err
	}
	var email string
	err = db.WithTenant(ctx, s.pool, a.TenantID, func(tx pgx.Tx) error {
		q := gen.New(tx)
		row, err := q.MfaGet(ctx, gen.MfaGetParams{TenantID: a.TenantID, ID: a.UserID})
		if err != nil {
			return err
		}
		email = row.Email
		n, err := q.MfaBegin(ctx, gen.MfaBeginParams{TenantID: a.TenantID, ID: a.UserID, SecretEnc: pgtype.Text{String: sealed, Valid: true}})
		if err != nil {
			return err
		}
		if n == 0 {
			return ErrEnabled
		}
		return nil
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrUserNotFound
	}
	if err != nil {
		return nil, err
	}
	return &Setup{Secret: secret, URL: pauth.TOTPURL(issuer, email, secret)}, nil
}

func (s *Service) replaceRecovery(ctx context.Context, q *gen.Queries, tenant, user uuid.UUID) ([]string, error) {
	codes, hashes, err := pauth.NewRecoveryCodes(recoveryCount)
	if err != nil {
		return nil, err
	}
	if err := q.MfaRecoveryDelete(ctx, gen.MfaRecoveryDeleteParams{TenantID: tenant, UserID: user}); err != nil {
		return nil, err
	}
	if err := q.MfaRecoveryInsert(ctx, gen.MfaRecoveryInsertParams{TenantID: tenant, UserID: user, Hashes: hashes}); err != nil {
		return nil, err
	}
	return codes, nil
}

func record(ctx context.Context, tx pgx.Tx, a authz.Actor, action string, target uuid.UUID) error {
	return audit.Record(ctx, tx, audit.FromActor(a), audit.Entry{Action: action, Entity: audit.EntityUser, EntityID: target.String()})
}

// Enable mengonfirmasi pendaftaran dengan kode pertama, mengaktifkan 2FA, dan mengembalikan kode pemulihan
// (hanya ditampilkan sekali).
func (s *Service) Enable(ctx context.Context, a authz.Actor, code string) ([]string, error) {
	if a.Impersonator != uuid.Nil {
		return nil, ErrForbidden
	}
	var codes []string
	err := db.WithTenant(ctx, s.pool, a.TenantID, func(tx pgx.Tx) error {
		q := gen.New(tx)
		row, err := q.MfaGet(ctx, gen.MfaGetParams{TenantID: a.TenantID, ID: a.UserID})
		if err != nil {
			return err
		}
		if row.TotpEnabledAt.Valid {
			return ErrEnabled
		}
		if row.TotpSecretEnc.String == "" {
			return ErrNotPending
		}
		secret, err := s.box.Open(row.TotpSecretEnc.String, a.UserID.String())
		if err != nil {
			return err
		}
		step, ok := pauth.VerifyTOTP(secret, code, s.now())
		if !ok {
			return ErrInvalidCode
		}
		if n, err := q.MfaEnable(ctx, gen.MfaEnableParams{TenantID: a.TenantID, ID: a.UserID, Step: step}); err != nil {
			return err
		} else if n == 0 {
			return ErrEnabled
		}
		if codes, err = s.replaceRecovery(ctx, q, a.TenantID, a.UserID); err != nil {
			return err
		}
		return record(ctx, tx, a, audit.ActionUserMFAEnable, a.UserID)
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrUserNotFound
	}
	return codes, err
}

// Regenerate mengganti semua kode pemulihan (yang lama hangus) setelah verifikasi kode TOTP.
func (s *Service) Regenerate(ctx context.Context, a authz.Actor, code string) ([]string, error) {
	if a.Impersonator != uuid.Nil {
		return nil, ErrForbidden
	}
	var codes []string
	err := db.WithTenant(ctx, s.pool, a.TenantID, func(tx pgx.Tx) error {
		q := gen.New(tx)
		row, err := q.MfaGet(ctx, gen.MfaGetParams{TenantID: a.TenantID, ID: a.UserID})
		if err != nil {
			return err
		}
		ok, err := s.check(ctx, q, a.TenantID, a.UserID, row, code, false)
		if err != nil {
			return err
		}
		if !ok {
			return ErrInvalidCode
		}
		if codes, err = s.replaceRecovery(ctx, q, a.TenantID, a.UserID); err != nil {
			return err
		}
		return record(ctx, tx, a, audit.ActionUserMFARecovery, a.UserID)
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrUserNotFound
	}
	return codes, err
}

// Disable mematikan 2FA sendiri: wajib password + kode (TOTP atau pemulihan).
func (s *Service) Disable(ctx context.Context, a authz.Actor, password, code string) error {
	if a.Impersonator != uuid.Nil {
		return ErrForbidden
	}
	err := db.WithTenant(ctx, s.pool, a.TenantID, func(tx pgx.Tx) error {
		q := gen.New(tx)
		row, err := q.MfaGet(ctx, gen.MfaGetParams{TenantID: a.TenantID, ID: a.UserID})
		if err != nil {
			return err
		}
		if ok, _ := pauth.VerifyPassword(password, row.PasswordHash); !ok {
			return ErrBadPassword
		}
		ok, err := s.check(ctx, q, a.TenantID, a.UserID, row, code, true)
		if err != nil {
			return err
		}
		if !ok {
			return ErrInvalidCode
		}
		if err := s.reset(ctx, q, a.TenantID, a.UserID, false); err != nil {
			return err
		}
		return record(ctx, tx, a, audit.ActionUserMFADisable, a.UserID)
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrUserNotFound
	}
	return err
}

func (s *Service) reset(ctx context.Context, q *gen.Queries, tenant, user uuid.UUID, revoke bool) error {
	n, err := q.MfaReset(ctx, gen.MfaResetParams{TenantID: tenant, ID: user, Revoke: revoke, ValidAfter: pgtype.Timestamptz{Time: s.now(), Valid: true}})
	if err != nil {
		return err
	}
	if n == 0 {
		return pgx.ErrNoRows
	}
	return q.MfaRecoveryDelete(ctx, gen.MfaRecoveryDeleteParams{TenantID: tenant, UserID: user})
}

// Reset: pengguna berizin mengelola pengguna menghapus 2FA orang lain (perangkat hilang + kode pemulihan habis).
// Semua sesi target dicabut; tercatat di audit. Tidak untuk diri sendiri (pakai Disable).
func (s *Service) Reset(ctx context.Context, a authz.Actor, target uuid.UUID) error {
	if a.UserID == target || a.Impersonator != uuid.Nil {
		return ErrForbidden
	}
	err := db.WithTenant(ctx, s.pool, a.TenantID, func(tx pgx.Tx) error {
		if err := s.reset(ctx, gen.New(tx), a.TenantID, target, true); err != nil {
			return err
		}
		return record(ctx, tx, a, audit.ActionUserMFAReset, target)
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrUserNotFound
	}
	if err != nil {
		return err
	}
	return s.sessions.RevokeUser(ctx, target.String())
}
