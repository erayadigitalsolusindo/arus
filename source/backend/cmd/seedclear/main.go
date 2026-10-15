// Command seedclear menghapus SEMUA transaksi (penjualan, retur, piutang, pembayaran
// piutang, voucher, pembelian, retur beli, hutang, gerakan stok, saldo, poin member,
// jurnal otomatis, shift kasir, kas kecil, ringkasan harian, dan penomoran) pada satu
// tenant dev. Pasangan cmd/seedsales: dipakai saat ingin mengosongkan transaksi untuk
// mengulang pengujian dari nol (total reset).
//
//	cd source/backend && go run ./cmd/seedclear           # dry-run: lihat yang akan dihapus
//	cd source/backend && go run ./cmd/seedclear --yes     # eksekusi penghapusan
//
// Akses memakai MIGRATE_DATABASE_URL (pemilik skema; melewati RLS) karena DELETE
// dicabut dari role aplikasi pada tabel transaksi (AGENTS.md: nota tak dihapus
// permanen — tool ini khusus mereset data dev, bukan produksi).
//
// Env: SEED_TENANT_CODE (kosong = satu-satunya tenant) atau SEED_USER_EMAIL
// (bawaan aciraba.id@gmail.com). Master (barang/metode/member/level/akun/supplier)
// TIDAK dihapus; catatan audit tetap (append-only). Catatan: barang tetap
// ada (150rb item dari uji beban) — bila ingin barang demo, jalankan cmd/seeddemo
// setelahnya.
package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/joho/godotenv"
)

// Langkah hapus, urutan anak-dalam-lebih-dulu sesuai peta FK:
//   sales → sale_lines/sale_payments/sale_costs/sale_vouchers/sales_returns/receivables/erapay_charges
//   receivables ← receivable_payments ← receivable_settlements
//   purchases → purchase_lines/purchase_costs/payables/purchase_returns
//   payables ← payable_payments ← payable_settlements
//   sales/purchases punya self-FK Rantai (supersedes/root): drop dulu, baru hapus.
//   journal_lines ← journal_entries (hanya sumber otomatis transaksi).
type step struct {
	name string
	sql  string
}

