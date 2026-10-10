package sales

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/shopspring/decimal"

	"aciraba/internal/receivable"
	"aciraba/internal/stock"
)

func saleRefundMethod(t *testing.T, e *env, kind string) uuid.UUID {
	t.Helper()
	var id uuid.UUID
	if err := e.admin.QueryRow(context.Background(), `SELECT id FROM payment_methods WHERE tenant_id=$1 AND kind=$2 ORDER BY created_at LIMIT 1`, e.tenant, kind).Scan(&id); err != nil {
		t.Fatal(err)
	}
	return id
}

func saleBucket(t *testing.T, e *env, item uuid.UUID, bucket string) string {
	t.Helper()
	var qty decimal.Decimal
	if err := e.admin.QueryRow(context.Background(), `SELECT coalesce(qty,0) FROM stock_balances WHERE tenant_id=$1 AND outlet_id=$2 AND item_id=$3 AND bucket=$4`, e.tenant, e.outlet, item, bucket).Scan(&qty); err != nil && !errors.Is(err, pgx.ErrNoRows) {
		t.Fatal(err)
	}
	return qty.String()
}

func saleReturnRequest(saleID uuid.UUID, position int, qty string, method *uuid.UUID) SaleReturnRequest {
	return SaleReturnRequest{SaleID: saleID, Lines: []SaleReturnLineIn{{Position: position, Qty: jsonNumber(qty)}}, RefundMethodID: method}
}

func jsonNumber(value string) json.Number { return json.Number(value) }

func TestSaleReturnPartialValueRefundAndEditGuard(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	a := e.actor(e.tenant)
	item := e.item(t, "goods", "1000", "500", 10, false)
	sale, _, err := e.svc.Create(ctx, a, key(), Request{Lines: []LineIn{line(item, "3")}, Discount: "300", ApplyTax: true,
		Payments: []PaymentIn{pay("cash", "2997")}})
	if err != nil {
		t.Fatal(err)
	}
	method := saleRefundMethod(t, e, "transfer")
	first := saleReturnRequest(sale.ID, 1, "1", &method)
	first.RefundRef = "TR-001"
	quote, err := e.svc.QuoteSaleReturn(ctx, a, first)
	if err != nil || quote.Subtotal != "1000.00" || quote.Discount != "100.00" || quote.TaxAmount != "99.00" || quote.Total != "999.00" || quote.Refund != "999.00" {
		var fields FieldErrors
		errors.As(err, &fields)
		t.Fatalf("quote retur pertama: %+v fields=%#v err=%v", quote, fields, err)
	}
	idemKey := key()
	created, replayed, err := e.svc.CreateSaleReturn(ctx, a, idemKey, first)
	if err != nil || replayed || created.Refund != "999.00" || created.RefundMethodName != "Transfer" || created.RefundRef != "TR-001" {
		t.Fatalf("retur pertama: %+v replay=%v err=%v", created, replayed, err)
	}
	replay, wasReplay, err := e.svc.CreateSaleReturn(ctx, a, idemKey, first)
	if err != nil || !wasReplay || replay.ID != created.ID {
		t.Fatalf("replay retur: %+v replay=%v err=%v", replay, wasReplay, err)
	}
	second := saleReturnRequest(sale.ID, 1, "2", &method)
	second.RefundRef = "TR-002"
	last, _, err := e.svc.CreateSaleReturn(ctx, a, key(), second)
	if err != nil || last.Subtotal != "2000.00" || last.Discount != "200.00" || last.TaxAmount != "198.00" || last.Total != "1998.00" {
		t.Fatalf("retur akhir: %+v err=%v", last, err)
	}
	if saleBucket(t, e, item, stock.BucketReturns) != "3" || saleBucket(t, e, item, stock.BucketDisplay) != "7" {
		t.Fatalf("stok retur/display = %s/%s", saleBucket(t, e, item, stock.BucketReturns), saleBucket(t, e, item, stock.BucketDisplay))
	}
	source, err := e.svc.SaleReturnSource(ctx, a, sale.ID)
	if err != nil || source.Lines[0].Returnable != "0" {
		t.Fatalf("sumber setelah retur: %+v %v", source, err)
	}
	if _, err := e.svc.Void(ctx, a, sale.ID, VoidInput{Reason: "salah input"}); !errors.Is(err, ErrSaleHasReturns) {
		t.Fatalf("nota beretur harus terkunci dari pembatalan: %v", err)
	}
	if e.count(t, "sales_returns") != 2 || e.count(t, "stock_movements") < 3 {
		t.Fatal("dokumen retur atau ledger stok tidak tercatat")
	}
}

