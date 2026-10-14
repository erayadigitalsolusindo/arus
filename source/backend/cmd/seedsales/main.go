// Command seedsales mengisi PENJUALAN DUMMY satu tenant dev untuk menguji laporan penjualan per tanggal.
// Rentang bawaan: bulan lalu, bulan ini, dan bulan depan (3 bulan kalender). Jumlah nota per hari mengikuti pola
// hari kerja/akhir pekan agar terlihat realistis.
//
//	cd source/backend && go run ./cmd/seedsales
//
// Data ditulis LANGSUNG ke tabel (bukan lewat service) agar `created_at` bisa di-backdate ke tanggal lampau/depan —
// service selalu memakai jam sekarang. Konsekuensinya stok/kartu stok TIDAK ikut berubah (laporan penjualan tidak
// membaca stok). Jalankan `cmd/seeddemo` lebih dulu bila tenant belum punya barang.
//
// Env: SEED_TENANT_CODE (kosong = satu-satunya tenant), MIGRATE_DATABASE_URL (pemilik skema; melewati RLS),
// SEED_SALES_PER_DAY (bawaan 24), SEED_SALES_SEED (benih acak, opsional), SEED_SALES_APPEND=1 (izinkan menambah
// walau sudah ada baris seed).
package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"math/rand"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/joho/godotenv"
	"github.com/shopspring/decimal"
)

type outlet struct {
	id       uuid.UUID
	code     string
	tz       string
	taxStore decimal.Decimal
	taxGov   decimal.Decimal
	loc      *time.Location
}

type item struct {
	id       uuid.UUID
	sku      string
	name     string
	unitID   uuid.UUID
	unitName string
	price    decimal.Decimal
	cost     decimal.Decimal
}

type method struct {
	id   uuid.UUID
	name string
	kind string
}

type linePlan struct {
	item     item
	position int
	qty      decimal.Decimal
	price    decimal.Decimal
	cost     decimal.Decimal
	discount decimal.Decimal
	total    decimal.Decimal
}

type paymentPlan struct {
	method method
	amount decimal.Decimal
}

type salePlan struct {
	outlet    *outlet
	cashier   uuid.UUID
	createdAt time.Time
	status    string
	voidAt    time.Time
	lines     []linePlan
	payments  []paymentPlan
	subtotal  decimal.Decimal
	discount  decimal.Decimal
	taxStore  decimal.Decimal
	taxGov    decimal.Decimal
	total     decimal.Decimal
	paid      decimal.Decimal
	change    decimal.Decimal
	docNo     string
}

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {
	_ = godotenv.Load()
	adminURL := strings.TrimSpace(os.Getenv("MIGRATE_DATABASE_URL"))
	if adminURL == "" {
		return errors.New("MIGRATE_DATABASE_URL wajib diisi (jalankan dari source/backend agar .env terbaca)")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, adminURL)
	if err != nil {
		return err
	}
	defer pool.Close()

	tid, tname, err := resolveTenant(ctx, pool)
	if err != nil {
		return err
	}
	fmt.Printf("tenant: %s (%s)\n", tname, tid)

	outlets, err := loadOutlets(ctx, pool, tid)
	if err != nil {
		return err
	}
	cashiers, err := loadCashiers(ctx, pool, tid)
	if err != nil {
		return err
	}
	items, err := loadItems(ctx, pool, tid)
	if err != nil {
		return err
	}
	methods, cash, err := loadMethods(ctx, pool, tid)
	if err != nil {
		return err
	}
	if err := ensureFresh(ctx, pool, tid); err != nil {
		return err
	}

	dayBase := envInt("SEED_SALES_PER_DAY", 24)
	seed := int64(envInt("SEED_SALES_SEED", int(time.Now().UnixNano())))
	rng := rand.New(rand.NewSource(seed))

	from, to := monthRange(time.Now(), outlets[0].loc)
	fmt.Printf("rentang: %s s/d %s (%d hari), %d outlet, %d barang, %d kasir\n",
		from.Format("2006-01-02"), to.Format("2006-01-02"), int(to.Sub(from).Hours()/24)+1, len(outlets), len(items), len(cashiers))

	counters, err := loadCounters(ctx, pool, tid)
	if err != nil {
		return err
	}
	retCounters, err := loadReturnCounters(ctx, pool, tid)
	if err != nil {
		return err
	}

	totalSales, totalLines, totalReturns := 0, 0, 0
	for day := from; !day.After(to); day = day.AddDate(0, 0, 1) {
		plans := planDay(rng, day, dayBase, outlets, items, methods, cash, cashiers, counters)
		returns := planReturns(rng, plans, cash, retCounters, time.Now())
		if err := insertDay(ctx, pool, tid, plans, returns, cashiers, cash, rng); err != nil {
			return fmt.Errorf("hari %s: %w", day.Format("2006-01-02"), err)
		}
		for _, p := range plans {
			totalSales++
			totalLines += len(p.lines)
		}
		totalReturns += len(returns)
	}
	if err := saveCounters(ctx, pool, tid, counters, retCounters); err != nil {
		return err
	}
	fmt.Printf("selesai: %d nota, %d baris, %d retur (benih acak %d)\n", totalSales, totalLines, totalReturns, seed)
	fmt.Println("catatan: stok tidak diubah (data khusus laporan penjualan).")
	return nil
}

