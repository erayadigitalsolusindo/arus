package sales

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
	"github.com/shopspring/decimal"

	"aciraba/internal/approval"
	"aciraba/internal/authz"
	pauth "aciraba/internal/platform/auth"
	"aciraba/internal/platform/db"
	"aciraba/internal/stock"
)

type env struct {
	app, admin *pgxpool.Pool
	svc        *Service
	tenant     uuid.UUID
	other      uuid.UUID
	outlet     uuid.UUID
	user       uuid.UUID
	unit       uuid.UUID
}

func (e *env) actor(tenant uuid.UUID) authz.Actor {
	return authz.Actor{TenantID: tenant, UserID: e.user, OutletID: e.outlet, Name: "Kasir Uji"}
}

func newEnv(t *testing.T) *env {
	t.Helper()
	appURL, adminURL := os.Getenv("TEST_DATABASE_URL"), os.Getenv("TEST_ADMIN_DATABASE_URL")
	if appURL == "" || adminURL == "" {
		t.Skip("TEST_DATABASE_URL/TEST_ADMIN_DATABASE_URL tidak di-set")
	}
	ctx := context.Background()
	app, err := pgxpool.New(ctx, appURL)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(app.Close)
	admin, err := pgxpool.New(ctx, adminURL)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(admin.Close)

	e := &env{app: app, admin: admin, svc: NewService(app, approval.NewService(app, nil, "uji-rahasia-uji-rahasia-uji-rahasia")), tenant: uuid.New(), other: uuid.New(), outlet: uuid.New(), user: uuid.New(), unit: uuid.New()}
	role := uuid.New()
	for _, s := range []struct {
		sql  string
		args []any
	}{
		{`INSERT INTO tenants (id, code, name) VALUES ($1, $2, 'UJI-JUAL')`, []any{e.tenant, "sj-" + e.tenant.String()[:8]}},
		{`INSERT INTO tenants (id, code, name) VALUES ($1, $2, 'UJI-JUAL-2')`, []any{e.other, "sj-" + e.other.String()[:8]}},
		{`INSERT INTO outlets (id, tenant_id, code, name, tax_store_pct, tax_gov_pct) VALUES ($1, $2, 'main', 'Pusat', 10, 1)`, []any{e.outlet, e.tenant}},
		{`INSERT INTO roles (id, tenant_id, name) VALUES ($1, $2, 'Kasir')`, []any{role, e.tenant}},
		{`INSERT INTO users (id, tenant_id, role_id, email, name, password_hash) VALUES ($1, $2, $3, $4, 'Kasir Uji', 'x')`, []any{e.user, e.tenant, role, "kasir-" + e.user.String() + "@example.test"}},
		{`INSERT INTO units (id, tenant_id, name) VALUES ($1, $2, 'Pcs')`, []any{e.unit, e.tenant}},
		{defaultMethodsSQL, []any{e.tenant}},
		{defaultMethodsSQL, []any{e.other}},
	} {
		if _, err := admin.Exec(ctx, s.sql, s.args...); err != nil {
			t.Fatal(err)
		}
	}
	t.Cleanup(func() {
		for _, tid := range []uuid.UUID{e.tenant, e.other} {
			for _, tbl := range []string{"audit_log", "receivable_payments", "receivable_payment_counters", "receivables", "sale_payments", "payment_methods", "sale_lines", "sale_vouchers", "sale_costs", "sales", "vouchers", "sale_counters", "salespeople", "member_point_movements", "members", "member_counters", "member_levels", "stock_movements", "stock_balances", "item_outlet_costs",
				"items", "units", "users", "roles", "outlets"} {
				if tbl == "sales" {
					// Rantai revisi saling merujuk (RESTRICT): putus tautan nota lama, lalu hapus revisi dari yang terbaru.
					_, _ = admin.Exec(ctx, `UPDATE sales SET status = 'completed', superseded_by = NULL WHERE tenant_id = $1 AND superseded_by IS NOT NULL`, tid)
					for rev := 12; rev >= 2; rev-- {
						_, _ = admin.Exec(ctx, `DELETE FROM sales WHERE tenant_id = $1 AND revision = $2`, tid, rev)
					}
				}
				_, _ = admin.Exec(ctx, `DELETE FROM `+tbl+` WHERE tenant_id = $1`, tid)
			}
			_, _ = admin.Exec(ctx, `DELETE FROM tenants WHERE id = $1`, tid)
		}
	})
	return e
}

// defaultMethodsSQL = metode bawaan seperti yang diisi paymentmethod.SeedDefaults saat register.
const defaultMethodsSQL = `INSERT INTO payment_methods (tenant_id, name, kind, is_system)
	VALUES ($1, 'Tunai', 'cash', true), ($1, 'Transfer', 'transfer', false), ($1, 'Debit', 'debit', false),
	       ($1, 'Kartu Kredit', 'credit_card', false), ($1, 'E-Wallet', 'ewallet', false)`

