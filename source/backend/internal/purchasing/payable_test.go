package purchasing

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/google/uuid"

	"aciraba/internal/payable"
)

func (e *env) method(t *testing.T) uuid.UUID {
	t.Helper()
	id := uuid.New()
	if _, err := e.admin.Exec(context.Background(), `INSERT INTO payment_methods (id, tenant_id, name, kind) VALUES ($1, $2, 'Tunai', 'cash')`, id, e.tenant); err != nil {
		t.Fatal(err)
	}
	return id
}

func (e *env) creditReq(lines ...LineIn) Request {
	r := e.req(lines...)
	r.PaymentType = "credit"
	return r
}

// Cicilan, lebih bayar ditolak, replay idempoten, pembayaran serentak tidak melewati sisa, nota yang sudah dibayar tidak bisa diubah.
func TestPayablePaymentsAndLocks(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	a := e.actor(e.outlet)
	m := e.method(t)
	svc := payable.NewService(e.app)
	it := e.item(t, "goods", "1000", 0)
	p, _, err := e.svc.Create(ctx, a, key(), e.creditReq(line(it, "10", "0", "1000"))) // total 10.000
	if err != nil || p.Payable == nil {
		t.Fatalf("buat nota kredit: %v", err)
	}
	pid := p.Payable.ID
	if p.Payable.Balance != "10000.00" {
		t.Errorf("saldo awal %s", p.Payable.Balance)
	}

	k := key()
	d, replayed, err := svc.Pay(ctx, a, pid, k, payable.PayInput{MethodID: m, Amount: num("4000")})
	if err != nil || replayed || d.Balance != "6000.00" || d.Status != "open" || len(d.Payments) != 1 {
		t.Fatalf("cicilan 1: %+v replay=%v err=%v", d, replayed, err)
	}
	if d2, rp, err := svc.Pay(ctx, a, pid, k, payable.PayInput{MethodID: m, Amount: num("4000")}); err != nil || !rp || len(d2.Payments) != 1 {
		t.Errorf("replay: %v rp=%v n=%d", err, rp, len(d2.Payments))
	}
	if _, _, err := svc.Pay(ctx, a, pid, k, payable.PayInput{MethodID: m, Amount: num("1")}); !errors.Is(err, payable.ErrKeyMismatch) {
		t.Errorf("kunci sama isi beda: %v", err)
	}
	var fe payable.FieldErrors
	if _, _, err := svc.Pay(ctx, a, pid, key(), payable.PayInput{MethodID: m, Amount: num("6000.01")}); !errors.As(err, &fe) || fe["amount"] != "OVERPAID" {
		t.Errorf("lebih bayar: %v", err)
	}

	// Edit/batal ditolak setelah ada pembayaran.
	if _, _, err := e.svc.Edit(ctx, a, p.ID, key(), e.creditReq(line(it, "5", "0", "1000")), "salah ketik"); !errors.Is(err, ErrPayablePaid) {
		t.Errorf("edit nota berhutang terbayar: %v", err)
	}
	if _, err := e.svc.Void(ctx, a, p.ID, "batal"); !errors.Is(err, ErrPayablePaid) {
		t.Errorf("batal nota berhutang terbayar: %v", err)
	}
	if got, _ := e.svc.Get(ctx, a, p.ID); got.Payable == nil || got.Payable.Paid != "4000.00" {
		t.Errorf("detail nota: %+v", got.Payable)
	}

	// 10 pembayaran 1.000 serentak pada sisa 6.000 → tepat 6 diterima.
	var wg sync.WaitGroup
	var mu sync.Mutex
	ok := 0
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, _, err := svc.Pay(ctx, a, pid, key(), payable.PayInput{MethodID: m, Amount: num("1000")}); err == nil {
				mu.Lock()
				ok++
				mu.Unlock()
			}
		}()
	}
	wg.Wait()
	if ok != 6 {
		t.Errorf("pembayaran serentak diterima %d, mau 6", ok)
	}
	d, _ = svc.Get(ctx, a, pid)
	if d.Balance != "0.00" || d.Status != "paid" {
		t.Errorf("setelah lunas: %s %s", d.Balance, d.Status)
	}
	res, err := svc.List(ctx, a, payable.ListParams{Status: "all"})
	if err != nil || len(res.Data) != 1 || res.Summary.Outstanding != "0.00" {
		t.Errorf("daftar: %+v %v", res, err)
	}
	// Outlet lain tidak melihat hutang ini.
	if _, err := svc.Get(ctx, e.actor(e.outlet2), pid); !errors.Is(err, payable.ErrNotFound) {
		t.Errorf("akses outlet lain: %v", err)
	}
}

