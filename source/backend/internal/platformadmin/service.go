// Package platformadmin: Platform Admin = operator ACIRABA (pemilik aplikasi) yang dapat melihat semua tenant dan
// "masuk sebagai" ke tenant mana pun (hanya-baca) untuk menelusuri kejanggalan. Akun terpisah total dari pengguna tenant:
// tabel sendiri, token berkunci/issuer sendiri, cookie refresh sendiri, dan semua tindakan tercatat di audit platform
// (serta di audit tenant bila menyentuh tenant itu).
package platformadmin

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"errors"
	"net"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"aciraba/internal/authz"
	gen "aciraba/internal/gen"
	pauth "aciraba/internal/platform/auth"
)

var (
	ErrInvalidCredentials = errors.New("email atau password salah")
	ErrAccountDisabled    = errors.New("akun dinonaktifkan")
	ErrInvalidSession     = errors.New("sesi tidak valid")
	ErrSetupUnavailable   = errors.New("setup tidak tersedia")
	ErrBadSetupToken      = errors.New("token setup salah")
	ErrEmailTaken         = errors.New("email sudah terdaftar")
	ErrNotFound           = errors.New("tidak ditemukan")
	ErrLastAdmin          = errors.New("admin aktif terakhir")
	ErrSelf               = errors.New("tidak boleh pada diri sendiri")
	ErrOutletNotFound     = errors.New("outlet tidak ditemukan")
	ErrLanguageTaken      = errors.New("kode bahasa sudah terpakai")
	ErrLanguageProtected  = errors.New("bahasa dilindungi tidak dapat dihapus")
)

const (
	maxConcurrentHashes = 4
	maxName             = 100
	adminCacheTTL       = 10 * time.Second
)

// Deps = ketergantungan layanan.
type Deps struct {
	Pool     *pgxpool.Pool
	Tokens   *pauth.TokenIssuer // penerbit token tenant (token "masuk sebagai")
	PTokens  *pauth.TokenIssuer // penerbit token Platform Admin
	Sessions *pauth.Sessions
	OneTime  *pauth.OneTime  // tantangan login 2FA
	TOTP     *pauth.TOTPBox  // enkripsi rahasia TOTP di DB
	Perms    *authz.Resolver // dibuang cache-nya saat status tenant berubah
	// SetupToken = rahasia dari lingkungan server untuk membuat Platform Admin pertama; kosong = setup mati.
	SetupToken string
	// DomainTarget = host aplikasi yang dituju CNAME domain kustom tenant (mis. arus.erayadigital.co.id); kosong = verifikasi domain mati.
	DomainTarget string
}

type Service struct {
	Deps
	hashSem chan struct{}
	now     func() time.Time
	dns     dnsResolver

	mu    sync.Mutex
	cache map[uuid.UUID]cachedAdmin
}

type cachedAdmin struct {
	admin authz.PlatformAdmin
	err   error
	at    time.Time
}

func NewService(d Deps) *Service {
	return &Service{Deps: d, hashSem: make(chan struct{}, maxConcurrentHashes), now: time.Now, dns: net.DefaultResolver, cache: map[uuid.UUID]cachedAdmin{}}
}

// Admin memenuhi authz.PlatformChecker: status Platform Admin terkini (cache singkat; dibuang saat admin berubah).
func (s *Service) Admin(ctx context.Context, id uuid.UUID) (authz.PlatformAdmin, error) {
	s.mu.Lock()
	if e, ok := s.cache[id]; ok && s.now().Sub(e.at) < adminCacheTTL {
		s.mu.Unlock()
		return e.admin, e.err
	}
	s.mu.Unlock()

	var out authz.PlatformAdmin
	row, err := gen.New(s.Pool).PlatformAdminByID(ctx, id)
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		err = authz.ErrInactive
	case err != nil:
		return authz.PlatformAdmin{}, err // galat sementara: jangan di-cache
	case !row.Active:
		err = authz.ErrInactive
	default:
		out = authz.PlatformAdmin{Name: row.Name, ValidAfter: row.TokensValidAfter.Time, MFA: row.TotpEnabledAt.Valid}
	}
	s.mu.Lock()
	if len(s.cache) > 1000 {
		clear(s.cache)
	}
	s.cache[id] = cachedAdmin{admin: out, err: err, at: s.now()}
	s.mu.Unlock()
	return out, err
}

