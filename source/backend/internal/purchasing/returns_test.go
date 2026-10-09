package purchasing

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/shopspring/decimal"

	"aciraba/internal/payable"
	"aciraba/internal/platform/db"
	"aciraba/internal/stock"
)

// toReturns memindahkan qty barang dari Display ke bucket Retur di outlet utama (seperti mutasi antar bucket).
func (e *env) toReturns(t *testing.T, item uuid.UUID, qty string) {
	t.Helper()
	ctx := context.Background()
	q := decimal.RequireFromString(qty)
	err := db.WithTenant(ctx, e.app, e.tenant, func(tx pgx.Tx) error {
		_, err := stock.ApplyAll(ctx, tx, []stock.Movement{
			{TenantID: e.tenant, OutletID: e.outlet, ItemID: item, Bucket: stock.BucketDisplay, Delta: q.Neg(), RefType: stock.RefTransferOut},
			{TenantID: e.tenant, OutletID: e.outlet, ItemID: item, Bucket: stock.BucketReturns, Delta: q, RefType: stock.RefTransferIn},
		})
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
}

func retReq(purchase uuid.UUID, lines ...ReturnLineIn) ReturnRequest {
	return ReturnRequest{PurchaseID: purchase, Lines: lines}
}

func rl(pos int, qty string) ReturnLineIn { return ReturnLineIn{Position: pos, Qty: num(qty)} }

func TestReturnCashRefundStockAndCost(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	a := e.actor(e.outlet)
	m := e.method(t)
	it := e.item(t, "goods", "1000", 10) // stok 10 @1.000
	in := e.req(line(it, "10", "0", "1300"))
	in.TaxPct = num("11")
	p, _, err := e.svc.Create(ctx, a, key(), in) // HPP → (10×1000 + 10×1300)/20 = 1.150
	if err != nil {
		t.Fatal(err)
	}
	if avg, _ := e.cost(t, e.outlet, it); avg != "1150.00" {
		t.Fatalf("HPP setelah beli %s", avg)
	}

	// Tanpa stok di bucket Retur: ditolak, tidak ada yang berubah.
	if _, _, err := e.svc.CreateReturn(ctx, a, key(), ReturnRequest{PurchaseID: p.ID, Lines: []ReturnLineIn{rl(0, "3")}, RefundMethodID: &m}); !errors.Is(err, stock.ErrInsufficient) {
		t.Fatalf("tanpa stok retur: %v", err)
	}
	e.toReturns(t, it, "4")

	q, err := e.svc.QuoteReturn(ctx, a, retReq(p.ID, rl(0, "3")))
	if err != nil || q.Subtotal != "3900.00" || q.TaxAmount != "429.00" || q.Total != "4329.00" || q.Refund != "4329.00" || q.PayableCut != "0.00" || q.PayableBalance != nil {
		t.Fatalf("pratinjau: %+v %v", q, err)
	}
	var fe FieldErrors
	if _, _, err := e.svc.CreateReturn(ctx, a, key(), retReq(p.ID, rl(0, "3"))); !errors.As(err, &fe) || fe["refund_method_id"] != "REQUIRED" {
		t.Fatalf("dana kembali tanpa metode: %v", err)
	}
	if _, _, err := e.svc.CreateReturn(ctx, a, key(), retReq(p.ID, rl(0, "11"))); !errors.As(err, &fe) || fe["lines.0.qty"] != codeTooHigh {
		t.Fatalf("qty melebihi dibeli: %v", err)
	}
	if _, _, err := e.svc.CreateReturn(ctx, a, key(), retReq(p.ID, rl(7, "1"))); !errors.As(err, &fe) || fe["lines.0.position"] != "INVALID" {
		t.Fatalf("baris asing: %v", err)
	}
	req := retReq(p.ID, rl(0, "3"))
	req.RefundMethodID, req.RefundRef = &m, "KWT-01"
	r, replayed, err := e.svc.CreateReturn(ctx, a, key(), req)
	if err != nil || replayed {
		t.Fatal(err)
	}
	if r.Total != q.Total || r.Refund != "4329.00" || r.RefundMethodName != "Tunai" || r.Status != "completed" || len(r.Lines) != 1 || r.Lines[0].UnitCost != "1300.00" {
		t.Errorf("dokumen: %+v", r)
	}
	if e.bal(t, it, stock.BucketReturns) != "1" || e.bal(t, it, stock.BucketDisplay) != "16" {
		t.Errorf("stok: retur %s display %s", e.bal(t, it, stock.BucketReturns), e.bal(t, it, stock.BucketDisplay))
	}
	// HPP mundur: (20×1150 − 3×1300) ÷ 17 = 1.123,53; harga beli akhir tidak berubah.
	if avg, last := e.cost(t, e.outlet, it); avg != "1123.53" || last != "1300.00" {
		t.Errorf("HPP setelah retur %s / %s", avg, last)
	}
	// Sisa yang bisa diretur = 7.
	src, err := e.svc.ReturnSource(ctx, a, p.ID)
	if err != nil || src.Lines[0].Returnable != "7" || src.Lines[0].Returned != "3" || src.Lines[0].ReturnStock != "1" {
		t.Errorf("sumber: %+v %v", src, err)
	}
	// Nota yang punya retur aktif tidak bisa dibatalkan.
	if _, err := e.svc.Void(ctx, a, p.ID, "salah input"); !errors.Is(err, ErrHasReturns) {
		t.Errorf("batal nota beretur: %v", err)
	}

	// Batal retur: stok kembali ke Retur, HPP maju dengan HPP baris → (17×1123,53 + 3×1300) ÷ 20 = 1.150,00.
	v, err := e.svc.VoidReturn(ctx, a, r.ID, "barang tidak jadi diretur")
	if err != nil || v.Status != "void" || v.VoidReason == "" {
		t.Fatalf("batal retur: %+v %v", v, err)
	}
	if e.bal(t, it, stock.BucketReturns) != "4" {
		t.Errorf("stok retur setelah batal %s", e.bal(t, it, stock.BucketReturns))
	}
	if avg, _ := e.cost(t, e.outlet, it); avg != "1150.00" {
		t.Errorf("HPP setelah batal retur %s", avg)
	}
	if _, err := e.svc.VoidReturn(ctx, a, r.ID, "dua kali"); !errors.Is(err, ErrReturnNotActive) {
		t.Errorf("batal dua kali: %v", err)
	}
	// Setelah retur dibatalkan, nota boleh dibatalkan lagi (stok di Display + Retur cukup).
	if src, _ := e.svc.ReturnSource(ctx, a, p.ID); src.Lines[0].Returnable != "10" {
		t.Errorf("sisa setelah batal retur %s", src.Lines[0].Returnable)
	}
	if n := e.count(t, "audit_log"); n == 0 {
		t.Error("audit kosong")
	}
}

func TestReturnCutsPayableThenRefund(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	a := e.actor(e.outlet)
	m := e.method(t)
	svc := payable.NewService(e.app)
	it := e.item(t, "goods", "1000", 0)
	p, _, err := e.svc.Create(ctx, a, key(), e.creditReq(line(it, "10", "0", "1000"))) // hutang 10.000
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := svc.Pay(ctx, a, p.Payable.ID, key(), payable.PayInput{MethodID: m, Amount: num("7000")}); err != nil {
		t.Fatal(err)
	}
	e.toReturns(t, it, "10")

	// Retur 5 = 5.000: memotong sisa hutang 3.000, kelebihan 2.000 dikembalikan pemasok.
	req := retReq(p.ID, rl(0, "5"))
	req.RefundMethodID = &m
	r1, _, err := e.svc.CreateReturn(ctx, a, key(), req)
	if err != nil || r1.PayableCut != "3000.00" || r1.Refund != "2000.00" {
		t.Fatalf("retur 1: %+v %v", r1, err)
	}
	d, err := svc.Get(ctx, a, p.Payable.ID)
	if err != nil || d.Balance != "0.00" || d.Returned != "3000.00" || d.Status != "paid" {
		t.Fatalf("hutang setelah retur: %+v %v", d, err)
	}
	var fe payable.FieldErrors
	if _, _, err := svc.Pay(ctx, a, p.Payable.ID, key(), payable.PayInput{MethodID: m, Amount: num("1")}); !errors.As(err, &fe) || fe["amount"] != "SETTLED" {
		t.Errorf("bayar setelah lunas oleh retur: %v", err)
	}
	pp, err := e.svc.Get(ctx, a, p.ID)
	if err != nil || pp.Payable.Balance != "0.00" || pp.Payable.Returned != "3000.00" || len(pp.Returns) != 1 {
		t.Errorf("detail nota: %+v %v", pp.Payable, err)
	}

	// Pembayaran terjadi SEBELUM retur → retur boleh dibatalkan; potongan hutang kembali.
	if _, err := e.svc.VoidReturn(ctx, a, r1.ID, "batal"); err != nil {
		t.Fatal(err)
	}
	if d, _ := svc.Get(ctx, a, p.Payable.ID); d.Balance != "3000.00" || d.Returned != "0.00" {
		t.Errorf("hutang setelah batal retur: %+v", d)
	}

	// Retur 2 = 2.000 (seluruhnya memotong hutang), lalu dibayar 1.000 → retur tak bisa dibatalkan lagi.
	r2, _, err := e.svc.CreateReturn(ctx, a, key(), retReq(p.ID, rl(0, "2")))
	if err != nil || r2.PayableCut != "2000.00" || r2.Refund != "0.00" {
		t.Fatalf("retur 2: %+v %v", r2, err)
	}
	if _, _, err := svc.Pay(ctx, a, p.Payable.ID, key(), payable.PayInput{MethodID: m, Amount: num("1000.01")}); !errors.As(err, &fe) || fe["amount"] != "OVERPAID" {
		t.Errorf("lebih bayar setelah retur: %v", err)
	}
	if _, _, err := svc.Pay(ctx, a, p.Payable.ID, key(), payable.PayInput{MethodID: m, Amount: num("1000")}); err != nil {
		t.Fatal(err)
	}
	if _, err := e.svc.VoidReturn(ctx, a, r2.ID, "batal"); !errors.Is(err, ErrReturnLocked) {
		t.Errorf("batal retur setelah dibayar: %v", err)
	}
	// Ringkasan daftar hutang memperhitungkan retur.
	lst, err := svc.List(ctx, a, payable.ListParams{Status: "all"})
	if err != nil || lst.Summary.Outstanding != "0.00" || len(lst.Data) != 1 || lst.Data[0].Returned != "2000.00" {
		t.Errorf("daftar hutang: %+v %v", lst, err)
	}
}

// Nota diretur habis dalam beberapa kali: Σ nilai = nilai baris tepat, Σ PPN = PPN nota tepat.
func TestReturnFullyInPartsSumsExactly(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	a := e.actor(e.outlet)
	m := e.method(t)
	it := e.item(t, "goods", "0", 0)
	in := e.req(line(it, "3", "0", "333.3333")) // nilai baris 1.000,00
	in.TaxPct = num("11")                         // PPN 110,00
	p, _, err := e.svc.Create(ctx, a, key(), in)
	if err != nil || p.Subtotal != "1000.00" || p.TaxAmount != "110.00" {
		t.Fatalf("nota: %+v %v", p, err)
	}
	e.toReturns(t, it, "3")
	r1 := retReq(p.ID, rl(0, "1"))
	r1.RefundMethodID = &m
	a1, _, err := e.svc.CreateReturn(ctx, a, key(), r1)
	if err != nil || a1.Subtotal != "333.33" || a1.TaxAmount != "36.67" {
		t.Fatalf("retur 1: %+v %v", a1, err)
	}
	r2 := retReq(p.ID, rl(0, "2"))
	r2.RefundMethodID = &m
	a2, _, err := e.svc.CreateReturn(ctx, a, key(), r2)
	if err != nil || a2.Subtotal != "666.67" || a2.TaxAmount != "73.33" {
		t.Fatalf("retur 2: %+v %v", a2, err)
	}
	if _, _, err := e.svc.CreateReturn(ctx, a, key(), r2); err == nil {
		t.Error("retur melebihi sisa diterima")
	}
	// Nota yang sudah habis diretur tidak muncul di pemilih nota.
	list, err := e.svc.ReturnablePurchases(ctx, a, "")
	if err != nil || len(list) != 0 {
		t.Errorf("pemilih nota: %+v %v", list, err)
	}
}

func TestReturnIdempotencyAndConcurrency(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	a := e.actor(e.outlet)
	m := e.method(t)
	it := e.item(t, "goods", "1000", 0)
	p, _, err := e.svc.Create(ctx, a, key(), e.req(line(it, "10", "0", "1000")))
	if err != nil {
		t.Fatal(err)
	}
	e.toReturns(t, it, "10")
	req := retReq(p.ID, rl(0, "2"))
	req.RefundMethodID = &m

	// Kunci sama dikirim serentak → tepat satu dokumen.
	k := key()
	var wg sync.WaitGroup
	ids := make(chan uuid.UUID, 6)
	for range 6 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if r, _, err := e.svc.CreateReturn(ctx, a, k, req); err == nil {
				ids <- r.ID
			} else {
				t.Errorf("kunci sama: %v", err)
			}
		}()
	}
	wg.Wait()
	close(ids)
	var first uuid.UUID
	for id := range ids {
		if first == uuid.Nil {
			first = id
		} else if id != first {
			t.Error("kunci sama menghasilkan dokumen berbeda")
		}
	}
	other := req
	other.Note = "beda"
	if _, _, err := e.svc.CreateReturn(ctx, a, k, other); !errors.Is(err, ErrKeyMismatch) {
		t.Errorf("kunci sama isi beda: %v", err)
	}

	// Sisa 8: 8 retur serentak masing-masing 2 → tepat 4 berhasil, stok Retur habis tepat 0.
	var ok, tooHigh int
	var mu sync.Mutex
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _, err := e.svc.CreateReturn(ctx, a, key(), req)
			mu.Lock()
			defer mu.Unlock()
			var fe FieldErrors
			switch {
			case err == nil:
				ok++
			case errors.As(err, &fe) && fe["lines.0.qty"] == codeTooHigh:
				tooHigh++
			default:
				t.Errorf("serentak: %v", err)
			}
		}()
	}
	wg.Wait()
	if ok != 4 || tooHigh != 4 {
		t.Errorf("serentak: ok=%d ditolak=%d", ok, tooHigh)
	}
	if e.bal(t, it, stock.BucketReturns) != "0" || e.count(t, "purchase_returns") != 5 {
		t.Errorf("stok retur %s, dokumen %d", e.bal(t, it, stock.BucketReturns), e.count(t, "purchase_returns"))
	}

	// Daftar keyset: 5 dokumen, 2 per halaman tanpa duplikat.
	seen := map[uuid.UUID]bool{}
	cursor := ""
	for page := 0; page < 5; page++ {
		res, err := e.svc.ListReturns(ctx, a, ReturnListParams{Limit: 2, Cursor: cursor})
		if err != nil {
			t.Fatal(err)
		}
		for _, r := range res.Data {
			if seen[r.ID] {
				t.Error("duplikat di daftar")
			}
			seen[r.ID] = true
		}
		if res.Summary.Count != 5 || res.Summary.Total != "10000.00" {
			t.Errorf("ringkasan %+v", res.Summary)
		}
		if !res.HasMore {
			break
		}
		cursor = res.NextCursor
	}
	if len(seen) != 5 {
		t.Errorf("daftar memuat %d dokumen", len(seen))
	}
}

