package purchasing

import (
	"testing"

	"github.com/shopspring/decimal"
)

func d(s string) dec { return decimal.RequireFromString(s) }

func TestLineTotalTieredDiscounts(t *testing.T) {
	zero := [4]dec{}
	if got := lt1(d("10"), d("1000"), zero); !got.Equal(d("10000")) {
		t.Errorf("tanpa diskon = %s", got)
	}
	// 10% lalu 5% bertingkat: 10.000 × 0,9 × 0,95 = 8.550 (bukan 8.500 jika dijumlah).
	if got := lt1(d("10"), d("1000"), [4]dec{d("10"), d("5"), {}, {}}); !got.Equal(d("8550")) {
		t.Errorf("2 tingkat = %s", got)
	}
	// 4 tingkat: 100.000 × 0,9 × 0,95 × 0,98 × 0,99 = 82.952,10 (dibulatkan sekali di akhir).
	if got := lt1(d("1"), d("100000"), [4]dec{d("10"), d("5"), d("2"), d("1")}); !got.Equal(d("82952.10")) {
		t.Errorf("4 tingkat = %s", got)
	}
	// Pembulatan 2 desimal di akhir.
	if got := lt1(d("3"), d("33.333"), zero); !got.Equal(d("100.00")) {
		t.Errorf("pembulatan = %s", got)
	}
}

func TestAllocateSumsExactly(t *testing.T) {
	sum := func(xs []dec) dec {
		s := decimal.Zero
		for _, x := range xs {
			s = s.Add(x)
		}
		return s
	}
	cases := []struct {
		cost string
		w    []string
	}{
		{"10000", []string{"60000", "40000"}},
		{"0.02", []string{"1", "1", "1", "1"}}, // kasus yang bisa melebihi bila dibulatkan ke atas
		{"1", []string{"1", "1", "1"}},
		{"12345.67", []string{"333.33", "666.67", "1000", "0.01"}},
		{"500", []string{"0", "0"}}, // semua bobot nol → pakai qty
	}
	for _, c := range cases {
		var w, q []dec
		for _, x := range c.w {
			w = append(w, d(x))
			q = append(q, d("1"))
		}
		got := allocate(d(c.cost), w, q)
		if !sum(got).Equal(d(c.cost)) {
			t.Errorf("%s %v: jumlah alokasi %s ≠ biaya", c.cost, c.w, sum(got))
		}
		for _, x := range got {
			if x.IsNegative() {
				t.Errorf("%s %v: alokasi negatif %s", c.cost, c.w, x)
			}
		}
	}
	// Proporsional: 10.000 untuk nilai 60.000 : 40.000 → 6.000 : 4.000.
	got := allocate(d("10000"), []dec{d("60000"), d("40000")}, []dec{d("1"), d("1")})
	if !got[0].Equal(d("6000")) || !got[1].Equal(d("4000")) {
		t.Errorf("proporsi = %v", got)
	}
	if got := allocate(decimal.Zero, []dec{d("1")}, []dec{d("1")}); !got[0].IsZero() {
		t.Errorf("biaya nol = %v", got)
	}
}

func TestWeightedAvg(t *testing.T) {
	// Contoh produk: stok 10 @ 1.000, beli 10 @ 1.200 → (10.000 + 12.000) ÷ 20 = 1.100.
	if got := weightedAvg(d("10"), d("1000"), d("10"), d("1200")); !got.Equal(d("1100")) {
		t.Errorf("rata-rata = %s", got)
	}
	// Bukti bug legacy: stok sebelum dihitung SEBELUM penerimaan (bukan sesudah). Bila sesudah (20) dipakai sebagai
	// "stok sebelum", hasilnya (20×1000 + 10×1200) ÷ 30 = 1.066,67 — salah.
	if got := weightedAvg(d("10"), d("1000"), d("10"), d("1200")); got.Equal(d("1066.67")) {
		t.Error("memakai stok sesudah penerimaan")
	}
	// Stok sebelum ≤ 0 → HPP baru = HPP baris.
	for _, st := range []string{"0", "-5"} {
		if got := weightedAvg(d(st), d("1000"), d("10"), d("1200")); !got.Equal(d("1200")) {
			t.Errorf("stok %s: %s", st, got)
		}
	}
	// HPP lama kosong → tidak diencerkan oleh nol.
	if got := weightedAvg(d("10"), decimal.Zero, d("10"), d("1200")); !got.Equal(d("1200")) {
		t.Errorf("HPP kosong: %s", got)
	}
	// Pembulatan 2 desimal: (3×100 + 1×101) ÷ 4 = 100,25; (1×100 + 2×101) ÷ 3 = 100,67.
	if got := weightedAvg(d("1"), d("100"), d("2"), d("101")); !got.Equal(d("100.67")) {
		t.Errorf("pembulatan: %s", got)
	}
}

func lt1(q, p dec, disc [4]dec) dec { v, _ := lineTotal(q, p, disc); return v }

func TestLineTotalRupiahDiscount(t *testing.T) {
	if got := lt1(d("10"), d("1000"), [4]dec{d("500"), {}, {}, {}}); !got.Equal(d("9500")) {
		t.Fatalf("rupiah 500: %s", got)
	}
	// persen lalu rupiah: 10000 → 9000 → 8500
	if got := lt1(d("10"), d("1000"), [4]dec{d("10"), d("500"), {}, {}}); !got.Equal(d("8500")) {
		t.Fatalf("persen+rupiah: %s", got)
	}
	if _, ok := lineTotal(d("1"), d("1000"), [4]dec{d("1500"), {}, {}, {}}); ok {
		t.Fatal("potongan melebihi nilai baris harus ditolak")
	}
}

func TestPriceFourDecimals(t *testing.T) {
	if got := lt1(d("3"), d("333.3333"), [4]dec{}); !got.Equal(d("1000.00")) {
		t.Fatalf("3 × 333,3333: %s", got)
	}
	if priceString(d("8000")) != "8000.00" || priceString(d("333.3333")) != "333.3333" || priceString(d("12.5")) != "12.50" {
		t.Fatal("format harga")
	}
}
