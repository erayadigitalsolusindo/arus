package item

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/google/uuid"
)

func TestSearchPatterns(t *testing.T) {
	got := searchPatterns("  Lip   WARDAH lip 50%_\\ ")
	want := []string{"%lip%", "%wardah%", `%50\%\_\\%`}
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("searchPatterns = %q, ingin %q", got, want)
	}
	if n, id, ok := decodeCursor(encodeCursor("kopi susu", [16]byte{1})); !ok || n != "kopi susu" || id != ([16]byte{1}) {
		t.Fatalf("cursor bolak-balik gagal: %q %v %v", n, id, ok)
	}
	for _, c := range []string{"!!", "YWJj", encodeCursor("x", [16]byte{})[:10]} {
		if _, _, ok := decodeCursor(c); ok {
			t.Errorf("cursor %q seharusnya ditolak", c)
		}
	}
}

func TestSearch(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	unit := e.master(t, "units", e.a.TenantID, "Pcs", true)
	unitB := e.master(t, "units", e.b.TenantID, "Pcs", true)

	mk := func(sku, barcode, name, price string) uuid.UUID {
		t.Helper()
		it, err := e.svc.Create(ctx, e.a, Input{SKU: sku, Barcode: barcode, Name: name, UnitID: unit.String(), SellPrice: price})
		if err != nil {
			t.Fatal(err)
		}
		return it.ID
	}
	lip := mk("WRD-1", "8991", "Wardah Lip Cream Rose", "45000")
	mk("WRD-10", "", "Wardah Bedak Padat", "60000")
	mk("EMN-1", "", "Emina Lip Tint", "30000")
	mk("PCT-1", "", "Diskon 50% Pack", "5000")
	off := mk("WRD-2", "", "Wardah Lip Matte", "40000")
	if _, err := e.svc.SetActive(ctx, e.a, off, false); err != nil {
		t.Fatal(err)
	}
	// Tenant lain dengan nama yang sama tidak boleh ikut.
	if _, err := e.svc.Create(ctx, e.b, Input{SKU: "WRD-1", Name: "Wardah Lip Cream Rose", UnitID: unitB.String(), SellPrice: "1"}); err != nil {
		t.Fatal(err)
	}

	names := func(rows []Row) []string {
		out := []string{}
		for _, r := range rows {
			out = append(out, r.Name)
		}
		return out
	}
	cases := map[string][]string{
		"":            {"Diskon 50% Pack", "Emina Lip Tint", "Wardah Bedak Padat", "Wardah Lip Cream Rose"},
		"lip wardah":  {"Wardah Lip Cream Rose"}, // urutan kata bebas, semua kata wajib
		"LIP":         {"Emina Lip Tint", "Wardah Lip Cream Rose"},
		"8991":        {"Wardah Lip Cream Rose"}, // barcode
		"emn":         {"Emina Lip Tint"},        // kode
		"50%":         {"Diskon 50% Pack"},       // % dicocokkan apa adanya
		"5_%":         {},
		"wardah zzzz": {},
	}
	for q, want := range cases {
		page, err := e.svc.Search(ctx, e.a, SearchParams{Q: q})
		if err != nil {
			t.Fatalf("%q: %v", q, err)
		}
		if fmt.Sprint(names(page.Data)) != fmt.Sprint(want) {
			t.Errorf("%q: dapat %v, ingin %v", q, names(page.Data), want)
		}
	}

	// Harga, stok, dan kolom lain terisi seperti List.
	page, _ := e.svc.Search(ctx, e.a, SearchParams{Q: "rose"})
	if len(page.Data) != 1 || page.Data[0].ID != lip || page.Data[0].Price != "45000.00" || page.Data[0].Unit != "Pcs" || page.Data[0].Stock.Display != "0" {
		t.Fatalf("baris hasil cari: %+v", page.Data)
	}

	// Kode persis: WRD-1 juga cocok sebagian dengan WRD-10, tetapi Exact hanya WRD-1 (tanpa membedakan huruf).
	page, _ = e.svc.Search(ctx, e.a, SearchParams{Q: "wrd-1"})
	if len(page.Data) != 2 || len(page.Exact) != 1 || page.Exact[0].ID != lip {
		t.Fatalf("kode persis: data=%v exact=%v", names(page.Data), names(page.Exact))
	}
	if page, _ = e.svc.Search(ctx, e.a, SearchParams{Q: "8991"}); len(page.Exact) != 1 {
		t.Errorf("barcode persis: %v", names(page.Exact))
	}
	if page, _ = e.svc.Search(ctx, e.a, SearchParams{Q: "WRD-2"}); len(page.Exact) != 0 {
		t.Errorf("barang nonaktif tidak boleh muncul sebagai kode persis")
	}

	// Filter kategori: hanya barang kategori itu (kata cari tetap berlaku); id bukan uuid ditolak.
	cat := e.master(t, "categories", e.a.TenantID, "Lip", true)
	if _, err := e.admin.Exec(ctx, `UPDATE items SET category_id = $1 WHERE id = $2`, cat, lip); err != nil {
		t.Fatal(err)
	}
	page, err := e.svc.Search(ctx, e.a, SearchParams{CategoryID: cat.String()})
	if err != nil || len(page.Data) != 1 || page.Data[0].ID != lip {
		t.Fatalf("filter kategori: %v err=%v", names(page.Data), err)
	}
	if page, _ = e.svc.Search(ctx, e.a, SearchParams{Q: "emina", CategoryID: cat.String()}); len(page.Data) != 0 {
		t.Errorf("kategori + kata: %v", names(page.Data))
	}
	var cfe FieldErrors
	if _, err := e.svc.Search(ctx, e.a, SearchParams{CategoryID: "bukan-uuid"}); !errors.As(err, &cfe) || cfe["category_id"] == "" {
		t.Errorf("category_id rusak: %v", err)
	}

	// Keyset: 2 + 2 tanpa duplikat, lalu habis.
	seen := map[string]bool{}
	cursor := ""
	for range 3 {
		page, err := e.svc.Search(ctx, e.a, SearchParams{Cursor: cursor, Limit: 2})
		if err != nil {
			t.Fatal(err)
		}
		for _, r := range page.Data {
			if seen[r.Name] {
				t.Fatalf("duplikat antar halaman: %s", r.Name)
			}
			seen[r.Name] = true
		}
		if len(page.Exact) != 0 {
			t.Error("Exact hanya di halaman pertama dengan kata")
		}
		cursor = page.NextCursor
		if cursor == "" {
			break
		}
	}
	if len(seen) != 4 || cursor != "" {
		t.Fatalf("keyset: %v sisa cursor %q", seen, cursor)
	}

	var fe FieldErrors
	if _, err := e.svc.Search(ctx, e.a, SearchParams{Cursor: "rusak!"}); !errors.As(err, &fe) || fe["cursor"] == "" {
		t.Errorf("cursor rusak: %v", err)
	}
	if _, err := e.svc.Search(ctx, e.a, SearchParams{Q: "a b c d e f g h i"}); !errors.As(err, &fe) || fe["q"] == "" {
		t.Errorf("lebih dari 8 kata: %v", err)
	}

	// Fungsi pencari tanpa tenant di transaksi = kosong (fail closed), walau berjalan sebagai pemilik tabel.
	var n int
	if err := e.svc.pool.QueryRow(ctx, `SELECT count(*) FROM item_search('{}', NULL, NULL, NULL, 50)`).Scan(&n); err != nil || n != 0 {
		t.Errorf("tanpa tenant: n=%d err=%v", n, err)
	}
	if err := e.svc.pool.QueryRow(ctx, `SELECT count(*) FROM item_find_exact('WRD-1', NULL)`).Scan(&n); err != nil || n != 0 {
		t.Errorf("exact tanpa tenant: n=%d err=%v", n, err)
	}
}
