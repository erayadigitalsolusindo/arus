package auth

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sync"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"

	pauth "aciraba/internal/platform/auth"
)

func TestValidateRegister(t *testing.T) {
	ok := RegisterInput{BusinessName: " Toko  Maju ", OwnerName: "Budi", Email: "BUDI@Toko.id", Phone: "0812 3456 7890", OutletName: "Pusat", Password: "sandi-aman-123"}
	c, f := ValidateRegister(ok)
	if f != nil || c.BusinessName != "Toko Maju" || c.Email != "budi@toko.id" || c.Phone != "+6281234567890" {
		t.Fatalf("input valid ditolak/dinormalkan salah: %+v %v", c, f)
	}

	bad := RegisterInput{BusinessName: "<img src=x onerror=alert(1)>", OwnerName: "", Email: "bukan-email", Phone: "abc", OutletName: "Pusat\x00", Password: "pendek"}
	_, f = ValidateRegister(bad)
	for _, k := range []string{"business_name", "owner_name", "email", "phone", "outlet_name", "password"} {
		if f[k] == "" {
			t.Errorf("field %s seharusnya ditolak", k)
		}
	}
}

// Integrasi: butuh TEST_DATABASE_URL dan TEST_REDIS_URL (database dev boleh dipakai; data uji dibersihkan).
func newTestService(t *testing.T) (*Service, *pgxpool.Pool) {
	t.Helper()
	dbURL, redisURL := os.Getenv("TEST_DATABASE_URL"), os.Getenv("TEST_REDIS_URL")
	if dbURL == "" || redisURL == "" {
		t.Skip("TEST_DATABASE_URL/TEST_REDIS_URL tidak di-set")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dbURL)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	opt, err := redis.ParseURL(redisURL)
	if err != nil {
		t.Fatal(err)
	}
	rdb := redis.NewClient(opt)
	t.Cleanup(func() { _ = rdb.Close() })
	return NewService(pool, pauth.NewTokenIssuer("rahasia-uji-rahasia-uji-rahasia-uji-123"), pauth.NewSessions(rdb)), pool
}

func cleanup(t *testing.T, pool *pgxpool.Pool, emailLike string) {
	t.Cleanup(func() {
		ctx := context.Background()
		for _, q := range []string{
			`DELETE FROM users WHERE email LIKE $1`,
			`DELETE FROM roles WHERE tenant_id IN (SELECT id FROM tenants WHERE name LIKE 'UJI-%' AND NOT EXISTS (SELECT 1 FROM users u WHERE u.tenant_id = tenants.id))`,
			`DELETE FROM outlets WHERE tenant_id IN (SELECT id FROM tenants WHERE name LIKE 'UJI-%' AND NOT EXISTS (SELECT 1 FROM users u WHERE u.tenant_id = tenants.id))`,
			`DELETE FROM tenants WHERE name LIKE 'UJI-%' AND NOT EXISTS (SELECT 1 FROM users u WHERE u.tenant_id = tenants.id)`,
		} {
			args := []any{}
			if q[len(q)-2:] == "$1" {
				args = append(args, emailLike)
			}
			if _, err := pool.Exec(ctx, q, args...); err != nil {
				t.Logf("cleanup: %v", err)
			}
		}
	})
}

func count(t *testing.T, pool *pgxpool.Pool, q string, args ...any) int {
	t.Helper()
	var n int
	if err := pool.QueryRow(context.Background(), q, args...).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func input(name, email string) CleanRegister {
	return CleanRegister{BusinessName: "UJI-" + name, OwnerName: "Penguji", Email: email, Phone: "+6281234567890", OutletName: "Pusat", Password: "sandi-aman-123"}
}

func TestRegisterCreatesWorkspaceAndRejectsDuplicateEmail(t *testing.T) {
	svc, pool := newTestService(t)
	ctx := context.Background()
	email := fmt.Sprintf("uji-%d@register.test", os.Getpid())
	cleanup(t, pool, "%@register.test")

	sess, err := svc.Register(ctx, input("Satu", email))
	if err != nil {
		t.Fatal(err)
	}
	if sess.AccessToken == "" || sess.RefreshToken == "" || sess.Tenant.Code == "" {
		t.Fatalf("sesi tidak lengkap: %+v", sess)
	}
	if n := count(t, pool, `SELECT count(*) FROM users u JOIN roles r ON r.id = u.role_id AND r.name = 'Owner' WHERE u.email = $1 AND u.password_hash LIKE '$argon2id$%'`, email); n != 1 {
		t.Fatalf("owner user = %d, want 1", n)
	}
	if n := count(t, pool, `SELECT count(*) FROM outlets WHERE tenant_id = $1`, sess.Tenant.ID); n != 1 {
		t.Fatalf("outlets = %d, want 1", n)
	}

	// Email sama ditolak, dan tidak meninggalkan tenant yatim (atomik).
	dup := input("Dua", email)
	if _, err := svc.Register(ctx, dup); !errors.Is(err, ErrEmailTaken) {
		t.Fatalf("err = %v, want ErrEmailTaken", err)
	}
	if n := count(t, pool, `SELECT count(*) FROM tenants WHERE name = 'UJI-Dua'`); n != 0 {
		t.Fatalf("tenant yatim = %d, want 0 (transaksi harus rollback)", n)
	}
}

func TestRegisterConcurrentSameEmail(t *testing.T) {
	svc, pool := newTestService(t)
	email := fmt.Sprintf("balap-%d@register.test", os.Getpid())
	cleanup(t, pool, "%@register.test")

	var wg sync.WaitGroup
	results := make(chan error, 6)
	for i := range 6 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := svc.Register(context.Background(), input(fmt.Sprintf("Balap%d", i), email))
			results <- err
		}()
	}
	wg.Wait()
	close(results)
	var ok, taken int
	for err := range results {
		switch {
		case err == nil:
			ok++
		case errors.Is(err, ErrEmailTaken):
			taken++
		default:
			t.Errorf("error tak terduga: %v", err)
		}
	}
	if ok != 1 || taken != 5 {
		t.Fatalf("ok=%d taken=%d, want 1 dan 5", ok, taken)
	}
	if n := count(t, pool, `SELECT count(*) FROM tenants WHERE name LIKE 'UJI-Balap%'`); n != 1 {
		t.Fatalf("tenants = %d, want 1", n)
	}
}