func TestSaleReturnCutsReceivableThenRefundsSelectedMethod(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	a := e.actor(e.tenant)
	a.Outlets = map[uuid.UUID]bool{e.outlet: true}
	item := e.item(t, "goods", "1000", "500", 10, false)
	memberID := e.newMember(t, 0, true)
	sale, _, err := e.svc.Create(ctx, a, key(), Request{Lines: []LineIn{line(item, "5")}, MemberID: &memberID, Credit: true,
		Payments: []PaymentIn{pay("cash", "2000")}})
	if err != nil {
		t.Fatal(err)
	}
	method := saleRefundMethod(t, e, "ewallet")
	request := saleReturnRequest(sale.ID, 1, "4", &method)
	request.RefundRef = "EW-REF-001"
	quote, err := e.svc.QuoteSaleReturn(ctx, a, request)
	if err != nil || quote.ReceivableBalance != "3000.00" || quote.ReceivableCut != "3000.00" || quote.Refund != "1000.00" {
		t.Fatalf("quote piutang/refund: %+v %v", quote, err)
	}
	created, _, err := e.svc.CreateSaleReturn(ctx, a, key(), request)
	if err != nil || created.ReceivableCut != "3000.00" || created.Refund != "1000.00" || created.RefundMethodName != "E-Wallet" {
		t.Fatalf("retur kredit: %+v %v", created, err)
	}
	detail, err := e.svc.Get(ctx, a, sale.ID)
	if err != nil || detail.Credit == nil || detail.Credit.Balance != "0.00" {
		t.Fatalf("saldo piutang setelah retur: %+v %v", detail.Credit, err)
	}
	receivableService := receivable.NewService(e.app)
	receivableDetail, err := receivableService.Get(ctx, a, sale.Credit.ID)
	if err != nil || receivableDetail.Returned != "3000.00" || receivableDetail.Balance != "0.00" || receivableDetail.Status != "paid" {
		t.Fatalf("daftar piutang setelah retur: %+v %v", receivableDetail.Row, err)
	}
	receivableList, err := receivableService.List(ctx, a, receivable.ListParams{Status: "all"})
	if err != nil || receivableList.Summary.Outstanding != "0.00" || len(receivableList.Data) != 1 || receivableList.Data[0].Returned != "3000.00" {
		t.Fatalf("ringkasan piutang setelah retur: %+v %v", receivableList, err)
	}
	var fields receivable.FieldErrors
	if _, _, err := receivableService.Pay(ctx, a, sale.Credit.ID, key(), receivable.PayInput{MethodID: method, Amount: json.Number("1")}); !errors.As(err, &fields) || fields["amount"] != "SETTLED" {
		t.Fatalf("piutang yang sudah dipotong retur tak boleh ditagih lagi: %v", err)
	}
}