var steps = []step{
	// ── Rantai penjualan ────────────────────────────────────────────────
	{"sales_return_lines", `DELETE FROM sales_return_lines WHERE tenant_id = $1`},
	{"sales_returns", `DELETE FROM sales_returns WHERE tenant_id = $1`},
	{"receivable_payments", `DELETE FROM receivable_payments WHERE tenant_id = $1`},
	{"receivable_settlements", `DELETE FROM receivable_settlements WHERE tenant_id = $1`},
	{"receivables", `DELETE FROM receivables WHERE tenant_id = $1`},
	{"sale_vouchers", `DELETE FROM sale_vouchers WHERE tenant_id = $1`},
	{"sale_costs", `DELETE FROM sale_costs WHERE tenant_id = $1`},
	{"sale_payments", `DELETE FROM sale_payments WHERE tenant_id = $1`},
	{"sale_lines", `DELETE FROM sale_lines WHERE tenant_id = $1`},
	{"erapay_charges", `DELETE FROM erapay_charges WHERE tenant_id = $1`},
	// ── Rantai pembelian ────────────────────────────────────────────────
	{"purchase_return_lines", `DELETE FROM purchase_return_lines WHERE tenant_id = $1`},
	{"purchase_returns", `DELETE FROM purchase_returns WHERE tenant_id = $1`},
	{"payable_payments", `DELETE FROM payable_payments WHERE tenant_id = $1`},
	{"payable_settlements", `DELETE FROM payable_settlements WHERE tenant_id = $1`},
	{"payables", `DELETE FROM payables WHERE tenant_id = $1`},
	{"purchase_costs", `DELETE FROM purchase_costs WHERE tenant_id = $1`},
	{"purchase_lines", `DELETE FROM purchase_lines WHERE tenant_id = $1`},
	// ── Pembayaran poin & jurnal otomatis ───────────────────────────────
	{"member_point_movements", `DELETE FROM member_point_movements WHERE tenant_id = $1`},
	{"member_deposit_movements", `DELETE FROM member_deposit_movements WHERE tenant_id = $1`},
	{"supplier_credit_movements", `DELETE FROM supplier_credit_movements WHERE tenant_id = $1`},
	{"journal_lines", `DELETE FROM journal_lines WHERE tenant_id = $1`},
	{"journal_entries", `DELETE FROM journal_entries WHERE tenant_id = $1`},
	// ── Induk transaksi ────────────────────────────────────────────────
	// Nota revisi saling mereferensi dengan nota aslinya (supersedes_id ← →
	// superseded_by, keduanya RESTRICT) sehingga DELETE biasa buntu. Solusi:
	// DROP self-FK dalam transaksi yang sama, hapus, lalu pulihkan constraint
	// (DDL transaksional di Postgres; setelah baris habis constraint valid).
	// Setiap statement dieksekusi terpisah (pgx menolak multi-command).
	{"sales drop supersedes_fk", `ALTER TABLE sales DROP CONSTRAINT IF EXISTS sales_supersedes_fk`},
	{"sales drop superseded_by_fk", `ALTER TABLE sales DROP CONSTRAINT IF EXISTS sales_superseded_by_fk`},
	{"sales drop root_fk", `ALTER TABLE sales DROP CONSTRAINT IF EXISTS sales_root_fk`},
	{"sales", `DELETE FROM sales WHERE tenant_id = $1`},
	{"sales add supersedes_fk", `ALTER TABLE sales ADD CONSTRAINT sales_supersedes_fk FOREIGN KEY (tenant_id, supersedes_id) REFERENCES sales (tenant_id, id) ON DELETE RESTRICT`},
	{"sales add superseded_by_fk", `ALTER TABLE sales ADD CONSTRAINT sales_superseded_by_fk FOREIGN KEY (tenant_id, superseded_by) REFERENCES sales (tenant_id, id) ON DELETE RESTRICT`},
	{"sales add root_fk", `ALTER TABLE sales ADD CONSTRAINT sales_root_fk FOREIGN KEY (tenant_id, root_id) REFERENCES sales (tenant_id, id) ON DELETE RESTRICT`},
	{"purchases drop root_fk", `ALTER TABLE purchases DROP CONSTRAINT IF EXISTS purchases_root_fk`},
	{"purchases drop supersedes_fk", `ALTER TABLE purchases DROP CONSTRAINT IF EXISTS purchases_supersedes_fk`},
	{"purchases drop superseded_by_fk", `ALTER TABLE purchases DROP CONSTRAINT IF EXISTS purchases_superseded_by_fk`},
	{"purchases", `DELETE FROM purchases WHERE tenant_id = $1`},
	{"purchases add root_fk", `ALTER TABLE purchases ADD CONSTRAINT purchases_root_fk FOREIGN KEY (tenant_id, root_id) REFERENCES purchases (tenant_id, id)`},
	{"purchases add supersedes_fk", `ALTER TABLE purchases ADD CONSTRAINT purchases_supersedes_fk FOREIGN KEY (tenant_id, supersedes_id) REFERENCES purchases (tenant_id, id)`},
	{"purchases add superseded_by_fk", `ALTER TABLE purchases ADD CONSTRAINT purchases_superseded_by_fk FOREIGN KEY (tenant_id, superseded_by) REFERENCES purchases (tenant_id, id) DEFERRABLE INITIALLY DEFERRED`}, // wajib deferred (migration 00040)
	// ── Stok: ledger + saldo ────────────────────────────────────────────
	{"stock_count_lines", `DELETE FROM stock_count_lines WHERE tenant_id = $1`},
	{"stock_movements", `DELETE FROM stock_movements WHERE tenant_id = $1`},
	{"stock_counts", `DELETE FROM stock_counts WHERE tenant_id = $1`},
	{"stock_balances", `DELETE FROM stock_balances WHERE tenant_id = $1`},
	// ── Ringkasan harian, shift, dan penomoran ──────────────────────────
	{"sales_daily_summary", `DELETE FROM sales_daily_summary WHERE tenant_id = $1`},
	{"sales_daily_method_summary", `DELETE FROM sales_daily_method_summary WHERE tenant_id = $1`},
	{"sale_counters", `DELETE FROM sale_counters WHERE tenant_id = $1`},
	{"sales_return_counters", `DELETE FROM sales_return_counters WHERE tenant_id = $1`},
	{"receivable_payment_counters", `DELETE FROM receivable_payment_counters WHERE tenant_id = $1`},
	{"purchase_counters", `DELETE FROM purchase_counters WHERE tenant_id = $1`},
	{"purchase_return_counters", `DELETE FROM purchase_return_counters WHERE tenant_id = $1`},
	{"payable_payment_counters", `DELETE FROM payable_payment_counters WHERE tenant_id = $1`},
	{"cash_shift_counters", `DELETE FROM cash_shift_counters WHERE tenant_id = $1`},
	{"cash_shift_counts", `DELETE FROM cash_shift_counts WHERE tenant_id = $1`},
	{"cash_shift_flows", `DELETE FROM cash_shift_flows WHERE tenant_id = $1`},
	{"cash_shifts", `DELETE FROM cash_shifts WHERE tenant_id = $1`},
	{"cash_movements", `DELETE FROM cash_movements WHERE tenant_id = $1`},
	// ── Kolom turunan member ────────────────────────────────────────────
	{"members.points/lifetime", `UPDATE members SET points = 0, lifetime_points = 0 WHERE tenant_id = $1`},
}

