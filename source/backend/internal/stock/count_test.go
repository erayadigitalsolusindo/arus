package stock

import (
	"context"
	"errors"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/google/uuid"
)

func TestCountLifecycle(t *testing.T) {
	e := newEnv(t)
	t.Cleanup(func() {
		_, _ = e.admin.Exec(context.Background(), `DELETE FROM audit_log WHERE tenant_id = $1`, e.tenant)
	})
	svc := NewService(e.app)
	ctx := context.Background()
	a := e.convActor(t, e.outlet)
	x, y, z := e.item(t, e.tenant, "goods", false), e.item(t, e.tenant, "goods", false), e.item(t, e.tenant, "goods", false)
	e.seed(t, a, svc, x, "10")
	e.seed(t, a, svc, y, "5")
	e.setCost(t, x, "1000")

	id, err := svc.CreateCount(ctx, a, BucketDisplay, "Rak depan")
	if err != nil {
		t.Fatal(err)
	}
	d, err := svc.GetCount(ctx, a, id)
	if err != nil || d.Status != CountDraft || !strings.HasPrefix(d.DocNo, "OP-MAIN-") || !strings.HasSuffix(d.DocNo, "-0001") {
		t.Fatalf("draf: %+v %v", d, err)
	}
	if n, err := svc.AddCountItems(ctx, a, id, AddItemsInput{ItemIDs: []uuid.UUID{x, y}}); err != nil || n != 2 {
		t.Fatalf("tambah: %d %v", n, err)
	}
	// Menambah lagi ('semua') hanya menambah barang yang belum ada.
	if n, err := svc.AddCountItems(ctx, a, id, AddItemsInput{All: true}); err != nil || n != 1 {
		t.Fatalf("tambah semua: %d %v", n, err)
	}

	// Penjualan di sela penghitungan: x 10 → 8. Hasil hitung 7 terhadap snapshot 10 → selisih −3 → saldo akhir 5 (bukan 7).
	if _, err := e.apply(t, e.mv(x, BucketDisplay, "-2", RefSale)); err != nil {
		t.Fatal(err)
	}
	if err := svc.SetCounted(ctx, a, id, x, "7"); err != nil {
		t.Fatal(err)
	}
	for _, bad := range []string{"-1", "abc", "1.2345", "6,5"} {
		if err := svc.SetCounted(ctx, a, id, y, bad); err == nil {
			t.Errorf("hasil hitung %q seharusnya ditolak", bad)
		}
	}
	if err := svc.SetCounted(ctx, a, id, y, "6.5"); err != nil {
		t.Fatal(err)
	}

	d, _ = svc.GetCount(ctx, a, id)
	if d.Lines != 3 || d.Counted != 2 || d.Differences != 2 {
		t.Errorf("ringkasan draf: lines=%d counted=%d diff=%d", d.Lines, d.Counted, d.Differences)
	}
	// Nilai selisih pratinjau: x −3 × 1000 = −3000; y +1.5 × 0 = 0.
	if d.DiffMinus != "-3000.00" || d.DiffValue != "-3000.00" {
		t.Errorf("nilai selisih: plus=%s minus=%s total=%s", d.DiffPlus, d.DiffMinus, d.DiffValue)
	}

	if err := svc.CompleteCount(ctx, a, id); err != nil {
		t.Fatal(err)
	}
	if got := e.balance(t, x, BucketDisplay); got != "5" {
		t.Errorf("saldo x = %s, seharusnya 5 (8 − 3)", got)
	}
	if got := e.balance(t, y, BucketDisplay); got != "6.5" {
		t.Errorf("saldo y = %s", got)
	}
	if got := e.balance(t, z, BucketDisplay); got != "" && got != "0" {
		t.Errorf("z tidak dihitung, saldo = %s", got)
	}
	var n int
	if err := e.admin.QueryRow(ctx, `SELECT count(*) FROM stock_movements WHERE ref_type='OPNAME' AND ref_id=$1`, id).Scan(&n); err != nil || n != 2 {
		t.Errorf("movement OPNAME = %d %v", n, err)
	}

	d, _ = svc.GetCount(ctx, a, id)
	if d.Status != CountCompleted || d.CompletedAt == nil || d.CompletedBy != "Kasir Uji" {
		t.Errorf("selesai: %+v", d.CountSummary)
	}
	// Setelah selesai tidak bisa diubah lagi.
	if err := svc.SetCounted(ctx, a, id, z, "1"); !errors.Is(err, ErrCountNotDraft) {
		t.Errorf("ubah setelah selesai: %v", err)
	}
	if err := svc.CompleteCount(ctx, a, id); !errors.Is(err, ErrCountNotDraft) {
		t.Errorf("selesai dua kali: %v", err)
	}
	if err := svc.CancelCount(ctx, a, id); !errors.Is(err, ErrCountNotDraft) {
		t.Errorf("batal setelah selesai: %v", err)
	}
	if _, err := svc.AddCountItems(ctx, a, id, AddItemsInput{All: true}); !errors.Is(err, ErrCountNotDraft) {
		t.Errorf("tambah setelah selesai: %v", err)
	}
}

