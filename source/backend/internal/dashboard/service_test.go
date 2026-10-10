package dashboard_test

import (
	"context"
	"encoding/json"
	"os"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/shopspring/decimal"

	"aciraba/internal/approval"
	"aciraba/internal/authz"
	"aciraba/internal/dashboard"
	"aciraba/internal/payable"
	"aciraba/internal/platform/db"
	"aciraba/internal/receivable"
	"aciraba/internal/sales"
	"aciraba/internal/stock"
)

type env struct {
	app, admin *pgxpool.Pool
	tenant     uuid.UUID
	outlet     uuid.UUID
	unit       uuid.UUID
	owner      authz.Actor
	svc        *dashboard.Service
	sales      *sales.Service
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
	e := &env{app: app, admin: admin, tenant: uuid.New(), outlet: uuid.New(), unit: uuid.New()}
	e.svc = dashboard.NewService(app, receivable.NewService(app), payable.NewService(app)).WithStockTTL(0)
	e.sales = sales.NewService(app, approval.NewService(app, nil, "uji-rahasia-uji-rahasia-uji-rahasia"))
	for _, q := range []string{
		`INSERT INTO tenants (id, code, name) VALUES ($1::uuid, 'db-' || substr($1::uuid::text, 1, 8), 'UJI-DASH')`,
	} {
		e.exec(t, q, e.tenant)
	}
	e.exec(t, `INSERT INTO outlets (id, tenant_id, code, name) VALUES ($1, $2, 'main', 'Pusat')`, e.outlet, e.tenant)
	e.exec(t, `INSERT INTO units (id, tenant_id, name) VALUES ($1, $2, 'Pcs')`, e.unit, e.tenant)
	e.exec(t, `INSERT INTO payment_methods (tenant_id, name, kind, is_system) VALUES ($1, 'Tunai', 'cash', true)`, e.tenant)
	role, user := uuid.New(), uuid.New()
	e.exec(t, `INSERT INTO roles (id, tenant_id, name, permissions, is_system) VALUES ($1, $2, 'Owner', '{"*":true}'::jsonb, true)`, role, e.tenant)
	e.exec(t, `INSERT INTO users (id, tenant_id, role_id, email, name, password_hash) VALUES ($1, $2, $3, $4, 'Bu Owner', 'x')`, user, e.tenant, role, user.String()+"@example.test")
	e.owner = authz.Actor{TenantID: e.tenant, UserID: user, OutletID: e.outlet, Name: "Bu Owner", Perms: authz.ParseStored([]byte(`{"*":true}`)),
		Outlets: map[uuid.UUID]bool{e.outlet: true}}
	t.Cleanup(func() {
		for _, tbl := range []string{"audit_log", "sale_payments", "sale_lines", "sales", "sale_counters", "payment_methods", "stock_movements", "stock_balances",
			"items", "units", "user_outlets", "users", "roles", "outlets"} {
			_, _ = admin.Exec(ctx, `DELETE FROM `+tbl+` WHERE tenant_id = $1`, e.tenant)
		}
		_, _ = admin.Exec(ctx, `DELETE FROM tenants WHERE id = $1`, e.tenant)
	})
	return e
}

func (e *env) exec(t *testing.T, sql string, args ...any) {
	t.Helper()
	if _, err := e.admin.Exec(context.Background(), sql, args...); err != nil {
		t.Fatal(err)
	}
}

