package auth

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
)

func rlLoaderOf(raw string, err error) *RateLimitLoader {
	return newRateLimitLoader(func(context.Context) ([]byte, error) { return []byte(raw), err }, quiet, time.Minute)
}

func TestRateLimitLoader(t *testing.T) {
	ctx := context.Background()

	got := rlLoaderOf(`{"forgot_per_ip":500,"forgot_per_email":7}`, nil).Get(ctx)
	if got.ForgotPerIP != 500 || got.ForgotPerEmail != 7 || got.RegisterPerIP != DefaultRateLimitPolicy.RegisterPerIP {
		t.Fatalf("nilai sebagian harus melengkapi bawaan: %+v", got)
	}
	if got := rlLoaderOf("", pgx.ErrNoRows).Get(ctx); got != DefaultRateLimitPolicy {
		t.Fatalf("baris hilang harus bawaan: %+v", got)
	}
	for _, raw := range []string{`{"forgot_per_ip":0}`, `{"forgot_per_ip":-1}`, `{"forgot_per_ip":100001}`, `{"typo":1}`, `bukan json`} {
		if got := rlLoaderOf(raw, nil).Get(ctx); got != DefaultRateLimitPolicy {
			t.Fatalf("%s harus ditolak: %+v", raw, got)
		}
	}
	if got := rlLoaderOf("", errors.New("db down")).Get(ctx); got != DefaultRateLimitPolicy {
		t.Fatalf("DB gagal harus bawaan: %+v", got)
	}
	var nilLoader *RateLimitLoader
	if nilLoader.Get(ctx) != DefaultRateLimitPolicy {
		t.Fatal("loader nil harus bawaan")
	}
}

func TestRateLimitLoaderKeepsLastValidAndRefreshes(t *testing.T) {
	ctx := context.Background()
	raw := `{"forgot_per_ip":50}`
	now := time.Now()
	l := newRateLimitLoader(func(context.Context) ([]byte, error) { return []byte(raw), nil }, quiet, time.Minute)
	l.now = func() time.Time { return now }
	if l.Get(ctx).ForgotPerIP != 50 {
		t.Fatal("nilai awal")
	}
	raw = `{"forgot_per_ip":60}`
	if l.Get(ctx).ForgotPerIP != 50 {
		t.Fatal("harus memakai cache dalam TTL")
	}
	now = now.Add(2 * time.Minute)
	if l.Get(ctx).ForgotPerIP != 60 {
		t.Fatal("harus memuat ulang setelah TTL")
	}
	raw = `{"forgot_per_ip":0}`
	now = now.Add(2 * time.Minute)
	if l.Get(ctx).ForgotPerIP != 60 {
		t.Fatal("nilai rusak harus mempertahankan nilai valid terakhir")
	}
}
