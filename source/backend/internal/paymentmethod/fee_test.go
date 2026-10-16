package paymentmethod

import (
	"testing"

	"github.com/shopspring/decimal"
)

func d(s string) decimal.Decimal { return decimal.RequireFromString(s) }

// Aturan transaksi kecil (BI): <= batas -> MDR flat menggantikan persen + nominal; di atas batas -> tarif biasa.
func TestRuleCompute(t *testing.T) {
	r := Rule{Pct: d("0.7"), Flat: d("100"), SmallLimit: d("100000"), SmallFlat: d("500")}
	for _, c := range []struct {
		name, amount, fee, pct, flat string
	}{
		{"jauh di bawah batas", "10000", "500.00", "0", "500"},
		{"tepat di batas", "100000", "500.00", "0", "500"},
		{"sedikit di atas batas", "100000.01", "800.00", "0.7", "100"}, // 100000.01 × 0,7% = 700.00007 + 100, dibulatkan
		{"besar", "1000000", "7100.00", "0.7", "100"},
	} {
		fee, pct, flat := r.Compute(d(c.amount))
		if !fee.Equal(d(c.fee)) || !pct.Equal(d(c.pct)) || !flat.Equal(d(c.flat)) {
			t.Errorf("%s: fee=%s pct=%s flat=%s, ingin %s/%s/%s", c.name, fee, pct, flat, c.fee, c.pct, c.flat)
		}
	}
	// Aturan nonaktif (limit 0): tarif biasa, termasuk jumlah kecil.
	off := Rule{Pct: d("0.7"), Flat: d("100"), SmallFlat: d("500")}
	if fee, _, _ := off.Compute(d("1000")); !fee.Equal(d("107")) {
		t.Errorf("nonaktif: fee=%s, ingin 107", fee)
	}
	// MDR flat 0 di bawah batas = gratis; tanpa tarif sama sekali = 0.
	free := Rule{Pct: d("0.7"), SmallLimit: d("50000")}
	if fee, _, _ := free.Compute(d("20000")); !fee.IsZero() {
		t.Errorf("gratis di bawah batas: fee=%s", fee)
	}
	if fee, _, _ := (Rule{}).Compute(d("20000")); !fee.IsZero() {
		t.Errorf("tanpa tarif: fee=%s", fee)
	}
}

func TestSmallTx(t *testing.T) {
	for _, c := range []struct {
		name string
		in   Input
		kind string
		want string
	}{
		{"nonaktif", Input{}, KindEWallet, ""},
		{"aktif", Input{SmallTxLimit: "100000", SmallTxFlat: "500"}, KindEWallet, ""},
		{"gratis di bawah batas", Input{SmallTxLimit: "100000"}, KindDebit, ""},
		{"flat tanpa batas", Input{SmallTxFlat: "500"}, KindDebit, "small_tx_limit"},
		{"batas negatif", Input{SmallTxLimit: "-1", SmallTxFlat: "5"}, KindDebit, "small_tx_limit"},
		{"flat teks", Input{SmallTxLimit: "1000", SmallTxFlat: "abc"}, KindDebit, "small_tx_flat"},
		{"tunai tak boleh", Input{SmallTxLimit: "1000", SmallTxFlat: "5"}, KindCash, "small_tx_limit"},
	} {
		f := FieldErrors{}
		smallTx(c.in, c.kind, f)
		if c.want == "" && len(f) > 0 || c.want != "" && f[c.want] == "" {
			t.Errorf("%s: dapat %v, ingin galat %q", c.name, f, c.want)
		}
	}
}