// item membuat barang berharga `price`, HPP `cost`, dan stok display `qty`.
func (e *env) item(t *testing.T, kind, price, cost string, qty int, allowNeg bool) uuid.UUID {
	t.Helper()
	ctx := context.Background()
	id := uuid.New()
	if _, err := e.admin.Exec(ctx, `INSERT INTO items (id, tenant_id, sku, name, unit_id, kind, sell_price, avg_cost, last_cost, allow_negative_stock)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $8, $9)`, id, e.tenant, "S-"+id.String()[:8], "Barang "+id.String()[:4], e.unit, kind, price, cost, allowNeg); err != nil {
		t.Fatal(err)
	}
	if qty > 0 {
		err := db.WithTenant(ctx, e.app, e.tenant, func(tx pgx.Tx) error {
			_, err := stock.Apply(ctx, tx, stock.Movement{TenantID: e.tenant, OutletID: e.outlet, ItemID: id, Bucket: stock.BucketDisplay,
				Delta: decimal.NewFromInt(int64(qty)), RefType: stock.RefOpening})
			return err
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	return id
}

func (e *env) exec(t *testing.T, sql string, args ...any) {
	t.Helper()
	if _, err := e.admin.Exec(context.Background(), sql, args...); err != nil {
		t.Fatal(err)
	}
}

func (e *env) stockOf(t *testing.T, item uuid.UUID) string {
	t.Helper()
	var q decimal.Decimal
	err := e.admin.QueryRow(context.Background(), `SELECT coalesce(sum(qty),0) FROM stock_balances WHERE tenant_id=$1 AND item_id=$2`, e.tenant, item).Scan(&q)
	if err != nil {
		t.Fatal(err)
	}
	return q.String()
}

func (e *env) count(t *testing.T, table string) int {
	t.Helper()
	var n int
	if err := e.admin.QueryRow(context.Background(), `SELECT count(*) FROM `+table+` WHERE tenant_id=$1`, e.tenant).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func line(item uuid.UUID, qty string) LineIn { return LineIn{ItemID: item, Qty: json.Number(qty)} }
func pay(method, amount string) PaymentIn {
	return PaymentIn{Method: method, Amount: json.Number(amount)}
}
func key() string { return "k-" + uuid.NewString() }

func fieldErr(t *testing.T, err error, field, code string) {
	t.Helper()
	var f FieldErrors
	if !errors.As(err, &f) || f[field] != code {
		t.Fatalf("galat %q ingin %s=%s, dapat %v", err, field, code, f)
	}
}

// Contoh dari catatan produk (grosir 2→9.500, 6→8.500, 11→8.300) + potongan baris/global, pajak, biaya lain, kembalian.
func TestCreateComputesTotalsOnServer(t *testing.T) {
	e := newEnv(t)
	a := e.item(t, "goods", "10000", "5000", 100, false)
	b := e.item(t, "goods", "2500.50", "1000", 100, false)
	e.exec(t, `INSERT INTO item_wholesale_tiers (tenant_id, item_id, min_qty, price) VALUES ($1,$2,2,9500),($1,$2,6,8500),($1,$2,11,8300)`, e.tenant, a)

	// A × 7 → grosir 8.500 → 59.500, potongan baris 500 → 59.000. B × 2 → 5.001,00. Subtotal 64.001.
	// Potongan global 1 → dasar 64.000. Pajak toko 10% = 6.400; negara 1% = 640. Biaya lain 360 → total 71.400.
	in := Request{
		Lines:     []LineIn{{ItemID: a, Qty: "7", Discount: "500"}, line(b, "2")},
		Approval:  e.approverIn(t),
		Discount:  "1",
		OtherCost: "360",
		ApplyTax:  true,
		Payments:  []PaymentIn{pay("debit", "20000"), pay("cash", "60000")},
	}
	s, replayed, err := e.svc.Create(context.Background(), e.actor(e.tenant), key(), in)
	if err != nil || replayed {
		t.Fatalf("Create: %v replayed=%v", err, replayed)
	}
	want := map[string]string{"subtotal": "64001.00", "discount": "1.00", "tax_store": "6400.00", "tax_gov": "640.00", "other": "360.00", "total": "71400.00", "paid": "80000.00", "change": "8600.00"}
	got := map[string]string{"subtotal": s.Subtotal, "discount": s.Discount, "tax_store": s.TaxStore, "tax_gov": s.TaxGov, "other": s.OtherCost, "total": s.Total, "paid": s.Paid, "change": s.Change}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("%s = %s, ingin %s", k, got[k], v)
		}
	}
	if s.Lines[0].UnitPrice != "8500.00" || s.Lines[0].LineTotal != "59000.00" {
		t.Errorf("baris A: %+v", s.Lines[0])
	}
	if !strings.HasPrefix(s.DocNo, "MAIN-") || !strings.HasSuffix(s.DocNo, "-0001") {
		t.Errorf("nomor nota %q", s.DocNo)
	}
	if e.stockOf(t, a) != "93" || e.stockOf(t, b) != "98" {
		t.Errorf("stok A=%s B=%s", e.stockOf(t, a), e.stockOf(t, b))
	}
	var n int
	_ = e.admin.QueryRow(context.Background(), "SELECT count(*) FROM audit_log WHERE tenant_id=$1 AND action='sale.create'", e.tenant).Scan(&n)
	if n != 1 {
		t.Errorf("audit = %d", n)
	}
	// Nomor berikutnya naik.
	s2, _, err := e.svc.Create(context.Background(), e.actor(e.tenant), key(), Request{Lines: []LineIn{line(b, "1")}, Payments: []PaymentIn{pay("cash", "3000")}})
	if err != nil || !strings.HasSuffix(s2.DocNo, "-0002") {
		t.Fatalf("nota kedua: %v %q", err, s2.DocNo)
	}
	// Tanpa apply_tax: tidak ada pajak.
	if s2.TaxStore != "0.00" || s2.Total != "2500.50" || s2.Change != "499.50" {
		t.Errorf("nota kedua: %+v", s2)
	}
}

func TestPaymentRules(t *testing.T) {
	e := newEnv(t)
	a := e.item(t, "goods", "1000", "0", 50, false)
	ctx := context.Background()
	_, _, err := e.svc.Create(ctx, e.actor(e.tenant), key(), Request{Lines: []LineIn{line(a, "2")}, Payments: []PaymentIn{pay("cash", "1999")}})
	fieldErr(t, err, "payments", codePaymentShort)
	_, _, err = e.svc.Create(ctx, e.actor(e.tenant), key(), Request{Lines: []LineIn{line(a, "2")}})
	fieldErr(t, err, "payments", codePaymentShort)
	// Kartu tidak boleh melebihi total (tidak ada kembalian kartu), meski ada tunai.
	_, _, err = e.svc.Create(ctx, e.actor(e.tenant), key(), Request{Lines: []LineIn{line(a, "2")}, Payments: []PaymentIn{pay("debit", "2500"), pay("cash", "1000")}})
	fieldErr(t, err, "payments", codeNonCashOver)
	_, _, err = e.svc.Create(ctx, e.actor(e.tenant), key(), Request{Lines: []LineIn{line(a, "2")}, Payments: []PaymentIn{pay("barter", "2000")}})
	fieldErr(t, err, "payments.0.method", "INVALID")
	if e.count(t, "sales") != 0 || e.stockOf(t, a) != "50" {
		t.Fatal("penolakan tidak boleh meninggalkan nota atau mengubah stok")
	}
}

func TestValidationAndPricingGuards(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	a := e.item(t, "goods", "1000", "800", 10, false)
	cases := []struct {
		name, field, code string
		in                Request
	}{
		{"tanpa baris", "lines", "REQUIRED", Request{}},
		{"qty nol", "lines.0.qty", "INVALID", Request{Lines: []LineIn{line(a, "0")}}},
		{"qty 4 desimal", "lines.0.qty", "INVALID", Request{Lines: []LineIn{line(a, "1.2345")}}},
		{"qty negatif", "lines.0.qty", "INVALID", Request{Lines: []LineIn{line(a, "-1")}}},
		{"barang tak ada", "lines.0.item_id", codeItemMissing, Request{Lines: []LineIn{line(uuid.New(), "1")}}},
		{"potongan baris > baris", "lines.0.discount", codeDiscountOver, Request{Lines: []LineIn{{ItemID: a, Qty: "1", Discount: "1001"}}}},
		{"potongan global > subtotal", "discount", codeDiscountOver, Request{Lines: []LineIn{line(a, "1")}, Discount: "1001"}},
		{"jual di bawah HPP", "lines.0.item_id", codeBelowCost, Request{Lines: []LineIn{{ItemID: a, Qty: "1", Discount: "300"}}}},
		{"satuan asing", "lines.0.unit_id", codeUnitUnknown, Request{Lines: []LineIn{{ItemID: a, UnitID: ptr(uuid.New()), Qty: "1"}}}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, _, err := e.svc.Create(ctx, e.actor(e.tenant), key(), c.in)
			fieldErr(t, err, c.field, c.code)
		})
	}
	// Barang nonaktif.
	e.exec(t, `UPDATE items SET active=false WHERE id=$1`, a)
	_, _, err := e.svc.Create(ctx, e.actor(e.tenant), key(), Request{Lines: []LineIn{line(a, "1")}, Payments: []PaymentIn{pay("cash", "1000")}})
	fieldErr(t, err, "lines.0.item_id", codeItemMissing)
	// Kunci idempotensi wajib & berformat.
	_, _, err = e.svc.Create(ctx, e.actor(e.tenant), "pendek", Request{})
	if !errors.Is(err, ErrKeyRequired) {
		t.Fatalf("kunci pendek: %v", err)
	}
	// Barang tenant lain tidak terlihat.
	_, _, err = e.svc.Create(ctx, e.actor(e.other), key(), Request{Lines: []LineIn{line(a, "1")}, Payments: []PaymentIn{pay("cash", "1000")}})
	if err == nil {
		t.Fatal("tenant lain tidak boleh menjual barang ini")
	}
}

func ptr[T any](v T) *T { return &v }

func TestStockInsufficientRollsBackWholeSale(t *testing.T) {
	e := newEnv(t)
	a := e.item(t, "goods", "1000", "0", 10, false)
	b := e.item(t, "goods", "1000", "0", 1, false)
	_, _, err := e.svc.Create(context.Background(), e.actor(e.tenant), key(), Request{
		Lines: []LineIn{line(a, "3"), line(b, "2")}, Payments: []PaymentIn{pay("cash", "5000")}})
	var se *StockError
	if !errors.As(err, &se) || se.ItemID != b {
		t.Fatalf("ingin StockError untuk B, dapat %v", err)
	}
	if e.stockOf(t, a) != "10" || e.stockOf(t, b) != "1" || e.count(t, "sales") != 0 || e.count(t, "sale_lines") != 0 {
		t.Fatal("nota harus batal seluruhnya (stok A tidak boleh berkurang)")
	}
	// Barang yang sama di dua baris dihitung total qty-nya terhadap stok.
	_, _, err = e.svc.Create(context.Background(), e.actor(e.tenant), key(), Request{
		Lines: []LineIn{line(b, "1"), line(b, "1")}, Payments: []PaymentIn{pay("cash", "2000")}})
	if !errors.As(err, &se) {
		t.Fatalf("dua baris barang sama harus melewati stok: %v", err)
	}
}

func TestServiceItemsDoNotTouchStock(t *testing.T) {
	e := newEnv(t)
	svc := e.item(t, "service", "75000", "0", 0, false)
	s, _, err := e.svc.Create(context.Background(), e.actor(e.tenant), key(), Request{Lines: []LineIn{line(svc, "1")}, Payments: []PaymentIn{pay("transfer", "75000")}})
	if err != nil || s.Total != "75000.00" {
		t.Fatalf("jasa: %v %+v", err, s)
	}
	if e.count(t, "stock_movements") != 0 {
		t.Fatal("jasa tidak boleh membuat movement")
	}
}

func TestOutletPriceAndOutletTiersReplaceDefault(t *testing.T) {
	e := newEnv(t)
	a := e.item(t, "goods", "10000", "0", 100, false)
	e.exec(t, `INSERT INTO item_wholesale_tiers (tenant_id, item_id, outlet_id, min_qty, price) VALUES ($1,$2,NULL,2,9000)`, e.tenant, a)
	e.exec(t, `INSERT INTO item_outlet_prices (tenant_id, item_id, outlet_id, sell_price) VALUES ($1,$2,$3,9800)`, e.tenant, a, e.outlet)
	// Tanpa set tier cabang: tier default berlaku (qty 2 → 9.000); qty 1 → harga cabang 9.800.
	s, _, err := e.svc.Create(context.Background(), e.actor(e.tenant), key(), Request{Lines: []LineIn{line(a, "1")}, Payments: []PaymentIn{pay("cash", "9800")}})
	if err != nil || s.Lines[0].UnitPrice != "9800.00" {
		t.Fatalf("harga cabang: %v %+v", err, s.Lines)
	}
	// Set tier cabang menggantikan SELURUH set default.
	e.exec(t, `INSERT INTO item_wholesale_tiers (tenant_id, item_id, outlet_id, min_qty, price) VALUES ($1,$2,$3,5,7000)`, e.tenant, a, e.outlet)
	s, _, err = e.svc.Create(context.Background(), e.actor(e.tenant), key(), Request{Lines: []LineIn{line(a, "2")}, Payments: []PaymentIn{pay("cash", "19600")}})
	if err != nil || s.Lines[0].UnitPrice != "9800.00" { // tier default 2→9000 tidak lagi berlaku
		t.Fatalf("tier cabang menggantikan default: %v %+v", err, s.Lines)
	}
	s, _, err = e.svc.Create(context.Background(), e.actor(e.tenant), key(), Request{Lines: []LineIn{line(a, "5")}, Payments: []PaymentIn{pay("cash", "35000")}})
	if err != nil || s.Lines[0].UnitPrice != "7000.00" {
		t.Fatalf("tier cabang: %v %+v", err, s.Lines)
	}
}

func TestAlternateUnits(t *testing.T) {
	e := newEnv(t)
	a := e.item(t, "goods", "1000", "600", 100, false)
	dus, pak := uuid.New(), uuid.New()
	e.exec(t, `INSERT INTO units (id, tenant_id, name) VALUES ($1,$3,'Dus'),($2,$3,'Pak')`, dus, pak, e.tenant)
	// Dus: faktor 12, harga manual 11.000. Pak: faktor 6, tanpa harga manual (6 × harga dasar/grosir).
	e.exec(t, `INSERT INTO item_units (tenant_id, item_id, unit_id, factor, sell_price, position) VALUES ($1,$2,$3,12,11000,1),($1,$2,$4,6,NULL,2)`, e.tenant, a, dus, pak)
	e.exec(t, `INSERT INTO item_wholesale_tiers (tenant_id, item_id, min_qty, price) VALUES ($1,$2,10,900)`, e.tenant, a)

	// 1 Dus (12 dasar) → harga manual 11.000, stok −12, HPP snapshot 12 × 600.
	s, _, err := e.svc.Create(context.Background(), e.actor(e.tenant), key(), Request{Lines: []LineIn{{ItemID: a, UnitID: &dus, Qty: "1"}}, Payments: []PaymentIn{pay("cash", "11000")}})
	if err != nil || s.Lines[0].UnitPrice != "11000.00" || s.Lines[0].Unit != "Dus" || s.Lines[0].Factor != "12" || e.stockOf(t, a) != "88" {
		t.Fatalf("dus: %v %+v stok=%s", err, s.Lines, e.stockOf(t, a))
	}
	// 1 Pak (6 dasar) + 5 Pcs = 11 dasar total → tier 10 berlaku untuk keduanya: Pak 6×900 = 5.400; Pcs 5×900 = 4.500.
	s, _, err = e.svc.Create(context.Background(), e.actor(e.tenant), key(), Request{
		Lines: []LineIn{{ItemID: a, UnitID: &pak, Qty: "1"}, line(a, "5")}, Payments: []PaymentIn{pay("cash", "9900")}})
	if err != nil || s.Lines[0].UnitPrice != "5400.00" || s.Lines[1].UnitPrice != "900.00" || s.Total != "9900.00" || e.stockOf(t, a) != "77" {
		t.Fatalf("pak+pcs: %v %+v stok=%s", err, s.Lines, e.stockOf(t, a))
	}
	// Qty satuan tambahan yang menghasilkan qty dasar > 3 desimal ditolak (faktor 6 × 0,0001 tidak valid → qty 4 desimal ditolak lebih dulu).
	_, _, err = e.svc.Create(context.Background(), e.actor(e.tenant), key(), Request{Lines: []LineIn{{ItemID: a, UnitID: &pak, Qty: "0.0001"}}, Payments: []PaymentIn{pay("cash", "1")}})
	fieldErr(t, err, "lines.0.qty", "INVALID")
}

func TestIdempotency(t *testing.T) {
	e := newEnv(t)
	a := e.item(t, "goods", "1000", "0", 50, false)
	ctx := context.Background()
	in := Request{Lines: []LineIn{line(a, "2")}, Payments: []PaymentIn{pay("cash", "2000")}}
	k := key()
	s1, r1, err := e.svc.Create(ctx, e.actor(e.tenant), k, in)
	if err != nil || r1 {
		t.Fatal(err, r1)
	}
	s2, r2, err := e.svc.Create(ctx, e.actor(e.tenant), k, in)
	if err != nil || !r2 || s2.ID != s1.ID || s2.DocNo != s1.DocNo {
		t.Fatalf("kirim ulang harus mengembalikan nota yang sama: %v replay=%v", err, r2)
	}
	if e.count(t, "sales") != 1 || e.stockOf(t, a) != "48" {
		t.Fatal("kirim ulang tidak boleh menggandakan nota/stok")
	}
	// Kunci sama dengan isi berbeda ditolak.
	other := in
	other.Lines = []LineIn{line(a, "3")}
	other.Payments = []PaymentIn{pay("cash", "3000")}
	if _, _, err = e.svc.Create(ctx, e.actor(e.tenant), k, other); !errors.Is(err, ErrKeyMismatch) {
		t.Fatalf("ingin ErrKeyMismatch, dapat %v", err)
	}
	// Nota bernomor berurutan tanpa celah setelah replay.
	s3, _, err := e.svc.Create(ctx, e.actor(e.tenant), key(), in)
	if err != nil || !strings.HasSuffix(s3.DocNo, "-0002") {
		t.Fatalf("nomor setelah replay: %v %s", err, s3.DocNo)
	}
}

// Dua kasir / klik ganda bersamaan.
func TestConcurrentSameKeyCreatesOneSale(t *testing.T) {
	e := newEnv(t)
	a := e.item(t, "goods", "1000", "0", 50, false)
	in := Request{Lines: []LineIn{line(a, "1")}, Payments: []PaymentIn{pay("cash", "1000")}}
	k := key()
	var wg sync.WaitGroup
	ids := make(chan uuid.UUID, 8)
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			s, _, err := e.svc.Create(context.Background(), e.actor(e.tenant), k, in)
			if err != nil {
				t.Error(err)
				return
			}
			ids <- s.ID
		}()
	}
	wg.Wait()
	close(ids)
	seen := map[uuid.UUID]bool{}
	for id := range ids {
		seen[id] = true
	}
	if len(seen) != 1 || e.count(t, "sales") != 1 || e.stockOf(t, a) != "49" {
		t.Fatalf("nota=%d db=%d stok=%s", len(seen), e.count(t, "sales"), e.stockOf(t, a))
	}
}

