package sales

// Ringkasan penjualan harian (migration 00077): laporan per tanggal + ekspor
// membaca tabel ini (≤366 baris, indeks penuh) sehingga tetap cepat walau
// tabel sales mencapai ratusan juta baris.
//
// Kontrak (harus identik dengan semantik live query laporan):
//   - grain (outlet, tanggal LOKAL outlet, kasir); kasir NULL → sentinel nol.
//   - nota completed menambah agregat completed; void menambah void (keduanya
//     pada TANGGAL NOTA, bukan tanggal aksi); revisi membalik versi lama lalu
//     menerapkan versi baru; retur menambah kolom retur pada TANGGAL RETUR
//     dengan kasir = kasir NOTA ASAL.
//   - amount_netto per metode = Σ pembayaran − kembalian nota ini untuk
//     metode tunai (persis pay_hdr lama).
// Semua fungsi idempotent-terhadap-status: hanya bergerak bila status baris
// sumber sesuai harapan pemanggil, sehingga replay tak menggandakan angka.

import (
	"context"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/shopspring/decimal"
)

// zeroCashier = sentinel kasir NULL (nota tanpa kasir). Filter kasir spesifik
// tak pernah cocok sentinel; filter "semua" menjumlahkan semua baris.
var zeroCashier = uuid.MustParse("00000000-0000-0000-0000-000000000000")

// saleAgg = agregat satu nota completed untuk ringkasan.
type saleAgg struct {
	outlet  uuid.UUID
	day     pgtype.Date
	cashier uuid.UUID
	subtotal, discount, taxStore, taxGov, total, paidNet, receivable decimal.Decimal
	methods []methodAgg
}

type methodAgg struct {
	methodID              uuid.UUID
	amountNet, fee, surch decimal.Decimal
}

// loadSaleAgg membaca agregat satu nota langsung dari baris tersimpan
// (satu-satunya sumber kebenaran; tanpa bergantung struct kalkulasi pemanggil).
// completedOnly=true: mengembalikan completed=false bila status bukan completed
// (pemanggil membalik/menerapkan hanya nota completed).
func loadSaleAgg(ctx context.Context, tx pgx.Tx, tenant, saleID uuid.UUID, completedOnly bool) (a saleAgg, completed bool, err error) {
	var status string
	var subtotal, discount, taxStore, taxGov, total, paid, change, receivable decimal.Decimal
	var outletID, cashierID pgtype.UUID
	var localDay pgtype.Date
	err = tx.QueryRow(ctx, `
		SELECT s.status, s.subtotal, s.discount, s.tax_store, s.tax_gov, s.total,
		       s.paid, s.change, s.receivable, s.outlet_id, s.cashier_id,
		       (s.created_at AT TIME ZONE o.timezone)::date AS day
		FROM sales s
		JOIN outlets o ON o.tenant_id = s.tenant_id AND o.id = s.outlet_id
		WHERE s.tenant_id = $1 AND s.id = $2`,
		tenant, saleID).Scan(&status, &subtotal, &discount, &taxStore, &taxGov, &total,
		&paid, &change, &receivable, &outletID, &cashierID, &localDay)
	if err != nil {
		return saleAgg{}, false, err
	}
	if completedOnly && status != "completed" {
		return saleAgg{}, false, nil
	}
	a = saleAgg{
		outlet: outletID.Bytes, day: localDay, cashier: uuid.UUID(cashierID.Bytes),
		subtotal: subtotal, discount: discount, taxStore: taxStore, taxGov: taxGov,
		total: total, paidNet: paid.Sub(change), receivable: receivable,
	}
	rows, err := tx.Query(ctx, `
		SELECT p.method_id,
		       sum(p.amount) - CASE WHEN pm.kind = 'cash' THEN $3::numeric ELSE 0 END AS amt,
		       coalesce(sum(p.fee_amount) FILTER (WHERE p.fee_bearer = 'store'), 0) AS fee,
		       coalesce(sum(p.fee_amount) FILTER (WHERE p.fee_bearer = 'customer'), 0) AS surcharge
		FROM sale_payments p
		JOIN payment_methods pm ON pm.tenant_id = p.tenant_id AND pm.id = p.method_id
		WHERE p.tenant_id = $1 AND p.sale_id = $2
		GROUP BY p.method_id, pm.kind
		ORDER BY p.method_id`,
		tenant, saleID, change)
	if err != nil {
		return saleAgg{}, false, err
	}
	defer rows.Close()
	for rows.Next() {
		var m methodAgg
		if err := rows.Scan(&m.methodID, &m.amountNet, &m.fee, &m.surch); err != nil {
			return saleAgg{}, false, err
		}
		a.methods = append(a.methods, m)
	}
	if err := rows.Err(); err != nil {
		return saleAgg{}, false, err
	}
	return a, true, nil
}

