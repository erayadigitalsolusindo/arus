package extapi

import (
	"context"
	"crypto/rand"
	"encoding/base32"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/netip"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	ErrNotFound = errors.New("tidak ditemukan")
	ErrInvalid  = errors.New("data tidak valid")
	ErrTaken    = errors.New("sudah terpakai")
	ErrDefault  = errors.New("profil bawaan tidak dapat dihapus")
)

const (
	maxIPEntries = 50
	maxHidden    = 100
)

// Store = akses DB lewat fungsi SECURITY DEFINER (tabel tidak bisa diakses langsung oleh role aplikasi).
type Store struct {
	pool *pgxpool.Pool
	box  *secretBox
}

func NewStore(pool *pgxpool.Pool, jwtSecret string) (*Store, error) {
	box, err := newSecretBox(jwtSecret)
	if err != nil {
		return nil, err
	}
	return &Store{pool: pool, box: box}, nil
}

// ---- kredensial ----

type Credential struct {
	ID          uuid.UUID  `json:"id"`
	TenantID    uuid.UUID  `json:"tenant_id"`
	TenantName  string     `json:"tenant_name"`
	UserID      uuid.UUID  `json:"user_id"`
	UserName    string     `json:"user_name"`
	OutletID    uuid.UUID  `json:"outlet_id"`
	OutletName  string     `json:"outlet_name"`
	Label       string     `json:"label"`
	KeyID       string     `json:"key_id"`
	IPAllowlist []string   `json:"ip_allowlist"`
	RateLimit   int        `json:"rate_limit_min"`
	Active      bool       `json:"active"`
	ExpiresAt   *time.Time `json:"expires_at"`
	LastUsedAt  *time.Time `json:"last_used_at"`
	LastUsedIP  *string    `json:"last_used_ip"`
	Rotating    bool       `json:"rotating"`
	CreatedAt   time.Time  `json:"created_at"`
}

type CredentialInput struct {
	TenantID    uuid.UUID
	UserID      uuid.UUID
	OutletID    uuid.UUID
	Label       string
	IPAllowlist []string
	RateLimit   int
	ExpiresAt   *time.Time
}

// Created = hasil pembuatan/rotasi: secret hanya ada di sini, tidak pernah bisa dibaca lagi.
type Created struct {
	ID     uuid.UUID `json:"id"`
	KeyID  string    `json:"key_id"`
	Secret string    `json:"secret"`
}

// NormalizeIPs memvalidasi IP/CIDR dan menormalkannya (IP tunggal tetap IP; CIDR memakai alamat jaringan).
func NormalizeIPs(in []string) ([]string, error) {
	if len(in) == 0 || len(in) > maxIPEntries {
		return nil, fmt.Errorf("%w: 1–%d entri IP/CIDR", ErrInvalid, maxIPEntries)
	}
	seen := map[string]bool{}
	out := make([]string, 0, len(in))
	for _, raw := range in {
		s := strings.TrimSpace(raw)
		var norm string
		if strings.Contains(s, "/") {
			p, err := netip.ParsePrefix(s)
			if err != nil {
				return nil, fmt.Errorf("%w: %q", ErrInvalid, s)
			}
			norm = p.Masked().String()
		} else {
			a, err := netip.ParseAddr(s)
			if err != nil {
				return nil, fmt.Errorf("%w: %q", ErrInvalid, s)
			}
			norm = a.Unmap().String()
		}
		if !seen[norm] {
			seen[norm] = true
			out = append(out, norm)
		}
	}
	return out, nil
}

// IPAllowed: ip harus cocok dengan salah satu entri (IP persis atau di dalam CIDR). Daftar kosong = ditolak.
func IPAllowed(list []string, ip string) bool {
	addr, err := netip.ParseAddr(ip)
	if err != nil {
		return false
	}
	addr = addr.Unmap()
	for _, e := range list {
		if strings.Contains(e, "/") {
			if p, err := netip.ParsePrefix(e); err == nil && p.Contains(addr) {
				return true
			}
			continue
		}
		if a, err := netip.ParseAddr(e); err == nil && a.Unmap() == addr {
			return true
		}
	}
	return false
}

func randomKeyID() (string, error) {
	b := make([]byte, 12)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return "ak_" + strings.ToLower(base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(b)), nil
}

func randomSecret() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return "sk_" + base64.RawURLEncoding.EncodeToString(b), nil
}

func (s *Store) CreateCredential(ctx context.Context, admin uuid.UUID, in CredentialInput) (Created, error) {
	ips, err := NormalizeIPs(in.IPAllowlist)
	if err != nil {
		return Created{}, err
	}
	label := strings.TrimSpace(in.Label)
	if l := len([]rune(label)); l < 1 || l > 100 || in.RateLimit < 1 || in.RateLimit > 100000 {
		return Created{}, ErrInvalid
	}
	keyID, err := randomKeyID()
	if err != nil {
		return Created{}, err
	}
	secret, err := randomSecret()
	if err != nil {
		return Created{}, err
	}
	sealed, err := s.box.seal(secret, keyID)
	if err != nil {
		return Created{}, err
	}
	var id uuid.UUID
	err = s.pool.QueryRow(ctx, `SELECT platform_ext_cred_create($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)`,
		admin, in.TenantID, in.UserID, in.OutletID, label, keyID, sealed, ips, in.RateLimit, in.ExpiresAt).Scan(&id)
	if err != nil {
		var pe *pgconn.PgError
		if errors.As(err, &pe) && pe.Code == "P0002" {
			return Created{}, ErrInvalid
		}
		return Created{}, err
	}
	return Created{ID: id, KeyID: keyID, Secret: secret}, nil
}

