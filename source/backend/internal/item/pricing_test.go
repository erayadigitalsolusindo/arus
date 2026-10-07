package item

import (
	"errors"
	"testing"

	"github.com/shopspring/decimal"
)

func d(s string) decimal.Decimal { return decimal.RequireFromString(s) }

func tiers(pairs ...string) []Tier {
	out := []Tier{}
	for i := 0; i+1 < len(pairs); i += 2 {
		out = append(out, Tier{MinQty: d(pairs[i]), Price: d(pairs[i+1])})
	}
	return out
}

// Contoh dari permintaan produk: "Beli 2–5 harga 9.500; 6–10 harga 8.500; 11 ke atas harga 8.300".
func TestPriceForSpecExample(t *testing.T) {
	ts := tiers("2", "9500", "6", "8500", "11", "8300")
	base := d("10000")
	for qty, want := range map[string]string{
		"1": "10000", // di bawah tier pertama = harga biasa
		"2": "9500", "3": "9500", "5": "9500",
		"6": "8500", "10": "8500",
		"11": "8300", "12": "8300", "100": "8300", "99999": "8300", // tier terakhir berlaku sampai stok habis
	} {
		got, err := PriceFor(base, ts, d(qty))
		if err != nil || !got.Equal(d(want)) {
			t.Errorf("qty %s: harga = %s (err %v), want %s", qty, got, err, want)
		}
	}
}

func TestPriceForEdges(t *testing.T) {
	base := d("10000")
	if got, _ := PriceFor(base, nil, d("50")); !got.Equal(base) {
		t.Errorf("tanpa tier = %s, want harga biasa", got)
	}
	// Jumlah pecahan (barang timbang) dan batas tepat.
	ts := tiers("0.5", "9000", "5", "8000")
	for qty, want := range map[string]string{"0.25": "10000", "0.5": "9000", "4.999": "9000", "5": "8000", "5.001": "8000"} {
		if got, _ := PriceFor(base, ts, d(qty)); !got.Equal(d(want)) {
			t.Errorf("qty %s: %s, want %s", qty, got, want)
		}
	}
	for _, q := range []string{"0", "-1"} {
		if _, err := PriceFor(base, ts, d(q)); !errors.Is(err, ErrBadQty) {
			t.Errorf("qty %s harus ditolak: %v", q, err)
		}
	}
	// Harga tier boleh 0 (gratis) — dan tidak boleh dianggap "tidak ada tier".
	if got, _ := PriceFor(base, tiers("10", "0"), d("10")); !got.IsZero() {
		t.Errorf("tier harga 0 = %s", got)
	}
}

func TestChooseTiers(t *testing.T) {
	def, own := tiers("2", "9500"), tiers("3", "9000")
	if got := ChooseTiers(def, own); len(got) != 1 || !got[0].Price.Equal(d("9000")) {
		t.Errorf("set cabang menggantikan default sepenuhnya: %+v", got)
	}
	if got := ChooseTiers(def, nil); !got[0].Price.Equal(d("9500")) {
		t.Errorf("tanpa set cabang → default: %+v", got)
	}
	if got := ChooseTiers(nil, nil); len(got) != 0 {
		t.Errorf("tanpa keduanya: %+v", got)
	}
}

func TestNormalizeTiers(t *testing.T) {
	got, code := normalizeTiers(tiers("11", "8300", "2", "9500", "6", "8500")) // tak terurut → diurutkan
	if code != "" || got[0].MinQty.String() != "2" || got[2].MinQty.String() != "11" {
		t.Fatalf("urut: %+v %q", got, code)
	}
	if _, code := normalizeTiers(tiers("2", "9500", "2", "9000")); code != codeDuplicate {
		t.Errorf("MinQty ganda: %q", code)
	}
	if _, code := normalizeTiers(tiers("2", "9500", "6", "9600")); code != codeNotDecreasing {
		t.Errorf("harga naik: %q", code)
	}
	if _, code := normalizeTiers(tiers("2", "9500", "6", "9500")); code != "" {
		t.Errorf("harga sama pada tier lebih besar boleh: %q", code)
	}
	many := []Tier{}
	for i := 1; i <= maxTiers+1; i++ {
		many = append(many, Tier{MinQty: decimal.NewFromInt(int64(i)), Price: decimal.NewFromInt(int64(1000 - i))})
	}
	if _, code := normalizeTiers(many); code != codeTooMany {
		t.Errorf("terlalu banyak: %q", code)
	}
	if got, code := normalizeTiers(nil); code != "" || len(got) != 0 {
		t.Errorf("kosong: %+v %q", got, code)
	}
}
