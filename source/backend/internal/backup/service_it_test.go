package backup

import (
	"context"
	"io"
	"log/slog"
	"os"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
)

func testService(t *testing.T) *Service {
	t.Helper()
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("TEST_DATABASE_URL tidak di-set")
	}
	pool, err := pgxpool.New(context.Background(), url)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	s, err := NewService(pool, nil, "test-secret", "", t.TempDir(), "https://arus.example", slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func TestConfigHiddenFromAppRoleButLoadable(t *testing.T) {
	s := testService(t)
	ctx := context.Background()
	if _, err := s.pool.Exec(ctx, `SELECT 1 FROM backup_config`); err == nil {
		t.Fatal("role aplikasi tidak boleh membaca backup_config langsung")
	}
	l, err := s.load(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if l.ready() {
		t.Fatal("konfigurasi baru tidak boleh dianggap siap")
	}
	if _, err := s.ListRuns(ctx, 5); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Start(ctx, "manual"); err != ErrNotReady {
		t.Fatalf("Start tanpa Drive/passphrase harus ErrNotReady, dapat %v", err)
	}
}

func TestParseConnAndTarGz(t *testing.T) {
	c, err := parseConn("postgres://aciraba:pw@db.local:6969/aciraba?sslmode=disable")
	if err != nil || c.host != "db.local" || c.port != "6969" || c.user != "aciraba" || c.pass != "pw" || c.name != "aciraba" || c.ssl != "disable" {
		t.Fatalf("parseConn: %+v %v", c, err)
	}
	if _, err := parseConn(""); err == nil {
		t.Fatal("URL kosong harus ditolak")
	}
	dir := t.TempDir()
	if err := os.WriteFile(dir+"/a.txt", []byte("halo"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := tarGz(io.Discard, dir); err != nil {
		t.Fatal(err)
	}
}
