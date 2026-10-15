package purchases

import (
	"bytes"
	"testing"
)

// Test tanpa DB: keempat tabel ekspor menghasilkan XLSX/PDF valid, tanpa panic,
// dan jumlah nilai tiap baris = jumlah kolom.
func TestExportTables(t *testing.T) {
	h := Header{From: "2026-10-01", To: "2026-10-15", TenantName: "Toko Uji", OutletName: "Pusat", OutletCode: "PST"}
	inv := InvoiceResult{Header: h, HasSummary: true, Summary: InvoiceSummary{PurchaseCount: 1, Subtotal: "100.00", Total: "111.00"},
		Data: []InvoiceRow{{DocNo: "PB-1", Date: "2026-10-02", SupplierName: "PT Maju", PaymentType: "credit", DueDate: "2026-11-02",
			Subtotal: "100.00", TaxAmount: "11.00", Total: "111.00",
			Lines: []LineRow{{SKU: "A1", Name: "Gula 1kg", Unit: "pcs", Qty: "2.500", UnitPrice: "40.00", Disc1: "10.00", LineTotal: "90.00"}}}}}
	tables := map[string]table{
		"invoices": InvoiceTable(inv),
		"detail":   InvoiceDetailTable(inv),
		"items": ItemTable(ItemResult{Header: h, Summary: ItemSummary{ItemCount: 1, Total: "90.00"},
			Data: []ItemRow{{SKU: "A1", Name: "Gula", Unit: "pcs", PurchaseCount: 1, Qty: "2.500", Total: "90.00", AvgPrice: "36.00", LastPrice: "40.00"}}}),
		"bydate": ByDateTable(ByDateResult{Header: h,
			Data:   []DayRow{{Date: "2026-10-02", PurchaseCount: 1, Total: "111.00", NetTotal: "111.00"}},
			Totals: DayRow{PurchaseCount: 1, Total: "111.00", NetTotal: "111.00"}}),
	}
	for name, tb := range tables {
		for _, r := range append(append([][]string{}, tb.rows...), tb.total) {
			if len(r) != len(tb.cols) {
				t.Fatalf("%s: baris %d nilai, kolom %d", name, len(r), len(tb.cols))
			}
		}
		x, err := tb.toXLSX()
		if err != nil || !bytes.HasPrefix(x, []byte("PK")) {
			t.Fatalf("%s xlsx: %v", name, err)
		}
		p, err := tb.toPDF()
		if err != nil || !bytes.HasPrefix(p, []byte("%PDF")) {
			t.Fatalf("%s pdf: %v", name, err)
		}
	}
}

func TestPDFMoney(t *testing.T) {
	for in, want := range map[string]string{"0": "0,00", "1234567.5": "1.234.567,50", "-999.99": "-999,99", "1000": "1.000,00"} {
		if got := pdfMoney(in); got != want {
			t.Errorf("pdfMoney(%s) = %s, want %s", in, got, want)
		}
	}
}

func TestCursorRoundTrip(t *testing.T) {
	if _, _, _, ok := decodeCursor("???"); ok {
		t.Fatal("cursor rusak harus ditolak")
	}
}
