package sales

// Ringkasan harian laporan (migration 00077): hook tulis di Create/Void/Edit/
// CreateSaleReturn/VoidSaleReturn + bacaan ByDate + isolasi tenant.
// Butuh DB: TEST_DATABASE_URL (role app), TEST_ADMIN_DATABASE_URL (pemilik),
// TEST_REDIS_URL tidak diperlukan di sini (tanpa lockout).
// Lokal: set ketiganya dari backend/.env (DATABASE_URL/MIGRATE/REDIS_URL).

import (
	"context"
	"fmt"
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"aciraba/internal/platform/db"
	reportsales "aciraba/internal/reports/sales"
)

type dailyRow struct {
	saleCount, voidCount, retCount int
	subtotal, discount, total, paidNet, receivable, voidTotal, retTotal string
}

func summaryOf(t *testing.T, e *env, day string) dailyRow {
	t.Helper()
	var r dailyRow
	err := e.admin.QueryRow(context.Background(), `
		SELECT sale_count, void_count, return_count, subtotal::text, discount::text,
		       total::text, paid_net::text, receivable::text, void_total::text, return_total::text
		FROM sales_daily_summary WHERE tenant_id = $1 AND outlet_id = $2 AND day = $3::date AND cashier_id = $4`,
		e.tenant, e.outlet, day, e.user).Scan(
		&r.saleCount, &r.voidCount, &r.retCount, &r.subtotal, &r.discount,
		&r.total, &r.paidNet, &r.receivable, &r.voidTotal, &r.retTotal)
	if err != nil {
		t.Fatalf("baca ringkasan: %v", err)
	}
	return r
}

func summaryDay(t *testing.T, e *env) string {
	t.Helper()
	var day string
	if err := e.admin.QueryRow(context.Background(),
		`SELECT day::text FROM sales_daily_summary WHERE tenant_id = $1 LIMIT 1`, e.tenant).Scan(&day); err != nil {
		t.Fatalf("hari ringkasan: %v", err)
	}
	return day
}

