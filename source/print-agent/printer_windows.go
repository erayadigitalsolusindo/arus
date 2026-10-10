//go:build windows

package main

import (
	"errors"
	"fmt"
	"syscall"
	"unsafe"
)

// Cetak RAW lewat spooler Windows: driver printer yang sudah terpasang tetap dipakai sebagai jalur,
// tetapi isi dikirim apa adanya (perintah ESC/POS), bukan dirender driver menjadi gambar.
var (
	winspool           = syscall.NewLazyDLL("winspool.drv")
	procOpenPrinter    = winspool.NewProc("OpenPrinterW")
	procClosePrinter   = winspool.NewProc("ClosePrinter")
	procStartDoc       = winspool.NewProc("StartDocPrinterW")
	procEndDoc         = winspool.NewProc("EndDocPrinter")
	procStartPage      = winspool.NewProc("StartPagePrinter")
	procEndPage        = winspool.NewProc("EndPagePrinter")
	procWritePrinter   = winspool.NewProc("WritePrinter")
	procDefaultPrinter = winspool.NewProc("GetDefaultPrinterW")
)

type docInfo1 struct {
	docName    *uint16
	outputFile *uint16
	datatype   *uint16
}

type spoolPrinter struct{ name string }

// newPrinter: nama kosong = printer default Windows (dibaca ulang tiap cetak agar perubahan default langsung berlaku).
func newPrinter(cfg Config) (Printer, error) { return &spoolPrinter{name: cfg.Printer}, nil }

func (p *spoolPrinter) Name() string {
	if p.name != "" {
		return p.name
	}
	n, err := defaultPrinter()
	if err != nil {
		return "(default: " + err.Error() + ")"
	}
	return n
}

func defaultPrinter() (string, error) {
	var size uint32
	procDefaultPrinter.Call(0, uintptr(unsafe.Pointer(&size)))
	if size == 0 {
		return "", errors.New("tidak ada printer default")
	}
	buf := make([]uint16, size)
	if r, _, err := procDefaultPrinter.Call(uintptr(unsafe.Pointer(&buf[0])), uintptr(unsafe.Pointer(&size))); r == 0 {
		return "", fmt.Errorf("GetDefaultPrinter: %w", err)
	}
	return syscall.UTF16ToString(buf), nil
}

func (p *spoolPrinter) Print(job string, data []byte) error {
	name := p.name
	if name == "" {
		var err error
		if name, err = defaultPrinter(); err != nil {
			return err
		}
	}
	pName, err := syscall.UTF16PtrFromString(name)
	if err != nil {
		return err
	}
	var h syscall.Handle
	if r, _, err := procOpenPrinter.Call(uintptr(unsafe.Pointer(pName)), uintptr(unsafe.Pointer(&h)), 0); r == 0 {
		return fmt.Errorf("OpenPrinter %q: %w", name, err)
	}
	defer procClosePrinter.Call(uintptr(h))

	docName, _ := syscall.UTF16PtrFromString(job)
	raw, _ := syscall.UTF16PtrFromString("RAW")
	di := docInfo1{docName: docName, datatype: raw}
	if r, _, err := procStartDoc.Call(uintptr(h), 1, uintptr(unsafe.Pointer(&di))); r == 0 {
		return fmt.Errorf("StartDocPrinter: %w", err)
	}
	defer procEndDoc.Call(uintptr(h))
	if r, _, err := procStartPage.Call(uintptr(h)); r == 0 {
		return fmt.Errorf("StartPagePrinter: %w", err)
	}
	defer procEndPage.Call(uintptr(h))
	for len(data) > 0 {
		var written uint32
		r, _, err := procWritePrinter.Call(uintptr(h), uintptr(unsafe.Pointer(&data[0])), uintptr(len(data)), uintptr(unsafe.Pointer(&written)))
		if r == 0 || written == 0 {
			return fmt.Errorf("WritePrinter: %w", err)
		}
		data = data[written:]
	}
	return nil
}
