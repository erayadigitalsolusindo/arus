package debt

import (
	"bytes"
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/shopspring/decimal"

	"aciraba/internal/authz"
)

// Test tanpa DB: semua tabel ekspor menghasilkan XLSX/PDF valid, jumlah nilai tiap baris = jumlah kolom.
func TestExportTables(t *testing.T) {
	h := Header{AsOf: "2026-10-15", From: "2026-10-01", To: "2026-10-15", TenantName: "Toko Uji", OutletName: "Pusat", OutletCode: "PST",
		PartyCode: "S-1", PartyName: "CV Maju"}
	b := Buckets{"100.00", "10.00", "20.00", "30.00", "40.00"}
	for _, k := range []Kind{KindPayable, KindReceivable} {
		tables := map[string]table{
			"outstanding": OutstandingTable(k, OutstandingResult{Header: h, HasSummary: true,
				Summary: Summary{DocCount: 1, Amount: "200.00", Paid: "50.00", Returned: "0.00", Balance: "150.00", Buckets: b},
				Data: []OutstandingRow{{DocNo: "PB-1", RefNo: "F1", PartyName: "CV Maju", DocDate: "2026-09-01", DueDate: "2026-09-30",
					Amount: "200.00", Paid: "50.00", Returned: "0.00", Balance: "150.00", DaysOverdue: 15, Bucket: 1}}}),
			"balances": BalanceTable(k, BalanceResult{Header: h, HasSummary: true,
				Summary: Summary{DocCount: 1, PartyCount: 1, Amount: "200.00", Paid: "50.00", Balance: "150.00", Buckets: b},
				Data:    []BalanceRow{{Code: "S-1", Name: "CV Maju", DocCount: 1, Amount: "200.00", Paid: "50.00", Balance: "150.00", Buckets: b, OldestDue: "2026-09-30"}}}),
			"payments": PaymentTable(k, PaymentResult{Header: h, HasSummary: true,
				Summary: PaymentSummary{Count: 1, Amount: "50.00", Fee: "0.00", ByMethod: []MethodTotal{{Name: "Tunai", Count: 1, Amount: "50.00"}}},
				Data:    []PaymentRow{{DocNo: "BH-1", Date: "2026-10-02", Time: "10:00", TargetDocNo: "PB-1", PartyName: "CV Maju", Method: "Tunai", Amount: "50.00", UserName: "Budi"}}}),
			"ledger": LedgerTable(k, LedgerResult{Header: h, HasSummary: true,
				Summary: LedgerSummary{Opening: "0.00", Increase: "200.00", Decrease: "50.00", Closing: "150.00"},
				Data: []LedgerRow{{Date: "2026-10-01", Type: EventInvoice, DocNo: "PB-1", Increase: "200.00", Decrease: "0.00", Balance: "200.00"},
					{Date: "2026-10-02", Type: EventPayment, DocNo: "BH-1", RefNo: "PB-1", Increase: "0.00", Decrease: "50.00", Balance: "150.00"}}}),
		}
		if k == KindPayable {
			tables["credit"] = CreditTable(CreditResult{Header: h, HasSummary: true,
				Summary: CreditSummary{SupplierCount: 1, Opening: "0.00", Added: "5.00", Closing: "5.00"},
				Data:    []CreditRow{{Code: "S-1", Name: "CV Maju", Opening: "0.00", Added: "5.00", Closing: "5.00"}}})
		}
		for name, tb := range tables {
			for _, r := range append(append([][]string{}, tb.Rows...), tb.Total) {
				if len(r) != len(tb.Cols) {
					t.Fatalf("kind %d %s: baris %d nilai, kolom %d", k, name, len(r), len(tb.Cols))
				}
			}
			w := 0.0
			for _, c := range tb.Cols {
				w += c.Width
			}
			if w > 339 {
				t.Fatalf("kind %d %s: lebar PDF %.0f > 339", k, name, w)
			}
			if x, err := tb.ToXLSX(); err != nil || !bytes.HasPrefix(x, []byte("PK")) {
				t.Fatalf("kind %d %s xlsx: %v", k, name, err)
			}
			if p, err := tb.ToPDF(); err != nil || !bytes.HasPrefix(p, []byte("%PDF")) {
				t.Fatalf("kind %d %s pdf: %v", k, name, err)
			}
		}
	}
}