func TestReturnAccessRules(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	a := e.actor(e.outlet)
	it := e.item(t, "goods", "1000", 0)
	p, _, err := e.svc.Create(ctx, a, key(), e.req(line(it, "5", "0", "1000")))
	if err != nil {
		t.Fatal(err)
	}
	e.toReturns(t, it, "5")
	// Cabang lain yang tidak boleh diakses → 403; boleh diakses tapi bukan cabang aktif → harus pindah outlet.
	b := e.actor(e.outlet2)
	if _, err := e.svc.QuoteReturn(ctx, b, retReq(p.ID, rl(0, "1"))); !errors.Is(err, ErrOutletForbidden) {
		t.Errorf("cabang lain: %v", err)
	}
	b.Outlets[e.outlet] = true
	if _, err := e.svc.QuoteReturn(ctx, b, retReq(p.ID, rl(0, "1"))); !errors.Is(err, ErrOutletMismatch) {
		t.Errorf("bukan cabang aktif: %v", err)
	}
	// Tenant lain tidak melihat nota.
	x := a
	x.TenantID = e.other
	if _, err := e.svc.QuoteReturn(ctx, x, retReq(p.ID, rl(0, "1"))); !errors.Is(err, ErrNotFound) {
		t.Errorf("tenant lain: %v", err)
	}
	// Nota batal tidak bisa diretur.
	p2, _, err := e.svc.Create(ctx, a, key(), e.req(line(it, "2", "0", "1000")))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := e.svc.Void(ctx, a, p2.ID, "salah input"); err != nil {
		t.Fatal(err)
	}
	if _, err := e.svc.QuoteReturn(ctx, a, retReq(p2.ID, rl(0, "1"))); !errors.Is(err, ErrNotReturnable) {
		t.Errorf("nota batal: %v", err)
	}
	// Baris retur tidak bisa diubah/dihapus oleh role aplikasi.
	err = db.WithTenant(ctx, e.app, e.tenant, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `UPDATE purchase_returns SET total = 0`)
		return err
	})
	if err == nil {
		t.Error("UPDATE total retur diizinkan")
	}
}
