package sales

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"aciraba/internal/platform/db"
)

// Reset metode pembayaran (danger zone): semua nota + jurnal otomatisnya hilang, metode non-bawaan hilang, metode bawaan kembali awal,
// saldo agregat jurnal dikurangi, tenant lain tidak tersentuh.
func TestDangerResetPaymentMethods(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	a := e.item(t, "goods", "1000", "0", 50, false)
	qris := e.method(t, e.tenant, "EraPay", "ewallet", true)
	if _, _, err := e.svc.Create(ctx, e.actor(e.tenant), key(), Request{Lines: []LineIn{line(a, "3")}, Payments: []PaymentIn{payID(qris, "3000")}}); err != nil {
		t.Fatal(err)
	}
	e.exec(t, `UPDATE payment_methods SET name = 'Kas Utama' WHERE tenant_id = $1 AND kind = 'cash'`, e.tenant)

	// Jurnal otomatis terposting + draf, dengan saldo agregat.
	acc1, acc2, post, draft := uuid.New(), uuid.New(), uuid.New(), uuid.New()
	e.exec(t, `INSERT INTO accounts (id, tenant_id, code, name, kind, class, normal_side) VALUES
		($1, $3, 'D-1', 'Kas', 'ledger', 'asset', 'debit'), ($2, $3, 'D-2', 'Penjualan', 'ledger', 'revenue', 'credit')`, acc1, acc2, e.tenant)
	e.exec(t, `INSERT INTO journal_entries (id, tenant_id, outlet_id, doc_no, entry_date, type, status, source_type, source_ref, posted_by, posted_at)
		VALUES ($1, $2, $3, 'JU-D1', '2026-11-01', 'SALES', 'posted', 'sales_daily', 'o:2026-11-01', $4, now())`, post, e.tenant, e.outlet, e.user)
	e.exec(t, `INSERT INTO journal_entries (id, tenant_id, outlet_id, entry_date, type, source_type, source_ref) VALUES ($1, $2, $3, '2026-11-02', 'SALES', 'sales_daily', 'o:2026-11-02')`, draft, e.tenant, e.outlet)
	e.exec(t, `INSERT INTO journal_lines (tenant_id, entry_id, entry_date, line_no, account_id, debit, credit) VALUES
		($1, $2, '2026-11-01', 1, $3, 3000, 0), ($1, $2, '2026-11-01', 2, $4, 0, 3000)`, e.tenant, post, acc1, acc2)
	e.exec(t, `INSERT INTO account_period_balances (tenant_id, outlet_id, account_id, period_month, debit, credit) VALUES
		($1, $2, $3, '2026-11-01', 5000, 0), ($1, $2, $4, '2026-11-01', 0, 3000)`, e.tenant, e.outlet, acc1, acc2)
	defer func() {
		for _, tbl := range []string{"journal_entries", "account_period_balances", "accounts"} {
			_, _ = e.admin.Exec(ctx, `DELETE FROM `+tbl+` WHERE tenant_id = $1`, e.tenant)
		}
	}()

	// Poin, deposit member, dan shift bersumber nota juga harus hilang.
	mem := uuid.New()
	e.exec(t, `INSERT INTO members (id, tenant_id, code, name, points, lifetime_points) VALUES ($1, $2, 'M-DZ', 'Member Uji', 7, 9)`, mem, e.tenant)
	defer func() {
		_, _ = e.admin.Exec(ctx, `DELETE FROM member_point_movements WHERE tenant_id = $1`, e.tenant)
		_, _ = e.admin.Exec(ctx, `DELETE FROM member_deposit_movements WHERE tenant_id = $1`, e.tenant)
	}()
	e.exec(t, `INSERT INTO member_deposit_movements (tenant_id, member_id, outlet_id, kind, amount, balance_after, method_id, method_name, method, idempotency_key)
		SELECT $1, $2, $3, 'TOPUP', 5000, 5000, id, 'Tunai', 'cash', 'dz-topup-key' FROM payment_methods WHERE tenant_id = $1 AND kind = 'cash'`, e.tenant, mem, e.outlet)
	if e.count(t, "sales") != 1 {
		t.Fatalf("sales sebelum reset: %d", e.count(t, "sales"))
	}
	before := e.count(t, "payment_methods")
	var out string
	if err := db.WithTenant(ctx, e.app, e.tenant, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT danger_reset_payment_methods($1)::text`, e.tenant).Scan(&out)
	}); err != nil {
		t.Fatal(err)
	}
	t.Log(out)

	var sales, lines, pays, journals, jl int
	var names string
	var debit1 string
	q := func(sql string, dst ...any) {
		t.Helper()
		if err := e.admin.QueryRow(ctx, sql, e.tenant).Scan(dst...); err != nil {
			t.Fatal(err)
		}
	}
	var movs, deps, pts int
	q(`SELECT count(*) FROM stock_movements WHERE tenant_id = $1 AND ref_type = 'SALE'`, &movs)
	q(`SELECT count(*) FROM member_deposit_movements WHERE tenant_id = $1`, &deps)
	q(`SELECT points FROM members WHERE tenant_id = $1 AND code = 'M-DZ'`, &pts)
	if movs != 0 || deps != 0 || pts != 0 {
		t.Fatalf("stok jual=%d deposit=%d poin=%d harus nol", movs, deps, pts)
	}
	q(`SELECT count(*) FROM sales WHERE tenant_id = $1`, &sales)
	q(`SELECT count(*) FROM sale_lines WHERE tenant_id = $1`, &lines)
	q(`SELECT count(*) FROM sale_payments WHERE tenant_id = $1`, &pays)
	q(`SELECT count(*) FROM journal_entries WHERE tenant_id = $1`, &journals)
	q(`SELECT count(*) FROM journal_lines WHERE tenant_id = $1`, &jl)
	q(`SELECT string_agg(name, ',' ORDER BY name) FROM payment_methods WHERE tenant_id = $1`, &names)
	q(`SELECT debit::text FROM account_period_balances WHERE tenant_id = $1 AND account_id = (SELECT id FROM accounts WHERE tenant_id = $1 AND code = 'D-1')`, &debit1)
	if sales+lines+pays+journals+jl != 0 {
		t.Fatalf("transaksi tersisa: sales=%d lines=%d pays=%d journals=%d jl=%d", sales, lines, pays, journals, jl)
	}
	if names != "Deposit Member,Kredit Pemasok,Tunai" {
		t.Fatalf("metode setelah reset: %s (sebelum %d)", names, before)
	}
	if debit1 != "2000.00" {
		t.Fatalf("saldo agregat harus berkurang 3000: %s", debit1)
	}
	var fee string
	q(`SELECT name FROM payment_methods WHERE tenant_id = $1 AND kind = 'cash'`, &fee)
	if fee != "Tunai" {
		t.Fatalf("nama Tunai kembali awal: %s", fee)
	}
	var other int
	if err := e.admin.QueryRow(ctx, `SELECT count(*) FROM payment_methods WHERE tenant_id = $1`, e.other).Scan(&other); err != nil || other != 7 {
		t.Fatalf("tenant lain tersentuh: %d %v", other, err)
	}

	// Tenant sesi lain ditolak.
	err := db.WithTenant(ctx, e.app, e.tenant, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `SELECT danger_clear_sales($1)`, e.other)
		return err
	})
	if err == nil {
		t.Fatal("fungsi harus menolak tenant selain sesi")
	}
}