// countTables = tabel yang dihitung untuk laporan dry-run.
var countTables = []struct {
	name string
	sql  string
}{
	{"sales", `SELECT count(*) FROM sales WHERE tenant_id = $1`},
	{"sale_lines", `SELECT count(*) FROM sale_lines WHERE tenant_id = $1`},
	{"sale_payments", `SELECT count(*) FROM sale_payments WHERE tenant_id = $1`},
	{"sale_costs", `SELECT count(*) FROM sale_costs WHERE tenant_id = $1`},
	{"sale_vouchers", `SELECT count(*) FROM sale_vouchers WHERE tenant_id = $1`},
	{"sales_returns", `SELECT count(*) FROM sales_returns WHERE tenant_id = $1`},
	{"sales_return_lines", `SELECT count(*) FROM sales_return_lines WHERE tenant_id = $1`},
	{"receivables", `SELECT count(*) FROM receivables WHERE tenant_id = $1`},
	{"receivable_payments", `SELECT count(*) FROM receivable_payments WHERE tenant_id = $1`},
	{"receivable_settlements", `SELECT count(*) FROM receivable_settlements WHERE tenant_id = $1`},
	{"erapay_charges", `SELECT count(*) FROM erapay_charges WHERE tenant_id = $1`},
	{"purchases", `SELECT count(*) FROM purchases WHERE tenant_id = $1`},
	{"purchase_returns", `SELECT count(*) FROM purchase_returns WHERE tenant_id = $1`},
	{"payables", `SELECT count(*) FROM payables WHERE tenant_id = $1`},
	{"payable_payments", `SELECT count(*) FROM payable_payments WHERE tenant_id = $1`},
	{"payable_settlements", `SELECT count(*) FROM payable_settlements WHERE tenant_id = $1`},
	{"member_point_movements", `SELECT count(*) FROM member_point_movements WHERE tenant_id = $1`},
	{"member_deposit_movements", `SELECT count(*) FROM member_deposit_movements WHERE tenant_id = $1`},
	{"supplier_credit_movements", `SELECT count(*) FROM supplier_credit_movements WHERE tenant_id = $1`},
	{"journal_entries", `SELECT count(*) FROM journal_entries WHERE tenant_id = $1`},
	{"journal_lines", `SELECT count(*) FROM journal_lines WHERE tenant_id = $1`},
	{"stock_movements", `SELECT count(*) FROM stock_movements WHERE tenant_id = $1`},
	{"stock_balances", `SELECT count(*) FROM stock_balances WHERE tenant_id = $1`},
	{"stock_count_lines", `SELECT count(*) FROM stock_count_lines WHERE tenant_id = $1`},
	{"sales_daily_summary", `SELECT count(*) FROM sales_daily_summary WHERE tenant_id = $1`},
	{"sales_daily_method_summary", `SELECT count(*) FROM sales_daily_method_summary WHERE tenant_id = $1`},
	{"cash_shifts", `SELECT count(*) FROM cash_shifts WHERE tenant_id = $1`},
	{"cash_movements", `SELECT count(*) FROM cash_movements WHERE tenant_id = $1`},
}