func TestCursorRoundTrip(t *testing.T) {
	if _, ok := decodeCursor("???", 2); ok {
		t.Fatal("cursor rusak harus ditolak")
	}
	c := encodeCursor("id", "nama | dengan pipa")
	parts, ok := decodeCursor(c, 2)
	if !ok || parts[1] != "nama | dengan pipa" {
		t.Fatalf("kursor nama ber-pipa: %v %v", parts, ok)
	}
}

func TestBucketOf(t *testing.T) {
	asOf := time.Date(2026, 10, 15, 0, 0, 0, 0, time.UTC)
	cases := []struct {
		due        string
		days, want int
	}{{"", 0, 0}, {"2026-10-15", 0, 0}, {"2026-10-16", 0, 0}, {"2026-10-14", 1, 1}, {"2026-09-15", 30, 1},
		{"2026-09-14", 31, 2}, {"2026-08-16", 60, 2}, {"2026-08-15", 61, 3}, {"2026-07-17", 90, 3}, {"2026-07-16", 91, 4}}
	for _, c := range cases {
		var due pgtype.Date
		if c.due != "" {
			d, _ := time.Parse("2006-01-02", c.due)
			due = pgtype.Date{Time: d, Valid: true}
		}
		days, b := bucketOf(due, asOf)
		if days != c.days || b != c.want {
			t.Errorf("due %q: days=%d bucket=%d, want %d/%d", c.due, days, b, c.days, c.want)
		}
	}
}

