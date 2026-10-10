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
	"aciraba/internal/wallet"
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

	// Retur yang sudah menerima dana kembali tidak bisa dibatalkan; stok & HPP tidak berubah.
	if _, err := e.svc.VoidReturn(ctx, a, r.ID, "barang tidak jadi diretur"); !errors.Is(err, ErrReturnRefunded) {
		t.Fatalf("batal retur ber-refund: %v", err)
	}
	if e.bal(t, it, stock.BucketReturns) != "1" {
		t.Errorf("stok retur setelah batal ditolak %s", e.bal(t, it, stock.BucketReturns))
	}
	if avg, _ := e.cost(t, e.outlet, it); avg != "1123.53" {
		t.Errorf("HPP setelah batal ditolak %s", avg)
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
	it := e.item(t, "goods", "1000", 10)                                               // stok 10 @1.000
	p, _, err := e.svc.Create(ctx, a, key(), e.creditReq(line(it, "10", "0", "1300"))) // hutang 13.000, HPP 1.150
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := svc.Pay(ctx, a, p.Payable.ID, key(), payable.PayInput{MethodID: m, Amount: num("7000")}); err != nil {
		t.Fatal(err)
	}
	e.toReturns(t, it, "10")

	// Retur A = 2 × 1.300 = 2.600, seluruhnya memotong hutang. HPP mundur: (20×1150 − 2×1300) ÷ 18 = 1.133,33.
	rA, _, err := e.svc.CreateReturn(ctx, a, key(), retReq(p.ID, rl(0, "2")))
	if err != nil || rA.PayableCut != "2600.00" || rA.Refund != "0.00" {
		t.Fatalf("retur A: %+v %v", rA, err)
	}
	if avg, _ := e.cost(t, e.outlet, it); avg != "1133.33" {
		t.Errorf("HPP setelah retur A %s", avg)
	}
	// Pembayaran terjadi SEBELUM retur dan tanpa dana kembali → retur boleh dibatalkan; potongan hutang & HPP kembali.
	if _, err := e.svc.VoidReturn(ctx, a, rA.ID, "batal"); err != nil {
		t.Fatal(err)
	}
	if d, _ := svc.Get(ctx, a, p.Payable.ID); d.Balance != "6000.00" || d.Returned != "0.00" {
		t.Errorf("hutang setelah batal retur: %+v", d)
	}
	if avg, _ := e.cost(t, e.outlet, it); avg != "1150.00" {
		t.Errorf("HPP setelah batal retur A %s", avg)
	}

	// Retur B = 1.300 memotong hutang, lalu dibayar → retur tak bisa dibatalkan lagi.
	rB, _, err := e.svc.CreateReturn(ctx, a, key(), retReq(p.ID, rl(0, "1")))
	if err != nil || rB.PayableCut != "1300.00" || rB.Refund != "0.00" {
		t.Fatalf("retur B: %+v %v", rB, err)
	}
	var fe payable.FieldErrors
	if _, _, err := svc.Pay(ctx, a, p.Payable.ID, key(), payable.PayInput{MethodID: m, Amount: num("4700.01")}); !errors.As(err, &fe) || fe["amount"] != "OVERPAID" {
		t.Errorf("lebih bayar setelah retur: %v", err)
	}
	if _, _, err := svc.Pay(ctx, a, p.Payable.ID, key(), payable.PayInput{MethodID: m, Amount: num("500")}); err != nil {
		t.Fatal(err)
	}
	if _, err := e.svc.VoidReturn(ctx, a, rB.ID, "batal"); !errors.Is(err, ErrReturnLocked) {
		t.Errorf("batal retur setelah dibayar: %v", err)
	}

	// Retur C = 5 × 1.300 = 6.500: memotong sisa hutang 4.200, kelebihan 2.300 dikembalikan pemasok.
	req := retReq(p.ID, rl(0, "5"))
	req.RefundMethodID = &m
	rC, _, err := e.svc.CreateReturn(ctx, a, key(), req)
	if err != nil || rC.PayableCut != "4200.00" || rC.Refund != "2300.00" {
		t.Fatalf("retur C: %+v %v", rC, err)
	}
	d, err := svc.Get(ctx, a, p.Payable.ID)
	if err != nil || d.Balance != "0.00" || d.Returned != "5500.00" || d.Status != "paid" {
		t.Fatalf("hutang setelah retur C: %+v %v", d, err)
	}
	// Pemilih nota menandai nota kredit yang sudah lunas (sisa 0) agar petugas tak mengira masih ada hutang.
	if list, err := e.svc.ReturnablePurchases(ctx, a, ""); err != nil || len(list) != 1 || list[0].PayableBalance == nil || *list[0].PayableBalance != "0.00" {
		t.Errorf("pemilih nota lunas: %+v %v", list, err)
	}
	if _, _, err := svc.Pay(ctx, a, p.Payable.ID, key(), payable.PayInput{MethodID: m, Amount: num("1")}); !errors.As(err, &fe) || fe["amount"] != "SETTLED" {
		t.Errorf("bayar setelah lunas oleh retur: %v", err)
	}
	pp, err := e.svc.Get(ctx, a, p.ID)
	if err != nil || pp.Payable.Balance != "0.00" || pp.Payable.Returned != "5500.00" {
		t.Errorf("detail nota: %+v %v", pp.Payable, err)
	}
	// Retur C menerima dana kembali → tidak bisa dibatalkan.
	if _, err := e.svc.VoidReturn(ctx, a, rC.ID, "batal"); !errors.Is(err, ErrReturnRefunded) {
		t.Errorf("batal retur ber-refund: %v", err)
	}
	// Ringkasan daftar hutang memperhitungkan retur.
	lst, err := svc.List(ctx, a, payable.ListParams{Status: "all"})
	if err != nil || lst.Summary.Outstanding != "0.00" || len(lst.Data) != 1 || lst.Data[0].Returned != "5500.00" {
		t.Errorf("daftar hutang: %+v %v", lst, err)
	}
}

