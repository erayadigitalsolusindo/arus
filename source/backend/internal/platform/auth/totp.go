package auth

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha1" //nolint:gosec // TOTP RFC 6238 default (HMAC-SHA1) agar kompatibel dengan semua aplikasi autentikator
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base32"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"
)

// TOTP (RFC 6238): HMAC-SHA1, 6 digit, periode 30 detik: kompatibel dengan Google Authenticator, Authy, 1Password, dll.
const (
	TOTPPeriod = 30
	TOTPDigits = 6
	// totpSkew = toleransi selisih jam: kode periode sebelumnya/sesudahnya masih diterima.
	totpSkew = 1
)

var b32 = base32.StdEncoding.WithPadding(base32.NoPadding)

// NewTOTPSecret membuat rahasia acak 160 bit (base32, tanpa padding) untuk didaftarkan di aplikasi autentikator.
func NewTOTPSecret() (string, error) {
	b := make([]byte, 20)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("rahasia totp: %w", err)
	}
	return b32.EncodeToString(b), nil
}

// TOTPURL = URI `otpauth://` (dikodekan ke QR di klien).
func TOTPURL(issuer, account, secret string) string {
	label := url.PathEscape(issuer + ":" + account)
	q := url.Values{"secret": {secret}, "issuer": {issuer}, "algorithm": {"SHA1"}, "digits": {"6"}, "period": {"30"}}
	return "otpauth://totp/" + label + "?" + q.Encode()
}

func totpAt(secret string, step int64) (string, error) {
	key, err := b32.DecodeString(strings.ToUpper(secret))
	if err != nil {
		return "", errors.New("rahasia totp rusak")
	}
	var msg [8]byte
	binary.BigEndian.PutUint64(msg[:], uint64(step))
	mac := hmac.New(sha1.New, key)
	mac.Write(msg[:])
	sum := mac.Sum(nil)
	off := sum[len(sum)-1] & 0x0f
	code := (binary.BigEndian.Uint32(sum[off:off+4]) & 0x7fffffff) % 1_000_000
	return fmt.Sprintf("%0*d", TOTPDigits, code), nil
}

// TOTPStep = nomor periode untuk waktu t.
func TOTPStep(t time.Time) int64 { return t.Unix() / TOTPPeriod }

// VerifyTOTP memeriksa kode terhadap periode sekarang ±skew dan mengembalikan nomor periode yang cocok (untuk mencegah
// pemakaian ulang kode yang sama: simpan periode terakhir yang dipakai dan tolak yang <= itu).
func VerifyTOTP(secret, code string, now time.Time) (int64, bool) {
	code = strings.ReplaceAll(strings.TrimSpace(code), " ", "")
	if len(code) != TOTPDigits {
		return 0, false
	}
	cur := TOTPStep(now)
	matched, ok := int64(0), false
	for s := cur - totpSkew; s <= cur+totpSkew; s++ { // tanpa early-exit: waktu konstan terhadap posisi kecocokan
		want, err := totpAt(secret, s)
		if err != nil {
			return 0, false
		}
		if subtle.ConstantTimeCompare([]byte(want), []byte(code)) == 1 {
			matched, ok = s, true
		}
	}
	return matched, ok
}

// NewRecoveryCodes membuat n kode pemulihan sekali pakai (10 karakter base32, tampil "ABCDE-FGHJK") beserta hash SHA-256
// untuk disimpan. Kode mentah hanya ditampilkan sekali.
func NewRecoveryCodes(n int) (codes, hashes []string, err error) {
	for range n {
		b := make([]byte, 7)
		if _, err := rand.Read(b); err != nil {
			return nil, nil, fmt.Errorf("kode pemulihan: %w", err)
		}
		raw := b32.EncodeToString(b)[:10]
		codes = append(codes, raw[:5]+"-"+raw[5:])
		hashes = append(hashes, RecoveryHash(raw))
	}
	return codes, hashes, nil
}

// NormalizeRecovery membuang pemisah/spasi dan menyeragamkan huruf besar; ok=false bila bentuknya bukan kode pemulihan.
func NormalizeRecovery(s string) (string, bool) {
	s = strings.ToUpper(strings.NewReplacer("-", "", " ", "").Replace(strings.TrimSpace(s)))
	if len(s) != 10 {
		return "", false
	}
	if _, err := b32.DecodeString(s); err != nil {
		return "", false
	}
	return s, true
}

func RecoveryHash(normalized string) string {
	h := sha256.Sum256([]byte("platform-recovery:" + normalized))
	return base64.RawStdEncoding.EncodeToString(h[:])
}

// TOTPBox mengenkripsi rahasia TOTP di DB (AES-256-GCM). Kunci diturunkan dari JWT_SECRET dengan label khusus.
type TOTPBox struct{ aead cipher.AEAD }

func NewTOTPBox(secret string) (*TOTPBox, error) {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte("aciraba-platform-totp-v1"))
	block, err := aes.NewCipher(mac.Sum(nil))
	if err != nil {
		return nil, err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	return &TOTPBox{aead: aead}, nil
}

// Seal mengikat ciphertext ke `aad` (id admin) agar tidak bisa dipindah ke akun lain.
func (b *TOTPBox) Seal(plain, aad string) (string, error) {
	nonce := make([]byte, b.aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return "", err
	}
	return base64.RawStdEncoding.EncodeToString(b.aead.Seal(nonce, nonce, []byte(plain), []byte(aad))), nil
}

func (b *TOTPBox) Open(sealed, aad string) (string, error) {
	raw, err := base64.RawStdEncoding.DecodeString(sealed)
	if err != nil || len(raw) < b.aead.NonceSize() {
		return "", errors.New("rahasia totp rusak")
	}
	n := b.aead.NonceSize()
	plain, err := b.aead.Open(nil, raw[:n], raw[n:], []byte(aad))
	if err != nil {
		return "", errors.New("rahasia totp tidak dapat dibuka")
	}
	return string(plain), nil
}

// TOTPCode menghasilkan kode untuk waktu t (dipakai test dan alat bantu; klien sebenarnya memakai aplikasi autentikator).
func TOTPCode(secret string, t time.Time) (string, error) { return totpAt(secret, TOTPStep(t)) }
