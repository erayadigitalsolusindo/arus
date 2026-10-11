package accounting_test

import (
	"context"
	"testing"

	"github.com/google/uuid"

	"aciraba/internal/accounting"
)

func (s *salesEnv) exec(t *testing.T, sql string, args ...any) {
	t.Helper()
	if _, err := s.admin.Exec(context.Background(), sql, args...); err != nil {
		t.Fatalf("%v\n%s", err, sql)
	}
}

// Satu hari berisi pembelian, retur pembelian, retur penjualan, pelunasan piutang, pembayaran hutang, deposit, dan pencairan kredit
// pemasok; tiap sumber menghasilkan satu draf yang seimbang dengan susunan akun yang diharapkan.
func TestDailySourcesDrafts(t *testing.T) {
	s := newSalesEnv(t)
	ctx := context.Background()
	const day = "2026-03-20"
	supplier, p1, p2, pr1, sr1 := uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New()

	s.exec(t, `INSERT INTO suppliers (id, tenant_id, name) VALUES ($1, $2, 'Pemasok Uji')`, supplier, s.other)
	purchase := func(id uuid.UUID, typ, subtotal, tax, other, total string) {
		s.exec(t, `INSERT INTO purchases (id, tenant_id, outlet_id, doc_no, idempotency_key, request_hash, supplier_id, purchase_date, payment_type,
			subtotal, tax_amount, other_cost, total) VALUES ($1, $2, $3, $4, $5, 'h', $6, $7::date, $8, $9, $10, $11, $12)`,
			id, s.other, s.outlet, "PB-"+id.String()[:8], "idem-"+id.String(), supplier, day, typ, subtotal, tax, other, total)
	}
	purchase(p1, "credit", "100000", "11000", "5000", "116000")
	purchase(p2, "cash", "20000", "0", "0", "20000")
	s.exec(t, `INSERT INTO payables (id, tenant_id, outlet_id, supplier_id, purchase_id, amount) VALUES ($1, $2, $3, $4, $5, 116000)`, uuid.New(), s.other, s.outlet, supplier, p1)

	// Retur pembelian: Rp 10.000 + PPN 1.100; potong hutang 6.100, sisanya 5.000 jadi kredit pemasok. Harga pokok 2 × 4.500.
	s.exec(t, `INSERT INTO purchase_returns (id, tenant_id, outlet_id, purchase_id, supplier_id, doc_no, idempotency_key, request_hash, return_date,
		subtotal, tax_amount, total, payable_cut, refund, refund_method, refund_method_id, refund_method_name)
		VALUES ($1, $2, $3, $4, $5, $6, $7, 'h', $8::date, 10000, 1100, 11100, 6100, 5000, 'supplier_credit', $9, 'Kredit Pemasok')`,
		pr1, s.other, s.outlet, p1, supplier, "RB-"+pr1.String()[:8], "idem-"+pr1.String(), day, s.supCredit)
	s.exec(t, `INSERT INTO purchase_return_lines (tenant_id, return_id, position, purchase_position, item_id, item_sku, item_name, unit_name, qty, value, unit_cost)
		VALUES ($1, $2, 1, 1, $3, 'SKU-1', 'Barang Uji', 'Pcs', 2, 10000, 4500)`, s.other, pr1, s.item)

	// Penjualan kredit hari sebelumnya (tidak dijurnal di hari ini) sebagai dasar retur penjualan dan piutang.
	sale := s.sale(t, "2026-03-19 10:00+07", "completed", "8000", "0", "0", "0", "8000", "3000", "0", "0", "1", "3000", "5000",
		payRow{s.cash, "cash", "Tunai", "3000", "0", "store"})
	s.exec(t, `INSERT INTO sales_returns (id, tenant_id, outlet_id, sale_id, doc_no, idempotency_key, request_hash, return_date, subtotal, discount, tax_amount,
		total, receivable_cut, refund, refund_method, refund_method_id, refund_method_name)
		VALUES ($1, $2, $3, $4, $5, $6, 'h', $7::date, 8000, 0, 0, 8000, 3000, 5000, 'cash', $8, 'Tunai')`,
		sr1, s.other, s.outlet, sale, "RJ-"+sr1.String()[:8], "idem-"+sr1.String(), day, s.cash)
	s.exec(t, `INSERT INTO sales_return_lines (tenant_id, return_id, position, sale_position, item_id, item_sku, item_name, unit_name, factor, qty, base_qty, value, discount, tax_amount, unit_cost)
		VALUES ($1, $2, 1, 1, $3, 'SKU-1', 'Barang Uji', 'Pcs', 1, 1, 1, 8000, 0, 0, 3000)`, s.other, sr1, s.item)

	// Pelunasan piutang: QRIS 4.000 (MDR 40 ditanggung toko) + tunai 1.000.
	recv := uuid.New()
	s.exec(t, `INSERT INTO receivables (id, tenant_id, outlet_id, sale_id, member_id, amount) VALUES ($1, $2, $3, $4, $5, 5000)`, recv, s.other, s.outlet, sale, s.member)
	pay := func(method uuid.UUID, kind, name, amount, fee string) {
		id := uuid.New()
		s.exec(t, `INSERT INTO receivable_payments (id, tenant_id, receivable_id, outlet_id, doc_no, idempotency_key, request_hash, method, method_id, method_name,
			amount, fee_amount, created_at) VALUES ($1, $2, $3, $4, $5, $6, 'h', $7, $8, $9, $10, $11, $12::timestamptz)`,
			id, s.other, recv, s.outlet, "PP-"+id.String()[:8], "idem-"+id.String(), kind, method, name, amount, fee, day+" 09:00+07")
	}
	pay(s.qris, "ewallet", "QRIS", "4000", "40")
	pay(s.cash, "cash", "Tunai", "1000", "0")

	// Pembayaran hutang Rp 16.000 tunai.
	var payable uuid.UUID
	if err := s.admin.QueryRow(ctx, `SELECT id FROM payables WHERE tenant_id = $1 AND purchase_id = $2`, s.other, p1).Scan(&payable); err != nil {
		t.Fatal(err)
	}
	apid := uuid.New()
	s.exec(t, `INSERT INTO payable_payments (id, tenant_id, payable_id, outlet_id, doc_no, idempotency_key, request_hash, method, method_id, method_name, amount, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, 'h', 'cash', $7, 'Tunai', 16000, $8::timestamptz)`,
		apid, s.other, payable, s.outlet, "PH-"+apid.String()[:8], "idem-"+apid.String(), s.cash, day+" 10:00+07")

	// Top-up deposit member 50.000 tunai dan pencairan kredit pemasok 7.000 tunai.
	s.exec(t, `INSERT INTO member_deposit_movements (tenant_id, member_id, outlet_id, kind, amount, balance_after, doc_no, method, method_id, method_name, idempotency_key, created_at)
		VALUES ($1, $2, $3, 'TOPUP', 50000, 50000, 'DM-1', 'cash', $4, 'Tunai', $5, $6::timestamptz)`, s.other, s.member, s.outlet, s.cash, "idem-dm-"+uuid.NewString(), day+" 11:00+07")
	s.exec(t, `INSERT INTO supplier_credit_movements (tenant_id, supplier_id, outlet_id, kind, amount, balance_after, doc_no, method, method_id, method_name, idempotency_key, created_at)
		VALUES ($1, $2, $3, 'CASH_OUT', -7000, 0, 'KP-1', 'cash', $4, 'Tunai', $5, $6::timestamptz)`, s.other, supplier, s.outlet, s.cash, "idem-kp-"+uuid.NewString(), day+" 12:00+07")

	res, err := s.svc.GenerateDrafts(ctx, s.otherOwner, accounting.GenerateInput{From: day, To: day, OutletID: s.outlet})
	if err != nil {
		t.Fatal(err)
	}
	if res.Created != 7-1 { // 6 sumber berisi (penjualan hari itu kosong)
		t.Fatalf("hasil: %+v", res)
	}
	bySrc := map[string]accounting.Journal{}
	for _, it := range res.Items {
		j, err := s.svc.Get(ctx, s.otherOwner, *it.JournalID)
		if err != nil {
			t.Fatal(err)
		}
		if j.Status != "draft" || j.TotalDebit != j.TotalCredit {
			t.Fatalf("%s: %+v", it.Source, j)
		}
		bySrc[it.Source] = j
	}
	want := map[string]struct {
		typ string
		net map[string]string
	}{
		accounting.SourcePurchasesDaily:       {"PURCHASE", map[string]string{"1310": "125000", "1330": "11000", "2110": "-116000", "1110": "-20000"}},
		accounting.SourcePurchaseReturnsDaily: {"PURCHASE", map[string]string{"2110": "6100", "1320": "5000", "1310": "-9000", "1330": "-1100", "5200": "-1000"}},
		accounting.SourceSalesReturnsDaily:    {"SALES", map[string]string{"4100": "8000", "1210": "-3000", "1110": "-5000", "1310": "3000", "5100": "-3000"}},
		accounting.SourceReceivablesDaily:     {"AR", map[string]string{"1110": "1000", "1130": "3960", "6160": "40", "1210": "-5000"}},
		accounting.SourcePayablesDaily:        {"AP", map[string]string{"2110": "16000", "1110": "-16000"}},
		accounting.SourceWalletDaily:          {"WALLET", map[string]string{"1110": "57000", "2140": "-50000", "1320": "-7000"}},
	}
	for src, w := range want {
		j, ok := bySrc[src]
		if !ok {
			t.Errorf("sumber %s tidak menghasilkan draf", src)
			continue
		}
		if j.Type != w.typ || j.Date != day {
			t.Errorf("%s: jenis/tanggal %s %s", src, j.Type, j.Date)
		}
		wantNet(t, net(t, j), w.net)
	}

	// Semua draf bisa diposting (jenis jurnal dan akun sah), lalu hasil kedua tanpa perubahan.
	ids := make([]uuid.UUID, 0, len(res.Items))
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
	if res, err = s.svc.GenerateDrafts(ctx, s.otherOwner, accounting.GenerateInput{From: day, To: day, OutletID: s.outlet}); err != nil || res.Unchanged != 6 || res.Adjusted != 0 {
		t.Fatalf("setelah posting: %+v %v", res, err)
	}

	// Retur pembelian dibatalkan setelah diposting -> draf selisih khusus sumber itu.
	s.exec(t, `UPDATE purchase_returns SET status = 'void', void_reason = 'uji', voided_at = now(), voided_by = $2 WHERE id = $1`, pr1, s.otherOwner.UserID)
	res, err = s.svc.GenerateDrafts(ctx, s.otherOwner, accounting.GenerateInput{From: day, To: day, OutletID: s.outlet, Sources: []string{accounting.SourcePurchaseReturnsDaily}})
	if err != nil || res.Adjusted != 1 {
		t.Fatalf("selisih retur: %+v %v", res, err)
	}
	adj, err := s.svc.Get(ctx, s.otherOwner, *res.Items[0].JournalID)
	if err != nil {
		t.Fatal(err)
	}
	wantNet(t, net(t, adj), map[string]string{"2110": "-6100", "1320": "-5000", "1310": "9000", "1330": "1100", "5200": "1000"})

	// Sumber tak dikenal ditolak.
	var fe accounting.FieldErrors
	if _, err := s.svc.GenerateDrafts(ctx, s.otherOwner, accounting.GenerateInput{From: day, To: day, Sources: []string{"xyz"}}); err == nil {
		t.Fatal("sumber asing harus ditolak")
	} else if !asFieldErrors(err, &fe) {
		t.Fatalf("mau FieldErrors: %v", err)
	}
}

func asFieldErrors(err error, out *accounting.FieldErrors) bool {
	fe, ok := err.(accounting.FieldErrors)
	if ok {
		*out = fe
	}
	return ok
}
