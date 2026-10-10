//go:build !windows

package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDevicePrinterAppends(t *testing.T) {
	path := filepath.Join(t.TempDir(), "lp0")
	p, err := newPrinter(Config{Device: path})
	if err != nil {
		t.Fatal(err)
	}
	_ = p.Print("a", []byte("A"))
	_ = p.Print("b", []byte("B"))
	if b, _ := os.ReadFile(path); string(b) != "AB" {
		t.Fatalf("isi perangkat %q", b)
	}
}
