package auth

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/redis/go-redis/v9"
)

var quiet = slog.New(slog.NewTextHandler(io.Discard, nil))

func loaderOf(raw string, err error) *PolicyLoader {
	return newPolicyLoader(func(context.Context) ([]byte, error) { return []byte(raw), err }, quiet, time.Minute)
}

func TestPolicyLoader(t *testing.T) {
	ctx := context.Background()

	got := loaderOf(`{"max_failures":3,"lockout_minutes":[2,10],"reset_after_hours":12,"email_max_failures_per_hour":30}`, nil).Get(ctx)
	if got.MaxFailures != 3 || len(got.LockoutMinutes) != 2 || got.ResetAfterHours != 12 || got.EmailMaxFailuresPerHour != 30 {
		t.Fatalf("kebijakan tidak terbaca: %+v", got)
	}

	if got := loaderOf("", pgx.ErrNoRows).Get(ctx); got.MaxFailures != DefaultLockoutPolicy.MaxFailures {
		t.Fatalf("baris hilang harus memakai bawaan: %+v", got)
	}

	// Isi rusak/berbahaya tidak boleh melumpuhkan login: pertahankan nilai valid sebelumnya (di sini bawaan).
	for name, raw := range map[string]string{
		"json rusak":        `{`,
		"field asing":       `{"max_failurez":3}`,
		"max nol":           `{"max_failures":0,"lockout_minutes":[1],"reset_after_hours":1,"email_max_failures_per_hour":10}`,
		"durasi nol":        `{"max_failures":3,"lockout_minutes":[0],"reset_after_hours":1,"email_max_failures_per_hour":10}`,
		"durasi kosong":     `{"max_failures":3,"lockout_minutes":[],"reset_after_hours":1,"email_max_failures_per_hour":10}`,
		"batas email kecil": `{"max_failures":10,"lockout_minutes":[1],"reset_after_hours":1,"email_max_failures_per_hour":5}`,
	} {
		if got := loaderOf(raw, nil).Get(ctx); got.MaxFailures != DefaultLockoutPolicy.MaxFailures {
			t.Errorf("%s: harus ditolak, got %+v", name, got)
		}
	}

	// Cache: perubahan di DB baru terlihat setelah TTL.
	now := time.Now()
	raw := `{"max_failures":4,"lockout_minutes":[1],"reset_after_hours":1,"email_max_failures_per_hour":10}`
	l := newPolicyLoader(func(context.Context) ([]byte, error) { return []byte(raw), nil }, quiet, time.Minute)
	l.now = func() time.Time { return now }
	if l.Get(ctx).MaxFailures != 4 {
		t.Fatal("awal salah")
	}
	raw = `{"max_failures":9,"lockout_minutes":[1],"reset_after_hours":1,"email_max_failures_per_hour":10}`
	if l.Get(ctx).MaxFailures != 4 {
		t.Fatal("harus memakai cache")
	}
	now = now.Add(2 * time.Minute)
	if l.Get(ctx).MaxFailures != 9 {
		t.Fatal("setelah TTL harus memuat ulang")
	}

	// DB error: nilai terakhir dipertahankan.
	l = loaderOf("", errors.New("db mati"))
	if l.Get(ctx).MaxFailures != DefaultLockoutPolicy.MaxFailures {
		t.Fatal("DB error harus memakai nilai terakhir")
	}
}

func newTestLockout(t *testing.T, raw string) *Lockout {
	t.Helper()
	url := os.Getenv("TEST_REDIS_URL")
	if url == "" {
		t.Skip("TEST_REDIS_URL tidak di-set")
	}
	opt, err := redis.ParseURL(url)
	if err != nil {
		t.Fatal(err)
	}
	rdb := redis.NewClient(opt)
	t.Cleanup(func() { _ = rdb.Close() })
	return NewLockout(rdb, loaderOf(raw, nil))
}

func (l *Lockout) wipe(t *testing.T, ip, email string) {
	t.Helper()
	f, lv, u, e := lockKeys(ip, email)
	l.rdb.Del(context.Background(), f, lv, u, e)
}

func TestLockoutEscalationAndReset(t *testing.T) {
	lk := newTestLockout(t, `{"max_failures":3,"lockout_minutes":[1,5],"reset_after_hours":1,"email_max_failures_per_hour":100}`)
	ctx := context.Background()
	ip, email := "203.0.113.7", fmt.Sprintf("uji-kunci-%d", os.Getpid())
	lk.wipe(t, ip, email)
	t.Cleanup(func() { lk.wipe(t, ip, email) })

	fail := func() time.Duration {
		d, err := lk.RecordFailure(ctx, ip, email)
		if err != nil {
			t.Fatal(err)
		}
		return d
	}
	if d, _ := lk.Check(ctx, ip, email); d != 0 {
		t.Fatalf("awal tidak boleh terkunci: %v", d)
	}
	if fail() != 0 || fail() != 0 {
		t.Fatal("dua gagal pertama tidak boleh mengunci")
	}
	if d := fail(); d != time.Minute {
		t.Fatalf("kunci ke-1 = %v, want 1m", d)
	}
	if d, _ := lk.Check(ctx, ip, email); d <= 0 || d > time.Minute {
		t.Fatalf("Check saat terkunci = %v", d)
	}

	// Kunci lain (IP berbeda, email sama) tidak ikut terkunci: korban tidak bisa dikunci pihak lain.
	if d, _ := lk.Check(ctx, "198.51.100.9", email); d != 0 {
		t.Fatalf("IP lain terkunci: %v", d)
	}

	// Tingkat naik: kunci ke-2 = 5m, ke-3 memakai nilai terakhir (5m) berulang.
	_, _, until, _ := lockKeys(ip, email)
	lk.rdb.Del(ctx, until)
	fail()
	fail()
	if d := fail(); d != 5*time.Minute {
		t.Fatalf("kunci ke-2 = %v, want 5m", d)
	}
	lk.rdb.Del(ctx, until)
	fail()
	fail()
	if d := fail(); d != 5*time.Minute {
		t.Fatalf("kunci ke-3 = %v, want 5m (nilai terakhir diulang)", d)
	}

	// Login berhasil menghapus hitungan dan tingkat.
	lk.rdb.Del(ctx, until)
	fail()
	if err := lk.Reset(ctx, ip, email); err != nil {
		t.Fatal(err)
	}
	fail()
	fail()
	if d := fail(); d != time.Minute {
		t.Fatalf("setelah reset kunci pertama harus 1m lagi, got %v", d)
	}
}

func TestLockoutEmailCeilingAcrossIPs(t *testing.T) {
	lk := newTestLockout(t, `{"max_failures":10,"lockout_minutes":[1],"reset_after_hours":1,"email_max_failures_per_hour":10}`)
	ctx := context.Background()
	email := fmt.Sprintf("uji-terdistribusi-%d", os.Getpid())
	ips := make([]string, 10)
	for i := range ips {
		ips[i] = fmt.Sprintf("192.0.2.%d", i+1)
		lk.wipe(t, ips[i], email)
	}
	lk.wipe(t, "192.0.2.200", email)
	t.Cleanup(func() {
		for _, ip := range append(ips, "192.0.2.200") {
			lk.wipe(t, ip, email)
		}
	})

	for _, ip := range ips { // 10 IP berbeda, masing-masing hanya 1 gagal
		if _, err := lk.RecordFailure(ctx, ip, email); err != nil {
			t.Fatal(err)
		}
	}
	if d, _ := lk.Check(ctx, "192.0.2.200", email); d <= 0 {
		t.Fatal("batas per email dari banyak IP harus mengunci IP baru juga")
	}
}