func TestConcurrentSalesNeverOversell(t *testing.T) {
	e := newEnv(t)
	a := e.item(t, "goods", "1000", "0", 10, false)
	var ok, fail atomic.Int32
	var wg sync.WaitGroup
	for range 25 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _, err := e.svc.Create(context.Background(), e.actor(e.tenant), key(), Request{Lines: []LineIn{line(a, "1")}, Payments: []PaymentIn{pay("cash", "1000")}})
			var se *StockError
			switch {
			case err == nil:
				ok.Add(1)
			case errors.As(err, &se):
				fail.Add(1)
			default:
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	if ok.Load() != 10 || fail.Load() != 15 || e.stockOf(t, a) != "0" || e.count(t, "sales") != 10 {
		t.Fatalf("ok=%d gagal=%d stok=%s nota=%d", ok.Load(), fail.Load(), e.stockOf(t, a), e.count(t, "sales"))
	}
	var distinct int
	_ = e.admin.QueryRow(context.Background(), `SELECT count(DISTINCT doc_no) FROM sales WHERE tenant_id=$1`, e.tenant).Scan(&distinct)
	if distinct != 10 {
		t.Fatalf("nomor nota harus unik: %d", distinct)
	}
}

func TestGetIsTenantScopedAndSalesAreImmutable(t *testing.T) {
	e := newEnv(t)
	a := e.item(t, "goods", "1000", "0", 5, false)
	s, _, err := e.svc.Create(context.Background(), e.actor(e.tenant), key(), Request{Lines: []LineIn{line(a, "1")}, Payments: []PaymentIn{pay("cash", "1000")}, Note: " catatan  uji "})
	if err != nil || s.Note != "catatan uji" || s.CashierName != "Kasir Uji" {
		t.Fatalf("%v %+v", err, s)
	}
	if _, err := e.svc.Get(context.Background(), e.actor(e.other), s.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("tenant lain: %v", err)
	}
	// Role aplikasi tidak bisa menghapus nota maupun mengubah barisnya.
	err = db.WithTenant(context.Background(), e.app, e.tenant, func(tx pgx.Tx) error {
		_, err := tx.Exec(context.Background(), `DELETE FROM sales WHERE id = $1`, s.ID)
		return err
	})
	if err == nil {
		t.Fatal("DELETE nota seharusnya ditolak")
	}
	err = db.WithTenant(context.Background(), e.app, e.tenant, func(tx pgx.Tx) error {
		_, err := tx.Exec(context.Background(), `UPDATE sale_lines SET unit_price = 1 WHERE sale_id = $1`, s.ID)
		return err
	})
	if err == nil {
		t.Fatal("UPDATE baris nota seharusnya ditolak")
	}
}

// Quote memakai aturan harga yang sama dengan Create, tidak menyimpan apa pun, dan tidak menyentuh stok.
func TestQuoteMatchesCreateAndPersistsNothing(t *testing.T) {
	e := newEnv(t)
	a := e.item(t, "goods", "10000", "0", 20, false)
	e.exec(t, `INSERT INTO item_wholesale_tiers (tenant_id, item_id, min_qty, price) VALUES ($1,$2,2,9500),($1,$2,6,8500)`, e.tenant, a)
	in := Request{Lines: []LineIn{line(a, "7")}, OtherCost: "100", ApplyTax: true}
	q, err := e.svc.Quote(context.Background(), e.actor(e.tenant), in)
	if err != nil || q.Lines[0].UnitPrice != "8500.00" || q.Total != "66145.00" { // 59.500 + pajak 10% 5.950 + 1% 595 + biaya 100
		t.Fatalf("quote: %v %+v", err, q)
	}
	if e.count(t, "sales") != 0 || e.count(t, "stock_movements") != 1 { // 1 = saldo awal
		t.Fatal("quote tidak boleh menyimpan nota atau menggerakkan stok")
	}
	in.Payments = []PaymentIn{pay("cash", "70000")}
	s, _, err := e.svc.Create(context.Background(), e.actor(e.tenant), key(), in)
	if err != nil || s.Total != q.Total || s.Lines[0].UnitPrice != q.Lines[0].UnitPrice {
		t.Fatalf("create harus sama dengan quote: %v %+v", err, s)
	}
	// Quote tidak peduli stok (stok tinggal 13; minta 500).
	if _, err := e.svc.Quote(context.Background(), e.actor(e.tenant), Request{Lines: []LineIn{line(a, "500")}}); err != nil {
		t.Fatalf("quote tanpa cek stok: %v", err)
	}
}

// Quote menandai baris yang tak boleh dibayar (stok kurang / di bawah HPP) tanpa menggagalkan seluruh hitungan.
func TestQuoteFlagsStockAndBelowCost(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	a := e.item(t, "goods", "1000", "800", 5, false)
	neg := e.item(t, "goods", "1000", "0", 0, true)
	svc := e.item(t, "service", "500", "0", 0, false)
	q, err := e.svc.Quote(ctx, e.actor(e.tenant), Request{Lines: []LineIn{line(a, "3"), line(a, "3"), line(neg, "4"), line(svc, "2")}})
	if err != nil {
		t.Fatal(err)
	}
	// A: total 6 > stok 5 → kedua barisnya bermasalah (stok dihitung lintas baris); minus-boleh dan jasa bebas.
	if q.Lines[0].Issue != codeStockShort || q.Lines[1].Issue != codeStockShort || q.Lines[0].Available != "5" {
		t.Errorf("stok A: %+v", q.Lines[:2])
	}
	if q.Lines[2].Issue != "" || q.Lines[3].Issue != "" || q.Lines[3].Available != "" {
		t.Errorf("minus/jasa: %+v", q.Lines[2:])
	}
	q, err = e.svc.Quote(ctx, e.actor(e.tenant), Request{Lines: []LineIn{{ItemID: a, Qty: "1", Discount: "300"}}})
	if err != nil || q.Lines[0].Issue != codeBelowCost {
		t.Fatalf("di bawah HPP: %v %+v", err, q.Lines)
	}
}

// Nota besar (ratusan jenis barang) harus tetap diterima dan selesai dalam waktu wajar.
func TestLargeSale(t *testing.T) {
	e := newEnv(t)
	var lines []LineIn
	for range 400 {
		lines = append(lines, line(e.item(t, "goods", "1000", "0", 5, false), "2"))
	}
	start := time.Now()
	q, err := e.svc.Quote(context.Background(), e.actor(e.tenant), Request{Lines: lines})
	if err != nil || q.Total != "800000.00" {
		t.Fatalf("quote: %v %s", err, q.Total)
	}
	qd := time.Since(start)
	start = time.Now()
	s, _, err := e.svc.Create(context.Background(), e.actor(e.tenant), key(), Request{Lines: lines, Payments: []PaymentIn{pay("cash", "800000")}})
	if err != nil || len(s.Lines) != 400 {
		t.Fatalf("create: %v", err)
	}
	t.Logf("400 baris: quote %v, simpan %v", qd, time.Since(start))
	if time.Since(start) > 10*time.Second {
		t.Fatal("terlalu lambat")
	}
	over := make([]LineIn, 501)
	for i := range over {
		over[i] = lines[0]
	}
	_, _, err = e.svc.Create(context.Background(), e.actor(e.tenant), key(), Request{Lines: over})
	fieldErr(t, err, "lines", codeTooMany)
}

// ---- Ubah harga dengan PIN penyetuju ----

const testPassword = "Passw0rd-Uji9"

// person membuat pengguna (role dengan izin tertentu); outlet=true memberinya akses ke outlet uji.
func (e *env) person(t *testing.T, name, perms string, outlet bool) authz.Actor {
	t.Helper()
	role, user := uuid.New(), uuid.New()
	hash, err := pauth.HashPassword(testPassword)
	if err != nil {
		t.Fatal(err)
	}
	e.exec(t, "INSERT INTO roles (id, tenant_id, name, permissions) VALUES ($1, $2, $3, $4::jsonb)", role, e.tenant, name+"-"+role.String()[:6], perms)
	e.exec(t, "INSERT INTO users (id, tenant_id, role_id, email, name, password_hash) VALUES ($1, $2, $3, $4, $5, $6)", user, e.tenant, role, name+"-"+user.String()+"@example.test", name, hash)
	if outlet {
		e.exec(t, "INSERT INTO user_outlets (tenant_id, user_id, outlet_id) VALUES ($1, $2, $3)", e.tenant, user, e.outlet)
	}
	perm, err := authz.Normalize(map[string][]string{})
	if err != nil {
		t.Fatal(err)
	}
	_ = perm
	return authz.Actor{TenantID: e.tenant, UserID: user, OutletID: e.outlet, Name: name, Perms: authz.ParseStored([]byte(perms))}
}

func overrideReq(item uuid.UUID, price string, ap *ApprovalIn) Request {
	return Request{Lines: []LineIn{{ItemID: item, Qty: "2", UnitPrice: json.Number(price)}}, Payments: []PaymentIn{pay("cash", "100000")}, Approval: ap}
}

func TestPriceOverrideNeedsApproverPin(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	a := e.item(t, "goods", "10000", "5000", 20, false)
	spv := e.person(t, "Supervisor", `{"price_override":["approve"]}`, true)
	away := e.person(t, "SupervisorLain", `{"price_override":["approve"]}`, false) // tanpa akses outlet ini
	plain := e.person(t, "Kasir2", `{"sales_orders":["create"]}`, true)
	cashier := e.actor(e.tenant)
	ap := e.svc.approvals

	// PIN: format & kemudahan ditebak; password salah; bukan pemegang izin.
	for _, bad := range []string{"12345", "1234567", "abcdef", "111111", "123456", "654321"} {
		if err := ap.SetPin(ctx, spv, testPassword, bad); err == nil {
			t.Fatalf("PIN %q seharusnya ditolak", bad)
		}
	}
	if err := ap.SetPin(ctx, spv, "salah", "482915"); !errors.Is(err, approval.ErrBadPassword) {
		t.Fatalf("password salah: %v", err)
	}
	if err := ap.SetPin(ctx, plain, testPassword, "482915"); !errors.Is(err, approval.ErrForbidden) {
		t.Fatalf("bukan penyetuju: %v", err)
	}
	for _, p := range []authz.Actor{spv, away} {
		if err := ap.SetPin(ctx, p, testPassword, "482915"); err != nil {
			t.Fatal(err)
		}
	}
	var stored string
	_ = e.admin.QueryRow(ctx, "SELECT pin_hash FROM users WHERE id = $1", spv.UserID).Scan(&stored)
	if stored == "" || strings.Contains(stored, "482915") {
		t.Fatal("PIN tidak boleh tersimpan polos")
	}

	// Daftar penyetuju hanya yang berizin DAN berakses ke outlet kasir.
	list, err := ap.Approvers(ctx, cashier)
	if err != nil || len(list) != 1 || list[0].ID != spv.UserID {
		t.Fatalf("penyetuju: %v %+v", err, list)
	}

	// Tanpa persetujuan: ditolak, tak ada nota.
	if _, _, err := e.svc.Create(ctx, cashier, key(), overrideReq(a, "9000", nil)); !errors.Is(err, approval.ErrPinRequired) {
		t.Fatalf("tanpa persetujuan: %v", err)
	}
	// PIN salah, penyetuju tanpa izin, penyetuju tanpa akses outlet, penyetuju asing: semuanya galat yang sama.
	for name, in := range map[string]*ApprovalIn{
		"pin salah":    {UserID: spv.UserID, PIN: "000111"},
		"tanpa izin":   {UserID: plain.UserID, PIN: "482915"},
		"tanpa outlet": {UserID: away.UserID, PIN: "482915"},
		"tak ada":      {UserID: uuid.New(), PIN: "482915"},
	} {
		if _, _, err := e.svc.Create(ctx, cashier, key(), overrideReq(a, "9000", in)); !errors.Is(err, approval.ErrInvalidPin) {
			t.Fatalf("%s: %v", name, err)
		}
	}
	if e.count(t, "sales") != 0 || e.stockOf(t, a) != "20" {
		t.Fatal("penolakan tidak boleh meninggalkan nota/stok")
	}

	// Disetujui: harga baris berubah, harga daftar & penyetuju tercatat, audit khusus.
	s, _, err := e.svc.Create(ctx, cashier, key(), overrideReq(a, "9000", &ApprovalIn{UserID: spv.UserID, PIN: "482915"}))
	if err != nil {
		t.Fatal(err)
	}
	l := s.Lines[0]
	if l.UnitPrice != "9000.00" || l.ListPrice != "10000.00" || !l.PriceOverride || s.Total != "18000.00" || s.ApprovedBy != "Supervisor" {
		t.Fatalf("hasil: %+v approvedBy=%q total=%s", l, s.ApprovedBy, s.Total)
	}
	var n int
	_ = e.admin.QueryRow(ctx, "SELECT count(*) FROM audit_log WHERE tenant_id=$1 AND action='sale.price_override'", e.tenant).Scan(&n)
	if n != 1 {
		t.Fatalf("audit override = %d", n)
	}
	// Harga yang sama dengan harga daftar bukan "ubah harga": tanpa PIN pun lolos.
	s, _, err = e.svc.Create(ctx, cashier, key(), overrideReq(a, "10000", nil))
	if err != nil || s.Lines[0].PriceOverride {
		t.Fatalf("harga sama: %v %+v", err, s.Lines)
	}
	// Di bawah HPP tetap ditolak walau disetujui.
	_, _, err = e.svc.Create(ctx, cashier, key(), overrideReq(a, "4000", &ApprovalIn{UserID: spv.UserID, PIN: "482915"}))
	fieldErr(t, err, "lines.0.item_id", codeBelowCost)
	// Quote menampilkan harga baru tanpa PIN.
	q, err := e.svc.Quote(ctx, cashier, overrideReq(a, "9500", nil))
	if err != nil || q.Lines[0].UnitPrice != "9500.00" || !q.Lines[0].PriceOverride || q.Lines[0].ListPrice != "10000.00" {
		t.Fatalf("quote: %v %+v", err, q.Lines)
	}
}

// Penyetuju terkunci setelah 5 PIN salah (butuh Redis).
func TestApproverPinLockout(t *testing.T) {
	url := os.Getenv("TEST_REDIS_URL")
	if url == "" {
		t.Skip("TEST_REDIS_URL tidak di-set")
	}
	opt, err := redis.ParseURL(url)
	if err != nil {
		t.Fatal(err)
	}
	rdb := redis.NewClient(opt)
	t.Cleanup(func() { _ = rdb.Close() })
	e := newEnv(t)
	ctx := context.Background()
	ap := approval.NewService(e.app, rdb, "uji-rahasia-uji-rahasia-uji-rahasia")
	e.svc.approvals = ap
	a := e.item(t, "goods", "10000", "5000", 20, false)
	spv := e.person(t, "Supervisor", `{"price_override":["approve"]}`, true)
	if err := ap.SetPin(ctx, spv, testPassword, "482915"); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { rdb.Del(ctx, "pin:fail:"+e.tenant.String()+":"+spv.UserID.String()) })
	cashier := e.actor(e.tenant)
	for i := 0; i < 5; i++ {
		if _, _, err := e.svc.Create(ctx, cashier, key(), overrideReq(a, "9000", &ApprovalIn{UserID: spv.UserID, PIN: "000111"})); !errors.Is(err, approval.ErrInvalidPin) {
			t.Fatalf("percobaan %d: %v", i, err)
		}
	}
	// Setelah terkunci, bahkan PIN benar ditolak sampai jendela habis.
	_, _, err = e.svc.Create(ctx, cashier, key(), overrideReq(a, "9000", &ApprovalIn{UserID: spv.UserID, PIN: "482915"}))
	var lk *approval.LockedError
	if !errors.As(err, &lk) {
		t.Fatalf("ingin terkunci, dapat %v", err)
	}
}

