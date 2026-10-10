package shift_test

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/shopspring/decimal"

	"aciraba/internal/approval"
	"aciraba/internal/authz"
	pauth "aciraba/internal/platform/auth"
	"aciraba/internal/platform/db"
	"aciraba/internal/sales"
	"aciraba/internal/shift"
	"aciraba/internal/stock"
)

const testPassword = "Rahasia-Uji-123"

type env struct {
	app, admin   *pgxpool.Pool
	appr         *approval.Service
	svc          *shift.Service
	sales        *sales.Service
	tenant       uuid.UUID
	other        uuid.UUID
	outlet       uuid.UUID
	unit         uuid.UUID
	cashier      authz.Actor
	cashID, trID uuid.UUID
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
	appr := approval.NewService(app, nil, "uji-rahasia-uji-rahasia-uji-rahasia")
	e := &env{app: app, admin: admin, appr: appr, svc: shift.NewService(app, appr), sales: sales.NewService(app, appr).WithShiftGuard(shift.Guard),
		tenant: uuid.New(), other: uuid.New(), outlet: uuid.New(), unit: uuid.New()}
	for _, s := range []struct {
		sql  string
		args []any
	}{
		{`INSERT INTO tenants (id, code, name) VALUES ($1, $2, 'UJI-SHIFT')`, []any{e.tenant, "sh-" + e.tenant.String()[:8]}},
		{`INSERT INTO tenants (id, code, name) VALUES ($1, $2, 'UJI-SHIFT-2')`, []any{e.other, "sh-" + e.other.String()[:8]}},
		{`INSERT INTO outlets (id, tenant_id, code, name) VALUES ($1, $2, 'main', 'Pusat')`, []any{e.outlet, e.tenant}},
		{`INSERT INTO units (id, tenant_id, name) VALUES ($1, $2, 'Pcs')`, []any{e.unit, e.tenant}},
		{`INSERT INTO payment_methods (tenant_id, name, kind, is_system) VALUES ($1, 'Tunai', 'cash', true), ($1, 'Transfer', 'transfer', false),
			($1, 'QRIS', 'ewallet', false), ($1, 'Deposit Member', 'deposit', true), ($1, 'Kredit Pemasok', 'supplier_credit', true)`, []any{e.tenant}},
	} {
		if _, err := admin.Exec(ctx, s.sql, s.args...); err != nil {
			t.Fatal(err)
		}
	}
	t.Cleanup(func() {
		for _, tid := range []uuid.UUID{e.tenant, e.other} {
			for _, tbl := range []string{"audit_log", "cash_shift_flows", "cash_shift_counts", "cash_shifts", "cash_shift_counters", "sale_payments", "sale_lines",
				"sales", "sale_counters", "payment_methods", "stock_movements", "stock_balances", "items", "units", "user_outlets", "users", "roles", "outlets"} {
				_, _ = admin.Exec(ctx, `DELETE FROM `+tbl+` WHERE tenant_id = $1`, tid)
			}
			_, _ = admin.Exec(ctx, `DELETE FROM tenants WHERE id = $1`, tid)
		}
	})
	if err := admin.QueryRow(ctx, `SELECT id FROM payment_methods WHERE tenant_id = $1 AND kind = 'cash'`, e.tenant).Scan(&e.cashID); err != nil {
		t.Fatal(err)
	}
	if err := admin.QueryRow(ctx, `SELECT id FROM payment_methods WHERE tenant_id = $1 AND kind = 'transfer'`, e.tenant).Scan(&e.trID); err != nil {
		t.Fatal(err)
	}
	e.cashier = e.person(t, "Kasir", `{"sales_orders":["view","create"]}`)
	return e
}

func (e *env) exec(t *testing.T, sql string, args ...any) {
	t.Helper()
	if _, err := e.admin.Exec(context.Background(), sql, args...); err != nil {
		t.Fatal(err)
	}
}