func methodAmounts(t *testing.T, e *env, day string) map[string]string {
	t.Helper()
	rows, err := e.admin.Query(context.Background(), `
		SELECT pm.kind, m.amount_net::text FROM sales_daily_method_summary m
		JOIN payment_methods pm ON pm.tenant_id = m.tenant_id AND pm.id = m.method_id
		WHERE m.tenant_id = $1 AND m.outlet_id = $2 AND m.day = $3::date AND m.cashier_id = $4`,
		e.tenant, e.outlet, day, e.user)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	out := map[string]string{}
	for rows.Next() {
		var kind, amt string
		if err := rows.Scan(&kind, &amt); err != nil {
			t.Fatal(err)
		}
		out[kind] = amt
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return out
}

func byDate(t *testing.T, e *env, day string) (saleCount, voidCount, retCount int, total, paid, retTotal string, methods map[string]string) {
	t.Helper()
	res, err := reportsales.NewService(e.app).ByDate(context.Background(),
		e.actor(e.tenant), reportsales.ByDateParams{From: day, To: day})
	if err != nil {
		t.Fatalf("ByDate: %v", err)
	}
	if len(res.Data) != 1 {
		t.Fatalf("ByDate hari = %d, want 1", len(res.Data))
	}
	d := res.Data[0]
	methods = map[string]string{}
	for _, m := range d.Methods {
		methods[m.Kind] = m.Amount
	}
	return d.SaleCount, d.VoidCount, d.ReturnCount, d.Total, d.Paid, d.ReturnTotal, methods
}

func wantDaily(t *testing.T, got dailyRow, want dailyRow) {
	t.Helper()
	if got != want {
		t.Errorf("ringkasan = %+v, want %+v", got, want)
	}
}

// Buat → ringkasan terisi (termasuk netto tunai = bayar − kembali);
// batal → completed dibalik + void tercatat; laporan cocok.
func TestDailySummaryCreateVoidAndReport(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	a := e.actor(e.tenant)
	a.Outlets = map[uuid.UUID]bool{e.outlet: true}
	it := e.item(t, "goods", "10000", "5000", 100, false)

	// Nota 1: 2×10.000 = 20.000; tunai 15.000 + debit 5.000 (pas, tanpa kembali).
	s1, _, err := e.svc.Create(ctx, a, key(), Request{Lines: []LineIn{line(it, "2")},
		Payments: []PaymentIn{pay("cash", "15000"), pay("debit", "5000")}})
	if err != nil {
		t.Fatal(err)
	}
	// Nota 2: 1×10.000; tunai 12.000 → kembali 2.000, netto 10.000.
	if _, _, err := e.svc.Create(ctx, a, key(), Request{Lines: []LineIn{line(it, "1")},
		Payments: []PaymentIn{pay("cash", "12000")}}); err != nil {
		t.Fatal(err)
	}
	day := summaryDay(t, e)
	wantDaily(t, summaryOf(t, e, day), dailyRow{saleCount: 2,
		subtotal: "30000.00", discount: "0.00", total: "30000.00", paidNet: "30000.00",
		receivable: "0.00", voidTotal: "0.00", retTotal: "0.00"})
	if m := methodAmounts(t, e, day); m["cash"] != "25000.00" || m["debit"] != "5000.00" {
		t.Errorf("metode = %v, want cash=25000 debit=5000", m)
	}
	if sc, vc, rc, total, paid, rt, m := byDate(t, e, day); sc != 2 || vc != 0 || rc != 0 ||
		total != "30000.00" || paid != "30000.00" || rt != "0.00" || m["cash"] != "25000.00" || m["debit"] != "5000.00" {
		t.Errorf("laporan: sc=%d vc=%d rc=%d total=%s paid=%s ret=%s m=%v", sc, vc, rc, total, paid, rt, m)
	}

	// Batal nota 1: completed-nya hilang, void tercatat pada tanggal yang sama.
	_, ap := e.editor(t)
	if _, err := e.svc.Void(ctx, a, s1.ID, VoidInput{Reason: "pelanggan batal", Approval: ap}); err != nil {
		t.Fatal(err)
	}
	wantDaily(t, summaryOf(t, e, day), dailyRow{saleCount: 1, voidCount: 1,
		subtotal: "10000.00", discount: "0.00", total: "10000.00", paidNet: "10000.00",
		receivable: "0.00", voidTotal: "20000.00", retTotal: "0.00"})
	if sc, vc, _, total, paid, _, m := byDate(t, e, day); sc != 1 || vc != 1 ||
		total != "10000.00" || paid != "10000.00" || m["cash"] != "10000.00" || m["debit"] != "" {
		t.Errorf("laporan setelah void: sc=%d vc=%d total=%s paid=%s m=%v", sc, vc, total, paid, m)
	}
}

// Edit = balik versi lama + catat revisi; jumlah nota tetap 1, superseded tak dihitung.
func TestDailySummaryEditKeepsSingleCount(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	a := e.actor(e.tenant)
	a.Outlets = map[uuid.UUID]bool{e.outlet: true}
	it := e.item(t, "goods", "10000", "5000", 10, false)
	_, ap := e.editor(t)

	orig, _, err := e.svc.Create(ctx, a, key(), Request{Lines: []LineIn{line(it, "2")},
		Payments: []PaymentIn{pay("cash", "20000")}})
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := e.svc.Edit(ctx, a, orig.ID, key(), editReq(it, "3", "30000", ap)); err != nil {
		t.Fatal(err)
	}
	day := summaryDay(t, e)
	wantDaily(t, summaryOf(t, e, day), dailyRow{saleCount: 1,
		subtotal: "30000.00", discount: "0.00", total: "30000.00", paidNet: "30000.00",
		receivable: "0.00", voidTotal: "0.00", retTotal: "0.00"})
	if m := methodAmounts(t, e, day); m["cash"] != "30000.00" {
		t.Errorf("metode setelah edit = %v", m)
	}
}

// Retur menambah kolom retur (kasir = kasir nota asal); batal retur membalikkan.
func TestDailySummaryReturnAndVoid(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	a := e.actor(e.tenant)
	a.Outlets = map[uuid.UUID]bool{e.outlet: true}
	it := e.item(t, "goods", "1000", "500", 10, false)
	member := e.newMember(t, 0, true)

	sale, _, err := e.svc.Create(ctx, a, key(), Request{Lines: []LineIn{line(it, "3")},
		Payments: []PaymentIn{pay("cash", "3000")}, MemberID: &member})
	if err != nil {
		t.Fatal(err)
	}
	deposit := saleRefundMethod(t, e, "deposit")
	ret, _, err := e.svc.CreateSaleReturn(ctx, a, key(), saleReturnRequest(sale.ID, 1, "1", &deposit))
	if err != nil {
		t.Fatal(err)
	}
	day := summaryDay(t, e)
	if got := summaryOf(t, e, day); got.retCount != 1 || got.retTotal != "1000.00" {
		t.Errorf("retur: %+v", got)
	}
	if _, _, _, _, _, rt, _ := byDate(t, e, day); rt != "1000.00" {
		t.Errorf("laporan retur = %s", rt)
	}
	if _, err := e.svc.VoidSaleReturn(ctx, a, ret.ID, "salah barang"); err != nil {
		t.Fatal(err)
	}
	if got := summaryOf(t, e, day); got.retCount != 0 || got.retTotal != "0.00" {
		t.Errorf("batal retur: %+v", got)
	}
}

// Isolasi tenant (AGENTS.md backend): ringkasan tenant lain tak terlihat dan
// tak bisa diubah; baris ringkasan tak bisa dihapus aplikasi.
func TestDailySummaryIsolation(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	it := e.item(t, "goods", "1000", "0", 50, false)
	a := e.actor(e.tenant)
	if _, _, err := e.svc.Create(ctx, a, key(), Request{Lines: []LineIn{line(it, "1")},
		Payments: []PaymentIn{pay("cash", "1000")}}); err != nil {
		t.Fatal(err)
	}
	err := db.WithTenant(ctx, e.app, e.other, func(tx pgx.Tx) error {
		for _, tbl := range []string{"sales_daily_summary", "sales_daily_method_summary"} {
			var n int
			if err := tx.QueryRow(ctx, `SELECT count(*) FROM `+tbl).Scan(&n); err != nil || n != 0 {
				return fmt.Errorf("%s terlihat %d baris dari tenant lain (err=%v)", tbl, n, err)
			}
			// UPDATE ke baris tenant lain: RLS menyaring → 0 baris, tanpa error.
			tag, err := tx.Exec(ctx, `UPDATE `+tbl+` SET tenant_id = tenant_id WHERE tenant_id = $1`, e.tenant)
			if err != nil || tag.RowsAffected() != 0 {
				return fmt.Errorf("UPDATE %s lintas tenant: rows=%d err=%v", tbl, tag.RowsAffected(), err)
			}
		}
		// Pernyataan yang SENGAJA gagal (izin dicabut / WITH CHECK) harus dibungkus
		// savepoint: error apa pun membatalkan transaksi sampai titik itu.
		mustFail := func(name, sql string, args ...any) error {
			if _, err := tx.Exec(ctx, `SAVEPOINT want_fail`); err != nil {
				return err
			}
			_, err := tx.Exec(ctx, sql, args...)
			if _, rb := tx.Exec(ctx, `ROLLBACK TO SAVEPOINT want_fail`); rb != nil {
				return rb
			}
			if err == nil {
				return fmt.Errorf("%s seharusnya ditolak", name)
			}
			return nil
		}
		// DELETE dicabut dari role aplikasi (seperti tabel sales).
		for _, tbl := range []string{"sales_daily_summary", "sales_daily_method_summary"} {
			if err := mustFail("DELETE "+tbl, `DELETE FROM `+tbl+` WHERE tenant_id = $1`, e.tenant); err != nil {
				return err
			}
		}
		// INSERT dengan tenant milik orang lain ditolak WITH CHECK.
		if err := mustFail("INSERT lintas tenant", `INSERT INTO sales_daily_summary (tenant_id, outlet_id, day, cashier_id)
			VALUES ($1, $2, current_date, $3)`, e.tenant, e.outlet, e.user); err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	// Data tenant A utuh (diperiksa lewat admin).
	var n int
	if err := e.admin.QueryRow(ctx, `SELECT sale_count FROM sales_daily_summary WHERE tenant_id = $1`, e.tenant).Scan(&n); err != nil || n != 1 {
		t.Fatalf("ringkasan tenant A = %d (%v)", n, err)
	}
}

// Pembuatan serentak pada hari yang sama: upsert atomik, tanpa angka hilang.
func TestDailySummaryConcurrent(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	it := e.item(t, "service", "5000", "0", 0, false)
	const n = 8
	var wg sync.WaitGroup
	errs := make(chan error, n)
	for range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _, err := e.svc.Create(ctx, e.actor(e.tenant), key(), Request{
				Lines: []LineIn{line(it, "1")}, Payments: []PaymentIn{pay("cash", "5000")}})
			errs <- err
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	day := summaryDay(t, e)
	if got := summaryOf(t, e, day); got.saleCount != n || got.subtotal != "40000.00" || got.paidNet != "40000.00" {
		t.Errorf("serentak: %+v", got)
	}
}
