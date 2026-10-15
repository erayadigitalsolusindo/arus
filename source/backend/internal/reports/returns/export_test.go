package returns

import (
	"bytes"
	"strings"
	"testing"

	"github.com/shopspring/decimal"
)

// Test tanpa DB: keempat tabel ekspor menghasilkan XLSX/PDF valid, jumlah nilai tiap
// baris = jumlah kolom, dan SQL kedua jenis retur memakai parameter yang konsisten.
func TestExportTables(t *testing.T) {
	h := Header{From: "2026-10-01", To: "2026-10-15", TenantName: "Toko Uji", OutletName: "Pusat", OutletCode: "PST"}
	res := Result{Header: h, HasSummary: true, Summary: Summary{
		ReturnCount: 1, Subtotal: "100.00", Discount: "10.00", TaxAmount: "9.90", Total: "99.90",
		Offset: "39.90", Refund: "60.00", BaseTotal: "999.00", RatioPct: "10.00"},
		Data: []Row{{DocNo: "RJ-1", RefDocNo: "INV-1", Date: "2026-10-02", PartyName: "Budi", UserName: "Kasir",
			Subtotal: "100.00", Discount: "10.00", TaxAmount: "9.90", Total: "99.90", Offset: "39.90", Refund: "60.00", RefundMethod: "Tunai",
			Lines: []Line{{SKU: "A1", Name: "Gula 1kg", Unit: "pcs", Qty: "2.500", UnitPrice: "40.00", Value: "100.00", Discount: "10.00", TaxAmount: "9.90", Net: "99.90"}}}}}
	for _, k := range []Kind{KindSale, KindPurchase} {
		for name, tb := range map[string]table{"invoices": InvoiceTable(k, res), "detail": DetailTable(k, res)} {
			for _, r := range append(append([][]string{}, tb.Rows...), tb.Total) {
				if len(r) != len(tb.Cols) {
					t.Fatalf("kind %d %s: baris %d nilai, kolom %d", k, name, len(r), len(tb.Cols))
				}
			}
			w := 0.0
			for _, c := range tb.Cols {
				w += c.Width
			}
			if w > 339 {
				t.Fatalf("kind %d %s: lebar PDF %.0f > 339", k, name, w)
			}
			if x, err := tb.ToXLSX(); err != nil || !bytes.HasPrefix(x, []byte("PK")) {
				t.Fatalf("kind %d %s xlsx: %v", k, name, err)
			}
			if p, err := tb.ToPDF(); err != nil || !bytes.HasPrefix(p, []byte("%PDF")) {
				t.Fatalf("kind %d %s pdf: %v", k, name, err)
			}
		}
	}
	if _, _, meta := kop(KindSale, "", res); !strings.Contains(strings.Join(meta, "|"), "Rasio retur: 10,00%") {
		t.Fatalf("kop tanpa rasio retur: %v", meta)
	}
}

func TestRatioPct(t *testing.T) {
	if got := ratioPct(mustDec("25"), mustDec("1000")); got != "2.50" {
		t.Fatalf("ratio = %s", got)
	}
	if got := ratioPct(mustDec("25"), mustDec("0")); got != "0.00" {
		t.Fatalf("ratio dasar 0 = %s", got)
	}
}

func TestSQLPlaceholders(t *testing.T) {
	for k, set := range sets {
		for _, flat := range []bool{false, true} {
			sql := set.rowsSQL(flat)
			for _, ph := range []string{"$1", "$2", "$3", "$4", "$5", "$6", "$7", "$8", "$9", "$10", "$11", "$12", "$13"} {
				if !strings.Contains(sql, ph) {
					t.Errorf("kind %d flat=%v: %s tidak dirujuk", k, flat, ph)
				}
			}
		}
		for _, search := range []bool{false, true} {
			sql := set.summarySQL(search)
			for _, ph := range []string{"$1", "$2", "$3", "$4", "$5", "$6", "$7", "$8"} {
				if !strings.Contains(sql, ph) {
					t.Errorf("kind %d summary search=%v: %s tidak dirujuk", k, search, ph)
				}
			}
		}
	}
}

func TestCursorRoundTrip(t *testing.T) {
	if _, _, _, ok := decodeCursor("???"); ok {
		t.Fatal("cursor rusak harus ditolak")
	}
}

func mustDec(s string) decimal.Decimal {
	d, err := decimal.NewFromString(s)
	if err != nil {
		panic(err)
	}
	return d
}
