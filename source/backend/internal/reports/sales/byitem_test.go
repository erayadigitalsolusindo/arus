package sales

import "testing"

func TestQtyText(t *testing.T) {
	cases := map[string]string{
		"12.000":     "12",
		"1234.500":   "1.234,5",
		"0.250":      "0,25",
		"-3.000":     "-3",
		"1000000.00": "1.000.000",
		"x":          "0",
	}
	for in, want := range cases {
		if got := qtyText(in); got != want {
			t.Errorf("qtyText(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestItemExports(t *testing.T) {
	res := ItemResult{
		From: "2026-10-01", To: "2026-10-15", TenantName: "Toko", OutletName: "Pusat", OutletCode: "PST",
		SortLabel: "Total Penjualan",
		Data: []ItemRow{{ItemID: "x", SKU: "A1", Name: "Barang Uji", CategoryName: "Umum", UnitName: "Pcs",
			Qty: "12.500", LineCount: 3, Gross: "100000.00", Discount: "5000.00", Net: "95000.00"}},
		Summary: ItemSummary{ItemCount: 1, Gross: "100000.00", Discount: "5000.00", Net: "95000.00"},
	}
	if b, err := ItemToXLSX(res); err != nil || len(b) < 100 {
		t.Fatalf("xlsx: %v len=%d", err, len(b))
	}
	if b, err := ItemToPDF(res); err != nil || len(b) < 100 || string(b[:4]) != "%PDF" {
		t.Fatalf("pdf: %v len=%d", err, len(b))
	}
}
