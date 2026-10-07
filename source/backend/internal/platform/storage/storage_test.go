package storage

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"
)

func TestLocalPutOpenDelete(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	s, err := NewLocal(filepath.Join(root, "uploads"))
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Put(ctx, "tenant1/img1.jpg", []byte("halo")); err != nil {
		t.Fatal(err)
	}
	f, err := s.Open(ctx, "tenant1/img1.jpg")
	if err != nil {
		t.Fatal(err)
	}
	b, _ := io.ReadAll(f)
	f.Close()
	if string(b) != "halo" {
		t.Errorf("isi = %q", b)
	}
	// Menimpa menggantikan isi; tidak ada file sementara tersisa.
	if err := s.Put(ctx, "tenant1/img1.jpg", []byte("baru")); err != nil {
		t.Fatal(err)
	}
	entries, _ := os.ReadDir(filepath.Join(root, "uploads", "tenant1"))
	if len(entries) != 1 {
		t.Errorf("file di folder = %d, want 1 (tanpa sisa sementara)", len(entries))
	}
	if err := s.Delete(ctx, "tenant1/img1.jpg"); err != nil {
		t.Fatal(err)
	}
	if err := s.Delete(ctx, "tenant1/img1.jpg"); err != nil {
		t.Errorf("hapus file yang sudah tidak ada tidak boleh error: %v", err)
	}
	if _, err := s.Open(ctx, "tenant1/img1.jpg"); !errors.Is(err, ErrNotFound) {
		t.Errorf("buka file hilang: %v", err)
	}
}

func TestLocalRejectsBadKeys(t *testing.T) {
	ctx := context.Background()
	s, err := NewLocal(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"", "../x", "a/../../x", "/abs/x", "a//b", "A/b.jpg", "a b", "a\\b", "a/.hidden", "a/b.", "a/b.JPG", "a/b\x00.jpg", "..", "a/.."} {
		if err := s.Put(ctx, key, []byte("x")); err == nil {
			t.Errorf("Put(%q) seharusnya ditolak", key)
		}
		if _, err := s.Open(ctx, key); err == nil || errors.Is(err, ErrNotFound) {
			t.Errorf("Open(%q) seharusnya ditolak sebagai kunci tidak valid, got %v", key, err)
		}
		if err := s.Delete(ctx, key); err == nil {
			t.Errorf("Delete(%q) seharusnya ditolak", key)
		}
	}
}
