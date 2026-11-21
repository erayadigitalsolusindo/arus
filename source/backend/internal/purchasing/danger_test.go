package purchasing

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5"

	"aciraba/internal/platform/db"
)

// Danger zone "hutang-piutang-pembelian": nota beli (tunai + kredit), hutang, retur beli hilang; stok dari pembelian ikut kembali nol.
func TestDangerClearPurchasing(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	a := e.actor(e.outlet)
	it := e.item(t, "goods", "1000", 0)
	if _, _, err := e.svc.Create(ctx, a, key(), e.req(line(it, "4", "0", "1000"))); err != nil {
		t.Fatal(err)
	}
	cr := e.req(line(it, "2", "0", "1000"))
	cr.PaymentType = "credit"
	if _, _, err := e.svc.Create(ctx, a, key(), cr); err != nil {
		t.Fatal(err)
	}
	if e.count(t, "purchases") != 2 || e.count(t, "payables") != 1 {
		t.Fatalf("data awal: %d pembelian, %d hutang", e.count(t, "purchases"), e.count(t, "payables"))
	}
	if before := e.bal(t, it, "display"); before != "6" {
		t.Fatalf("stok sebelum: %s", before)
	}
	if err := db.WithTenant(ctx, e.app, e.tenant, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `SELECT danger_clear_purchasing($1)`, e.tenant)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	for _, tbl := range []string{"purchases", "purchase_lines", "payables"} {
		if n := e.count(t, tbl); n != 0 {
			t.Errorf("%s tersisa %d", tbl, n)
		}
	}
	if got := e.bal(t, it, "display"); got != "0" {
		t.Errorf("stok harus kembali nol: %s", got)
	}
	if n := e.count(t, "stock_movements"); n != 0 {
		t.Errorf("mutasi stok tersisa %d", n)
	}
}
