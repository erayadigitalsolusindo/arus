package dangerzone

import (
	"context"
	"errors"
	"os"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"aciraba/internal/authz"
	"aciraba/internal/platform/auth"
)

func TestClearGuards(t *testing.T) {
	appURL, adminURL := os.Getenv("TEST_DATABASE_URL"), os.Getenv("TEST_ADMIN_DATABASE_URL")
	if appURL == "" || adminURL == "" {
		t.Skip("TEST_DATABASE_URL/TEST_ADMIN_DATABASE_URL tidak di-set")
	}
	ctx := context.Background()
	app, err := pgxpool.New(ctx, appURL)
	if err != nil {
		t.Fatal(err)
	}
	defer app.Close()
	admin, err := pgxpool.New(ctx, adminURL)
	if err != nil {
		t.Fatal(err)
	}
	defer admin.Close()
	hash, err := auth.HashPassword("rahasia-uji-123")
	if err != nil {
		t.Fatal(err)
	}
	tenant, role, user := uuid.New(), uuid.New(), uuid.New()
	for _, s := range []struct {
		sql  string
		args []any
	}{
		{`INSERT INTO tenants (id, code, name) VALUES ($1, $2, 'UJI-DANGER')`, []any{tenant, "dz-" + tenant.String()[:8]}},
		{`INSERT INTO roles (id, tenant_id, name) VALUES ($2, $1, 'Owner')`, []any{tenant, role}},
		{`INSERT INTO users (id, tenant_id, role_id, email, name, password_hash) VALUES ($3, $1, $2, $5, 'Owner', $4)`, []any{tenant, role, user, hash, "dz-" + user.String() + "@example.test"}},
		{`INSERT INTO payment_methods (tenant_id, name, kind, is_system) VALUES ($1, 'Tunai', 'cash', true), ($1, 'Transfer', 'transfer', false)`, []any{tenant}},
	} {
		if _, err := admin.Exec(ctx, s.sql, s.args...); err != nil {
			t.Fatalf("%s: %v", s.sql, err)
		}
	}
	defer func() {
		for _, tbl := range []string{"audit_log", "payment_methods", "users", "roles"} {
			_, _ = admin.Exec(ctx, `DELETE FROM `+tbl+` WHERE tenant_id = $1`, tenant)
		}
		_, _ = admin.Exec(ctx, `DELETE FROM tenants WHERE id = $1`, tenant)
	}()

	svc := NewService(app, nil)
	owner := authz.Actor{TenantID: tenant, UserID: user, Name: "Owner", Perms: authz.Permissions{All: true}}
	staff := authz.Actor{TenantID: tenant, UserID: user, Name: "Kasir"}
	imp := owner
	imp.Impersonator = uuid.New()
	for name, c := range map[string]struct {
		a              authz.Actor
		scope, pw, cfm string
		want           error
	}{
		"bukan owner":      {staff, ScopeSales, "rahasia-uji-123", ConfirmWord, ErrOwnerOnly},
		"masuk-sebagai":    {imp, ScopeSales, "rahasia-uji-123", ConfirmWord, ErrImpersonating},
		"cakupan asing":    {owner, "users", "rahasia-uji-123", ConfirmWord, ErrScope},
		"konfirmasi salah": {owner, ScopeSales, "rahasia-uji-123", "hapus", ErrConfirm},
		"sandi salah":      {owner, ScopeSales, "salah", ConfirmWord, ErrPassword},
	} {
		if _, err := svc.Clear(ctx, c.a, c.scope, c.pw, c.cfm); !errors.Is(err, c.want) {
			t.Errorf("%s: err = %v, mau %v", name, err, c.want)
		}
	}
	counts, err := svc.Clear(ctx, owner, ScopePaymentMethods, "rahasia-uji-123", ConfirmWord)
	if err != nil {
		t.Fatal(err)
	}
	var n, audits int
	_ = admin.QueryRow(ctx, `SELECT count(*) FROM payment_methods WHERE tenant_id = $1`, tenant).Scan(&n)
	_ = admin.QueryRow(ctx, `SELECT count(*) FROM audit_log WHERE tenant_id = $1 AND action = 'danger_zone.clear'`, tenant).Scan(&audits)
	if n != 1 || audits != 1 || counts["payment_methods_deleted"] != float64(1) {
		t.Fatalf("metode=%d audit=%d counts=%v", n, audits, counts)
	}
}
