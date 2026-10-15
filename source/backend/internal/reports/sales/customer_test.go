package sales

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/shopspring/decimal"
)

func sampleCustomerResult() CustomerResult {
	return CustomerResult{
		From: "2026-10-01", To: "2026-10-15", TenantName: "Toko", OutletName: "Pusat", OutletCode: "MAIN", SortLabel: "Omset",
		Data: []CustomerRow{{MemberID: "m1", Code: "M-1", Name: "Ani", Phone: "0811", InvoiceCount: 2, Subtotal: "150000.00",
			Discount: "0.00", Total: "150000.00", Average: "75000.00", LastDate: "2026-10-10"}},
		Summary: CustomerSummary{CustomerCount: 1, InvoiceCount: 2, Subtotal: "150000.00", Discount: "0.00", Total: "150000.00"},
	}
}

func sampleCustInvResult(withItems bool) CustInvResult {
	sale := CustInvSale{ID: "s1", DocNo: "INV-1", Date: "2026-10-10", Time: "10:00", OutletCode: "MAIN", CashierName: "Budi",
		Subtotal: "100000.00", Discount: "0.00", Total: "100000.00", Paid: "100000.00", Receivable: "0.00"}
	if withItems {
		sale.Lines = []DetailLine{{Position: 1, SKU: "A1", Name: "Barang A", UnitName: "pcs", Qty: "2.000", UnitPrice: "50000.00", LineTotal: "100000.00"}}
	}
	c := CustInvCustomer{CustomerRow: sampleCustomerResult().Data[0], Sales: []CustInvSale{sale}}
	return CustInvResult{From: "2026-10-01", To: "2026-10-15", TenantName: "Toko", OutletName: "Pusat", OutletCode: "MAIN",
		WithItems: withItems, Customers: []CustInvCustomer{c}, Summary: sampleCustomerResult().Summary}
}

func TestCustomerExports(t *testing.T) {
	r := sampleCustomerResult()
	if x, err := CustomerToXLSX(r); err != nil || string(x[:2]) != "PK" {
		t.Fatalf("xlsx omset: %v", err)
	}
	if p, err := CustomerToPDF(r); err != nil || !strings.HasPrefix(string(p), "%PDF") {
		t.Fatalf("pdf omset: %v", err)
	}
	for _, withItems := range []bool{false, true} {
		ci := sampleCustInvResult(withItems)
		if x, err := CustInvToXLSX(ci); err != nil || string(x[:2]) != "PK" {
			t.Fatalf("xlsx pelanggan (items=%v): %v", withItems, err)
		}
		if p, err := CustInvToPDF(ci); err != nil || !strings.HasPrefix(string(p), "%PDF") {
			t.Fatalf("pdf pelanggan (items=%v): %v", withItems, err)
		}
	}
	d := DetailResult{From: "2026-10-01", To: "2026-10-15", TenantName: "Toko", OutletName: "Pusat",
		Data: []DetailDay{{Date: "2026-10-10", Sales: []DetailSale{{DocNo: "INV-1", Time: "10:00", Status: "completed", Total: "100000.00",
			Lines: []DetailLine{{SKU: "A1", Name: "Barang A", UnitName: "pcs", Qty: "2.000", UnitPrice: "50000.00", LineTotal: "100000.00"}}}}}}}
	if p, err := InvoiceDetailToPDF(d); err != nil || !strings.HasPrefix(string(p), "%PDF") {
		t.Fatalf("pdf faktur detail: %v", err)
	}
}

