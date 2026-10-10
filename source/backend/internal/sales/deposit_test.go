package sales

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/google/uuid"

	"aciraba/internal/receivable"
	"aciraba/internal/stock"
	"aciraba/internal/wallet"
)

func (e *env) deposit(t *testing.T, member uuid.UUID) string {
	t.Helper()
	acc, err := wallet.NewService(e.app).Account(context.Background(), e.actor(e.tenant), wallet.MemberDeposit, member, 0)
	if err != nil {
		t.Fatal(err)
	}
	return acc.Balance
}

func (e *env) topup(t *testing.T, member uuid.UUID, amount string) {
	t.Helper()
	if _, _, err := wallet.NewService(e.app).Cash(context.Background(), e.actor(e.tenant), wallet.MemberDeposit, wallet.DepTopup, member, key(),
		wallet.CashInput{Amount: jn(amount), MethodID: e.methodID(t, "cash")}); err != nil {
		t.Fatal(err)
	}
}

func depositPay(id uuid.UUID, amount string) PaymentIn {
	return PaymentIn{MethodID: &id, Amount: jn(amount)}
}

// Top-up, bayar nota dengan deposit, saldo kurang ditolak tanpa jejak, tanpa member ditolak, batal/edit nota mengembalikan deposit.
func TestDepositPaysSaleAndIsReturnedOnVoid(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	a := e.actor(e.tenant)
	it := e.item(t, "goods", "10000", "5000", 20, false)
	m := e.newMember(t, 0, true)
	dep := e.methodID(t, "deposit")
	ws := wallet.NewService(e.app)

	// Top-up idempoten: kunci sama = satu baris; isi beda = ditolak.
	k := key()
	in := wallet.CashInput{Amount: jn("50000"), MethodID: e.methodID(t, "cash")}
	if acc, replay, err := ws.Cash(ctx, a, wallet.MemberDeposit, wallet.DepTopup, m, k, in); err != nil || replay || acc.Balance != "50000.00" || acc.Entries[0].DocNo == "" {
		t.Fatalf("top-up: %+v %v %v", acc, replay, err)
	}
	if acc, replay, err := ws.Cash(ctx, a, wallet.MemberDeposit, wallet.DepTopup, m, k, in); err != nil || !replay || acc.Balance != "50000.00" {
		t.Fatalf("top-up ulang: %+v %v %v", acc, replay, err)
	}
	in.Amount = jn("1")
	if _, _, err := ws.Cash(ctx, a, wallet.MemberDeposit, wallet.DepTopup, m, k, in); !errors.Is(err, wallet.ErrKeyMismatch) {
		t.Fatalf("kunci sama isi beda: %v", err)
	}
	// Top-up dari deposit / non-tunai tanpa referensi ditolak.
	var wf wallet.FieldErrors
	if _, _, err := ws.Cash(ctx, a, wallet.MemberDeposit, wallet.DepTopup, m, key(), wallet.CashInput{Amount: jn("1"), MethodID: dep}); !errors.As(err, &wf) || wf["method_id"] != "INVALID" {
		t.Fatalf("top-up dari deposit: %v", err)
	}
	if _, _, err := ws.Cash(ctx, a, wallet.MemberDeposit, wallet.DepTopup, m, key(), wallet.CashInput{Amount: jn("1"), MethodID: e.methodID(t, "transfer")}); !errors.As(err, &wf) || wf["ref_no"] != "REQUIRED" {
		t.Fatalf("transfer tanpa referensi: %v", err)
	}

	q, err := e.svc.Quote(ctx, a, Request{Lines: []LineIn{line(it, "3")}, MemberID: &m})
	if err != nil || q.Member == nil || q.Member.Deposit != "50000.00" {
		t.Fatalf("quote deposit: %+v %v", q.Member, err)
	}
	s, _, err := e.svc.Create(ctx, a, key(), Request{Lines: []LineIn{line(it, "3")}, MemberID: &m, Payments: []PaymentIn{depositPay(dep, "30000")}})
	if err != nil || s.Payments[0].Method != "deposit" {
		t.Fatalf("bayar dengan deposit: %+v %v", s, err)
	}
	if b := e.deposit(t, m); b != "20000.00" {
		t.Fatalf("saldo setelah bayar %s", b)
	}
	// Saldo kurang: nota tidak tersimpan, stok tak berubah.
	before := e.count(t, "sales")
	_, _, err = e.svc.Create(ctx, a, key(), Request{Lines: []LineIn{line(it, "3")}, MemberID: &m, Payments: []PaymentIn{depositPay(dep, "30000")}})
	fieldErr(t, err, "payments.0.amount", "DEPOSIT_INSUFFICIENT")
	if e.count(t, "sales") != before || e.stockOf(t, it) != "17" {
		t.Fatalf("nota gagal meninggalkan jejak: sales %d stok %s", e.count(t, "sales"), e.stockOf(t, it))
	}
	// Tanpa member ditolak; campuran deposit + tunai boleh.
	_, _, err = e.svc.Create(ctx, a, key(), Request{Lines: []LineIn{line(it, "1")}, Payments: []PaymentIn{depositPay(dep, "10000")}})
	fieldErr(t, err, "payments.0.amount", "MEMBER_REQUIRED")
	mixed, _, err := e.svc.Create(ctx, a, key(), Request{Lines: []LineIn{line(it, "3")}, MemberID: &m,
		Payments: []PaymentIn{depositPay(dep, "20000"), pay("cash", "10000")}})
	if err != nil {
		t.Fatalf("campuran: %v", err)
	}
	if b := e.deposit(t, m); b != "0.00" {
		t.Fatalf("saldo setelah campuran %s", b)
	}
	// Batal nota → deposit yang dipakai kembali; ringkasan laci tidak menghitung deposit sebagai uang.
	_, ap := e.editor(t)
	if _, err := e.svc.Void(ctx, a, s.ID, VoidInput{Reason: "pelanggan batal", Approval: ap}); err != nil {
		t.Fatal(err)
	}
	if b := e.deposit(t, m); b != "30000.00" {
		t.Fatalf("saldo setelah batal %s", b)
	}
	// Edit nota campuran: deposit lama dikembalikan lalu dipotong lagi sesuai revisi.
	edited, _, err := e.svc.Edit(ctx, a, mixed.ID, key(), EditRequest{Request: Request{Lines: []LineIn{line(it, "2")}, MemberID: &m,
		Payments: []PaymentIn{depositPay(dep, "20000")}, Approval: ap}, Reason: "kurangi qty"})
	if err != nil || edited.Total != "20000.00" {
		t.Fatalf("edit: %+v %v", edited, err)
	}
	if b := e.deposit(t, m); b != "30000.00" {
		t.Fatalf("saldo setelah edit %s", b)
	}
}

