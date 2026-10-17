package sales

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"

	"aciraba/internal/plan"
)

// Batas transaksi harian paket: nota ke-(max+1) hari itu ditolak tanpa mengubah stok; replay Idempotency-Key tidak
// memakai jatah lagi; kegagalan validasi tidak memakai jatah.
func TestPlanTxLimit(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	svc := e.svc.WithPlanGuard(plan.ConsumeTx)
	planID := uuid.New()
	e.exec(t, `INSERT INTO plans (id, code, name, max_tx_per_day) VALUES ($1, $2, 'UJI', 2)`, planID, "sl-"+planID.String()[:8])
	e.exec(t, `INSERT INTO tenant_subscriptions (tenant_id, plan_id) VALUES ($1, $2)`, e.tenant, planID)
	t.Cleanup(func() { _, _ = e.admin.Exec(ctx, `DELETE FROM tenant_subscriptions WHERE tenant_id = $1`, e.tenant); _, _ = e.admin.Exec(ctx, `DELETE FROM plans WHERE id = $1`, planID) })

	a := e.item(t, "goods", "1000", "0", 50, false)
	in := Request{Lines: []LineIn{line(a, "1")}, Payments: []PaymentIn{pay("cash", "1000")}}
	k := key()
	if _, _, err := svc.Create(ctx, e.actor(e.tenant), k, in); err != nil {
		t.Fatal(err)
	}
	if _, replayed, err := svc.Create(ctx, e.actor(e.tenant), k, in); err != nil || !replayed { // replay tidak memakai jatah
		t.Fatalf("replay: %v %v", err, replayed)
	}
	bad := Request{Lines: []LineIn{line(a, "0")}, Payments: []PaymentIn{pay("cash", "0")}} // gagal validasi: tidak memakai jatah
	if _, _, err := svc.Create(ctx, e.actor(e.tenant), key(), bad); err == nil {
		t.Fatal("nota tidak valid harus ditolak")
	}
	if _, _, err := svc.Create(ctx, e.actor(e.tenant), key(), in); err != nil {
		t.Fatalf("nota ke-2: %v", err)
	}
	if _, _, err := svc.Create(ctx, e.actor(e.tenant), key(), in); !errors.Is(err, plan.ErrTxLimit) {
		t.Fatalf("nota ke-3 = %v, want ErrTxLimit", err)
	}
	if got := e.stockOf(t, a); got != "48" || e.count(t, "sales") != 2 {
		t.Fatalf("stok/nota setelah ditolak: stok=%s nota=%d, want 48/2", got, e.count(t, "sales"))
	}
}
