package sales

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"aciraba/internal/authz"
)

func sampleCashierResult() CashierResult {
	m := CashierMethod{Name: "Tunai", Kind: "cash", Sales: "100000.00", Flows: "-5000.00", Expected: "195000.00", Counted: "190000.00", Diff: "-5000.00"}
	c := CashierTotal{UserID: "u1", Name: "Ani", Shifts: 1, SaleCount: 3, SalesTotal: "100000.00", Expected: "195000.00", Counted: "190000.00",
		Diff: "-5000.00", DiffAbs: "5000.00", DiffShifts: 1, CashExpected: "195000.00", CashCounted: "190000.00", CashDiff: "-5000.00",
		CashOpening: "100000.00", Methods: []CashierMethod{m}}
	return CashierResult{From: "2026-10-01", To: "2026-10-15", TenantName: "Toko", OutletName: "Pusat", OutletCode: "MAIN", HasSummary: true,
		Summary: c, Cashiers: []CashierTotal{c},
		Data: []CashierShift{{ID: "s1", DocNo: "SH-1", Status: "closed", OutletCode: "MAIN", CashierName: "Ani", OpenedDate: "2026-10-01",
			OpenedTime: "08:00", ClosedTime: "16:00", Opening: "100000.00", SaleCount: 3, SalesTotal: "100000.00", CashExpected: "195000.00",
			CashCounted: "190000.00", CashDiff: "-5000.00", Diff: "-5000.00", Note: "kurang", ApprovedBy: "Budi"}}}
}

func TestCashierExports(t *testing.T) {
	res := sampleCashierResult()
	x, err := CashierToXLSX(res)
	if err != nil || len(x) < 4 || string(x[:2]) != "PK" {
		t.Fatalf("xlsx tidak valid: %v", err)
	}
	p, err := CashierToPDF(res)
	if err != nil || !strings.HasPrefix(string(p), "%PDF") {
		t.Fatalf("pdf tidak valid: %v", err)
	}
}