// Pembayaran deposit serentak tidak pernah membuat saldo minus.
func TestDepositNeverOverspendsUnderConcurrency(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	a := e.actor(e.tenant)
	it := e.item(t, "goods", "10000", "5000", 50, false)
	m := e.newMember(t, 0, true)
	dep := e.methodID(t, "deposit")
	e.topup(t, m, "30000")
	var wg sync.WaitGroup
	var mu sync.Mutex
	ok := 0
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, _, err := e.svc.Create(ctx, a, key(), Request{Lines: []LineIn{line(it, "1")}, MemberID: &m, Payments: []PaymentIn{depositPay(dep, "10000")}}); err == nil {
				mu.Lock()
				ok++
				mu.Unlock()
			}
		}()
	}
	wg.Wait()
	if ok != 3 || e.deposit(t, m) != "0.00" {
		t.Fatalf("berhasil %d (ingin 3), saldo %s", ok, e.deposit(t, m))
	}
	// Tenant lain tidak bisa melihat deposit member ini (RLS: member tidak ada baginya).
	if _, err := wallet.NewService(e.app).Account(ctx, e.actor(e.other), wallet.MemberDeposit, m, 0); !errors.Is(err, wallet.ErrNotFound) {
		t.Fatalf("lintas tenant: %v", err)
	}
	var n int
	if err := e.app.QueryRow(ctx, `SELECT count(*) FROM member_deposit_movements`).Scan(&n); err != nil || n != 0 {
		t.Fatalf("tanpa tenant terlihat %d baris (%v)", n, err)
	}
}

