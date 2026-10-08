package sales

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/shopspring/decimal"

	"aciraba/internal/authz"
	"aciraba/internal/voucher"
)

// voucherCalc = kupon yang lolos pada sebuah nota (kosong bila tanpa kupon).
type voucherCalc struct{ applied []voucher.Applied }

func (v voucherCalc) total() dec {
	sum := decimal.Zero
	for _, a := range v.applied {
		sum = sum.Add(a.Amount)
	}
	return sum
}

func (v voucherCalc) codes() []string {
	out := make([]string, 0, len(v.applied))
	for _, a := range v.applied {
		out = append(out, a.Code)
	}
	return out
}

func (v voucherCalc) infos() []VoucherInfo {
	out := make([]VoucherInfo, 0, len(v.applied))
	for _, a := range v.applied {
		out = append(out, VoucherInfo{Code: a.Code, Name: a.Name, Kind: a.Kind, Value: a.Value.StringFixed(2), Amount: a.Amount.StringFixed(2)})
	}
	return out
}

// costFloor = jumlah HPP barang yang tidak boleh dijual rugi. Potongan yang tidak memakai persetujuan PIN (tukar poin,
// kupon) tidak boleh membuat nota di bawah angka ini.
func costFloor(lines []calcLine) dec {
	floor := decimal.Zero
	for _, l := range lines {
		if !l.item.row.SellBelowCost {
			floor = floor.Add(l.unitCost.Mul(l.qty).Round(2))
		}
	}
	return floor
}

// resolveVouchers memeriksa kupon dan menambahkan potongannya ke potongan global nota (norm yang dikembalikan sudah
// memuatnya), lalu menghitung ulang. Basis = subtotal − potongan global manual; tiap kupon dihitung dari basis yang
// sama. Kupon dikunci bila lock=true (simpan nota) agar kuota tak terlampaui nota lain. Galat per kupon:
// voucher_codes.<n> = kode VOUCHER_*; potongan melebihi basis atau membuat nota di bawah HPP → voucher_codes.
func resolveVouchers(ctx context.Context, tx pgx.Tx, a authz.Actor, n norm, localDay pgtype.Date, lock bool, infos map[uuid.UUID]*itemInfo,
	taxStorePct, taxGovPct dec, lines []calcLine, t totals) (voucherCalc, norm, []calcLine, totals, FieldErrors, error) {
	if len(n.vouchers) == 0 {
		return voucherCalc{}, n, lines, t, nil, nil
	}
	rules, err := voucher.Load(ctx, tx, a.TenantID, n.vouchers, lock)
	if err != nil {
		return voucherCalc{}, n, lines, t, nil, err
	}
	base := t.subtotal.Sub(t.discount)
	applied, bad := voucher.Evaluate(n.vouchers, rules, base, localDay)
	if len(bad) > 0 {
		fe := FieldErrors{}
		for i, c := range bad {
			fe[fmt.Sprintf("voucher_codes.%d", i)] = c
		}
		return voucherCalc{}, n, lines, t, fe, nil
	}
	vc := voucherCalc{applied: applied}
	if vc.total().GreaterThan(base) {
		return vc, n, lines, t, FieldErrors{"voucher_codes": codeVoucherTooHigh}, nil
	}
	n.discount = n.discount.Add(vc.total())
	var fe FieldErrors
	if lines, t, fe = price(n, infos, taxStorePct, taxGovPct); len(fe) > 0 {
		return vc, n, nil, t, fe, nil
	}
	if t.subtotal.Sub(t.discount).LessThan(costFloor(lines)) {
		return vc, n, lines, t, FieldErrors{"voucher_codes": codeVoucherBelowCost}, nil
	}
	return vc, n, lines, t, nil, nil
}
