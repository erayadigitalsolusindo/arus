package auth

import (
	"context"
	"sync"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"aciraba/internal/audit"
	"aciraba/internal/authz"
	gen "aciraba/internal/gen"
	pauth "aciraba/internal/platform/auth"
	"aciraba/internal/platform/db"
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
// semuanya ErrInvalidCredentials; ErrAccountDisabled dan ErrNoOutlet hanya muncul setelah password terbukti benar.
func (s *Service) Login(ctx context.Context, in LoginInput) (*Session, error) {
	acc, err := accountByEmail(ctx, s.Pool, in.Email)
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
	if acc.OutletID == uuid.Nil {
		return nil, ErrNoOutlet
	}

	access, err := s.issueAccess(acc, s.now())
	if err != nil {
		return nil, err
	}
	perms, err := s.Perms.For(ctx, acc.TenantID, acc.UserID)
	if err != nil {
		return nil, err
	}
	refresh, err := s.Sessions.Create(ctx, acc.UserID.String(), in.Remember)
	if err != nil {
		return nil, err
	}
	// Penanda login terakhir dan audit bersifat pelengkap; kegagalannya tidak boleh menggagalkan login.
	_ = db.WithTenant(ctx, s.Pool, acc.TenantID, func(tx pgx.Tx) error {
		if err := gen.New(tx).TouchLastLogin(ctx, gen.TouchLastLoginParams{TenantID: acc.TenantID, ID: acc.UserID}); err != nil {
			return err
		}
		return audit.Record(ctx, tx, auditActor(acc), audit.Entry{
			Action: audit.ActionLogin, Entity: audit.EntityUser, EntityID: acc.UserID.String(),
			Details: map[string]any{"remember": in.Remember},
		})
	})
	return buildSession(acc, perms.Perms, access, refresh, in.Remember), nil
}

// Refresh menukar refresh token dengan token akses baru dan merotasi refresh token. Pada jendela grace
// (token lama baru saja dirotasi oleh permintaan lain) hanya token akses yang diterbitkan: RefreshToken kosong.
// Pemakaian ulang token lama di luar grace mencabut seluruh rantai sesi. Outlet pilihan sesi (pindah outlet) dipertahankan.
func (s *Service) Refresh(ctx context.Context, token string) (*Session, error) {
	res, err := s.Sessions.Rotate(ctx, token)
	if err != nil {
		return nil, err
	}
	if res.Status == pauth.RotateInvalid || res.Status == pauth.RotateReuse {
		return nil, ErrInvalidSession
	}
	uid, err := uuid.Parse(res.UserID)
	if err != nil {
		return nil, ErrInvalidSession
	}
	var preferred uuid.UUID
	if v, err := s.Sessions.Outlet(ctx, res.Family); err != nil {
		return nil, err
	} else if v != "" {
		preferred, _ = uuid.Parse(v)
	}

	acc, err := accountByUserID(ctx, s.Pool, uid, preferred)
	bad := isNoAccount(err) || (err == nil && (!acc.UserActive || !acc.TenantActive || acc.OutletID == uuid.Nil))
	if bad {
		// Akun dihapus/dinonaktifkan atau kehilangan semua outlet sejak login: sesi tidak boleh hidup terus.
		if res.Token != "" {
			_ = s.Sessions.Revoke(ctx, res.Token)
		}
		return nil, ErrInvalidSession
	}
	if err != nil {
		return nil, err
	}
	access, err := s.issueAccess(acc, s.now())
	if err != nil {
		return nil, err
	}
	perms, err := s.Perms.For(ctx, acc.TenantID, acc.UserID)
	if err != nil {
		return nil, err
	}
	return buildSession(acc, perms.Perms, access, res.Token, res.Remember), nil
}

// Logout mencabut rantai refresh token. Token tak dikenal diabaikan.
func (s *Service) Logout(ctx context.Context, token string) error {
	return s.Sessions.Revoke(ctx, token)
}

// Me memuat profil untuk pemanggil yang sudah lolos authz.Authenticate (izin, outlet, dan pencabutan token sudah
// diperiksa); ErrInvalidSession bila akun hilang/nonaktif/berpindah tenant.
func (s *Service) Me(ctx context.Context, a authz.Actor) (*Profile, error) {
	acc, err := accountByUserID(ctx, s.Pool, a.UserID, a.OutletID)
	if isNoAccount(err) {
		return nil, ErrInvalidSession
	}
	if err != nil {
		return nil, err
	}
	if !acc.UserActive || !acc.TenantActive || acc.TenantID != a.TenantID || acc.OutletID != a.OutletID {
		return nil, ErrInvalidSession
	}
	p := profileOf(acc, a.Perms)
	return &p, nil
}

func auditActor(a account) audit.Actor {
	return audit.Actor{TenantID: a.TenantID, UserID: a.UserID, OutletID: a.OutletID, Name: a.UserName}
}
