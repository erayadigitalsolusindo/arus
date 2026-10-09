package purchasing

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/shopspring/decimal"

	"aciraba/internal/authz"
	"aciraba/internal/platform/db"
	"aciraba/internal/stock"
)

type env struct {
	app, admin *pgxpool.Pool
	svc        *Service
	tenant     uuid.UUID
	other      uuid.UUID
	outlet     uuid.UUID
	outlet2    uuid.UUID
	unit       uuid.UUID
	supplier   uuid.UUID
	user       uuid.UUID
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

	e := &env{app: app, admin: admin, svc: NewService(app), tenant: uuid.New(), other: uuid.New(), outlet: uuid.New(), outlet2: uuid.New(),
		unit: uuid.New(), supplier: uuid.New(), user: uuid.New()}
	role := uuid.New()
	for _, s := range []struct {
		sql  string
		args []any
	}{
		{`INSERT INTO tenants (id, code, name) VALUES ($1, $2, 'UJI-BELI')`, []any{e.tenant, "pb-" + e.tenant.String()[:8]}},
		{`INSERT INTO tenants (id, code, name) VALUES ($1, $2, 'UJI-BELI-2')`, []any{e.other, "pb-" + e.other.String()[:8]}},
		{`INSERT INTO outlets (id, tenant_id, code, name) VALUES ($1, $2, 'main', 'Pusat')`, []any{e.outlet, e.tenant}},
		{`INSERT INTO outlets (id, tenant_id, code, name) VALUES ($1, $2, 'cab2', 'Cabang 2')`, []any{e.outlet2, e.tenant}},
		{`INSERT INTO units (id, tenant_id, name) VALUES ($1, $2, 'Pcs')`, []any{e.unit, e.tenant}},
		{`INSERT INTO suppliers (id, tenant_id, name) VALUES ($1, $2, 'PT Sumber Rejeki')`, []any{e.supplier, e.tenant}},
		{`INSERT INTO roles (id, tenant_id, name) VALUES ($1, $2, 'Owner')`, []any{role, e.tenant}},
		{`INSERT INTO users (id, tenant_id, role_id, email, name, password_hash) VALUES ($1, $2, $3, $4, 'Staf Gudang', 'x')`, []any{e.user, e.tenant, role, e.user.String() + "@uji.test"}},
	} {
		if _, err := admin.Exec(ctx, s.sql, s.args...); err != nil {
			t.Fatal(err)
		}
	}
	t.Cleanup(func() {
		for _, tid := range []uuid.UUID{e.tenant, e.other} {
			for _, tbl := range []string{"audit_log", "payables", "purchase_costs", "purchase_lines", "purchases", "purchase_counters", "stock_movements", "stock_balances",
				"item_outlet_costs", "items", "suppliers", "units", "users", "roles", "outlets"} {
				_, _ = admin.Exec(ctx, `DELETE FROM `+tbl+` WHERE tenant_id = $1`, tid)
			}
			_, _ = admin.Exec(ctx, `DELETE FROM tenants WHERE id = $1`, tid)
		}
	})
	return e
}

func (e *env) actor(outlet uuid.UUID) authz.Actor {
	return authz.Actor{TenantID: e.tenant, UserID: e.user, OutletID: outlet, Name: "Staf Gudang", Perms: authz.Permissions{All: true},
		Outlets: map[uuid.UUID]bool{outlet: true}}
}

