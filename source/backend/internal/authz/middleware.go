package authz

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	gen "aciraba/internal/gen"
	"aciraba/internal/platform/db"
	"aciraba/internal/platform/httpx"
)

// Actor = pemanggil yang sudah terautentikasi, dengan hak akses terkini dari DB.
type Actor struct {
	TenantID uuid.UUID
	UserID   uuid.UUID
	OutletID uuid.UUID // outlet aktif sesi ini (klaim `oid`), sudah diverifikasi boleh diakses
	Name     string
	Perms    Permissions
	// Outlets = seluruh outlet aktif yang boleh diakses pengguna ini.
	Outlets map[uuid.UUID]bool
	// Impersonator != uuid.Nil: sesi "masuk sebagai" milik Platform Admin (UserID kosong, izin penuh, hanya-baca).
	Impersonator uuid.UUID
}

// PlatformAdmin = status Platform Admin terkini (untuk memverifikasi token "masuk sebagai").
type PlatformAdmin struct {
	Name       string
	ValidAfter time.Time
	MFA        bool // 2FA (TOTP) sudah aktif
}

// PlatformChecker dipasang di Resolver (WithPlatform) oleh modul platformadmin; authz tidak mengimpornya (hindari siklus).
// Mengembalikan ErrInactive bila admin tidak ada atau dinonaktifkan.
type PlatformChecker interface {
	Admin(ctx context.Context, id uuid.UUID) (PlatformAdmin, error)
}

// WithPlatform mengaktifkan penerimaan token "masuk sebagai". Tanpa ini token tersebut ditolak (401).
func (r *Resolver) WithPlatform(c PlatformChecker) *Resolver {
	r.platform = c
	return r
}

type actorKey struct{}

// WithActor memasang Actor ke context (dipakai Authenticate dan test).
func WithActor(ctx context.Context, a Actor) context.Context {
	return context.WithValue(ctx, actorKey{}, a)
}

// ActorFrom mengambil Actor yang dipasang Authenticate.
func ActorFrom(ctx context.Context) (Actor, bool) {
	a, ok := ctx.Value(actorKey{}).(Actor)
	return a, ok
}

// Authenticate dipasang SETELAH httpx.RequireAuth. Memuat hak akses terkini dari DB dan menolak:
//   - akun/tenant nonaktif (401);
//   - token akses yang diterbitkan sebelum `tokens_valid_after` (reset password, penonaktifan) (401). Dibandingkan pada
//     presisi detik karena `iat` JWT berpresisi detik; token yang terbit di detik yang sama dengan pencabutan lolos;
//   - outlet pada token yang tidak (lagi) boleh diakses pengguna (403 OUTLET_FORBIDDEN).
func (r *Resolver) Authenticate(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		claims, ok := httpx.ClaimsFrom(req.Context())
		if !ok {
			httpx.Error(w, http.StatusUnauthorized, "UNAUTHORIZED", "Belum masuk.")
			return
		}
		tid, err1 := uuid.Parse(claims.TenantID)
		uid, err2 := uuid.Parse(claims.Subject)
		oid, err3 := uuid.Parse(claims.OutletID)
		if err1 != nil || err2 != nil || err3 != nil {
			httpx.Error(w, http.StatusUnauthorized, "UNAUTHORIZED", "Token tidak valid.")
			return
		}
		if claims.Impersonator != "" {
			r.authenticateImpersonation(w, req, next, claims.Impersonator, claims.Subject, claims.IssuedAt, tid, oid)
			return
		}
		access, err := r.For(req.Context(), tid, uid)
		switch {
		case errors.Is(err, ErrInactive):
			httpx.Error(w, http.StatusUnauthorized, "UNAUTHORIZED", "Sesi tidak valid.")
			return
		case err != nil:
			httpx.Error(w, http.StatusInternalServerError, "INTERNAL", "Terjadi kesalahan pada server.")
			return
		}
		if claims.IssuedAt == nil || claims.IssuedAt.Time.Before(access.ValidAfter.Truncate(time.Second)) {
			httpx.Error(w, http.StatusUnauthorized, "UNAUTHORIZED", "Sesi dicabut. Silakan masuk kembali.")
			return
		}
		if !access.Outlets[oid] {
			httpx.Error(w, http.StatusForbidden, "OUTLET_FORBIDDEN", "Anda tidak memiliki akses ke outlet ini.")
			return
		}
		a := Actor{TenantID: tid, UserID: uid, OutletID: oid, Name: access.Name, Perms: access.Perms, Outlets: access.Outlets}
		next.ServeHTTP(w, req.WithContext(WithActor(req.Context(), a)))
	})
}

// authenticateImpersonation: token "masuk sebagai" Platform Admin. Hanya-baca (metode aman saja), izin penuh di satu
// tenant, semua outlet aktifnya. Admin harus masih aktif dan token tidak boleh terbit sebelum pencabutan.
func (r *Resolver) authenticateImpersonation(w http.ResponseWriter, req *http.Request, next http.Handler, imp, sub string, iat *jwt.NumericDate, tid, oid uuid.UUID) {
	adminID, err := uuid.Parse(imp)
	if err != nil || imp != sub || r.platform == nil {
		httpx.Error(w, http.StatusUnauthorized, "UNAUTHORIZED", "Token tidak valid.")
		return
	}
	switch req.Method {
	case http.MethodGet, http.MethodHead, http.MethodOptions:
	default:
		httpx.Error(w, http.StatusForbidden, "PLATFORM_READ_ONLY", "Mode Platform Admin hanya-baca.")
		return
	}
	admin, err := r.platform.Admin(req.Context(), adminID)
	switch {
	case errors.Is(err, ErrInactive):
		httpx.Error(w, http.StatusUnauthorized, "UNAUTHORIZED", "Sesi tidak valid.")
		return
	case err != nil:
		httpx.Error(w, http.StatusInternalServerError, "INTERNAL", "Terjadi kesalahan pada server.")
		return
	}
	if iat == nil || iat.Time.Before(admin.ValidAfter.Truncate(time.Second)) {
		httpx.Error(w, http.StatusUnauthorized, "UNAUTHORIZED", "Sesi dicabut. Silakan masuk kembali.")
		return
	}
	outlets := map[uuid.UUID]bool{}
	err = db.WithTenant(req.Context(), r.pool, tid, func(tx pgx.Tx) error {
		ids, err := gen.New(tx).AuthzListAccessibleOutlets(req.Context(), gen.AuthzListAccessibleOutletsParams{TenantID: tid, AllOutlets: true})
		for _, id := range ids {
			outlets[id] = true
		}
		return err
	})
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "INTERNAL", "Terjadi kesalahan pada server.")
		return
	}
	if !outlets[oid] {
		httpx.Error(w, http.StatusForbidden, "OUTLET_FORBIDDEN", "Outlet tidak ditemukan atau nonaktif.")
		return
	}
	a := Actor{TenantID: tid, OutletID: oid, Name: "Platform: " + admin.Name, Perms: Permissions{All: true}, Outlets: outlets, Impersonator: adminID}
	next.ServeHTTP(w, req.WithContext(WithActor(req.Context(), a)))
}

// Require menolak (403) bila Actor tidak punya izin `module.action`. Dipakai di endpoint bisnis:
//
//	r.With(httpx.RequireAuth(tokens), res.Authenticate, authz.Require("items", authz.ActCreate)).Post(...)
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
