package purchasing

import (
	"context"
	"testing"
	"time"

	"aciraba/internal/reports/returns"
)

// Laporan retur pembelian (per faktur & per faktur detail): total hanya retur completed,
// potong hutang vs uang kembali terpisah, rasio terhadap pembelian, filter & pencarian.
func TestPurchaseReturnReport(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	a := e.actor(e.outlet)
	m := e.method(t)
	it := e.item(t, "goods", "1000", 10)

	in := e.req(line(it, "10", "0", "1300"))
	in.TaxPct = num("11")
	cash, _, err := e.svc.Create(ctx, a, key(), in) // 13.000 + PPN 1.430 = 14.430
	if err != nil {
		t.Fatal(err)
	}
	credit, _, err := e.svc.Create(ctx, a, key(), e.creditReq(line(it, "10", "0", "1300"))) // hutang 13.000
	if err != nil {
		t.Fatal(err)
	}
	e.toReturns(t, it, "5")

	refundReq := retReq(cash.ID, rl(0, "3")) // 3.900 + PPN 429 = 4.329 dikembalikan
	refundReq.RefundMethodID = &m
	if _, _, err := e.svc.CreateReturn(ctx, a, key(), refundReq); err != nil {
		t.Fatal(err)
	}
	if _, _, err := e.svc.CreateReturn(ctx, a, key(), retReq(credit.ID, rl(0, "2"))); err != nil { // 2.600 memotong hutang
		t.Fatal(err)
	}

	svc := returns.NewService(e.app)
	now := time.Now().UTC()
	base := returns.Params{OutletID: e.outlet.String(),
		From: now.AddDate(0, 0, -1).Format("2006-01-02"), To: now.AddDate(0, 0, 1).Format("2006-01-02")}
	run := func(p returns.Params) returns.Result {
		t.Helper()
		res, err := svc.Invoices(ctx, a, returns.KindPurchase, p)
		if err != nil {
			t.Fatal(err)
		}
		return res
	}

	res := run(base)
	s := res.Summary
	if !res.HasSummary || len(res.Data) != 2 || s.ReturnCount != 2 || s.Subtotal != "6500.00" || s.TaxAmount != "429.00" ||
		s.Total != "6929.00" || s.Offset != "2600.00" || s.Refund != "4329.00" {
		t.Fatalf("ringkasan/baris: %+v data=%d", s, len(res.Data))
	}
	// Dasar rasio = pembelian completed periode itu: 14.430 + 13.000 = 27.430 → 6.929 ÷ 27.430 = 25,26%.
	if s.BaseTotal != "27430.00" || s.RatioPct != "25.26" {
		t.Fatalf("rasio: base %s pct %s", s.BaseTotal, s.RatioPct)
	}
	if res.Data[0].Lines != nil || res.Data[0].PartyName != "PT Sumber Rejeki" || res.Data[0].RefDocNo == "" {
		t.Fatalf("baris per faktur: %+v", res.Data[0])
	}

	// Per faktur detail: baris barang, jumlah = nilai netto retur.
	withLines := base
	withLines.WithLines = true
	det := run(withLines)
	if len(det.Data) != 2 || len(det.Data[0].Lines) != 1 || len(det.Data[1].Lines) != 1 {
		t.Fatalf("detail: %+v", det.Data)
	}
	var lineSum string
	for _, r := range det.Data {
		lineSum = r.Lines[0].Net
		if r.Lines[0].UnitPrice != "1300.00" || r.Lines[0].Discount != "0.00" {
			t.Errorf("baris %+v", r.Lines[0])
		}
	}
	if lineSum == "" || detailRows(det) != 2 {
		t.Errorf("tabel ekspor: %s", lineSum)
	}

	// Filter penyelesaian, pencarian nomor faktur asal, filter pemasok, status.
	f := base
	f.Settle = "offset"
	if r := run(f); len(r.Data) != 1 || r.Summary.Total != "2600.00" || r.SettleLabel != "Potong Hutang" {
		t.Errorf("settle=offset: %+v", r.Summary)
	}
	f.Settle = "refund"
	if r := run(f); len(r.Data) != 1 || r.Summary.Total != "4329.00" {
		t.Errorf("settle=refund: %+v", r.Summary)
	}
	f = base
	f.Search = credit.DocNo
	if r := run(f); len(r.Data) != 1 || r.Data[0].RefDocNo != credit.DocNo || r.Summary.Total != "2600.00" {
		t.Errorf("q=faktur asal: %+v", r.Summary)
	}
	f = base
	f.PartyID = e.supplier.String()
	if r := run(f); len(r.Data) != 2 || r.PartyName != "PT Sumber Rejeki" {
		t.Errorf("filter pemasok: %d %s", len(r.Data), r.PartyName)
	}
	f.Status = "void"
	if r := run(f); len(r.Data) != 0 || r.Summary.Total != "0.00" {
		t.Errorf("status void: %+v", r.Summary)
	}

	// Paginasi keyset: halaman 1 berisi 1 dokumen + cursor; halaman 2 sisanya tanpa ringkasan.
	pg := base
	pg.Limit = 1
	p1 := run(pg)
	if len(p1.Data) != 1 || p1.NextCursor == "" || !p1.HasSummary {
		t.Fatalf("halaman 1: %d %q", len(p1.Data), p1.NextCursor)
	}
	pg.Cursor = p1.NextCursor
	p2 := run(pg)
	if len(p2.Data) != 1 || p2.NextCursor != "" || p2.HasSummary || p2.Data[0].ID == p1.Data[0].ID {
		t.Fatalf("halaman 2: %+v", p2)
	}

	// Validasi.
	bad := base
	bad.Settle = "x"
	if _, err := svc.Invoices(ctx, a, returns.KindPurchase, bad); err == nil {
		t.Error("settle tak valid harus ditolak")
	}
	bad = base
	bad.OutletID = e.outlet2.String()
	if _, err := svc.Invoices(ctx, a, returns.KindPurchase, bad); err == nil {
		t.Error("outlet di luar akses harus ditolak")
	}
}

// detailRows = jumlah baris tabel ekspor detail (tiap barang satu baris).
func detailRows(res returns.Result) int {
	return len(returns.DetailTable(returns.KindPurchase, res).Rows)
}
