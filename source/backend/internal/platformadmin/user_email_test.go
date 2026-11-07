package platformadmin

import (
	"context"
	"testing"

	"github.com/google/uuid"
)

func TestPlatformUbahEmailPengguna(t *testing.T) {
	e := newEnv(t, "")
	ctx := context.Background()
	adminID, adminEmail, adminPass := e.newAdmin(t, "Operator Email")
	tok := e.loginToken(t, adminEmail, adminPass)
	f := e.newTenant(t)

	role, user, other := uuid.New(), uuid.New(), uuid.New()
	sfx := f.ID.String()[:8]
	oldEmail, takenEmail := "lama-"+sfx+"@uji.test", "terpakai-"+sfx+"@uji.test"
	for _, q := range []struct {
		sql  string
		args []any
	}{
		{`INSERT INTO roles (id, tenant_id, name) VALUES ($1, $2, 'Kasir')`, []any{role, f.ID}},
		{`INSERT INTO users (id, tenant_id, role_id, email, name, password_hash, email_verified_at) VALUES ($1, $2, $3, $4, 'Kasir Uji', 'x', now())`, []any{user, f.ID, role, oldEmail}},
		{`INSERT INTO users (id, tenant_id, role_id, email, name, password_hash) VALUES ($1, $2, $3, $4, 'Lain', 'x')`, []any{other, f.ID, role, takenEmail}},
		{`INSERT INTO user_identities (provider, subject, tenant_id, user_id) VALUES ('google', $1, $2, $3)`, []any{"sub-" + sfx, f.ID, user}},
	} {
		if _, err := e.admin.Exec(ctx, q.sql, q.args...); err != nil {
			t.Fatal(err)
		}
	}
	t.Cleanup(func() {
		_, _ = e.admin.Exec(ctx, `DELETE FROM user_identities WHERE tenant_id = $1`, f.ID)
		_, _ = e.admin.Exec(ctx, `DELETE FROM users WHERE tenant_id = $1`, f.ID)
		_, _ = e.admin.Exec(ctx, `DELETE FROM roles WHERE tenant_id = $1`, f.ID)
	})

	path := "/platform/tenants/" + f.ID.String() + "/users/" + user.String() + "/email"
	for name, c := range map[string]struct {
		path, body string
		want       int
	}{
		"tanpa token":   {path, `{"email":"baru@uji.test"}`, 401},
		"format salah":  {path, `{"email":"bukan-email"}`, 422},
		"sama":          {path, `{"email":"` + oldEmail + `"}`, 422},
		"sudah dipakai": {path, `{"email":"` + takenEmail + `"}`, 409},
		"pengguna tak ada": {"/platform/tenants/" + f.ID.String() + "/users/" + uuid.NewString() + "/email",
			`{"email":"baru@uji.test"}`, 404},
	} {
		token := tok
		if name == "tanpa token" {
			token = ""
		}
		if rec := e.do("PATCH", c.path, token, c.body); rec.Code != c.want {
			t.Errorf("%s: %d, want %d (%s)", name, rec.Code, c.want, rec.Body)
		}
	}

	newEmail := "Baru-" + sfx + "@Uji.Test"
	if rec := e.do("PATCH", path, tok, `{"email":"`+newEmail+`"}`); rec.Code != 204 {
		t.Fatalf("ubah email: %d %s", rec.Code, rec.Body)
	}
	var (
		got      string
		verified bool
		idents   int
	)
	if err := e.admin.QueryRow(ctx, `SELECT email, email_verified_at IS NOT NULL FROM users WHERE id = $1`, user).Scan(&got, &verified); err != nil {
		t.Fatal(err)
	}
	if want := "baru-" + sfx + "@uji.test"; got != want {
		t.Errorf("email = %q, want %q (huruf kecil)", got, want)
	}
	if verified {
		t.Error("email baru seharusnya belum terverifikasi")
	}
	_ = e.admin.QueryRow(ctx, `SELECT count(*) FROM user_identities WHERE user_id = $1`, user).Scan(&idents)
	if idents != 0 {
		t.Errorf("tautan Google tersisa %d, want 0", idents)
	}

	// Tercatat di audit platform dan audit tenant.
	var n int
	_ = e.admin.QueryRow(ctx, `SELECT count(*) FROM platform_audit_log WHERE tenant_id = $1 AND admin_id = $2 AND action = $3`, f.ID, adminID, ActionUserEmail).Scan(&n)
	if n != 1 {
		t.Errorf("audit platform = %d, want 1", n)
	}
	_ = e.admin.QueryRow(ctx, `SELECT count(*) FROM audit_log WHERE tenant_id = $1 AND action = 'platform.user_email' AND entity_id = $2`, f.ID, user.String()).Scan(&n)
	if n != 1 {
		t.Errorf("audit tenant = %d, want 1", n)
	}
}
