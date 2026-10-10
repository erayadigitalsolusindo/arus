//go:build !windows

package main

import (
	"errors"
	"os"
)

// Di luar Windows (mis. PC kasir Linux) printer USB thermal muncul sebagai berkas perangkat, umumnya /dev/usb/lp0.
type devicePrinter struct{ path string }

func newPrinter(cfg Config) (Printer, error) {
	if cfg.Device == "" {
		return nil, errors.New(`isi "device" di konfigurasi, mis. /dev/usb/lp0`)
	}
	return &devicePrinter{path: cfg.Device}, nil
}

func (p *devicePrinter) Name() string { return p.path }

func (p *devicePrinter) Print(_ string, data []byte) error {
	f, err := os.OpenFile(p.path, os.O_WRONLY|os.O_APPEND|os.O_CREATE, 0o600)
	if err != nil {
		return err
	}
	if _, err := f.Write(data); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}
