package platformadmin

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"aciraba/internal/authz"
)

func TestRoutePerm(t *testing.T) {
	cases := []struct {
		method, pattern, module, action string
		ok                              bool
	}{
		{"GET", "/platform/tenants", "tenants", "view", true},
		{"PATCH", "/platform/tenants/{id}", "tenants", "update", true},
		{"POST", "/platform/tenants/bulk", "tenants", "create", true},
		{"POST", "/platform/tenants/bulk-plan", "tenants", "update", true},
		{"POST", "/platform/tenants/bulk-profile", "tenants", "update", true},
		{"POST", "/platform/tenants/{id}/purge", "tenants", "delete", true},
		{"PUT", "/platform/tenants/{id}/deletion", "tenants", "delete", true},
		{"POST", "/platform/tenants/{id}/impersonate", "tenant_impersonate", "view", true},
		{"POST", "/platform/tenants/{id}/erapay/quickstart", "tenant_erapay", "update", true},
		{"GET", "/platform/tenants/{id}/erapay", "tenant_erapay", "view", true},
		{"POST", "/platform/admins/{id}/reset-2fa", "admins", "update", true},
		{"POST", "/platform/admins/{id}/delete", "admins", "delete", true},
		{"PUT", "/platform/admins/{id}/role", "admins", "update", true},
		{"GET", "/platform/admins/roles", "admins", "view", true},
		{"DELETE", "/platform/roles/{id}", "roles", "delete", true},
		{"POST", "/platform/ai/rules/test", "ai", "view", true},
		{"DELETE", "/platform/ai/provider", "ai", "update", true},
		{"POST", "/platform/backup/run", "backup", "update", true},
		{"POST", "/platform/ext/credentials/{id}/rotate", "api_access", "update", true},
		{"GET", "/platform/live/stream", "live", "view", true},
		{"GET", "/platform/tenant-types", "tenants", "view", true},
		{"GET", "/platform/tenantsx", "", "", false}, // batas segmen: bukan awalan /platform/tenants
		{"GET", "/platform/unknown", "", "", false},
	}
	for _, c := range cases {
		m, a, ok := routePerm(c.method, c.pattern)
		if m != c.module || a != c.action || ok != c.ok {
			t.Errorf("%s %s = (%q,%q,%v), mau (%q,%q,%v)", c.method, c.pattern, m, a, ok, c.module, c.action, c.ok)
		}
	}
}

// Setiap rute panel harus punya aturan izin, dan modul + aksinya harus ada di registri (kalau tidak, hanya Owner yang bisa
// mengaksesnya: rute baru yang lupa didaftarkan akan ketahuan di sini, bukan di produksi).
func TestEveryPanelRouteHasPermissionRule(t *testing.T) {
	h := NewHandler(HandlerDeps{Service: NewService(Deps{})})
	r := chi.NewRouter()
	h.Routes(r)
	open := []string{"/platform/setup", "/platform/auth/", "/platform/security"} // di luar requirePerm
	n := 0
	err := chi.Walk(r, func(method, route string, _ http.Handler, _ ...func(http.Handler) http.Handler) error {
		if !strings.HasPrefix(route, "/platform/") {
			return nil
		}
		for _, o := range open {
			if strings.HasPrefix(route, o) {
				return nil
			}
		}
		n++
		m, a, ok := routePerm(method, route)
		if !ok {
			t.Errorf("%s %s tanpa aturan izin", method, route)
			return nil
		}
		mod, found := moduleOf(m)
		if !found || !hasAction(mod, a) {
			t.Errorf("%s %s memakai %s:%s yang tidak ada di PlatformModules", method, route, m, a)
		}
		return nil
	})
	if err != nil || n < 50 {
		t.Fatalf("walk: err=%v rute=%d", err, n)
	}
}

func moduleOf(id string) (authz.Module, bool) {
	for _, m := range PlatformModules {
		if m.ID == id {
			return m, true
		}
	}
	return authz.Module{}, false
}

func hasAction(m authz.Module, a string) bool {
	for _, x := range m.Actions {
		if x == a {
			return true
		}
	}
	return false
}

func TestRequirePerm(t *testing.T) {
	perms := func(grants map[string][]string) authz.Permissions {
		p, err := authz.NormalizeIn(PlatformModules, grants)
		if err != nil {
			t.Fatal(err)
		}
		return p
	}
	var cur authz.Permissions
	r := chi.NewRouter()
	r.Group(func(r chi.Router) {
		r.Use(func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
				next.ServeHTTP(w, req.WithContext(context.WithValue(req.Context(), actorKey{}, Actor{ID: uuid.New(), Perms: cur})))
			})
		}, requirePerm)
		ok := func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) }
		r.Get("/platform/tenants", ok)
		r.Patch("/platform/tenants/{id}", ok)
		r.Get("/platform/admins", ok)
		r.Get("/platform/belum-didaftarkan", ok)
	})
	call := func(method, path string) int {
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, httptest.NewRequest(method, path, nil))
		return rec.Code
	}

	cur = perms(map[string][]string{"tenants": {"view"}})
	if c := call("GET", "/platform/tenants"); c != 204 {
		t.Errorf("view tenant = %d, mau 204", c)
	}
	if c := call("PATCH", "/platform/tenants/x"); c != 403 {
		t.Errorf("ubah tenant tanpa izin update = %d, mau 403", c)
	}
	if c := call("GET", "/platform/admins"); c != 403 {
		t.Errorf("admins tanpa izin = %d, mau 403", c)
	}
	cur = authz.Permissions{} // tanpa role
	if c := call("GET", "/platform/tenants"); c != 403 {
		t.Errorf("tanpa role = %d, mau 403", c)
	}
	cur = perms(map[string][]string{"tenants": {"view", "update"}})
	if c := call("PATCH", "/platform/tenants/x"); c != 204 {
		t.Errorf("update tenant = %d, mau 204", c)
	}
	if c := call("GET", "/platform/belum-didaftarkan"); c != 403 {
		t.Errorf("rute tanpa aturan untuk non-Owner = %d, mau 403", c)
	}
	cur = authz.Permissions{All: true}
	if c := call("GET", "/platform/belum-didaftarkan"); c != 204 {
		t.Errorf("Owner = %d, mau 204", c)
	}
}

func TestValidRole(t *testing.T) {
	if _, _, f := validRole("Support", map[string][]string{"*": {"view"}}); f == nil {
		t.Error("wildcard harus ditolak")
	}
	if _, _, f := validRole("Support", map[string][]string{"tenants": {"approve"}}); f == nil {
		t.Error("aksi yang tidak berlaku harus ditolak")
	}
	if _, _, f := validRole("  ", nil); f["name"] == "" {
		t.Error("nama kosong harus ditolak")
	}
	n, p, f := validRole(" Support ", map[string][]string{"tenants": {"update"}})
	if f != nil || n != "Support" || !p.Has("tenants", "view") {
		t.Errorf("view harus ikut diberikan: n=%q f=%v", n, f)
	}
	owner := authz.Permissions{All: true}
	if !owner.Covers(p) || p.Covers(owner) {
		t.Error("Covers: Owner mencakup semua, role biasa tidak mencakup Owner")
	}
}