// approverIn membuat penyetuju ber-PIN (untuk uji yang butuh persetujuan) dan mengembalikan bukti persetujuannya.
func (e *env) approverIn(t *testing.T) *ApprovalIn {
	t.Helper()
	spv := e.person(t, "Penyetuju", `{"price_override":["approve"]}`, true)
	if err := e.svc.approvals.SetPin(context.Background(), spv, testPassword, "482915"); err != nil {
		t.Fatal(err)
	}
	return &ApprovalIn{UserID: spv.UserID, PIN: "482915"}
}

func TestLineDiscountNeedsApproverPin(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	a := e.item(t, "goods", "10000", "5000", 20, false)
	cashier := e.actor(e.tenant)
	req := func(ap *ApprovalIn) Request {
		return Request{Lines: []LineIn{{ItemID: a, Qty: "2", Discount: "1500"}}, Payments: []PaymentIn{pay("cash", "100000")}, Approval: ap}
	}
	if _, _, err := e.svc.Create(ctx, cashier, key(), req(nil)); !errors.Is(err, approval.ErrPinRequired) {
		t.Fatalf("tanpa persetujuan: %v", err)
	}
	ap := e.approverIn(t)
	if _, _, err := e.svc.Create(ctx, cashier, key(), req(&ApprovalIn{UserID: ap.UserID, PIN: "000111"})); !errors.Is(err, approval.ErrInvalidPin) {
		t.Fatalf("PIN salah: %v", err)
	}
	if e.count(t, "sales") != 0 || e.stockOf(t, a) != "20" {
		t.Fatal("penolakan tidak boleh meninggalkan nota/stok")
	}
	s, _, err := e.svc.Create(ctx, cashier, key(), req(ap))
	if err != nil || s.Total != "18500.00" || s.Lines[0].Discount != "1500.00" {
		t.Fatalf("disetujui: %v %+v", err, s)
	}
	var n int
	_ = e.admin.QueryRow(ctx, "SELECT count(*) FROM audit_log WHERE tenant_id=$1 AND action='sale.line_discount'", e.tenant).Scan(&n)
	if n != 1 {
		t.Fatalf("audit potongan = %d", n)
	}
	// Tanpa potongan tidak butuh PIN.
	if _, _, err := e.svc.Create(ctx, cashier, key(), Request{Lines: []LineIn{line(a, "1")}, Payments: []PaymentIn{pay("cash", "10000")}}); err != nil {
		t.Fatal(err)
	}
}

