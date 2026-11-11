package backup

import (
	"bytes"
	"crypto/rand"
	"io"
	"testing"
)

func roundTrip(t *testing.T, plain []byte, pass string) ([]byte, []byte, error) {
	t.Helper()
	var enc bytes.Buffer
	w, err := Encrypt(&enc, pass)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := w.Write(plain); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	cipher := append([]byte(nil), enc.Bytes()...)
	r, err := Decrypt(&enc, pass)
	if err != nil {
		return cipher, nil, err
	}
	out, err := io.ReadAll(r)
	return cipher, out, err
}

func TestEncryptRoundTripSizes(t *testing.T) {
	for _, n := range []int{0, 1, chunkSize - 1, chunkSize, chunkSize + 1, 2*chunkSize + 7} {
		plain := make([]byte, n)
		_, _ = rand.Read(plain)
		_, got, err := roundTrip(t, plain, "pass")
		if err != nil || !bytes.Equal(got, plain) {
			t.Fatalf("n=%d: err=%v sama=%v", n, err, bytes.Equal(got, plain))
		}
	}
}

func TestDecryptRejectsWrongPassphraseTruncationAndTamper(t *testing.T) {
	plain := make([]byte, chunkSize+100)
	_, _ = rand.Read(plain)
	enc, _, _ := roundTrip(t, plain, "benar")

	if _, err := io.ReadAll(mustDecrypt(t, enc, "salah")); err == nil {
		t.Fatal("passphrase salah harus ditolak")
	}
	if _, err := io.ReadAll(mustDecrypt(t, enc[:len(enc)-30], "benar")); err == nil {
		t.Fatal("berkas terpotong harus ditolak")
	}
	cut := append([]byte(nil), enc...)
	cut[len(cut)/2] ^= 1
	if _, err := io.ReadAll(mustDecrypt(t, cut, "benar")); err == nil {
		t.Fatal("berkas diubah harus ditolak")
	}
}

func mustDecrypt(t *testing.T, b []byte, pass string) io.Reader {
	t.Helper()
	r, err := Decrypt(bytes.NewReader(b), pass)
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func TestSealerAndPassphrase(t *testing.T) {
	s, _ := newSealer("jwt-secret")
	enc, _ := s.seal("rahasia", "a")
	if got, err := s.open(enc, "a"); err != nil || got != "rahasia" {
		t.Fatalf("got %q err %v", got, err)
	}
	if _, err := s.open(enc, "b"); err == nil {
		t.Fatal("AAD berbeda harus ditolak")
	}
	if p, _ := NewPassphrase(); len(p) != 39 { // 32 karakter + 7 tanda hubung
		t.Fatalf("panjang passphrase %d", len(p))
	}
}
