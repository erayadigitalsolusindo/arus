package sales

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/google/uuid"

	"aciraba/internal/receivable"
)

func TestReceivableSettleOldestFirstAndManual(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	a := e.scoped()
	it := e.item(t, "goods", "50000", "30000", 10, false)
	m := e.creditMember(t, "0")
	mk := func(qty string) uuid.UUID {
		s, _, err := e.svc.Create(ctx, a, key(), creditReq(it, qty, &m, "", nil))
		if err != nil || s.Credit == nil {
			t.Fatalf("nota kredit: %v", err)
		}
		return s.Credit.ID
	}
	r1, r2, r3 := mk("1"), mk("2"), mk("1") // 50.000, 100.000, 50.000
	rs := receivable.NewService(e.app)
	cash := e.methodID(t, "cash")

	q, err := rs.SettleQuote(ctx, a, receivable.SettleInput{MemberID: m, Mode: "auto", Amount: jn("120000")})
	if err != nil || q.Outstanding != "200000.00" || len(q.Allocations) != 2 || q.Allocations[0].ReceivableID != r1 ||
		q.Allocations[0].Amount != "50000.00" || q.Allocations[1].ReceivableID != r2 || q.Allocations[1].Amount != "70000.00" {
		t.Fatalf("pratinjau: %+v %v", q, err)
	}

	k := key()
	in := receivable.SettleInput{MemberID: m, Mode: "auto", MethodID: cash, Amount: jn("120000")}
	s, replayed, err := rs.Settle(ctx, a, k, in)
	if err != nil || replayed || s.Total != "120000.00" || len(s.Allocations) != 2 {
		t.Fatalf("settle: %+v %v", s, err)
	}
	for id, want := range map[uuid.UUID]string{r1: "0.00", r2: "30000.00", r3: "50000.00"} {
		if d, _ := rs.Get(ctx, a, id); d.Balance != want {
			t.Errorf("saldo %s = %s, mau %s", id, d.Balance, want)
		}
	}
	if s2, rp, err := rs.Settle(ctx, a, k, in); err != nil || !rp || s2.ID != s.ID {
		t.Errorf("replay: %v rp=%v", err, rp)
	}
	in2 := in
	in2.Amount = jn("1")
	if _, _, err := rs.Settle(ctx, a, k, in2); !errors.Is(err, receivable.ErrKeyMismatch) {
		t.Errorf("kunci sama isi beda: %v", err)
	}

	// Manual: sisa r2 sebagian + r3 penuh.
	ms, _, err := rs.Settle(ctx, a, key(), receivable.SettleInput{MemberID: m, Mode: "manual", MethodID: cash,
		Allocations: []receivable.SettleAlloc{{ReceivableID: r3, Amount: jn("50000")}, {ReceivableID: r2, Amount: jn("10000")}}})
	if err != nil || ms.Total != "60000.00" {
		t.Fatalf("manual: %+v %v", ms, err)
	}
	if d, _ := rs.Get(ctx, a, r3); d.Status != "paid" {
		t.Errorf("r3: %s", d.Status)
	}

	var fe receivable.FieldErrors
	if _, _, err := rs.Settle(ctx, a, key(), receivable.SettleInput{MemberID: m, Mode: "auto", MethodID: cash, Amount: jn("20000.01")}); !errors.As(err, &fe) || fe["amount"] != "OVERPAID" {
		t.Errorf("lebih bayar: %v", err)
	}
	if _, _, err := rs.Settle(ctx, a, key(), receivable.SettleInput{MemberID: m, Mode: "manual", MethodID: cash,
		Allocations: []receivable.SettleAlloc{{ReceivableID: r1, Amount: jn("1")}}}); !errors.As(err, &fe) || fe["allocations.0.amount"] != "SETTLED" {
		t.Errorf("nota lunas: %v", err)
	}
	if _, _, err := rs.Settle(ctx, a, key(), receivable.SettleInput{MemberID: m, Mode: "manual", MethodID: cash,
		Allocations: []receivable.SettleAlloc{{ReceivableID: uuid.New(), Amount: jn("1")}}}); !errors.As(err, &fe) || fe["allocations.0.receivable_id"] != "INVALID" {
		t.Errorf("nota asing: %v", err)
	}
}

func TestReceivableSettleConcurrent(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	a := e.scoped()
	it := e.item(t, "goods", "50000", "30000", 10, false)
	m := e.creditMember(t, "0")
	for i := 0; i < 2; i++ {
		if _, _, err := e.svc.Create(ctx, a, key(), creditReq(it, "1", &m, "", nil)); err != nil {
			t.Fatal(err)
		}
	}
	rs := receivable.NewService(e.app)
	cash := e.methodID(t, "cash")
	var wg sync.WaitGroup
	var mu sync.Mutex
	ok := 0
	for i := 0; i < 8; i++ { // tunggakan 100.000, tiap pelunasan 40.000 → tepat 2 diterima
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, _, err := rs.Settle(ctx, a, key(), receivable.SettleInput{MemberID: m, Mode: "auto", MethodID: cash, Amount: jn("40000")}); err == nil {
				mu.Lock()
				ok++
				mu.Unlock()
			}
		}()
	}
	wg.Wait()
	if ok != 2 {
		t.Errorf("diterima %d, mau 2", ok)
	}
	res, _ := rs.List(ctx, a, receivable.ListParams{})
	if res.Summary.Outstanding != "20000.00" {
		t.Errorf("sisa %s", res.Summary.Outstanding)
	}
}
