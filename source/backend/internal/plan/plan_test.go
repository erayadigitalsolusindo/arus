package plan

import (
	"context"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"aciraba/internal/platform/db"
)

// Integrasi: TEST_DATABASE_URL = role aplikasi, TEST_ADMIN_DATABASE_URL = pemilik skema.
type env struct {
	app, admin *pgxpool.Pool
	tenant     uuid.UUID
	role       uuid.UUID
	suffix     string
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
	e := &env{app: app, admin: admin, tenant: uuid.New(), role: uuid.New(), suffix: fmt.Sprintf("%d-%d", os.Getpid(), time.Now().UnixNano()%1_000_000)}
	e.exec(t, `INSERT INTO tenants (id, code, name) VALUES ($1, $2, 'UJI-PLAN')`, e.tenant, "pl-"+e.suffix)
	e.exec(t, `INSERT INTO roles (id, tenant_id, name) VALUES ($1, $2, 'Owner')`, e.role, e.tenant)
	e.addUser(t, "owner")
	t.Cleanup(func() {
		for _, q := range []string{
			`DELETE FROM tenant_daily_tx WHERE tenant_id = $1`, `DELETE FROM subscription_payments WHERE tenant_id = $1`,
			`DELETE FROM tenant_subscriptions WHERE tenant_id = $1`, `DELETE FROM users WHERE tenant_id = $1`,
			`DELETE FROM roles WHERE tenant_id = $1`, `DELETE FROM tenants WHERE id = $1`,
		} {
			_, _ = admin.Exec(ctx, q, e.tenant)
		}
		_, _ = admin.Exec(ctx, `DELETE FROM plans WHERE code LIKE 'uji-%'`)
	})
	return e
}

func (e *env) exec(t *testing.T, sql string, args ...any) {
	t.Helper()
	if _, err := e.admin.Exec(context.Background(), sql, args...); err != nil {
		t.Fatalf("%s: %v", sql, err)
	}
}

func (e *env) addUser(t *testing.T, name string) {
	t.Helper()
	e.exec(t, `INSERT INTO users (tenant_id, role_id, email, name, password_hash) VALUES ($1, $2, $3, $4, 'x')`,
		e.tenant, e.role, fmt.Sprintf("%s-%s@plan.test", name, e.suffix), name)
}

// newPlan membuat paket uji dan memasangnya ke tenant (kedaluwarsa `exp`, nil = tanpa kedaluwarsa).
func (e *env) newPlan(t *testing.T, code string, maxTx, maxUsers *int, exp *time.Time) uuid.UUID {
	t.Helper()
	id := uuid.New()
	e.exec(t, `INSERT INTO plans (id, code, name, max_tx_per_day, max_users) VALUES ($1, $2, $2, $3, $4)`, id, code+"-"+e.suffix, maxTx, maxUsers)
	e.exec(t, `INSERT INTO tenant_subscriptions (tenant_id, plan_id, expires_at) VALUES ($1, $2, $3)
		ON CONFLICT (tenant_id) DO UPDATE SET plan_id = EXCLUDED.plan_id, expires_at = EXCLUDED.expires_at`, e.tenant, id, exp)
	return id
}

func intp(n int) *int { return &n }

func day(s string) pgtype.Date {
	tm, _ := time.Parse("2006-01-02", s)
	return pgtype.Date{Time: tm, Valid: true}
}

func (e *env) consume(t *testing.T, d string) error {
	t.Helper()
	return db.WithTenant(context.Background(), e.app, e.tenant, func(tx pgx.Tx) error {
		return ConsumeTx(context.Background(), tx, e.tenant, day(d))
	})
}

func (e *env) userSlot() error {
	return db.WithTenant(context.Background(), e.app, e.tenant, func(tx pgx.Tx) error {
		return CheckUserSlot(context.Background(), tx, e.tenant)
	})
}

