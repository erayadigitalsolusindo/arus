package stock

import (
	"context"
	"errors"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"aciraba/internal/authz"
)

func trim0(s string) string {
	if strings.Contains(s, ".") {
		s = strings.TrimRight(strings.TrimRight(s, "0"), ".")
	}
	return s
}

func (e *env) balanceAt(t *testing.T, outlet, item uuid.UUID, bucket string) string {
	t.Helper()
	var q string
	if err := e.admin.QueryRow(context.Background(), `SELECT coalesce(sum(qty), 0)::text FROM stock_balances WHERE tenant_id=$1 AND outlet_id=$2 AND item_id=$3 AND bucket=$4`,
		e.tenant, outlet, item, bucket).Scan(&q); err != nil {
		t.Fatal(err)
	}
	return trim0(q)
}

func (e *env) costAt(t *testing.T, outlet, item uuid.UUID) string {
	t.Helper()
	var c string
	if err := e.admin.QueryRow(context.Background(), `SELECT coalesce(oc.avg_cost, i.avg_cost)::text FROM items i
		LEFT JOIN item_outlet_costs oc ON oc.tenant_id = i.tenant_id AND oc.item_id = i.id AND oc.outlet_id = $2 WHERE i.id = $1`, item, outlet).Scan(&c); err != nil {
		t.Fatal(err)
	}
	return c
}

