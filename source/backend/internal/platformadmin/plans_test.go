package platformadmin

import (
	"strings"
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

// Modul paket: daftar tersimpan apa adanya, nil = semua modul, id tak dikenal ditolak.
func TestPlanModulesSave(t *testing.T) {
	e := newEnv(t, "")
	a := mustActor(t, e)
	ctx := context.Background()
	code := "uji-" + uuid.NewString()[:8]
	t.Cleanup(func() { _, _ = e.admin.Exec(context.Background(), `DELETE FROM plans WHERE code = $1`, code) })

	in := SavePlanInput{Code: code, Name: "Uji Modul", Price: decimal.Zero, Active: true, Modules: []string{"items", "members"}}
	id, err := e.svc.SavePlan(ctx, a, in)
	if err != nil {
		t.Fatal(err)
	}
	modulesOf := func() []string {
		t.Helper()
		plans, err := e.svc.ListPlans(ctx)
		if err != nil {
			t.Fatal(err)
		}
		for _, p := range plans {
			if p.ID == id {
				return p.Modules
			}
		}
		t.Fatal("paket tidak ditemukan")
		return nil
	}
	if got := modulesOf(); len(got) != 2 || got[0] != "items" || got[1] != "members" {
		t.Fatalf("modul = %v", got)
	}
	bad := in
	bad.ID, bad.Modules = &id, []string{"items", "tidak_ada"}
	var fe FieldErrors
	if _, err := e.svc.SavePlan(ctx, a, bad); !errors.As(err, &fe) || fe["modules"] == "" {
		t.Fatalf("modul tak dikenal = %v, want validasi modules", err)
	}
	all := in
	all.ID, all.Modules = &id, nil
	if _, err := e.svc.SavePlan(ctx, a, all); err != nil {
		t.Fatal(err)
	}
	if got := modulesOf(); got != nil {
		t.Fatalf("modul = %v, want nil (semua modul)", got)
	}
}

// Poin keunggulan: dipangkas, baris kosong dibuang, batas 12 poin dan 120 karakter ditegakkan.
func TestPlanHighlights(t *testing.T) {
	e := newEnv(t, "")
	a := mustActor(t, e)
	ctx := context.Background()
	code := "uji-" + uuid.NewString()[:8]
	t.Cleanup(func() { _, _ = e.admin.Exec(context.Background(), `DELETE FROM plans WHERE code = $1`, code) })

	in := SavePlanInput{Code: code, Name: "Uji Poin", Price: decimal.Zero, Active: true, Highlights: cleanLines([]string{"  Backup harian ", "", "Dukungan prioritas"})}
	id, err := e.svc.SavePlan(ctx, a, in)
	if err != nil {
		t.Fatal(err)
	}
	plans, err := e.svc.ListPlans(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range plans {
		if p.ID == id && (len(p.Highlights) != 2 || p.Highlights[0] != "Backup harian") {
			t.Fatalf("poin = %q", p.Highlights)
		}
	}
	var fe FieldErrors
	long := in
	long.ID, long.Highlights = &id, []string{strings.Repeat("x", 121)}
	if _, err := e.svc.SavePlan(ctx, a, long); !errors.As(err, &fe) || fe["highlights"] == "" {
		t.Fatalf("poin 121 karakter = %v, want validasi highlights", err)
	}
	many := in
	many.ID, many.Highlights = &id, make([]string, 13)
	for i := range many.Highlights {
		many.Highlights[i] = "poin"
	}
	if _, err := e.svc.SavePlan(ctx, a, many); !errors.As(err, &fe) || fe["highlights"] == "" {
		t.Fatalf("13 poin = %v, want validasi highlights", err)
	}
}
