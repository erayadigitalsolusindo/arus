package auth

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"

	gen "aciraba/internal/gen"
)

// SettingLoginLockout = key di tabel app_settings untuk kebijakan kunci login.
const SettingLoginLockout = "auth.login_lockout"

// LockoutPolicy dibaca dari app_settings sehingga angkanya bisa diubah tanpa deploy ulang.
type LockoutPolicy struct {
	// MaxFailures = gagal berturut-turut (per IP+email) sebelum akun dikunci.
	MaxFailures int `json:"max_failures"`
	// LockoutMinutes = durasi kunci ke-1, ke-2, ...; nilai terakhir dipakai berulang.
	LockoutMinutes []int `json:"lockout_minutes"`
	// ResetAfterHours = tingkat kunci dan hitungan gagal dilupakan setelah selama ini tanpa kegagalan.
	ResetAfterHours int `json:"reset_after_hours"`
	// EmailMaxFailuresPerHour = batas gagal per email dari semua IP (menahan serangan terdistribusi).
	EmailMaxFailuresPerHour int `json:"email_max_failures_per_hour"`
}

var DefaultLockoutPolicy = LockoutPolicy{MaxFailures: 5, LockoutMinutes: []int{1, 5, 15, 60}, ResetAfterHours: 24, EmailMaxFailuresPerHour: 50}

// Validate menolak angka yang akan merusak perilaku (nol, negatif, atau terlalu besar).
func (p LockoutPolicy) Validate() error {
	switch {
	case p.MaxFailures < 1 || p.MaxFailures > 100:
		return errors.New("max_failures harus 1..100")
	case len(p.LockoutMinutes) == 0 || len(p.LockoutMinutes) > 20:
		return errors.New("lockout_minutes harus berisi 1..20 nilai")
	case p.ResetAfterHours < 1 || p.ResetAfterHours > 24*30:
		return errors.New("reset_after_hours harus 1..720")
	case p.EmailMaxFailuresPerHour < p.MaxFailures || p.EmailMaxFailuresPerHour > 10000:
		return errors.New("email_max_failures_per_hour harus >= max_failures dan <= 10000")
	}
	for _, m := range p.LockoutMinutes {
		if m < 1 || m > 24*60 {
			return errors.New("setiap nilai lockout_minutes harus 1..1440")
		}
	}
	return nil
}

// PolicyLoader memuat kebijakan dari DB dengan cache singkat agar login tidak membebani DB.
// Bila baris hilang/rusak/DB gagal, dipakai nilai terakhir yang valid (atau bawaan).
type PolicyLoader struct {
	read func(ctx context.Context) ([]byte, error)
	log  *slog.Logger
	ttl  time.Duration
	now  func() time.Time

	mu  sync.Mutex
	cur LockoutPolicy
	at  time.Time
}

func NewPolicyLoader(pool *pgxpool.Pool, log *slog.Logger) *PolicyLoader {
	return newPolicyLoader(func(ctx context.Context) ([]byte, error) {
		return gen.New(pool).GetAppSetting(ctx, SettingLoginLockout)
	}, log, 30*time.Second)
}

func newPolicyLoader(read func(context.Context) ([]byte, error), log *slog.Logger, ttl time.Duration) *PolicyLoader {
	return &PolicyLoader{read: read, log: log, ttl: ttl, now: time.Now, cur: DefaultLockoutPolicy}
}

func (l *PolicyLoader) Get(ctx context.Context) LockoutPolicy {
	l.mu.Lock()
	defer l.mu.Unlock()
	if !l.at.IsZero() && l.now().Sub(l.at) < l.ttl {
		return l.cur
	}
	l.at = l.now() // juga saat gagal: jangan menghantam DB yang sedang bermasalah di setiap login
	raw, err := l.read(ctx)
	if errors.Is(err, pgx.ErrNoRows) {
		l.cur = DefaultLockoutPolicy
		return l.cur
	}
	if err != nil {
		l.log.Error("baca kebijakan kunci login gagal", "err", err)
		return l.cur
	}
	p := DefaultLockoutPolicy
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&p); err != nil {
		l.log.Error("kebijakan kunci login rusak, memakai nilai sebelumnya", "err", err)
		return l.cur
	}
	if err := p.Validate(); err != nil {
		l.log.Error("kebijakan kunci login tidak valid, memakai nilai sebelumnya", "err", err)
		return l.cur
	}
	l.cur = p
	return p
}

