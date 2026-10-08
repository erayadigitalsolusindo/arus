package sales

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/google/uuid"

	"aciraba/internal/voucher"
)

// newVoucher membuat kupon. starts/ends berupa ekspresi SQL tanggal ("NULL" atau mis. "current_date - 5").
func (e *env) newVoucher(t *testing.T, code, kind, value, maxDisc, minSpend, starts, ends string, maxUses any, active bool) uuid.UUID {
	t.Helper()
	id := uuid.New()
	if _, err := e.admin.Exec(context.Background(),
		`INSERT INTO vouchers (id, tenant_id, code, name, kind, value, max_discount, min_spend, starts_on, ends_on, max_uses, active)
		 VALUES ($1, $2, $3, 'Kupon '||$3, $4, $5, NULLIF($6,'')::numeric, $7, `+starts+`, `+ends+`, $8, $9)`,
		id, e.tenant, code, kind, value, maxDisc, minSpend, maxUses, active); err != nil {
		t.Fatal(err)
	}
	return id
}

func (e *env) voucherUsed(t *testing.T, id uuid.UUID) (used int) {
	t.Helper()
	if err := e.admin.QueryRow(context.Background(), `SELECT used_count FROM vouchers WHERE id = $1`, id).Scan(&used); err != nil {
		t.Fatal(err)
	}
	return
}

func TestVouchersStackAndPersist(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	a := e.actor(e.tenant)
	it := e.item(t, "goods", "100000", "30000", 20, false)
	pct := e.newVoucher(t, "HEMAT10", "percent", "10", "15000", "0", "current_date - 5", "current_date + 5", nil, true)
	amt := e.newVoucher(t, "POTONG5", "amount", "5000", "", "100000", "NULL", "NULL", nil, true)

	// 2 × 100.000 = 200.000. 10% = 20.000 dibatasi 15.000; + 5.000 = 20.000 → dasar 180.000, pajak 11% = 19.800 → 199.800.
	req := Request{Lines: []LineIn{line(it, "2")}, ApplyTax: true, VoucherCodes: []string{" hemat10 ", "POTONG5"}}
	q, err := e.svc.Quote(ctx, a, req)
	if err != nil || q.Discount != "20000.00" || q.VoucherAmount != "20000.00" || q.Total != "199800.00" || len(q.Vouchers) != 2 {
		t.Fatalf("quote: %+v err=%v", q, err)
	}
	if q.Vouchers[0].Code != "HEMAT10" || q.Vouchers[0].Amount != "15000.00" || q.Vouchers[1].Amount != "5000.00" {
		t.Fatalf("rincian kupon: %+v", q.Vouchers)
	}
	if e.voucherUsed(t, pct) != 0 || e.voucherUsed(t, amt) != 0 {
		t.Fatal("quote tidak boleh memakai kuota")
	}

	req.Payments = []PaymentIn{pay("cash", "199800")}
	s, _, err := e.svc.Create(ctx, a, key(), req)
	if err != nil {
		t.Fatal(err)
	}
	if s.Total != "199800.00" || s.Discount != "20000.00" || len(s.Vouchers) != 2 || s.Vouchers[0].Code != "HEMAT10" || s.Vouchers[1].Amount != "5000.00" {
		t.Fatalf("nota: %+v", s)
	}
	if e.voucherUsed(t, pct) != 1 || e.voucherUsed(t, amt) != 1 {
		t.Fatalf("pemakaian = %d/%d, want 1/1", e.voucherUsed(t, pct), e.voucherUsed(t, amt))
	}
	// Kirim ulang dengan kunci yang sama tidak memakai kuota lagi.
	k := key()
	if _, _, err = e.svc.Create(ctx, a, k, req); err != nil {
		t.Fatal(err)
	}
	if _, rep, err := e.svc.Create(ctx, a, k, req); err != nil || !rep || e.voucherUsed(t, pct) != 2 {
		t.Fatalf("replay: rep=%v err=%v used=%d", rep, err, e.voucherUsed(t, pct))
	}
}