func TestCountRules(t *testing.T) {
	e := newEnv(t)
	t.Cleanup(func() {
		_, _ = e.admin.Exec(context.Background(), `DELETE FROM audit_log WHERE tenant_id = $1`, e.tenant)
	})
	svc := NewService(e.app)
	ctx := context.Background()
	a := e.convActor(t, e.outlet)
	goods, svcItem := e.item(t, e.tenant, "goods", false), e.item(t, e.tenant, "service", false)
	foreign := e.item(t, e.other, "goods", false)

	if _, err := svc.CreateCount(ctx, a, "bogus", ""); err == nil {
		t.Error("bucket asing harus ditolak")
	}
	id, _ := svc.CreateCount(ctx, a, BucketWarehouse, "")

	// Jasa dan barang tenant lain tidak ikut.
	if n, err := svc.AddCountItems(ctx, a, id, AddItemsInput{ItemIDs: []uuid.UUID{svcItem, foreign}}); err != nil || n != 0 {
		t.Errorf("jasa/tenant lain: %d %v", n, err)
	}
	if _, err := svc.AddCountItems(ctx, a, id, AddItemsInput{}); err == nil {
		t.Error("tanpa cakupan harus ditolak")
	}
	if n, err := svc.AddCountItems(ctx, a, id, AddItemsInput{ItemIDs: []uuid.UUID{goods}}); err != nil || n != 1 {
		t.Fatalf("tambah: %d %v", n, err)
	}
	// Selesaikan tanpa satu pun hitungan → ditolak, sesi tetap draf.
	if err := svc.CompleteCount(ctx, a, id); !errors.Is(err, ErrCountEmpty) {
		t.Errorf("selesai kosong: %v", err)
	}
	// Hasil hitung dikosongkan kembali = belum dihitung.
	_ = svc.SetCounted(ctx, a, id, goods, "3")
	_ = svc.SetCounted(ctx, a, id, goods, "")
	if err := svc.CompleteCount(ctx, a, id); !errors.Is(err, ErrCountEmpty) {
		t.Errorf("selesai setelah dikosongkan: %v", err)
	}
	// Barang yang dibuang tak bisa diisi.
	if err := svc.RemoveCountItem(ctx, a, id, goods); err != nil {
		t.Fatal(err)
	}
	if err := svc.SetCounted(ctx, a, id, goods, "1"); !errors.Is(err, ErrNotFound) {
		t.Errorf("isi barang yang tak ada: %v", err)
	}

	// Batal tidak mengubah stok; sesudahnya tak bisa diselesaikan.
	_, _ = svc.AddCountItems(ctx, a, id, AddItemsInput{ItemIDs: []uuid.UUID{goods}})
	_ = svc.SetCounted(ctx, a, id, goods, "9")
	if err := svc.CancelCount(ctx, a, id); err != nil {
		t.Fatal(err)
	}
	if got := e.balance(t, goods, BucketWarehouse); got != "" && got != "0" {
		t.Errorf("batal mengubah stok: %s", got)
	}
	if err := svc.CompleteCount(ctx, a, id); !errors.Is(err, ErrCountNotDraft) {
		t.Errorf("selesai setelah batal: %v", err)
	}

	// Akses outlet: aktor tanpa akses ke outlet sesi ditolak; tenant lain tak melihatnya (RLS).
	id2, _ := svc.CreateCount(ctx, a, BucketDisplay, "")
	b := e.convActor(t, e.outlet2)
	if _, err := svc.GetCount(ctx, b, id2); !errors.Is(err, ErrOutletForbidden) {
		t.Errorf("outlet lain: %v", err)
	}
	b.Outlets = map[uuid.UUID]bool{e.outlet: true}
	if _, err := svc.GetCount(ctx, b, id2); err != nil {
		t.Errorf("dengan akses: %v", err)
	}
	c := a
	c.TenantID = e.other
	if _, err := svc.GetCount(ctx, c, id2); !errors.Is(err, ErrNotFound) {
		t.Errorf("tenant lain: %v", err)
	}

	// Daftar: hanya outlet aktif; filter status.
	rows, total, err := svc.ListCounts(ctx, a, CountDraft, "", "", 20, 0)
	if err != nil || total != 1 || len(rows) != 1 || rows[0].ID != id2 {
		t.Errorf("daftar draf: %d %v %v", total, rows, err)
	}
	if _, _, err := svc.ListCounts(ctx, a, "x", "", "", 20, 0); err == nil {
		t.Error("status asing harus ditolak")
	}
}