// Daftar penjualan kasir: default hari ini di outlet aktif, rentang tanggal, cari no. nota, total per metode (tunai bersih).
func TestListRangeSearchAndTotals(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	it := e.item(t, "goods", "1000", "0", 10, false)
	a := e.actor(e.tenant)
	var ids []uuid.UUID
	for i := 0; i < 2; i++ {
		s, _, err := e.svc.Create(ctx, a, key(), Request{Lines: []LineIn{line(it, "1")}, Payments: []PaymentIn{pay("cash", "1500")}})
		if err != nil {
			t.Fatal(err)
		}
		ids = append(ids, s.ID)
	}
	e.exec(t, `UPDATE sales SET created_at = created_at - interval '2 days' WHERE id = $1`, ids[0])
	res, err := e.svc.List(ctx, a, "", "", "")
	if err != nil || len(res.Data) != 1 || res.Data[0].ID != ids[1] || res.Data[0].Total != "1000.00" || res.Data[0].LineCount != 1 {
		t.Fatalf("hari ini: %+v %v", res, err)
	}
	if res.Totals["cash"] != "1000.00" || res.Total != "1000.00" { // 1500 diterima - 500 kembalian
		t.Fatalf("total tunai bersih: %+v", res)
	}
	from := time.Now().AddDate(0, 0, -3).Format("2006-01-02")
	to := time.Now().AddDate(0, 0, 1).Format("2006-01-02")
	if wide, err := e.svc.List(ctx, a, from, to, ""); err != nil || len(wide.Data) != 2 || wide.Total != "2000.00" {
		t.Fatalf("rentang: %+v %v", wide, err)
	}
	if none, err := e.svc.List(ctx, a, from, to, "tidak-ada"); err != nil || len(none.Data) != 0 {
		t.Fatalf("cari: %+v %v", none, err)
	}
	var fe FieldErrors
	if _, err := e.svc.List(ctx, a, "2026-13-40", "", ""); !errors.As(err, &fe) {
		t.Fatalf("tanggal salah: %v", err)
	}
	if _, err := e.svc.List(ctx, a, "2026-01-01", "2026-12-31", ""); !errors.As(err, &fe) {
		t.Fatalf("rentang kepanjangan: %v", err)
	}
	// Outlet milik tenant lain tidak terlihat sama sekali.
	if _, err := e.svc.List(ctx, e.actor(e.other), "", "", ""); !errors.Is(err, ErrOutletInactive) {
		t.Fatalf("tenant lain: %v", err)
	}
}

