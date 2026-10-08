package voucher

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/shopspring/decimal"

	gen "aciraba/internal/gen"
)

// Kode galat per kupon (field `voucher_codes.<n>` pada permintaan penjualan).
const (
	CodeUnknown    = "VOUCHER_UNKNOWN"
	CodeInactive   = "VOUCHER_INACTIVE"
	CodeNotStarted = "VOUCHER_NOT_STARTED"
	CodeExpired    = "VOUCHER_EXPIRED"
	CodeExhausted  = "VOUCHER_EXHAUSTED"
	CodeMinSpend   = "VOUCHER_MIN_SPEND"
	CodeEmpty      = "VOUCHER_EMPTY"
)

var ErrExhausted = errors.New("kuota kupon habis")

// Rule = kupon yang dimuat untuk dievaluasi pada sebuah nota.
type Rule struct {
	ID          uuid.UUID
	Code, Name  string
	Kind        string
	Value       decimal.Decimal
	MaxDiscount decimal.NullDecimal
	MinSpend    decimal.Decimal
	StartsOn    pgtype.Date
	EndsOn      pgtype.Date
	MaxUses     pgtype.Int4
	Used        int32
	Active      bool
}

// Applied = kupon yang lolos beserta potongannya.
type Applied struct {
	Rule
	Amount decimal.Decimal
}

// Load memuat kupon menurut kode (sudah dinormalkan). lock=true mengunci barisnya (terurut id) sampai transaksi
// selesai, sehingga pemeriksaan kuota dan Consume tak bisa disalip nota lain.
func Load(ctx context.Context, tx pgx.Tx, tenantID uuid.UUID, codes []string, lock bool) (map[string]Rule, error) {
	out := make(map[string]Rule, len(codes))
	q := gen.New(tx)
	if lock {
		rows, err := q.VoucherByCodesLock(ctx, gen.VoucherByCodesLockParams{TenantID: tenantID, Codes: codes})
		if err != nil {
			return nil, err
		}
		for _, r := range rows {
			out[r.Code] = Rule{r.ID, r.Code, r.Name, r.Kind, r.Value, numNull(r.MaxDiscount), r.MinSpend, r.StartsOn, r.EndsOn, r.MaxUses, r.UsedCount, r.Active}
		}
		return out, nil
	}
	rows, err := q.VoucherByCodes(ctx, gen.VoucherByCodesParams{TenantID: tenantID, Codes: codes})
	if err != nil {
		return nil, err
	}
	for _, r := range rows {
		out[r.Code] = Rule{r.ID, r.Code, r.Name, r.Kind, r.Value, numNull(r.MaxDiscount), r.MinSpend, r.StartsOn, r.EndsOn, r.MaxUses, r.UsedCount, r.Active}
	}
	return out, nil
}

var hundred = decimal.NewFromInt(100)

// AmountFor = potongan satu kupon atas `base` (subtotal setelah potongan baris dan potongan global). Persen dibulatkan
// 2 desimal lalu dibatasi max_discount; nominal apa adanya. Setiap kupon dihitung dari basis yang SAMA (tidak
// bertingkat), jadi urutan kupon tidak mengubah hasil.
func AmountFor(r Rule, base decimal.Decimal) decimal.Decimal {
	if r.Kind == KindAmount {
		return r.Value
	}
	d := base.Mul(r.Value).Div(hundred).Round(2)
	if r.MaxDiscount.Valid && d.GreaterThan(r.MaxDiscount.Decimal) {
		d = r.MaxDiscount.Decimal
	}
	return d
}

// Evaluate memeriksa tiap kode (urutan sesuai masukan) pada hari `today` (menurut zona waktu outlet). Hasil: kupon yang
// lolos, dan peta indeks→kode galat untuk yang tidak lolos. Total potongan vs basis diperiksa pemanggil.
func Evaluate(codes []string, rules map[string]Rule, base decimal.Decimal, today pgtype.Date) ([]Applied, map[int]string) {
	var ok []Applied
	bad := map[int]string{}
	for i, c := range codes {
		r, found := rules[c]
		switch {
		case !found:
			bad[i] = CodeUnknown
		case !r.Active:
			bad[i] = CodeInactive
		case r.StartsOn.Valid && today.Time.Before(r.StartsOn.Time):
			bad[i] = CodeNotStarted
		case r.EndsOn.Valid && today.Time.After(r.EndsOn.Time):
			bad[i] = CodeExpired
		case r.MaxUses.Valid && r.Used >= r.MaxUses.Int32:
			bad[i] = CodeExhausted
		case base.LessThan(r.MinSpend):
			bad[i] = CodeMinSpend
		default:
			amt := AmountFor(r, base)
			if !amt.IsPositive() {
				bad[i] = CodeEmpty
				continue
			}
			ok = append(ok, Applied{Rule: r, Amount: amt})
		}
	}
	return ok, bad
}

// Consume menambah hitungan pemakaian satu kupon di dalam transaksi nota. Pemanggil sudah mengunci barisnya lewat
// Load(lock=true); penjaga `used_count < max_uses` tetap dipasang di SQL sebagai pengaman terakhir.
func Consume(ctx context.Context, tx pgx.Tx, tenantID, id uuid.UUID) error {
	n, err := gen.New(tx).VoucherUse(ctx, gen.VoucherUseParams{TenantID: tenantID, ID: id})
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrExhausted
	}
	return nil
}

func numNull(n pgtype.Numeric) decimal.NullDecimal {
	if !n.Valid || n.NaN || n.InfinityModifier != pgtype.Finite {
		return decimal.NullDecimal{}
	}
	return decimal.NullDecimal{Decimal: decimal.NewFromBigInt(n.Int, n.Exp), Valid: true}
}

// Release mengembalikan satu pemakaian kupon ketika nota yang memakainya dibatalkan atau diedit (kebalikan Consume).
func Release(ctx context.Context, tx pgx.Tx, tenantID, id uuid.UUID) error {
	_, err := gen.New(tx).VoucherRelease(ctx, gen.VoucherReleaseParams{TenantID: tenantID, ID: id})
	return err
}