// Penyesuaian yang membuat stok minus (ada penjualan setelah dihitung) membatalkan seluruh penyelesaian.
func TestCompleteIsAtomic(t *testing.T) {
	e := newEnv(t)
	t.Cleanup(func() {
		_, _ = e.admin.Exec(context.Background(), `DELETE FROM audit_log WHERE tenant_id = $1`, e.tenant)
	})
	svc := NewService(e.app)
	ctx := context.Background()
	a := e.convActor(t, e.outlet)
	x, y := e.item(t, e.tenant, "goods", false), e.item(t, e.tenant, "goods", false)
	e.seed(t, a, svc, x, "4")
	e.seed(t, a, svc, y, "4")

	id, _ := svc.CreateCount(ctx, a, BucketDisplay, "")
	_, _ = svc.AddCountItems(ctx, a, id, AddItemsInput{ItemIDs: []uuid.UUID{x, y}})
	_ = svc.SetCounted(ctx, a, id, x, "0") // selisih −4
	_ = svc.SetCounted(ctx, a, id, y, "9") // selisih +5
	// Setelah dihitung, x terjual habis → pengurangan −4 tidak mungkin.
	if _, err := e.apply(t, e.mv(x, BucketDisplay, "-4", RefSale)); err != nil {
		t.Fatal(err)
	}
	if err := svc.CompleteCount(ctx, a, id); !errors.Is(err, ErrInsufficient) {
		t.Fatalf("seharusnya stok tidak cukup, dapat %v", err)
	}
	if got := e.balance(t, y, BucketDisplay); got != "4" {
		t.Errorf("y berubah walau batal: %s", got)
	}
	if d, _ := svc.GetCount(ctx, a, id); d.Status != CountDraft {
		t.Errorf("status = %s", d.Status)
	}
}

