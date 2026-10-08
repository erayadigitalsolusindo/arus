// Package iam: manajemen role dan pengguna dalam satu tenant (PRD FR-AUTH-05). Izin, resolver, dan middleware
// otorisasinya ada di paket authz; pencatatan perubahan di paket audit.
package iam

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"aciraba/internal/authz"
	pauth "aciraba/internal/platform/auth"
)

var (
	ErrNotFound        = errors.New("data tidak ditemukan")
	ErrSystemRole      = errors.New("role sistem tidak dapat diubah atau dihapus")
	ErrRoleInUse       = errors.New("role masih dipakai pengguna")
	ErrNameTaken       = errors.New("nama role sudah dipakai")
	ErrEmailTaken      = errors.New("email sudah terdaftar")
	ErrEscalation      = errors.New("tidak boleh memberi izin melebihi izin sendiri")
	ErrLastOwner       = errors.New("pemilik aktif terakhir tidak boleh dinonaktifkan atau diganti rolenya")
	ErrSelfChange      = errors.New("tidak boleh menonaktifkan atau mengganti role akun sendiri")
	ErrSelfVerify      = errors.New("tidak boleh memverifikasi email akun sendiri")
	ErrOutletForbidden = errors.New("tidak punya akses ke outlet yang dipilih")
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
	resolver *authz.Resolver
	sessions *pauth.Sessions
	hashSem  chan struct{}
}

func NewService(pool *pgxpool.Pool, resolver *authz.Resolver, sessions *pauth.Sessions) *Service {
	return &Service{pool: pool, resolver: resolver, sessions: sessions, hashSem: make(chan struct{}, maxConcurrentHash)}
}

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

func isUnique(err error, constraint string) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505" && pgErr.ConstraintName == constraint
}