func (e *env) transferDocs(t *testing.T) int {
	t.Helper()
	var n int
	if err := e.admin.QueryRow(context.Background(), `SELECT count(*) FROM stock_transfers WHERE tenant_id = $1`, e.tenant).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

// seedAt mengisi saldo awal display di cabang aktor.
func (e *env) seedAt(t *testing.T, svc *Service, a authz.Actor, item uuid.UUID, qty string) {
	t.Helper()
	e.seed(t, a, svc, item, qty)
}

func tin(to uuid.UUID, fb, tb string, lines ...TransferLineInput) TransferInput {
	return TransferInput{ToOutletID: to, FromBucket: fb, ToBucket: tb, Lines: lines}
}

func tl(item uuid.UUID, qty string) TransferLineInput {
	return TransferLineInput{ItemID: item, Qty: qty}
}

func TestTransferFlowAndCost(t *testing.T) {
	e := newEnv(t)
	t.Cleanup(func() {
		_, _ = e.admin.Exec(context.Background(), `DELETE FROM audit_log WHERE tenant_id = $1`, e.tenant)
	})
	svc := NewService(e.app)
	ctx := context.Background()
	a1, a2 := e.convActor(t, e.outlet), e.convActor(t, e.outlet2)
	item := e.item(t, e.tenant, "goods", false)
	e.seedAt(t, svc, a1, item, "20")
	e.seedAt(t, svc, a2, item, "10")
	e.setCost(t, item, "1000") // HPP cabang pusat (awal barang)
	if _, err := e.admin.Exec(ctx, `INSERT INTO item_outlet_costs (tenant_id, outlet_id, item_id, avg_cost, last_cost) VALUES ($1,$2,$3,2000,1800)`, e.tenant, e.outlet2, item); err != nil {
		t.Fatal(err)
	}

	tr, replayed, err := svc.SendTransfer(ctx, a1, key(), tin(e.outlet2, BucketDisplay, BucketWarehouse, tl(item, "10")))
	if err != nil || replayed {
		t.Fatalf("kirim: %v", err)
	}
	if tr.Status != TransferSent || !strings.HasPrefix(tr.DocNo, "MT-MAIN-") || !strings.HasSuffix(tr.DocNo, "-0001") {
		t.Errorf("dokumen: %+v", tr)
	}
	// Setelah kirim: stok asal berkurang, tujuan belum berubah (dalam perjalanan).
	if e.balanceAt(t, e.outlet, item, BucketDisplay) != "10" || e.balanceAt(t, e.outlet2, item, BucketWarehouse) != "0" {
		t.Errorf("setelah kirim: asal=%s tujuan=%s", e.balanceAt(t, e.outlet, item, BucketDisplay), e.balanceAt(t, e.outlet2, item, BucketWarehouse))
	}
	if tr.Lines[0].UnitCost != "1000.00" || tr.Lines[0].QtyReceived != nil {
		t.Errorf("baris: %+v", tr.Lines[0])
	}

	// Pengirim tidak punya akses ke cabang tujuan → tidak bisa menerima.
	if err := svc.ReceiveTransfer(ctx, a1, tr.ID, nil); !errors.Is(err, ErrOutletForbidden) {
		t.Errorf("terima oleh pengirim: %v", err)
	}
	// Validasi qty diterima.
	if err := svc.ReceiveTransfer(ctx, a2, tr.ID, []ReceiveLine{{ItemID: item, Qty: "11"}}); err == nil {
		t.Error("qty terima melebihi kirim harus ditolak")
	}
	if err := svc.ReceiveTransfer(ctx, a2, tr.ID, []ReceiveLine{{ItemID: uuid.New(), Qty: "1"}}); err == nil {
		t.Error("barang asing harus ditolak")
	}
	// Terima 8 dari 10: HPP tujuan = (10 × 2000 + 8 × 1000) ÷ 18 = 1555,56 (stok tujuan sebelum = 10 di display).
	if err := svc.ReceiveTransfer(ctx, a2, tr.ID, []ReceiveLine{{ItemID: item, Qty: "8"}}); err != nil {
		t.Fatalf("terima: %v", err)
	}
	if e.balanceAt(t, e.outlet2, item, BucketWarehouse) != "8" || e.balanceAt(t, e.outlet, item, BucketDisplay) != "10" {
		t.Errorf("setelah terima: tujuan=%s", e.balanceAt(t, e.outlet2, item, BucketWarehouse))
	}
	if c := e.costAt(t, e.outlet2, item); c != "1555.56" {
		t.Errorf("HPP tujuan = %s", c)
	}
	if c := e.costAt(t, e.outlet, item); c != "1000.00" {
		t.Errorf("HPP asal berubah: %s", c)
	}
	var last string
	_ = e.admin.QueryRow(ctx, `SELECT last_cost::text FROM item_outlet_costs WHERE tenant_id=$1 AND outlet_id=$2 AND item_id=$3`, e.tenant, e.outlet2, item).Scan(&last)
	if last != "1800.00" {
		t.Errorf("HPP terakhir tujuan berubah: %s", last)
	}
	got, err := svc.GetTransfer(ctx, a2, tr.ID)
	if err != nil || got.Status != TransferReceived || got.QtyShort != "2.000" || *got.Lines[0].QtyReceived != "8.000" || got.ReceivedBy != "Kasir Uji" {
		t.Errorf("dokumen akhir: %+v %v", got, err)
	}
	if err := svc.ReceiveTransfer(ctx, a2, tr.ID, nil); !errors.Is(err, ErrTransferNotPending) {
		t.Errorf("terima ulang: %v", err)
	}
	if err := svc.CancelTransfer(ctx, a1, tr.ID, "salah kirim"); !errors.Is(err, ErrTransferNotPending) {
		t.Errorf("batal setelah diterima: %v", err)
	}
	var out, in int
	_ = e.admin.QueryRow(ctx, `SELECT count(*) FILTER (WHERE ref_type='TRANSFER_OUT'), count(*) FILTER (WHERE ref_type='TRANSFER_IN') FROM stock_movements WHERE ref_id=$1`, tr.ID).Scan(&out, &in)
	if out != 1 || in != 1 {
		t.Errorf("movement out=%d in=%d", out, in)
	}
	if e.auditCount(t, "stock.transfer_send") != 1 || e.auditCount(t, "stock.transfer_receive") != 1 {
		t.Error("audit tidak lengkap")
	}

	// Terima penuh tanpa daftar baris; tujuan tanpa stok sebelumnya → HPP = HPP asal.
	item2 := e.item(t, e.tenant, "goods", false)
	e.seedAt(t, svc, a1, item2, "5")
	e.setCost(t, item2, "300")
	tr2, _, err := svc.SendTransfer(ctx, a1, key(), tin(e.outlet2, BucketDisplay, BucketDisplay, tl(item2, "5")))
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.ReceiveTransfer(ctx, a2, tr2.ID, nil); err != nil {
		t.Fatal(err)
	}
	if e.balanceAt(t, e.outlet2, item2, BucketDisplay) != "5" || e.costAt(t, e.outlet2, item2) != "300.00" {
		t.Errorf("terima penuh: stok=%s hpp=%s", e.balanceAt(t, e.outlet2, item2, BucketDisplay), e.costAt(t, e.outlet2, item2))
	}
}

func TestTransferCancelRestoresSource(t *testing.T) {
	e := newEnv(t)
	svc := NewService(e.app)
	ctx := context.Background()
	a1, a2 := e.convActor(t, e.outlet), e.convActor(t, e.outlet2)
	item := e.item(t, e.tenant, "goods", false)
	e.seedAt(t, svc, a1, item, "6")
	tr, _, err := svc.SendTransfer(ctx, a1, key(), tin(e.outlet2, BucketDisplay, BucketDisplay, tl(item, "4")))
	if err != nil {
		t.Fatal(err)
	}
	if e.balanceAt(t, e.outlet, item, BucketDisplay) != "2" {
		t.Fatalf("asal = %s", e.balanceAt(t, e.outlet, item, BucketDisplay))
	}
	if err := svc.CancelTransfer(ctx, a1, tr.ID, "x"); err == nil {
		t.Error("alasan terlalu pendek harus ditolak")
	}
	// Penerima juga boleh menolak (batal).
	if err := svc.CancelTransfer(ctx, a2, tr.ID, "barang tidak sesuai"); err != nil {
		t.Fatal(err)
	}
	if e.balanceAt(t, e.outlet, item, BucketDisplay) != "6" || e.balanceAt(t, e.outlet2, item, BucketDisplay) != "0" {
		t.Errorf("setelah batal: asal=%s tujuan=%s", e.balanceAt(t, e.outlet, item, BucketDisplay), e.balanceAt(t, e.outlet2, item, BucketDisplay))
	}
	got, _ := svc.GetTransfer(ctx, a1, tr.ID)
	if got.Status != TransferCancelled || got.CancelReason != "barang tidak sesuai" {
		t.Errorf("dokumen: %+v", got)
	}
	if err := svc.ReceiveTransfer(ctx, a2, tr.ID, nil); !errors.Is(err, ErrTransferNotPending) {
		t.Errorf("terima setelah batal: %v", err)
	}
	if err := svc.CancelTransfer(ctx, a1, tr.ID, "sekali lagi"); !errors.Is(err, ErrTransferNotPending) {
		t.Errorf("batal ganda: %v", err)
	}
}

func TestTransferBetweenBucketsSameOutlet(t *testing.T) {
	e := newEnv(t)
	svc := NewService(e.app)
	ctx := context.Background()
	a := e.convActor(t, e.outlet)
	item := e.item(t, e.tenant, "goods", false)
	e.seedAt(t, svc, a, item, "10")
	e.setCost(t, item, "500")
	tr, _, err := svc.SendTransfer(ctx, a, key(), tin(e.outlet, BucketDisplay, BucketReturns, tl(item, "3")))
	if err != nil {
		t.Fatal(err)
	}
	if tr.Status != TransferReceived || *tr.Lines[0].QtyReceived != "3.000" || tr.QtyShort != "0.000" {
		t.Errorf("dokumen: %+v", tr)
	}
	if e.balanceAt(t, e.outlet, item, BucketDisplay) != "7" || e.balanceAt(t, e.outlet, item, BucketReturns) != "3" || e.costAt(t, e.outlet, item) != "500.00" {
		t.Errorf("saldo display=%s retur=%s hpp=%s", e.balanceAt(t, e.outlet, item, BucketDisplay), e.balanceAt(t, e.outlet, item, BucketReturns), e.costAt(t, e.outlet, item))
	}
	// Dari bucket yang tidak cukup → batal total, tidak ada dokumen baru.
	before := e.transferDocs(t)
	if _, _, err := svc.SendTransfer(ctx, a, key(), tin(e.outlet, BucketReturns, BucketWarehouse, tl(item, "4"))); !errors.Is(err, ErrInsufficient) {
		t.Errorf("stok retur kurang: %v", err)
	}
	if e.transferDocs(t) != before {
		t.Error("dokumen gagal harus ikut batal")
	}
}

func TestTransferValidationAndTenantBoundary(t *testing.T) {
	e := newEnv(t)
	svc := NewService(e.app)
	ctx := context.Background()
	a := e.convActor(t, e.outlet)
	item, svcItem := e.item(t, e.tenant, "goods", false), e.item(t, e.tenant, "service", false)
	e.seedAt(t, svc, a, item, "10")

	// Cabang milik tenant lain: sama dengan tidak ada.
	foreign := uuid.New()
	if _, err := e.admin.Exec(ctx, `INSERT INTO outlets (id, tenant_id, code, name) VALUES ($1, $2, 'x', 'Asing')`, foreign, e.other); err != nil {
		t.Fatal(err)
	}
	_, _, err := svc.SendTransfer(ctx, a, key(), tin(foreign, BucketDisplay, BucketDisplay, tl(item, "1")))
	var fe FieldErrors
	if !errors.As(err, &fe) || fe["to_outlet_id"] != "INVALID" {
		t.Errorf("lintas tenant: %v", err)
	}
	// Cabang tidak aktif.
	if _, err := e.admin.Exec(ctx, `UPDATE outlets SET active = false WHERE id = $1`, e.outlet2); err != nil {
		t.Fatal(err)
	}
	if _, _, err := svc.SendTransfer(ctx, a, key(), tin(e.outlet2, BucketDisplay, BucketDisplay, tl(item, "1"))); !errors.As(err, &fe) {
		t.Errorf("cabang nonaktif: %v", err)
	}
	if _, err := e.admin.Exec(ctx, `UPDATE outlets SET active = true WHERE id = $1`, e.outlet2); err != nil {
		t.Fatal(err)
	}

	bad := []struct {
		name  string
		in    TransferInput
		field string
	}{
		{"lokasi sama", tin(e.outlet, BucketDisplay, BucketDisplay, tl(item, "1")), "to_bucket"},
		{"tanpa tujuan", tin(uuid.Nil, BucketDisplay, BucketDisplay, tl(item, "1")), "to_outlet_id"},
		{"bucket asing", tin(e.outlet2, "x", BucketDisplay, tl(item, "1")), "from_bucket"},
		{"tanpa baris", tin(e.outlet2, BucketDisplay, BucketDisplay), "lines"},
		{"barang ganda", tin(e.outlet2, BucketDisplay, BucketDisplay, tl(item, "1"), tl(item, "2")), "lines.1.item_id"},
		{"qty nol", tin(e.outlet2, BucketDisplay, BucketDisplay, tl(item, "0")), "lines.0.qty"},
		{"qty 4 desimal", tin(e.outlet2, BucketDisplay, BucketDisplay, tl(item, "1.0001")), "lines.0.qty"},
		{"barang asing", tin(e.outlet2, BucketDisplay, BucketDisplay, tl(uuid.New(), "1")), "lines.0.item_id"},
	}
	for _, c := range bad {
		_, _, err := svc.SendTransfer(ctx, a, key(), c.in)
		if !errors.As(err, &fe) || fe[c.field] == "" {
			t.Errorf("%s: err=%v", c.name, err)
		}
	}
	if _, _, err := svc.SendTransfer(ctx, a, key(), tin(e.outlet2, BucketDisplay, BucketDisplay, tl(svcItem, "1"))); !errors.Is(err, ErrNotStocked) {
		t.Errorf("jasa: %v", err)
	}
	if _, _, err := svc.SendTransfer(ctx, a, "pendek", tin(e.outlet2, BucketDisplay, BucketDisplay, tl(item, "1"))); !errors.Is(err, ErrKeyRequired) {
		t.Errorf("tanpa kunci: %v", err)
	}
	// Satu baris kurang stok → seluruh dokumen batal (barang lain tidak berkurang).
	other := e.item(t, e.tenant, "goods", false)
	e.seedAt(t, svc, a, other, "1")
	if _, _, err := svc.SendTransfer(ctx, a, key(), tin(e.outlet2, BucketDisplay, BucketDisplay, tl(item, "5"), tl(other, "2"))); !errors.Is(err, ErrInsufficient) {
		t.Fatalf("stok kurang: %v", err)
	}
	if e.balanceAt(t, e.outlet, item, BucketDisplay) != "10" || e.transferDocs(t) != 0 {
		t.Errorf("harus atomik: stok=%s dokumen=%d", e.balanceAt(t, e.outlet, item, BucketDisplay), e.transferDocs(t))
	}
	// Aktor tanpa akses ke kedua cabang tidak bisa melihat dokumen; tenant lain tidak menemukannya sama sekali.
	tr, _, err := svc.SendTransfer(ctx, a, key(), tin(e.outlet2, BucketDisplay, BucketDisplay, tl(item, "1")))
	if err != nil {
		t.Fatal(err)
	}
	third := authz.Actor{TenantID: e.tenant, UserID: uuid.New(), OutletID: uuid.New()}
	if _, err := svc.GetTransfer(ctx, third, tr.ID); !errors.Is(err, ErrOutletForbidden) {
		t.Errorf("aktor tanpa akses: %v", err)
	}
	outsider := authz.Actor{TenantID: e.other, UserID: uuid.New(), OutletID: foreign}
	if _, err := svc.GetTransfer(ctx, outsider, tr.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("tenant lain harus tidak ditemukan: %v", err)
	}
	if err := svc.ReceiveTransfer(ctx, outsider, tr.ID, nil); !errors.Is(err, ErrNotFound) {
		t.Errorf("terima lintas tenant: %v", err)
	}
	if err := svc.CancelTransfer(ctx, outsider, tr.ID, "coba batalkan"); !errors.Is(err, ErrNotFound) {
		t.Errorf("batal lintas tenant: %v", err)
	}
	// Akses lewat penugasan cabang (a.Outlets) cukup untuk menerima.
	multi := e.convActor(t, e.outlet)
	multi.Outlets = map[uuid.UUID]bool{e.outlet2: true}
	if err := svc.ReceiveTransfer(ctx, multi, tr.ID, nil); err != nil {
		t.Errorf("akses lewat Outlets: %v", err)
	}
}

func TestTransferIdempotency(t *testing.T) {
	e := newEnv(t)
	svc := NewService(e.app)
	ctx := context.Background()
	a := e.convActor(t, e.outlet)
	item := e.item(t, e.tenant, "goods", false)
	e.seedAt(t, svc, a, item, "100")
	in := tin(e.outlet2, BucketDisplay, BucketDisplay, tl(item, "2"))
	k := key()
	t1, rp, err := svc.SendTransfer(ctx, a, k, in)
	if err != nil || rp {
		t.Fatal(err)
	}
	t2, rp, err := svc.SendTransfer(ctx, a, k, in)
	if err != nil || !rp || t2.ID != t1.ID {
		t.Fatalf("replay: %v rp=%v", err, rp)
	}
	if _, _, err := svc.SendTransfer(ctx, a, k, tin(e.outlet2, BucketDisplay, BucketDisplay, tl(item, "3"))); !errors.Is(err, ErrKeyMismatch) {
		t.Errorf("isi beda: %v", err)
	}
	if e.balanceAt(t, e.outlet, item, BucketDisplay) != "98" {
		t.Errorf("stok = %s", e.balanceAt(t, e.outlet, item, BucketDisplay))
	}
	// 8 kiriman bersamaan dengan kunci sama → tepat satu dokumen, stok berkurang sekali.
	k2 := key()
	var wg sync.WaitGroup
	var created int32
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, rp, err := svc.SendTransfer(ctx, a, k2, in); err == nil && !rp {
				atomic.AddInt32(&created, 1)
			}
		}()
	}
	wg.Wait()
	if created != 1 || e.balanceAt(t, e.outlet, item, BucketDisplay) != "96" || e.transferDocs(t) != 2 {
		t.Errorf("serentak: dibuat=%d stok=%s dokumen=%d", created, e.balanceAt(t, e.outlet, item, BucketDisplay), e.transferDocs(t))
	}
}