// Dua penyelesaian bersamaan: tepat satu menang; stok tidak berlipat.
func TestCompleteConcurrent(t *testing.T) {
	e := newEnv(t)
	t.Cleanup(func() {
		_, _ = e.admin.Exec(context.Background(), `DELETE FROM audit_log WHERE tenant_id = $1`, e.tenant)
	})
	svc := NewService(e.app)
	ctx := context.Background()
	a := e.convActor(t, e.outlet)
	x := e.item(t, e.tenant, "goods", false)
	e.seed(t, a, svc, x, "10")
	id, _ := svc.CreateCount(ctx, a, BucketDisplay, "")
	_, _ = svc.AddCountItems(ctx, a, id, AddItemsInput{ItemIDs: []uuid.UUID{x}})
	_ = svc.SetCounted(ctx, a, id, x, "12")

	var ok, rejected atomic.Int32
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			switch err := svc.CompleteCount(ctx, a, id); {
			case err == nil:
				ok.Add(1)
			case errors.Is(err, ErrCountNotDraft):
				rejected.Add(1)
			default:
				t.Errorf("galat tak terduga: %v", err)
			}
		}()
	}
	wg.Wait()
	if ok.Load() != 1 || rejected.Load() != 7 {
		t.Errorf("menang=%d ditolak=%d", ok.Load(), rejected.Load())
	}
	if got := e.balance(t, x, BucketDisplay); got != "12" {
		t.Errorf("saldo = %s", got)
	}
}

