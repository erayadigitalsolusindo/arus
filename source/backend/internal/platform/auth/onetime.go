package auth

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

// Tujuan token sekali pakai. Token satu tujuan tidak dapat dipakai untuk tujuan lain.
const (
	PurposeVerifyEmail   = "verify_email"
	PurposePasswordReset = "password_reset"
	PurposePlatformMFA   = "platform_mfa" // tantangan login Platform Admin setelah password benar (menunggu kode TOTP)
)

// Umur token.
const (
	VerifyEmailTTL   = 24 * time.Hour
	PasswordResetTTL = 30 * time.Minute
	PlatformMFATTL   = 5 * time.Minute
)

// ErrInvalidToken: token tidak dikenal, sudah dipakai, atau kedaluwarsa (sengaja tidak dibedakan).
var ErrInvalidToken = errors.New("token tidak valid atau kedaluwarsa")

// OneTime menerbitkan token acak (256 bit) sekali pakai dengan masa berlaku, disimpan di Redis hanya sebagai hash
// SHA-256. Satu (tujuan, subjek) hanya punya satu token aktif: menerbitkan yang baru mencabut yang lama.
type OneTime struct{ rdb *redis.Client }

func NewOneTime(rdb *redis.Client) *OneTime { return &OneTime{rdb: rdb} }

var issueScript = redis.NewScript(`
local prev = redis.call('GET', KEYS[2])
if prev then redis.call('DEL', 'ot:' .. ARGV[1] .. ':' .. prev) end
redis.call('SET', KEYS[1], ARGV[2], 'EX', ARGV[4])
redis.call('SET', KEYS[2], ARGV[3], 'EX', ARGV[4])
return 1
`)

// Issue mengembalikan token mentah (kirim ke pengguna; jangan disimpan/dicatat). subject = id pengguna.
func (o *OneTime) Issue(ctx context.Context, purpose, subject string, ttl time.Duration) (string, error) {
	tok, err := newToken()
	if err != nil {
		return "", err
	}
	h := hashOf(tok)
	err = issueScript.Run(ctx, o.rdb,
		[]string{"ot:" + purpose + ":" + h, "otu:" + purpose + ":" + subject},
		purpose, subject, h, int(ttl.Seconds()),
	).Err()
	if err != nil {
		return "", fmt.Errorf("terbitkan token: %w", err)
	}
	return tok, nil
}

// Consume memakai token (atomik, sekali pakai) dan mengembalikan subjeknya.
func (o *OneTime) Consume(ctx context.Context, purpose, token string) (string, error) {
	if len(token) < 20 || len(token) > 128 {
		return "", ErrInvalidToken
	}
	subject, err := o.rdb.GetDel(ctx, "ot:"+purpose+":"+hashOf(token)).Result()
	if errors.Is(err, redis.Nil) {
		return "", ErrInvalidToken
	}
	if err != nil {
		return "", fmt.Errorf("pakai token: %w", err)
	}
	_ = o.rdb.Del(ctx, "otu:"+purpose+":"+subject).Err()
	return subject, nil
}

// Peek membaca subjek token TANPA memakainya (validasi lebih dulu, mis. kekuatan password, sebelum token dibakar).
func (o *OneTime) Peek(ctx context.Context, purpose, token string) (string, error) {
	if len(token) < 20 || len(token) > 128 {
		return "", ErrInvalidToken
	}
	subject, err := o.rdb.Get(ctx, "ot:"+purpose+":"+hashOf(token)).Result()
	if errors.Is(err, redis.Nil) {
		return "", ErrInvalidToken
	}
	if err != nil {
		return "", fmt.Errorf("baca token: %w", err)
	}
	return subject, nil
}

// Revoke mencabut token aktif (jika ada) untuk (tujuan, subjek), mis. setelah password diganti.
func (o *OneTime) Revoke(ctx context.Context, purpose, subject string) error {
	h, err := o.rdb.GetDel(ctx, "otu:"+purpose+":"+subject).Result()
	if errors.Is(err, redis.Nil) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("cabut token: %w", err)
	}
	return o.rdb.Del(ctx, "ot:"+purpose+":"+h).Err()
}