func TestSaleReturnEarnedPointsCanGoNegativeAndRedeemedPointsReturn(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	a := e.actor(e.tenant)
	item := e.item(t, "goods", "10000", "5000", 10, false)
	memberID := e.newMember(t, 0, true)
	first, _, err := e.svc.Create(ctx, a, key(), Request{Lines: []LineIn{line(item, "1")}, MemberID: &memberID, Payments: []PaymentIn{pay("cash", "10000")}})
	if err != nil || first.PointsEarned != 1 {
		t.Fatalf("nota yang menghasilkan poin: %+v %v", first, err)
	}
	second, _, err := e.svc.Create(ctx, a, key(), Request{Lines: []LineIn{line(item, "1")}, MemberID: &memberID, RedeemPoints: 1,
		Payments: []PaymentIn{pay("cash", "9900")}})
	if err != nil || second.PointsRedeemed != 1 {
		t.Fatalf("nota yang memakai poin: %+v %v", second, err)
	}
	if points, _ := e.memberPoints(t, memberID); points != 0 {
		t.Fatalf("saldo sebelum retur = %d; want 0", points)
	}
	method := saleRefundMethod(t, e, "cash")
	returned, _, err := e.svc.CreateSaleReturn(ctx, a, key(), saleReturnRequest(first.ID, 1, "1", &method))
	if err != nil || returned.PointsEarnedReversed != 1 {
		t.Fatalf("retur dengan poin terpakai: %+v %v", returned, err)
	}
	if points, lifetime := e.memberPoints(t, memberID); points != -1 || lifetime != 0 {
		t.Fatalf("saldo/lifetime setelah retur = %d/%d; want -1/0", points, lifetime)
	}
	_, _, err = e.svc.Create(ctx, a, key(), Request{Lines: []LineIn{line(item, "1")}, MemberID: &memberID, RedeemPoints: 1,
		Payments: []PaymentIn{pay("cash", "9900")}})
	fieldErr(t, err, "redeem_points", codePointsInsufficient)
}

func TestConcurrentSaleReturnsCannotExceedSoldQty(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	a := e.actor(e.tenant)
	item := e.item(t, "goods", "100", "50", 10, false)
	sale, _, err := e.svc.Create(ctx, a, key(), Request{Lines: []LineIn{line(item, "8")}, Payments: []PaymentIn{pay("cash", "800")}})
	if err != nil {
		t.Fatal(err)
	}
	method := saleRefundMethod(t, e, "cash")
	var wg sync.WaitGroup
	var mu sync.Mutex
	accepted, rejected := 0, 0
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _, err := e.svc.CreateSaleReturn(ctx, a, key(), saleReturnRequest(sale.ID, 1, "2", &method))
			mu.Lock()
			defer mu.Unlock()
			if err == nil {
				accepted++
			} else {
				var fields FieldErrors
				if errors.As(err, &fields) && fields["lines.0.qty"] == "TOO_HIGH" {
					rejected++
				} else {
					t.Errorf("retur serentak gagal dengan galat lain: %v", err)
				}
			}
		}()
	}
	wg.Wait()
	if accepted != 4 || rejected != 4 {
		t.Fatalf("retur diterima/ditolak = %d/%d; want 4/4", accepted, rejected)
	}
	if saleBucket(t, e, item, stock.BucketReturns) != "8" {
		t.Fatalf("qty di bucket Retur = %s; want 8", saleBucket(t, e, item, stock.BucketReturns))
	}
}

func TestSaleReturnTenantIsolation(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	item := e.item(t, "goods", "100", "50", 5, false)
	sale, _, err := e.svc.Create(ctx, e.actor(e.tenant), key(), Request{Lines: []LineIn{line(item, "1")}, Payments: []PaymentIn{pay("cash", "100")}})
	if err != nil {
		t.Fatal(err)
	}
	other := e.actor(e.other)
	if _, err := e.svc.SaleReturnSource(ctx, other, sale.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("nota tenant lain harus tak terlihat: %v", err)
	}
}

// Biaya metode yang ditagihkan ke pelanggan (surcharge) tidak ikut dikembalikan, juga saat nota habis diretur.
func TestSaleReturnDoesNotRefundSurcharge(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	a := e.actor(e.tenant)
	item := e.item(t, "goods", "1000", "0", 100, false)
	qris := e.method(t, e.tenant, "QRIS Retur", "ewallet", true)
	e.exec(t, `UPDATE payment_methods SET fee_pct = 0.7, fee_flat = 0, fee_bearer = 'customer' WHERE id = $1`, qris)
	sale, _, err := e.svc.Create(ctx, a, key(), Request{Lines: []LineIn{line(item, "100")}, Payments: []PaymentIn{payID(qris, "100000")}})
	if err != nil || sale.Surcharge != "700.00" {
		t.Fatalf("nota: %+v %v", sale, err)
	}
	cash := saleRefundMethod(t, e, "cash")
	for i, qty := range []string{"40", "60"} {
		r, _, err := e.svc.CreateSaleReturn(ctx, a, key(), saleReturnRequest(sale.ID, 1, qty, &cash))
		want := decimal.RequireFromString(qty).Mul(decimal.NewFromInt(1000)).StringFixed(2)
		if err != nil || r.Surcharge != "0.00" || r.Total != want || r.Refund != want {
			t.Fatalf("retur %d: surcharge=%s total=%s refund=%s want=%s %v", i+1, r.Surcharge, r.Total, r.Refund, want, err)
		}
	}
}