func TestTransferConcurrency(t *testing.T) {
	e := newEnv(t)
	svc := NewService(e.app)
	ctx := context.Background()
	a1, a2 := e.convActor(t, e.outlet), e.convActor(t, e.outlet2)
	x, y := e.item(t, e.tenant, "goods", false), e.item(t, e.tenant, "goods", false)
	for _, it := range []uuid.UUID{x, y} {
		e.seedAt(t, svc, a1, it, "100")
		e.seedAt(t, svc, a2, it, "100")
	}
	// Dua arah berlawanan dengan urutan baris berlawanan: tidak boleh deadlock, stok tetap seimbang.
	var wg sync.WaitGroup
	var fails int32
	for i := 0; i < 30; i++ {
		wg.Add(2)
		go func() {
			defer wg.Done()
			if _, _, err := svc.SendTransfer(ctx, a1, key(), tin(e.outlet2, BucketDisplay, BucketDisplay, tl(x, "1"), tl(y, "1"))); err != nil {
				atomic.AddInt32(&fails, 1)
			}
		}()
		go func() {
			defer wg.Done()
			if _, _, err := svc.SendTransfer(ctx, a2, key(), tin(e.outlet, BucketDisplay, BucketDisplay, tl(y, "1"), tl(x, "1"))); err != nil {
				atomic.AddInt32(&fails, 1)
			}
		}()
	}
	wg.Wait()
	if fails != 0 {
		t.Fatalf("kiriman gagal: %d", fails)
	}
	// Barang masih dalam perjalanan: tiap cabang kehilangan 30 dari kirimannya sendiri.
	if e.balanceAt(t, e.outlet, x, BucketDisplay) != "70" || e.balanceAt(t, e.outlet2, y, BucketDisplay) != "70" {
		t.Errorf("saldo: %s %s", e.balanceAt(t, e.outlet, x, BucketDisplay), e.balanceAt(t, e.outlet2, y, BucketDisplay))
	}
	var dup int
	_ = e.admin.QueryRow(ctx, `SELECT count(*) - count(DISTINCT doc_no) FROM stock_transfers WHERE tenant_id = $1`, e.tenant).Scan(&dup)
	if dup != 0 {
		t.Errorf("nomor ganda: %d", dup)
	}

	// Terima, batal, dan terima sebagian bersamaan pada satu dokumen → tepat satu pemenang.
	tr, _, err := svc.SendTransfer(ctx, a1, key(), tin(e.outlet2, BucketDisplay, BucketDisplay, tl(x, "5")))
	if err != nil {
		t.Fatal(err)
	}
	var wins int32
	for i := 0; i < 6; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			var err error
			switch i % 3 {
			case 0:
				err = svc.ReceiveTransfer(ctx, a2, tr.ID, nil)
			case 1:
				err = svc.CancelTransfer(ctx, a1, tr.ID, "batal bersamaan")
			default:
				err = svc.ReceiveTransfer(ctx, a2, tr.ID, []ReceiveLine{{ItemID: x, Qty: "3"}})
			}
			if err == nil {
				atomic.AddInt32(&wins, 1)
			}
		}(i)
	}
	wg.Wait()
	if wins != 1 {
		t.Fatalf("pemenang = %d", wins)
	}
	got, _ := svc.GetTransfer(ctx, a1, tr.ID)
	var sum string
	_ = e.admin.QueryRow(ctx, `SELECT sum(qty)::text FROM stock_balances WHERE tenant_id=$1 AND item_id=$2`, e.tenant, x).Scan(&sum)
	var ledger string
	_ = e.admin.QueryRow(ctx, `SELECT sum(qty_delta)::text FROM stock_movements WHERE tenant_id=$1 AND item_id=$2`, e.tenant, x).Scan(&ledger)
	if trim0(sum) != trim0(ledger) {
		t.Errorf("saldo %s ≠ ledger %s (status %s)", sum, ledger, got.Status)
	}
}