// Daftar penjualan & detail nota hanya milik kasir yang login (untuk mencocokkan uang fisik); kasir lain tak terlihat.
func TestListAndGetAreScopedToCashier(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	it := e.item(t, "goods", "1000", "0", 10, false)
	var role uuid.UUID
	if err := e.admin.QueryRow(ctx, `SELECT id FROM roles WHERE tenant_id = $1 LIMIT 1`, e.tenant).Scan(&role); err != nil {
		t.Fatal(err)
	}
	userB := uuid.New()
	e.exec(t, `INSERT INTO users (id, tenant_id, role_id, email, name, password_hash) VALUES ($1, $2, $3, $4, 'Kasir B', 'x')`, userB, e.tenant, role, "b-"+userB.String()+"@example.test")
	a, b := e.actor(e.tenant), e.actor(e.tenant)
	b.UserID = userB

	sa, _, err := e.svc.Create(ctx, a, key(), Request{Lines: []LineIn{line(it, "1")}, Payments: []PaymentIn{pay("cash", "1000")}})
	if err != nil {
		t.Fatal(err)
	}
	sb, _, err := e.svc.Create(ctx, b, key(), Request{Lines: []LineIn{line(it, "2")}, Payments: []PaymentIn{pay("cash", "2000")}})
	if err != nil {
		t.Fatal(err)
	}
	ra, _ := e.svc.List(ctx, a, "", "", "")
	rb, _ := e.svc.List(ctx, b, "", "", "")
	if len(ra.Data) != 1 || ra.Data[0].ID != sa.ID || ra.Total != "1000.00" || len(rb.Data) != 1 || rb.Data[0].ID != sb.ID || rb.Total != "2000.00" {
		t.Fatalf("daftar harus per kasir: A=%+v B=%+v", ra, rb)
	}
	if _, err := e.svc.Get(ctx, a, sb.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("nota kasir lain tidak boleh dibuka: %v", err)
	}
	if _, err := e.svc.Get(ctx, a, sa.ID); err != nil {
		t.Fatalf("nota sendiri: %v", err)
	}
	// Pemegang izin daftar penjualan boleh membuka nota kasir lain.
	a.Perms = authz.Permissions{All: false, Grants: map[string][]string{"sales_list": {"view"}}}
	if _, err := e.svc.Get(ctx, a, sb.ID); err != nil {
		t.Fatalf("pemegang sales_list.view: %v", err)
	}
}