// adjustDaily menambah (sign=+1) atau membalik (sign=−1) agregat harian.
func adjustDaily(ctx context.Context, tx pgx.Tx, tenant uuid.UUID, a saleAgg, sign int) error {
	_, err := tx.Exec(ctx, `
		INSERT INTO sales_daily_summary AS d
		    (tenant_id, outlet_id, day, cashier_id, sale_count, subtotal, discount,
		     tax_store, tax_gov, total, paid_net, receivable)
		VALUES ($1, $2, $3, $4, 1, $5, $6, $7, $8, $9, $10, $11)
		ON CONFLICT (tenant_id, outlet_id, day, cashier_id) DO UPDATE SET
		    sale_count = d.sale_count + $12,
		    subtotal   = d.subtotal   + $5 * $12, discount = d.discount + $6 * $12,
		    tax_store  = d.tax_store  + $7 * $12, tax_gov = d.tax_gov + $8 * $12,
		    total      = d.total      + $9 * $12, paid_net = d.paid_net + $10 * $12,
		    receivable = d.receivable + $11 * $12`,
		tenant, a.outlet, a.day.Time, a.cashier,
		a.subtotal, a.discount, a.taxStore, a.taxGov, a.total, a.paidNet, a.receivable,
		sign)
	if err != nil {
		return err
	}
	for _, m := range a.methods {
		if _, err := tx.Exec(ctx, `
			INSERT INTO sales_daily_method_summary AS d
			    (tenant_id, outlet_id, day, cashier_id, method_id, amount_net, fee, surcharge)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
			ON CONFLICT (tenant_id, outlet_id, day, cashier_id, method_id) DO UPDATE SET
			    amount_net = d.amount_net + $6 * $9,
			    fee        = d.fee        + $7 * $9,
			    surcharge  = d.surcharge  + $8 * $9`,
			tenant, a.outlet, a.day.Time, a.cashier, m.methodID,
			m.amountNet, m.fee, m.surch, sign); err != nil {
			return err
		}
	}
	return nil
}

// ApplySale mencatat nota completed baru (atau revisi baru) ke ringkasan.
func ApplySale(ctx context.Context, tx pgx.Tx, tenant, saleID uuid.UUID) error {
	a, ok, err := loadSaleAgg(ctx, tx, tenant, saleID, true)
	if err != nil || !ok {
		return err
	}
	return adjustDaily(ctx, tx, tenant, a, 1)
}

// ReverseSale membalik agregat completed satu nota (sebelum void/supersede).
func ReverseSale(ctx context.Context, tx pgx.Tx, tenant, saleID uuid.UUID) error {
	a, ok, err := loadSaleAgg(ctx, tx, tenant, saleID, true)
	if err != nil || !ok {
		return err
	}
	return adjustDaily(ctx, tx, tenant, a, -1)
}

// ApplyVoid mencatat pembatalan: void_count/void_total pada tanggal nota.
func ApplyVoid(ctx context.Context, tx pgx.Tx, tenant, saleID uuid.UUID) error {
	var outletID, cashierID pgtype.UUID
	var localDay pgtype.Date
	var total decimal.Decimal
	err := tx.QueryRow(ctx, `
		SELECT s.outlet_id, s.cashier_id,
		       (s.created_at AT TIME ZONE o.timezone)::date AS day, s.total
		FROM sales s
		JOIN outlets o ON o.tenant_id = s.tenant_id AND o.id = s.outlet_id
		WHERE s.tenant_id = $1 AND s.id = $2 AND s.status = 'void'`,
		tenant, saleID).Scan(&outletID, &cashierID, &localDay, &total)
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `
		INSERT INTO sales_daily_summary AS d
		    (tenant_id, outlet_id, day, cashier_id, void_count, void_total)
		VALUES ($1, $2, $3, $4, 1, $5)
		ON CONFLICT (tenant_id, outlet_id, day, cashier_id) DO UPDATE SET
		    void_count = d.void_count + 1,
		    void_total = d.void_total + $5`,
		tenant, outletID.Bytes, localDay.Time, uuid.UUID(cashierID.Bytes), total)
	return err
}

// adjustReturn menambah (sign=+1) atau membalik (sign=−1) kolom retur pada
// (outlet retur, tanggal lokal retur, kasir nota asal).
func adjustReturn(ctx context.Context, tx pgx.Tx, tenant, returnID uuid.UUID, sign int) error {
	var outletID, saleCashier pgtype.UUID
	var localDay pgtype.Date
	var total decimal.Decimal
	err := tx.QueryRow(ctx, `
		SELECT r.outlet_id, s.cashier_id,
		       (r.created_at AT TIME ZONE o.timezone)::date AS day, r.total
		FROM sales_returns r
		JOIN outlets o ON o.tenant_id = r.tenant_id AND o.id = r.outlet_id
		JOIN sales s ON s.tenant_id = r.tenant_id AND s.id = r.sale_id
		WHERE r.tenant_id = $1 AND r.id = $2`,
		tenant, returnID).Scan(&outletID, &saleCashier, &localDay, &total)
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `
		INSERT INTO sales_daily_summary AS d
		    (tenant_id, outlet_id, day, cashier_id, return_count, return_total)
		VALUES ($1, $2, $3, $4, 1, $5)
		ON CONFLICT (tenant_id, outlet_id, day, cashier_id) DO UPDATE SET
		    return_count = d.return_count + $6::int,
		    return_total = d.return_total + $5 * $6`,
		tenant, outletID.Bytes, localDay.Time, uuid.UUID(saleCashier.Bytes), total, sign)
	return err
}

// ApplyReturn mencatat retur aktif baru ke ringkasan.
func ApplyReturn(ctx context.Context, tx pgx.Tx, tenant, returnID uuid.UUID) error {
	return adjustReturn(ctx, tx, tenant, returnID, 1)
}

// ReverseReturn membalik retur yang dibatalkan.
func ReverseReturn(ctx context.Context, tx pgx.Tx, tenant, returnID uuid.UUID) error {
	return adjustReturn(ctx, tx, tenant, returnID, -1)
}
