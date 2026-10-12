package erapay

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
)

// secretBox mengenkripsi API key EraPay tenant di DB (AES-256-GCM). Kunci diturunkan dari JWT_SECRET dengan label khusus modul
// ini; ciphertext diikat ke `aad` (id tenant) sehingga salinan baris tidak bisa dipakai di tenant lain.
type secretBox struct{ aead cipher.AEAD }

func newSecretBox(secret string) (*secretBox, error) {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte("aciraba-erapay-apikey-v1"))
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
	n := b.aead.NonceSize()
	if err != nil || len(raw) < n {
		return "", errors.New("kunci EraPay rusak")
	}
	plain, err := b.aead.Open(nil, raw[:n], raw[n:], []byte(aad))
	if err != nil {
		return "", errors.New("kunci EraPay tidak dapat dibuka")
	}
	return string(plain), nil
}