func (s *Service) forget(id uuid.UUID) {
	s.mu.Lock()
	delete(s.cache, id)
	s.mu.Unlock()
}

func (s *Service) hash(ctx context.Context, password string) (string, error) {
	if err := s.acquire(ctx); err != nil {
		return "", err
	}
	defer func() { <-s.hashSem }()
	return pauth.HashPassword(password)
}

func (s *Service) verify(ctx context.Context, password, encoded string) (bool, error) {
	if err := s.acquire(ctx); err != nil {
		return false, err
	}
	defer func() { <-s.hashSem }()
	return pauth.VerifyPassword(password, encoded)
}

func (s *Service) acquire(ctx context.Context) error {
	select {
	case s.hashSem <- struct{}{}:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// sameSecret membandingkan dua rahasia dalam waktu konstan (lewat hash agar panjang tidak bocor).
func sameSecret(a, b string) bool {
	ha, hb := sha256.Sum256([]byte(a)), sha256.Sum256([]byte(b))
	return subtle.ConstantTimeCompare(ha[:], hb[:]) == 1
}

// ---- languages ----

type Language struct {
	Code      string    `json:"code"`
	Name      string    `json:"name"`
	Short     string    `json:"short"`
	Intl      string    `json:"intl"`
	Protected bool      `json:"protected"`
	Active    bool      `json:"active"`
	SortOrder int       `json:"sort_order"`
	CreatedAt time.Time `json:"created_at"`
}

func (s *Service) ListLanguages(ctx context.Context, a Actor) ([]Language, error) {
	rows, err := s.Pool.Query(ctx, `SELECT code, name, short, intl, protected, active, sort_order, created_at
		FROM platform_list_languages($1)`, a.ID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Language{}
	for rows.Next() {
		var (
			r       Language
			sort    int32
			created pgtype.Timestamptz
		)
		if err := rows.Scan(&r.Code, &r.Name, &r.Short, &r.Intl, &r.Protected, &r.Active, &sort, &created); err != nil {
			return nil, err
		}
		r.SortOrder = int(sort)
		r.CreatedAt = created.Time
		out = append(out, r)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func (s *Service) CreateLanguage(ctx context.Context, a Actor, code, name, short, intl string, sortOrder int) (string, error) {
	_, err := gen.New(s.Pool).PlatformCreateLanguage(ctx, gen.PlatformCreateLanguageParams{
		ActorID: a.ID, Code: code, Name: name, Short: short, Intl: intl, SortOrder: int32(sortOrder),
	})
	switch {
	case pgCode(err) == "23505" || pgCode(err) == "P0001": // duplikat kode (PK bentrok / fungsi menolak)
		return "", ErrLanguageTaken
	case pgCode(err) == "42501": // fungsi DB menolak karena bukan platform admin aktif
		return "", ErrInvalidSession
	}
	if err != nil {
		return "", err
	}
	s.record(ctx, a, ActionLanguageCreate, uuid.Nil, "", map[string]any{"code": code, "name": name, "short": short, "intl": intl, "sort_order": sortOrder})
	return code, nil
}

func (s *Service) UpdateLanguage(ctx context.Context, a Actor, code, name string, active bool, sortOrder int) error {
	ok, err := gen.New(s.Pool).PlatformUpdateLanguage(ctx, gen.PlatformUpdateLanguageParams{
		ActorID: a.ID, Code: code, Name: name, Active: active, SortOrder: int32(sortOrder),
	})
	switch {
	case pgCode(err) == "42501":
		return ErrInvalidSession
	}
	if err != nil {
		return err
	}
	if !ok {
		return ErrNotFound
	}
	s.record(ctx, a, ActionLanguageUpdate, uuid.Nil, "", map[string]any{"code": code, "name": name, "active": active, "sort_order": sortOrder})
	return nil
}

func (s *Service) DeleteLanguage(ctx context.Context, a Actor, code string) error {
	_, err := gen.New(s.Pool).PlatformDeleteLanguage(ctx, gen.PlatformDeleteLanguageParams{
		ActorID: a.ID, Code: code,
	})
	switch {
	case pgCode(err) == "P0001": // platform_delete_language menolak protected
		return ErrLanguageProtected
	case pgCode(err) == "42501":
		return ErrInvalidSession
	}
	if err != nil {
		return err
	}
	s.record(ctx, a, ActionLanguageDelete, uuid.Nil, "", map[string]any{"code": code})
	return nil
}