// TestCashierQueriesAgainstDB menjalankan query asli terhadap skema nyata di dalam transaksi yang di-rollback
// (tidak ada data tersisa): rekonsiliasi per kasir/metode/shift harus cocok dengan data beku shift.
func TestCashierQueriesAgainstDB(t *testing.T) {
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

	var tenant, outlet, user, cashM uuid.UUID
	err = tx.QueryRow(ctx, `SELECT o.tenant_id, o.id, u.id, pm.id FROM outlets o
		JOIN users u ON u.tenant_id = o.tenant_id
		JOIN payment_methods pm ON pm.tenant_id = o.tenant_id AND pm.kind = 'cash'
		ORDER BY o.created_at LIMIT 1`).Scan(&tenant, &outlet, &user, &cashM)
	if err != nil {
		t.Skip("tidak ada tenant dev untuk fixture: ", err)
	}
	// Dua shift tertutup (satu selisih −5.000) + satu berjalan, tanggal jauh di masa depan agar tidak bentrok.
	day := time.Date(2099, 1, 5, 3, 0, 0, 0, time.UTC)
	ins := func(no, status string, at time.Time, diffAbs, counted string) uuid.UUID {
		id := uuid.New()
		if status == "open" {
			_, err = tx.Exec(ctx, `INSERT INTO cash_shifts (id, tenant_id, outlet_id, user_id, doc_no, status, opening_cash, opened_at)
				VALUES ($1,$2,$3,$4,$5,'open',100000,$6)`, id, tenant, outlet, user, no, at)
		} else {
			_, err = tx.Exec(ctx, `INSERT INTO cash_shifts (id, tenant_id, outlet_id, user_id, doc_no, status, opening_cash, opened_at, closed_at, closed_by,
				expected_total, counted_total, diff_total, diff_abs, sale_count, void_count, sales_total, receivable_total, note, approved_by, approved_name, close_key)
				VALUES ($1,$2,$3,$4::uuid,$5,'closed',100000,$6,$7,$4::uuid,195000,$8::numeric,$8::numeric-195000,$9::numeric,3,1,100000,0,
				        'kurang', CASE WHEN $9::numeric > 0 THEN $4::uuid END, CASE WHEN $9::numeric > 0 THEN 'Penyetuju' END, $10)`,
				id, tenant, outlet, user, no, at, at.Add(8*time.Hour), counted, diffAbs, "zz-"+id.String())
		}
		if err != nil {
			t.Fatalf("fixture shift: %v", err)
		}
		if status == "closed" {
			_, err = tx.Exec(ctx, `INSERT INTO cash_shift_counts (tenant_id, shift_id, position, method_id, method_name, kind, sales, flows, opening, expected, counted, diff)
				VALUES ($1,$2,1,$3,'Tunai','cash',100000,-5000,100000,195000,$4::numeric,$4::numeric-195000)`, tenant, id, cashM, counted)
			if err != nil {
				t.Fatalf("fixture counts: %v", err)
			}
		}
		return id
	}
	ins("ZZ-T1", "closed", day, "5000", "190000")
	ins("ZZ-T2", "closed", day.Add(24*time.Hour), "0", "195000")
	ins("ZZ-T3", "open", day.Add(48*time.Hour), "0", "0")

	var tz string
	if err := tx.QueryRow(ctx, `SELECT timezone FROM outlets WHERE id = $1`, outlet).Scan(&tz); err != nil {
		t.Fatal(err)
	}
	fd, td := time.Date(2099, 1, 1, 0, 0, 0, 0, time.UTC), time.Date(2099, 1, 31, 0, 0, 0, 0, time.UTC)
	outlets := []uuid.UUID{outlet}
	res := &CashierResult{}
	if err := (&Service{}).cashierSummary(ctx, tx, res, authz.Actor{TenantID: tenant}, outlets, fd, td, uuid.Nil, "", false); err != nil {
		t.Fatal(err)
	}
	if len(res.Cashiers) != 1 || res.Summary.Shifts != 3 || res.Summary.OpenShifts != 1 || res.Summary.DiffShifts != 1 {
		t.Fatalf("ringkasan salah: %+v", res.Summary)
	}
	if res.Summary.Diff != "-5000.00" || res.Summary.CashDiff != "-5000.00" || res.Summary.CashExpected != "390000.00" || res.Summary.CashCounted != "385000.00" {
		t.Fatalf("rekonsiliasi kas salah: %+v", res.Summary)
	}
	if res.Summary.SaleCount != 6 || res.Summary.SalesTotal != "200000.00" {
		t.Fatalf("penjualan salah: %+v", res.Summary)
	}
	if len(res.Summary.Methods) != 1 || res.Summary.Methods[0].Diff != "-5000.00" {
		t.Fatalf("metode salah: %+v", res.Summary.Methods)
	}

	rows, err := tx.Query(ctx, cashierShiftsSQL, tenant, outlets, fd, td, uuid.Nil, "", true, false, time.Time{}, uuid.Nil, 10)
	if err != nil {
		t.Fatal(err)
	}
	n := 0
	for rows.Next() {
		n++
	}
	rows.Close()
	if rows.Err() != nil || n != 1 { // diff_only → hanya shift selisih
		t.Fatalf("diff_only: %d baris, err=%v", n, rows.Err())
	}
	rows, err = tx.Query(ctx, cashierShiftsSQL, tenant, outlets, fd, td, uuid.Nil, "open", false, false, time.Time{}, uuid.Nil, 10)
	if err != nil {
		t.Fatal(err)
	}
	n = 0
	for rows.Next() {
		n++
	}
	rows.Close()
	if rows.Err() != nil || n != 1 {
		t.Fatalf("status open: %d baris, err=%v", n, rows.Err())
	}
	_ = tz
}
