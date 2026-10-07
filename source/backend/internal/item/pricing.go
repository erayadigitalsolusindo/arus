package item

import (
	"errors"
	"sort"

	"github.com/shopspring/decimal"
)

// Tier = satu anak tangga harga grosir: mulai MinQty (dalam satuan DASAR item) harga per satuan = Price.
// Harga tier berlaku untuk seluruh jumlah (bukan potongan bertingkat). Batas atas tier = tier berikutnya.
type Tier struct {
	MinQty decimal.Decimal `json:"min_qty"`
	Price  decimal.Decimal `json:"price"`
}

var ErrBadQty = errors.New("jumlah harus lebih dari nol")

// PriceFor mengembalikan harga per satuan dasar untuk jumlah qty (satuan dasar). tiers WAJIB terurut naik menurut
// MinQty (urutan yang disimpan/dikembalikan service). Di bawah tier pertama (atau tanpa tier) berlaku `base`.
// Contoh (base 10.000; tier 2→9.500, 6→8.500, 11→8.300): qty 1 → 10.000; 2–5 → 9.500; 6–10 → 8.500; ≥ 11 → 8.300.
// Pemanggil memilih set tier lebih dulu lewat ChooseTiers (set cabang menggantikan set default sepenuhnya).
func PriceFor(base decimal.Decimal, tiers []Tier, qty decimal.Decimal) (decimal.Decimal, error) {
	if !qty.IsPositive() {
		return decimal.Zero, ErrBadQty
	}
	price := base
	for _, t := range tiers {
		if qty.LessThan(t.MinQty) {
			break
		}
		price = t.Price
	}
	return price, nil
}

// ChooseTiers memilih set tier yang berlaku di satu cabang: set cabang bila ada (tidak kosong), selain itu default.
func ChooseTiers(def []Tier, outlet []Tier) []Tier {
	if len(outlet) > 0 {
		return outlet
	}
	return def
}

// Batas validasi satu set tier.
const maxTiers = 10

// Kode galat set tier (diterjemahkan klien sebagai errors.FIELD_<kode>).
const (
	codeTooMany       = "TOO_MANY"
	codeDuplicate     = "DUPLICATE"
	codeNotDecreasing = "NOT_DECREASING"
)

// normalizeTiers mengurutkan naik dan memvalidasi: tanpa MinQty ganda, dan harga tidak boleh naik pada tier yang
// lebih besar (membeli lebih banyak tidak boleh lebih mahal per satuan). Mengembalikan kode galat atau "".
func normalizeTiers(in []Tier) ([]Tier, string) {
	if len(in) > maxTiers {
		return nil, codeTooMany
	}
	out := append([]Tier(nil), in...)
	sort.Slice(out, func(i, j int) bool { return out[i].MinQty.LessThan(out[j].MinQty) })
	for i := 1; i < len(out); i++ {
		switch {
		case out[i].MinQty.Equal(out[i-1].MinQty):
			return nil, codeDuplicate
		case out[i].Price.GreaterThan(out[i-1].Price):
			return nil, codeNotDecreasing
		}
	}
	return out, ""
}
