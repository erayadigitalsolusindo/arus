package backup

// Enkripsi dua lapis: (1) rahasia konfigurasi di DB (client secret, refresh token, passphrase) dikunci AES-GCM dari JWT_SECRET;
// (2) berkas backup di Drive dikunci AES-256-GCM per potongan 1 MiB dengan kunci scrypt(passphrase). Passphrase dipegang operator,
// jadi salinan tetap terbuka walau server dan JWT_SECRET hilang. Format berkas: "ARBK1\n" + salt(16) + potongan[uint32 panjang
// (bit tinggi = potongan terakhir) + ciphertext]. Bendera "terakhir" masuk nonce sehingga berkas terpotong ketahuan.

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base32"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"io"

	"golang.org/x/crypto/hkdf"
	"golang.org/x/crypto/scrypt"
)

const (
	magic     = "ARBK1\n"
	chunkSize = 1 << 20
	lastBit   = 1 << 31
)

var ErrBadFile = errors.New("berkas backup rusak, terpotong, atau passphrase salah")

type sealer struct{ aead cipher.AEAD }

func newSealer(secret string) (*sealer, error) {
	key := make([]byte, 32)
	if _, err := io.ReadFull(hkdf.New(sha256.New, []byte(secret), nil, []byte("aciraba-backup-v1")), key); err != nil {
		return nil, err
	}
	b, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	g, err := cipher.NewGCM(b)
	return &sealer{g}, err
}

func (s *sealer) seal(plain, aad string) (string, error) {
	nonce := make([]byte, s.aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return "", err
	}
	return base64.RawStdEncoding.EncodeToString(s.aead.Seal(nonce, nonce, []byte(plain), []byte(aad))), nil
}

func (s *sealer) open(enc, aad string) (string, error) {
	raw, err := base64.RawStdEncoding.DecodeString(enc)
	if err != nil || len(raw) < s.aead.NonceSize() {
		return "", errors.New("backup: rahasia rusak")
	}
	n := s.aead.NonceSize()
	out, err := s.aead.Open(nil, raw[:n], raw[n:], []byte(aad))
	return string(out), err
}

func fileAEAD(passphrase string, salt []byte) (cipher.AEAD, error) {
	key, err := scrypt.Key([]byte(passphrase), salt, 1<<15, 8, 1, 32)
	if err != nil {
		return nil, err
	}
	b, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	return cipher.NewGCM(b)
}

func chunkNonce(n uint64, last bool) []byte {
	nonce := make([]byte, 12)
	binary.BigEndian.PutUint64(nonce, n)
	if last {
		nonce[8] = 1
	}
	return nonce
}

type encWriter struct {
	w    io.Writer
	aead cipher.AEAD
	buf  []byte
	n    uint64
}

// Encrypt membungkus w: tulis plaintext ke hasilnya, lalu Close() (wajib) untuk potongan terakhir.
func Encrypt(w io.Writer, passphrase string) (io.WriteCloser, error) {
	salt := make([]byte, 16)
	if _, err := rand.Read(salt); err != nil {
		return nil, err
	}
	aead, err := fileAEAD(passphrase, salt)
	if err != nil {
		return nil, err
	}
	if _, err := w.Write(append([]byte(magic), salt...)); err != nil {
		return nil, err
	}
	return &encWriter{w: w, aead: aead, buf: make([]byte, 0, chunkSize)}, nil
}

func (e *encWriter) Write(p []byte) (int, error) {
	total := len(p)
	for len(p) > 0 {
		k := min(chunkSize-len(e.buf), len(p))
		e.buf = append(e.buf, p[:k]...)
		p = p[k:]
		if len(e.buf) == chunkSize && len(p) > 0 { // potongan penuh bukan yang terakhir bila masih ada data
			if err := e.flush(false); err != nil {
				return 0, err
			}
		}
	}
	return total, nil
}

func (e *encWriter) flush(last bool) error {
	ct := e.aead.Seal(nil, chunkNonce(e.n, last), e.buf, nil)
	hdr := uint32(len(ct))
	if last {
		hdr |= lastBit
	}
	var h [4]byte
	binary.BigEndian.PutUint32(h[:], hdr)
	if _, err := e.w.Write(append(h[:], ct...)); err != nil {
		return err
	}
	e.n++
	e.buf = e.buf[:0]
	return nil
}

func (e *encWriter) Close() error { return e.flush(true) }

// Decrypt membuka aliran hasil Encrypt. Galat ErrBadFile bila passphrase salah, rusak, atau terpotong.
func Decrypt(r io.Reader, passphrase string) (io.Reader, error) {
	head := make([]byte, len(magic)+16)
	if _, err := io.ReadFull(r, head); err != nil || string(head[:len(magic)]) != magic {
		return nil, ErrBadFile
	}
	aead, err := fileAEAD(passphrase, head[len(magic):])
	if err != nil {
		return nil, err
	}
	pr, pw := io.Pipe()
	go func() {
		var n uint64
		for {
			var h [4]byte
			if _, err := io.ReadFull(r, h[:]); err != nil {
				pw.CloseWithError(ErrBadFile) // berkas berakhir tanpa potongan "terakhir"
				return
			}
			v := binary.BigEndian.Uint32(h[:])
			last := v&lastBit != 0
			size := v &^ lastBit
			if size < uint32(aead.Overhead()) || size > chunkSize+uint32(aead.Overhead()) {
				pw.CloseWithError(ErrBadFile)
				return
			}
			ct := make([]byte, size)
			if _, err := io.ReadFull(r, ct); err != nil {
				pw.CloseWithError(ErrBadFile)
				return
			}
			pt, err := aead.Open(nil, chunkNonce(n, last), ct, nil)
			if err != nil {
				pw.CloseWithError(ErrBadFile)
				return
			}
			n++
			if _, err := pw.Write(pt); err != nil {
				return
			}
			if last {
				pw.Close()
				return
			}
		}
	}()
	return pr, nil
}

// NewPassphrase = 32 karakter acak (base32) dikelompokkan 4-4 agar mudah dicatat.
func NewPassphrase() (string, error) {
	b := make([]byte, 20)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	s := base32.StdEncoding.EncodeToString(b) // 20 byte = tepat 32 karakter, tanpa padding
	out := ""
	for i := 0; i < len(s); i += 4 {
		if i > 0 {
			out += "-"
		}
		out += s[i : i+4]
	}
	return out, nil
}