// Dana kembali lewat metode non-tunai wajib memakai referensi (nomor transfer/bukti).
func TestReturnNonCashRefundNeedsReference(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	a := e.actor(e.outlet)
	tr := uuid.New()
	if _, err := e.admin.Exec(ctx, `INSERT INTO payment_methods (id, tenant_id, name, kind) VALUES ($1, $2, 'Transfer', 'transfer')`, tr, e.tenant); err != nil {
		t.Fatal(err)
	}
	it := e.item(t, "goods", "1000", 0)
	p, _, err := e.svc.Create(ctx, a, key(), e.req(line(it, "4", "0", "1000")))
	if err != nil {
		t.Fatal(err)
	}
	e.toReturns(t, it, "1")
	req := retReq(p.ID, rl(0, "1"))
	req.RefundMethodID = &tr
	var fe FieldErrors
	if _, _, err := e.svc.CreateReturn(ctx, a, key(), req); !errors.As(err, &fe) || fe["refund_ref"] != "REQUIRED" {
		t.Fatalf("transfer tanpa referensi: %v", err)
	}
	req.RefundRef = "TRF-123"
	if r, _, err := e.svc.CreateReturn(ctx, a, key(), req); err != nil || r.RefundRef != "TRF-123" || r.RefundMethodName != "Transfer" {
		t.Fatalf("transfer dengan referensi: %+v %v", r, err)
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
	in.TaxPct = num("11")                       // PPN 110,00
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

func (e *env) internalMethod(t *testing.T, kind, name string) uuid.UUID {
	t.Helper()
	id := uuid.New()
	if _, err := e.admin.Exec(context.Background(), `INSERT INTO payment_methods (id, tenant_id, name, kind, is_system) VALUES ($1, $2, $3, $4, true)`, id, e.tenant, name, kind); err != nil {
		t.Fatal(err)
	}
	return id
}

func (e *env) credit(t *testing.T) string {
	t.Helper()
	acc, err := wallet.NewService(e.app).Account(context.Background(), e.actor(e.outlet), wallet.SupplierCredit, e.supplier, 0)
	if err != nil {
		t.Fatal(err)
	}
	return acc.Balance
}

// Dana kembali retur → kredit pemasok; kredit dipakai bayar hutang (per nota & kolektif), dicairkan, dan batal retur menarik
// kreditnya lagi (ditolak bila sudah terpakai).
func TestReturnToSupplierCredit(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	a := e.actor(e.outlet)
	cash := e.method(t)
	cr := e.internalMethod(t, "supplier_credit", "Kredit Pemasok")
	dep := e.internalMethod(t, "deposit", "Deposit Member")
	ps := payable.NewService(e.app)
	ws := wallet.NewService(e.app)
	it := e.item(t, "goods", "1000", 0)
	p, _, err := e.svc.Create(ctx, a, key(), e.req(line(it, "10", "0", "1000"))) // tunai
	if err != nil {
		t.Fatal(err)
	}
	e.toReturns(t, it, "6")

	// Retur 2 → kredit 2.000 (tanpa referensi). Deposit member ditolak sebagai tujuan.
	req := retReq(p.ID, rl(0, "2"))
	req.RefundMethodID = &dep
	var fe FieldErrors
	if _, _, err := e.svc.CreateReturn(ctx, a, key(), req); !errors.As(err, &fe) || fe["refund_method_id"] != "INVALID" {
		t.Fatalf("retur ke deposit member: %v", err)
	}
	req.RefundMethodID = &cr
	r1, _, err := e.svc.CreateReturn(ctx, a, key(), req)
	if err != nil || r1.Refund != "2000.00" || r1.RefundMethod != "supplier_credit" {
		t.Fatalf("retur ke kredit: %+v %v", r1, err)
	}
	if c := e.credit(t); c != "2000.00" {
		t.Fatalf("kredit %s", c)
	}

	// Hutang 5.000 dibayar 1.500 dari kredit; kredit kurang ditolak; deposit ditolak.
	cp, _, err := e.svc.Create(ctx, a, key(), e.creditReq(line(it, "5", "0", "1000")))
	if err != nil {
		t.Fatal(err)
	}
	var pf payable.FieldErrors
	if _, _, err := ps.Pay(ctx, a, cp.Payable.ID, key(), payable.PayInput{MethodID: dep, Amount: num("1")}); !errors.As(err, &pf) || pf["method_id"] != "INVALID" {
		t.Fatalf("bayar hutang dengan deposit: %v", err)
	}
	if _, _, err := ps.Pay(ctx, a, cp.Payable.ID, key(), payable.PayInput{MethodID: cr, Amount: num("1500")}); err != nil {
		t.Fatal(err)
	}
	if _, _, err := ps.Pay(ctx, a, cp.Payable.ID, key(), payable.PayInput{MethodID: cr, Amount: num("1000")}); !errors.As(err, &pf) || pf["amount"] != "CREDIT_INSUFFICIENT" {
		t.Fatalf("kredit kurang: %v", err)
	}
	if c := e.credit(t); c != "500.00" {
		t.Fatalf("kredit setelah bayar %s", c)
	}
	// Kredit sudah terpakai → batal retur ditolak, stok tidak berubah.
	if _, err := e.svc.VoidReturn(ctx, a, r1.ID, "batal"); !errors.Is(err, wallet.ErrInsufficient) {
		t.Fatalf("batal retur kredit terpakai: %v", err)
	}
	if e.bal(t, it, stock.BucketReturns) != "4" {
		t.Fatalf("stok retur %s", e.bal(t, it, stock.BucketReturns))
	}
	// Retur kedua ke kredit lalu dibatalkan: kredit ditarik, barang kembali ke Retur.
	req2 := retReq(p.ID, rl(0, "1"))
	req2.RefundMethodID = &cr
	r2, _, err := e.svc.CreateReturn(ctx, a, key(), req2)
	if err != nil {
		t.Fatal(err)
	}
	if v, err := e.svc.VoidReturn(ctx, a, r2.ID, "batal"); err != nil || v.Status != "void" {
		t.Fatalf("batal retur kredit: %+v %v", v, err)
	}
	if c := e.credit(t); c != "500.00" || e.bal(t, it, stock.BucketReturns) != "4" {
		t.Fatalf("setelah batal: kredit %s stok %s", c, e.bal(t, it, stock.BucketReturns))
	}
	// Pelunasan kolektif dari kredit (retur 1 lagi = +1.000 → kredit 1.500).
	req3 := retReq(p.ID, rl(0, "1"))
	req3.RefundMethodID = &cr
	if _, _, err := e.svc.CreateReturn(ctx, a, key(), req3); err != nil {
		t.Fatal(err)
	}
	if _, _, err := ps.Settle(ctx, a, key(), payable.SettleInput{SupplierID: e.supplier, Mode: "auto", MethodID: cr, Amount: num("1000")}); err != nil {
		t.Fatal(err)
	}
	if d, _ := ps.Get(ctx, a, cp.Payable.ID); d.Balance != "2500.00" {
		t.Fatalf("hutang setelah pelunasan %s", d.Balance)
	}
	// Pencairan kredit: pemasok membayar sisa kredit ke toko.
	var wf wallet.FieldErrors
	if _, _, err := ws.Cash(ctx, a, wallet.SupplierCredit, wallet.CrCashOut, e.supplier, key(), wallet.CashInput{Amount: num("501"), MethodID: cash}); !errors.As(err, &wf) || wf["amount"] != "BALANCE_INSUFFICIENT" {
		t.Fatalf("pencairan melebihi kredit: %v", err)
	}
	acc, _, err := ws.Cash(ctx, a, wallet.SupplierCredit, wallet.CrCashOut, e.supplier, key(), wallet.CashInput{Amount: num("500"), MethodID: cash})
	if err != nil || acc.Balance != "0.00" || acc.Entries[0].Kind != wallet.CrCashOut {
		t.Fatalf("pencairan: %+v %v", acc, err)
	}
	list, _, err := ws.CreditList(ctx, a, "", false, "", 10)
	if err != nil || len(list) != 1 || list[0].Balance != "0.00" {
		t.Fatalf("daftar kredit: %+v %v", list, err)
	}
	if list, _, _ := ws.CreditList(ctx, a, "", true, "", 10); len(list) != 0 {
		t.Fatalf("daftar kredit > 0: %+v", list)
	}
}
