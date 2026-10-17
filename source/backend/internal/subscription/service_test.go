package subscription

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"aciraba/internal/authz"
	"aciraba/internal/plan"
	"aciraba/internal/platform/db"
)

type env struct {
	svc        *Service
	app, admin *pgxpool.Pool
	tenant     uuid.UUID
	planID     uuid.UUID
	status     string // status yang dilaporkan EraPay palsu
	mu         sync.Mutex
	creates    int
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

	e := &env{app: app, admin: admin, tenant: uuid.New(), planID: uuid.New(), status: "pending"}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		e.mu.Lock()
		defer e.mu.Unlock()
		if r.Header.Get("Authorization") != "Bearer platform-key" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		if r.Method == http.MethodPost {
			e.creates++
			w.WriteHeader(http.StatusCreated)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"id": "pay_1", "status": e.status, "qr_code": map[string]string{"type": "qris", "value": "000201QR"}})
	}))
	t.Cleanup(srv.Close)
	e.svc = NewService(app, srv.URL, "platform-key")

	sfx := e.tenant.String()[:8]
	for _, q := range []struct {
		sql  string
		args []any
	}{
		{`INSERT INTO tenants (id, code, name) VALUES ($1, $2, 'UJI-SUB')`, []any{e.tenant, "sb-" + sfx}},
		{`INSERT INTO plans (id, code, name, price, duration_days, max_tx_per_day, max_users, self_service) VALUES ($1, $2, 'UJI-PRO', 20000, 30, 300, 4, true)`, []any{e.planID, "sb-" + sfx}},
	} {
		if _, err := admin.Exec(ctx, q.sql, q.args...); err != nil {
			t.Fatal(err)
		}
	}
	t.Cleanup(func() {
		for _, q := range []string{`DELETE FROM audit_log WHERE tenant_id = $1`, `DELETE FROM subscription_payments WHERE tenant_id = $1`, `DELETE FROM tenant_subscriptions WHERE tenant_id = $1`, `DELETE FROM tenants WHERE id = $1`} {
			_, _ = admin.Exec(ctx, q, e.tenant)
		}
		_, _ = admin.Exec(ctx, `DELETE FROM plans WHERE id = $1`, e.planID)
	})
	return e
}

func (e *env) owner() authz.Actor {
	return authz.Actor{TenantID: e.tenant, UserID: uuid.New(), Perms: authz.Permissions{All: true}}
}

func (e *env) setStatus(s string) { e.mu.Lock(); e.status = s; e.mu.Unlock() }