func TestVoucherRejections(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	a := e.actor(e.tenant)
	it := e.item(t, "goods", "100000", "90000", 20, false)
	e.newVoucher(t, "SOON", "amount", "1000", "", "0", "current_date + 5", "NULL", nil, true)
	e.newVoucher(t, "OLD", "amount", "1000", "", "0", "NULL", "current_date - 5", nil, true)
	e.newVoucher(t, "OFF", "amount", "1000", "", "0", "NULL", "NULL", nil, false)
	e.newVoucher(t, "BIG", "amount", "1000", "", "500000", "NULL", "NULL", nil, true)
	e.newVoucher(t, "HUGE", "amount", "300000", "", "0", "NULL", "NULL", nil, true)
	e.newVoucher(t, "RUGI", "amount", "15000", "", "0", "NULL", "NULL", nil, true)
	e.newVoucher(t, "FULL", "amount", "1000", "", "0", "NULL", "NULL", 1, true)
	if _, err := e.admin.Exec(ctx, `UPDATE vouchers SET used_count = 1 WHERE code = 'FULL' AND tenant_id = $1`, e.tenant); err != nil {
		t.Fatal(err)
	}
	e.newVoucher(t, "OTHER-TENANT", "amount", "1000", "", "0", "NULL", "NULL", nil, true)
	if _, err := e.admin.Exec(ctx, `UPDATE vouchers SET tenant_id = $1 WHERE code = 'OTHER-TENANT' AND tenant_id = $2`, e.other, e.tenant); err != nil {
		t.Fatal(err)
	}

	for code, want := range map[string]string{
		"nope": voucher.CodeUnknown, "SOON": voucher.CodeNotStarted, "OLD": voucher.CodeExpired, "OFF": voucher.CodeInactive,
		"BIG": voucher.CodeMinSpend, "FULL": voucher.CodeExhausted, "OTHER-TENANT": voucher.CodeUnknown, "!!": voucher.CodeUnknown,
	} {
		_, err := e.svc.Quote(ctx, a, Request{Lines: []LineIn{line(it, "1")}, VoucherCodes: []string{code}})
		fieldErr(t, err, "voucher_codes.0", want)
	}
	// Potongan melebihi dasar, dan membuat nota di bawah HPP (90.000 × 1; 100.000 − 15.000 = 85.000).
	_, err := e.svc.Quote(ctx, a, Request{Lines: []LineIn{line(it, "1")}, VoucherCodes: []string{"HUGE"}})
	fieldErr(t, err, "voucher_codes", codeVoucherTooHigh)
	_, err = e.svc.Quote(ctx, a, Request{Lines: []LineIn{line(it, "1")}, VoucherCodes: []string{"RUGI"}})
	fieldErr(t, err, "voucher_codes", codeVoucherBelowCost)
	// Kode ganda dan terlalu banyak.
	_, err = e.svc.Quote(ctx, a, Request{Lines: []LineIn{line(it, "1")}, VoucherCodes: []string{"OFF", "off"}})
	fieldErr(t, err, "voucher_codes.1", codeVoucherDuplicate)
	_, err = e.svc.Quote(ctx, a, Request{Lines: []LineIn{line(it, "1")}, VoucherCodes: []string{"A1A", "A2A", "A3A", "A4A", "A5A", "A6A"}})
	fieldErr(t, err, "voucher_codes", codeTooMany)

	// Nota yang ditolak tidak memakai kuota apa pun dan tidak membuat nota.
	_, _, err = e.svc.Create(ctx, a, key(), Request{Lines: []LineIn{line(it, "1")}, VoucherCodes: []string{"FULL"}, Payments: []PaymentIn{pay("cash", "100000")}})
	fieldErr(t, err, "voucher_codes.0", voucher.CodeExhausted)
	var n int
	if err := e.admin.QueryRow(ctx, `SELECT count(*) FROM sales WHERE tenant_id = $1`, e.tenant).Scan(&n); err != nil || n != 0 {
		t.Fatalf("nota tersimpan = %d err=%v", n, err)
	}
}

// Kuota 3 dengan 10 nota bersamaan: tepat 3 berhasil, sisanya VOUCHER_EXHAUSTED; used_count tidak pernah melebihi batas.
func TestVoucherQuotaConcurrent(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	a := e.actor(e.tenant)
	it := e.item(t, "goods", "100000", "30000", 50, false)
	id := e.newVoucher(t, "RAME", "amount", "1000", "", "0", "NULL", "NULL", 3, true)

	var ok, exhausted, other atomic.Int32
	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _, err := e.svc.Create(ctx, a, key(), Request{Lines: []LineIn{line(it, "1")}, VoucherCodes: []string{"RAME"}, Payments: []PaymentIn{pay("cash", "99000")}})
			var f FieldErrors
			switch {
			case err == nil:
				ok.Add(1)
			case errors.As(err, &f) && f["voucher_codes.0"] == voucher.CodeExhausted:
				exhausted.Add(1)
			default:
				other.Add(1)
				t.Errorf("galat tak terduga: %v", err)
			}
		}()
	}
	wg.Wait()
	if ok.Load() != 3 || exhausted.Load() != 7 || e.voucherUsed(t, id) != 3 {
		t.Fatalf("berhasil=%d habis=%d lain=%d used=%d, want 3/7/0/3", ok.Load(), exhausted.Load(), other.Load(), e.voucherUsed(t, id))
	}
}
