package member

import (
	"testing"

	"github.com/shopspring/decimal"
)

func TestReturnedPointsAccumulateExactly(t *testing.T) {
	saleValue := decimal.NewFromInt(100)
	first := ReturnedPoints(7, decimal.NewFromInt(33), saleValue, 0)
	second := ReturnedPoints(7, decimal.NewFromInt(66), saleValue, first)
	last := ReturnedPoints(7, saleValue, saleValue, first+second)
	if first != 2 || second != 2 || last != 3 || first+second+last != 7 {
		t.Fatalf("partial return point adjustments = %d, %d, %d; want 2, 2, 3", first, second, last)
	}
}

func TestReturnedPointsBounds(t *testing.T) {
	value := decimal.NewFromInt(100)
	if got := ReturnedPoints(8, decimal.NewFromInt(150), value, 3); got != 5 {
		t.Fatalf("over-range returned value adjustment = %d; want remaining 5", got)
	}
	if got := ReturnedPoints(8, decimal.NewFromInt(20), value, 3); got != 0 {
		t.Fatalf("adjustment behind already-adjusted target = %d; want 0", got)
	}
	if got := ReturnedPoints(8, decimal.NewFromInt(20), decimal.Zero, 0); got != 0 {
		t.Fatalf("zero sale value adjustment = %d; want 0", got)
	}
}