// 12 kasir menyimpan nota bersamaan di outlet yang sama: nomor harus unik, berurutan 1..12, tanpa celah.
func TestTwelveCashiersGetDistinctGaplessNumbers(t *testing.T) {
	e := newEnv(t)
	it := e.item(t, "goods", "1000", "0", 100, false)
	var wg sync.WaitGroup
	for range 12 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, _, err := e.svc.Create(context.Background(), e.actor(e.tenant), key(), Request{Lines: []LineIn{line(it, "1")}, Payments: []PaymentIn{pay("cash", "1000")}}); err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	var distinct, maxNo int
	_ = e.admin.QueryRow(context.Background(), `SELECT count(DISTINCT doc_no), max(right(doc_no, 4)::int) FROM sales WHERE tenant_id=$1`, e.tenant).Scan(&distinct, &maxNo)
	if distinct != 12 || maxNo != 12 {
		t.Fatalf("nomor unik=%d terbesar=%d, harus 12 dan 12", distinct, maxNo)
	}
}

// HPP per cabang: snapshot HPP nota dan batas jual di bawah HPP memakai HPP cabang nota (bila ada), bukan HPP awal barang.
func TestSaleUsesOutletCost(t *testing.T) {
	e := newEnv(t)
	a := e.item(t, "goods", "5000", "3000", 10, false) // HPP awal 3.000
	e.exec(t, `INSERT INTO item_outlet_costs (tenant_id, outlet_id, item_id, avg_cost, last_cost) VALUES ($1,$2,$3,4000,4000)`, e.tenant, e.outlet, a)
	s, _, err := e.svc.Create(context.Background(), e.actor(e.tenant), key(), Request{Lines: []LineIn{line(a, "1")}, Payments: []PaymentIn{pay("cash", "5000")}})
	var snap string
	if err != nil {
		t.Fatal(err)
	}
	if err := e.admin.QueryRow(context.Background(), `SELECT unit_cost::text FROM sale_lines WHERE sale_id = $1`, s.ID).Scan(&snap); err != nil || snap != "4000.00" {
		t.Fatalf("snapshot HPP cabang: %q %v", snap, err)
	}
	// Potongan 1.500 → 3.500: di atas HPP awal (3.000) tetapi di bawah HPP cabang (4.000) → ditolak.
	l := line(a, "1")
	l.Discount = "1500"
	_, _, err = e.svc.Create(context.Background(), e.actor(e.tenant), key(), Request{Lines: []LineIn{l}, Payments: []PaymentIn{pay("cash", "3500")}})
	if err == nil {
		t.Fatal("jual di bawah HPP cabang harus ditolak")
	}
}
