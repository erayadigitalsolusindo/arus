package purchasing

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/shopspring/decimal"

	"aciraba/internal/platform/db"
	"aciraba/internal/stock"
)

func TestReverseAvgFormula(t *testing.T) {
	d := decimal.RequireFromString
	// Stok 20 @1.100 setelah pembelian 10 @1.200 → sebelum pembelian 10 @1.000.
	if v, ok := reverseAvg(d("20"), d("1100"), d("10"), d("12000")); !ok || !v.Equal(d("1000")) {
		t.Errorf("kasus dasar: %v %v", v, ok)
	}
	// Sebagian sudah terjual (stok 15): sisa 5 dihitung dari HPP sekarang.
	if v, ok := reverseAvg(d("15"), d("1100"), d("10"), d("12000")); !ok || !v.Equal(d("900")) {
		t.Errorf("stok berkurang: %v %v", v, ok)
	}
	// Sisa stok ≤ 0: HPP dibiarkan.
	if v, ok := reverseAvg(d("10"), d("1100"), d("10"), d("12000")); ok || !v.Equal(d("1100")) {
		t.Errorf("sisa nol: %v %v", v, ok)
	}
	// Hasil negatif dibulatkan ke nol.
	if v, ok := reverseAvg(d("20"), d("100"), d("10"), d("12000")); !ok || !v.IsZero() {
		t.Errorf("negatif: %v %v", v, ok)
	}
}

// Salah ketik qty (10 → 6): nota lama dibalik (stok & HPP kembali), revisi -R2 menggantikannya, HPP dihitung ulang.
func TestEditRevisesStockAndCostAndLinksChain(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	a := e.actor(e.outlet)
	it := e.item(t, "goods", "1000", 10)
	p1, _, err := e.svc.Create(ctx, a, key(), e.req(line(it, "10", "0", "1200")))
	if err != nil {
		t.Fatal(err)
	}
	if e.bal(t, it, stock.BucketDisplay) != "20" {
		t.Fatalf("stok setelah simpan: %s", e.bal(t, it, stock.BucketDisplay))
	}
	if avg, _ := e.cost(t, e.outlet, it); avg != "1100.00" {
		t.Fatalf("HPP setelah simpan: %s", avg)
	}

	p2, replayed, err := e.svc.Edit(ctx, a, p1.ID, key(), e.req(line(it, "6", "0", "1200")), "Salah ketik qty")
	if err != nil || replayed {
		t.Fatalf("revisi: %v replay=%v", err, replayed)
	}
	if p2.Revision != 2 || !strings.HasSuffix(p2.DocNo, "-R2") || p2.SupersedesID == nil || *p2.SupersedesID != p1.ID {
		t.Errorf("tautan revisi salah: rev=%d doc=%s supersedes=%v", p2.Revision, p2.DocNo, p2.SupersedesID)
	}
	if p2.Status != "completed" || p2.RevisionReason != "Salah ketik qty" {
		t.Errorf("status/alasan revisi: %s %q", p2.Status, p2.RevisionReason)
	}
	if got := e.bal(t, it, stock.BucketDisplay); got != "16" {
		t.Errorf("stok setelah revisi = %s, mau 16 (10 awal + 6 revisi)", got)
	}
	// HPP: (10 × 1000 + 6 × 1200) ÷ 16 = 1075.00.
	if avg, last := e.cost(t, e.outlet, it); avg != "1075.00" || last != "1200.00" {
		t.Errorf("HPP setelah revisi: avg=%s last=%s", avg, last)
	}

	old, err := e.svc.Get(ctx, a, p1.ID)
	if err != nil {
		t.Fatal(err)
	}
	if old.Status != "superseded" || old.SupersededBy == nil || *old.SupersededBy != p2.ID || old.RevisedAt == nil {
		t.Errorf("nota lama: status=%s superseded_by=%v", old.Status, old.SupersededBy)
	}
	// Riwayat satu rantai: create (asal), superseded (asal), edit (revisi) — dibaca dari nota mana pun.
	actions := map[string]bool{}
	for _, ev := range p2.Events {
		actions[ev.Action] = true
	}
	if !actions["purchase.create"] || !actions["purchase.superseded"] || !actions["purchase.edit"] {
		t.Errorf("riwayat rantai tidak lengkap: %+v", p2.Events)
	}
	// Daftar hanya memuat revisi; ringkasan menghitung revisi saja.
	res, err := e.svc.List(ctx, a, ListParams{})
	if err != nil || len(res.Data) != 1 || res.Data[0].ID != p2.ID || res.Summary.Count != 1 || res.Summary.Total != "7200.00" {
		t.Errorf("daftar setelah revisi: %v %+v", err, res)
	}
	// Movement pembalikan tercatat negatif.
	var n int
	_ = e.admin.QueryRow(ctx, `SELECT count(*) FROM stock_movements WHERE tenant_id=$1 AND ref_type='PURCHASE_VOID' AND qty_delta = -10`, e.tenant).Scan(&n)
	if n != 1 {
		t.Errorf("movement PURCHASE_VOID = %d, mau 1", n)
	}
}