// Retur ke deposit, batal retur menarik deposit + mengeluarkan stok Retur + memulihkan poin; retur tunai tak bisa dibatalkan.
func TestSaleReturnToDepositAndVoid(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	a := e.actor(e.tenant)
	a.Outlets = map[uuid.UUID]bool{e.outlet: true}
	it := e.item(t, "goods", "10000", "5000", 20, false)
	m := e.newMember(t, 0, true)
	dep := e.methodID(t, "deposit")
	sale, _, err := e.svc.Create(ctx, a, key(), Request{Lines: []LineIn{line(it, "4")}, MemberID: &m, Payments: []PaymentIn{pay("cash", "40000")}})
	if err != nil || sale.PointsEarned != 4 {
		t.Fatalf("nota: %+v %v", sale, err)
	}
	// Retur 2 ke deposit (tanpa referensi), lalu 1 tunai.
	r1, _, err := e.svc.CreateSaleReturn(ctx, a, key(), saleReturnRequest(sale.ID, 1, "2", &dep))
	if err != nil || r1.Refund != "20000.00" || r1.RefundMethod != "deposit" || r1.Status != "completed" {
		t.Fatalf("retur ke deposit: %+v %v", r1, err)
	}
	if b := e.deposit(t, m); b != "20000.00" {
		t.Fatalf("deposit setelah retur %s", b)
	}
	cash := saleRefundMethod(t, e, "cash")
	r2, _, err := e.svc.CreateSaleReturn(ctx, a, key(), saleReturnRequest(sale.ID, 1, "1", &cash))
	if err != nil {
		t.Fatal(err)
	}
	if p, _ := e.memberPoints(t, m); p != 1 {
		t.Fatalf("poin setelah 2 retur %d", p)
	}
	if _, err := e.svc.VoidSaleReturn(ctx, a, r2.ID, "salah"); !errors.Is(err, ErrSaleReturnRefunded) {
		t.Fatalf("batal retur tunai: %v", err)
	}
	if _, err := e.svc.VoidSaleReturn(ctx, a, r1.ID, "x"); err == nil {
		t.Fatal("alasan terlalu pendek diterima")
	}
	// Batal retur ke deposit: deposit ditarik, stok keluar dari Retur, poin kembali, sisa yang bisa diretur bertambah.
	v, err := e.svc.VoidSaleReturn(ctx, a, r1.ID, "barang tidak jadi diretur")
	if err != nil || v.Status != "void" || v.VoidReason == "" || v.VoidedAt == nil {
		t.Fatalf("batal retur: %+v %v", v, err)
	}
	if b := e.deposit(t, m); b != "0.00" {
		t.Fatalf("deposit setelah batal retur %s", b)
	}
	if got := saleBucket(t, e, it, stock.BucketReturns); got != "1" {
		t.Fatalf("stok retur setelah batal %s", got)
	}
	if p, _ := e.memberPoints(t, m); p != 3 {
		t.Fatalf("poin setelah batal retur %d", p)
	}
	src, err := e.svc.SaleReturnSource(ctx, a, sale.ID)
	if err != nil || src.Lines[0].Returnable != "3" || src.Lines[0].Returned != "1" {
		t.Fatalf("sumber setelah batal: %+v %v", src, err)
	}
	if _, err := e.svc.VoidSaleReturn(ctx, a, r1.ID, "dua kali"); !errors.Is(err, ErrSaleReturnInactive) {
		t.Fatalf("batal dua kali: %v", err)
	}
	list, err := e.svc.ListSaleReturns(ctx, a, SaleReturnListParams{Status: "void"})
	if err != nil || len(list.Data) != 1 || list.Data[0].Status != "void" {
		t.Fatalf("filter void: %+v %v", list, err)
	}

	// Deposit hasil retur yang sudah terpakai → batal retur ditolak, tidak ada yang berubah.
	r3, _, err := e.svc.CreateSaleReturn(ctx, a, key(), saleReturnRequest(sale.ID, 1, "1", &dep))
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := e.svc.Create(ctx, a, key(), Request{Lines: []LineIn{line(it, "1")}, MemberID: &m, Payments: []PaymentIn{depositPay(dep, "10000")}}); err != nil {
		t.Fatal(err)
	}
	if _, err := e.svc.VoidSaleReturn(ctx, a, r3.ID, "batal"); !errors.Is(err, wallet.ErrInsufficient) {
		t.Fatalf("batal retur deposit terpakai: %v", err)
	}
	if got := saleBucket(t, e, it, stock.BucketReturns); got != "2" {
		t.Fatalf("stok retur setelah batal ditolak %s", got)
	}
	// Retur ke deposit untuk nota tanpa member ditolak.
	plain, _, err := e.svc.Create(ctx, a, key(), Request{Lines: []LineIn{line(it, "1")}, Payments: []PaymentIn{pay("cash", "10000")}})
	if err != nil {
		t.Fatal(err)
	}
	_, _, err = e.svc.CreateSaleReturn(ctx, a, key(), saleReturnRequest(plain.ID, 1, "1", &dep))
	fieldErr(t, err, "refund_method_id", "MEMBER_REQUIRED")
}

