package stock

import (
	"bytes"
	"testing"

	"github.com/xuri/excelize/v2"
)

func sampleMasterTable() MasterTable {
	return MasterTable{
		Title: "LAPORAN MASTER SALES", Tenant: "TOKO", Sheet: "Master Sales", FileBase: "master-sales",
		Meta: []string{"Status: Semua"},
		Cols: []masterCol{
			{"No", 8, "R", fmtText}, {"Kode", 24, "L", fmtText}, {"Nama", 70, "L", fmtText},
			{"Komisi (%)", 24, "R", fmtDec}, {"Catatan", 0, "L", fmtText},
		},
		Rows: [][]string{
			{"S1", "Budi", "2.50", "catatan panjang"},
			{"S2", "Ani", "0.00", ""},
		},
	}
}

func TestMasterTableXLSX(t *testing.T) {
	b, err := sampleMasterTable().ToXLSX()
	if err != nil {
		t.Fatal(err)
	}
	f, err := excelize.OpenReader(bytes.NewReader(b))
	if err != nil {
		t.Fatal(err)
	}
	rows, err := f.GetRows("Master Sales")
	if err != nil {
		t.Fatal(err)
	}
	// kop (judul, tenant, 1 meta) + baris kosong + header + 2 data
	if len(rows) != 7 {
		t.Fatalf("baris = %d, mau 7: %v", len(rows), rows)
	}
	if got := rows[5]; got[0] != "1" || got[2] != "Budi" || got[4] != "catatan panjang" {
		t.Fatalf("baris data: %v", got)
	}
	// kolom hanya-Excel (Catatan) tetap ada di header
	if h := rows[4]; h[4] != "Catatan" {
		t.Fatalf("header: %v", h)
	}
}

func TestMasterTablePDF(t *testing.T) {
	tb := sampleMasterTable()
	for i := 0; i < 200; i++ { // paksa lebih dari satu halaman
		tb.Rows = append(tb.Rows, []string{"X", "Nama sangat panjang sekali sampai harus dipotong agar muat di sel", "1.00", ""})
	}
	b, err := tb.ToPDF()
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.HasPrefix(b, []byte("%PDF")) {
		t.Fatal("bukan PDF")
	}
}

func TestGroupThousands(t *testing.T) {
	cases := map[string]string{
		"0": "0", "999": "999", "1000": "1.000", "1234567.00": "1.234.567",
		"1234.50": "1.234,5", "-1500.00": "-1.500", "2.50": "2,5",
	}
	for in, want := range cases {
		if got := groupThousands(in); got != want {
			t.Errorf("groupThousands(%q) = %q, mau %q", in, got, want)
		}
	}
}

func TestMasterParamsNormalize(t *testing.T) {
	if _, _, _, _, err := (MasterParams{Status: "x"}).normalize(); err == nil {
		t.Fatal("status tak dikenal harus ditolak")
	}
	if _, _, _, _, err := (MasterParams{Cursor: "bukan-uuid"}).normalize(); err == nil {
		t.Fatal("cursor non-uuid harus ditolak")
	}
	limit, q, _, _, err := (MasterParams{Limit: 99999, Q: " 50%_A "}).normalize()
	if err != nil || limit != maxMasterLimit {
		t.Fatalf("limit=%d err=%v", limit, err)
	}
	if q != `50\%\_a` {
		t.Fatalf("wildcard harus di-escape: %q", q)
	}
}
