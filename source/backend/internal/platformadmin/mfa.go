package platformadmin

import (
	"context"
	"errors"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	gen "aciraba/internal/gen"
	pauth "aciraba/internal/platform/auth"
)

// Aksi audit 2FA.
const (
	ActionMFAEnable   = "platform.mfa_enable"
	ActionMFADisable  = "platform.mfa_disable"
	ActionMFAReset    = "platform.mfa_reset"
	ActionMFARecovery = "platform.mfa_recovery"

	totpIssuer    = "ACIRABA Platform"
	recoveryCount = 10
)

var (
	ErrInvalidCode     = errors.New("kode 2FA salah")
	ErrInvalidMFAToken = errors.New("tantangan 2FA tidak valid atau kedaluwarsa")
	ErrMFAEnabled      = errors.New("2FA sudah aktif")
	ErrMFANotPending   = errors.New("pendaftaran 2FA belum dimulai")
)

func (s *Service) adminRow(ctx context.Context, id uuid.UUID) (gen.PlatformAdmin, error) {
	row, err := gen.New(s.Pool).PlatformAdminByID(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return row, ErrInvalidSession
	}
	return row, err
}

// secondFactor memeriksa kode 6 digit TOTP (periodenya ditandai terpakai: tidak bisa diulang) atau, bila
// allowRecovery, kode pemulihan (sekali pakai). false = salah/sudah dipakai.
func (s *Service) secondFactor(ctx context.Context, row gen.PlatformAdmin, code string, allowRecovery bool) (bool, error) {
	if !row.TotpEnabledAt.Valid || row.TotpSecretEnc.String == "" {
		return false, nil
	}
	if rec, ok := pauth.NormalizeRecovery(code); ok && allowRecovery { // 10 karakter ≠ 6 digit TOTP: tak ada tabrakan
		var used bool
		err := s.Pool.QueryRow(ctx, `SELECT platform_admin_use_recovery($1, $2)`, row.ID, pauth.RecoveryHash(rec)).Scan(&used)
		return used, err
	}
	secret, err := s.TOTP.Open(row.TotpSecretEnc.String, row.ID.String())
	if err != nil {
		return false, err
	}
	step, ok := pauth.VerifyTOTP(secret, code, s.now())
	if !ok {
		return false, nil
	}
	var fresh bool
	if err := s.Pool.QueryRow(ctx, `SELECT platform_admin_totp_step($1, $2)`, row.ID, step).Scan(&fresh); err != nil {
		return false, err
	}
	return fresh, nil
}

// MFAChallengeAdmin membaca tantangan login TANPA memakainya (agar salah kode tidak membakar tantangan; percobaan
// dibatasi Lockout di handler). Mengembalikan id admin.
func (s *Service) MFAChallengeAdmin(ctx context.Context, token string) (uuid.UUID, error) {
	subject, err := s.OneTime.Peek(ctx, pauth.PurposePlatformMFA, token)
	if errors.Is(err, pauth.ErrInvalidToken) {
		return uuid.Nil, ErrInvalidMFAToken
	}
	if err != nil {
		return uuid.Nil, err
	}
	id, _, _ := strings.Cut(subject, "|")
	adminID, err := uuid.Parse(id)
	if err != nil {
		return uuid.Nil, ErrInvalidMFAToken
	}
	return adminID, nil
}

// LoginMFA menyelesaikan login: kode TOTP atau kode pemulihan benar → tantangan dipakai dan sesi terbit.
func (s *Service) LoginMFA(ctx context.Context, token, code string) (*Session, error) {
	subject, err := s.OneTime.Peek(ctx, pauth.PurposePlatformMFA, token)
	if errors.Is(err, pauth.ErrInvalidToken) {
		return nil, ErrInvalidMFAToken
	}
	if err != nil {
		return nil, err
	}
	id, flag, _ := strings.Cut(subject, "|")
	adminID, err := uuid.Parse(id)
	if err != nil {
		return nil, ErrInvalidMFAToken
	}
	row, err := s.adminRow(ctx, adminID)
	if errors.Is(err, ErrInvalidSession) || (err == nil && !row.Active) {
		return nil, ErrInvalidMFAToken
	}
	if err != nil {
		return nil, err
	}
	ok, err := s.secondFactor(ctx, row, code, true)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, ErrInvalidCode
	}
	if _, err := s.OneTime.Consume(ctx, pauth.PurposePlatformMFA, token); err != nil {
		return nil, ErrInvalidMFAToken // keduluan permintaan lain dengan tantangan yang sama
	}
	return s.finishLogin(ctx, row, flag == "1")
}

// MFAStatus = status 2FA akun yang sedang masuk.
type MFAStatus struct {
	Enabled           bool `json:"enabled"`
	Pending           bool `json:"pending"` // rahasia sudah dibuat tetapi belum dikonfirmasi
	RecoveryRemaining int  `json:"recovery_remaining"`
}

func (s *Service) MFAStatus(ctx context.Context, a Actor) (*MFAStatus, error) {
	row, err := s.adminRow(ctx, a.ID)
	if err != nil {
		return nil, err
	}
	n, err := gen.New(s.Pool).PlatformRecoveryRemaining(ctx, a.ID)
	if err != nil {
		return nil, err
	}
	return &MFAStatus{Enabled: row.TotpEnabledAt.Valid, Pending: !row.TotpEnabledAt.Valid && row.TotpSecretEnc.String != "", RecoveryRemaining: int(n)}, nil
}

