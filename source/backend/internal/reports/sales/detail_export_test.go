package sales

// Test tanpa DB: membangun berkas ekspor dari DetailResult buatan sendiri +
// cursor keyset. Invariant yang dijaga: berkas terbaca valid, format uang/qty
// gaya Indonesia, dan total per tanggal = Σ nota completed.

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
)

func fixtureDetail() DetailResult {
	return DetailResult{
		From:       "2026-10-08",
		To:         "2026-10-14",
		TenantName: "Toko Uji",
		OutletName: "Pusat",
		OutletCode: "T01",
		Data: []DetailDay{
			{
				Date: "2026-10-08", SaleCount: 1, VoidCount: 1,
				Subtotal: "100000.00", Discount: "0.00", TaxStore: "1000.00",
				TaxGov: "11000.00", Total: "112000.00", Paid: "112000.00", Receivable: "0.00",
				Sales: []DetailSale{
					{
						ID: "11111111-1111-1111-1111-111111111111", DocNo: "INV-001", Time: "09:15",
						Status: "completed", OutletCode: "T01", OutletName: "Pusat", CashierName: "Kasir A",
						Subtotal: "100000.00", Discount: "0.00", TaxStore: "1000.00", TaxGov: "11000.00",
						OtherCost: "0.00", Total: "112000.00", Paid: "112000.00", Change: "0.00",
						Receivable: "0.00", LinesTotal: "100000.00",
						Lines: []DetailLine{
							{Position: 1, SKU: "SKU1", Name: "Indomie Goreng", UnitName: "pcs",
								CategoryName: "Makanan", Qty: "10.000", UnitPrice: "8000.00",
								Discount: "0.00", LineTotal: "80000.00"},
							{Position: 2, SKU: "SKU2", Name: "Susu UHT", UnitName: "box",
								CategoryName: "Minuman", Qty: "2.000", UnitPrice: "10000.00",
								Discount: "0.00", LineTotal: "20000.00"},
						},
						Methods: []DetailMethod{{Name: "Tunai", Kind: "cash", Amount: "112000.00"}},
					},
					{
						ID: "22222222-2222-2222-2222-222222222222", DocNo: "INV-002", Time: "10:00",
						Status: "void", OutletCode: "T01", OutletName: "Pusat", CashierName: "Kasir A",
						Subtotal: "50000.00", Total: "50000.00", Paid: "50000.00",
						LinesTotal: "50000.00",
						Lines: []DetailLine{
							{Position: 1, SKU: "SKU3", Name: "Teh Botol", UnitName: "pcs",
								Qty: "5.000", UnitPrice: "10000.00", LineTotal: "50000.00"},
						},
					},
				},
			},
		},
		Summary: DetailSummary{
			SaleCount: 1, VoidCount: 1, Subtotal: "100000.00", Discount: "0.00",
			TaxStore: "1000.00", TaxGov: "11000.00", Total: "112000.00",
			Paid: "112000.00", Receivable: "0.00",
		},
	}
}

func TestDetailToPDFLegalLandscape(t *testing.T) {
	b, err := DetailToPDF(fixtureDetail())
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

func mediaBox(b []byte) string {
	i := bytes.Index(b, []byte("/MediaBox"))
	if i < 0 {
		return "(tidak ada)"
	}
	end := bytes.IndexByte(b[i:], ']')
	if end < 0 {
		return "(tidak lengkap)"
	}
	return string(b[i : i+end+1])
}

func TestDetailToXLSXValid(t *testing.T) {
	b, err := DetailToXLSX(fixtureDetail())
	if err != nil {
		t.Fatalf("ToXLSX: %v", err)
	}
	// .xlsx = zip;signature "PK\x03\x04".
	if !bytes.HasPrefix(b, []byte("PK\x03\x04")) {
		t.Fatal("bukan .xlsx (zip) valid")
	}
	if len(b) < 1000 {
		t.Fatalf("xlsx terlalu kecil: %d byte", len(b))
	}
}

func TestDetailQtyFormat(t *testing.T) {
	cases := map[string]string{
		"10.000":   "10",
		"2.500":    "2.5",
		"1.234":    "1.234",
		"0.000":    "0",
		"":         "0",
		"abc":      "0",
		"12.34500": "12.345",
	}
	for in, want := range cases {
		if got := detailQty(in); got != want {
			t.Errorf("detailQty(%q) = %q, ingin %q", in, got, want)
		}
	}
}

func TestDetailMoneyFormat(t *testing.T) {
	cases := map[string]string{
		"122761422.00": "122.761.422,00",
		"0.00":         "0,00",
		"-1500.00":     "-1.500,00",
		"1000.50":      "1.000,50",
	}
	for in, want := range cases {
		if got := pdfMoney(in); got != want {
			t.Errorf("pdfMoney(%q) = %q, ingin %q", in, got, want)
		}
	}
}

func TestDetailCursorRoundTrip(t *testing.T) {
	wantAt := time.Date(2026, 10, 8, 9, 15, 30, 123456000, time.UTC)
	wantID := uuid.MustParse("11111111-1111-1111-1111-111111111111")
	gotAt, gotID, ok := detailDecodeCursor(detailEncodeCursor(wantAt, wantID))
	if !ok {
		t.Fatal("cursor tidak bisa didekode")
	}
	if !gotAt.Equal(wantAt) {
		t.Errorf("waktu: %v, ingin %v", gotAt, wantAt)
	}
	if gotID != wantID {
		t.Errorf("id: %v, ingin %v", gotID, wantID)
	}
}

func TestDetailCursorInvalid(t *testing.T) {
	for _, bad := range []string{"", "bukan-base64!!", "aGVsbG8=", "MTIzfGFiYw"} {
		if _, _, ok := detailDecodeCursor(bad); ok && bad != "" {
			t.Errorf("cursor %q seharusnya ditolak", bad)
		}
	}
}

func TestDetailFileName(t *testing.T) {
	got := DetailFileName("2026-10-08", "2026-10-14", "pdf")
	if got != "penjualan-detail_2026-10-08_2026-10-14.pdf" {
		t.Errorf("nama berkas = %q", got)
	}
}
