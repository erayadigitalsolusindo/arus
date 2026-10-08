package platformadmin

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"aciraba/internal/audit"
	"aciraba/internal/authz"
	gen "aciraba/internal/gen"
	pauth "aciraba/internal/platform/auth"
	"aciraba/internal/platform/httpx"
)

// Aksi audit platform (tabel platform_audit_log).
const (
	ActionSetup         = "platform.setup"
	ActionLogin         = "platform.login"
	ActionAdminCreate   = "platform.admin_create"
	ActionAdminStatus   = "platform.admin_status"
	ActionAdminPassword = "platform.admin_password"
	ActionTenantStatus  = "platform.tenant_status"
	ActionEditWindow    = "platform.tenant_edit_window"
	ActionImpersonate   = "platform.impersonate"

	platformRole = "platform"
)

type Profile struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	Email      string `json:"email"`
	MFAEnabled bool   `json:"mfa_enabled"`
}

type Session struct {
	AccessToken string  `json:"access_token"`
	ExpiresIn   int     `json:"expires_in"`
	Admin       Profile `json:"admin"`
	// RefreshToken kosong pada jendela grace (cookie tidak diubah).
	RefreshToken string `json:"-"`
	Remember     bool   `json:"-"`
}

// Actor = Platform Admin yang terautentikasi (dari token platform).
type Actor struct {
	ID   uuid.UUID
	Name string
	MFA  bool // 2FA aktif; tanpa ini hanya halaman keamanan yang boleh diakses
}

type actorKey struct{}

func ActorFrom(ctx context.Context) (Actor, bool) {
	a, ok := ctx.Value(actorKey{}).(Actor)
	return a, ok
}

var dummyHash = sync.OnceValue(func() string {
	h, err := pauth.HashPassword("dummy-password-for-timing-only")
	if err != nil {
		panic(err)
	}
	return h
})

func (s *Service) newSession(ctx context.Context, row gen.PlatformAdmin, refresh string, remember, issueRefresh bool) (*Session, error) {
	access, err := s.PTokens.Issue(row.ID.String(), "", "", platformRole, s.now())
	if err != nil {
		return nil, err
	}
	if issueRefresh {
		if refresh, err = s.Sessions.Create(ctx, row.ID.String(), remember); err != nil {
			return nil, err
		}
	}
	return &Session{
		AccessToken: access, ExpiresIn: int(pauth.AccessTTL.Seconds()),
		Admin:        Profile{ID: row.ID.String(), Name: row.Name, Email: row.Email, MFAEnabled: row.TotpEnabledAt.Valid},
		RefreshToken: refresh, Remember: remember,
	}, nil
}

// Login memverifikasi kredensial Platform Admin. Email salah/password salah/akun tak ada = ErrInvalidCredentials
// (hash dummy menyamakan waktu respons); ErrAccountDisabled hanya setelah password terbukti benar.
func (s *Service) Login(ctx context.Context, email, password string, remember bool) (*Session, string, error) {
	row, err := gen.New(s.Pool).PlatformAdminByEmail(ctx, email)
	found := err == nil
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return nil, "", err
	}
	hash := dummyHash()
	if found {
		hash = row.PasswordHash
	}
	ok, err := s.verify(ctx, password, hash)
	if err != nil {
		return nil, "", err
	}
	if !found || !ok {
		return nil, "", ErrInvalidCredentials
	}
	if !row.Active {
		return nil, "", ErrAccountDisabled
	}
	if row.TotpEnabledAt.Valid {
		// Password benar tetapi belum cukup: kembalikan tantangan 2FA (token sekali pakai, 5 menit). Sesi baru terbit di LoginMFA.
		r := "0"
		if remember {
			r = "1"
		}
		tok, err := s.OneTime.Issue(ctx, pauth.PurposePlatformMFA, row.ID.String()+"|"+r, pauth.PlatformMFATTL)
		return nil, tok, err
	}
	sess, err := s.finishLogin(ctx, row, remember)
	return sess, "", err
}

// finishLogin menerbitkan sesi setelah semua faktor terpenuhi.
func (s *Service) finishLogin(ctx context.Context, row gen.PlatformAdmin, remember bool) (*Session, error) {
	sess, err := s.newSession(ctx, row, "", remember, true)
	if err != nil {
		return nil, err
	}
	_, _ = s.Pool.Exec(ctx, `SELECT platform_admin_touch_login($1, $2)`, row.ID, s.now())
	s.record(ctx, Actor{ID: row.ID, Name: row.Name}, ActionLogin, uuid.Nil, "", map[string]any{"mfa": row.TotpEnabledAt.Valid})
	return sess, nil
}

// Refresh menukar cookie refresh dengan token akses baru (rotasi, grace, deteksi pemakaian ulang: lihat pauth.Sessions).
func (s *Service) Refresh(ctx context.Context, token string) (*Session, error) {
	res, err := s.Sessions.Rotate(ctx, token)
	if err != nil {
		return nil, err
	}
	if res.Status == pauth.RotateInvalid || res.Status == pauth.RotateReuse {
		return nil, ErrInvalidSession
	}
	id, err := uuid.Parse(res.UserID)
	if err != nil {
		return nil, ErrInvalidSession
	}
	row, err := gen.New(s.Pool).PlatformAdminByID(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && !row.Active) {
		if res.Token != "" {
			_ = s.Sessions.Revoke(ctx, res.Token)
		}
		return nil, ErrInvalidSession
	}
	if err != nil {
		return nil, err
	}
	return s.newSession(ctx, row, res.Token, res.Remember, false)
}

