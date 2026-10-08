package stock

import (
	"context"
	"errors"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/shopspring/decimal"

	"aciraba/internal/platform/db"
)

type env struct {
	app, admin *pgxpool.Pool
	tenant     uuid.UUID
	other      uuid.UUID // tenant lain (isolasi)
	outlet     uuid.UUID
	outlet2    uuid.UUID
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

	e := &env{app: app, admin: admin, tenant: uuid.New(), other: uuid.New(), outlet: uuid.New(), outlet2: uuid.New()}
	for _, s := range []struct {
		sql  string
		args []any
	}{
		{`INSERT INTO tenants (id, code, name) VALUES ($1, $2, 'UJI-STOK')`, []any{e.tenant, "st-" + e.tenant.String()[:8]}},
		{`INSERT INTO tenants (id, code, name) VALUES ($1, $2, 'UJI-STOK-2')`, []any{e.other, "st-" + e.other.String()[:8]}},
		{`INSERT INTO outlets (id, tenant_id, code, name) VALUES ($1, $2, 'main', 'Pusat')`, []any{e.outlet, e.tenant}},
		{`INSERT INTO outlets (id, tenant_id, code, name) VALUES ($1, $2, 'cab2', 'Cabang 2')`, []any{e.outlet2, e.tenant}},
	} {
		if _, err := admin.Exec(ctx, s.sql, s.args...); err != nil {
			t.Fatal(err)
		}
	}
	t.Cleanup(func() {
		for _, tid := range []uuid.UUID{e.tenant, e.other} {
			for _, tbl := range []string{"stock_movements", "stock_balances", "stock_conversions", "stock_conversion_counters", "items", "units", "users", "roles", "outlets"} {
				_, _ = admin.Exec(ctx, `DELETE FROM `+tbl+` WHERE tenant_id = $1`, tid)
			}
			_, _ = admin.Exec(ctx, `DELETE FROM tenants WHERE id = $1`, tid)
		}
	})
	return e
}

// item membuat satuan + item di tenant tertentu.
func (e *env) item(t *testing.T, tenant uuid.UUID, kind string, allowNeg bool) uuid.UUID {
	t.Helper()
	ctx := context.Background()
	unit, id := uuid.New(), uuid.New()
	if _, err := e.admin.Exec(ctx, `INSERT INTO units (id, tenant_id, name) VALUES ($1, $2, $3)`, unit, tenant, "u-"+unit.String()[:8]); err != nil {
		t.Fatal(err)
	}
	if _, err := e.admin.Exec(ctx, `INSERT INTO items (id, tenant_id, sku, name, unit_id, kind, allow_negative_stock) VALUES ($1, $2, $3, 'Barang', $4, $5, $6)`,
		id, tenant, "S-"+id.String()[:8], unit, kind, allowNeg); err != nil {
		t.Fatal(err)
	}
	return id
}

// tx menjalankan fn dalam transaksi tenant (commit bila nil, rollback bila error), seperti use-case sungguhan.
func (e *env) tx(fn func(ctx context.Context, tx pgx.Tx) error) error {
	ctx := context.Background()
	return db.WithTenant(ctx, e.app, e.tenant, func(tx pgx.Tx) error { return fn(ctx, tx) })
}

func (e *env) mv(item uuid.UUID, bucket, delta, ref string) Movement {
	return Movement{TenantID: e.tenant, OutletID: e.outlet, ItemID: item, Bucket: bucket, Delta: decimal.RequireFromString(delta), RefType: ref}
}

func (e *env) apply(t *testing.T, m Movement) (Result, error) {
	t.Helper()
	var r Result
	err := e.tx(func(ctx context.Context, tx pgx.Tx) (err error) {
		r, err = Apply(ctx, tx, m)
		return
	})
	return r, err
}

func (e *env) balance(t *testing.T, item uuid.UUID, bucket string) string {
	t.Helper()
	var q decimal.Decimal
	err := e.admin.QueryRow(context.Background(), `SELECT coalesce(sum(qty), 0) FROM stock_balances WHERE tenant_id=$1 AND outlet_id=$2 AND item_id=$3 AND bucket=$4`,
		e.tenant, e.outlet, item, bucket).Scan(&q)
	if err != nil {
		t.Fatal(err)
	}
	return q.String()
}

