// Package auth: pendaftaran tenant, login, refresh, logout, pemulihan password, verifikasi email, dan pindah outlet.
// Berkas dipecah per use-case: register.go, login.go (login/refresh/logout/me), recovery.go, verify.go, outlet.go.
// Use-case = satu transaksi DB (AGENTS.md §3.2); efek samping luar (email) berjalan setelah commit.
package auth

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"aciraba/internal/approval"
	"aciraba/internal/authz"
	pauth "aciraba/internal/platform/auth"
	"aciraba/internal/platform/background"
	"aciraba/internal/platform/mailer"
)

var (
	ErrInvalidCredentials = errors.New("email atau password salah")
	ErrAccountDisabled    = errors.New("akun atau tenant dinonaktifkan")
	ErrNoOutlet           = errors.New("akun tidak memiliki outlet aktif")
	ErrInvalidSession     = errors.New("sesi tidak valid")
	ErrOutletForbidden    = errors.New("tidak punya akses ke outlet ini")
	ErrEmailTaken         = errors.New("email sudah terdaftar")
)

// TermsVersion = versi syarat layanan yang disetujui saat mendaftar (disimpan di users.terms_version).
// Naikkan saat isi berubah bermakna; pengguna lama dapat diminta menyetujui ulang.
const TermsVersion = "2026-10"

const (
	// Batas hashing argon2id bersamaan (masing-masing ±64 MiB) agar lonjakan login/register tidak menghabiskan memori.
	maxConcurrentHashes = 4
	maxName             = 100
)

// Deps = ketergantungan layanan. Struct (bukan argumen panjang) agar menambah dependensi tidak mengubah semua pemanggil.
type Deps struct {
	Pool     *pgxpool.Pool
	Tokens   *pauth.TokenIssuer
	Sessions *pauth.Sessions
	OneTime  *pauth.OneTime
	Perms    *authz.Resolver
	Mailer   mailer.Mailer
	Jobs     *background.Runner
	// Approvals memverifikasi PIN penyetuju untuk pindah outlet dari kasir; nil = pindah dari kasir selalu ditolak.
	Approvals *approval.Service
	// BaseURL = alamat SPA untuk tautan di email, tanpa slash akhir (mis. https://app.contoh.id).
	BaseURL string
}

type Service struct {
	Deps
	hashSem chan struct{}
	now     func() time.Time
}

func NewService(d Deps) *Service {
	return &Service{Deps: d, hashSem: make(chan struct{}, maxConcurrentHashes), now: time.Now}
}

// FieldErrors = kode galat per field untuk respons VALIDATION.
type FieldErrors map[string]string

func (f FieldErrors) Error() string { return "input tidak valid" }

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

// verify memeriksa password di dalam batas konkurensi argon2id yang sama dengan hash.
func (s *Service) verify(ctx context.Context, password, encoded string) (bool, error) {
	if err := s.acquire(ctx); err != nil {
		return false, err
	}
	defer s.release()
	return pauth.VerifyPassword(password, encoded)
}