// Piutang bisa dibayar dari deposit; batal retur yang memotong piutang ditolak bila piutang dibayar sesudahnya.
func TestReceivablePaidWithDepositAndReturnLock(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	a := e.scoped()
	it := e.item(t, "goods", "10000", "5000", 20, false)
	m := e.creditMember(t, "0")
	dep := e.methodID(t, "deposit")
	sale, _, err := e.svc.Create(ctx, a, key(), creditReq(it, "5", &m, "", nil)) // piutang 50.000
	if err != nil {
		t.Fatal(err)
	}
	// Retur 1 memotong piutang 10.000 (tanpa dana kembali) → masih bisa dibatalkan.
	r1, _, err := e.svc.CreateSaleReturn(ctx, a, key(), saleReturnRequest(sale.ID, 1, "1", nil))
	if err != nil || r1.ReceivableCut != "10000.00" || r1.Refund != "0.00" {
		t.Fatalf("retur potong piutang: %+v %v", r1, err)
	}
	rs := receivable.NewService(e.app)
	var rf receivable.FieldErrors
	if _, _, err := rs.Pay(ctx, a, sale.Credit.ID, key(), receivable.PayInput{MethodID: dep, Amount: jn("5000")}); !errors.As(err, &rf) || rf["amount"] != "DEPOSIT_INSUFFICIENT" {
		t.Fatalf("bayar piutang dengan deposit kosong: %v", err)
	}
	e.topup(t, m, "15000")
	if _, _, err := rs.Pay(ctx, a, sale.Credit.ID, key(), receivable.PayInput{MethodID: dep, Amount: jn("15000")}); err != nil {
		t.Fatal(err)
	}
	if b := e.deposit(t, m); b != "0.00" {
		t.Fatalf("deposit setelah bayar piutang %s", b)
	}
	// Kredit pemasok bukan alat bayar piutang.
	if _, _, err := rs.Pay(ctx, a, sale.Credit.ID, key(), receivable.PayInput{MethodID: e.methodID(t, "supplier_credit"), Amount: jn("1")}); !errors.As(err, &rf) || rf["method_id"] != "INVALID" {
		t.Fatalf("kredit pemasok untuk piutang: %v", err)
	}
	if _, err := e.svc.VoidSaleReturn(ctx, a, r1.ID, "batal"); !errors.Is(err, ErrSaleReturnLocked) {
		t.Fatalf("batal retur setelah piutang dibayar: %v", err)
	}
	// Pelunasan kolektif dari deposit.
	e.topup(t, m, "25000")
	if _, _, err := rs.Settle(ctx, a, key(), receivable.SettleInput{MemberID: m, Mode: "auto", MethodID: dep, Amount: jn("25000")}); err != nil {
		t.Fatal(err)
	}
	if b := e.deposit(t, m); b != "0.00" {
		t.Fatalf("deposit setelah pelunasan %s", b)
	}
	d, err := rs.Get(ctx, a, sale.Credit.ID)
	if err != nil || d.Balance != "0.00" {
		t.Fatalf("piutang setelah pelunasan: %+v %v", d.Row, err)
	}
}

