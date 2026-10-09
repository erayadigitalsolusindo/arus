package purchasing

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	"aciraba/internal/payable"
)

func daysAgo(n int) string { return time.Now().AddDate(0, 0, -n).Format("2006-01-02") }

// Tiga nota kredit dibuat TIDAK berurutan tanggal: 10.000 (10 hari lalu), 5.000 (5 hari lalu), 8.000 (kemarin).
func (e *env) threeNotes(t *testing.T) (old, mid, new_ uuid.UUID) {
	t.Helper()
	ctx := context.Background()
	a := e.actor(e.outlet)
	it := e.item(t, "goods", "1000", 0)
	mk := func(qty string, ago int) uuid.UUID {
		r := e.creditReq(line(it, qty, "0", "1000"))
		r.PurchaseDate = daysAgo(ago)
		p, _, err := e.svc.Create(ctx, a, key(), r)
		if err != nil || p.Payable == nil {
			t.Fatalf("buat nota: %v", err)
		}
		return p.Payable.ID
	}
	new_ = mk("8", 1)
	old = mk("10", 10)
	mid = mk("5", 5)
	return
}

func TestSettleAutoOldestFirstAndManual(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	a := e.actor(e.outlet)
	m := e.method(t)
	svc := payable.NewService(e.app)
	old, mid, newest := e.threeNotes(t)

	// Pratinjau tidak menulis apa pun.
	q, err := svc.SettleQuote(ctx, a, payable.SettleInput{SupplierID: e.supplier, Mode: "auto", Amount: num("12000")})
	if err != nil || q.Total != "12000.00" || q.Outstanding != "23000.00" || len(q.Allocations) != 2 ||
		q.Allocations[0].PayableID != old || q.Allocations[0].Amount != "10000.00" || q.Allocations[1].PayableID != mid || q.Allocations[1].Amount != "2000.00" {
		t.Fatalf("pratinjau: %+v %v", q, err)
	}
	if n := e.count(t, "payable_payments"); n != 0 {
		t.Fatalf("pratinjau menulis %d pembayaran", n)
	}

	k := key()
	in := payable.SettleInput{SupplierID: e.supplier, Mode: "auto", MethodID: m, Amount: num("12000"), RefNo: "TRF-1"}
	s, replayed, err := svc.Settle(ctx, a, k, in)
	if err != nil || replayed || s.Total != "12000.00" || len(s.Allocations) != 2 {
		t.Fatalf("settle auto: %+v replay=%v err=%v", s, replayed, err)
	}
	if d, _ := svc.Get(ctx, a, old); d.Balance != "0.00" || d.Status != "paid" {
		t.Errorf("nota terlama: %s %s", d.Balance, d.Status)
	}
	if d, _ := svc.Get(ctx, a, mid); d.Balance != "3000.00" || len(d.Payments) != 1 {
		t.Errorf("nota tengah: %s", d.Balance)
	}
	if d, _ := svc.Get(ctx, a, newest); d.Balance != "8000.00" {
		t.Errorf("nota terbaru tak boleh tersentuh: %s", d.Balance)
	}
	// Replay: tidak ada pembayaran ganda; isi beda → mismatch.
	if s2, rp, err := svc.Settle(ctx, a, k, in); err != nil || !rp || s2.ID != s.ID || e.count(t, "payable_payments") != 2 {
		t.Errorf("replay: %v rp=%v n=%d", err, rp, e.count(t, "payable_payments"))
	}
	in2 := in
	in2.Amount = num("1")
	if _, _, err := svc.Settle(ctx, a, k, in2); !errors.Is(err, payable.ErrKeyMismatch) {
		t.Errorf("kunci sama isi beda: %v", err)
	}

	// Manual: sisa nota tengah penuh + sebagian nota terbaru.
	ms, _, err := svc.Settle(ctx, a, key(), payable.SettleInput{SupplierID: e.supplier, Mode: "manual", MethodID: m,
		Allocations: []payable.SettleAlloc{{PayableID: newest, Amount: num("1500")}, {PayableID: mid, Amount: num("3000")}}})
	if err != nil || ms.Total != "4500.00" || len(ms.Allocations) != 2 {
		t.Fatalf("manual: %+v %v", ms, err)
	}
	if d, _ := svc.Get(ctx, a, mid); d.Status != "paid" {
		t.Errorf("nota tengah lunas: %s", d.Status)
	}
	if d, _ := svc.Get(ctx, a, newest); d.Balance != "6500.00" {
		t.Errorf("nota terbaru: %s", d.Balance)
	}

	// Validasi.
	var fe payable.FieldErrors
	bad := map[string]payable.SettleInput{
		"amount":                    {SupplierID: e.supplier, Mode: "auto", MethodID: m, Amount: num("6500.01")}, // > tunggakan
		"allocations.0.amount":      {SupplierID: e.supplier, Mode: "manual", MethodID: m, Allocations: []payable.SettleAlloc{{PayableID: newest, Amount: num("6500.01")}}},
		"allocations.1.payable_id":  {SupplierID: e.supplier, Mode: "manual", MethodID: m, Allocations: []payable.SettleAlloc{{PayableID: newest, Amount: num("1")}, {PayableID: newest, Amount: num("1")}}},
		"allocations.0.payable_id ": {SupplierID: e.supplier, Mode: "manual", MethodID: m, Allocations: []payable.SettleAlloc{{PayableID: uuid.New(), Amount: num("1")}}},
		"mode":                      {SupplierID: e.supplier, Mode: "x", MethodID: m},
		"method_id":                 {SupplierID: e.supplier, Mode: "auto", Amount: num("1")},
		"supplier_id":               {SupplierID: uuid.New(), Mode: "auto", MethodID: m, Amount: num("1")},
	}
	for field, in := range bad {
		field = trim(field)
		if _, _, err := svc.Settle(ctx, a, key(), in); !errors.As(err, &fe) || fe[field] == "" {
			t.Errorf("%s: %v (%v)", field, err, fe)
		}
	}
	// Nota lunas ditolak di manual; tak ada tunggakan di auto.
	if _, _, err := svc.Settle(ctx, a, key(), payable.SettleInput{SupplierID: e.supplier, Mode: "manual", MethodID: m,
		Allocations: []payable.SettleAlloc{{PayableID: old, Amount: num("1")}}}); !errors.As(err, &fe) || fe["allocations.0.amount"] != "SETTLED" {
		t.Errorf("nota lunas: %v", err)
	}
	// Outlet lain tak bisa melihat pelunasan ini.
	if _, err := svc.GetSettlement(ctx, e.actor(e.outlet2), s.ID); !errors.Is(err, payable.ErrNotFound) {
		t.Errorf("akses outlet lain: %v", err)
	}
	// Nota yang sudah dilunasi lewat pelunasan kolektif tidak bisa dibatalkan.
	if _, err := e.svc.Void(ctx, a, e.purchaseOf(t, old), "salah"); !errors.Is(err, ErrPayablePaid) {
		t.Errorf("batal nota terbayar: %v", err)
	}
}

