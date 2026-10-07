package authz

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	pauth "aciraba/internal/platform/auth"
	"aciraba/internal/platform/httpx"
)

// Integrasi Authenticate: pencabutan token akses (tokens_valid_after), akun nonaktif, dan akses outlet.
func TestAuthenticateMiddleware(t *testing.T) {
	appURL, adminURL := os.Getenv("TEST_DATABASE_URL"), os.Getenv("TEST_ADMIN_DATABASE_URL")
	if appURL == "" || adminURL == "" {
		t.Skip("TEST_DATABASE_URL/TEST_ADMIN_DATABASE_URL tidak di-set")
	}
	ctx := context.Background()
	app, err := pgxpool.New(ctx, appURL)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(app.Close)
	admin, err := pgxpool.New(ctx, adminURL)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(admin.Close)

	tenant, roleID, userID, outlet1, outlet2 := uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New()
	sfx := tenant.String()[:8]
	for _, q := range []struct {
		sql  string
		args []any
	}{
		{`INSERT INTO tenants (id, code, name) VALUES ($1, $2, $3)`, []any{tenant, "az-" + sfx, "UJI-AZ-" + sfx}},
		{`INSERT INTO outlets (id, tenant_id, code, name) VALUES ($1, $2, 'a', 'A'), ($3, $2, 'b', 'B')`, []any{outlet1, tenant, outlet2}},
		{`INSERT INTO roles (id, tenant_id, name, permissions) VALUES ($1, $2, 'Kasir', '{"items":["view"]}')`, []any{roleID, tenant}},
		{`INSERT INTO users (id, tenant_id, role_id, email, name, password_hash) VALUES ($1, $2, $3, $4, 'K', 'x')`, []any{userID, tenant, roleID, "az-" + sfx + "@az.test"}},
		{`INSERT INTO user_outlets (tenant_id, user_id, outlet_id) VALUES ($1, $2, $3)`, []any{tenant, userID, outlet1}},
	} {
		if _, err := admin.Exec(ctx, q.sql, q.args...); err != nil {
			t.Fatalf("siapkan: %v", err)
		}
	}
	t.Cleanup(func() {
		for _, tbl := range []string{"user_outlets", "users", "roles", "outlets"} {
			_, _ = admin.Exec(ctx, `DELETE FROM `+tbl+` WHERE tenant_id = $1`, tenant)
		}
		_, _ = admin.Exec(ctx, `DELETE FROM tenants WHERE id = $1`, tenant)
	})

	issuer := pauth.NewTokenIssuer("rahasia-uji-rahasia-uji-rahasia-uji-123")
	res := NewResolver(app)
	var got Actor
	h := httpx.RequireAuth(issuer)(res.Authenticate(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got, _ = ActorFrom(r.Context())
	})))
	call := func(oid uuid.UUID, issued time.Time) int {
		tok, err := issuer.Issue(userID.String(), tenant.String(), oid.String(), "Kasir", issued)
		if err != nil {
			t.Fatal(err)
		}
		req := httptest.NewRequest("GET", "/", nil)
		req.Header.Set("Authorization", "Bearer "+tok)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		return rec.Code
	}
	setValidAfter := func(at time.Time) {
		t.Helper()
		if _, err := admin.Exec(ctx, `UPDATE users SET tokens_valid_after = $2 WHERE id = $1`, userID, at); err != nil {
			t.Fatal(err)
		}
		res.InvalidateTenant(tenant)
	}

	if code := call(outlet1, time.Now()); code != 200 || got.OutletID != outlet1 || !got.Perms.Has("items", "view") || got.Name != "K" {
		t.Fatalf("token sah: status %d, actor %+v", code, got)
	}
	if code := call(outlet2, time.Now()); code != 403 {
		t.Errorf("outlet yang bukan haknya: status %d, want 403", code)
	}

	// Pencabutan: token terbit sebelum tokens_valid_after ditolak; yang terbit sesudahnya diterima.
	revokedAt := time.Now().Add(-1 * time.Minute)
	setValidAfter(revokedAt)
	if code := call(outlet1, revokedAt.Add(-time.Hour)); code != 401 {
		t.Errorf("token sebelum pencabutan: status %d, want 401", code)
	}
	if code := call(outlet1, revokedAt.Add(time.Minute)); code != 200 {
		t.Errorf("token sesudah pencabutan: status %d, want 200", code)
	}

	// Akun dinonaktifkan: seketika ditolak walau token masih berlaku.
	if _, err := admin.Exec(ctx, `UPDATE users SET active = false WHERE id = $1`, userID); err != nil {
		t.Fatal(err)
	}
	res.InvalidateTenant(tenant)
	if code := call(outlet1, time.Now()); code != 401 {
		t.Errorf("akun nonaktif: status %d, want 401", code)
	}
}
