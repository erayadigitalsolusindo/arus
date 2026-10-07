package db

import (
	"context"
	"fmt"
	"os"
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Integrasi: TEST_DATABASE_URL = role aplikasi (aciraba_app), TEST_ADMIN_DATABASE_URL = pemilik skema.
// Membuktikan bahwa RLS menutup akses lintas tenant walaupun query aplikasinya "salah" (tanpa filter tenant).
type rlsEnv struct {
	app, admin *pgxpool.Pool
	a, b       uuid.UUID
}

func newRLSEnv(t *testing.T) *rlsEnv {
	t.Helper()
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

	e := &rlsEnv{app: app, admin: admin, a: uuid.New(), b: uuid.New()}
	for i, id := range []uuid.UUID{e.a, e.b} {
		role := uuid.New()
		suffix := fmt.Sprintf("%d-%d", os.Getpid(), i)
		for _, q := range []struct {
			sql  string
			args []any
		}{
			{`INSERT INTO tenants (id, code, name) VALUES ($1, $2, $3)`, []any{id, "rls-" + suffix, "UJI-RLS-" + suffix}},
			{`INSERT INTO roles (id, tenant_id, name) VALUES ($1, $2, 'Owner')`, []any{role, id}},
			{`INSERT INTO outlets (tenant_id, code, name) VALUES ($1, 'main', 'Pusat')`, []any{id}},
			{`INSERT INTO users (tenant_id, role_id, email, name, password_hash) VALUES ($1, $2, $3, 'Uji', 'x')`, []any{id, role, fmt.Sprintf("rls-%s@rls.test", suffix)}},
		} {
			if _, err := admin.Exec(ctx, q.sql, q.args...); err != nil {
				t.Fatalf("siapkan data: %v", err)
			}
		}
	}
	t.Cleanup(func() {
		for _, id := range []uuid.UUID{e.a, e.b} {
			for _, tbl := range []string{"users", "outlets", "roles"} {
				_, _ = admin.Exec(ctx, `DELETE FROM `+tbl+` WHERE tenant_id = $1`, id)
			}
			_, _ = admin.Exec(ctx, `DELETE FROM tenants WHERE id = $1`, id)
		}
	})
	return e
}

func countIn(ctx context.Context, tx pgx.Tx, table string) (int, error) {
	var n int
	err := tx.QueryRow(ctx, `SELECT count(*) FROM `+table).Scan(&n)
	return n, err
}

func TestRLSRoleIsNotPrivileged(t *testing.T) {
	e := newRLSEnv(t)
	var super, bypass bool
	var owner string
	err := e.app.QueryRow(context.Background(),
		`SELECT r.rolsuper, r.rolbypassrls, (SELECT tableowner FROM pg_tables WHERE tablename = 'users')
		   FROM pg_roles r WHERE r.rolname = current_user`).Scan(&super, &bypass, &owner)
	if err != nil {
		t.Fatal(err)
	}
	if super || bypass {
		t.Fatalf("role aplikasi superuser/BYPASSRLS: RLS tidak berlaku (super=%v bypass=%v)", super, bypass)
	}
	var cur string
	_ = e.app.QueryRow(context.Background(), `SELECT current_user`).Scan(&cur)
	if cur == owner {
		t.Fatalf("role aplikasi %q adalah pemilik tabel: RLS terlewati", cur)
	}
}

func TestRLSNoTenantSeesNothing(t *testing.T) {
	e := newRLSEnv(t)
	ctx := context.Background()
	// Di luar WithTenant (koneksi pool biasa): nol baris, bukan error.
	for _, tbl := range []string{"tenants", "outlets", "roles", "users"} {
		var n int
		if err := e.app.QueryRow(ctx, `SELECT count(*) FROM `+tbl).Scan(&n); err != nil {
			t.Fatalf("%s: %v", tbl, err)
		}
		if n != 0 {
			t.Errorf("%s tanpa tenant terlihat %d baris, want 0", tbl, n)
		}
	}
}

func TestRLSTenantIsolation(t *testing.T) {
	e := newRLSEnv(t)
	ctx := context.Background()

	err := WithTenant(ctx, e.app, e.a, func(tx pgx.Tx) error {
		for _, tbl := range []string{"outlets", "roles", "users"} {
			if n, err := countIn(ctx, tx, tbl); err != nil || n != 1 {
				return fmt.Errorf("%s: n=%d err=%v, want 1 (hanya tenant A)", tbl, n, err)
			}
		}
		if n, err := countIn(ctx, tx, "tenants"); err != nil || n != 1 {
			return fmt.Errorf("tenants: n=%d err=%v, want 1", n, err)
		}

		// Query tanpa filter tenant tetap tidak melihat tenant B; UPDATE/DELETE ke baris B mengenai 0 baris.
		tag, err := tx.Exec(ctx, `UPDATE users SET name = 'diretas' WHERE tenant_id = $1`, e.b)
		if err != nil || tag.RowsAffected() != 0 {
			return fmt.Errorf("UPDATE lintas tenant: rows=%d err=%v", tag.RowsAffected(), err)
		}
		tag, err = tx.Exec(ctx, `DELETE FROM outlets WHERE tenant_id = $1`, e.b)
		if err != nil || tag.RowsAffected() != 0 {
			return fmt.Errorf("DELETE lintas tenant: rows=%d err=%v", tag.RowsAffected(), err)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	// INSERT dengan tenant_id milik tenant lain ditolak oleh WITH CHECK.
	err = WithTenant(ctx, e.app, e.a, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `INSERT INTO outlets (tenant_id, code, name) VALUES ($1, 'curang', 'X')`, e.b)
		return err
	})
	if err == nil {
		t.Fatal("INSERT ke tenant lain seharusnya ditolak")
	}

	// Data tenant B utuh (diperiksa lewat admin).
	var name string
	var outlets int
	if err := e.admin.QueryRow(ctx, `SELECT name FROM users WHERE tenant_id = $1`, e.b).Scan(&name); err != nil || name != "Uji" {
		t.Fatalf("user tenant B berubah: %q %v", name, err)
	}
	if err := e.admin.QueryRow(ctx, `SELECT count(*) FROM outlets WHERE tenant_id = $1`, e.b).Scan(&outlets); err != nil || outlets != 1 {
		t.Fatalf("outlet tenant B = %d %v, want 1", outlets, err)
	}
}

func TestRLSTenantDoesNotLeakAcrossPooledConnections(t *testing.T) {
	e := newRLSEnv(t)
	ctx := context.Background()

	// Setelah transaksi tenant selesai, koneksi yang sama kembali ke pool tanpa tenant.
	if err := WithTenant(ctx, e.app, e.a, func(tx pgx.Tx) error { return nil }); err != nil {
		t.Fatal(err)
	}
	var n int
	if err := e.app.QueryRow(ctx, `SELECT count(*) FROM users`).Scan(&n); err != nil || n != 0 {
		t.Fatalf("setelah WithTenant, pool biasa melihat %d user (err=%v), want 0", n, err)
	}

	// Banyak goroutine bergantian A/B pada pool kecil: tiap transaksi hanya boleh melihat tenant-nya sendiri.
	var wg sync.WaitGroup
	errs := make(chan error, 200)
	for i := range 200 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			me, other := e.a, e.b
			if i%2 == 1 {
				me, other = e.b, e.a
			}
			errs <- WithTenant(ctx, e.app, me, func(tx pgx.Tx) error {
				var mine, theirs int
				if err := tx.QueryRow(ctx, `SELECT count(*) FROM users WHERE tenant_id = $1`, me).Scan(&mine); err != nil {
					return err
				}
				if err := tx.QueryRow(ctx, `SELECT count(*) FROM users WHERE tenant_id = $1`, other).Scan(&theirs); err != nil {
					return err
				}
				if mine != 1 || theirs != 0 {
					return fmt.Errorf("tenant %s melihat mine=%d theirs=%d", me, mine, theirs)
				}
				return nil
			})
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Error(err)
		}
	}
}

func TestRLSAuthLookupFunctionsAreNarrow(t *testing.T) {
	e := newRLSEnv(t)
	ctx := context.Background()
	// Tanpa tenant, fungsi SECURITY DEFINER hanya mengembalikan akun yang diminta (satu baris), bukan semuanya.
	rows, err := e.app.Query(ctx, `SELECT user_id FROM auth_account_by_email($1)`, fmt.Sprintf("rls-%d-0@rls.test", os.Getpid()))
	if err != nil {
		t.Fatal(err)
	}
	n := 0
	for rows.Next() {
		n++
	}
	rows.Close()
	if n != 1 {
		t.Fatalf("lookup email = %d baris, want 1", n)
	}
	// Peran aplikasi tidak boleh bisa membuat tenant dengan id selain tenant aktifnya.
	if _, err := e.app.Exec(ctx, `INSERT INTO tenants (id, code, name) VALUES ($1, 'tanpa-ctx', 'X')`, uuid.New()); err == nil {
		t.Fatal("INSERT tenants tanpa app.tenant_id seharusnya ditolak")
	}
}