// Lockout menghitung login gagal di Redis dan mengunci kombinasi IP+email dengan durasi bertingkat.
// Kunci per IP+email (bukan email saja) agar orang lain tidak bisa mengunci akun korban dengan sengaja;
// batas per email dari semua IP (EmailMaxFailuresPerHour) menutup serangan dari banyak IP.
type Lockout struct {
	rdb    *redis.Client
	policy *PolicyLoader
}

func NewLockout(rdb *redis.Client, policy *PolicyLoader) *Lockout {
	return &Lockout{rdb: rdb, policy: policy}
}

func lockKeys(ip, emailHash string) (fails, level, until, email string) {
	id := ip + "|" + emailHash
	return "lk:f:" + id, "lk:l:" + id, "lk:u:" + id, "lk:e:" + emailHash
}

// Check mengembalikan sisa waktu kunci (> 0 = sedang dikunci). Error = Redis gagal (tolak: fail closed).
func (l *Lockout) Check(ctx context.Context, ip, emailHash string) (time.Duration, error) {
	_, _, until, email := lockKeys(ip, emailHash)
	pipe := l.rdb.Pipeline()
	untilTTL := pipe.PTTL(ctx, until)
	emailCount := pipe.Get(ctx, email)
	emailTTL := pipe.PTTL(ctx, email)
	if _, err := pipe.Exec(ctx); err != nil && !errors.Is(err, redis.Nil) {
		return 0, err
	}
	locked := max(untilTTL.Val(), 0)
	if n, err := emailCount.Int(); err == nil && n >= l.policy.Get(ctx).EmailMaxFailuresPerHour {
		locked = max(locked, emailTTL.Val())
	}
	return locked, nil
}

var failScript = redis.NewScript(`
local n = redis.call('INCR', KEYS[1])
redis.call('EXPIRE', KEYS[1], ARGV[2])
local e = redis.call('INCR', KEYS[4])
if e == 1 then redis.call('EXPIRE', KEYS[4], 3600) end
if n >= tonumber(ARGV[1]) then
  local lvl = redis.call('INCR', KEYS[2])
  redis.call('EXPIRE', KEYS[2], ARGV[2])
  redis.call('DEL', KEYS[1])
  local idx = math.min(lvl, #ARGV - 2)
  local secs = tonumber(ARGV[2 + idx]) * 60
  redis.call('SET', KEYS[3], '1', 'EX', secs)
  return {secs, 0}
end
return {0, tonumber(ARGV[1]) - n}
`)

// FailResult = akibat satu login gagal: LockedFor > 0 bila percobaan ini memicu kunci; selain itu AttemptsLeft = sisa
// percobaan gagal sebelum akun dikunci (dihitung per IP+email, sama untuk email yang tidak terdaftar).
type FailResult struct {
	LockedFor    time.Duration
	AttemptsLeft int
}

// RecordFailure mencatat satu login gagal.
func (l *Lockout) RecordFailure(ctx context.Context, ip, emailHash string) (FailResult, error) {
	p := l.policy.Get(ctx)
	fails, level, until, email := lockKeys(ip, emailHash)
	args := []any{p.MaxFailures, p.ResetAfterHours * 3600}
	for _, m := range p.LockoutMinutes {
		args = append(args, m)
	}
	res, err := failScript.Run(ctx, l.rdb, []string{fails, level, until, email}, args...).Int64Slice()
	if err != nil || len(res) != 2 {
		return FailResult{}, fmt.Errorf("catat gagal login: %w", err)
	}
	return FailResult{LockedFor: time.Duration(res[0]) * time.Second, AttemptsLeft: int(res[1])}, nil
}

// Reset menghapus hitungan dan tingkat kunci setelah login berhasil.
func (l *Lockout) Reset(ctx context.Context, ip, emailHash string) error {
	fails, level, _, _ := lockKeys(ip, emailHash)
	return l.rdb.Del(ctx, fails, level).Err()
}