func (s *Store) ListCredentials(ctx context.Context, admin uuid.UUID, tenant *uuid.UUID) ([]Credential, error) {
	rows, err := s.pool.Query(ctx, `SELECT * FROM platform_ext_cred_list($1,$2)`, admin, tenant)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Credential{}
	for rows.Next() {
		var c Credential
		if err := rows.Scan(&c.ID, &c.TenantID, &c.TenantName, &c.UserID, &c.UserName, &c.OutletID, &c.OutletName, &c.Label, &c.KeyID,
			&c.IPAllowlist, &c.RateLimit, &c.Active, &c.ExpiresAt, &c.LastUsedAt, &c.LastUsedIP, &c.Rotating, &c.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

type CredentialUpdate struct {
	Label       string
	IPAllowlist []string
	RateLimit   int
	Active      bool
	ExpiresAt   *time.Time
}

func (s *Store) UpdateCredential(ctx context.Context, admin, id uuid.UUID, in CredentialUpdate) error {
	ips, err := NormalizeIPs(in.IPAllowlist)
	if err != nil {
		return err
	}
	label := strings.TrimSpace(in.Label)
	if l := len([]rune(label)); l < 1 || l > 100 || in.RateLimit < 1 || in.RateLimit > 100000 {
		return ErrInvalid
	}
	var ok bool
	if err := s.pool.QueryRow(ctx, `SELECT platform_ext_cred_update($1,$2,$3,$4,$5,$6,$7)`,
		admin, id, label, ips, in.RateLimit, in.Active, in.ExpiresAt).Scan(&ok); err != nil {
		return err
	}
	if !ok {
		return ErrNotFound
	}
	return nil
}

// RotateCredential membuat secret baru. grace > 0: secret lama tetap sah selama itu (rotasi tanpa putus layanan);
// grace = 0: secret lama langsung mati (mis. bocor).
func (s *Store) RotateCredential(ctx context.Context, admin, id uuid.UUID, keyID string, grace time.Duration) (Created, error) {
	secret, err := randomSecret()
	if err != nil {
		return Created{}, err
	}
	sealed, err := s.box.seal(secret, keyID)
	if err != nil {
		return Created{}, err
	}
	var until *time.Time
	if grace > 0 {
		t := time.Now().Add(grace)
		until = &t
	}
	var ok bool
	if err := s.pool.QueryRow(ctx, `SELECT platform_ext_cred_rotate($1,$2,$3,$4)`, admin, id, sealed, until).Scan(&ok); err != nil {
		return Created{}, err
	}
	if !ok {
		return Created{}, ErrNotFound
	}
	return Created{ID: id, KeyID: keyID, Secret: secret}, nil
}

func (s *Store) DeleteCredential(ctx context.Context, admin, id uuid.UUID) error {
	var ok bool
	if err := s.pool.QueryRow(ctx, `SELECT platform_ext_cred_delete($1,$2)`, admin, id).Scan(&ok); err != nil {
		return err
	}
	if !ok {
		return ErrNotFound
	}
	return nil
}

// TenantActors = pilihan akun layanan & outlet tenant (untuk form kredensial) sebagai JSON siap kirim.
func (s *Store) TenantActors(ctx context.Context, admin, tenant uuid.UUID) (json.RawMessage, error) {
	var raw []byte
	if err := s.pool.QueryRow(ctx, `SELECT platform_ext_tenant_actors($1,$2)`, admin, tenant).Scan(&raw); err != nil {
		return nil, err
	}
	return raw, nil
}

// lookup = bahan verifikasi untuk gateway (sebelum autentikasi).
type lookup struct {
	ID, TenantID, UserID, OutletID uuid.UUID
	SecretEnc                      string
	PrevSecretEnc                  *string
	PrevExpiresAt                  *time.Time
	IPAllowlist                    []string
	RateLimit                      int
	Active                         bool
	ExpiresAt                      *time.Time
}

func (s *Store) lookup(ctx context.Context, keyID string) (*lookup, error) {
	var l lookup
	err := s.pool.QueryRow(ctx, `SELECT id, tenant_id, user_id, outlet_id, secret_enc, prev_secret_enc, prev_expires_at,
		ip_allowlist, rate_limit_min, active, expires_at FROM ext_cred_lookup($1)`, keyID).
		Scan(&l.ID, &l.TenantID, &l.UserID, &l.OutletID, &l.SecretEnc, &l.PrevSecretEnc, &l.PrevExpiresAt, &l.IPAllowlist, &l.RateLimit, &l.Active, &l.ExpiresAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	return &l, err
}

func (s *Store) touch(id uuid.UUID, ip string) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	_, _ = s.pool.Exec(ctx, `SELECT ext_cred_touch($1,$2)`, id, ip)
}
