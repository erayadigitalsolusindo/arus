package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

// Umur refresh token. "Tetap masuk" = panjang; selain itu sesi pendek.
const (
	RefreshTTLRemember = 30 * 24 * time.Hour
	RefreshTTLShort    = 12 * time.Hour
)

// Sessions menyimpan refresh token di Redis. Hanya hash SHA-256 token yang menjadi key, sehingga
// kebocoran isi Redis tidak membocorkan token yang masih berlaku.
type Sessions struct{ rdb *redis.Client }

func NewSessions(rdb *redis.Client) *Sessions { return &Sessions{rdb: rdb} }

func key(token string) string {
	h := sha256.Sum256([]byte(token))
	return "refresh:" + hex.EncodeToString(h[:])
}

// Create membuat refresh token acak (256 bit) yang mengarah ke userID.
func (s *Sessions) Create(ctx context.Context, userID string, ttl time.Duration) (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("token acak: %w", err)
	}
	tok := base64.RawURLEncoding.EncodeToString(b)
	if err := s.rdb.Set(ctx, key(tok), userID, ttl).Err(); err != nil {
		return "", fmt.Errorf("simpan sesi: %w", err)
	}
	return tok, nil
}