func (e *env) person(t *testing.T, name, perms string) authz.Actor {
	t.Helper()
	role, user := uuid.New(), uuid.New()
	hash, err := pauth.HashPassword(testPassword)
	if err != nil {
		t.Fatal(err)
	}
	e.exec(t, "INSERT INTO roles (id, tenant_id, name, permissions) VALUES ($1, $2, $3, $4::jsonb)", role, e.tenant, name+"-"+role.String()[:6], perms)
	e.exec(t, "INSERT INTO users (id, tenant_id, role_id, email, name, password_hash) VALUES ($1, $2, $3, $4, $5, $6)", user, e.tenant, role, name+"-"+user.String()+"@example.test", name, hash)
	e.exec(t, "INSERT INTO user_outlets (tenant_id, user_id, outlet_id) VALUES ($1, $2, $3)", e.tenant, user, e.outlet)
	return authz.Actor{TenantID: e.tenant, UserID: user, OutletID: e.outlet, Name: name, Perms: authz.ParseStored([]byte(perms)),
		Outlets: map[uuid.UUID]bool{e.outlet: true}}
}

func (e *env) item(t *testing.T, price string, qty int) uuid.UUID {
	t.Helper()
	ctx := context.Background()
	id := uuid.New()
	e.exec(t, `INSERT INTO items (id, tenant_id, sku, name, unit_id, kind, sell_price, avg_cost, last_cost)
		VALUES ($1, $2, $3, $4, $5, 'goods', $6, 1000, 1000)`, id, e.tenant, "S-"+id.String()[:8], "Barang "+id.String()[:4], e.unit, price)
	err := db.WithTenant(ctx, e.app, e.tenant, func(tx pgx.Tx) error {
		_, err := stock.Apply(ctx, tx, stock.Movement{TenantID: e.tenant, OutletID: e.outlet, ItemID: id, Bucket: stock.BucketDisplay,
			Delta: decimal.NewFromInt(int64(qty)), RefType: stock.RefOpening})
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func (e *env) sell(a authz.Actor, item uuid.UUID, qty, method, amount string) (sales.Sale, error) {
	s, _, err := e.sales.Create(context.Background(), a, "k-"+uuid.NewString(), sales.Request{
		Lines:    []sales.LineIn{{ItemID: item, Qty: json.Number(qty)}},
		Payments: []sales.PaymentIn{{Method: method, Amount: json.Number(amount)}},
	})
	return s, err
}

func countOf(sh shift.Shift, method uuid.UUID) *shift.Count {
	for i := range sh.Counts {
		if sh.Counts[i].MethodID == method {
			return &sh.Counts[i]
		}
	}
	return nil
}

func key() string { return "c-" + uuid.NewString() }

func TestOpenRecapClose(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	it := e.item(t, "10000", 100)

	if _, err := e.sell(e.cashier, it, "1", "cash", "10000"); !errors.Is(err, shift.ErrRequired) {
		t.Fatalf("tanpa shift harus ditolak: %v", err)
	}
	if cur, err := e.svc.Current(ctx, e.cashier); err != nil || cur != nil {
		t.Fatalf("belum ada shift: %v %v", cur, err)
	}
	if _, err := e.svc.Open(ctx, e.cashier, shift.OpenInput{OpeningCash: "-1"}); err == nil {
		t.Fatal("modal negatif harus ditolak")
	}
	sh, err := e.svc.Open(ctx, e.cashier, shift.OpenInput{OpeningCash: "100000"})
	if err != nil || sh.Status != "open" || sh.OpeningCash != "100000.00" {
		t.Fatalf("buka: %v %+v", err, sh)
	}
	if _, err := e.svc.Open(ctx, e.cashier, shift.OpenInput{OpeningCash: "5"}); !errors.Is(err, shift.ErrAlreadyOpen) {
		t.Fatalf("buka ganda: %v", err)
	}
	// 2 × 10.000 dibayar 50.000 tunai (kembali 30.000) + 3 × 10.000 transfer.
	if _, err := e.sell(e.cashier, it, "2", "cash", "50000"); err != nil {
		t.Fatal(err)
	}
	if _, err := e.sell(e.cashier, it, "3", "transfer", "30000"); err != nil {
		t.Fatal(err)
	}
	// Kasir lain tanpa shift tetap ditolak; nota kasir lain tidak masuk shift ini.
	other := e.person(t, "Kasir2", `{"sales_orders":["view","create"]}`)
	if _, err := e.sell(other, it, "1", "cash", "10000"); !errors.Is(err, shift.ErrRequired) {
		t.Fatalf("kasir lain tanpa shift: %v", err)
	}

	cur, err := e.svc.Current(ctx, e.cashier)
	if err != nil || cur == nil {
		t.Fatalf("current: %v", err)
	}
	if c := countOf(*cur, e.cashID); c == nil || c.Expected != "120000.00" || c.Sales != "20000.00" || c.Opening != "100000.00" {
		t.Fatalf("tunai: %+v", c)
	}
	if c := countOf(*cur, e.trID); c == nil || c.Expected != "30000.00" {
		t.Fatalf("transfer: %+v", c)
	}
	if cur.SaleCount != 2 || cur.SalesTotal != "50000.00" || cur.ExpectedTotal != "150000.00" {
		t.Fatalf("ringkasan: %+v", cur)
	}

	// Metode di rekap wajib diisi semua.
	_, err = e.svc.Close(ctx, e.cashier, sh.ID, key(), shift.CloseInput{Counts: []shift.CountIn{{MethodID: e.cashID, Counted: "120000"}}})
	if !errors.Is(err, shift.ErrRecapChanged) {
		t.Fatalf("metode kurang: %v", err)
	}
	k := key()
	in := shift.CloseInput{Counts: []shift.CountIn{{MethodID: e.cashID, Counted: "120000"}, {MethodID: e.trID, Counted: "30000"}}}
	closed, err := e.svc.Close(ctx, e.cashier, sh.ID, k, in)
	if err != nil || closed.Status != "closed" || *closed.DiffTotal != "0.00" || *closed.CountedTotal != "150000.00" || closed.ApprovedByName != "" {
		t.Fatalf("tutup: %v %+v", err, closed)
	}
	// Kirim ulang dengan kunci sama = hasil sama; kunci lain = sudah ditutup.
	if again, err := e.svc.Close(ctx, e.cashier, sh.ID, k, in); err != nil || again.ClosedAt == nil || !again.ClosedAt.Equal(*closed.ClosedAt) {
		t.Fatalf("replay: %v", err)
	}
	if _, err := e.svc.Close(ctx, e.cashier, sh.ID, key(), in); !errors.Is(err, shift.ErrNotOpen) {
		t.Fatalf("tutup dua kali: %v", err)
	}
	if _, err := e.sell(e.cashier, it, "1", "cash", "10000"); !errors.Is(err, shift.ErrRequired) {
		t.Fatalf("setelah tutup: %v", err)
	}

	// Rekap beku: nota yang dibuat langsung di DB dengan waktu dalam rentang shift tidak mengubahnya.
	e.exec(t, `UPDATE sales SET total = total WHERE tenant_id = $1`, e.tenant)
	got, err := e.svc.Get(ctx, e.cashier, sh.ID)
	if err != nil || got.ExpectedTotal != "150000.00" || len(got.Counts) != 2 || *countOf(got, e.cashID).Counted != "120000.00" {
		t.Fatalf("baca beku: %v %+v", err, got)
	}

	// Shift baru bisa dibuka lagi; nomor dokumen berurutan.
	sh2, err := e.svc.Open(ctx, e.cashier, shift.OpenInput{OpeningCash: "0"})
	if err != nil || sh2.DocNo == sh.DocNo || sh2.SaleCount != 0 {
		t.Fatalf("shift kedua: %v %+v", err, sh2)
	}
}

func TestCloseWithDifferenceNeedsNoteAndPin(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	it := e.item(t, "10000", 100)
	sh, err := e.svc.Open(ctx, e.cashier, shift.OpenInput{OpeningCash: "50000"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := e.sell(e.cashier, it, "1", "cash", "10000"); err != nil {
		t.Fatal(err)
	}
	if _, err := e.sell(e.cashier, it, "2", "transfer", "20000"); err != nil {
		t.Fatal(err)
	}
	// Selisih saling menutup (tunai +1.000, transfer −1.000) tetap butuh persetujuan.
	counts := []shift.CountIn{{MethodID: e.cashID, Counted: "61000"}, {MethodID: e.trID, Counted: "19000"}}
	var de *shift.DiffError
	if _, err := e.svc.Close(ctx, e.cashier, sh.ID, key(), shift.CloseInput{Counts: counts}); !errors.As(err, &de) || de.Diff != "0.00" {
		t.Fatalf("tanpa catatan: %v", err)
	}
	if _, err := e.svc.Close(ctx, e.cashier, sh.ID, key(), shift.CloseInput{Counts: counts, Note: "salah input transfer"}); !errors.Is(err, approval.ErrPinRequired) {
		t.Fatalf("tanpa PIN: %v", err)
	}
	spv := e.person(t, "Supervisor", `{"shift_close":["approve"]}`)
	if err := e.appr.SetPin(ctx, spv, testPassword, "482915"); err != nil {
		t.Fatal(err)
	}
	priceOnly := e.person(t, "SpvHarga", `{"price_override":["approve"]}`)
	if err := e.appr.SetPin(ctx, priceOnly, testPassword, "593816"); err != nil {
		t.Fatal(err)
	}
	if _, err := e.svc.Close(ctx, e.cashier, sh.ID, key(), shift.CloseInput{Counts: counts, Note: "salah input transfer",
		Approval: &shift.ApprovalIn{UserID: priceOnly.UserID, Pin: "593816"}}); !errors.Is(err, approval.ErrInvalidPin) {
		t.Fatalf("penyetuju tanpa izin shift_close: %v", err)
	}
	if _, err := e.svc.Close(ctx, e.cashier, sh.ID, key(), shift.CloseInput{Counts: counts, Note: "salah input transfer",
		Approval: &shift.ApprovalIn{UserID: spv.UserID, Pin: "000111"}}); !errors.Is(err, approval.ErrInvalidPin) {
		t.Fatalf("PIN salah: %v", err)
	}
	closed, err := e.svc.Close(ctx, e.cashier, sh.ID, key(), shift.CloseInput{Counts: counts, Note: "salah input transfer",
		Approval: &shift.ApprovalIn{UserID: spv.UserID, Pin: "482915"}})
	if err != nil || closed.ApprovedByName != "Supervisor" || *closed.DiffAbs != "2000.00" || closed.Note != "salah input transfer" {
		t.Fatalf("disetujui: %v %+v", err, closed)
	}
	if c := countOf(closed, e.trID); *c.Diff != "-1000.00" {
		t.Fatalf("selisih transfer: %+v", c)
	}

	// Daftar supervisor: filter selisih + kasir.
	boss := e.person(t, "Boss", `{"cash_shifts":["view","update"]}`)
	list, err := e.svc.List(ctx, boss, shift.ListParams{DiffOnly: true})
	if err != nil || len(list.Data) != 1 || list.Data[0].ID != sh.ID || len(list.Cashiers) != 1 {
		t.Fatalf("daftar selisih: %v %+v", err, list)
	}
}

func TestExtraMethodAndAccess(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	sh, err := e.svc.Open(ctx, e.cashier, shift.OpenInput{OpeningCash: "0"})
	if err != nil {
		t.Fatal(err)
	}
	// Kasir lain (tanpa izin) tidak boleh melihat/menutup; supervisor boleh; tenant lain tidak menemukan.
	other := e.person(t, "Kasir2", `{"sales_orders":["view","create"]}`)
	if _, err := e.svc.Get(ctx, other, sh.ID); !errors.Is(err, shift.ErrForbidden) {
		t.Fatalf("kasir lain lihat: %v", err)
	}
	if _, err := e.svc.Close(ctx, other, sh.ID, key(), shift.CloseInput{Counts: []shift.CountIn{{MethodID: e.cashID, Counted: "0"}}}); !errors.Is(err, shift.ErrForbidden) {
		t.Fatalf("kasir lain tutup: %v", err)
	}
	stranger := authz.Actor{TenantID: e.other, UserID: uuid.New(), OutletID: uuid.New(), Perms: authz.ParseStored([]byte(`{"*":true}`))}
	if _, err := e.svc.Get(ctx, stranger, sh.ID); !errors.Is(err, shift.ErrNotFound) {
		t.Fatalf("tenant lain: %v", err)
	}
	boss := e.person(t, "Boss", `{"cash_shifts":["view","update"]}`)
	if _, err := e.svc.Get(ctx, boss, sh.ID); err != nil {
		t.Fatalf("supervisor lihat: %v", err)
	}
	// Uang fisik di metode yang tidak ada di rekap = selisih lebih (butuh catatan).
	var de *shift.DiffError
	_, err = e.svc.Close(ctx, boss, sh.ID, key(), shift.CloseInput{Counts: []shift.CountIn{{MethodID: e.cashID, Counted: "0"}, {MethodID: e.trID, Counted: "5000"}}})
	if !errors.As(err, &de) || de.Diff != "5000.00" {
		t.Fatalf("metode tambahan: %v", err)
	}
	closed, err := e.svc.Close(ctx, boss, sh.ID, key(), shift.CloseInput{Counts: []shift.CountIn{{MethodID: e.cashID, Counted: "0"}, {MethodID: e.trID, Counted: "0"}}})
	if err != nil || len(closed.Counts) != 1 || closed.ClosedByName != "Boss" {
		t.Fatalf("supervisor tutup: %v %+v", err, closed)
	}
}

func TestConcurrentOpenAndCloseWhileSelling(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	it := e.item(t, "1000", 1000)

	var wg sync.WaitGroup
	var mu sync.Mutex
	ok := 0
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := e.svc.Open(ctx, e.cashier, shift.OpenInput{OpeningCash: "1000"}); err == nil {
				mu.Lock()
				ok++
				mu.Unlock()
			} else if !errors.Is(err, shift.ErrAlreadyOpen) {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	if ok != 1 {
		t.Fatalf("buka serentak: %d berhasil", ok)
	}
	cur, err := e.svc.Current(ctx, e.cashier)
	if err != nil || cur == nil {
		t.Fatal(err)
	}

	// Penjualan berjalan bersamaan dengan tutup shift: setiap nota yang tersimpan harus masuk rekap beku; sisanya ditolak.
	for round := 0; round < 3; round++ {
		if round > 0 {
			sh, err := e.svc.Open(ctx, e.cashier, shift.OpenInput{OpeningCash: "0"})
			if err != nil {
				t.Fatal(err)
			}
			cur = &sh
		}
		var sold, rejected int
		start := make(chan struct{})
		for i := 0; i < 12; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				<-start
				_, err := e.sell(e.cashier, it, "1", "cash", "1000")
				mu.Lock()
				defer mu.Unlock()
				switch {
				case err == nil:
					sold++
				case errors.Is(err, shift.ErrRequired):
					rejected++
				default:
					t.Error(err)
				}
			}()
		}
		var closed shift.Shift
		var cerr error
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			// Isi uang sesuai rekap saat itu; bila rekap berubah di sela, ulangi (seperti kasir memuat ulang).
			for try := 0; try < 50; try++ {
				r, err := e.svc.Get(ctx, e.cashier, cur.ID)
				if err != nil {
					cerr = err
					return
				}
				var counts []shift.CountIn
				for _, c := range r.Counts {
					counts = append(counts, shift.CountIn{MethodID: c.MethodID, Counted: json.Number(c.Expected)})
				}
				note := ""
				closed, cerr = e.svc.Close(ctx, e.cashier, cur.ID, key(), shift.CloseInput{Counts: counts, Note: note})
				var de *shift.DiffError
				if cerr == nil || !errors.As(cerr, &de) {
					return
				}
			}
		}()
		close(start)
		wg.Wait()
		if cerr != nil {
			t.Fatalf("tutup: %v", cerr)
		}
		if sold+rejected != 12 || closed.SaleCount != sold {
			t.Fatalf("ronde %d: terjual %d, ditolak %d, rekap %d nota", round, sold, rejected, closed.SaleCount)
		}
	}
}