// item membuat barang dengan HPP awal `cost` dan stok awal `qty` di Display outlet utama.
func (e *env) item(t *testing.T, kind, cost string, qty int) uuid.UUID {
	t.Helper()
	ctx := context.Background()
	id := uuid.New()
	if _, err := e.admin.Exec(ctx, `INSERT INTO items (id, tenant_id, sku, name, unit_id, kind, avg_cost, last_cost) VALUES ($1, $2, $3, $4, $5, $6, $7, $7)`,
		id, e.tenant, "S-"+id.String()[:8], "Barang "+id.String()[:4], e.unit, kind, cost); err != nil {
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

func (e *env) bal(t *testing.T, item uuid.UUID, bucket string) string {
	t.Helper()
	var q string
	err := e.admin.QueryRow(context.Background(), `SELECT coalesce((SELECT qty::text FROM stock_balances WHERE tenant_id=$1 AND outlet_id=$2 AND item_id=$3 AND bucket=$4), '0')`,
		e.tenant, e.outlet, item, bucket).Scan(&q)
	if err != nil {
		t.Fatal(err)
	}
	return decimal.RequireFromString(q).String()
}

// cost = HPP efektif (HPP cabang bila ada, selain itu HPP awal) dan harga beli akhir di outlet tertentu.
func (e *env) cost(t *testing.T, outlet, item uuid.UUID) (avg, last string) {
	t.Helper()
	err := e.admin.QueryRow(context.Background(), `SELECT coalesce(oc.avg_cost, i.avg_cost)::text, coalesce(oc.last_cost, i.last_cost)::text FROM items i
		LEFT JOIN item_outlet_costs oc ON oc.tenant_id = i.tenant_id AND oc.item_id = i.id AND oc.outlet_id = $2 WHERE i.id = $1`, item, outlet).Scan(&avg, &last)
	if err != nil {
		t.Fatal(err)
	}
	return avg, last
}

func (e *env) count(t *testing.T, tbl string) int {
	t.Helper()
	var n int
	if err := e.admin.QueryRow(context.Background(), `SELECT count(*) FROM `+tbl+` WHERE tenant_id = $1`, e.tenant).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

var keySeq int

func key() string { keySeq++; return fmt.Sprintf("test-key-%d-%s", keySeq, uuid.NewString()[:8]) }

func num(s string) json.Number { return json.Number(s) }

// line membuat baris pembelian: qd = qty Display, qw = qty Gudang.
func line(item uuid.UUID, qd, qw, price string, disc ...string) LineIn {
	l := LineIn{ItemID: item, QtyDisplay: num(qd), QtyWarehouse: num(qw), UnitPrice: num(price)}
	for _, d := range disc {
		l.Discounts = append(l.Discounts, num(d))
	}
	return l
}

func (e *env) req(lines ...LineIn) Request {
	return Request{SupplierID: e.supplier, PaymentType: "cash", Lines: lines}
}

func TestCashPurchaseUpdatesStockAndOutletCost(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	a := e.actor(e.outlet)
	it := e.item(t, "goods", "1000", 10) // stok 10 @ 1.000
	p, replayed, err := e.svc.Create(ctx, a, key(), e.req(line(it, "6", "4", "1200")))
	if err != nil || replayed {
		t.Fatalf("simpan: %v replay=%v", err, replayed)
	}
	if !strings.HasPrefix(p.DocNo, "PB-MAIN-") || !strings.HasSuffix(p.DocNo, "-0001") {
		t.Errorf("nomor = %s", p.DocNo)
	}
	if p.Subtotal != "12000.00" || p.Total != "12000.00" || p.TaxAmount != "0.00" || p.Payable != nil || p.PaymentType != "cash" {
		t.Errorf("header: %+v", p)
	}
	if e.bal(t, it, "display") != "16" || e.bal(t, it, "warehouse") != "4" {
		t.Errorf("stok display=%s gudang=%s", e.bal(t, it, "display"), e.bal(t, it, "warehouse"))
	}
	// HPP = (10 × 1.000 + 10 × 1.200) ÷ 20 = 1.100; harga beli akhir 1.200. Cabang lain tetap memakai HPP awal.
	if avg, last := e.cost(t, e.outlet, it); avg != "1100.00" || last != "1200.00" {
		t.Errorf("HPP cabang utama = %s/%s", avg, last)
	}
	if avg, _ := e.cost(t, e.outlet2, it); avg != "1000.00" {
		t.Errorf("HPP cabang 2 = %s", avg)
	}
	var base string
	if err := e.admin.QueryRow(ctx, `SELECT avg_cost::text FROM items WHERE id = $1`, it).Scan(&base); err != nil || base != "1000.00" {
		t.Errorf("HPP awal barang berubah: %s %v", base, err)
	}
	l := p.Lines[0]
	if l.StockBefore != "10" || l.AvgBefore != "1000.00" || l.AvgAfter != "1100.00" || l.UnitCost != "1200.00" || l.Qty != "10" {
		t.Errorf("baris: %+v", l)
	}
	var n int
	if err := e.admin.QueryRow(ctx, `SELECT count(*) FROM stock_movements WHERE ref_type='PURCHASE' AND ref_id=$1`, p.ID).Scan(&n); err != nil || n != 2 {
		t.Errorf("movement PURCHASE = %d %v", n, err)
	}
}

func TestTotalsDiscountsTaxAndCostAllocation(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	a := e.actor(e.outlet)
	i1, i2 := e.item(t, "goods", "0", 0), e.item(t, "goods", "0", 0)
	in := e.req(line(i1, "10", "0", "1000", "10", "5"), line(i2, "0", "5", "2000"))
	in.PaymentType = "credit"
	in.TaxPct = num("11")
	in.OtherCosts = []CostIn{{Name: "Ongkir", Amount: num("1000")}, {Name: "Bongkar", Amount: num("500")}}
	in.DueDate = time.Now().AddDate(0, 0, 30).Format("2006-01-02")
	in.SupplierInvoiceNo = "INV-001"
	p, _, err := e.svc.Create(ctx, a, key(), in)
	if err != nil {
		t.Fatal(err)
	}
	// 10 × 1.000 × 0,9 × 0,95 = 8.550; 5 × 2.000 = 10.000; subtotal 18.550; PPN 11% = 2.040,50; biaya lain 1.500.
	if p.Lines[0].LineTotal != "8550.00" || p.Lines[1].LineTotal != "10000.00" || p.Subtotal != "18550.00" ||
		p.TaxAmount != "2040.50" || p.OtherCost != "1500.00" || p.Total != "22090.50" {
		t.Fatalf("total: %+v", p)
	}
	// Biaya lain dibagi proporsional nilai baris dan jumlahnya tepat 1.500.
	if p.Lines[0].CostAlloc != "691.37" || p.Lines[1].CostAlloc != "808.63" {
		t.Errorf("alokasi = %s + %s", p.Lines[0].CostAlloc, p.Lines[1].CostAlloc)
	}
	// HPP baris = (nilai + alokasi) ÷ qty; PPN tidak ikut HPP. Stok awal 0 → HPP rata-rata = HPP baris.
	if p.Lines[0].UnitCost != "924.14" || p.Lines[1].UnitCost != "2161.73" {
		t.Errorf("HPP baris = %s, %s", p.Lines[0].UnitCost, p.Lines[1].UnitCost)
	}
	if avg, last := e.cost(t, e.outlet, i1); avg != "924.14" || last != "924.14" {
		t.Errorf("HPP = %s/%s", avg, last)
	}
	if len(p.Lines[0].Discounts) != 2 || len(p.Costs) != 2 || p.Costs[0].Name != "Ongkir" {
		t.Errorf("diskon/biaya: %+v %+v", p.Lines[0].Discounts, p.Costs)
	}
	// Kredit → hutang sebesar total + jatuh tempo.
	if p.Payable == nil || p.Payable.Amount != "22090.50" || p.Payable.DueDate == nil || p.DueDate == nil {
		t.Errorf("hutang: %+v", p.Payable)
	}
}

func TestValidationAndAtomicity(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	a := e.actor(e.outlet)
	it := e.item(t, "goods", "1000", 5)
	svcItem := e.item(t, "service", "0", 0)
	inactive := e.item(t, "goods", "0", 0)
	e.admin.Exec(ctx, `UPDATE items SET active = false WHERE id = $1`, inactive)
	inactiveSup := uuid.New()
	e.admin.Exec(ctx, `INSERT INTO suppliers (id, tenant_id, name, active) VALUES ($1, $2, 'Lama', false)`, inactiveSup, e.tenant)
	today := time.Now().Format("2006-01-02")
	cases := map[string]struct {
		mut  func(r *Request)
		want string
	}{
		"tanpa baris":           {func(r *Request) { r.Lines = nil }, "lines"},
		"tanpa pemasok":         {func(r *Request) { r.SupplierID = uuid.Nil }, "supplier_id"},
		"pemasok nonaktif":      {func(r *Request) { r.SupplierID = inactiveSup }, "supplier_id"},
		"pemasok asing":         {func(r *Request) { r.SupplierID = uuid.New() }, "supplier_id"},
		"jenis bayar":           {func(r *Request) { r.PaymentType = "barter" }, "payment_type"},
		"qty nol":               {func(r *Request) { r.Lines[0] = line(it, "0", "0", "1000") }, "lines.0.qty_display"},
		"qty negatif":           {func(r *Request) { r.Lines[0] = line(it, "-1", "0", "1000") }, "lines.0.qty_display"},
		"qty 4 desimal":         {func(r *Request) { r.Lines[0] = line(it, "1.0001", "0", "1000") }, "lines.0.qty_display"},
		"harga kosong":          {func(r *Request) { r.Lines[0].UnitPrice = "" }, "lines.0.unit_price"},
		"diskon rupiah > nilai": {func(r *Request) { r.Lines[0] = line(it, "1", "0", "1000", "1500") }, "lines.0.discounts"},
		"diskon 5 tingkat":      {func(r *Request) { r.Lines[0] = line(it, "1", "0", "1000", "1", "1", "1", "1", "1") }, "lines.0.discounts"},
		"barang jasa":           {func(r *Request) { r.Lines[0] = line(svcItem, "1", "0", "1000") }, "lines.0.item_id"},
		"barang nonaktif":       {func(r *Request) { r.Lines[0] = line(inactive, "1", "0", "1000") }, "lines.0.item_id"},
		"barang asing":          {func(r *Request) { r.Lines[0] = line(uuid.New(), "1", "0", "1000") }, "lines.0.item_id"},
		"tanggal depan":         {func(r *Request) { r.PurchaseDate = time.Now().AddDate(0, 0, 3).Format("2006-01-02") }, "purchase_date"},
		"tanggal rusak":         {func(r *Request) { r.PurchaseDate = "kemarin" }, "purchase_date"},
		"jatuh tempo tunai":     {func(r *Request) { r.DueDate = today }, "due_date"},
		"jatuh tempo lampau":    {func(r *Request) { r.PaymentType, r.DueDate = "credit", "2000-01-01" }, "due_date"},
		"PPN 101":               {func(r *Request) { r.TaxPct = num("101") }, "tax_pct"},
		"biaya nol":             {func(r *Request) { r.OtherCosts = []CostIn{{Name: "X", Amount: num("0")}} }, "other_costs.0.amount"},
		"nilai raksasa":         {func(r *Request) { r.Lines[0] = line(it, "999999999", "0", "999999999") }, "lines.0.unit_price"},
		"faktur 61 huruf":       {func(r *Request) { r.SupplierInvoiceNo = strings.Repeat("x", 61) }, "supplier_invoice_no"},
	}
	for name, c := range cases {
		r := e.req(line(it, "1", "0", "1000"))
		c.mut(&r)
		_, _, err := e.svc.Create(ctx, a, key(), r)
		var fe FieldErrors
		if !errors.As(err, &fe) || fe[c.want] == "" {
			t.Errorf("%s: ingin galat di %q, dapat %v", name, c.want, err)
		}
	}
	// Satu baris tidak valid → seluruh nota batal: tak ada nota, stok, movement, atau HPP berubah.
	_, _, err := e.svc.Create(ctx, a, key(), e.req(line(it, "3", "0", "1500"), line(svcItem, "1", "0", "1")))
	if err == nil {
		t.Fatal("harus gagal")
	}
	if e.count(t, "purchases") != 0 || e.count(t, "purchase_lines") != 0 || e.bal(t, it, "display") != "5" || e.count(t, "item_outlet_costs") != 0 {
		t.Error("nota gagal tidak boleh meninggalkan jejak")
	}
	// Tanpa Idempotency-Key.
	if _, _, err := e.svc.Create(ctx, a, "pendek", e.req(line(it, "1", "0", "1000"))); !errors.Is(err, ErrKeyRequired) {
		t.Errorf("tanpa kunci: %v", err)
	}
}

func TestDuplicateSupplierInvoiceIsRejectedPerSupplier(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	a := e.actor(e.outlet)
	it := e.item(t, "goods", "0", 0)
	mk := func(sup uuid.UUID, inv string) error {
		r := e.req(line(it, "1", "0", "1000"))
		r.SupplierID, r.SupplierInvoiceNo = sup, inv
		_, _, err := e.svc.Create(ctx, a, key(), r)
		return err
	}
	if err := mk(e.supplier, "F-100"); err != nil {
		t.Fatal(err)
	}
	var fe FieldErrors
	if err := mk(e.supplier, "f-100"); !errors.As(err, &fe) || fe["supplier_invoice_no"] != "INVOICE_DUPLICATE" {
		t.Errorf("faktur ganda (beda huruf besar) harus ditolak: %v", err)
	}
	other := uuid.New()
	e.admin.Exec(ctx, `INSERT INTO suppliers (id, tenant_id, name) VALUES ($1, $2, 'PT Lain')`, other, e.tenant)
	if err := mk(other, "F-100"); err != nil {
		t.Errorf("pemasok lain boleh memakai nomor yang sama: %v", err)
	}
	if err := mk(e.supplier, ""); err != nil {
		t.Errorf("tanpa nomor faktur: %v", err)
	}
	if err := mk(e.supplier, ""); err != nil {
		t.Errorf("tanpa nomor faktur (kedua): %v", err)
	}
	if e.count(t, "purchases") != 4 || e.bal(t, it, "display") != "4" {
		t.Errorf("nota=%d stok=%s", e.count(t, "purchases"), e.bal(t, it, "display"))
	}
}

func TestIdempotency(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	a := e.actor(e.outlet)
	it := e.item(t, "goods", "1000", 0)
	k := key()
	r := e.req(line(it, "5", "0", "1000"))
	p1, rep, err := e.svc.Create(ctx, a, k, r)
	if err != nil || rep {
		t.Fatal(err, rep)
	}
	p2, rep, err := e.svc.Create(ctx, a, k, r)
	if err != nil || !rep || p2.ID != p1.ID {
		t.Fatalf("replay: %v rep=%v id=%s/%s", err, rep, p2.ID, p1.ID)
	}
	if e.count(t, "purchases") != 1 || e.bal(t, it, "display") != "5" {
		t.Errorf("replay tidak boleh menggandakan stok: nota=%d stok=%s", e.count(t, "purchases"), e.bal(t, it, "display"))
	}
	if _, _, err := e.svc.Create(ctx, a, k, e.req(line(it, "6", "0", "1000"))); !errors.Is(err, ErrKeyMismatch) {
		t.Errorf("kunci sama isi beda: %v", err)
	}

	// Delapan kiriman bersamaan dengan kunci yang sama → tepat satu nota.
	k2 := key()
	r2 := e.req(line(it, "2", "0", "1000"))
	var wg sync.WaitGroup
	ids := make(chan uuid.UUID, 8)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if p, _, err := e.svc.Create(ctx, a, k2, r2); err == nil {
				ids <- p.ID
			}
		}()
	}
	wg.Wait()
	close(ids)
	seen := map[uuid.UUID]bool{}
	for id := range ids {
		seen[id] = true
	}
	if len(seen) != 1 || e.count(t, "purchases") != 2 || e.bal(t, it, "display") != "7" {
		t.Errorf("kiriman bersamaan: nota unik=%d total=%d stok=%s", len(seen), e.count(t, "purchases"), e.bal(t, it, "display"))
	}
}

func TestSameItemOnSeveralLinesAveragesStepwise(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	a := e.actor(e.outlet)
	it := e.item(t, "goods", "0", 0) // stok kosong, HPP 0
	p, _, err := e.svc.Create(ctx, a, key(), e.req(line(it, "10", "0", "1000"), line(it, "10", "0", "1400")))
	if err != nil {
		t.Fatal(err)
	}
	// Baris 1: stok 0 → HPP 1.000. Baris 2: (10 × 1.000 + 10 × 1.400) ÷ 20 = 1.200.
	if p.Lines[0].StockBefore != "0" || p.Lines[0].AvgAfter != "1000.00" || p.Lines[1].StockBefore != "10" || p.Lines[1].AvgBefore != "1000.00" || p.Lines[1].AvgAfter != "1200.00" {
		t.Errorf("langkah rata-rata: %+v", p.Lines)
	}
	if avg, last := e.cost(t, e.outlet, it); avg != "1200.00" || last != "1400.00" || e.bal(t, it, "display") != "20" {
		t.Errorf("hasil: avg=%s last=%s stok=%s", avg, last, e.bal(t, it, "display"))
	}
}

func TestNegativeStockBeforeUsesLineCost(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	a := e.actor(e.outlet)
	it := e.item(t, "goods", "500", 0)
	e.admin.Exec(ctx, `UPDATE items SET allow_negative_stock = true WHERE id = $1`, it)
	// Stok -3 (terjual sebelum barang datang) lalu beli 10 @ 800: stok sebelum ≤ 0 → HPP baru = HPP baris.
	err := db.WithTenant(ctx, e.app, e.tenant, func(tx pgx.Tx) error {
		_, err := stock.Apply(ctx, tx, stock.Movement{TenantID: e.tenant, OutletID: e.outlet, ItemID: it, Bucket: stock.BucketDisplay, Delta: decimal.NewFromInt(-3), RefType: stock.RefAdjustment})
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	p, _, err := e.svc.Create(ctx, a, key(), e.req(line(it, "10", "0", "800")))
	if err != nil {
		t.Fatal(err)
	}
	if p.Lines[0].StockBefore != "-3" || p.Lines[0].AvgAfter != "800.00" || e.bal(t, it, "display") != "7" {
		t.Errorf("stok minus: %+v stok=%s", p.Lines[0], e.bal(t, it, "display"))
	}
}

func TestConcurrentPurchasesKeepAverageConsistent(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	a := e.actor(e.outlet)
	it := e.item(t, "goods", "1000", 10) // 10 @ 1.000
	var wg sync.WaitGroup
	errs := make(chan error, 2)
	for _, price := range []string{"1200", "1400"} {
		wg.Add(1)
		go func(price string) {
			defer wg.Done()
			_, _, err := e.svc.Create(ctx, a, key(), e.req(line(it, "10", "0", price)))
			errs <- err
		}(price)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	// (10 × 1.000 + 10 × 1.200 + 10 × 1.400) ÷ 30 = 1.200, apa pun urutan commit-nya; dua nota → dua nomor.
	if avg, _ := e.cost(t, e.outlet, it); avg != "1200.00" || e.bal(t, it, "display") != "30" || e.count(t, "purchases") != 2 {
		t.Errorf("avg=%s stok=%s nota=%d", avg, e.bal(t, it, "display"), e.count(t, "purchases"))
	}
	var distinct int
	if err := e.admin.QueryRow(ctx, `SELECT count(DISTINCT doc_no) FROM purchases WHERE tenant_id = $1`, e.tenant).Scan(&distinct); err != nil || distinct != 2 {
		t.Errorf("nomor nota unik = %d %v", distinct, err)
	}

	// 12 pembelian bersamaan @ 1 unit → stok dan jumlah nota tepat, tanpa deadlock.
	it2 := e.item(t, "goods", "0", 0)
	errs = make(chan error, 12)
	for i := 0; i < 12; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _, err := e.svc.Create(ctx, a, key(), e.req(line(it2, "1", "0", "1000"), line(it, "0", "1", "1200")))
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
	if e.bal(t, it2, "display") != "12" || e.bal(t, it, "warehouse") != "12" || e.count(t, "purchases") != 14 {
		t.Errorf("setelah 12 nota bersamaan: %s %s nota=%d", e.bal(t, it2, "display"), e.bal(t, it, "warehouse"), e.count(t, "purchases"))
	}
}

func TestAccessAndTenantIsolation(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	a := e.actor(e.outlet)
	it := e.item(t, "goods", "0", 0)
	p, _, err := e.svc.Create(ctx, a, key(), e.req(line(it, "1", "0", "1000")))
	if err != nil {
		t.Fatal(err)
	}
	// Pemanggil yang hanya punya akses cabang 2 tidak boleh membaca nota cabang utama (termasuk lewat replay kunci).
	b := e.actor(e.outlet2)
	if _, err := e.svc.Get(ctx, b, p.ID); !errors.Is(err, ErrOutletForbidden) {
		t.Errorf("lintas cabang: %v", err)
	}
	// Tenant lain: tidak ada (RLS).
	c := authz.Actor{TenantID: e.other, UserID: uuid.New(), OutletID: e.outlet, Perms: authz.Permissions{All: true}, Outlets: map[uuid.UUID]bool{e.outlet: true}}
	if _, err := e.svc.Get(ctx, c, p.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("lintas tenant: %v", err)
	}
	// Pemasok tenant lain ditolak.
	foreign := uuid.New()
	e.admin.Exec(ctx, `INSERT INTO suppliers (id, tenant_id, name) VALUES ($1, $2, 'Asing')`, foreign, e.other)
	r := e.req(line(it, "1", "0", "1000"))
	r.SupplierID = foreign
	var fe FieldErrors
	if _, _, err := e.svc.Create(ctx, a, key(), r); !errors.As(err, &fe) || fe["supplier_id"] == "" {
		t.Errorf("pemasok asing: %v", err)
	}
}

func TestListFiltersAndSummary(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	a := e.actor(e.outlet)
	it := e.item(t, "goods", "0", 0)
	sup2 := uuid.New()
	e.admin.Exec(ctx, `INSERT INTO suppliers (id, tenant_id, name) VALUES ($1, $2, 'CV Maju_Jaya')`, sup2, e.tenant)
	mk := func(sup uuid.UUID, typ, inv, price string) {
		r := e.req(line(it, "1", "0", price))
		r.SupplierID, r.PaymentType, r.SupplierInvoiceNo = sup, typ, inv
		if _, _, err := e.svc.Create(ctx, a, key(), r); err != nil {
			t.Fatal(err)
		}
	}
	mk(e.supplier, "cash", "A-1", "1000")
	mk(e.supplier, "credit", "A-2", "2000")
	mk(sup2, "credit", "B-1", "4000")

	res, err := e.svc.List(ctx, a, ListParams{})
	if err != nil || len(res.Data) != 3 || res.Summary.Count != 3 || res.Summary.Total != "7000.00" || res.Summary.CreditTotal != "6000.00" {
		t.Fatalf("semua: %v %+v", err, res)
	}
	if res, _ = e.svc.List(ctx, a, ListParams{PaymentType: "credit"}); res.Summary.Count != 2 || res.Summary.Total != "6000.00" {
		t.Errorf("kredit: %+v", res.Summary)
	}
	if res, _ = e.svc.List(ctx, a, ListParams{SupplierID: &sup2}); res.Summary.Count != 1 || res.Data[0].SupplierName != "CV Maju_Jaya" {
		t.Errorf("pemasok: %+v", res)
	}
	if res, _ = e.svc.List(ctx, a, ListParams{Q: "a-2"}); res.Summary.Count != 1 || res.Data[0].SupplierInvoiceNo != "A-2" {
		t.Errorf("cari faktur: %+v", res)
	}
	// Underscore bukan wildcard: "u_J" harus cocok literal "Maju_Jaya" saja, bukan "SumbeR Rejeki".
	if res, _ = e.svc.List(ctx, a, ListParams{Q: "u_J"}); res.Summary.Count != 1 {
		t.Errorf("escape LIKE: %+v", res.Summary.Count)
	}
	if res, _ = e.svc.List(ctx, a, ListParams{Q: "%"}); res.Summary.Count != 0 {
		t.Errorf("persen literal: %d", res.Summary.Count)
	}
	// Cabang lain tidak melihat nota cabang utama.
	if res, _ = e.svc.List(ctx, e.actor(e.outlet2), ListParams{}); res.Summary.Count != 0 {
		t.Errorf("cabang 2: %d", res.Summary.Count)
	}
	var fe FieldErrors
	if _, err := e.svc.List(ctx, a, ListParams{From: "2020-01-01", To: "2026-12-31"}); !errors.As(err, &fe) {
		t.Errorf("rentang terlalu panjang: %v", err)
	}
	if _, err := e.svc.List(ctx, a, ListParams{PaymentType: "x"}); !errors.As(err, &fe) {
		t.Errorf("jenis tidak valid: %v", err)
	}
	items, err := e.svc.Items(ctx, a, "barang")
	if err != nil || len(items) != 1 || items[0].StockTotal != "3" {
		t.Errorf("pemilih barang: %v %+v", err, items)
	}
}

func TestQuoteMatchesCreateAndPersistsNothing(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	a := e.actor(e.outlet)
	it := e.item(t, "goods", "1000", 10)
	in := e.req(line(it, "6", "4", "1200", "10"))
	in.TaxPct = num("11")
	in.OtherCosts = []CostIn{{Name: "Ongkir", Amount: num("333")}}
	q, err := e.svc.Quote(ctx, a, in)
	if err != nil {
		t.Fatal(err)
	}
	if e.count(t, "purchases") != 0 || e.bal(t, it, "display") != "10" || e.count(t, "item_outlet_costs") != 0 {
		t.Fatal("quote tidak boleh menyimpan apa pun")
	}
	p, _, err := e.svc.Create(ctx, a, key(), in)
	if err != nil {
		t.Fatal(err)
	}
	l := p.Lines[0]
	if q.Total != p.Total || q.Tax != p.TaxAmount || q.Subtotal != p.Subtotal || q.Lines[0].UnitCost != l.UnitCost ||
		q.Lines[0].AvgAfter != l.AvgAfter || q.Lines[0].StockBefore != l.StockBefore || q.Lines[0].CostAlloc != l.CostAlloc {
		t.Errorf("quote ≠ simpan:\n%+v\n%+v", q, p)
	}
	// Quote tanpa pemasok/jenis bayar/baris tetap boleh (form yang belum lengkap).
	if q, err = e.svc.Quote(ctx, a, Request{}); err != nil || q.Total != "0.00" || len(q.Lines) != 0 {
		t.Errorf("quote kosong: %v %+v", err, q)
	}
	// Barang tidak sah dilaporkan seperti saat simpan.
	var fe FieldErrors
	if _, err := e.svc.Quote(ctx, a, Request{Lines: []LineIn{line(uuid.New(), "1", "0", "1000")}}); !errors.As(err, &fe) || fe["lines.0.item_id"] == "" {
		t.Errorf("quote barang asing: %v", err)
	}
}

// RLS gagal-tertutup + tabel baris/biaya/hutang append-only: aplikasi tidak bisa mengubah/menghapus dokumen yang sudah tersimpan.
func TestRLSAndImmutability(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	a := e.actor(e.outlet)
	it := e.item(t, "goods", "0", 0)
	r := e.req(line(it, "1", "0", "1000"))
	r.PaymentType = "credit"
	if _, _, err := e.svc.Create(ctx, a, key(), r); err != nil {
		t.Fatal(err)
	}
	// Tanpa konteks tenant (query "salah" tanpa filter) tidak ada baris yang terlihat.
	for _, tbl := range []string{"purchases", "purchase_lines", "purchase_costs", "payables", "purchase_counters", "item_outlet_costs"} {
		var n int
		if err := e.app.QueryRow(ctx, `SELECT count(*) FROM `+tbl).Scan(&n); err != nil || n != 0 {
			t.Errorf("%s tanpa tenant: n=%d err=%v", tbl, n, err)
		}
	}
	// Dengan tenant: UPDATE/DELETE pada tabel append-only ditolak oleh hak akses.
	err := db.WithTenant(ctx, e.app, e.tenant, func(tx pgx.Tx) error {
		for _, q := range []string{
			`UPDATE purchase_lines SET unit_price = 1`, `DELETE FROM purchase_lines`, `UPDATE payables SET amount = 1`, `DELETE FROM payables`,
			`DELETE FROM purchases`, `UPDATE purchase_costs SET amount = 1`,
		} {
			sp, _ := tx.Begin(ctx)
			if _, err := sp.Exec(ctx, q); err == nil {
				_ = sp.Rollback(ctx)
				t.Errorf("%q seharusnya ditolak", q)
				continue
			}
			_ = sp.Rollback(ctx)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