func envInt(key string, def int) int {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return def
}

func resolveTenant(ctx context.Context, pool *pgxpool.Pool) (uuid.UUID, string, error) {
	code := strings.TrimSpace(os.Getenv("SEED_TENANT_CODE"))
	if code == "" {
		// Tanpa kode: pakai tenant milik akun demo (SEED_USER_EMAIL) — dev DB bisa punya banyak tenant dari test.
		email := strings.TrimSpace(os.Getenv("SEED_USER_EMAIL"))
		if email == "" {
			email = "aciraba.id@gmail.com"
		}
		var id uuid.UUID
		var name string
		err := pool.QueryRow(ctx, `SELECT t.id, t.name FROM tenants t
			JOIN users u ON u.tenant_id = t.id
			WHERE t.active AND lower(u.email) = lower($1)
			ORDER BY u.created_at LIMIT 1`, email).Scan(&id, &name)
		if err == nil {
			return id, name, nil
		}
	}
	rows, err := pool.Query(ctx, `SELECT id, name FROM tenants WHERE active AND ($1 = '' OR code = $1) ORDER BY created_at`, code)
	if err != nil {
		return uuid.Nil, "", err
	}
	defer rows.Close()
	type t struct {
		id   uuid.UUID
		name string
	}
	var ts []t
	for rows.Next() {
		var x t
		if err := rows.Scan(&x.id, &x.name); err != nil {
			return uuid.Nil, "", err
		}
		ts = append(ts, x)
	}
	switch {
	case len(ts) == 0:
		return uuid.Nil, "", errors.New("tenant tidak ditemukan")
	case len(ts) > 1:
		return uuid.Nil, "", fmt.Errorf("ada %d tenant; set SEED_TENANT_CODE", len(ts))
	}
	return ts[0].id, ts[0].name, nil
}

func loadOutlets(ctx context.Context, pool *pgxpool.Pool, tid uuid.UUID) ([]*outlet, error) {
	rows, err := pool.Query(ctx, `SELECT id, code, timezone, tax_store_pct, tax_gov_pct
		FROM outlets WHERE tenant_id = $1 AND active ORDER BY created_at`, tid)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*outlet
	for rows.Next() {
		o := &outlet{}
		if err := rows.Scan(&o.id, &o.code, &o.tz, &o.taxStore, &o.taxGov); err != nil {
			return nil, err
		}
		o.loc, err = time.LoadLocation(o.tz)
		if err != nil {
			o.loc = time.UTC
		}
		out = append(out, o)
	}
	if len(out) == 0 {
		return nil, errors.New("tenant belum punya outlet aktif")
	}
	return out, nil
}