func (e *env) item(t *testing.T, stockQty int, negative bool) uuid.UUID {
	t.Helper()
	id := uuid.New()
	e.exec(t, `INSERT INTO items (id, tenant_id, sku, name, unit_id, kind, sell_price, avg_cost, last_cost, allow_negative_stock)
		VALUES ($1, $2, $3, $4, $5, 'goods', 10000, 1000, 1000, $6)`, id, e.tenant, "S-"+id.String()[:8], "Barang "+id.String()[:4], e.unit, negative)
	if stockQty > 0 {
		err := db.WithTenant(context.Background(), e.app, e.tenant, func(tx pgx.Tx) error {
			_, err := stock.Apply(context.Background(), tx, stock.Movement{TenantID: e.tenant, OutletID: e.outlet, ItemID: id, Bucket: stock.BucketDisplay,
				Delta: decimal.NewFromInt(int64(stockQty)), RefType: stock.RefOpening})
			return err
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	return id
}

func (e *env) sell(t *testing.T, item uuid.UUID, qty, pay string) {
	t.Helper()
	_, _, err := e.sales.Create(context.Background(), e.owner, "k-"+uuid.NewString(), sales.Request{
		Lines:    []sales.LineIn{{ItemID: item, Qty: json.Number(qty)}},
		Payments: []sales.PaymentIn{{Method: "cash", Amount: json.Number(pay)}},
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestOverview(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()

	// Toko kosong: tidak ada nota, semua pemeriksaan beres.
	o, err := e.svc.Overview(ctx, e.owner, false)
	if err != nil {
		t.Fatal(err)
	}
	if o.Verdict != "ok" || o.Sales == nil || o.Sales.Today.Count != 0 || len(o.Sales.Trend) != 28 || len(o.Sales.Hourly) != 24 {
		t.Fatalf("toko kosong: %+v", o)
	}

	good := e.item(t, 10, false)
	e.sell(t, good, "2", "20000")
	o, err = e.svc.Overview(ctx, e.owner, false)
	if err != nil {
		t.Fatal(err)
	}
	s := o.Sales
	if s.Today.Total != "20000.00" || s.Today.Count != 1 || s.Today.Average != "20000.00" || s.Week != "20000.00" || s.Month != "20000.00" {
		t.Fatalf("penjualan hari ini: %+v", s.Today)
	}
	if s.Today.Profit == nil || *s.Today.Profit != "18000.00" {
		t.Fatalf("laba: %v", s.Today.Profit)
	}
	if s.Trend[27].Total != "20000.00" || s.Trend[26].Total != "0.00" {
		t.Fatalf("tren: %+v", s.Trend[26:])
	}
	if len(s.TopItems) != 1 || s.TopItems[0].Revenue != "20000.00" || s.TopItems[0].Qty != "2.000" {
		t.Fatalf("terlaris: %+v", s.TopItems)
	}
	if s.Today.PeakHour == nil {
		t.Fatal("jam tersibuk kosong")
	}
	if o.Stock == nil || o.Stock.Tracked != 1 || o.Stock.Negative != 0 || o.Stock.Value == nil || *o.Stock.Value != "8000.00" {
		t.Fatalf("stok: %+v", o.Stock)
	}
	if o.Verdict != "ok" {
		t.Fatalf("seharusnya baik: %s %+v", o.Verdict, o.Checks)
	}

	// Stok minus → peringatan.
	neg := e.item(t, 1, true)
	e.sell(t, neg, "3", "30000")
	if o, err = e.svc.Overview(ctx, e.owner, false); err != nil {
		t.Fatal(err)
	}
	if o.Stock.Negative != 1 || len(o.Stock.Negatives) != 1 || o.Verdict != "warn" {
		t.Fatalf("stok minus: %+v verdict=%s", o.Stock, o.Verdict)
	}

	// Stok menipis: batas minimum 5, saldo 3 → peringatan; tanpa batas di barang lain → tidak dihitung.
	if o.Stock.Monitored != 0 || o.Verdict != "warn" {
		t.Fatalf("belum ada batas minimum: %+v", o.Stock)
	}
	low := e.item(t, 3, false)
	e.exec(t, `UPDATE items SET min_stock = 5 WHERE tenant_id = $1 AND id = $2`, e.tenant, low)
	if o, err = e.svc.Overview(ctx, e.owner, false); err != nil {
		t.Fatal(err)
	}
	if o.Stock.Monitored != 1 || o.Stock.Low != 1 || len(o.Stock.Lows) != 1 || o.Stock.Lows[0].Min != "5.000" || o.Stock.Lows[0].Qty != "3.000" {
		t.Fatalf("stok menipis: %+v", o.Stock)
	}
	hasLow := false
	for _, c := range o.Checks {
		hasLow = hasLow || (c.Code == "stock_low" && c.Level == "warn")
	}
	if !hasLow {
		t.Fatalf("pemeriksaan stock_low tidak ada: %+v", o.Checks)
	}
	e.exec(t, `UPDATE items SET min_stock = 2 WHERE tenant_id = $1 AND id = $2`, e.tenant, low) // saldo di atas batas
	if o, err = e.svc.Overview(ctx, e.owner, false); err != nil || o.Stock.Low != 0 {
		t.Fatalf("di atas batas: %v %+v", err, o.Stock)
	}

	// Bulan ini vs bulan lalu: nota hari ini ikut bulan ini; bulan lalu kosong → tanpa persen.
	mc := o.Sales.MonthCompare
	if mc.This != o.Sales.Month || mc.Last != "0.00" || mc.Pct != nil || o.Sales.Month != "50000.00" {
		t.Fatalf("bulan ini: %+v month=%s", mc, o.Sales.Month)
	}
	e.exec(t, `UPDATE sales SET created_at = created_at - interval '1 month' WHERE tenant_id = $1 AND doc_no = (SELECT doc_no FROM sales WHERE tenant_id = $1 ORDER BY created_at LIMIT 1)`, e.tenant)
	if o, err = e.svc.Overview(ctx, e.owner, false); err != nil {
		t.Fatal(err)
	}
	// Nota 20.000 dipindah ke bulan lalu pada tanggal/jam yang sama → masuk pembanding.
	if mc = o.Sales.MonthCompare; mc.Last != "20000.00" || mc.This != "30000.00" || mc.Pct == nil || *mc.Pct != 50 {
		t.Fatalf("pembanding bulan lalu: %+v", mc)
	}

	// Semua cabang: rincian per cabang (cabang kedua kosong), hanya untuk cakupan semua.
	out2 := uuid.New()
	e.exec(t, `INSERT INTO outlets (id, tenant_id, code, name) VALUES ($1, $2, 'cab2', 'Cabang Dua')`, out2, e.tenant)
	both := e.owner
	both.Outlets = map[uuid.UUID]bool{e.outlet: true, out2: true}
	if o, err = e.svc.Overview(ctx, both, false); err != nil || len(o.ByOutlet) != 0 || o.Scope != "outlet" {
		t.Fatalf("cakupan satu cabang tidak boleh memuat rincian: %v %+v", err, o.ByOutlet)
	}
	if o, err = e.svc.Overview(ctx, both, true); err != nil || o.Scope != "all" || len(o.ByOutlet) != 2 {
		t.Fatalf("semua cabang: %v %+v", err, o.ByOutlet)
	}
	for _, r := range o.ByOutlet {
		switch r.Code {
		case "main":
			if r.Count != 1 || r.Today != "30000.00" || r.Negative == nil || *r.Negative != 1 {
				t.Fatalf("cabang utama: %+v", r)
			}
		case "cab2":
			if r.Count != 0 || r.Today != "0.00" || *r.Negative != 0 {
				t.Fatalf("cabang dua: %+v", r)
			}
		default:
			t.Fatalf("cabang asing: %+v", r)
		}
	}

	// Pengguna tanpa izin HPP/stok: bagian itu tidak dikirim, laba tidak bocor.
	lim := e.owner
	lim.Perms = authz.ParseStored([]byte(`{"sales_list":["view"]}`))
	if o, err = e.svc.Overview(ctx, lim, false); err != nil {
		t.Fatal(err)
	}
	if o.Stock != nil || o.Receivable != nil || o.Payable != nil || o.Shifts != nil || o.Sales == nil || o.Sales.Today.Profit != nil {
		t.Fatalf("izin terbatas: %+v", o)
	}
	none := e.owner
	none.Perms = authz.ParseStored([]byte(`{}`))
	if o, err = e.svc.Overview(ctx, none, false); err != nil || o.Sales != nil || o.Stock != nil {
		t.Fatalf("tanpa izin: %v %+v", err, o)
	}

	// Tenant lain tidak ikut terhitung.
	other := e.owner
	other.TenantID, other.OutletID = uuid.New(), uuid.New()
	if _, err = e.svc.Overview(ctx, other, false); err == nil {
		t.Fatal("outlet asing seharusnya galat")
	}
}
