package extapi

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
)

// secretBox mengenkripsi secret kredensial di DB (AES-256-GCM). HMAC butuh secret asli di server, jadi hash saja tidak cukup.
// Kunci diturunkan dari JWT_SECRET dengan label khusus modul ini; ciphertext diikat ke `aad` (key_id) sehingga salinan
// baris tidak bisa dipindah ke kredensial lain.
type secretBox struct{ aead cipher.AEAD }

func newSecretBox(secret string) (*secretBox, error) {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte("aciraba-extapi-secret-v1"))
	block, err := aes.NewCipher(mac.Sum(nil))
	if err != nil {
		return nil, err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	return &secretBox{aead: aead}, nil
}

func (b *secretBox) seal(plain, aad string) (string, error) {
	nonce := make([]byte, b.aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return "", err
	}
	return base64.RawStdEncoding.EncodeToString(b.aead.Seal(nonce, nonce, []byte(plain), []byte(aad))), nil
}

func (b *secretBox) open(sealed, aad string) (string, error) {
	raw, err := base64.RawStdEncoding.DecodeString(sealed)
	if err != nil {
		return "", err
	}
	if len(raw) < b.aead.NonceSize() {
		return "", errors.New("ciphertext terlalu pendek")
	}
	plain, err := b.aead.Open(nil, raw[:b.aead.NonceSize()], raw[b.aead.NonceSize():], []byte(aad))
	if err != nil {
		return "", err
	}
	return string(plain), nil
}