// Ringkasan laci kasir memuat arus uang lain (bayar piutang, top-up/tarik deposit, dana kembali retur) per metode.
func TestCashierDrawerIncludesOtherFlows(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	a := e.scoped()
	it := e.item(t, "goods", "10000", "5000", 20, false)
	m := e.creditMember(t, "0")
	cash := e.methodID(t, "cash")
	dep := e.methodID(t, "deposit")
	s1, _, err := e.svc.Create(ctx, a, key(), Request{Lines: []LineIn{line(it, "3")}, Payments: []PaymentIn{pay("cash", "50000")}}) // tunai 30.000
	if err != nil {
		t.Fatal(err)
	}
	credit, _, err := e.svc.Create(ctx, a, key(), creditReq(it, "2", &m, "", nil)) // piutang 20.000
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := receivable.NewService(e.app).Pay(ctx, a, credit.Credit.ID, key(), receivable.PayInput{MethodID: cash, Amount: jn("5000")}); err != nil {
		t.Fatal(err)
	}
	e.topup(t, m, "40000")
	if _, _, err := wallet.NewService(e.app).Cash(ctx, a, wallet.MemberDeposit, wallet.DepWithdraw, m, key(), wallet.CashInput{Amount: jn("15000"), MethodID: cash}); err != nil {
		t.Fatal(err)
	}
	if _, _, err := e.svc.CreateSaleReturn(ctx, a, key(), saleReturnRequest(s1.ID, 1, "1", &cash)); err != nil { // tunai keluar 10.000
		t.Fatal(err)
	}
	// Nota dari deposit (bukan uang di laci).
	if _, _, err := e.svc.Create(ctx, a, key(), Request{Lines: []LineIn{line(it, "1")}, MemberID: &m, Payments: []PaymentIn{depositPay(dep, "10000")}}); err != nil {
		t.Fatal(err)
	}
	res, err := e.svc.List(ctx, a, "", "", "")
	if err != nil {
		t.Fatal(err)
	}
	flows := map[string]string{}
	for _, f := range res.Flows {
		flows[f.Source] = f.Amount
	}
	if flows["receivable_payment"] != "5000.00" || flows["deposit_topup"] != "40000.00" || flows["deposit_withdraw"] != "-15000.00" || flows["sale_return"] != "-10000.00" {
		t.Fatalf("arus: %+v", res.Flows)
	}
	// Laci tunai = 30.000 + 5.000 + 40.000 − 15.000 − 10.000 = 50.000; deposit tidak muncul di laci.
	if len(res.Drawer) != 1 || res.Drawer[0].Kind != "cash" || res.Drawer[0].Amount != "50000.00" {
		t.Fatalf("laci: %+v", res.Drawer)
	}
}
