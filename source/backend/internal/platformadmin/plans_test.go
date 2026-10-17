package platformadmin

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

// Invariant paket: (1) seed FREE default + PRO ada; (2) kode unik; (3) hanya Platform Admin aktif yang menyimpan paket;
// (4) validasi (dijual sendiri wajib berharga & berdurasi); (5) paket default tidak bisa dijual/dinonaktifkan;
// (6) paket dipasang ke tenant, override berlaku, dan "tanpa paket" mengembalikan ke default.
func TestPlansSeedAndCRUD(t *testing.T) {
	e := newEnv(t, "")
	a := mustActor(t, e)
	ctx := context.Background()

	plans, err := e.svc.ListPlans(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var free *PlanRow
	for i := range plans {
		if plans[i].Code == "free" {
			free = &plans[i]
		}
	}
	if free == nil || !free.IsDefault || free.MaxTxPerDay == nil || *free.MaxTxPerDay != 100 || free.MaxUsers == nil || *free.MaxUsers != 2 {
		t.Fatalf("seed FREE = %+v", free)
	}

	code := "uji-" + uuid.NewString()[:8]
	t.Cleanup(func() { _, _ = e.admin.Exec(context.Background(), `DELETE FROM plans WHERE code = $1`, code) })
	three, five := 3, 5
	in := SavePlanInput{Code: code, Name: "Uji", Price: decimal.NewFromInt(50000), DurationDays: intPtr(30), MaxTxPerDay: &three, MaxUsers: &five, SelfService: true, Active: true, SortOrder: 9}
	id, err := e.svc.SavePlan(ctx, a, in)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := e.svc.SavePlan(ctx, a, in); !errors.Is(err, ErrPlanCodeTaken) {
		t.Fatalf("kode ganda = %v, want ErrPlanCodeTaken", err)
	}
	if _, err := e.svc.SavePlan(ctx, Actor{ID: uuid.New()}, in); !errors.Is(err, ErrInvalidSession) {
		t.Fatalf("bukan admin = %v, want ErrInvalidSession", err)
	}
	bad := in
	bad.Code, bad.DurationDays = "uji-bad", nil
	var fe FieldErrors
	if _, err := e.svc.SavePlan(ctx, a, bad); !errors.As(err, &fe) || fe["self_service"] == "" {
		t.Fatalf("dijual sendiri tanpa durasi = %v, want validasi self_service", err)
	}
	// Ubah (code tidak berubah); batas NULL = tanpa batas.
	upd := in
	upd.ID, upd.Name, upd.MaxTxPerDay, upd.MaxUsers = &id, "Uji 2", nil, nil
	if _, err := e.svc.SavePlan(ctx, a, upd); err != nil {
		t.Fatal(err)
	}
	// Paket default tidak boleh dijual sendiri dan tetap aktif walau diminta nonaktif.
	def := SavePlanInput{ID: &free.ID, Name: free.Name, Price: decimal.Zero, MaxTxPerDay: free.MaxTxPerDay, MaxUsers: free.MaxUsers, Active: false}
	if _, err := e.svc.SavePlan(ctx, a, def); err != nil {
		t.Fatal(err)
	}
	var active bool
	_ = e.admin.QueryRow(ctx, `SELECT active FROM plans WHERE id = $1`, free.ID).Scan(&active)
	if !active {
		t.Fatal("paket default dinonaktifkan")
	}
	def.SelfService = true
	def.Price, def.DurationDays, def.Active = decimal.NewFromInt(1000), intPtr(30), true
	if _, err := e.svc.SavePlan(ctx, a, def); err == nil {
		t.Fatal("paket default dijual sendiri harus ditolak")
	}

	// Pasang ke tenant dengan override; lalu kembali ke default.
	tn := e.newTenant(t)
	exp := time.Now().Add(48 * time.Hour)
	ten := 7
	if err := e.svc.SetTenantPlan(ctx, a, tn.ID, SetTenantPlanInput{PlanID: &id, ExpiresAt: &exp, OverrideTx: &ten, Note: "uji"}); err != nil {
		t.Fatal(err)
	}
	tp, err := e.svc.GetTenantPlan(ctx, tn.ID)
	if err != nil {
		t.Fatal(err)
	}
	if tp.Effective.PlanID != id || tp.Effective.MaxTxPerDay == nil || *tp.Effective.MaxTxPerDay != 7 || tp.Effective.MaxUsers != nil || tp.Subscription == nil || tp.Subscription.Note != "uji" {
		t.Fatalf("paket tenant = %+v / %+v", tp.Effective, tp.Subscription)
	}
	if err := e.svc.SetTenantPlan(ctx, a, tn.ID, SetTenantPlanInput{PlanID: &id, OverrideTx: intPtr(0)}); !errors.As(err, &fe) {
		t.Fatalf("override 0 = %v, want validasi", err)
	}
	if err := e.svc.SetTenantPlan(ctx, a, tn.ID, SetTenantPlanInput{}); err != nil {
		t.Fatal(err)
	}
	tp, _ = e.svc.GetTenantPlan(ctx, tn.ID)
	if !tp.Effective.IsDefault || tp.Subscription != nil {
		t.Fatalf("setelah dikembalikan = %+v / %+v", tp.Effective, tp.Subscription)
	}
	if _, err := e.svc.GetTenantPlan(ctx, uuid.New()); !errors.Is(err, ErrNotFound) {
		t.Fatalf("tenant tak ada = %v", err)
	}
	if err := e.svc.SetTenantPlan(ctx, Actor{ID: uuid.New()}, tn.ID, SetTenantPlanInput{PlanID: &id}); !errors.Is(err, ErrInvalidSession) {
		t.Fatalf("bukan admin = %v, want ErrInvalidSession", err)
	}
}

func intPtr(n int) *int { return &n }