func TestQuickCount(t *testing.T) {
	e := newEnv(t)
	t.Cleanup(func() {
		_, _ = e.admin.Exec(context.Background(), `DELETE FROM audit_log WHERE tenant_id = $1`, e.tenant)
	})
	svc := NewService(e.app)
	ctx := context.Background()
	a := e.convActor(t, e.outlet)
	x, y, svcItem := e.item(t, e.tenant, "goods", false), e.item(t, e.tenant, "goods", false), e.item(t, e.tenant, "service", false)
	e.seed(t, a, svc, x, "10")
	e.setCost(t, x, "1000")

	// Mode replace: stok x 10 → 4 (selisih −6), y tanpa saldo → 3 (selisih +3). Langsung selesai, satu dokumen.
	id, err := svc.QuickCount(ctx, a, QuickInput{Bucket: BucketDisplay, Mode: ModeReplace, Note: "cek rak",
		Items: []QuickLine{{x, "4"}, {y, "3"}}})
	if err != nil {
		t.Fatal(err)
	}
	if got := e.balance(t, x, BucketDisplay); got != "4" {
		t.Errorf("x = %s, seharusnya 4", got)
	}
	if got := e.balance(t, y, BucketDisplay); got != "3" {
		t.Errorf("y = %s, seharusnya 3", got)
	}
	d, err := svc.GetCount(ctx, a, id)
	if err != nil || d.Status != CountCompleted || d.Kind != KindQuick || d.Mode != ModeReplace || d.Lines != 2 || d.Differences != 2 {
		t.Fatalf("dokumen: %+v %v", d.CountSummary, err)
	}
	if d.DiffMinus != "-6000.00" {
		t.Errorf("nilai kurang = %s", d.DiffMinus)
	}
	for _, l := range d.Items {
		if l.ItemID == x && (l.Snapshot != "10" || l.Counted == nil || *l.Counted != "4" || l.Diff == nil || *l.Diff != "-6") {
			t.Errorf("baris x: %+v", l)
		}
	}
	// Dokumen langsung tak bisa diubah/diselesaikan lagi.
	if err := svc.CompleteCount(ctx, a, id); !errors.Is(err, ErrCountNotDraft) {
		t.Errorf("selesai lagi: %v", err)
	}

	// Mode adjust: x −1 → 3, y +2.5 → 5.5.
	id2, err := svc.QuickCount(ctx, a, QuickInput{Bucket: BucketDisplay, Mode: ModeAdjust, Items: []QuickLine{{x, "-1"}, {y, "2.5"}}})
	if err != nil {
		t.Fatal(err)
	}
	if e.balance(t, x, BucketDisplay) != "3" || e.balance(t, y, BucketDisplay) != "5.5" {
		t.Errorf("setelah adjust: x=%s y=%s", e.balance(t, x, BucketDisplay), e.balance(t, y, BucketDisplay))
	}
	d2, _ := svc.GetCount(ctx, a, id2)
	if d2.Mode != ModeAdjust || !strings.HasSuffix(d2.DocNo, "-0002") {
		t.Errorf("dokumen 2: %+v", d2.CountSummary)
	}

	// Kurang melebihi stok → batal seluruhnya (x tidak berubah, nomor dokumen tak terpakai oleh dokumen setengah jadi).
	if _, err := svc.QuickCount(ctx, a, QuickInput{Bucket: BucketDisplay, Mode: ModeAdjust, Items: []QuickLine{{y, "1"}, {x, "-99"}}}); !errors.Is(err, ErrInsufficient) {
		t.Errorf("minus: %v", err)
	}
	if e.balance(t, y, BucketDisplay) != "5.5" {
		t.Errorf("y harus tetap 5.5, = %s", e.balance(t, y, BucketDisplay))
	}

	// Validasi.
	bad := []QuickInput{
		{Bucket: "x", Mode: ModeReplace, Items: []QuickLine{{x, "1"}}},
		{Bucket: BucketDisplay, Mode: "?", Items: []QuickLine{{x, "1"}}},
		{Bucket: BucketDisplay, Mode: ModeReplace},
		{Bucket: BucketDisplay, Mode: ModeReplace, Items: []QuickLine{{x, "-1"}}},
		{Bucket: BucketDisplay, Mode: ModeAdjust, Items: []QuickLine{{x, "0"}}},
		{Bucket: BucketDisplay, Mode: ModeAdjust, Items: []QuickLine{{x, "1.2345"}}},
		{Bucket: BucketDisplay, Mode: ModeAdjust, Items: []QuickLine{{x, "1"}, {x, "2"}}},
	}
	for i, in := range bad {
		if _, err := svc.QuickCount(ctx, a, in); err == nil {
			t.Errorf("kasus %d seharusnya ditolak", i)
		}
	}
	if _, err := svc.QuickCount(ctx, a, QuickInput{Bucket: BucketDisplay, Mode: ModeReplace, Items: []QuickLine{{svcItem, "1"}}}); !errors.Is(err, ErrNotStocked) {
		t.Errorf("jasa: %v", err)
	}

	// Daftar: filter jenis & cari nomor.
	rows, total, err := svc.ListCounts(ctx, a, "", KindQuick, "", 20, 0)
	if err != nil || total != 2 || len(rows) != 2 {
		t.Errorf("daftar quick: %d %v", total, err)
	}
	if rows, _, _ := svc.ListCounts(ctx, a, "", "", d2.DocNo, 20, 0); len(rows) != 1 {
		t.Errorf("cari nomor: %d", len(rows))
	}
	sum, err := svc.CountsSummary(ctx, a)
	if err != nil || sum.Done != 2 || sum.DiffLines != 4 || sum.Minus != "-7000.00" {
		t.Errorf("ringkasan: %+v %v", sum, err)
	}

	// 10 opname replace bersamaan atas barang yang sama: saldo akhir salah satu nilai yang dimasukkan, bukan campuran.
	var wg sync.WaitGroup
	for i := 1; i <= 10; i++ {
		wg.Add(1)
		go func(v int) {
			defer wg.Done()
			_, _ = svc.QuickCount(ctx, a, QuickInput{Bucket: BucketDisplay, Mode: ModeReplace, Items: []QuickLine{{x, strings.Repeat("1", v)}}})
		}(i)
	}
	wg.Wait()
	ok := false
	for v := 1; v <= 10; v++ {
		if e.balance(t, x, BucketDisplay) == strings.Repeat("1", v) {
			ok = true
		}
	}
	if !ok {
		t.Errorf("saldo akhir bukan salah satu nilai yang dimasukkan: %s", e.balance(t, x, BucketDisplay))
	}
	var _ = atomic.Int32{}
}
