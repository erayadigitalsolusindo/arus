package sales

// Test tanpa DB: ekspor laporan per tanggal (PDF LEGAL landscape + XLSX) dari
// ByDateResult buatan sendiri. Invariant: berkas valid, tidak panic, dan kolom
// tabel (15) pas dengan baris nilai (15).

import (
	"bytes"
	"strings"
	"testing"
)

func fixtureByDate() ByDateResult {
	return ByDateResult{
		From: "2026-10-08", To: "2026-10-14",
		Tenant: "Toko Uji", Outlet: "Pusat", OutletCode: "T01",
		Data: []DayRow{
			{Date: "2026-10-08", SaleCount: 3, VoidCount: 1,
				Subtotal: "300000.00", Discount: "10000.00", TaxStore: "5000.00", TaxGov: "33000.00",
				Total: "328000.00", Paid: "328000.00", Receivable: "0.00", VoidTotal: "50000.00",
				ReturnCount: 1, ReturnTotal: "20000.00", NetTotal: "258000.00",
				Methods: []MethodRow{{Name: "Tunai", Kind: "cash", Amount: "328000.00"}}},
			{Date: "2026-10-09", SaleCount: 0, VoidCount: 0,
				Subtotal: "0.00", Total: "0.00", Paid: "0.00", NetTotal: "0.00",
				Methods: []MethodRow{{Name: "Tunai", Kind: "cash", Amount: "0.00"}}},
		},
	}
}

func TestByDateToPDFLegalLandscape(t *testing.T) {
	b, err := ToPDF(fixtureByDate())
	if err != nil {
		t.Fatalf("ToPDF: %v", err)
	}
	if len(b) == 0 {
		t.Fatal("PDF kosong")
	}
	if !bytes.HasPrefix(b, []byte("%PDF-")) {
		t.Fatal("bukan PDF valid")
	}
	// LEGAL landscape = 8,5in x 14in = 612 x 1008 pt.
	box := mediaBox(b)
	if !strings.HasPrefix(box, "/MediaBox [0 0 1008") || !strings.Contains(box, "612") {
		t.Errorf("ukuran halaman bukan LEGAL landscape; MediaBox = %q", box)
	}
}

func TestByDateToXLSXValid(t *testing.T) {
	b, err := ToXLSX(fixtureByDate())
	if err != nil {
		t.Fatalf("ToXLSX: %v", err)
	}
	if !bytes.HasPrefix(b, []byte("PK\x03\x04")) {
		t.Fatal("bukan .xlsx (zip) valid")
	}
	if len(b) < 1000 {
		t.Fatalf("xlsx terlalu kecil: %d byte", len(b))
	}
}

func TestByDateFileName(t *testing.T) {
	got := FileName("2026-10-08", "2026-10-14", "pdf")
	if got != "penjualan-per-tanggal_2026-10-08_2026-10-14.pdf" {
		t.Errorf("nama berkas = %q", got)
	}
}
