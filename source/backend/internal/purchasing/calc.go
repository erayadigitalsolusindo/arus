package purchasing

import "github.com/shopspring/decimal"

type dec = decimal.Decimal

var (
	hundred  = decimal.NewFromInt(100)
	maxLine  = decimal.New(1, 13) // batas nilai satu baris / total nota (kolom numeric(18,2) jauh di atas ini)
	maxTotal = decimal.New(1, 14)
)

// lineTotal = qty × harga, lalu tiap tingkat diskon berurutan (bertingkat, bukan dijumlah): nilai < 100 = persen
// (v × (100 − d)/100), nilai ≥ 100 = rupiah yang dipotong dari nilai baris saat itu. Dibulatkan 2 desimal sekali di
// akhir. ok=false bila potongan rupiah melebihi nilai baris (hasil negatif).
func lineTotal(qty, price dec, disc [4]dec) (dec, bool) {
	v := qty.Mul(price)
	for _, d := range disc {
		switch {
		case !d.IsPositive():
		case d.LessThan(hundred):
			v = v.Mul(hundred.Sub(d)).Shift(-2)
		default:
			v = v.Sub(d)
			if v.IsNegative() {
				return decimal.Zero, false
			}
		}
	}
	return v.Round(2), true
}

// allocate membagi `cost` ke baris-baris secara proporsional bobot (nilai baris; bila semua nol, qty). Jumlah hasil
// SELALU tepat sama dengan cost: semua baris kecuali terakhir dipotong (truncate) ke 2 desimal, baris terakhir
// mengambil sisa (tidak pernah negatif).
func allocate(cost dec, weights, qtys []dec) []dec {
	out := make([]dec, len(weights))
	if len(weights) == 0 || !cost.IsPositive() {
		return out
	}
	w := weights
	total := decimal.Zero
	for _, x := range w {
		total = total.Add(x)
	}
	if !total.IsPositive() {
		w, total = qtys, decimal.Zero
		for _, x := range w {
			total = total.Add(x)
		}
	}
	used := decimal.Zero
	for i := 0; i < len(w)-1; i++ {
		out[i] = cost.Mul(w[i]).Div(total).Truncate(2)
		used = used.Add(out[i])
	}
	out[len(w)-1] = cost.Sub(used)
	return out
}

// weightedAvg = HPP rata-rata tertimbang baru untuk satu cabang (keputusan 2026-10-09):
//
//	(stok_sebelum × HPP lama + qty × HPP baris) ÷ (stok_sebelum + qty)
//
// stok_sebelum = stok cabang (semua bucket) tepat SEBELUM penerimaan ini. Bila stok sebelum ≤ 0 atau HPP lama kosong
// (belum ada nilai yang bisa dirata-ratakan), HPP baru = HPP baris.
func weightedAvg(stockBefore, avgBefore, qty, unitCost dec) dec {
	if !stockBefore.IsPositive() || !avgBefore.IsPositive() {
		return unitCost
	}
	return stockBefore.Mul(avgBefore).Add(qty.Mul(unitCost)).Div(stockBefore.Add(qty)).Round(2)
}
