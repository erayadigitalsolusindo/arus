// Package auth berisi primitif autentikasi: hash password, token akses, dan sesi refresh.
package auth

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"

	"golang.org/x/crypto/argon2"
)

// Parameter argon2id (rekomendasi OWASP: m=64MiB, t=3, p=2).
const (
	argonMemory  = 64 * 1024
	argonTime    = 3
	argonThreads = 2
	argonKeyLen  = 32
	argonSaltLen = 16
)

// HashPassword menghasilkan hash argon2id dalam format PHC.
func HashPassword(password string) (string, error) {
	salt := make([]byte, argonSaltLen)
	if _, err := rand.Read(salt); err != nil {
		return "", fmt.Errorf("salt: %w", err)
	}
	key := argon2.IDKey([]byte(password), salt, argonTime, argonMemory, argonThreads, argonKeyLen)
	return fmt.Sprintf("$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s", argon2.Version, argonMemory, argonTime, argonThreads,
		base64.RawStdEncoding.EncodeToString(salt), base64.RawStdEncoding.EncodeToString(key)), nil
}

// VerifyPassword membandingkan password dengan hash PHC dalam waktu konstan.
func VerifyPassword(password, encoded string) (bool, error) {
	p := strings.Split(encoded, "$")
	if len(p) != 6 || p[1] != "argon2id" {
		return false, errors.New("format hash tidak dikenal")
	}
	var v, m, t, th int
	if _, err := fmt.Sscanf(p[2], "v=%d", &v); err != nil || v != argon2.Version {
		return false, errors.New("versi argon2 tidak didukung")
	}
	if _, err := fmt.Sscanf(p[3], "m=%d,t=%d,p=%d", &m, &t, &th); err != nil {
		return false, errors.New("parameter hash tidak valid")
	}
	salt, err := base64.RawStdEncoding.DecodeString(p[4])
	if err != nil {
		return false, err
	}
	want, err := base64.RawStdEncoding.DecodeString(p[5])
	if err != nil {
		return false, err
	}
	got := argon2.IDKey([]byte(password), salt, uint32(t), uint32(m), uint8(th), uint32(len(want)))
	return subtle.ConstantTimeCompare(got, want) == 1, nil
}
