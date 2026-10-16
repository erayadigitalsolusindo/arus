package paymentmethod

import "github.com/shopspring/decimal"

// Rule = tarif biaya (MDR) satu metode. SmallLimit = 0 berarti aturan transaksi kecil nonaktif.
type Rule struct {
	Pct, Flat, SmallLimit, SmallFlat decimal.Decimal
}

// Compute = biaya atas jumlah yang dibayar, dibulatkan 2 desimal, plus tarif efektif yang dipakai (untuk snapshot nota).
// Aturan transaksi kecil: jumlah <= SmallLimit -> biaya = SmallFlat dan menggantikan persen + nominal biasa.
// Di atas batas: amount × Pct% + Flat.
func (r Rule) Compute(amount decimal.Decimal) (fee, pct, flat decimal.Decimal) {
	if r.SmallLimit.IsPositive() && !amount.GreaterThan(r.SmallLimit) {
		return r.SmallFlat.Round(2), decimal.Zero, r.SmallFlat
	}
	if !r.Pct.IsPositive() && !r.Flat.IsPositive() {
		return decimal.Zero, r.Pct, r.Flat
	}
	return amount.Mul(r.Pct).Div(decimal.NewFromInt(100)).Add(r.Flat).Round(2), r.Pct, r.Flat
}
