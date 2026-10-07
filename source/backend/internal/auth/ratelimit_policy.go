package auth

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	gen "aciraba/internal/gen"
)

// SettingRateLimits = key di tabel app_settings untuk batas permintaan endpoint auth.
const SettingRateLimits = "auth.rate_limits"

// RateLimitPolicy dibaca dari app_settings sehingga angkanya bisa diubah tanpa deploy ulang.
// Semua batas berjendela satu jam. Batas per IP dihitung dari IP klien; di dev semua permintaan berasal dari
// satu IP sehingga angkanya bisa dinaikkan, sedangkan batas per email/pengguna sebaiknya tetap ketat.
type RateLimitPolicy struct {
	RegisterPerIP         int `json:"register_per_ip"`
	ForgotPerIP           int `json:"forgot_per_ip"`
	ForgotPerEmail        int `json:"forgot_per_email"`
	ResetPerIP            int `json:"reset_per_ip"`
	VerifyPerIP           int `json:"verify_per_ip"`
	ResendVerificationPer int `json:"resend_verification_per_user"`
}

var DefaultRateLimitPolicy = RateLimitPolicy{
	RegisterPerIP: 10, ForgotPerIP: 10, ForgotPerEmail: 3, ResetPerIP: 20, VerifyPerIP: 30, ResendVerificationPer: 3,
}

const maxRateLimit = 100000

// Validate menolak nol/negatif (akan memblokir endpoint selamanya) dan angka yang tak masuk akal.
func (p RateLimitPolicy) Validate() error {
	for name, v := range map[string]int{
		"register_per_ip": p.RegisterPerIP, "forgot_per_ip": p.ForgotPerIP, "forgot_per_email": p.ForgotPerEmail,
		"reset_per_ip": p.ResetPerIP, "verify_per_ip": p.VerifyPerIP, "resend_verification_per_user": p.ResendVerificationPer,
	} {
		if v < 1 || v > maxRateLimit {
			return errors.New(name + " harus 1..100000")
		}
	}
	return nil
}

// RateLimitLoader memuat kebijakan dari DB dengan cache singkat. Bila baris hilang/rusak/DB gagal,
// dipakai nilai terakhir yang valid (atau bawaan). Pemanggil nil memakai nilai bawaan.
type RateLimitLoader struct {
	read func(ctx context.Context) ([]byte, error)
	log  *slog.Logger
	ttl  time.Duration
	now  func() time.Time

	mu  sync.Mutex
	cur RateLimitPolicy
	at  time.Time
}

func NewRateLimitLoader(pool *pgxpool.Pool, log *slog.Logger) *RateLimitLoader {
	return newRateLimitLoader(func(ctx context.Context) ([]byte, error) {
		return gen.New(pool).GetAppSetting(ctx, SettingRateLimits)
	}, log, 15*time.Second)
}

func newRateLimitLoader(read func(context.Context) ([]byte, error), log *slog.Logger, ttl time.Duration) *RateLimitLoader {
	return &RateLimitLoader{read: read, log: log, ttl: ttl, now: time.Now, cur: DefaultRateLimitPolicy}
}

func (l *RateLimitLoader) Get(ctx context.Context) RateLimitPolicy {
	if l == nil {
		return DefaultRateLimitPolicy
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if !l.at.IsZero() && l.now().Sub(l.at) < l.ttl {
		return l.cur
	}
	l.at = l.now() // juga saat gagal: jangan menghantam DB yang sedang bermasalah di setiap request
	raw, err := l.read(ctx)
	if errors.Is(err, pgx.ErrNoRows) {
		l.cur = DefaultRateLimitPolicy
		return l.cur
	}
	if err != nil {
		l.log.Error("baca batas permintaan gagal", "err", err)
		return l.cur
	}
	p := DefaultRateLimitPolicy
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&p); err != nil {
		l.log.Error("batas permintaan rusak, memakai nilai sebelumnya", "err", err)
		return l.cur
	}
	if err := p.Validate(); err != nil {
		l.log.Error("batas permintaan tidak valid, memakai nilai sebelumnya", "err", err)
		return l.cur
	}
	l.cur = p
	return p
}