func TestListTransfers(t *testing.T) {
	e := newEnv(t)
	svc := NewService(e.app)
	ctx := context.Background()
	a1, a2 := e.convActor(t, e.outlet), e.convActor(t, e.outlet2)
	item := e.item(t, e.tenant, "goods", false)
	e.seedAt(t, svc, a1, item, "100")
	var ids []uuid.UUID
	for i := 0; i < 5; i++ {
		tr, _, err := svc.SendTransfer(ctx, a1, key(), tin(e.outlet2, BucketDisplay, BucketDisplay, tl(item, "1")))
		if err != nil {
			t.Fatal(err)
		}
		ids = append(ids, tr.ID)
	}
	if err := svc.ReceiveTransfer(ctx, a2, ids[0], nil); err != nil {
		t.Fatal(err)
	}
	// Keyset 2+2+1 tanpa duplikat.
	seen := map[uuid.UUID]bool{}
	cur := ""
	pages := 0
	for {
		pg, err := svc.ListTransfers(ctx, a1, TransferListParams{Direction: "out", Limit: 2, Cursor: cur})
		if err != nil {
			t.Fatal(err)
		}
		for _, r := range pg.Data {
			if seen[r.ID] {
				t.Fatalf("duplikat %s", r.DocNo)
			}
			seen[r.ID] = true
		}
		pages++
		if !pg.HasMore {
			break
		}
		cur = pg.NextCursor
	}
	if len(seen) != 5 || pages != 3 {
		t.Errorf("terbaca %d dokumen dalam %d halaman", len(seen), pages)
	}
	pg, _ := svc.ListTransfers(ctx, a1, TransferListParams{Direction: "out", Status: TransferSent})
	if len(pg.Data) != 4 || pg.Summary.InTransit != 4 || pg.Summary.ToReceive != 0 {
		t.Errorf("filter status: %d %+v", len(pg.Data), pg.Summary)
	}
	in, _ := svc.ListTransfers(ctx, a2, TransferListParams{Direction: "in"})
	if len(in.Data) != 5 || in.Summary.ToReceive != 4 {
		t.Errorf("masuk: %d %+v", len(in.Data), in.Summary)
	}
	if out2, _ := svc.ListTransfers(ctx, a2, TransferListParams{Direction: "out"}); len(out2.Data) != 0 {
		t.Errorf("cabang tujuan tidak punya kiriman keluar: %d", len(out2.Data))
	}
	if q, _ := svc.ListTransfers(ctx, a1, TransferListParams{Q: "%"}); len(q.Data) != 0 {
		t.Errorf("wildcard harus di-escape: %d", len(q.Data))
	}
	if _, err := svc.ListTransfers(ctx, a1, TransferListParams{Cursor: "###"}); err == nil {
		t.Error("kursor rusak harus ditolak")
	}
	if _, err := svc.ListTransfers(ctx, a1, TransferListParams{Direction: "sideways"}); err == nil {
		t.Error("arah asing harus ditolak")
	}
	dest, err := svc.Destinations(ctx, a1)
	if err != nil || len(dest) != 2 {
		t.Errorf("tujuan: %v %v", dest, err)
	}
}