func (s *Service) Logout(ctx context.Context, token string) error {
	return s.Sessions.Revoke(ctx, token)
}

// Authenticate dipasang SETELAH httpx.RequireAuth(PTokens): memastikan admin masih aktif dan token belum dicabut.
func (s *Service) Authenticate(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		claims, ok := httpx.ClaimsFrom(r.Context())
		if !ok || claims.Role != platformRole {
			httpx.Error(w, http.StatusUnauthorized, "UNAUTHORIZED", "Belum masuk.")
			return
		}
		id, err := uuid.Parse(claims.Subject)
		if err != nil {
			httpx.Error(w, http.StatusUnauthorized, "UNAUTHORIZED", "Token tidak valid.")
			return
		}
		admin, err := s.Admin(r.Context(), id)
		switch {
		case errors.Is(err, authz.ErrInactive):
			httpx.Error(w, http.StatusUnauthorized, "UNAUTHORIZED", "Sesi tidak valid.")
			return
		case err != nil:
			httpx.Error(w, http.StatusInternalServerError, "INTERNAL", "Terjadi kesalahan pada server.")
			return
		}
		if claims.IssuedAt == nil || claims.IssuedAt.Time.Before(admin.ValidAfter.Truncate(time.Second)) {
			httpx.Error(w, http.StatusUnauthorized, "UNAUTHORIZED", "Sesi dicabut. Silakan masuk kembali.")
			return
		}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), actorKey{}, Actor{ID: id, Name: admin.Name, MFA: admin.MFA})))
	})
}

// Me = profil admin saat ini.
func (s *Service) Me(ctx context.Context, a Actor) (*Profile, error) {
	row, err := gen.New(s.Pool).PlatformAdminByID(ctx, a.ID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrInvalidSession
	}
	if err != nil {
		return nil, err
	}
	return &Profile{ID: row.ID.String(), Name: row.Name, Email: row.Email, MFAEnabled: row.TotpEnabledAt.Valid}, nil
}

// SetupRequired: setup hanya tersedia selama belum ada Platform Admin DAN SetupToken dikonfigurasi di server.
func (s *Service) SetupRequired(ctx context.Context) (bool, error) {
	if s.SetupToken == "" {
		return false, nil
	}
	n, err := gen.New(s.Pool).PlatformAdminCount(ctx)
	return n == 0, err
}

// Setup membuat Platform Admin pertama (atomik di DB: hanya berhasil bila tabel masih kosong) lalu langsung masuk.
func (s *Service) Setup(ctx context.Context, setupToken, name, email, password string) (*Session, error) {
	required, err := s.SetupRequired(ctx)
	if err != nil {
		return nil, err
	}
	if !required {
		return nil, ErrSetupUnavailable
	}
	if !sameSecret(setupToken, s.SetupToken) {
		return nil, ErrBadSetupToken
	}
	hash, err := s.hash(ctx, password)
	if err != nil {
		return nil, err
	}
	var id pgtype.UUID
	if err := s.Pool.QueryRow(ctx, `SELECT platform_admin_bootstrap($1, $2, $3)`, email, name, hash).Scan(&id); err != nil {
		return nil, err
	}
	if !id.Valid {
		return nil, ErrSetupUnavailable // keduluan setup lain
	}
	row, err := gen.New(s.Pool).PlatformAdminByID(ctx, id.Bytes)
	if err != nil {
		return nil, err
	}
	_, _ = s.Pool.Exec(ctx, `SELECT platform_admin_touch_login($1, $2)`, row.ID, s.now())
	s.record(ctx, Actor{ID: row.ID, Name: row.Name}, ActionSetup, uuid.Nil, "", nil)
	return s.newSession(ctx, row, "", false, true)
}

// record menulis audit platform sebagai pelengkap: kegagalan tidak menggagalkan tindakan. Untuk pencatatan yang
// harus atomik dengan perubahan, pakai recordTx di dalam transaksinya.
func (s *Service) record(ctx context.Context, a Actor, action string, tenantID uuid.UUID, tenantName string, details map[string]any) {
	_ = s.recordTx(ctx, s.Pool, a, action, tenantID, tenantName, details)
}

func (s *Service) recordTx(ctx context.Context, db gen.DBTX, a Actor, action string, tenantID uuid.UUID, tenantName string, details map[string]any) error {
	if details == nil {
		details = map[string]any{}
	}
	raw, err := json.Marshal(details)
	if err != nil {
		return err
	}
	m := audit.MetaFrom(ctx)
	var tid pgtype.UUID
	if tenantID != uuid.Nil {
		tid = pgtype.UUID{Bytes: tenantID, Valid: true}
	}
	return gen.New(db).PlatformAuditInsert(ctx, gen.PlatformAuditInsertParams{
		AdminID: pgtype.UUID{Bytes: a.ID, Valid: a.ID != uuid.Nil}, AdminName: a.Name,
		Action: action, TenantID: tid, TenantName: tenantName,
		Details: raw, Ip: m.IP, RequestID: m.RequestID,
	})
}