// MFASetup = rahasia untuk didaftarkan di aplikasi autentikator (tampilkan QR dari URL, atau ketik rahasia manual).
type MFASetup struct {
	Secret string `json:"secret"`
	URL    string `json:"url"`
}

// BeginMFA membuat rahasia baru (menimpa pendaftaran yang belum dikonfirmasi). Ditolak bila 2FA sudah aktif.
func (s *Service) BeginMFA(ctx context.Context, a Actor) (*MFASetup, error) {
	row, err := s.adminRow(ctx, a.ID)
	if err != nil {
		return nil, err
	}
	if row.TotpEnabledAt.Valid {
		return nil, ErrMFAEnabled
	}
	secret, err := pauth.NewTOTPSecret()
	if err != nil {
		return nil, err
	}
	sealed, err := s.TOTP.Seal(secret, row.ID.String())
	if err != nil {
		return nil, err
	}
	var ok bool
	if err := s.Pool.QueryRow(ctx, `SELECT platform_admin_totp_begin($1, $2)`, row.ID, sealed).Scan(&ok); err != nil {
		return nil, err
	}
	if !ok {
		return nil, ErrMFAEnabled
	}
	return &MFASetup{Secret: secret, URL: pauth.TOTPURL(totpIssuer, row.Email, secret)}, nil
}

// EnableMFA mengonfirmasi pendaftaran dengan kode pertama, mengaktifkan 2FA, dan mengembalikan kode pemulihan
// (hanya ditampilkan sekali).
func (s *Service) EnableMFA(ctx context.Context, a Actor, code string) ([]string, error) {
	row, err := s.adminRow(ctx, a.ID)
	if err != nil {
		return nil, err
	}
	if row.TotpEnabledAt.Valid {
		return nil, ErrMFAEnabled
	}
	if row.TotpSecretEnc.String == "" {
		return nil, ErrMFANotPending
	}
	secret, err := s.TOTP.Open(row.TotpSecretEnc.String, row.ID.String())
	if err != nil {
		return nil, err
	}
	step, ok := pauth.VerifyTOTP(secret, code, s.now())
	if !ok {
		return nil, ErrInvalidCode
	}
	codes, hashes, err := pauth.NewRecoveryCodes(recoveryCount)
	if err != nil {
		return nil, err
	}
	var enabled bool
	if err := s.Pool.QueryRow(ctx, `SELECT platform_admin_totp_enable($1, $2, $3, $4)`, row.ID, step, hashes, s.now()).Scan(&enabled); err != nil {
		return nil, err
	}
	if !enabled {
		return nil, ErrMFAEnabled
	}
	s.forget(a.ID)
	s.record(ctx, a, ActionMFAEnable, uuid.Nil, "", nil)
	return codes, nil
}

// RegenerateRecovery mengganti semua kode pemulihan (yang lama hangus) setelah verifikasi kode TOTP.
func (s *Service) RegenerateRecovery(ctx context.Context, a Actor, code string) ([]string, error) {
	row, err := s.adminRow(ctx, a.ID)
	if err != nil {
		return nil, err
	}
	if ok, err := s.secondFactor(ctx, row, code, false); err != nil {
		return nil, err
	} else if !ok {
		return nil, ErrInvalidCode
	}
	codes, hashes, err := pauth.NewRecoveryCodes(recoveryCount)
	if err != nil {
		return nil, err
	}
	if _, err := s.Pool.Exec(ctx, `SELECT platform_admin_recovery_replace($1, $2)`, a.ID, hashes); err != nil {
		return nil, err
	}
	s.record(ctx, a, ActionMFARecovery, uuid.Nil, "", nil)
	return codes, nil
}

// DisableOwnMFA mematikan 2FA sendiri: wajib password + kode (TOTP atau pemulihan). Semua sesi dicabut; setelah masuk
// lagi admin wajib mendaftar ulang 2FA.
func (s *Service) DisableOwnMFA(ctx context.Context, a Actor, password, code string) error {
	row, err := s.adminRow(ctx, a.ID)
	if err != nil {
		return err
	}
	ok, err := s.verify(ctx, password, row.PasswordHash)
	if err != nil {
		return err
	}
	if !ok {
		return ErrInvalidCredentials
	}
	if ok, err := s.secondFactor(ctx, row, code, true); err != nil {
		return err
	} else if !ok {
		return ErrInvalidCode
	}
	if err := s.resetMFA(ctx, a, a.ID); err != nil {
		return err
	}
	s.record(ctx, a, ActionMFADisable, uuid.Nil, "", nil)
	return nil
}

// ResetAdminMFA: admin lain menghapus 2FA seorang admin (perangkat hilang + kode pemulihan habis). Tercatat di audit.
func (s *Service) ResetAdminMFA(ctx context.Context, a Actor, target uuid.UUID) error {
	if a.ID == target {
		return ErrSelf
	}
	if err := s.resetMFA(ctx, a, target); err != nil {
		return err
	}
	s.record(ctx, a, ActionMFAReset, uuid.Nil, "", map[string]any{"target_id": target.String()})
	return nil
}

func (s *Service) resetMFA(ctx context.Context, a Actor, target uuid.UUID) error {
	var ok bool
	err := s.Pool.QueryRow(ctx, `SELECT platform_admin_totp_reset($1, $2, $3)`, a.ID, target, s.now()).Scan(&ok)
	switch {
	case pgCode(err) == "42501":
		return ErrInvalidSession
	case err != nil:
		return err
	case !ok:
		return ErrNotFound
	}
	s.forget(target)
	return s.Sessions.RevokeUser(ctx, target.String())
}