func loadCashiers(ctx context.Context, pool *pgxpool.Pool, tid uuid.UUID) ([]uuid.UUID, error) {
	rows, err := pool.Query(ctx, `SELECT id FROM users WHERE tenant_id = $1 AND active ORDER BY created_at`, tid)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []uuid.UUID
	for rows.Next() {
		var id uuid.UUID
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	if len(out) == 0 {
		return nil, errors.New("tenant belum punya user aktif")
	}
	return out, nil
}

func loadItems(ctx context.Context, pool *pgxpool.Pool, tid uuid.UUID) ([]item, error) {
	rows, err := pool.Query(ctx, `SELECT i.id, i.sku, i.name, i.unit_id, u.name, i.sell_price,
			greatest(i.last_cost, i.avg_cost, (i.sell_price * 0.7)::numeric(18,2))
		FROM items i JOIN units u ON u.tenant_id = i.tenant_id AND u.id = i.unit_id
		WHERE i.tenant_id = $1 AND i.active AND i.kind = 'goods' AND i.sell_price > 0
		ORDER BY i.name`, tid)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []item
	for rows.Next() {
		var it item
		if err := rows.Scan(&it.id, &it.sku, &it.name, &it.unitID, &it.unitName, &it.price, &it.cost); err != nil {
			return nil, err
		}
		out = append(out, it)
	}
	if len(out) == 0 {
		return nil, errors.New("tenant belum punya barang berharga jual; jalankan cmd/seeddemo dulu")
	}
	return out, nil
}

func loadMethods(ctx context.Context, pool *pgxpool.Pool, tid uuid.UUID) ([]method, method, error) {
	rows, err := pool.Query(ctx, `SELECT id, name, kind FROM payment_methods
		WHERE tenant_id = $1 AND active ORDER BY is_system DESC, name`, tid)
	if err != nil {
		return nil, method{}, err
	}
	defer rows.Close()
	var all []method
	var cash method
	for rows.Next() {
		var m method
		if err := rows.Scan(&m.id, &m.name, &m.kind); err != nil {
			return nil, method{}, err
		}
		if m.kind == "cash" && cash.id == uuid.Nil {
			cash = m
		}
		if m.kind != "deposit" && m.kind != "supplier_credit" {
			all = append(all, m)
		}
	}
	if cash.id == uuid.Nil {
		return nil, method{}, errors.New("tenant belum punya metode pembayaran tunai")
	}
	if len(all) == 0 {
		all = []method{cash}
	}
	return all, cash, nil
}

// ensureFresh menolak penambahan bila sudah ada baris seed, kecuali SEED_SALES_APPEND=1.
func ensureFresh(ctx context.Context, pool *pgxpool.Pool, tid uuid.UUID) error {
	if strings.TrimSpace(os.Getenv("SEED_SALES_APPEND")) == "1" {
		return nil
	}
	var n int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM sales WHERE tenant_id = $1 AND idempotency_key LIKE 'seed-%'`, tid).Scan(&n); err != nil {
		return err
	}
	if n > 0 {
		return fmt.Errorf("sudah ada %d nota seed; hapus dulu atau set SEED_SALES_APPEND=1", n)
	}
	return nil
}

type counterKey struct {
	outlet uuid.UUID
	day    string
}

func loadCounters(ctx context.Context, pool *pgxpool.Pool, tid uuid.UUID) (map[counterKey]int64, error) {
	out := map[counterKey]int64{}
	rows, err := pool.Query(ctx, `SELECT outlet_id, day, last_no FROM sale_counters WHERE tenant_id = $1`, tid)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var k counterKey
		var day time.Time
		var n int64
		if err := rows.Scan(&k.outlet, &day, &n); err != nil {
			return nil, err
		}
		k.day = day.Format("2006-01-02")
		out[k] = n
	}
	return out, nil
}

func loadReturnCounters(ctx context.Context, pool *pgxpool.Pool, tid uuid.UUID) (map[counterKey]int64, error) {
	out := map[counterKey]int64{}
	rows, err := pool.Query(ctx, `SELECT outlet_id, day, last_no FROM sales_return_counters WHERE tenant_id = $1`, tid)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var k counterKey
		var day time.Time
		var n int64
		if err := rows.Scan(&k.outlet, &day, &n); err != nil {
			return nil, err
		}
		k.day = day.Format("2006-01-02")
		out[k] = n
	}
	return out, nil
}

// monthRange mengembalikan hari pertama bulan LALU sampai hari terakhir bulan DEPAN menurut zona outlet.
func monthRange(now time.Time, loc *time.Location) (time.Time, time.Time) {
	n := now.In(loc)
	from := time.Date(n.Year(), n.Month(), 1, 0, 0, 0, 0, loc).AddDate(0, -1, 0)
	to := time.Date(n.Year(), n.Month(), 1, 0, 0, 0, 0, loc).AddDate(0, 2, 0).AddDate(0, 0, -1)
	return from, to
}

func planDay(rng *rand.Rand, day time.Time, base int, outlets []*outlet, items []item, methods []method, cash method, cashiers []uuid.UUID, counters map[counterKey]int64) []salePlan {
	n := base
	switch day.Weekday() {
	case time.Saturday:
		n = base * 17 / 10
	case time.Sunday:
		n = base * 15 / 10
	}
	n = n * (60 + rng.Intn(80)) / 100 // 60%..140%
	if n < 1 {
		n = 1
	}
	plans := make([]salePlan, 0, n)
	for i := 0; i < n; i++ {
		o := outlets[rng.Intn(len(outlets))]
		created := randomTime(rng, day, o.loc, 8, 21)
		p := buildSale(rng, o, created, items, methods, cash, cashiers)
		key := counterKey{outlet: o.id, day: created.In(o.loc).Format("2006-01-02")}
		counters[key]++
		p.docNo = fmt.Sprintf("%s-%s-%04d", strings.ToUpper(o.code), created.In(o.loc).Format("060102"), counters[key])
		plans = append(plans, p)
	}
	return plans
}

func randomTime(rng *rand.Rand, day time.Time, loc *time.Location, hFrom, hTo int) time.Time {
	h := hFrom + rng.Intn(hTo-hFrom)
	m := rng.Intn(60)
	s := rng.Intn(60)
	return time.Date(day.Year(), day.Month(), day.Day(), h, m, s, 0, loc)
}

func buildSale(rng *rand.Rand, o *outlet, created time.Time, items []item, methods []method, cash method, cashiers []uuid.UUID) salePlan {
	p := salePlan{outlet: o, createdAt: created, cashier: cashiers[rng.Intn(len(cashiers))], status: "completed"}

	nLines := 1 + rng.Intn(6)
	used := map[uuid.UUID]bool{}
	for i := 0; i < nLines; i++ {
		it := items[rng.Intn(len(items))]
		if used[it.id] && len(items) > 4 {
			i--
			continue
		}
		used[it.id] = true
		qty := decimal.NewFromInt(int64(1 + rng.Intn(3)))
		if rng.Intn(10) == 0 {
			qty = decimal.NewFromInt(int64([]int{6, 12, 24}[rng.Intn(3)]))
		}
		total := qty.Mul(it.price).Round(2)
		p.lines = append(p.lines, linePlan{item: it, position: i + 1, qty: qty, price: it.price, cost: it.cost,
			discount: decimal.Zero, total: total})
		p.subtotal = p.subtotal.Add(total)
	}
	p.subtotal = p.subtotal.Round(2)
	if rng.Intn(8) == 0 { // potongan global sesekali (bulat)
		d := decimal.NewFromInt(int64([]int{500, 1000, 2000, 5000}[rng.Intn(4)]))
		if d.LessThan(p.subtotal) {
			p.discount = d
		}
	}
	baseAmt := p.subtotal.Sub(p.discount)
	p.taxStore = baseAmt.Mul(o.taxStore).Div(decimal.NewFromInt(100)).Round(2)
	p.taxGov = baseAmt.Mul(o.taxGov).Div(decimal.NewFromInt(100)).Round(2)
	p.total = baseAmt.Add(p.taxStore).Add(p.taxGov).Round(2)

	switch r := rng.Intn(100); {
	case r < 65: // tunai (kembalian dibulatkan ke atas)
		paid := roundUpTo(p.total, []decimal.Decimal{
			decimal.NewFromInt(5000), decimal.NewFromInt(10000), decimal.NewFromInt(20000), decimal.NewFromInt(50000),
		}[rng.Intn(4)])
		p.payments = []paymentPlan{{method: cash, amount: paid}}
		p.paid, p.change = paid, paid.Sub(p.total)
	case r < 90:
		m := nonCash(rng, methods, cash)
		p.payments = []paymentPlan{{method: m, amount: p.total}}
		p.paid = p.total
	default: // bayar campuran: sebagian non-tunai, sisanya tunai
		m := nonCash(rng, methods, cash)
		nonCashAmt := p.total.Mul(decimal.NewFromInt(int64(30 + rng.Intn(30)))).Div(decimal.NewFromInt(100)).Round(2)
		if nonCashAmt.IsZero() {
			nonCashAmt = decimal.NewFromInt(1)
		}
		if nonCashAmt.GreaterThanOrEqual(p.total) {
			nonCashAmt = p.total.Sub(decimal.NewFromInt(1))
		}
		rest := p.total.Sub(nonCashAmt)
		cashAmt := roundUpTo(rest, decimal.NewFromInt(5000))
		p.payments = []paymentPlan{{method: m, amount: nonCashAmt}, {method: cash, amount: cashAmt}}
		p.paid = nonCashAmt.Add(cashAmt)
		p.change = p.paid.Sub(p.total)
	}
	if rng.Intn(50) == 0 { // ~2% nota dibatalkan
		p.status = "void"
		p.voidAt = created.Add(time.Duration(5+rng.Intn(115)) * time.Minute)
	}
	return p
}

func nonCash(rng *rand.Rand, methods []method, cash method) method {
	var pool []method
	for _, m := range methods {
		if m.kind != "cash" {
			pool = append(pool, m)
		}
	}
	if len(pool) == 0 {
		return cash
	}
	return pool[rng.Intn(len(pool))]
}

func roundUpTo(v, step decimal.Decimal) decimal.Decimal {
	if step.IsZero() {
		return v
	}
	return v.Div(step).Ceil().Mul(step).Round(2)
}

type returnPlan struct {
	sale       *salePlan
	line       linePlan
	returnedAt time.Time
	returnDay  time.Time
	docNo      string
	refund     decimal.Decimal
	tax        decimal.Decimal
	value      decimal.Decimal
}

func planReturns(rng *rand.Rand, plans []salePlan, cash method, counters map[counterKey]int64, now time.Time) []returnPlan {
	var out []returnPlan
	for i := range plans {
		p := &plans[i]
		if p.status != "completed" || p.createdAt.After(now.AddDate(0, 0, -2)) {
			continue
		}
		if rng.Intn(50) != 0 { // ~2% nota punya retur
			continue
		}
		l := p.lines[rng.Intn(len(p.lines))]
		at := p.createdAt.AddDate(0, 0, 1+rng.Intn(4))
		if at.After(now) {
			at = now.Add(-time.Hour)
		}
		value := l.total
		tax := l.total.Mul(p.outlet.taxStore.Add(p.outlet.taxGov)).Div(decimal.NewFromInt(100)).Round(2)
		key := counterKey{outlet: p.outlet.id, day: at.In(p.outlet.loc).Format("2006-01-02")}
		counters[key]++
		out = append(out, returnPlan{
			sale: p, line: l, returnedAt: at, returnDay: at,
			docNo: fmt.Sprintf("RJ-%s-%s-%04d", strings.ToUpper(p.outlet.code), at.In(p.outlet.loc).Format("060102"), counters[key]),
			value: value, tax: tax, refund: value.Add(tax).Round(2),
		})
	}
	return out
}

func insertDay(ctx context.Context, pool *pgxpool.Pool, tid uuid.UUID, plans []salePlan, returns []returnPlan, cashiers []uuid.UUID, cash method, rng *rand.Rand) error {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	saleIDs := map[*salePlan]uuid.UUID{}
	for i := range plans {
		p := &plans[i]
		var id uuid.UUID
		key := "seed-" + uuid.NewString()
		note := ""
		var voidReason, voidedAt, voidedBy any
		if p.status == "void" {
			voidReason = []string{"salah input", "pelanggan batal", "salah bayar"}[rng.Intn(3)]
			voidedAt = p.voidAt
			voidedBy = p.cashier
		}
		err := tx.QueryRow(ctx, `INSERT INTO sales (tenant_id, outlet_id, doc_no, idempotency_key, request_hash,
				cashier_id, status, note, subtotal, discount, tax_store_pct, tax_gov_pct, tax_store, tax_gov, other_cost,
				total, paid, change, created_at, void_reason, voided_at, voided_by)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,0,$15,$16,$17,$18,$19,$20,$21) RETURNING id`,
			tid, p.outlet.id, p.docNo, key, "seed:"+key, p.cashier, p.status, note,
			p.subtotal, p.discount, p.outlet.taxStore, p.outlet.taxGov, p.taxStore, p.taxGov,
			p.total, p.paid, p.change, p.createdAt, voidReason, voidedAt, voidedBy).Scan(&id)
		if err != nil {
			return err
		}
		saleIDs[p] = id
		for _, l := range p.lines {
			if _, err := tx.Exec(ctx, `INSERT INTO sale_lines (tenant_id, sale_id, position, item_id, sku, name, unit_id, unit_name,
					factor, qty, unit_price, unit_cost, discount, line_total, list_price, note)
				VALUES ($1,$2,$3,$4,$5,$6,$7,$8,1,$9,$10,$11,$12,$13,$10,'')`,
				tid, id, l.position, l.item.id, l.item.sku, l.item.name, l.item.unitID, l.item.unitName,
				l.qty, l.price, l.cost, l.discount, l.total); err != nil {
				return err
			}
		}
		for pos, pay := range p.payments {
			if _, err := tx.Exec(ctx, `INSERT INTO sale_payments (tenant_id, sale_id, position, method, amount, ref_no,
					method_id, method_name, fee_amount, fee_bearer)
				VALUES ($1,$2,$3,$4,$5,'',$6,$7,0,'store')`,
				tid, id, pos+1, pay.method.kind, pay.amount, pay.method.id, pay.method.name); err != nil {
				return err
			}
		}
	}

	for i := range returns {
		r := &returns[i]
		saleID, ok := saleIDs[r.sale]
		if !ok {
			continue
		}
		var rid uuid.UUID
		key := "seed-" + uuid.NewString()
		createdBy := cashiers[rng.Intn(len(cashiers))]
		err := tx.QueryRow(ctx, `INSERT INTO sales_returns (tenant_id, outlet_id, sale_id, doc_no, idempotency_key, request_hash,
				return_date, note, subtotal, discount, tax_amount, surcharge, total, receivable_cut, refund,
				refund_method, refund_method_id, refund_method_name, created_by, created_at)
			VALUES ($1,$2,$3,$4,$5,$6,$7,'',$8,0,$9,0,$10,0,$10,$11,$12,$13,$14,$15) RETURNING id`,
			tid, r.sale.outlet.id, saleID, r.docNo, key, "seed:"+key, r.returnDay, r.value, r.tax, r.refund,
			cash.kind, cash.id, cash.name, createdBy, r.returnedAt).Scan(&rid)
		if err != nil {
			return err
		}
		l := r.line
		if _, err := tx.Exec(ctx, `INSERT INTO sales_return_lines (tenant_id, return_id, position, sale_position, item_id,
				item_sku, item_name, unit_name, factor, qty, base_qty, value, discount, tax_amount, unit_cost)
			VALUES ($1,$2,1,$3,$4,$5,$6,$7,1,$8,$8,$9,0,$10,$11)`,
			tid, rid, l.position, l.item.id, l.item.sku, l.item.name, l.item.unitName,
			l.qty, r.value, r.tax, l.cost); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

func saveCounters(ctx context.Context, pool *pgxpool.Pool, tid uuid.UUID, counters, retCounters map[counterKey]int64) error {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	for k, n := range counters {
		if _, err := tx.Exec(ctx, `INSERT INTO sale_counters (tenant_id, outlet_id, day, last_no)
			VALUES ($1,$2,$3,$4) ON CONFLICT (tenant_id, outlet_id, day) DO UPDATE SET last_no = GREATEST(sale_counters.last_no, EXCLUDED.last_no)`,
			tid, k.outlet, k.day, n); err != nil {
			return err
		}
	}
	for k, n := range retCounters {
		if _, err := tx.Exec(ctx, `INSERT INTO sales_return_counters (tenant_id, outlet_id, day, last_no)
			VALUES ($1,$2,$3,$4) ON CONFLICT (tenant_id, outlet_id, day) DO UPDATE SET last_no = GREATEST(sales_return_counters.last_no, EXCLUDED.last_no)`,
			tid, k.outlet, k.day, n); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}