// Nomor faktur pemasok yang sama wajib bisa dipakai revisi (nota lama sudah tidak 'completed' saat revisi masuk).
func TestEditKeepsSupplierInvoiceNumber(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	a := e.actor(e.outlet)
	it := e.item(t, "goods", "0", 0)
	r := e.req(line(it, "2", "0", "1000"))
	r.SupplierInvoiceNo = "FKT-77"
	p1, _, err := e.svc.Create(ctx, a, key(), r)
	if err != nil {
		t.Fatal(err)
	}
	r.Lines[0].QtyDisplay = num("3")
	if _, _, err := e.svc.Edit(ctx, a, p1.ID, key(), r, "Qty kurang"); err != nil {
		t.Fatalf("revisi dengan faktur sama: %v", err)
	}
}

// Batal: stok dan HPP kembali, nota tetap tercatat 'void' dengan alasan, dan tidak masuk total.
func TestVoidRestoresStockAndKeepsRecord(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	a := e.actor(e.outlet)
	it := e.item(t, "goods", "1000", 10)
	p, _, err := e.svc.Create(ctx, a, key(), e.req(line(it, "10", "0", "1200")))
	if err != nil {
		t.Fatal(err)
	}
	v, err := e.svc.Void(ctx, a, p.ID, "Salah input barang")
	if err != nil {
		t.Fatalf("batal: %v", err)
	}
	if v.Status != "void" || v.VoidReason != "Salah input barang" || v.VoidedAt == nil {
		t.Errorf("status batal: %s %q", v.Status, v.VoidReason)
	}
	if e.bal(t, it, stock.BucketDisplay) != "10" {
		t.Errorf("stok setelah batal = %s, mau 10", e.bal(t, it, stock.BucketDisplay))
	}
	if avg, _ := e.cost(t, e.outlet, it); avg != "1000.00" {
		t.Errorf("HPP setelah batal = %s, mau 1000.00", avg)
	}
	res, err := e.svc.List(ctx, a, ListParams{})
	if err != nil || len(res.Data) != 1 || res.Data[0].Status != "void" || res.Summary.Count != 0 || res.Summary.Total != "0.00" {
		t.Errorf("daftar setelah batal: %v %+v", err, res)
	}
	// Nota yang sudah batal tidak bisa diubah lagi.
	if _, err := e.svc.Void(ctx, a, p.ID, "Dua kali"); !errors.Is(err, ErrNotEditable) {
		t.Errorf("batal ganda: %v", err)
	}
}

// Batal pembelian kredit: hutangnya ikut dibatalkan (tidak lagi tampil di detail nota).
func TestVoidCreditCancelsPayable(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	a := e.actor(e.outlet)
	it := e.item(t, "goods", "0", 0)
	r := e.req(line(it, "4", "0", "500"))
	r.PaymentType = "credit"
	p, _, err := e.svc.Create(ctx, a, key(), r)
	if err != nil || p.Payable == nil {
		t.Fatalf("kredit: %v %+v", err, p.Payable)
	}
	if _, err := e.svc.Void(ctx, a, p.ID, "Batal kredit"); err != nil {
		t.Fatal(err)
	}
	got, _ := e.svc.Get(ctx, a, p.ID)
	if got.Payable != nil {
		t.Errorf("hutang masih tampil setelah batal: %+v", got.Payable)
	}
	var voided int
	_ = e.admin.QueryRow(ctx, `SELECT count(*) FROM payables WHERE tenant_id=$1 AND purchase_id=$2 AND voided_at IS NOT NULL`, e.tenant, p.ID).Scan(&voided)
	if voided != 1 {
		t.Errorf("hutang tidak ditandai batal: %d", voided)
	}
}

// Barang sudah terjual sebagian: pembalikan yang membuat stok gudang/display kurang ditolak, dan tidak ada yang berubah.
func TestVoidRejectedWhenGoodsAlreadySold(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	a := e.actor(e.outlet)
	it := e.item(t, "goods", "1000", 0)
	p, _, err := e.svc.Create(ctx, a, key(), e.req(line(it, "10", "0", "1200")))
	if err != nil {
		t.Fatal(err)
	}
	e.sell(t, it, "8") // stok tinggal 2
	_, err = e.svc.Void(ctx, a, p.ID, "Batal setelah laku")
	if !errors.Is(err, stock.ErrInsufficient) {
		t.Fatalf("mau stok kurang, dapat: %v", err)
	}
	if e.bal(t, it, stock.BucketDisplay) != "2" {
		t.Errorf("stok berubah walau batal gagal: %s", e.bal(t, it, stock.BucketDisplay))
	}
	got, _ := e.svc.Get(ctx, a, p.ID)
	if got.Status != "completed" {
		t.Errorf("status berubah walau gagal: %s", got.Status)
	}
}

// Batas hari edit tenant: lewat batas → ditolak; cukup dinaikkan → diizinkan.
func TestEditWindowFollowsTenantSetting(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	a := e.actor(e.outlet)
	it := e.item(t, "goods", "1000", 0)
	p, _, err := e.svc.Create(ctx, a, key(), e.req(line(it, "1", "0", "1000")))
	if err != nil {
		t.Fatal(err)
	}
	_, _ = e.admin.Exec(ctx, `UPDATE purchases SET created_at = now() - interval '2 days' WHERE id = $1`, p.ID)
	if _, err := e.svc.Void(ctx, a, p.ID, "Terlambat"); !errors.Is(err, ErrEditWindowClosed) {
		t.Fatalf("batas 0 hari, nota 2 hari: %v", err)
	}
	_, _ = e.admin.Exec(ctx, `UPDATE tenants SET sale_edit_window_days = 3 WHERE id = $1`, e.tenant)
	if _, err := e.svc.Void(ctx, a, p.ID, "Masih boleh"); err != nil {
		t.Fatalf("batas 3 hari: %v", err)
	}
}

// Kiriman ulang dengan Idempotency-Key yang sama tidak membalik apa pun dua kali; cabang lain ditolak.
func TestEditIdempotentAndOutletGuard(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	a := e.actor(e.outlet)
	it := e.item(t, "goods", "1000", 10)
	p, _, err := e.svc.Create(ctx, a, key(), e.req(line(it, "10", "0", "1200")))
	if err != nil {
		t.Fatal(err)
	}
	k := key()
	first, _, err := e.svc.Edit(ctx, a, p.ID, k, e.req(line(it, "6", "0", "1200")), "Revisi")
	if err != nil {
		t.Fatal(err)
	}
	again, replayed, err := e.svc.Edit(ctx, a, p.ID, k, e.req(line(it, "6", "0", "1200")), "Revisi")
	if err != nil || !replayed || again.ID != first.ID {
		t.Fatalf("kiriman ulang: %v replay=%v id=%v", err, replayed, again.ID)
	}
	if e.bal(t, it, stock.BucketDisplay) != "16" {
		t.Errorf("stok ganda setelah kiriman ulang: %s", e.bal(t, it, stock.BucketDisplay))
	}
	// Kunci sama tetapi isi berbeda ditolak.
	if _, _, err := e.svc.Edit(ctx, a, p.ID, k, e.req(line(it, "7", "0", "1200")), "Revisi"); !errors.Is(err, ErrKeyMismatch) {
		t.Errorf("isi beda dengan kunci sama: %v", err)
	}
	// Dari cabang lain: revisi ditolak; batal juga ditolak bila cabang itu tak punya akses.
	other := e.actor(e.outlet2)
	other.Outlets = map[uuid.UUID]bool{e.outlet2: true}
	if _, _, err := e.svc.Edit(ctx, other, first.ID, key(), e.req(line(it, "6", "0", "1200")), "Dari cabang lain"); !errors.Is(err, ErrOutletMismatch) {
		t.Errorf("revisi dari cabang lain: %v", err)
	}
	// Nota asal sudah digantikan: tidak bisa diedit atau dibatalkan lagi.
	if _, _, err := e.svc.Edit(ctx, a, p.ID, key(), e.req(line(it, "1", "0", "1200")), "Lagi"); !errors.Is(err, ErrNotEditable) {
		t.Errorf("edit nota asal yang sudah digantikan: %v", err)
	}
}

// Delapan pembatalan serentak atas satu nota: tepat satu yang berhasil dan stok hanya dibalik sekali.
func TestConcurrentVoidReversesOnce(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	a := e.actor(e.outlet)
	it := e.item(t, "goods", "1000", 10)
	p, _, err := e.svc.Create(ctx, a, key(), e.req(line(it, "10", "0", "1200")))
	if err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	var mu sync.Mutex
	ok := 0
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := e.svc.Void(ctx, a, p.ID, "Serentak"); err == nil {
				mu.Lock()
				ok++
				mu.Unlock()
			} else if !errors.Is(err, ErrNotEditable) {
				t.Errorf("galat tak terduga: %v", err)
			}
		}()
	}
	wg.Wait()
	if ok != 1 {
		t.Errorf("pembatalan berhasil = %d, mau 1", ok)
	}
	if e.bal(t, it, stock.BucketDisplay) != "10" {
		t.Errorf("stok = %s, mau 10 (dibalik sekali)", e.bal(t, it, stock.BucketDisplay))
	}
}

// Pembatalan nota berjalan serentak dengan penjualan barang yang sama: HPP hasil pembalikan harus dihitung dari stok
// tepat saat movement pembalikan diterapkan, yaitu 20 dikurangi penjualan yang sudah commit sebelum movement itu.
func TestVoidCostUsesStockAtReversal(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	a := e.actor(e.outlet)
	for round := 0; round < 5; round++ {
		it := e.item(t, "goods", "1000", 10)
		p, _, err := e.svc.Create(ctx, a, key(), e.req(line(it, "10", "0", "1200")))
		if err != nil {
			t.Fatal(err)
		}
		const sells = 6
		var wg sync.WaitGroup
		for i := 0; i < sells; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				e.sell(t, it, "1")
			}()
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := e.svc.Void(ctx, a, p.ID, "Serentak dengan penjualan"); err != nil {
				t.Errorf("void: %v", err)
			}
		}()
		wg.Wait()

		var before int
		if err := e.admin.QueryRow(ctx, `SELECT count(*) FROM stock_movements s WHERE s.tenant_id=$1 AND s.item_id=$2 AND s.ref_type='SALE'
			AND s.id < (SELECT id FROM stock_movements WHERE tenant_id=$1 AND item_id=$2 AND ref_type='PURCHASE_VOID' LIMIT 1)`, e.tenant, it).Scan(&before); err != nil {
			t.Fatal(err)
		}
		stockBefore := decimal.NewFromInt(int64(20 - before))
		want, ok := reverseAvg(stockBefore, decimal.RequireFromString("1100"), decimal.NewFromInt(10), decimal.NewFromInt(12000))
		avg, _ := e.cost(t, e.outlet, it)
		if !ok {
			// Sisa stok ≤ 0: HPP dibiarkan.
			want = decimal.RequireFromString("1100")
		}
		if avg != want.StringFixed(2) {
			t.Errorf("ronde %d: HPP = %s, mau %s (penjualan sebelum pembalikan = %d)", round, avg, want.StringFixed(2), before)
		}
	}
}

// sell mengurangi stok display seperti penjualan (untuk skenario barang sudah laku).
func (e *env) sell(t *testing.T, item uuid.UUID, qty string) {
	t.Helper()
	ctx := context.Background()
	err := db.WithTenant(ctx, e.app, e.tenant, func(tx pgx.Tx) error {
		_, err := stock.Apply(ctx, tx, stock.Movement{TenantID: e.tenant, OutletID: e.outlet, ItemID: item, Bucket: stock.BucketDisplay,
			Delta: decimal.RequireFromString("-" + qty), RefType: stock.RefSale, RefID: uuid.New(), ActorID: e.user})
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
}

// Regresi celah HPP: penjualan yang belum commit memegang kunci baris saldo saat batal dijalankan. HPP harus dihitung dari
// stok SESUDAH penjualan itu (bukan stok yang sempat terbaca sebelum kunci didapat).
func TestVoidCostUsesStockAfterConcurrentSale(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	a := e.actor(e.outlet)
	it := e.item(t, "goods", "1000", 10)
	p, _, err := e.svc.Create(ctx, a, key(), e.req(line(it, "10", "0", "1200"))) // stok 20 @ 1.100
	if err != nil {
		t.Fatal(err)
	}

	held, release := make(chan struct{}), make(chan struct{})
	saleDone := make(chan error, 1)
	go func() {
		saleDone <- db.WithTenant(ctx, e.app, e.tenant, func(tx pgx.Tx) error {
			if _, err := stock.Apply(ctx, tx, stock.Movement{TenantID: e.tenant, OutletID: e.outlet, ItemID: it, Bucket: stock.BucketDisplay,
				Delta: decimal.NewFromInt(-2), RefType: stock.RefSale, RefID: uuid.New(), ActorID: e.user}); err != nil {
				return err
			}
			close(held) // kunci baris saldo dipegang sampai release
			<-release
			return nil
		})
	}()
	select {
	case <-held:
	case err := <-saleDone:
		t.Fatalf("penjualan gagal sebelum menahan kunci: %v", err)
	}

	voidDone := make(chan error, 1)
	go func() {
		_, err := e.svc.Void(ctx, a, p.ID, "Batal saat ada penjualan")
		voidDone <- err
	}()
	time.Sleep(300 * time.Millisecond) // beri waktu batal mencapai antrean kunci saldo
	close(release)
	if err := <-saleDone; err != nil {
		t.Fatal(err)
	}
	if err := <-voidDone; err != nil {
		t.Fatalf("batal: %v", err)
	}

	if got := e.bal(t, it, stock.BucketDisplay); got != "8" {
		t.Errorf("stok = %s, mau 8 (20 − 2 terjual − 10 dibalik)", got)
	}
	// (18 × 1.100 − 12.000) ÷ 8 = 975; dengan stok basi (20) hasilnya salah: 1.000.
	if avg, _ := e.cost(t, e.outlet, it); avg != "975.00" {
		t.Errorf("HPP = %s, mau 975.00", avg)
	}
}
