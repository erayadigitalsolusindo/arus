package iam

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	gen "aciraba/internal/gen"
	"aciraba/internal/platform/db"
	"aciraba/internal/platform/httpx"
)

// ErrInactive: pengguna tidak ada, dinonaktifkan, atau tenant-nya nonaktif.
var ErrInactive = errors.New("pengguna atau tenant tidak aktif")

// Resolver menentukan izin efektif pengguna dari DB (bukan dari token), sehingga perubahan role, penggantian role,
// dan penonaktifan akun berlaku dalam <= ttl detik walaupun token akses (15 menit) masih hidup.
type Resolver struct {
	pool *pgxpool.Pool
	ttl  time.Duration
	now  func() time.Time

	mu    sync.Mutex
	cache map[string]cacheEntry
}

type cacheEntry struct {
	perms Permissions
	at    time.Time
}

const maxCacheEntries = 10_000

func NewResolver(pool *pgxpool.Pool) *Resolver {
	return &Resolver{pool: pool, ttl: 10 * time.Second, now: time.Now, cache: map[string]cacheEntry{}}
}

func cacheKey(tenantID, userID uuid.UUID) string { return tenantID.String() + ":" + userID.String() }

// For mengembalikan izin efektif; ErrInactive bila akun tidak boleh bertindak.
func (r *Resolver) For(ctx context.Context, tenantID, userID uuid.UUID) (Permissions, error) {
	key := cacheKey(tenantID, userID)
	r.mu.Lock()
	if e, ok := r.cache[key]; ok && r.now().Sub(e.at) < r.ttl {
		r.mu.Unlock()
		return e.perms, nil
	}
	r.mu.Unlock()

	var row gen.IamGetUserPermissionsRow
	err := db.WithTenant(ctx, r.pool, tenantID, func(tx pgx.Tx) error {
		var qerr error
		row, qerr = gen.New(tx).IamGetUserPermissions(ctx, gen.IamGetUserPermissionsParams{TenantID: tenantID, ID: userID})
		return qerr
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return Permissions{}, ErrInactive
	}
	if err != nil {
		return Permissions{}, err
	}
	if !row.UserActive || !row.TenantActive {
		return Permissions{}, ErrInactive
	}
	perms := ParseStored(row.Permissions)

	r.mu.Lock()
	if len(r.cache) >= maxCacheEntries {
		clear(r.cache) // batas memori sederhana; isi akan terisi ulang dari DB
	}
	r.cache[key] = cacheEntry{perms: perms, at: r.now()}
	r.mu.Unlock()
	return perms, nil
}

// InvalidateTenant membuang cache seluruh pengguna di satu tenant (dipanggil setelah role/pengguna berubah).
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

// Actor = pemanggil yang sudah terautentikasi, dengan izin efektif terkini.
type Actor struct {
	TenantID uuid.UUID
	UserID   uuid.UUID
	Perms    Permissions
}

type actorKey struct{}

// ActorFrom mengambil Actor yang dipasang Authenticate.
func ActorFrom(ctx context.Context) (Actor, bool) {
	a, ok := ctx.Value(actorKey{}).(Actor)
	return a, ok
}

// Authenticate dipasang SETELAH httpx.RequireAuth: memuat izin efektif dari DB dan menolak akun yang sudah
// dinonaktifkan (401) walaupun token akses-nya belum kedaluwarsa.
func (r *Resolver) Authenticate(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		claims, ok := httpx.ClaimsFrom(req.Context())
		if !ok {
			httpx.Error(w, http.StatusUnauthorized, "UNAUTHORIZED", "Belum masuk.")
			return
		}
		tid, err1 := uuid.Parse(claims.TenantID)
		uid, err2 := uuid.Parse(claims.Subject)
		if err1 != nil || err2 != nil {
			httpx.Error(w, http.StatusUnauthorized, "UNAUTHORIZED", "Token tidak valid.")
			return
		}
		perms, err := r.For(req.Context(), tid, uid)
		switch {
		case errors.Is(err, ErrInactive):
			httpx.Error(w, http.StatusUnauthorized, "UNAUTHORIZED", "Sesi tidak valid.")
			return
		case err != nil:
			httpx.Error(w, http.StatusInternalServerError, "INTERNAL", "Terjadi kesalahan pada server.")
			return
		}
		next.ServeHTTP(w, req.WithContext(context.WithValue(req.Context(), actorKey{}, Actor{TenantID: tid, UserID: uid, Perms: perms})))
	})
}

// Require menolak (403) bila Actor tidak punya izin `module.action`. Dipakai di endpoint bisnis:
//
//	r.With(httpx.RequireAuth(tokens), res.Authenticate, iam.Require("items", iam.ActCreate)).Post(...)
func Require(module, action string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			a, ok := ActorFrom(req.Context())
			if !ok || !a.Perms.Has(module, action) {
				httpx.Error(w, http.StatusForbidden, "FORBIDDEN", "Anda tidak memiliki izin untuk tindakan ini.")
				return
			}
			next.ServeHTTP(w, req)
		})
	}
}