func (e *env) effective(t *testing.T) plan.Effective {
	t.Helper()
	var got plan.Effective
	if err := db.WithTenant(context.Background(), e.app, e.tenant, func(tx pgx.Tx) error {
		var err error
		got, err = plan.Get(context.Background(), tx, e.tenant)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	return got
}

func TestCheckoutPayAndActivate(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	a := e.owner()
	key := "key-" + uuid.NewString()

	p, err := e.svc.Checkout(ctx, a, key, e.planID)
	if err != nil {
		t.Fatal(err)
	}
	if p.Status != "pending" || p.QRValue != "000201QR" || p.Amount != "20000" || p.Applied {
		t.Fatalf("tagihan = %+v", p)
	}
	// Kirim ulang dengan key sama: tagihan sama, EraPay tidak dipanggil lagi.
	again, err := e.svc.Checkout(ctx, a, key, e.planID)
	if err != nil || again.ID != p.ID || e.creates != 1 {
		t.Fatalf("idempoten: err=%v id sama=%v creates=%d", err, again.ID == p.ID, e.creates)
	}
	// Belum lunas: paket tetap FREE.
	got, err := e.svc.GetPayment(ctx, a, p.ID)
	if err != nil || got.Status != "pending" || got.Applied {
		t.Fatalf("belum lunas: %+v err=%v", got, err)
	}
	if c := e.effective(t); !c.IsDefault {
		t.Fatalf("paket sebelum bayar = %+v, want default", c)
	}
	// Lunas: langganan aktif sekali.
	e.setStatus("succeeded")
	got, err = e.svc.GetPayment(ctx, a, p.ID)
	if err != nil || got.Status != "succeeded" || !got.Applied || got.SubscriptionEnds == nil {
		t.Fatalf("lunas: %+v err=%v", got, err)
	}
	c := e.effective(t)
	if c.IsDefault || c.PlanID != e.planID || c.MaxTxPerDay == nil || *c.MaxTxPerDay != 300 || c.MaxUsers == nil || *c.MaxUsers != 4 {
		t.Fatalf("paket setelah bayar = %+v", c)
	}
	ends := *c.ExpiresAt
	if _, err := e.svc.GetPayment(ctx, a, p.ID); err != nil { // polling berulang tidak memperpanjang lagi
		t.Fatal(err)
	}
	if c2 := e.effective(t); !c2.ExpiresAt.Equal(ends) {
		t.Fatalf("polling ulang mengubah masa aktif: %v -> %v", ends, c2.ExpiresAt)
	}
}

func TestReconcilerActivatesPaidInvoice(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	p, err := e.svc.Checkout(ctx, e.owner(), "key-"+uuid.NewString(), e.planID)
	if err != nil {
		t.Fatal(err)
	}
	e.setStatus("succeeded") // dibayar, tenant menutup halaman
	r := NewReconciler(e.svc, nil)
	r.log = discardLogger()
	if n := r.Tick(ctx); n < 1 {
		t.Fatalf("penjadwal memeriksa %d pembayaran, want >= 1", n)
	}
	if c := e.effective(t); c.IsDefault {
		t.Fatalf("penjadwal tidak mengaktifkan paket: %+v (pembayaran %s)", c, p.ID)
	}
}

func TestCheckoutGuards(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	kasir := authz.Actor{TenantID: e.tenant, UserID: uuid.New()}
	if _, err := e.svc.Checkout(ctx, kasir, "key-"+uuid.NewString(), e.planID); !errors.Is(err, ErrOwnerOnly) {
		t.Fatalf("non-owner = %v, want ErrOwnerOnly", err)
	}
	imp := e.owner()
	imp.Impersonator = uuid.New()
	if _, err := e.svc.Checkout(ctx, imp, "key-"+uuid.NewString(), e.planID); !errors.Is(err, ErrImpersonating) {
		t.Fatalf("masuk-sebagai = %v, want ErrImpersonating", err)
	}
	if _, err := e.svc.Checkout(ctx, e.owner(), "key-"+uuid.NewString(), uuid.New()); !errors.Is(err, ErrPlanNotFound) {
		t.Fatalf("paket tak ada = %v", err)
	}
	// Paket non-self-service (mis. enterprise) tidak bisa dibeli lewat checkout.
	ent := uuid.New()
	if _, err := e.admin.Exec(ctx, `INSERT INTO plans (id, code, name) VALUES ($1, $2, 'ENT')`, ent, "en-"+ent.String()[:8]); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = e.admin.Exec(ctx, `DELETE FROM plans WHERE id = $1`, ent) })
	if _, err := e.svc.Checkout(ctx, e.owner(), "key-"+uuid.NewString(), ent); !errors.Is(err, ErrPlanNotFound) {
		t.Fatalf("paket non-self-service = %v, want ErrPlanNotFound", err)
	}
	// Key yang sama untuk paket lain ditolak.
	k := "key-" + uuid.NewString()
	if _, err := e.svc.Checkout(ctx, e.owner(), k, e.planID); err != nil {
		t.Fatal(err)
	}
	other := uuid.New()
	if _, err := e.admin.Exec(ctx, `INSERT INTO plans (id, code, name, price, duration_days, self_service) VALUES ($1, $2, 'P2', 50000, 30, true)`, other, "p2-"+other.String()[:8]); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = e.admin.Exec(ctx, `DELETE FROM subscription_payments WHERE plan_id = $1`, other); _, _ = e.admin.Exec(ctx, `DELETE FROM plans WHERE id = $1`, other) })
	if _, err := e.svc.Checkout(ctx, e.owner(), k, other); !errors.Is(err, ErrKeyConflict) {
		t.Fatalf("key dipakai paket lain = %v, want ErrKeyConflict", err)
	}
	st, err := e.svc.Status(ctx, e.owner())
	if err != nil || !st.PaymentsEnabled || !st.Current.IsDefault {
		t.Fatalf("status = %+v err=%v", st, err)
	}
	found := false
	for _, p := range st.Plans {
		if p.ID == e.planID {
			found = true
		}
		if p.ID == ent {
			t.Fatal("paket non-self-service tampil di halaman upgrade")
		}
	}
	if !found {
		t.Fatal(fmt.Sprint("paket self-service tidak tampil: ", st.Plans))
	}
}

func discardLogger() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }
