package stock

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"aciraba/internal/authz"
)

func (e *env) actor(outlet uuid.UUID) authz.Actor {
	return authz.Actor{TenantID: e.tenant, UserID: uuid.New(), OutletID: outlet, Name: "Uji", Perms: authz.Permissions{All: true}}
}

func (e *env) auditCount(t *testing.T, action string) int {
	t.Helper()
	var n int
	if err := e.admin.QueryRow(context.Background(), `SELECT count(*) FROM audit_log WHERE tenant_id = $1 AND action = $2`, e.tenant, action).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func TestSetOpening(t *testing.T) {
	e := newEnv(t)
	t.Cleanup(func() {
		_, _ = e.admin.Exec(context.Background(), `DELETE FROM audit_log WHERE tenant_id = $1`, e.tenant)
	})
	svc := NewService(e.app)
	ctx := context.Background()
	a := e.actor(e.outlet)
	item := e.item(t, e.tenant, "goods", false)

	bal, err := svc.SetOpening(ctx, a, item, BucketDisplay, "50")
	if err != nil || bal.String() != "50" {
		t.Fatalf("saldo awal: %v %v", bal, err)
	}
	// Mengubah saldo awal = movement selisih (ledger tetap append-only), bukan menimpa.
	if bal, err = svc.SetOpening(ctx, a, item, BucketDisplay, "42.5"); err != nil || bal.String() != "42.5" {
		t.Fatalf("koreksi: %v %v", bal, err)
	}
	if e.movements(t, item) != 2 || e.balance(t, item, BucketDisplay) != "42.5" {
		t.Errorf("movement=%d saldo=%s", e.movements(t, item), e.balance(t, item, BucketDisplay))
	}
	var sum string
	if err := e.admin.QueryRow(ctx, `SELECT sum(qty_delta)::text FROM stock_movements WHERE item_id = $1 AND ref_type = 'OPENING'`, item).Scan(&sum); err != nil || sum != "42.500" {
		t.Errorf("jumlah movement OPENING = %s %v", sum, err)
	}
	// Nilai sama = tidak ada movement baru; bisa diturunkan ke nol.
	if _, err = svc.SetOpening(ctx, a, item, BucketDisplay, "42.5"); err != nil || e.movements(t, item) != 2 {
		t.Errorf("nilai sama: %v movement=%d", err, e.movements(t, item))
	}
	if bal, err = svc.SetOpening(ctx, a, item, BucketDisplay, "0"); err != nil || !bal.IsZero() {
		t.Errorf("ke nol: %v %v", bal, err)
	}
	// Bucket dan outlet saling independen.
	if _, err = svc.SetOpening(ctx, a, item, BucketWarehouse, "7"); err != nil {
		t.Fatal(err)
	}
	if _, err = svc.SetOpening(ctx, e.actor(e.outlet2), item, BucketDisplay, "9"); err != nil {
		t.Fatal(err)
	}
	if e.balance(t, item, BucketDisplay) != "0" || e.balance(t, item, BucketWarehouse) != "7" {
		t.Error("outlet/bucket lain tidak boleh ikut berubah")
	}
	if n := e.auditCount(t, "stock.opening"); n != 5 {
		t.Errorf("audit stock.opening = %d, harus 5 (nilai sama tidak dicatat)", n)
	}

	// Validasi.
	for name, c := range map[string]struct{ bucket, qty, field string }{
		"qty negatif":   {BucketDisplay, "-1", "qty"},
		"4 desimal":     {BucketDisplay, "1.0001", "qty"},
		"bukan angka":   {BucketDisplay, "abc", "qty"},
		"qty kosong":    {BucketDisplay, "", "qty"},
		"terlalu besar": {BucketDisplay, "1000000000000", "qty"},
		"bucket asing":  {"toko", "1", "bucket"},
	} {
		var f FieldErrors
		if _, err := svc.SetOpening(ctx, a, item, c.bucket, c.qty); !errors.As(err, &f) || f[c.field] == "" {
			t.Errorf("%s: %v", name, err)
		}
	}
	// Jasa, item tak ada, item tenant lain.
	if _, err := svc.SetOpening(ctx, a, e.item(t, e.tenant, "service", false), BucketDisplay, "1"); !errors.Is(err, ErrNotStocked) {
		t.Errorf("jasa: %v", err)
	}
	if _, err := svc.SetOpening(ctx, a, uuid.New(), BucketDisplay, "1"); !errors.Is(err, ErrItemNotFound) {
		t.Errorf("item tak ada: %v", err)
	}
	if _, err := svc.SetOpening(ctx, a, e.item(t, e.other, "goods", false), BucketDisplay, "1"); !errors.Is(err, ErrItemNotFound) {
		t.Errorf("item tenant lain: %v", err)
	}
}

func TestOpeningLock(t *testing.T) {
	e := newEnv(t)
	t.Cleanup(func() {
		_, _ = e.admin.Exec(context.Background(), `DELETE FROM audit_log WHERE tenant_id = $1`, e.tenant)
	})
	svc := NewService(e.app)
	ctx := context.Background()
	a := e.actor(e.outlet)
	item := e.item(t, e.tenant, "goods", false)
	if _, err := svc.SetOpening(ctx, a, item, BucketDisplay, "10"); err != nil {
		t.Fatal(err)
	}
	if st, err := svc.Status(ctx, a); err != nil || st.Locked || st.StartDate != nil {
		t.Fatalf("status awal: %+v %v", st, err)
	}
	for _, bad := range []string{"", "2026-13-01", "besok", "1999-01-01"} {
		var f FieldErrors
		if _, err := svc.Lock(ctx, a, bad); !errors.As(err, &f) || f["start_date"] == "" {
			t.Errorf("tanggal %q harus ditolak: %v", bad, err)
		}
	}
	st, err := svc.Lock(ctx, a, "2026-11-01")
	if err != nil || !st.Locked || st.StartDate == nil || *st.StartDate != "2026-11-01" {
		t.Fatalf("kunci: %+v %v", st, err)
	}
	if _, err := svc.Lock(ctx, a, "2026-11-02"); !errors.Is(err, ErrAlreadyLocked) {
		t.Errorf("kunci ganda: %v", err)
	}
	// Setelah dikunci: ubah saldo awal ditolak (juga bila nilainya sama), dan Apply OPENING langsung pun ditolak.
	for _, q := range []string{"11", "10"} {
		if _, err := svc.SetOpening(ctx, a, item, BucketDisplay, q); !errors.Is(err, ErrOpeningLocked) {
			t.Errorf("qty %s setelah kunci: %v", q, err)
		}
	}
	if _, err := e.apply(t, Movement{TenantID: e.tenant, OutletID: e.outlet, ItemID: item, Bucket: BucketDisplay, Delta: decimal.NewFromInt(1), RefType: RefOpening}); !errors.Is(err, ErrOpeningLocked) {
		t.Errorf("Apply OPENING setelah kunci: %v", err)
	}
	// Movement non-OPENING tetap jalan (mis. opname/penjualan), dan outlet lain belum terkunci.
	if _, err := e.apply(t, e.mv(item, BucketDisplay, "-1", RefSale)); err != nil {
		t.Errorf("penjualan setelah kunci: %v", err)
	}
	if _, err := svc.SetOpening(ctx, e.actor(e.outlet2), item, BucketDisplay, "3"); err != nil {
		t.Errorf("outlet lain tidak ikut terkunci: %v", err)
	}
	if st, err := svc.Status(ctx, a); err != nil || !st.Locked || *st.StartDate != "2026-11-01" {
		t.Errorf("status: %+v %v", st, err)
	}
	if e.balance(t, item, BucketDisplay) != "9" {
		t.Errorf("saldo = %s, harus 9", e.balance(t, item, BucketDisplay))
	}
	if e.auditCount(t, "stock.opening_lock") != 1 {
		t.Error("audit stock.opening_lock harus tercatat sekali")
	}
}

// Banyak orang mengubah saldo awal yang sama bersamaan: saldo akhir = salah satu target (bukan jumlahnya),
// dan jumlah movement OPENING selalu sama dengan saldo.
func TestConcurrentSetOpening(t *testing.T) {
	e := newEnv(t)
	t.Cleanup(func() {
		_, _ = e.admin.Exec(context.Background(), `DELETE FROM audit_log WHERE tenant_id = $1`, e.tenant)
	})
	svc := NewService(e.app)
	a := e.actor(e.outlet)
	item := e.item(t, e.tenant, "goods", false)
	targets := []string{"5", "12", "20", "33", "40", "8", "17", "26"}
	var wg sync.WaitGroup
	for _, q := range targets {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := svc.SetOpening(context.Background(), a, item, BucketDisplay, q); err != nil {
				t.Errorf("set %s: %v", q, err)
			}
		}()
	}
	wg.Wait()
	bal := e.balance(t, item, BucketDisplay)
	found := false
	for _, q := range targets {
		found = found || q == bal
	}
	var sum string
	if err := e.admin.QueryRow(context.Background(), `SELECT sum(qty_delta)::numeric(18,0)::text FROM stock_movements WHERE item_id = $1`, item).Scan(&sum); err != nil {
		t.Fatal(err)
	}
	if !found || sum != bal {
		t.Errorf("saldo=%s jumlah movement=%s; harus salah satu target dan sama", bal, sum)
	}
}

func TestOpeningList(t *testing.T) {
	e := newEnv(t)
	svc := NewService(e.app)
	ctx := context.Background()
	a := e.actor(e.outlet)
	x, y := e.item(t, e.tenant, "goods", false), e.item(t, e.tenant, "goods", false)
	e.item(t, e.tenant, "service", false)
	if _, err := svc.SetOpening(ctx, a, x, BucketDisplay, "3"); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.SetOpening(ctx, e.actor(e.outlet2), y, BucketWarehouse, "4"); err != nil {
		t.Fatal(err)
	}
	rows, total, err := svc.List(ctx, a, ListParams{})
	if err != nil || total != 2 || len(rows) != 2 {
		t.Fatalf("hanya barang goods: %d %d %v", total, len(rows), err)
	}
	for _, r := range rows {
		switch r.ID {
		case x:
			if r.Display != "3" || r.Warehouse != "0" {
				t.Errorf("x: %+v", r)
			}
		case y:
			if r.Display != "0" || r.Warehouse != "0" {
				t.Errorf("y di outlet utama harus 0 (stok ada di cabang 2): %+v", r)
			}
		}
	}
	if _, total, _ := svc.List(ctx, a, ListParams{Q: "zzz-tak-ada"}); total != 0 {
		t.Errorf("pencarian kosong: %d", total)
	}
	var f FieldErrors
	if _, _, err := svc.List(ctx, a, ListParams{Q: "a\x00b"}); !errors.As(err, &f) {
		t.Errorf("q berkarakter kontrol: %v", err)
	}
}
