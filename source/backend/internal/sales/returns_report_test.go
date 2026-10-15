package sales

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"

	"aciraba/internal/reports/returns"
)

// Laporan retur penjualan (per faktur & per faktur detail): total retur completed, potong
// piutang vs uang kembali, rasio terhadap penjualan, filter member / penyelesaian / pencarian.
func TestSaleReturnReport(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	a := e.actor(e.tenant)
	a.Outlets = map[uuid.UUID]bool{e.outlet: true}
	item := e.item(t, "goods", "1000", "500", 10, false)
	method := saleRefundMethod(t, e, "transfer")
	ret := func(sale uuid.UUID, qty string) SaleReturnRequest {
		r := saleReturnRequest(sale, 1, qty, &method)
		r.RefundRef = "TR-" + uuid.NewString()[:6] // transfer wajib bernomor referensi
		return r
	}

	// Nota A (tunai, diskon 300, PPN): 2 retur parsial 999 + 1.998, dana kembali.
	saleA, _, err := e.svc.Create(ctx, a, key(), Request{Lines: []LineIn{line(item, "3")}, Discount: "300", ApplyTax: true,
		Payments: []PaymentIn{pay("cash", "2997")}})
	if err != nil {
		t.Fatal(err)
	}
	for _, q := range []string{"1", "2"} {
		if _, _, err := e.svc.CreateSaleReturn(ctx, a, key(), ret(saleA.ID, q)); err != nil {
			t.Fatal(err)
		}
	}
	// Nota B (kredit, member): retur 4 → memotong piutang 3.000 + dana kembali 1.000.
	memberID := e.newMember(t, 0, true)
	saleB, _, err := e.svc.Create(ctx, a, key(), Request{Lines: []LineIn{line(item, "5")}, MemberID: &memberID, Credit: true,
		Payments: []PaymentIn{pay("cash", "2000")}})
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := e.svc.CreateSaleReturn(ctx, a, key(), ret(saleB.ID, "4")); err != nil {
		t.Fatal(err)
	}

	svc := returns.NewService(e.app)
	now := time.Now().UTC()
	base := returns.Params{OutletID: e.outlet.String(),
		From: now.AddDate(0, 0, -1).Format("2006-01-02"), To: now.AddDate(0, 0, 1).Format("2006-01-02")}
	run := func(p returns.Params) returns.Result {
		t.Helper()
		res, err := svc.Invoices(ctx, a, returns.KindSale, p)
		if err != nil {
			t.Fatal(err)
		}
		return res
	}

	res := run(base)
	s := res.Summary
	if !res.HasSummary || len(res.Data) != 3 || s.ReturnCount != 3 || s.Total != "6997.00" ||
		s.Offset != "3000.00" || s.Refund != "3997.00" {
		t.Fatalf("ringkasan/baris: %+v data=%d", s, len(res.Data))
	}
	// Dasar rasio = penjualan completed: 2.997 + 5.000 = 7.997 → 6.997 ÷ 7.997 = 87,50%.
	if s.BaseTotal != "7997.00" || s.RatioPct != "87.50" {
		t.Fatalf("rasio: base %s pct %s", s.BaseTotal, s.RatioPct)
	}
	for _, r := range res.Data {
		if r.RefDocNo == "" || r.UserName == "" {
			t.Errorf("relasi kosong: %+v", r)
		}
	}

	// Per faktur detail: baris barang membawa diskon & PPN, Σ jumlah = subtotal − diskon + PPN.
	withLines := base
	withLines.WithLines = true
	det := run(withLines)
	if len(det.Data) != 3 {
		t.Fatalf("detail: %d dokumen", len(det.Data))
	}
	for _, r := range det.Data {
		if len(r.Lines) != 1 {
			t.Errorf("baris retur %s: %+v", r.DocNo, r.Lines)
		}
	}
	if n := len(returns.DetailTable(returns.KindSale, det).Rows); n != 3 {
		t.Errorf("baris tabel ekspor = %d", n)
	}

	// Filter member (bentuk datar), penyelesaian, pencarian nomor nota asal, status.
	f := base
	f.PartyID = memberID.String()
	if r := run(f); len(r.Data) != 1 || r.PartyName != "Budi" || r.Summary.Total != "4000.00" || r.Summary.BaseTotal != "5000.00" {
		t.Errorf("filter member: %d %+v", len(r.Data), r.Summary)
	}
	f = base
	f.Settle = "offset"
	if r := run(f); len(r.Data) != 1 || r.Summary.Offset != "3000.00" || r.SettleLabel != "Potong Piutang" {
		t.Errorf("settle=offset: %+v", r.Summary)
	}
	f.Settle = "refund"
	if r := run(f); len(r.Data) != 3 || r.Summary.Refund != "3997.00" {
		t.Errorf("settle=refund: %+v", r.Summary)
	}
	f = base
	f.Search = saleA.DocNo
	if r := run(f); len(r.Data) != 2 || r.Summary.Total != "2997.00" {
		t.Errorf("q=nota asal: %d %+v", len(r.Data), r.Summary)
	}
	f = base
	f.Status = "void"
	if r := run(f); len(r.Data) != 0 || r.Summary.Total != "0.00" || r.Summary.ReturnCount != 0 {
		t.Errorf("status void: %+v", r.Summary)
	}

	// Paginasi keyset.
	pg := base
	pg.Limit = 2
	p1 := run(pg)
	if len(p1.Data) != 2 || p1.NextCursor == "" || !p1.HasSummary {
		t.Fatalf("halaman 1: %d %q", len(p1.Data), p1.NextCursor)
	}
	pg.Cursor = p1.NextCursor
	p2 := run(pg)
	if len(p2.Data) != 1 || p2.NextCursor != "" || p2.HasSummary {
		t.Fatalf("halaman 2: %+v", p2)
	}
	for _, r := range p1.Data {
		if r.ID == p2.Data[0].ID {
			t.Fatal("dokumen muncul di dua halaman")
		}
	}
}
