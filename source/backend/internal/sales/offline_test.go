package sales

import (
	"context"
	"testing"
	"time"
)

// Nota offline: stok boleh minus, created_at = waktu transaksi, penanda offline; kirim ulang tidak menggandakan.
func TestOfflineSaleKeepsOccurredAtAndAllowsNegativeStock(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	a := e.actor(e.tenant)
	it := e.item(t, "goods", "1000", "400", 1, false)
	at := time.Now().Add(-30 * time.Minute).UTC().Truncate(time.Second)
	req := Request{Lines: []LineIn{line(it, "3")}, Payments: []PaymentIn{pay("cash", "3000")}, Offline: true, OccurredAt: &at}

	s, _, err := e.svc.Create(ctx, a, "off-key-0001", req)
	if err != nil {
		t.Fatal(err)
	}
	if got := e.stockOf(t, it); got != "-2" {
		t.Fatalf("stok: %s", got)
	}
	var off bool
	var created time.Time
	if err := e.admin.QueryRow(ctx, `SELECT offline, created_at FROM sales WHERE id = $1`, s.ID).Scan(&off, &created); err != nil {
		t.Fatal(err)
	}
	if !off || !created.Equal(at) {
		t.Fatalf("offline=%v created_at=%v want %v", off, created, at)
	}
	if _, rp, err := e.svc.Create(ctx, a, "off-key-0001", req); err != nil || !rp {
		t.Fatalf("replay: %v %v", rp, err)
	}
	// Nota online biasa tetap menolak stok kurang; waktu di luar rentang ditolak.
	if _, _, err := e.svc.Create(ctx, a, key(), Request{Lines: []LineIn{line(it, "1")}, Payments: []PaymentIn{pay("cash", "1000")}}); err == nil {
		t.Fatal("stok minus harus ditolak untuk nota online")
	}
	old := time.Now().Add(-30 * 24 * time.Hour)
	req.OccurredAt = &old
	_, _, err = e.svc.Create(ctx, a, key(), req)
	fieldErr(t, err, "occurred_at", "INVALID")
}
