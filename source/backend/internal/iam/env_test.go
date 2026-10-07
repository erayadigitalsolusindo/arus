package iam

import (
	"context"
	"fmt"
	"os"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"

	"aciraba/internal/authz"
	pauth "aciraba/internal/platform/auth"
)

const goodPassword = "sandi-aman-123"

// Integrasi: TEST_DATABASE_URL = role aplikasi (RLS berlaku), TEST_ADMIN_DATABASE_URL = pemilik skema (siapkan/bersihkan
// data), TEST_REDIS_URL. Tanpa ketiganya test di-skip.
type env struct {
	svc      *Service
	res      *authz.Resolver
	sessions *pauth.Sessions
	admin    *pgxpool.Pool
	tenant   uuid.UUID
	outlet   uuid.UUID // outlet pertama tenant uji
	owner    authz.Actor
	ownerRol uuid.UUID
	suffix   string
}

func newEnv(t *testing.T) *env {
	t.Helper()
	appURL, adminURL, redisURL := os.Getenv("TEST_DATABASE_URL"), os.Getenv("TEST_ADMIN_DATABASE_URL"), os.Getenv("TEST_REDIS_URL")
	if appURL == "" || adminURL == "" || redisURL == "" {
		t.Skip("TEST_DATABASE_URL/TEST_ADMIN_DATABASE_URL/TEST_REDIS_URL tidak di-set")
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
	opt, err := redis.ParseURL(redisURL)
	if err != nil {
		t.Fatal(err)
	}
	rdb := redis.NewClient(opt)
	t.Cleanup(func() { _ = rdb.Close() })

	e := &env{admin: admin, res: authz.NewResolver(app), sessions: pauth.NewSessions(rdb), tenant: uuid.New(), outlet: uuid.New(), ownerRol: uuid.New()}
	e.svc = NewService(app, e.res, e.sessions)
	e.suffix = fmt.Sprintf("%d-%s", os.Getpid(), e.tenant.String()[:8])
	owner := uuid.New()
	for _, q := range []struct {
		sql  string
		args []any
	}{
		{`INSERT INTO tenants (id, code, name) VALUES ($1, $2, $3)`, []any{e.tenant, "iam-" + e.suffix, "UJI-IAM-" + e.suffix}},
		{`INSERT INTO outlets (id, tenant_id, code, name) VALUES ($1, $2, 'main', 'Pusat')`, []any{e.outlet, e.tenant}},
		{`INSERT INTO roles (id, tenant_id, name, permissions, is_system) VALUES ($1, $2, 'Owner', '{"*":true}', true)`, []any{e.ownerRol, e.tenant}},
		{`INSERT INTO users (id, tenant_id, role_id, email, name, password_hash) VALUES ($1, $2, $3, $4, 'Pemilik', 'x')`, []any{owner, e.tenant, e.ownerRol, "owner-" + e.suffix + "@iam.test"}},
	} {
		if _, err := admin.Exec(ctx, q.sql, q.args...); err != nil {
			t.Fatalf("siapkan data: %v", err)
		}
	}
	t.Cleanup(func() {
		for _, tbl := range []string{"audit_log", "user_outlets", "users", "roles", "outlets"} {
			_, _ = admin.Exec(ctx, `DELETE FROM `+tbl+` WHERE tenant_id = $1`, e.tenant)
		}
		_, _ = admin.Exec(ctx, `DELETE FROM tenants WHERE id = $1`, e.tenant)
	})
	e.owner = e.actor(owner, authz.Permissions{All: true})
	return e
}

// actor membangun pemanggil di outlet pertama dengan akses ke outlet itu.
func (e *env) actor(userID uuid.UUID, perms authz.Permissions) authz.Actor {
	return authz.Actor{TenantID: e.tenant, UserID: userID, OutletID: e.outlet, Name: "Penguji", Perms: perms, Outlets: map[uuid.UUID]bool{e.outlet: true}}
}

func (e *env) mail(name string) string { return fmt.Sprintf("%s-%s@iam.test", name, e.suffix) }

func (e *env) role(t *testing.T, actor authz.Actor, name string, perms map[string][]string) *Role {
	t.Helper()
	r, err := e.svc.CreateRole(context.Background(), actor, name, perms, false)
	if err != nil {
		t.Fatalf("buat role %s: %v", name, err)
	}
	return r
}

func (e *env) user(t *testing.T, actor authz.Actor, email string, roleID uuid.UUID) *User {
	t.Helper()
	u, err := e.svc.CreateUser(context.Background(), actor, CreateUserInput{
		Name: "Pegawai", Email: email, Password: goodPassword, RoleID: roleID, OutletIDs: []uuid.UUID{e.outlet},
	})
	if err != nil {
		t.Fatalf("buat user %s: %v", email, err)
	}
	return u
}

func (e *env) count(t *testing.T, q string, args ...any) int {
	t.Helper()
	var n int
	if err := e.admin.QueryRow(context.Background(), q, args...).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}
