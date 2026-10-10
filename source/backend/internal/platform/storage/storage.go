// Package storage menyimpan file unggahan. Antarmuka Store sengaja kecil agar disk lokal sekarang dapat diganti
// object storage (S3/MinIO) nanti tanpa mengubah pemanggil. Kunci (key) selalu dibuat server — tidak pernah dari
// input pengguna — dan tetap divalidasi di sini sebagai lapis kedua terhadap path traversal.
package storage

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

var ErrNotFound = errors.New("file tidak ditemukan")

// Store: Put menulis atomik (tidak pernah meninggalkan file setengah jadi), Delete tidak error bila file sudah tidak ada.
type Store interface {
	Put(ctx context.Context, key string, data []byte) error
	Open(ctx context.Context, key string) (io.ReadSeekCloser, error)
	Delete(ctx context.Context, key string) error
}

// Format kunci: segmen [a-z0-9_-] dipisah "/", segmen terakhir boleh berakhiran .ext. Tanpa "..", tanpa awalan "/".
var keyRe = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]*(/[a-z0-9][a-z0-9_-]*)*(\.[a-z0-9]{1,8})?$`)

func validKey(key string) error {
	if len(key) > 200 || !keyRe.MatchString(key) {
		return fmt.Errorf("storage: kunci tidak valid %q", key)
	}
	return nil
}

// Local menyimpan file di bawah satu folder akar.
type Local struct{ root string }

// NewLocal membuat folder akar bila belum ada (izin 0750: hanya pemilik proses + grup).
func NewLocal(root string) (*Local, error) {
	abs, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(abs, 0o750); err != nil {
		return nil, fmt.Errorf("storage: buat folder: %w", err)
	}
	return &Local{root: abs}, nil
}

func (l *Local) path(key string) (string, error) {
	if err := validKey(key); err != nil {
		return "", err
	}
	p := filepath.Join(l.root, filepath.FromSlash(key))
	if !strings.HasPrefix(p, l.root+string(filepath.Separator)) {
		return "", fmt.Errorf("storage: kunci di luar akar %q", key)
	}
	return p, nil
}

func (l *Local) Put(_ context.Context, key string, data []byte) error {
	p, err := l.path(key)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o750); err != nil {
		return fmt.Errorf("storage: buat folder: %w", err)
	}
	// Tulis ke file sementara lalu rename: pembaca tidak pernah melihat file yang belum selesai ditulis.
	tmp, err := os.CreateTemp(filepath.Dir(p), ".tmp-*")
	if err != nil {
		return fmt.Errorf("storage: file sementara: %w", err)
	}
	name := tmp.Name()
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		os.Remove(name)
		return fmt.Errorf("storage: tulis: %w", err)
	}
	if err := tmp.Close(); err != nil {
		os.Remove(name)
		return fmt.Errorf("storage: tutup: %w", err)
	}
	if err := os.Chmod(name, 0o640); err != nil && !errors.Is(err, fs.ErrPermission) {
		os.Remove(name)
		return err
	}
	if err := os.Rename(name, p); err != nil {
		os.Remove(name)
		return fmt.Errorf("storage: rename: %w", err)
	}
	return nil
}

func (l *Local) Open(_ context.Context, key string) (io.ReadSeekCloser, error) {
	p, err := l.path(key)
	if err != nil {
		return nil, err
	}
	f, err := os.Open(p)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, ErrNotFound
	}
	return f, err
}

func (l *Local) Delete(_ context.Context, key string) error {
	p, err := l.path(key)
	if err != nil {
		return err
	}
	if err := os.Remove(p); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	return nil
}

// PutStream menulis isi r (streaming, tanpa menampung seluruhnya di memori) ke file sementara, menjalankan verify atas
// file itu, lalu rename atomik ke key. Dipakai untuk berkas besar (APK). verify boleh nil. Mengembalikan jumlah byte.
func (l *Local) PutStream(_ context.Context, key string, r io.Reader, verify func(path string, size int64) error) (int64, error) {
	p, err := l.path(key)
	if err != nil {
		return 0, err
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o750); err != nil {
		return 0, fmt.Errorf("storage: buat folder: %w", err)
	}
	tmp, err := os.CreateTemp(filepath.Dir(p), ".tmp-*")
	if err != nil {
		return 0, fmt.Errorf("storage: file sementara: %w", err)
	}
	name := tmp.Name()
	n, err := io.Copy(tmp, r)
	if cerr := tmp.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		os.Remove(name)
		return 0, fmt.Errorf("storage: tulis: %w", err)
	}
	if verify != nil {
		if err := verify(name, n); err != nil {
			os.Remove(name)
			return 0, err
		}
	}
	if err := os.Chmod(name, 0o640); err != nil && !errors.Is(err, fs.ErrPermission) {
		os.Remove(name)
		return 0, err
	}
	if err := os.Rename(name, p); err != nil {
		os.Remove(name)
		return 0, fmt.Errorf("storage: rename: %w", err)
	}
	return n, nil
}
