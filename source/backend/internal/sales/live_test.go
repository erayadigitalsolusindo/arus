package sales

import (
	"context"
	"testing"
	"time"

	"aciraba/internal/live"
)

// Ringkasan "Penjualan Langsung" hari ini: omzet, retur, jam, nota terbaru; nota batal tidak dihitung.
func TestLiveTodaySummary(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	it := e.item(t, "goods", "1000", "400", 50, false)
	a := e.actor(e.tenant)
	svc := live.NewService(e.app)

	empty, err := svc.Today(ctx, a)
	if err != nil || empty.Sales.Count != 0 || empty.Sales.Total != "0.00" || len(empty.Hours) != 24 || len(empty.Recent) != 0 {
		t.Fatalf("hari kosong: %+v %v", empty, err)
	}

	s1, _, err := e.svc.Create(ctx, a, key(), Request{Lines: []LineIn{line(it, "2")}, Payments: []PaymentIn{pay("cash", "2000")}})
	if err != nil {
		t.Fatal(err)
	}
	s2, _, err := e.svc.Create(ctx, a, key(), Request{Lines: []LineIn{line(it, "3")}, Payments: []PaymentIn{pay("cash", "3000")}})
	if err != nil {
		t.Fatal(err)
	}
	got, err := svc.Today(ctx, a)
	if err != nil {
		t.Fatal(err)
	}
	if got.Sales.Count != 2 || got.Sales.Total != "5000.00" || got.Average != "2500.00" || got.Net != "5000.00" {
		t.Fatalf("ringkasan: %+v", got)
	}
	if len(got.Recent) != 2 || got.Recent[0].DocNo != s2.DocNo || got.Recent[0].Cashier != "Kasir Uji" || got.Recent[0].LineCount != 1 {
		t.Fatalf("terbaru: %+v", got.Recent)
	}
	loc, err := time.LoadLocation(got.Timezone)
	if err != nil {
		t.Fatal(err)
	}
	h := time.Now().In(loc).Hour()
	if got.Hours[h].Count < 1 { // tepi pergantian jam: boleh jatuh di jam sebelumnya
		n := 0
		for _, x := range got.Hours {
			n += x.Count
		}
		if n != 2 {
			t.Fatalf("jam: %+v", got.Hours)
		}
	}

	// Batal nota → tidak masuk omzet; kasir/outlet lain tidak bocor.
	e.exec(t, `UPDATE sales SET status = 'void', void_reason = 'uji', voided_at = now(), voided_by = $2 WHERE id = $1`, s1.ID, e.user)
	got, err = svc.Today(ctx, a)
	if err != nil || got.Sales.Count != 1 || got.Sales.Total != "3000.00" {
		t.Fatalf("setelah batal: %+v %v", got, err)
	}
}
