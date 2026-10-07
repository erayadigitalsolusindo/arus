package auth

import (
	"context"
	"errors"
	"sync"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	gen "aciraba/internal/gen"
	"aciraba/internal/iam"
	pauth "aciraba/internal/platform/auth"
	"aciraba/internal/platform/db"
)

var (
	ErrInvalidCredentials = errors.New("email atau password salah")
	ErrAccountDisabled    = errors.New("akun atau tenant dinonaktifkan")
	ErrInvalidSession     = errors.New("sesi tidak valid")
)

// dummyHash dipakai saat email tidak ditemukan agar waktu respons sama dengan email yang ada
// (mencegah enumerasi akun lewat selisih waktu).
var dummyHash = sync.OnceValue(func() string {
	h, err := pauth.HashPassword("dummy-password-for-timing-only")
	if err != nil {
		panic(err)
	}
	return h
})

type LoginInput struct {
	Email    string // sudah dinormalkan (sanitize.Email)
	Password string
	Remember bool
}

// Login memverifikasi kredensial lalu menerbitkan sesi. Email salah, password salah, dan akun tak ada
// semuanya ErrInvalidCredentials; ErrAccountDisabled hanya muncul setelah password terbukti benar.
func (s *Service) Login(ctx context.Context, in LoginInput) (*Session, error) {
	acc, err := accountByEmail(ctx, s.pool, in.Email)
	found := err == nil
	if err != nil && !isNoAccount(err) {
		return nil, err
	}
	hash := dummyHash()
	if found {
		hash = acc.PasswordHash
	}
	ok, err := s.verify(ctx, in.Password, hash)
	if err != nil {
		return nil, err
	}
	if !found || !ok {
		return nil, ErrInvalidCredentials
	}
	if !acc.UserActive || !acc.TenantActive {
		return nil, ErrAccountDisabled
	}

	access, err := s.tokens.Issue(acc.UserID.String(), acc.TenantID.String(), acc.OutletID.String(), acc.RoleName, s.now())
	if err != nil {
		return nil, err
	}
	refresh, err := s.sessions.Create(ctx, acc.UserID.String(), in.Remember)
	if err != nil {
		return nil, err
	}
	// Penanda login terakhir bersifat informasi; kegagalannya tidak boleh menggagalkan login.
	_ = db.WithTenant(ctx, s.pool, acc.TenantID, func(tx pgx.Tx) error {
		return gen.New(tx).TouchLastLogin(ctx, gen.TouchLastLoginParams{TenantID: acc.TenantID, ID: acc.UserID})
	})
	perms, err := s.perms.For(ctx, acc.TenantID, acc.UserID)
	if err != nil {
		return nil, err
	}
	return buildSession(acc, perms, access, refresh, in.Remember), nil
}

// Refresh menukar refresh token dengan token akses baru dan merotasi refresh token. Pada jendela grace
// (token lama baru saja dirotasi oleh permintaan lain) hanya token akses yang diterbitkan: RefreshToken kosong.
// Pemakaian ulang token lama di luar grace mencabut seluruh rantai sesi.
func (s *Service) Refresh(ctx context.Context, token string) (*Session, error) {
	res, err := s.sessions.Rotate(ctx, token)
	if err != nil {
		return nil, err
	}
	if res.Status == pauth.RotateInvalid || res.Status == pauth.RotateReuse {
		return nil, ErrInvalidSession
	}
	acc, err := s.accountByID(ctx, res.UserID)
	if err != nil || !acc.UserActive || !acc.TenantActive {
		// Akun dihapus/dinonaktifkan sejak login: sesi tidak boleh hidup terus.
		if res.Token != "" {
			_ = s.sessions.Revoke(ctx, res.Token)
		}
		if err != nil && !errors.Is(err, ErrInvalidSession) {
			return nil, err
		}
		return nil, ErrInvalidSession
	}
	access, err := s.tokens.Issue(acc.UserID.String(), acc.TenantID.String(), acc.OutletID.String(), acc.RoleName, s.now())
	if err != nil {
		return nil, err
	}
	perms, err := s.perms.For(ctx, acc.TenantID, acc.UserID)
	if err != nil {
		return nil, err
	}
	return buildSession(acc, perms, access, res.Token, res.Remember), nil
}

// Logout mencabut rantai refresh token. Token tak dikenal diabaikan.
func (s *Service) Logout(ctx context.Context, token string) error {
	return s.sessions.Revoke(ctx, token)
}

// Me memuat profil untuk token akses yang sah; ErrInvalidSession bila akun hilang/nonaktif/pindah tenant.
func (s *Service) Me(ctx context.Context, userID, tenantID string) (*Profile, error) {
	uid, err1 := uuid.Parse(userID)
	tid, err2 := uuid.Parse(tenantID)
	if err1 != nil || err2 != nil {
		return nil, ErrInvalidSession
	}
	// Tenant dari token: query biasa di bawah RLS, jadi user milik tenant lain tidak mungkin terbaca.
	var acc account
	err := db.WithTenant(ctx, s.pool, tid, func(tx pgx.Tx) error {
		var qerr error
		acc, qerr = gen.New(tx).GetAccountInTenant(ctx, gen.GetAccountInTenantParams{TenantID: tid, ID: uid})
		return qerr
	})
	if isNoAccount(err) {
		return nil, ErrInvalidSession
	}
	if err != nil {
		return nil, err
	}
	if !acc.UserActive || !acc.TenantActive {
		return nil, ErrInvalidSession
	}
	perms, err := s.perms.For(ctx, acc.TenantID, acc.UserID)
	if err != nil {
		if errors.Is(err, iam.ErrInactive) {
			return nil, ErrInvalidSession
		}
		return nil, err
	}
	p := profileOf(acc, perms)
	return &p, nil
}

func (s *Service) accountByID(ctx context.Context, userID string) (account, error) {
	id, err := uuid.Parse(userID)
	if err != nil {
		return account{}, ErrInvalidSession
	}
	acc, err := accountByUserID(ctx, s.pool, id)
	if isNoAccount(err) {
		return account{}, ErrInvalidSession
	}
	if err != nil {
		return account{}, err
	}
	return acc, nil
}

func profileOf(a account, perms iam.Permissions) Profile {
	return Profile{
		Permissions: perms,
		User:        Identity{ID: a.UserID.String(), Name: a.UserName, Email: a.Email},
		Tenant:      Identity{ID: a.TenantID.String(), Code: a.TenantCode, Name: a.TenantName},
		Outlet:      Identity{ID: a.OutletID.String(), Code: a.OutletCode, Name: a.OutletName},
	}
}

func buildSession(a account, perms iam.Permissions, access, refresh string, remember bool) *Session {
	return &Session{
		AccessToken: access, ExpiresIn: int(pauth.AccessTTL.Seconds()), Profile: profileOf(a, perms),
		RefreshToken: refresh, Remember: remember,
	}
}

// verify memeriksa password di dalam batas konkurensi argon2id yang sama dengan hash.
func (s *Service) verify(ctx context.Context, password, encoded string) (bool, error) {
	if err := s.acquire(ctx); err != nil {
		return false, err
	}
	defer s.release()
	return pauth.VerifyPassword(password, encoded)
}