func trim(s string) string {
	for len(s) > 0 && s[len(s)-1] == ' ' {
		s = s[:len(s)-1]
	}
	return s
}

func (e *env) purchaseOf(t *testing.T, payableID uuid.UUID) uuid.UUID {
	t.Helper()
	var id uuid.UUID
	if err := e.admin.QueryRow(context.Background(), `SELECT purchase_id FROM payables WHERE id = $1`, payableID).Scan(&id); err != nil {
		t.Fatal(err)
	}
	return id
}

// Pelunasan serentak tidak boleh melewati total tunggakan; kunci sama serentak = satu dokumen.
func TestSettleConcurrent(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	a := e.actor(e.outlet)
	m := e.method(t)
	svc := payable.NewService(e.app)
	e.threeNotes(t) // tunggakan 23.000

	var wg sync.WaitGroup
	var mu sync.Mutex
	ok := 0
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, _, err := svc.Settle(ctx, a, key(), payable.SettleInput{SupplierID: e.supplier, Mode: "auto", MethodID: m, Amount: num("10000")}); err == nil {
				mu.Lock()
				ok++
				mu.Unlock()
			}
		}()
	}
	wg.Wait()
	if ok != 2 {
		t.Errorf("pelunasan serentak diterima %d, mau 2", ok)
	}
	res, _ := svc.List(ctx, a, payable.ListParams{})
	if res.Summary.Outstanding != "3000.00" {
		t.Errorf("sisa tunggakan %s", res.Summary.Outstanding)
	}

	// Kunci yang sama dikirim 8× serentak → tepat satu dokumen.
	before := e.count(t, "payable_settlements")
	k := key()
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			svc.Settle(ctx, a, k, payable.SettleInput{SupplierID: e.supplier, Mode: "auto", MethodID: m, Amount: num("1000")})
		}()
	}
	wg.Wait()
	if n := e.count(t, "payable_settlements") - before; n != 1 {
		t.Errorf("dokumen dari kunci sama: %d", n)
	}
}