// TestCustomerQueriesAgainstDB menjalankan query asli di transaksi yang di-rollback: non member tidak ikut,
// void tidak ikut, agregat per pelanggan dan keyset faktur benar.
func TestCustomerQueriesAgainstDB(t *testing.T) {
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

	var tenant, outlet uuid.UUID
	if err := tx.QueryRow(ctx, `SELECT tenant_id, id FROM outlets ORDER BY created_at LIMIT 1`).Scan(&tenant, &outlet); err != nil {
		t.Skip("tidak ada tenant dev untuk fixture: ", err)
	}
	mk := func(code, name string) uuid.UUID {
		id := uuid.New()
		if _, err := tx.Exec(ctx, `INSERT INTO members (id, tenant_id, code, name) VALUES ($1,$2,$3,$4)`, id, tenant, code, name); err != nil {
			t.Fatalf("fixture member: %v", err)
		}
		return id
	}
	a, b := mk("ZZ-A", "Zz Ani"), mk("ZZ-B", "Zz Budi")
	day := time.Date(2099, 3, 10, 3, 0, 0, 0, time.UTC)
	n := 0
	sale := func(member *uuid.UUID, status string, total int, at time.Time) {
		n++
		_, err := tx.Exec(ctx, `INSERT INTO sales (tenant_id, outlet_id, doc_no, idempotency_key, request_hash, status, subtotal, discount,
			tax_store_pct, tax_gov_pct, tax_store, tax_gov, other_cost, total, paid, change, created_at, member_id, void_reason)
			VALUES ($1,$2,$3,$4,'x',$5::text,$6,0,0,0,0,0,0,$6,$6,0,$7,$8,CASE WHEN $5::text = 'void' THEN 'zz-test' END)`,
			tenant, outlet, "ZZ-C-"+uuid.NewString()[:8], "zz-idem-"+uuid.NewString(), status, total, at, member)
		if err != nil {
			t.Fatalf("fixture sale %d: %v", n, err)
		}
	}
	sale(&a, "completed", 100000, day)
	sale(&a, "completed", 50000, day.Add(time.Hour))
	sale(&a, "void", 900000, day.Add(2*time.Hour)) // void: tidak boleh ikut
	sale(&b, "completed", 70000, day.Add(3*time.Hour))
	sale(nil, "completed", 800000, day.Add(4*time.Hour)) // non member: tidak boleh ikut

	outlets := []uuid.UUID{outlet}
	fd, td := time.Date(2099, 3, 1, 0, 0, 0, 0, time.UTC), time.Date(2099, 3, 31, 0, 0, 0, 0, time.UTC)

	rows, err := tx.Query(ctx, customerAggSQL(customerSortSQL["total"]),
		tenant, outlets, fd, td, uuid.Nil, false, []string{}, 100)
	if err != nil {
		t.Fatal(err)
	}
	type agg struct {
		cnt        int
		total      decimal.Decimal
		nAll, tCnt int
		tTot       decimal.Decimal
	}
	got := map[string]agg{}
	var order []string
	for rows.Next() {
		var id uuid.UUID
		var code, name, phone string
		var cnt, nAll, tCnt int
		var sub, disc, tot, tSub, tDisc, tTot decimal.Decimal
		var last time.Time
		if err := rows.Scan(&id, &code, &name, &phone, &cnt, &sub, &disc, &tot, &last, &nAll, &tCnt, &tSub, &tDisc, &tTot); err != nil {
			t.Fatal(err)
		}
		got[code] = agg{cnt, tot, nAll, tCnt, tTot}
		order = append(order, code)
	}
	rows.Close()
	if rows.Err() != nil {
		t.Fatal(rows.Err())
	}
	if len(got) != 2 || got["ZZ-A"].cnt != 2 || !got["ZZ-A"].total.Equal(decimal.NewFromInt(150000)) ||
		got["ZZ-B"].cnt != 1 || got["ZZ-B"].nAll != 2 || got["ZZ-B"].tCnt != 3 || !got["ZZ-B"].tTot.Equal(decimal.NewFromInt(220000)) {
		t.Fatalf("agregat salah: %+v", got)
	}
	if order[0] != "ZZ-A" { // omset terbesar dulu
		t.Fatalf("urutan salah: %v", order)
	}

	// Pencarian: hanya "budi".
	rows, err = tx.Query(ctx, customerAggSQL(customerSortSQL["name"]),
		tenant, outlets, fd, td, uuid.Nil, true, []string{"%budi%"}, 100)
	if err != nil {
		t.Fatal(err)
	}
	cnt := 0
	for rows.Next() {
		cnt++
	}
	rows.Close()
	if rows.Err() != nil || cnt != 1 {
		t.Fatalf("pencarian: %d baris, err=%v", cnt, rows.Err())
	}

	// Faktur pelanggan A: 2 baris completed, keyset halaman 1 (limit 1) lalu halaman 2.
	page := func(hasCursor bool, at time.Time, id uuid.UUID) (ids []uuid.UUID, ats []time.Time) {
		rs, err := tx.Query(ctx, custInvSalesSQL, tenant, outlets, fd, td, uuid.Nil, []uuid.UUID{a}, hasCursor, at, id, 1)
		if err != nil {
			t.Fatal(err)
		}
		defer rs.Close()
		for rs.Next() {
			var sid, mid uuid.UUID
			var doc, d, tm, oc, cn string
			var created time.Time
			var sub, disc, tot, paid, recv decimal.Decimal
			if err := rs.Scan(&sid, &mid, &doc, &created, &d, &tm, &oc, &cn, &sub, &disc, &tot, &paid, &recv); err != nil {
				t.Fatal(err)
			}
			ids, ats = append(ids, sid), append(ats, created)
		}
		if rs.Err() != nil {
			t.Fatal(rs.Err())
		}
		return
	}
	i1, a1 := page(false, time.Time{}, uuid.Nil)
	if len(i1) != 1 {
		t.Fatalf("halaman 1: %d", len(i1))
	}
	i2, a2 := page(true, a1[0], i1[0])
	if len(i2) != 1 || i2[0] == i1[0] {
		t.Fatalf("keyset halaman 2 salah: %v %v", i1, i2)
	}
	if i3, _ := page(true, a2[0], i2[0]); len(i3) != 0 { // void tidak ikut → habis setelah 2 faktur
		t.Fatalf("halaman 3 seharusnya kosong: %v", i3)
	}
}
