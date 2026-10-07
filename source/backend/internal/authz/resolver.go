package authz

import (
	"context"
	"errors"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	gen "aciraba/internal/gen"
	"aciraba/internal/platform/db"
)

// ErrInactive: pengguna tidak ada, dinonaktifkan, atau tenant-nya nonaktif.
var ErrInactive = errors.New("pengguna atau tenant tidak aktif")

// Access = hak akses pengguna terkini menurut DB (bukan menurut token).
type Access struct {
	Perms Permissions
	Name  string
	// ValidAfter: token akses yang diterbitkan sebelum waktu ini ditolak (reset password, penonaktifan).
	ValidAfter time.Time
	// Outlets = outlet aktif yang boleh diakses pengguna.
	Outlets map[uuid.UUID]bool
}

// Resolver menentukan hak akses pengguna dari DB, sehingga perubahan role, penugasan outlet, penggantian password,
// dan penonaktifan akun berlaku dalam <= ttl detik walaupun token akses (15 menit) masih hidup.
type Resolver struct {
	pool *pgxpool.Pool
	ttl  time.Duration
	now  func() time.Time

	mu    sync.Mutex
	cache map[string]cacheEntry
}

type cacheEntry struct {
	access Access
	at     time.Time
}

const maxCacheEntries = 10_000

func NewResolver(pool *pgxpool.Pool) *Resolver {
	return &Resolver{pool: pool, ttl: 10 * time.Second, now: time.Now, cache: map[string]cacheEntry{}}
}

func cacheKey(tenantID, userID uuid.UUID) string { return tenantID.String() + ":" + userID.String() }

// For mengembalikan hak akses terkini; ErrInactive bila akun tidak boleh bertindak.
func (r *Resolver) For(ctx context.Context, tenantID, userID uuid.UUID) (Access, error) {
	key := cacheKey(tenantID, userID)
	r.mu.Lock()
	if e, ok := r.cache[key]; ok && r.now().Sub(e.at) < r.ttl {
		r.mu.Unlock()
		return e.access, nil
	}
	r.mu.Unlock()

	var access Access
	err := db.WithTenant(ctx, r.pool, tenantID, func(tx pgx.Tx) error {
		q := gen.New(tx)
		row, err := q.AuthzGetUserAccess(ctx, gen.AuthzGetUserAccessParams{TenantID: tenantID, ID: userID})
		if err != nil {
			return err
		}
		if !row.UserActive || !row.TenantActive {
			return ErrInactive
		}
		perms := ParseStored(row.Permissions)
		// Administrator (izin `*`) mengakses semua outlet aktif tanpa penugasan; yang lain hanya outlet yang ditugaskan.
		ids, err := q.AuthzListAccessibleOutlets(ctx, gen.AuthzListAccessibleOutletsParams{TenantID: tenantID, AllOutlets: perms.All, UserID: userID})
		if err != nil {
			return err
		}
		access = Access{Perms: perms, Name: row.Name, ValidAfter: row.TokensValidAfter.Time, Outlets: make(map[uuid.UUID]bool, len(ids))}
		for _, id := range ids {
			access.Outlets[id] = true
		}
		return nil
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return Access{}, ErrInactive
	}
	if err != nil {
		return Access{}, err
	}

	r.mu.Lock()
	if len(r.cache) >= maxCacheEntries {
		clear(r.cache) // batas memori sederhana; isi akan terisi ulang dari DB
	}
	r.cache[key] = cacheEntry{access: access, at: r.now()}
	r.mu.Unlock()
	return access, nil
}

// InvalidateTenant membuang cache seluruh pengguna di satu tenant (dipanggil setelah role/pengguna/outlet berubah).
func (r *Resolver) InvalidateTenant(tenantID uuid.UUID) {
	prefix := tenantID.String() + ":"
	r.mu.Lock()
	defer r.mu.Unlock()
	for k := range r.cache {
		if strings.HasPrefix(k, prefix) {
			delete(r.cache, k)
		}
	}
}
