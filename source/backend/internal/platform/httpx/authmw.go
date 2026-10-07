package httpx

import (
	"context"
	"errors"
	"net/http"
	"slices"
	"strings"

	pauth "aciraba/internal/platform/auth"
)

type claimsKey struct{}

// ClaimsFrom mengambil klaim token akses yang dipasang RequireAuth.
func ClaimsFrom(ctx context.Context) (*pauth.Claims, bool) {
	c, ok := ctx.Value(claimsKey{}).(*pauth.Claims)
	return c, ok
}

// RequireAuth memverifikasi `Authorization: Bearer <jwt>`. Tenant/outlet/user hanya boleh diambil dari
// klaim ini (AGENTS.md §3.1). Token kedaluwarsa dibedakan (TOKEN_EXPIRED) agar klien tahu harus refresh.
func RequireAuth(tokens *pauth.TokenIssuer) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			h := r.Header.Get("Authorization")
			raw, ok := strings.CutPrefix(h, "Bearer ")
			if !ok || raw == "" {
				Error(w, http.StatusUnauthorized, "UNAUTHORIZED", "Belum masuk.")
				return
			}
			claims, err := tokens.Parse(raw)
			if errors.Is(err, pauth.ErrTokenExpired) {
				Error(w, http.StatusUnauthorized, "TOKEN_EXPIRED", "Token akses kedaluwarsa.")
				return
			}
			if err != nil {
				Error(w, http.StatusUnauthorized, "UNAUTHORIZED", "Token tidak valid.")
				return
			}
			next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), claimsKey{}, claims)))
		})
	}
}

// CSRFHeader = header kustom yang wajib ada di endpoint ber-cookie. Header non-sederhana memaksa preflight
// CORS, yang hanya lolos untuk origin yang diizinkan.
const CSRFHeader = "X-Requested-With"

// CSRFGuard melindungi endpoint yang memakai cookie (refresh, logout): Origin wajib ada dan termasuk
// `allowed`, serta header CSRFHeader wajib diisi. SameSite saja tidak cukup (mis. subdomain lain).
func CSRFGuard(allowed []string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !slices.Contains(allowed, r.Header.Get("Origin")) || r.Header.Get(CSRFHeader) == "" {
				Error(w, http.StatusForbidden, "FORBIDDEN", "Permintaan ditolak.")
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