func (e *env) movements(t *testing.T, item uuid.UUID) int {
	t.Helper()
	var n int
	if err := e.admin.QueryRow(context.Background(), `SELECT count(*) FROM stock_movements WHERE tenant_id=$1 AND item_id=$2`, e.tenant, item).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func TestValidate(t *testing.T) {
	ok := Movement{TenantID: uuid.New(), OutletID: uuid.New(), ItemID: uuid.New(), Bucket: BucketDisplay, Delta: decimal.RequireFromString("1.5"), RefType: RefOpening}
	if err := ok.validate(); err != nil {
		t.Fatal(err)
	}
	bad := map[string]func(m *Movement){
		"tenant kosong":   func(m *Movement) { m.TenantID = uuid.Nil },
		"bucket asing":    func(m *Movement) { m.Bucket = "toko" },
		"ref asing":       func(m *Movement) { m.RefType = "X" },
		"qty nol":         func(m *Movement) { m.Delta = decimal.Zero },
		"4 desimal":       func(m *Movement) { m.Delta = decimal.RequireFromString("1.0001") },
		"terlalu besar":   func(m *Movement) { m.Delta = decimal.New(1, 12) },
		"catatan panjang": func(m *Movement) { m.Note = strings.Repeat("a", 501) },
	}
	for name, mut := range bad {
		m := ok
		mut(&m)
		if err := m.validate(); !errors.Is(err, ErrInvalid) {
			t.Errorf("%s: harus ditolak, dapat %v", name, err)
		}
	}
}

func TestInOutAndBuckets(t *testing.T) {
	e := newEnv(t)
	item := e.item(t, e.tenant, "goods", false)

	r, err := e.apply(t, e.mv(item, BucketDisplay, "10", RefOpening))
	if err != nil || r.BalanceAfter.String() != "10" || r.MovementID == 0 {
		t.Fatalf("saldo awal: %+v %v", r, err)
	}
	if r, err = e.apply(t, e.mv(item, BucketDisplay, "-3.5", RefSale)); err != nil || r.BalanceAfter.String() != "6.5" {
		t.Fatalf("jual: %+v %v", r, err)
	}
	// Bucket independen: gudang kosong, tidak bisa dikurangi walau display ada.
	_, err = e.apply(t, e.mv(item, BucketWarehouse, "-1", RefTransferOut))
	var ins *InsufficientError
	if !errors.As(err, &ins) || !errors.Is(err, ErrInsufficient) || ins.ItemID != item {
		t.Fatalf("gudang kosong harus ditolak: %v", err)
	}
	// Kurang tepat sampai nol boleh; satu pecahan lebih tidak.
	if _, err = e.apply(t, e.mv(item, BucketDisplay, "-6.501", RefSale)); !errors.Is(err, ErrInsufficient) {
		t.Fatalf("melebihi saldo harus ditolak: %v", err)
	}
	if r, err = e.apply(t, e.mv(item, BucketDisplay, "-6.5", RefSale)); err != nil || !r.BalanceAfter.IsZero() {
		t.Fatalf("habiskan: %+v %v", r, err)
	}
	if got := e.balance(t, item, BucketDisplay); got != "0" {
		t.Errorf("saldo = %s", got)
	}
	// Yang ditolak tidak meninggalkan movement.
	if n := e.movements(t, item); n != 3 {
		t.Errorf("movement = %d, harus 3 (OPENING + 2 SALE)", n)
	}
	// Saldo pertama kali langsung negatif ditolak (tanpa baris saldo sama sekali).
	fresh := e.item(t, e.tenant, "goods", false)
	if _, err = e.apply(t, e.mv(fresh, BucketDisplay, "-1", RefSale)); !errors.Is(err, ErrInsufficient) {
		t.Fatalf("stok belum ada harus ditolak: %v", err)
	}
	if e.balance(t, fresh, BucketDisplay) != "0" || e.movements(t, fresh) != 0 {
		t.Error("penolakan tidak boleh membuat baris saldo/movement")
	}
}

func TestNegativeStockOnlyDisplay(t *testing.T) {
	e := newEnv(t)
	item := e.item(t, e.tenant, "goods", true)
	r, err := e.apply(t, e.mv(item, BucketDisplay, "-2", RefSale))
	if err != nil || r.BalanceAfter.String() != "-2" {
		t.Fatalf("display boleh minus: %+v %v", r, err)
	}
	if r, err = e.apply(t, e.mv(item, BucketDisplay, "5", RefOpening)); err != nil || r.BalanceAfter.String() != "3" {
		t.Fatalf("pulih dari minus: %+v %v", r, err)
	}
	for _, b := range []string{BucketWarehouse, BucketReturns} {
		if _, err := e.apply(t, e.mv(item, b, "-1", RefTransferOut)); !errors.Is(err, ErrInsufficient) {
			t.Errorf("bucket %s tidak boleh minus: %v", b, err)
		}
	}
}

func TestItemRules(t *testing.T) {
	e := newEnv(t)
	svc := e.item(t, e.tenant, "service", false)
	if _, err := e.apply(t, e.mv(svc, BucketDisplay, "1", RefOpening)); !errors.Is(err, ErrNotStocked) {
		t.Errorf("jasa tidak punya stok: %v", err)
	}
	if _, err := e.apply(t, e.mv(uuid.New(), BucketDisplay, "1", RefOpening)); !errors.Is(err, ErrItemNotFound) {
		t.Errorf("item tak ada: %v", err)
	}
	// Item milik tenant lain tidak terlihat (RLS) → tidak ditemukan, bukan tertulis ke tenant lain.
	foreign := e.item(t, e.other, "goods", false)
	if _, err := e.apply(t, e.mv(foreign, BucketDisplay, "1", RefOpening)); !errors.Is(err, ErrItemNotFound) {
		t.Errorf("item tenant lain: %v", err)
	}
	// Outlet yang tidak dikenal ditolak FK komposit.
	other := e.item(t, e.tenant, "goods", false)
	m := e.mv(other, BucketDisplay, "1", RefOpening)
	m.OutletID = uuid.New()
	if _, err := e.apply(t, m); err == nil {
		t.Error("outlet tak dikenal harus ditolak FK")
	}
}

func TestLedgerAppendOnly(t *testing.T) {
	e := newEnv(t)
	item := e.item(t, e.tenant, "goods", false)
	if _, err := e.apply(t, e.mv(item, BucketDisplay, "4", RefOpening)); err != nil {
		t.Fatal(err)
	}
	for _, q := range []string{`UPDATE stock_movements SET qty_delta = 99`, `DELETE FROM stock_movements`} {
		err := e.tx(func(ctx context.Context, tx pgx.Tx) error {
			_, err := tx.Exec(ctx, q)
			return err
		})
		if err == nil {
			t.Errorf("%q harus ditolak untuk role aplikasi", q)
		}
	}
	if e.movements(t, item) != 1 {
		t.Error("ledger berubah")
	}
}

// Dua puluh kasir menjual satu per satu dari stok 10: tepat 10 berhasil, saldo 0, tidak ada yang minus.
func TestConcurrentSalesNeverOversell(t *testing.T) {
	e := newEnv(t)
	item := e.item(t, e.tenant, "goods", false)
	if _, err := e.apply(t, e.mv(item, BucketDisplay, "10", RefOpening)); err != nil {
		t.Fatal(err)
	}
	var ok, short atomic.Int32
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := e.apply(t, e.mv(item, BucketDisplay, "-1", RefSale))
			switch {
			case err == nil:
				ok.Add(1)
			case errors.Is(err, ErrInsufficient):
				short.Add(1)
			default:
				t.Errorf("galat tak terduga: %v", err)
			}
		}()
	}
	wg.Wait()
	if ok.Load() != 10 || short.Load() != 10 {
		t.Fatalf("berhasil=%d kurang=%d, harus 10/10", ok.Load(), short.Load())
	}
	if got := e.balance(t, item, BucketDisplay); got != "0" {
		t.Errorf("saldo akhir = %s", got)
	}
	// Saldo berjalan di ledger konsisten: tidak pernah negatif.
	var minAfter decimal.Decimal
	if err := e.admin.QueryRow(context.Background(), `SELECT min(balance_after) FROM stock_movements WHERE item_id=$1`, item).Scan(&minAfter); err != nil || minAfter.IsNegative() {
		t.Errorf("balance_after minimum = %v %v", minAfter, err)
	}
}