func TestDefaultPlanWhenNoSubscription(t *testing.T) {
	e := newEnv(t)
	var got Effective
	err := db.WithTenant(context.Background(), e.app, e.tenant, func(tx pgx.Tx) error {
		var err error
		got, err = Get(context.Background(), tx, e.tenant)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	if !got.IsDefault || got.Code != "free" || got.MaxTxPerDay == nil || *got.MaxTxPerDay != 100 || got.MaxUsers == nil || *got.MaxUsers != 2 {
		t.Fatalf("paket default = %+v, want FREE 100 trx / 2 user", got)
	}
}

func TestTxLimitPerDay(t *testing.T) {
	e := newEnv(t)
	e.newPlan(t, "uji-tx", intp(3), nil, nil)
	for i := 0; i < 3; i++ {
		if err := e.consume(t, "2030-01-01"); err != nil {
			t.Fatalf("transaksi %d: %v", i+1, err)
		}
	}
	if err := e.consume(t, "2030-01-01"); !errors.Is(err, ErrTxLimit) {
		t.Fatalf("transaksi ke-4 = %v, want ErrTxLimit", err)
	}
	if err := e.consume(t, "2030-01-02"); err != nil { // hari berikutnya mulai dari nol
		t.Fatalf("hari baru: %v", err)
	}
	// Transaksi yang di-rollback tidak memakai jatah.
	boom := errors.New("batal")
	err := db.WithTenant(context.Background(), e.app, e.tenant, func(tx pgx.Tx) error {
		if err := ConsumeTx(context.Background(), tx, e.tenant, day("2030-01-03")); err != nil {
			return err
		}
		return boom
	})
	if !errors.Is(err, boom) {
		t.Fatal(err)
	}
	var n int
	_ = e.admin.QueryRow(context.Background(), `SELECT count(*) FROM tenant_daily_tx WHERE tenant_id = $1 AND day = '2030-01-03'`, e.tenant).Scan(&n)
	if n != 0 {
		t.Fatalf("hitungan bocor setelah rollback: %d baris", n)
	}
}

func TestTxLimitConcurrent(t *testing.T) {
	e := newEnv(t)
	e.newPlan(t, "uji-cc", intp(5), nil, nil)
	const workers = 20
	res := make(chan error, workers)
	for i := 0; i < workers; i++ {
		go func() { res <- e.consume(t, "2030-02-01") }()
	}
	ok, blocked := 0, 0
	for i := 0; i < workers; i++ {
		switch err := <-res; {
		case err == nil:
			ok++
		case errors.Is(err, ErrTxLimit):
			blocked++
		default:
			t.Fatal(err)
		}
	}
	if ok != 5 || blocked != workers-5 {
		t.Fatalf("lolos=%d ditolak=%d, want 5/%d", ok, blocked, workers-5)
	}
}

func TestUnlimitedPlan(t *testing.T) {
	e := newEnv(t)
	e.newPlan(t, "uji-unl", nil, nil, nil)
	for i := 0; i < 150; i++ { // melewati batas FREE (100)
		if err := e.consume(t, "2030-03-01"); err != nil {
			t.Fatalf("transaksi %d: %v", i+1, err)
		}
	}
	for i := 0; i < 5; i++ {
		e.addUser(t, fmt.Sprintf("u%d", i))
	}
	if err := e.userSlot(); err != nil {
		t.Fatalf("paket tanpa batas menolak pengguna: %v", err)
	}
}

func TestExpiredFallsBackToFree(t *testing.T) {
	e := newEnv(t)
	past := time.Now().Add(-time.Hour)
	e.newPlan(t, "uji-exp", nil, nil, &past) // tanpa batas, tapi sudah kedaluwarsa
	e.addUser(t, "kedua")                     // 2 pengguna aktif = batas FREE
	if err := e.userSlot(); !errors.Is(err, ErrUserLimit) {
		t.Fatalf("setelah kedaluwarsa, pengguna ke-3 = %v, want ErrUserLimit (FREE)", err)
	}
}

func TestUserLimitCountsActiveOnly(t *testing.T) {
	e := newEnv(t)
	e.newPlan(t, "uji-usr", nil, intp(2), nil)
	if err := e.userSlot(); err != nil { // 1 pengguna (owner), batas 2
		t.Fatalf("slot kedua: %v", err)
	}
	e.addUser(t, "kasir")
	if err := e.userSlot(); !errors.Is(err, ErrUserLimit) {
		t.Fatalf("slot ketiga = %v, want ErrUserLimit", err)
	}
	e.exec(t, `UPDATE users SET active = false WHERE tenant_id = $1 AND name = 'kasir'`, e.tenant)
	if err := e.userSlot(); err != nil {
		t.Fatalf("pengguna nonaktif tidak boleh dihitung: %v", err)
	}
}

func TestOverrideBeatsPlan(t *testing.T) {
	e := newEnv(t)
	e.newPlan(t, "uji-ov", intp(1), intp(1), nil)
	e.exec(t, `UPDATE tenant_subscriptions SET override_max_tx_per_day = 2, override_max_users = 3 WHERE tenant_id = $1`, e.tenant)
	if err := e.consume(t, "2030-04-01"); err != nil {
		t.Fatal(err)
	}
	if err := e.consume(t, "2030-04-01"); err != nil {
		t.Fatalf("override 2 trx: %v", err)
	}
	if err := e.consume(t, "2030-04-01"); !errors.Is(err, ErrTxLimit) {
		t.Fatalf("trx ke-3 = %v, want ErrTxLimit", err)
	}
	if err := e.userSlot(); err != nil {
		t.Fatalf("override 3 user: %v", err)
	}
}

func TestActivateOnceAndExtend(t *testing.T) {
	e := newEnv(t)
	planID := uuid.New()
	e.exec(t, `INSERT INTO plans (id, code, name, price, duration_days, max_tx_per_day, max_users, self_service) VALUES ($1, $2, 'UJI', 20000, 30, 300, 4, true)`, planID, "uji-act-"+e.suffix)
	pay := func() uuid.UUID {
		id := uuid.New()
		e.exec(t, `INSERT INTO subscription_payments (id, tenant_id, plan_id, idempotency_key, amount, duration_days, status) VALUES ($1, $2, $3, $4, 20000, 30, 'succeeded')`,
			id, e.tenant, planID, "k-"+id.String())
		return id
	}
	activate := func(id uuid.UUID) *time.Time {
		var ends pgtype.Timestamptz
		if err := db.WithTenant(context.Background(), e.app, e.tenant, func(tx pgx.Tx) error {
			return tx.QueryRow(context.Background(), `SELECT subscription_activate($1, $2)`, e.tenant, id).Scan(&ends)
		}); err != nil {
			t.Fatal(err)
		}
		if !ends.Valid {
			return nil
		}
		return &ends.Time
	}
	p1 := pay()
	first := activate(p1)
	if first == nil || first.Before(time.Now().Add(29*24*time.Hour)) || first.After(time.Now().Add(31*24*time.Hour)) {
		t.Fatalf("aktivasi pertama berakhir %v, want ~30 hari lagi", first)
	}
	if again := activate(p1); again != nil {
		t.Fatalf("pembayaran yang sama diterapkan dua kali: %v", again)
	}
	second := activate(pay()) // perpanjangan paket yang sama: dihitung dari tanggal berakhir lama
	if second == nil || second.Sub(*first) < 29*24*time.Hour || second.Sub(*first) > 31*24*time.Hour {
		t.Fatalf("perpanjangan berakhir %v dari %v, want +30 hari", second, first)
	}
	var cur Effective
	_ = db.WithTenant(context.Background(), e.app, e.tenant, func(tx pgx.Tx) error {
		var err error
		cur, err = Get(context.Background(), tx, e.tenant)
		return err
	})
	if cur.PlanID != planID || cur.MaxTxPerDay == nil || *cur.MaxTxPerDay != 300 {
		t.Fatalf("paket efektif setelah bayar = %+v", cur)
	}
	// Pembayaran belum lunas tidak mengaktifkan apa pun.
	pending := uuid.New()
	e.exec(t, `INSERT INTO subscription_payments (id, tenant_id, plan_id, idempotency_key, amount, duration_days) VALUES ($1, $2, $3, 'k-pending', 20000, 30)`, pending, e.tenant, planID)
	if got := activate(pending); got != nil {
		t.Fatalf("pembayaran pending diaktifkan: %v", got)
	}
}

func TestPlanWritesNeedPlatformAdmin(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	_, err := e.app.Exec(ctx, `INSERT INTO plans (code, name) VALUES ('uji-hack', 'x')`)
	if err == nil {
		t.Fatal("role aplikasi bisa menulis tabel plans langsung")
	}
	var id pgtype.UUID
	err = e.app.QueryRow(ctx, `SELECT platform_save_plan($1, NULL, 'uji-x', 'X', '', 0, NULL, NULL, NULL, false, true, 0)`, uuid.New()).Scan(&id)
	if err == nil {
		t.Fatal("bukan platform admin bisa menyimpan paket")
	}
	if _, err := e.app.Exec(ctx, `UPDATE tenant_subscriptions SET plan_id = plan_id`); err == nil {
		t.Fatal("role aplikasi bisa menulis tenant_subscriptions langsung")
	}
}
