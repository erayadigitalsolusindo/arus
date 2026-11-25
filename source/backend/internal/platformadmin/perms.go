package platformadmin

import (
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"

	"aciraba/internal/authz"
	"aciraba/internal/platform/httpx"
)

// PlatformModules = registri izin Platform Admin (terpisah dari authz.Modules milik tenant). ID-nya sama dengan id menu
// di panel platform agar UI menyaring menu dari izin yang dikirim /platform/auth/me. Menambah modul = tambah baris di sini,
// aturan rute di routeRules, dan kamus i18n `platform.perm.module.<id>`; role lama otomatis tidak punya izinnya (tertutup).
var PlatformModules = []authz.Module{
	{ID: "live", Actions: []string{authz.ActView}},
	{ID: "tenants", Actions: []string{authz.ActView, authz.ActCreate, authz.ActUpdate, authz.ActDelete}}, // delete = jadwal hapus & purge
	{ID: "tenant_impersonate", Actions: []string{authz.ActView}},                                         // masuk sebagai tenant
	{ID: "tenant_erapay", Actions: []string{authz.ActView, authz.ActUpdate}},
	{ID: "reports_sales", Actions: []string{authz.ActView}},
	{ID: "reports_expiring", Actions: []string{authz.ActView}},
	{ID: "reports_locations", Actions: []string{authz.ActView}},
	{ID: "plans", Actions: []string{authz.ActView, authz.ActCreate, authz.ActUpdate, authz.ActDelete}},
	{ID: "storefront_templates", Actions: []string{authz.ActView, authz.ActCreate, authz.ActUpdate, authz.ActDelete}},
	{ID: "ai", Actions: []string{authz.ActView, authz.ActUpdate}},
	{ID: "domains", Actions: []string{authz.ActView, authz.ActCreate, authz.ActUpdate, authz.ActDelete}},
	{ID: "api_access", Actions: []string{authz.ActView, authz.ActCreate, authz.ActUpdate, authz.ActDelete}},
	{ID: "mobile_apk", Actions: []string{authz.ActView, authz.ActUpdate, authz.ActDelete}},
	{ID: "backup", Actions: []string{authz.ActView, authz.ActUpdate}},
	{ID: "languages", Actions: []string{authz.ActView, authz.ActCreate, authz.ActUpdate, authz.ActDelete}},
	{ID: "qa", Actions: []string{authz.ActView, authz.ActCreate, authz.ActUpdate, authz.ActDelete}},
	{ID: "audit", Actions: []string{authz.ActView}},
	{ID: "admins", Actions: []string{authz.ActView, authz.ActCreate, authz.ActUpdate, authz.ActDelete}},
	{ID: "roles", Actions: []string{authz.ActView, authz.ActCreate, authz.ActUpdate, authz.ActDelete}},
}

// routeRule memetakan awalan pola rute chi ke modul izin. Aksi default mengikuti metode HTTP (GET=view, POST=create,
// PUT/PATCH=update, DELETE=delete); `write` bila diisi menggantikannya untuk semua metode selain GET/HEAD.
type routeRule struct{ prefix, module, write string }

// routeRules: awalan terpanjang yang cocok menang. Rute di dalam grup terautentikasi yang tidak cocok dengan satu pun
// aturan DITOLAK untuk semua kecuali Owner (default tertutup), jadi rute baru tidak pernah terbuka tanpa sengaja.
var routeRules = []routeRule{
	{"/platform/live", "live", ""},
	{"/platform/tenants/bulk-plan", "tenants", authz.ActUpdate},
	{"/platform/tenants/bulk-profile", "tenants", authz.ActUpdate},
	{"/platform/tenants/bulk", "tenants", authz.ActCreate},
	{"/platform/tenants/{id}/impersonate", "tenant_impersonate", authz.ActView},
	{"/platform/tenants/{id}/erapay", "tenant_erapay", authz.ActUpdate},
	{"/platform/tenants/{id}/deletion", "tenants", authz.ActDelete},
	{"/platform/tenants/{id}/purge", "tenants", authz.ActDelete},
	{"/platform/tenants", "tenants", ""},
	{"/platform/tenant-types", "tenants", ""},
	{"/platform/tenant-locations", "tenants", ""},
	{"/platform/reports/sales", "reports_sales", ""},
	{"/platform/reports/expiring", "reports_expiring", ""},
	{"/platform/reports/locations", "reports_locations", ""},
	{"/platform/audit", "audit", ""},
	{"/platform/admins/{id}/reset-2fa", "admins", authz.ActUpdate},
	{"/platform/admins/{id}/delete", "admins", authz.ActDelete},
	{"/platform/admins", "admins", ""},
	{"/platform/roles", "roles", ""},
	{"/platform/plans", "plans", ""},
	{"/platform/storefront-templates/preview", "storefront_templates", authz.ActView},
	{"/platform/storefront-templates", "storefront_templates", ""},
	{"/platform/ai", "ai", authz.ActUpdate},
	{"/platform/domains/{id}/verify", "domains", authz.ActUpdate},
	{"/platform/domains", "domains", ""},
	{"/platform/ext/credentials/{id}/rotate", "api_access", authz.ActUpdate},
	{"/platform/ext", "api_access", ""},
	{"/platform/mobile", "mobile_apk", ""},
	{"/platform/backup", "backup", authz.ActUpdate},
	{"/platform/languages", "languages", ""},
	{"/platform/qa", "qa", ""},
}

// readOnlyPosts = POST yang sebenarnya hanya-baca (uji aturan, pratinjau) sehingga cukup izin "view".
var readOnlyPosts = map[string]bool{
	"/platform/ai/rules/test":                true,
	"/platform/storefront-templates/preview": true,
}

func methodAction(method string) string {
	switch method {
	case http.MethodGet, http.MethodHead:
		return authz.ActView
	case http.MethodPost:
		return authz.ActCreate
	case http.MethodPut, http.MethodPatch:
		return authz.ActUpdate
	case http.MethodDelete:
		return authz.ActDelete
	}
	return ""
}

// routePerm = (modul, aksi) yang dibutuhkan rute. ok=false bila tak ada aturan (hanya Owner yang boleh).
func routePerm(method, pattern string) (module, action string, ok bool) {
	best := -1
	for i, r := range routeRules {
		if (pattern == r.prefix || strings.HasPrefix(pattern, r.prefix+"/")) && (best < 0 || len(r.prefix) > len(routeRules[best].prefix)) {
			best = i
		}
	}
	if best < 0 {
		return "", "", false
	}
	r := routeRules[best]
	action = methodAction(method)
	if readOnlyPosts[pattern] {
		action = authz.ActView
	} else if r.write != "" && action != authz.ActView {
		action = r.write
	}
	if action == "" {
		return "", "", false
	}
	return r.module, action, true
}

// requirePerm menegakkan izin role atas setiap rute panel (dipasang setelah Authenticate + requireMFA).
func requirePerm(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		a, ok := ActorFrom(r.Context())
		if !ok {
			httpx.Error(w, http.StatusUnauthorized, "UNAUTHORIZED", "Belum masuk.")
			return
		}
		pattern := ""
		if rc := chi.RouteContext(r.Context()); rc != nil {
			pattern = rc.RoutePattern()
		}
		module, action, known := routePerm(r.Method, pattern)
		if !(known && a.Perms.Has(module, action)) && !a.Perms.All {
			httpx.Error(w, http.StatusForbidden, "FORBIDDEN", "Anda tidak punya izin untuk tindakan ini.")
			return
		}
		next.ServeHTTP(w, r)
	})
}
