package item

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"

	"aciraba/internal/plan"
)

// Batas barang aktif paket: barang ke-(max+1) ditolak, mengarsipkan membebaskan slot, mengaktifkan kembali saat penuh ditolak,
// batas kosong = tanpa batas, dan override tenant mengalahkan batas paket.
func TestPlanItemLimit(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	unit := e.master(t, "units", e.a.TenantID, "Pcs", true)

	planID := uuid.New()
	if _, err := e.admin.Exec(ctx, `INSERT INTO plans (id, code, name, max_items) VALUES ($1, $2, 'UJI', 2)`, planID, "it-"+planID.String()[:8]); err != nil {
		t.Fatal(err)
	}
	if _, err := e.admin.Exec(ctx, `INSERT INTO tenant_subscriptions (tenant_id, plan_id) VALUES ($1, $2)`, e.a.TenantID, planID); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = e.admin.Exec(ctx, `DELETE FROM tenant_subscriptions WHERE tenant_id = $1`, e.a.TenantID)
		_, _ = e.admin.Exec(ctx, `DELETE FROM plans WHERE id = $1`, planID)
	})

	mk := func(name string) (*Item, error) {
		return e.svc.Create(ctx, e.a, Input{Name: name, UnitID: unit.String()})
	}
	i1, err := mk("Satu")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := mk("Dua"); err != nil {
		t.Fatal(err)
	}
	if _, err := mk("Tiga"); !errors.Is(err, plan.ErrItemLimit) {
		t.Fatalf("barang ke-3 = %v, want ErrItemLimit", err)
	}
	// Barang tenant lain tidak ikut dihitung.
	if _, err := e.svc.Create(ctx, e.b, Input{Name: "Milik B", UnitID: e.master(t, "units", e.b.TenantID, "Pcs", true).String()}); err != nil {
		t.Fatalf("tenant lain tak boleh terpengaruh: %v", err)
	}
	// Arsipkan satu -> slot bebas; aktifkan kembali saat penuh ditolak, saat belum penuh lolos.
	if _, err := e.svc.SetActive(ctx, e.a, i1.ID, false); err != nil {
		t.Fatal(err)
	}
	i3, err := mk("Tiga")
	if err != nil {
		t.Fatalf("setelah arsip: %v", err)
	}
	if _, err := e.svc.SetActive(ctx, e.a, i1.ID, true); !errors.Is(err, plan.ErrItemLimit) {
		t.Fatalf("aktifkan kembali saat penuh = %v, want ErrItemLimit", err)
	}
	// Mengaktifkan barang yang sudah aktif tidak memakai slot baru.
	if _, err := e.svc.SetActive(ctx, e.a, i3.ID, true); err != nil {
		t.Fatalf("aktifkan barang yang sudah aktif: %v", err)
	}
	// Override tenant menaikkan batas; batas kosong = tanpa batas.
	if _, err := e.admin.Exec(ctx, `UPDATE tenant_subscriptions SET override_max_items = 5 WHERE tenant_id = $1`, e.a.TenantID); err != nil {
		t.Fatal(err)
	}
	if _, err := e.svc.SetActive(ctx, e.a, i1.ID, true); err != nil {
		t.Fatalf("setelah override 5: %v", err)
	}
	if _, err := e.admin.Exec(ctx, `UPDATE tenant_subscriptions SET override_max_items = NULL WHERE tenant_id = $1`, e.a.TenantID); err != nil {
		t.Fatal(err)
	}
	if _, err := e.admin.Exec(ctx, `UPDATE plans SET max_items = NULL WHERE id = $1`, planID); err != nil {
		t.Fatal(err)
	}
	if _, err := mk("Empat"); err != nil {
		t.Fatalf("tanpa batas: %v", err)
	}
}