// Item boleh minus: semua penjualan bersamaan berhasil dan jumlahnya tepat.
func TestConcurrentNegativeAllowed(t *testing.T) {
	e := newEnv(t)
	item := e.item(t, e.tenant, "goods", true)
	var wg sync.WaitGroup
	for i := 0; i < 15; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := e.apply(t, e.mv(item, BucketDisplay, "-2", RefSale)); err != nil {
				t.Errorf("penjualan: %v", err)
			}
		}()
	}
	wg.Wait()
	if got := e.balance(t, item, BucketDisplay); got != "-30" {
		t.Errorf("saldo = %s, harus -30", got)
	}
}

// Dua nota yang menyentuh dua barang yang sama dengan urutan baris berlawanan: tanpa urutan kunci yang
// deterministik ini deadlock. Selain itu nota gagal harus membatalkan SEMUA barisnya.
func TestApplyAllNoDeadlockAndAtomic(t *testing.T) {
	e := newEnv(t)
	a, b := e.item(t, e.tenant, "goods", false), e.item(t, e.tenant, "goods", false)
	for _, it := range []uuid.UUID{a, b} {
		if _, err := e.apply(t, e.mv(it, BucketDisplay, "1000", RefOpening)); err != nil {
			t.Fatal(err)
		}
	}
	var wg sync.WaitGroup
	for i := 0; i < 30; i++ {
		first, second := a, b
		if i%2 == 1 {
			first, second = b, a
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			err := e.tx(func(ctx context.Context, tx pgx.Tx) error {
				_, err := ApplyAll(ctx, tx, []Movement{e.mv(first, BucketDisplay, "-1", RefSale), e.mv(second, BucketDisplay, "-1", RefSale)})
				return err
			})
			if err != nil {
				t.Errorf("nota: %v", err)
			}
		}()
	}
	wg.Wait()
	if e.balance(t, a, BucketDisplay) != "970" || e.balance(t, b, BucketDisplay) != "970" {
		t.Errorf("saldo a=%s b=%s, harus 970/970", e.balance(t, a, BucketDisplay), e.balance(t, b, BucketDisplay))
	}

	// Baris kedua gagal (stok kurang) → baris pertama ikut batal.
	err := e.tx(func(ctx context.Context, tx pgx.Tx) error {
		_, err := ApplyAll(ctx, tx, []Movement{e.mv(a, BucketDisplay, "-5", RefSale), e.mv(b, BucketDisplay, "-99999", RefSale)})
		return err
	})
	if !errors.Is(err, ErrInsufficient) {
		t.Fatalf("harus ditolak: %v", err)
	}
	if e.balance(t, a, BucketDisplay) != "970" || e.movements(t, a) != 1+30 {
		t.Errorf("nota gagal tidak boleh meninggalkan jejak: saldo=%s movement=%d", e.balance(t, a, BucketDisplay), e.movements(t, a))
	}
}

func TestBalances(t *testing.T) {
	e := newEnv(t)
	item := e.item(t, e.tenant, "goods", false)
	for _, m := range []Movement{e.mv(item, BucketDisplay, "3", RefOpening), e.mv(item, BucketWarehouse, "7.25", RefOpening)} {
		if _, err := e.apply(t, m); err != nil {
			t.Fatal(err)
		}
	}
	var got map[string]decimal.Decimal
	err := e.tx(func(ctx context.Context, tx pgx.Tx) (err error) {
		got, err = Balances(ctx, tx, e.tenant, e.outlet, item)
		return
	})
	if err != nil || len(got) != 2 || got[BucketDisplay].String() != "3" || got[BucketWarehouse].String() != "7.25" {
		t.Fatalf("saldo: %v %v", got, err)
	}
}
