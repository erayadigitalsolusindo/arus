package platformadmin

import (
	"context"
	"errors"
	"net/http"
	"slices"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"aciraba/internal/platform/httpx"
)

// Cakupan tenant: Actor.Scope nil = admin melihat semua tenant; non-nil = hanya tenant di daftar itu.
// Daftar tenant/jenis/lokasi dibatasi di DB (kolom platform_admins.tenant_scope); rute per-tenant dijaga scopeGuard;
// laporan per-tenant disaring di Go.

// ActionAdminScope = audit platform untuk perubahan cakupan tenant admin.
const ActionAdminScope = "platform.admin_scope"

var ErrScopeDenied = errors.New("tenant di luar cakupan admin")

// Allows = admin boleh mengakses tenant ini.
func (a Actor) Allows(tenant uuid.UUID) bool {
	return a.Scope == nil || slices.Contains(a.Scope, tenant)
}

// scopeBlocked = awalan rute lintas-tenant yang tidak bisa disaring per tenant; ditutup untuk admin berbatas
// (shortcut: ringkasan/stream live, audit global, AI, API eksternal, domain & backup ditolak, buka bila perlu disaring per tenant).
var scopeBlocked = []string{"/platform/live", "/platform/audit", "/platform/ai", "/platform/ext", "/platform/domains", "/platform/backup", "/platform/tenants/bulk"} // bulk-plan bukan awalan "bulk/", jadi tetap boleh (disaring per id)

// scopeGuard menolak admin berbatas yang menyentuh tenant di luar cakupan (id tenant dibaca dari segmen `tenants/<uuid>`
// di path, jadi satu pemeriksaan melindungi semua rute per-tenant, termasuk yang baru) atau rute lintas-tenant yang diblokir.
func scopeGuard(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		a, ok := ActorFrom(r.Context())
		if !ok || a.Scope == nil {
			next.ServeHTTP(w, r)
			return
		}
		pattern := ""
		if rc := chi.RouteContext(r.Context()); rc != nil {
			pattern = rc.RoutePattern()
		}
		for _, b := range scopeBlocked {
			if pattern == b || strings.HasPrefix(pattern, b+"/") {
				httpx.Error(w, http.StatusForbidden, "TENANT_SCOPE", "Akun Anda dibatasi ke tenant tertentu; fitur ini tidak tersedia.")
				return
			}
		}
		seg := strings.Split(r.URL.Path, "/")
		for i := 0; i+1 < len(seg); i++ {
			if seg[i] != "tenants" {
				continue
			}
			if id, err := uuid.Parse(seg[i+1]); err == nil && !a.Allows(id) {
				httpx.Error(w, http.StatusNotFound, "NOT_FOUND", "Tidak ditemukan.") // 404 agar keberadaan tenant lain tak bocor
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

// SetAdminScope mengatur tenant yang boleh dilihat admin target. mode: all | set | add | remove (lihat fungsi DB).
func (s *Service) SetAdminScope(ctx context.Context, a Actor, target uuid.UUID, mode string, ids []uuid.UUID) error {
	if a.Scope != nil {
		return ErrScopeDenied
	}
	if !slices.Contains([]string{"all", "set", "add", "remove"}, mode) || len(ids) > maxBulkPlan {
		return FieldErrors{"mode": "INVALID"}
	}
	var ok bool
	err := s.Pool.QueryRow(ctx, `SELECT platform_admin_set_scope($1, $2, $3, $4)`, a.ID, target, mode, ids).Scan(&ok)
	switch {
	case pgCode(err) == "42501":
		return ErrScopeDenied
	case err != nil:
		return err
	case !ok:
		return ErrNotFound
	}
	s.forget(target)
	s.record(ctx, a, ActionAdminScope, uuid.Nil, "", map[string]any{"target_id": target.String(), "mode": mode, "count": len(ids)})
	return nil
}

// PUT /platform/admins/{id}/tenants {mode: all|set|add|remove, tenant_ids: [...]}
func (h *Handler) SetAdminScope(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	var req struct {
		Mode      string      `json:"mode"`
		TenantIDs []uuid.UUID `json:"tenant_ids"`
	}
	if !httpx.DecodeJSON(w, r, &req) {
		return
	}
	var fe FieldErrors
	switch err := h.svc.SetAdminScope(r.Context(), actor(r), id, req.Mode, req.TenantIDs); {
	case errors.Is(err, ErrNotFound):
		httpx.Error(w, http.StatusNotFound, "NOT_FOUND", "Admin tidak ditemukan.")
	case errors.Is(err, ErrScopeDenied):
		httpx.Error(w, http.StatusForbidden, "TENANT_SCOPE", "Akun Anda dibatasi ke tenant tertentu; tidak dapat mengatur cakupan admin.")
	case errors.As(err, &fe):
		httpx.ValidationError(w, fe)
	case err != nil:
		h.internal(w, r, "atur cakupan tenant admin gagal", err)
	default:
		w.WriteHeader(http.StatusNoContent)
	}
}

// TenantLocations = lokasi tenant yang sudah dipakai (bahan filter), terbanyak dulu.
func (s *Service) TenantLocations(ctx context.Context, a Actor) ([]TenantLocationCount, error) {
	rows, err := s.Pool.Query(ctx, `SELECT location, n FROM platform_tenant_locations($1)`, a.ID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []TenantLocationCount{}
	for rows.Next() {
		var t TenantLocationCount
		if err := rows.Scan(&t.Location, &t.Count); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

type TenantLocationCount struct {
	Location string `json:"location"`
	Count    int64  `json:"count"`
}

func (h *Handler) TenantLocations(w http.ResponseWriter, r *http.Request) {
	list, err := h.svc.TenantLocations(r.Context(), actor(r))
	if err != nil {
		h.internal(w, r, "daftar lokasi tenant gagal", err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"items": list})
}