func TestTransferTablesGuarded(t *testing.T) {
	e := newEnv(t)
	svc := NewService(e.app)
	ctx := context.Background()
	a := e.convActor(t, e.outlet)
	item := e.item(t, e.tenant, "goods", false)
	e.seedAt(t, svc, a, item, "5")
	tr, _, err := svc.SendTransfer(ctx, a, key(), tin(e.outlet2, BucketDisplay, BucketDisplay, tl(item, "1")))
	if err != nil {
		t.Fatal(err)
	}
	for _, q := range []string{
		`DELETE FROM stock_transfers WHERE id = $1`,
		`UPDATE stock_transfers SET note = 'x' WHERE id = $1`,
		`UPDATE stock_transfer_lines SET qty_sent = 9 WHERE transfer_id = $1`,
		`DELETE FROM stock_transfer_lines WHERE transfer_id = $1`,
	} {
		q := q
		if err := e.tx(func(ctx context.Context, tx pgx.Tx) error { _, err := tx.Exec(ctx, q, tr.ID); return err }); err == nil {
			t.Errorf("harus ditolak: %s", q)
		}
	}
	// Tanpa tenant terpasang → nol baris (RLS fail-closed).
	var n int
	if err := e.app.QueryRow(ctx, `SELECT count(*) FROM stock_transfers`).Scan(&n); err == nil && n != 0 {
		t.Errorf("tanpa tenant harus 0 baris: %d", n)
	}
}
