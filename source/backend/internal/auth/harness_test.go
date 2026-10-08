package auth

import (
	"context"
	"io"
	"log/slog"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"

	"aciraba/internal/authz"
	pauth "aciraba/internal/platform/auth"
	"aciraba/internal/platform/background"
	"aciraba/internal/platform/mailer"
)

// captureMailer menyimpan email yang "terkirim" agar test dapat membaca tautannya.
type captureMailer struct {
	mu   sync.Mutex
	msgs []mailer.Message
}

func (c *captureMailer) Send(_ context.Context, m mailer.Message) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.msgs = append(c.msgs, m)
	return nil
}

// lastToken mengembalikan token dari tautan pada email terakhir ke alamat `to` (fragmen #token=...).
func (c *captureMailer) lastToken(to string) string {
	c.mu.Lock()
	defer c.mu.Unlock()
	for i := len(c.msgs) - 1; i >= 0; i-- {
		if c.msgs[i].To == to {
			_, tok, _ := strings.Cut(c.msgs[i].Text, "#token=")
			tok, _, _ = strings.Cut(tok, "\n")
			return strings.TrimSpace(tok)
		}
	}
	return ""
}

func (c *captureMailer) count(to string) int {
	c.mu.Lock()
	defer c.mu.Unlock()
	n := 0
	for _, m := range c.msgs {
		if m.To == to {
			n++
		}
	}
	return n
}

// harness = layanan auth sungguhan di atas DB/Redis uji. Integrasi: TEST_DATABASE_URL (role aplikasi, RLS berlaku),
// TEST_ADMIN_DATABASE_URL (pemilik skema, untuk memeriksa/membersihkan data lintas tenant), TEST_REDIS_URL.
type harness struct {
	svc   *Service
	admin *pgxpool.Pool
	mail  *captureMailer
	jobs  *background.Runner
	rdb   *redis.Client
}

func newHarness(t *testing.T) *harness {
	t.Helper()
	dbURL, adminURL, redisURL := os.Getenv("TEST_DATABASE_URL"), os.Getenv("TEST_ADMIN_DATABASE_URL"), os.Getenv("TEST_REDIS_URL")
	if dbURL == "" || adminURL == "" || redisURL == "" {
		t.Skip("TEST_DATABASE_URL/TEST_ADMIN_DATABASE_URL/TEST_REDIS_URL tidak di-set")
	}
	ctx := context.Background()
	appPool, err := pgxpool.New(ctx, dbURL)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(appPool.Close)
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

	h := &harness{admin: admin, rdb: rdb, mail: &captureMailer{}, jobs: background.New(slog.New(slog.NewTextHandler(io.Discard, nil)), 8, 10*time.Second)}
	h.svc = NewService(Deps{
		Pool: appPool, Tokens: pauth.NewTokenIssuer("rahasia-uji-rahasia-uji-rahasia-uji-123"), Sessions: pauth.NewSessions(rdb),
		OneTime: pauth.NewOneTime(rdb), Perms: authz.NewResolver(appPool), Mailer: h.mail, Jobs: h.jobs, BaseURL: "https://app.test",
	})
	return h
}

// newTestService dipertahankan untuk test lama: layanan + pool admin.
func newTestService(t *testing.T) (*Service, *pgxpool.Pool) {
	t.Helper()
	h := newHarness(t)
	return h.svc, h.admin
}

// cleanup menghapus semua tenant uji yang punya pengguna dengan email cocok pola (urutan menghormati FK).
func cleanup(t *testing.T, pool *pgxpool.Pool, emailLike string) {
	t.Cleanup(func() {
		ctx := context.Background()
		rows, err := pool.Query(ctx, `SELECT DISTINCT tenant_id FROM users WHERE email LIKE $1`, emailLike)
		if err != nil {
			t.Logf("cleanup: %v", err)
			return
		}
		var ids []string
		for rows.Next() {
			var id string
			if rows.Scan(&id) == nil {
				ids = append(ids, id)
			}
		}
		rows.Close()
		for _, id := range ids {
			for _, tbl := range []string{"audit_log", "user_outlets", "payment_methods", "users", "roles", "outlets"} {
				if _, err := pool.Exec(ctx, `DELETE FROM `+tbl+` WHERE tenant_id = $1`, id); err != nil {
					t.Logf("cleanup %s: %v", tbl, err)
				}
			}
			if _, err := pool.Exec(ctx, `DELETE FROM tenants WHERE id = $1`, id); err != nil {
				t.Logf("cleanup tenants: %v", err)
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