func main() {
	if err := godotenv.Load(); err != nil && !errors.Is(err, os.ErrNotExist) {
		log.Fatalf("muat .env: %v", err)
	}
	dsn := os.Getenv("MIGRATE_DATABASE_URL")
	if dsn == "" {
		log.Fatal("MIGRATE_DATABASE_URL kosong (butuh pemilik skema yang melewati RLS)")
	}
	confirm := len(os.Args) > 1 && os.Args[1] == "--yes"

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		log.Fatalf("connect DB: %v", err)
	}
	defer pool.Close()

	tid, err := resolveTenant(ctx, pool)
	if err != nil {
		log.Fatalf("resolve tenant: %v", err)
	}
	var tenantName string
	if err := pool.QueryRow(ctx, `SELECT name FROM tenants WHERE id = $1`, tid).Scan(&tenantName); err != nil {
		log.Fatalf("baca nama tenant: %v", err)
	}
	fmt.Printf("tenant: %s (%s)\n\n", tenantName, tid)

	// ── Ringkasan yang akan dihapus ───────────────────────────────────────
	for _, t := range countTables {
		var n int64
		if err := pool.QueryRow(ctx, t.sql, tid).Scan(&n); err != nil {
			fmt.Printf("  %-24s ? (%v)\n", t.name, err)
			continue
		}
		fmt.Printf("  %-24s %d baris\n", t.name, n)
	}
	var rangeOut string
	err = pool.QueryRow(ctx,
		`SELECT count(*)::text || ' nota (' ||
		        coalesce(to_char(min(created_at), 'YYYY-MM-DD'), '-') || ' … ' ||
		        coalesce(to_char(max(created_at), 'YYYY-MM-DD'), '-') || ')'
		   FROM sales WHERE tenant_id = $1`, tid).Scan(&rangeOut)
	if err == nil && rangeOut != "" {
		fmt.Println("\nrentang penjualan:", rangeOut)
	}
	var n int64
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM sales WHERE tenant_id = $1`, tid).Scan(&n); err != nil {
		log.Fatalf("hitung nota: %v", err)
	}

	if !confirm {
		fmt.Printf("\nDRY-RUN: %d nota akan dihapus berserta seluruh transaksi terkait\n", n)
		fmt.Println("(retur, piutang, pembelian, hutang, gerakan stok, saldo, poin member, jurnal, shift,")
		fmt.Println("ringkasan harian, dan penomoran). Master barang/metode/member/akun tidak dihapus;")
		fmt.Println("audit tetap (append-only). Jalankan dengan --yes untuk benar-benar menghapus.")
		return
	}

	// ── Eksekusi dalam satu transaksi (semua atau tidak sama sekali) ──────
	tx, err := pool.Begin(ctx)
	if err != nil {
		log.Fatalf("BEGIN: %v", err)
	}
	defer tx.Rollback(ctx)

	var total int64
	if err := tx.QueryRow(ctx, `SELECT count(*) FROM sales WHERE tenant_id = $1`, tid).Scan(&total); err != nil {
		log.Fatalf("recount: %v", err)
	}
	for _, s := range steps {
		var tag pgconn.CommandTag
		var err error
		if strings.Contains(s.sql, "$1") {
			tag, err = tx.Exec(ctx, s.sql, tid)
		} else {
			// DDL (ALTER TABLE …) tidak punya placeholder tenant.
			tag, err = tx.Exec(ctx, s.sql)
		}
		if err != nil {
			log.Fatalf("langkah %s: %v", s.name, err)
		}
		fmt.Printf("  %-26s hapus %d baris\n", s.name, tag.RowsAffected())
	}
	if err := tx.Commit(ctx); err != nil {
		log.Fatalf("COMMIT: %v", err)
	}
	fmt.Printf("\nSELESAI: %d nota (+ seluruh transaksi & gerakan stok) dihapus. Transaksi siap dimulai dari nol.\n", total)
	fmt.Println("Saldo awal stok hilang — isi ulang lewat onboarding/OPNAME. Barang/metode/member tetap.")
}

// resolveTenant: SEED_TENANT_CODE (unik) atau SEED_USER_EMAIL (tenant pengguna).
func resolveTenant(ctx context.Context, pool *pgxpool.Pool) (uuid.UUID, error) {
	if code := os.Getenv("SEED_TENANT_CODE"); code != "" {
		var id uuid.UUID
		err := pool.QueryRow(ctx, `SELECT id FROM tenants WHERE code = $1`, code).Scan(&id)
		if errors.Is(err, pgx.ErrNoRows) {
			return uuid.Nil, fmt.Errorf("tenant code %q tidak ada", code)
		}
		return id, err
	}
	email := os.Getenv("SEED_USER_EMAIL")
	if email == "" {
		email = "aciraba.id@gmail.com"
	}
	var id uuid.UUID
	err := pool.QueryRow(ctx, `SELECT tenant_id FROM users WHERE lower(email) = lower($1)`, email).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return uuid.Nil, fmt.Errorf("pengguna %q tidak ada (SEED_TENANT_CODE/SEED_USER_EMAIL)", email)
	}
	return id, err
}
