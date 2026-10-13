package accounting_test

import (
	"context"
	"testing"

	"github.com/google/uuid"

	"aciraba/internal/accounting"
)

// Opname selesai, selisih kas tutup shift, dan kas kecil (yang dibatalkan tidak ikut) menjadi draf STOCK / CASH.
func TestStage4Drafts(t *testing.T) {
	s := newSalesEnv(t)
	ctx := context.Background()
	const day = "2026-03-22"
	user := s.otherOwner.UserID
	t.Cleanup(func() { // sebelum cleanup salesEnv (LIFO): tabel milik tahap 4
		for _, tbl := range []string{"cash_movements", "cash_shift_counts", "cash_shifts", "stock_count_lines", "stock_counts"} {
			_, _ = s.admin.Exec(ctx, `DELETE FROM `+tbl+` WHERE tenant_id = $1`, s.other)
		}
	})

	// Opname: barang 1 kurang 2 × Rp 5.000, barang 2 lebih 2 × Rp 1.000 -> kurang bersih Rp 8.000.
	item2, count := uuid.New(), uuid.New()
	s.exec(t, `INSERT INTO items (id, tenant_id, sku, name, unit_id) VALUES ($1, $2, 'SKU-2', 'Barang Dua', $3)`, item2, s.other, s.unit)
	s.exec(t, `INSERT INTO stock_counts (id, tenant_id, outlet_id, doc_no, bucket, status, completed_by, completed_at)
		VALUES ($1, $2, $3, 'SO-UJI-1', 'display', 'completed', $4, $5::timestamptz)`, count, s.other, s.outlet, user, day+" 20:00+07")
	s.exec(t, `INSERT INTO stock_count_lines (tenant_id, count_id, item_id, snapshot_qty, counted_qty, diff, unit_cost) VALUES ($1, $2, $3, 10, 8, -2, 5000)`, s.other, count, s.item)
	s.exec(t, `INSERT INTO stock_count_lines (tenant_id, count_id, item_id, snapshot_qty, counted_qty, diff, unit_cost) VALUES ($1, $2, $3, 5, 7, 2, 1000)`, s.other, count, item2)

	// Shift ditutup: tunai kurang Rp 2.000, QRIS lebih Rp 500.
	shift := uuid.New()
	s.exec(t, `INSERT INTO cash_shifts (id, tenant_id, outlet_id, user_id, doc_no, status, opening_cash, opened_at, closed_at, closed_by, expected_total, counted_total,
		diff_total, diff_abs, sale_count, void_count, sales_total, receivable_total, note, approved_by, approved_name, close_key)
		VALUES ($1, $2, $3, $4, 'SH-UJI-1', 'closed', 0, $5::timestamptz, $6::timestamptz, $4, 150000, 148500, -1500, 2500, 3, 0, 150000, 0, 'selisih uji', $4, 'Owner', $7)`,
		shift, s.other, s.outlet, user, day+" 08:00+07", day+" 21:00+07", "close-"+shift.String())
	count1 := func(pos int, method uuid.UUID, name, kind, expected, counted, diff string) {
		s.exec(t, `INSERT INTO cash_shift_counts (tenant_id, shift_id, position, method_id, method_name, kind, sales, flows, opening, expected, counted, diff)
			VALUES ($1, $2, $3, $4, $5, $6, $7, 0, 0, $7, $8, $9)`, s.other, shift, pos, method, name, kind, expected, counted, diff)
	}
	count1(1, s.cash, "Tunai", "cash", "100000", "98000", "-2000")
	count1(2, s.qris, "QRIS", "ewallet", "50000", "50500", "500")

	// Kas kecil: keluar galon 15.000 + parkir 5.000, masuk tambah receh 30.000, satu catatan dibatalkan (diabaikan).
	move := func(dir, amount, cat string, voided bool) {
		id := uuid.New()
		var v any
		if voided {
			v = user
		}
		s.exec(t, `INSERT INTO cash_movements (id, tenant_id, outlet_id, shift_id, method_id, direction, amount, category, created_by, idem_key, created_at, voided_at, voided_by)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11::timestamptz, CASE WHEN $12::uuid IS NULL THEN NULL ELSE now() END, $12)`,
			id, s.other, s.outlet, shift, s.cash, dir, amount, cat, user, "pc-"+id.String(), day+" 12:00+07", v)
	}
	move("out", "15000", "Galon", false)
	move("out", "3000", "Galon", false)
	move("out", "5000", "Parkir", false)
	move("in", "30000", "Tambah receh", false)
	move("out", "99999", "Salah catat", true)
	nextDayID := uuid.New()
	s.exec(t, `INSERT INTO cash_movements (id, tenant_id, outlet_id, shift_id, method_id, direction, amount, category, created_by, idem_key, created_at)
		VALUES ($1, $2, $3, $4, $5, 'out', 99999, 'Besok', $6, $7, $8::timestamptz)`,
		nextDayID, s.other, s.outlet, shift, s.cash, user, "pc-"+nextDayID.String(), "2026-03-23 00:01+07")

	pettyReport, err := s.svc.PettyCashReport(ctx, s.otherOwner, s.outlet, day, day)
	if err != nil {
		t.Fatal(err)
	}
	if len(pettyReport.Rows) != 3 || pettyReport.Total.Entries != 4 || pettyReport.Total.In != "30000.00" || pettyReport.Total.Out != "23000.00" || pettyReport.Total.Net != "7000.00" {
		t.Fatalf("laporan kas kecil: %+v", pettyReport)
	}
	byCategory := map[string]accounting.PettyCashCategoryRow{}
	for _, row := range pettyReport.Rows {
		byCategory[row.Category] = row
	}
	if row := byCategory["Galon"]; row.Entries != 2 || row.Out != "18000.00" || row.Net != "-18000.00" {
		t.Errorf("agregat kategori Galon: %+v", row)
	}

	res, err := s.svc.GenerateDrafts(ctx, s.otherOwner, accounting.GenerateInput{From: day, To: day, OutletID: s.outlet,
		Sources: []string{accounting.SourceStockCountsDaily, accounting.SourceShiftVarianceDaily, accounting.SourcePettyCashDaily}})
	if err != nil {
		t.Fatal(err)
	}
	if res.Created != 3 {
		t.Fatalf("hasil: %+v", res)
	}
	want := map[string]struct {
		typ string
		net map[string]string
	}{
		accounting.SourceStockCountsDaily:   {"STOCK", map[string]string{"5200": "8000", "1310": "-8000"}},
		accounting.SourceShiftVarianceDaily: {"CASH", map[string]string{"6900": "2000", "1110": "-2000", "1130": "500", "4900": "-500"}},
		accounting.SourcePettyCashDaily:     {"CASH", map[string]string{"6900": "23000", "1110": "7000", "1120": "-30000"}},
	}
	for _, it := range res.Items {
		j, err := s.svc.Get(ctx, s.otherOwner, *it.JournalID)
		if err != nil {
			t.Fatal(err)
		}
		w := want[it.Source]
		if j.Type != w.typ || j.Status != "draft" || j.TotalDebit != j.TotalCredit {
			t.Errorf("%s: %+v", it.Source, j)
		}
		wantNet(t, net(t, j), w.net)
	}

	// Siap diposting; ulang tanpa perubahan.
	ids := make([]uuid.UUID, 0, 3)
	for _, it := range res.Items {
		ids = append(ids, *it.JournalID)
	}
	posted, err := s.svc.PostBatch(ctx, s.otherOwner, ids)
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range posted {
		if !p.OK {
			t.Errorf("posting %s gagal: %s", p.ID, p.Code)
		}
	}
	if res, err = s.svc.GenerateDrafts(ctx, s.otherOwner, accounting.GenerateInput{From: day, To: day, OutletID: s.outlet}); err != nil || res.Adjusted != 0 || res.Created != 0 {
		t.Fatalf("ulang: %+v %v", res, err)
	}
}