// Detail nota tetap memuat semua baris asli, tetapi menandai qty yang diretur per baris, daftar dokumen retur
// (termasuk yang dibatalkan), nilai bersih, dan gerakan stok retur.
func TestSaleDetailShowsReturns(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	a := e.actor(e.tenant)
	itemA := e.item(t, "goods", "1000", "500", 10, false)
	itemB := e.item(t, "goods", "2000", "900", 10, false)
	itemC := e.item(t, "goods", "3000", "1500", 10, false)
	member := e.newMember(t, 0, true)
	sale, _, err := e.svc.Create(ctx, a, key(), Request{Lines: []LineIn{line(itemA, "2"), line(itemB, "1"), line(itemC, "1")}, MemberID: &member,
		Payments: []PaymentIn{pay("cash", "7000")}})
	if err != nil {
		t.Fatal(err)
	}
	method := saleRefundMethod(t, e, "cash")
	retB, _, err := e.svc.CreateSaleReturn(ctx, a, key(), saleReturnRequest(sale.ID, 2, "1", &method))
	if err != nil {
		t.Fatal(err)
	}
	deposit := e.methodID(t, "deposit") // hanya retur ke deposit yang boleh dibatalkan
	retA, _, err := e.svc.CreateSaleReturn(ctx, a, key(), saleReturnRequest(sale.ID, 1, "1", &deposit))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := e.svc.VoidSaleReturn(ctx, a, retA.ID, "salah pilih barang"); err != nil {
		t.Fatal(err)
	}

	d, err := e.svc.Detail(ctx, a, sale.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(d.Lines) != 3 || d.Total != "7000.00" {
		t.Fatalf("nota asal harus utuh: %d baris, total %s", len(d.Lines), d.Total)
	}
	if d.Lines[0].ReturnedQty != "0" || d.Lines[1].ReturnedQty != "1" || d.Lines[2].ReturnedQty != "0" {
		t.Fatalf("qty diretur per baris: %s/%s/%s", d.Lines[0].ReturnedQty, d.Lines[1].ReturnedQty, d.Lines[2].ReturnedQty)
	}
	if d.ReturnedTotal != "2000.00" || d.NetTotal != "5000.00" {
		t.Fatalf("total retur/bersih = %s/%s", d.ReturnedTotal, d.NetTotal)
	}
	if len(d.Returns) != 2 || d.Returns[0].ID != retA.ID || d.Returns[0].Status != "void" || d.Returns[0].VoidReason != "salah pilih barang" ||
		d.Returns[1].ID != retB.ID || d.Returns[1].Status != "completed" || d.Returns[1].Refund != "2000.00" {
		t.Fatalf("daftar retur: %+v", d.Returns)
	}
	if l := d.Returns[1].Lines; len(l) != 1 || l[0].SalePosition != 2 || l[0].Qty != "1" {
		t.Fatalf("baris retur B: %+v", l)
	}
	returnMoves := 0
	for _, m := range d.Stock {
		if m.Type == "SALE_RETURN" {
			returnMoves++
		}
	}
	if returnMoves != 3 { // retur B masuk, retur A masuk lalu keluar lagi saat dibatalkan
		t.Fatalf("gerakan stok retur di detail = %d, mau 3", returnMoves)
	}

	plain, _, err := e.svc.Create(ctx, a, key(), Request{Lines: []LineIn{line(itemC, "1")}, Payments: []PaymentIn{pay("cash", "3000")}})
	if err != nil {
		t.Fatal(err)
	}
	pd, err := e.svc.Detail(ctx, a, plain.ID)
	if err != nil || len(pd.Returns) != 0 || pd.ReturnedTotal != "0.00" || pd.NetTotal != "3000.00" || pd.Lines[0].ReturnedQty != "0" {
		t.Fatalf("nota tanpa retur: %+v %v", pd.Returns, err)
	}
}