// TestQueriesAgainstDB menjalankan query asli di transaksi yang di-rollback: saldo per tanggal as-of,
// umur, pembayaran, buku bantu (saldo berjalan + keyset), saldo di pemasok.
func TestQueriesAgainstDB(t *testing.T) {
	url := os.Getenv("TEST_ADMIN_DATABASE_URL")
	if url == "" {
		t.Skip("TEST_ADMIN_DATABASE_URL tidak di-set")
	}
	ctx := context.Background()
	conn, err := pgx.Connect(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close(ctx)
	tx, err := conn.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)

	var tenant, outlet, cash uuid.UUID
	if err := tx.QueryRow(ctx, `SELECT o.tenant_id, o.id FROM outlets o ORDER BY o.created_at LIMIT 1`).Scan(&tenant, &outlet); err != nil {
		t.Skip("tidak ada tenant dev untuk fixture: ", err)
	}
	if err := tx.QueryRow(ctx, `SELECT id FROM payment_methods WHERE tenant_id = $1 AND kind = 'cash' LIMIT 1`, tenant).Scan(&cash); err != nil {
		t.Skip("tenant dev tanpa metode tunai: ", err)
	}
	exec := func(q string, args ...any) {
		t.Helper()
		if _, err := tx.Exec(ctx, q, args...); err != nil {
			t.Fatalf("fixture: %v\n%s", err, q)
		}
	}
	uid := func() string { return uuid.NewString()[:8] }
	supplier, member := uuid.New(), uuid.New()
	exec(`INSERT INTO suppliers (id, tenant_id, name) VALUES ($1, $2, 'Zz Pemasok Uji')`, supplier, tenant)
	exec(`INSERT INTO members (id, tenant_id, code, name) VALUES ($1, $2, 'ZZ-HUT', 'Zz Member Uji')`, member, tenant)

	// --- hutang ---
	purchase := func(date string, total int, due string) (uuid.UUID, uuid.UUID) {
		pid, pb := uuid.New(), uuid.New()
		exec(`INSERT INTO purchases (id, tenant_id, outlet_id, doc_no, idempotency_key, request_hash, supplier_id, supplier_invoice_no, purchase_date, payment_type,
			due_date, subtotal, total) VALUES ($1,$2,$3,$4,$5,'h',$6,$7,$8::date,'credit',$9::date,$10,$10)`,
			pid, tenant, outlet, "ZZ-PB-"+pid.String()[:8], "zz-idem-"+pid.String(), supplier, "ZZ-F-"+uid(), date, nullable(due), total)
		exec(`INSERT INTO payables (id, tenant_id, outlet_id, supplier_id, purchase_id, amount, due_date) VALUES ($1,$2,$3,$4,$5,$6,$7::date)`,
			pb, tenant, outlet, supplier, pid, total, nullable(due))
		return pid, pb
	}
	pay := func(pb uuid.UUID, amount int, at string) {
		exec(`INSERT INTO payable_payments (tenant_id, payable_id, outlet_id, doc_no, idempotency_key, request_hash, method, method_id, method_name, amount, created_at)
			VALUES ($1,$2,$3,$4,$5,'h','cash',$6,'Tunai',$7,$8::timestamptz)`,
			tenant, pb, outlet, "ZZ-BH-"+uid(), "zz-idem-"+uuid.NewString(), cash, amount, at)
	}
	p1, pb1 := purchase("2099-03-10", 1000000, "2099-03-20")
	purchase("2099-04-01", 500000, "")
	_, pb3 := purchase("2099-03-12", 200000, "2099-03-30") // lunas
	pay(pb1, 300000, "2099-03-15 05:00+00")
	pay(pb3, 200000, "2099-03-13 05:00+00")
	exec(`INSERT INTO purchase_returns (tenant_id, outlet_id, purchase_id, supplier_id, doc_no, idempotency_key, request_hash, return_date,
		subtotal, tax_amount, total, payable_cut, refund) VALUES ($1,$2,$3,$4,$5,$6,'h','2099-03-18',100000,0,100000,100000,0)`,
		tenant, outlet, p1, supplier, "ZZ-RB-"+uid(), "zz-idem-"+uuid.NewString())

	a := authz.Actor{TenantID: tenant, OutletID: outlet, Outlets: map[uuid.UUID]bool{outlet: true}}
	dec := func(s string) decimal.Decimal { d, _ := decimal.NewFromString(s); return d }
	out := func(asOf string) OutstandingResult {
		res, err := outstanding(ctx, tx, a, KindPayable, Params{AsOf: asOf, PartyID: supplier.String()})
		if err != nil {
			t.Fatalf("outstanding %s: %v", asOf, err)
		}
		return res
	}
	// 31 Mar: P1 sisa 600.000 (bayar 300.000 + retur 100.000) terlambat 11 hari; P2 belum terbit; P3 lunas.
	r := out("2099-03-31")
	if len(r.Data) != 1 || r.Data[0].Balance != "600000.00" || r.Data[0].DaysOverdue != 11 || r.Data[0].Bucket != 1 {
		t.Fatalf("beredar 31 Mar: %+v", r.Data)
	}
	if r.Summary.Balance != "600000.00" || r.Summary.Buckets.D1To30 != "600000.00" || r.Summary.Overdue != "600000.00" {
		t.Fatalf("ringkasan 31 Mar: %+v", r.Summary)
	}
	// 16 Mar: retur (18 Mar) belum berlaku → 700.000; belum jatuh tempo (20 Mar).
	r = out("2099-03-16")
	if len(r.Data) != 1 || r.Data[0].Balance != "700000.00" || r.Data[0].Bucket != 0 {
		t.Fatalf("beredar 16 Mar: %+v", r.Data)
	}
	// 12 Mar: P3 belum dibayar (13 Mar) → 200.000 ikut.
	r = out("2099-03-12")
	if len(r.Data) != 2 {
		t.Fatalf("beredar 12 Mar: %+v", r.Data)
	}
	// 30 Apr: P1 600.000 (>10 hari) + P2 500.000 (tanpa jatuh tempo).
	r = out("2099-04-30")
	if len(r.Data) != 2 || r.Summary.Balance != "1100000.00" || r.Summary.Buckets.Current != "500000.00" {
		t.Fatalf("beredar 30 Apr: %+v %+v", r.Data, r.Summary)
	}
	// keyset: limit 1 → halaman 2 tanpa tumpang tindih.
	p1res, err := outstanding(ctx, tx, a, KindPayable, Params{AsOf: "2099-04-30", PartyID: supplier.String(), Limit: 1})
	if err != nil || len(p1res.Data) != 1 || p1res.NextCursor == "" {
		t.Fatalf("halaman 1: %v %+v", err, p1res)
	}
	p2res, err := outstanding(ctx, tx, a, KindPayable, Params{AsOf: "2099-04-30", PartyID: supplier.String(), Limit: 1, Cursor: p1res.NextCursor})
	if err != nil || len(p2res.Data) != 1 || p2res.Data[0].ID == p1res.Data[0].ID || p2res.NextCursor != "" {
		t.Fatalf("halaman 2: %v %+v", err, p2res)
	}

	// saldo per pemasok
	bal, err := balances(ctx, tx, a, KindPayable, Params{AsOf: "2099-04-30", Search: "Zz Pemasok Uji"})
	if err != nil || len(bal.Data) != 1 || bal.Data[0].Balance != "1100000.00" || bal.Data[0].DocCount != 2 || bal.Data[0].OldestDue != "2099-03-20" {
		t.Fatalf("saldo pemasok: %v %+v", err, bal.Data)
	}

	// pembayaran
	pays, err := payments(ctx, tx, a, KindPayable, Params{From: "2099-03-01", To: "2099-03-31", PartyID: supplier.String()})
	if err != nil || len(pays.Data) != 2 || pays.Summary.Amount != "500000.00" || pays.Summary.Count != 2 ||
		len(pays.Summary.ByMethod) != 1 || pays.Summary.ByMethod[0].Name != "Tunai" {
		t.Fatalf("pembayaran: %v %+v %+v", err, pays.Data, pays.Summary)
	}
	if pays.Data[0].Date != "2099-03-15" || pays.Data[0].PartyName != "Zz Pemasok Uji" || !strings.HasPrefix(pays.Data[0].TargetDocNo, "ZZ-PB-") {
		t.Fatalf("baris pembayaran: %+v", pays.Data[0])
	}
	if _, err := payments(ctx, tx, a, KindPayable, Params{From: "2099-03-01", To: "2099-03-31", Search: "ZZ-BH-"}); err != nil {
		t.Fatalf("pembayaran cari: %v", err)
	}

	// buku bantu 1 Mar–30 Apr: saldo akhir = beredar 30 Apr
	led, err := ledger(ctx, tx, a, KindPayable, Params{From: "2099-03-01", To: "2099-04-30", PartyID: supplier.String()})
	if err != nil {
		t.Fatal(err)
	}
	if led.Summary.Opening != "0.00" || led.Summary.Closing != "1100000.00" || led.Summary.Increase != "1700000.00" || led.Summary.Decrease != "600000.00" {
		t.Fatalf("buku bantu ringkasan: %+v", led.Summary)
	}
	if len(led.Data) != 6 || !dec(led.Data[len(led.Data)-1].Balance).Equal(dec("1100000")) {
		t.Fatalf("buku bantu baris: %d %+v", len(led.Data), led.Data)
	}
	// urut kronologis: faktur 10 Mar, faktur 12 Mar, bayar 13 Mar, bayar 15 Mar, retur 18 Mar, faktur 1 Apr
	wantTypes := []string{EventInvoice, EventInvoice, EventPayment, EventPayment, EventReturn, EventInvoice}
	for i, w := range wantTypes {
		if led.Data[i].Type != w {
			t.Fatalf("urutan buku bantu: %d %s want %s (%+v)", i, led.Data[i].Type, w, led.Data)
		}
	}
	// halaman per 2 baris: saldo berjalan lintas halaman sama dengan satu halaman
	var paged []LedgerRow
	cur := ""
	for n := 0; n < 10; n++ {
		pg, err := ledger(ctx, tx, a, KindPayable, Params{From: "2099-03-01", To: "2099-04-30", PartyID: supplier.String(), Limit: 2, Cursor: cur})
		if err != nil {
			t.Fatal(err)
		}
		paged = append(paged, pg.Data...)
		if cur = pg.NextCursor; cur == "" {
			break
		}
	}
	if len(paged) != len(led.Data) {
		t.Fatalf("buku bantu berhalaman %d baris, satu halaman %d", len(paged), len(led.Data))
	}
	for i := range paged {
		if paged[i] != led.Data[i] {
			t.Fatalf("baris %d beda: %+v vs %+v", i, paged[i], led.Data[i])
		}
	}
	// saldo awal periode berikutnya = saldo akhir sebelumnya
	led2, err := ledger(ctx, tx, a, KindPayable, Params{From: "2099-04-01", To: "2099-04-30", PartyID: supplier.String()})
	if err != nil || led2.Summary.Opening != "600000.00" || led2.Summary.Closing != "1100000.00" {
		t.Fatalf("buku bantu April: %v %+v", err, led2.Summary)
	}
	if _, err := ledger(ctx, tx, a, KindPayable, Params{From: "2099-04-01", To: "2099-04-30"}); err == nil {
		t.Fatal("buku bantu tanpa pihak harus ditolak")
	}

	// --- saldo di pemasok ---
	exec(`INSERT INTO supplier_credit_movements (tenant_id, supplier_id, outlet_id, kind, amount, balance_after, doc_no, created_at)
		VALUES ($1,$2,$3,'PURCHASE_RETURN',50000,50000,'ZZ-K1','2099-03-18 05:00+00')`, tenant, supplier, outlet)
	exec(`INSERT INTO supplier_credit_movements (tenant_id, supplier_id, outlet_id, kind, amount, balance_after, doc_no, created_at)
		VALUES ($1,$2,$3,'PAYABLE_PAYMENT',-20000,30000,'ZZ-K2','2099-04-10 05:00+00')`, tenant, supplier, outlet)
	cr, err := credit(ctx, tx, a, Params{From: "2099-03-01", To: "2099-03-31", Search: "Zz Pemasok Uji"})
	if err != nil || len(cr.Data) != 1 || cr.Data[0].Opening != "0.00" || cr.Data[0].Added != "50000.00" || cr.Data[0].Closing != "50000.00" {
		t.Fatalf("saldo di pemasok Maret: %v %+v", err, cr.Data)
	}
	cr, err = credit(ctx, tx, a, Params{From: "2099-04-01", To: "2099-04-30", Search: "Zz Pemasok Uji"})
	if err != nil || len(cr.Data) != 1 || cr.Data[0].Opening != "50000.00" || cr.Data[0].Used != "20000.00" || cr.Data[0].Closing != "30000.00" {
		t.Fatalf("saldo di pemasok April: %v %+v", err, cr.Data)
	}

	// --- piutang: nota kredit 100.000, bayar tunai 40.000 + sisa piutang 60.000 jatuh tempo 25 Mar; cicil 10.000 ---
	sale := uuid.New()
	exec(`INSERT INTO sales (id, tenant_id, outlet_id, doc_no, idempotency_key, request_hash, status, subtotal, discount,
		tax_store_pct, tax_gov_pct, tax_store, tax_gov, other_cost, total, paid, change, receivable, created_at, member_id)
		VALUES ($1,$2,$3,$4,$5,'x','completed',100000,0,0,0,0,0,0,100000,40000,0,60000,'2099-03-10 05:00+00',$6)`,
		sale, tenant, outlet, "ZZ-INV-"+uid(), "zz-idem-"+uuid.NewString(), member)
	rc := uuid.New()
	exec(`INSERT INTO receivables (id, tenant_id, outlet_id, sale_id, member_id, amount, due_date) VALUES ($1,$2,$3,$4,$5,60000,'2099-03-25')`,
		rc, tenant, outlet, sale, member)
	exec(`INSERT INTO receivable_payments (tenant_id, receivable_id, outlet_id, doc_no, idempotency_key, request_hash, method, method_id, method_name, amount, created_at)
		VALUES ($1,$2,$3,$4,$5,'h','cash',$6,'Tunai',10000,'2099-03-20 05:00+00')`, tenant, rc, outlet, "ZZ-PP-"+uid(), "zz-idem-"+uuid.NewString(), cash)
	ro, err := outstanding(ctx, tx, a, KindReceivable, Params{AsOf: "2099-03-31", PartyID: member.String()})
	if err != nil || len(ro.Data) != 1 || ro.Data[0].Balance != "50000.00" || ro.Data[0].DaysOverdue != 6 || !strings.HasPrefix(ro.Data[0].DocNo, "ZZ-INV-") {
		t.Fatalf("piutang beredar: %v %+v", err, ro.Data)
	}
	rl, err := ledger(ctx, tx, a, KindReceivable, Params{From: "2099-03-01", To: "2099-03-31", PartyID: member.String()})
	if err != nil || rl.Summary.Closing != "50000.00" || len(rl.Data) != 2 || rl.Data[1].Balance != "50000.00" {
		t.Fatalf("buku bantu piutang: %v %+v %+v", err, rl.Summary, rl.Data)
	}
	rb, err := balances(ctx, tx, a, KindReceivable, Params{AsOf: "2099-03-31", Search: "Zz Member Uji"})
	if err != nil || len(rb.Data) != 1 || rb.Data[0].Balance != "50000.00" {
		t.Fatalf("saldo member: %v %+v", err, rb.Data)
	}
	rp, err := payments(ctx, tx, a, KindReceivable, Params{From: "2099-03-01", To: "2099-03-31", PartyID: member.String()})
	if err != nil || len(rp.Data) != 1 || rp.Summary.Amount != "10000.00" {
		t.Fatalf("pembayaran piutang: %v %+v", err, rp.Data)
	}
}

func nullable(s string) any {
	if s == "" {
		return nil
	}
	return s
}