// Aging + nota yang dibatalkan tidak muncul sebagai hutang.
func TestPayableListAgingAndVoid(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	a := e.actor(e.outlet)
	svc := payable.NewService(e.app)
	it := e.item(t, "goods", "1000", 0)
	p1, _, err := e.svc.Create(ctx, a, key(), e.creditReq(line(it, "10", "0", "1000")))
	if err != nil {
		t.Fatal(err)
	}
	p2, _, err := e.svc.Create(ctx, a, key(), e.creditReq(line(it, "5", "0", "1000")))
	if err != nil {
		t.Fatal(err)
	}
	// Jatuh tempo p1 dibuat 45 hari lalu lewat DB (aging 31–60).
	if _, err := e.admin.Exec(ctx, `UPDATE payables SET due_date = current_date - 45 WHERE id = $1`, p1.Payable.ID); err != nil {
		t.Fatal(err)
	}
	res, _ := svc.List(ctx, a, payable.ListParams{})
	if res.Summary.Outstanding != "15000.00" || res.Summary.Overdue != "10000.00" || res.Summary.Aging.D31to60 != "10000.00" || res.Summary.Aging.Current != "5000.00" {
		t.Errorf("ringkasan/aging: %+v", res.Summary)
	}
	if len(res.Data) != 2 || res.Data[0].ID != p1.Payable.ID || res.Data[0].Status != "overdue" {
		t.Errorf("urutan/status: %+v", res.Data)
	}
	if _, err := e.svc.Void(ctx, a, p2.ID, "salah input"); err != nil {
		t.Fatal(err)
	}
	res, _ = svc.List(ctx, a, payable.ListParams{})
	if len(res.Data) != 1 || res.Summary.Outstanding != "10000.00" {
		t.Errorf("setelah batal: %+v", res.Summary)
	}
}

func TestBuyPriceHistory(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	a := e.actor(e.outlet)
	it := e.item(t, "goods", "1000", 0)
	for _, price := range []string{"1000", "1200", "1100"} {
		if _, _, err := e.svc.Create(ctx, a, key(), e.req(line(it, "2", "0", price, "10"))); err != nil {
			t.Fatal(err)
		}
	}
	res, err := e.svc.PriceHistory(ctx, a, PriceHistoryParams{ItemID: &it, Limit: 2})
	if err != nil || len(res.Data) != 2 || res.NextCursor == "" {
		t.Fatalf("halaman 1: %+v %v", res, err)
	}
	if res.Data[0].UnitPrice != "1100.00" || res.Data[0].PrevPrice == nil || *res.Data[0].PrevPrice != "1200.00" || len(res.Data[0].Discounts) != 1 {
		t.Errorf("baris terbaru: %+v", res.Data[0])
	}
	res2, err := e.svc.PriceHistory(ctx, a, PriceHistoryParams{ItemID: &it, Limit: 2, Cursor: res.NextCursor})
	if err != nil || len(res2.Data) != 1 || res2.NextCursor != "" || res2.Data[0].UnitPrice != "1000.00" || res2.Data[0].PrevPrice != nil {
		t.Errorf("halaman 2: %+v %v", res2, err)
	}
	if _, err := e.svc.PriceHistory(ctx, a, PriceHistoryParams{Cursor: "x"}); err == nil {
		t.Error("kursor rusak harus ditolak")
	}
	if r, _ := e.svc.PriceHistory(ctx, e.actor(e.outlet2), PriceHistoryParams{ItemID: &it}); len(r.Data) != 0 {
		t.Error("outlet lain tidak boleh melihat")
	}
}
