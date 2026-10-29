package platformadmin

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"

	"github.com/google/uuid"
)

func (e *env) count(t *testing.T, table string, tenant uuid.UUID) int {
	t.Helper()
	var n int
	if err := e.admin.QueryRow(context.Background(), `SELECT count(*) FROM `+table+` WHERE tenant_id = $1`, tenant).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func (e *env) tenantName(t *testing.T, id uuid.UUID) string {
	t.Helper()
	var name string
	if err := e.admin.QueryRow(context.Background(), `SELECT name FROM tenants WHERE id = $1`, id).Scan(&name); err != nil {
		t.Fatal(err)
	}
	return name
}

func TestHapusTenantPermanen(t *testing.T) {
	e := newEnv(t, "")
	ctx := context.Background()
	adminID, _, _ := e.newAdmin(t, "Penghapus")
	a := Actor{ID: adminID, Name: "Penghapus"}
	victim, other := e.newTenant(t), e.newTenant(t)
	name := e.tenantName(t, victim.ID)

	// Tenant aktif tidak boleh dijadwalkan hapus.
	if _, err := e.svc.ScheduleDelete(ctx, a, victim.ID, 7); !errors.Is(err, ErrTenantActive) {
		t.Fatalf("jadwal tenant aktif: %v", err)
	}
	if err := e.svc.SetTenantActive(ctx, a, victim.ID, false); err != nil {
		t.Fatal(err)
	}
	for _, d := range []int{-1, maxDeleteDays + 1} {
		if _, err := e.svc.ScheduleDelete(ctx, a, victim.ID, d); !errors.Is(err, ErrInvalidDays) {
			t.Errorf("days=%d: %v", d, err)
		}
	}
	if _, err := e.svc.ScheduleDelete(ctx, a, uuid.New(), 1); !errors.Is(err, ErrNotFound) {
		t.Errorf("tenant tak ada: %v", err)
	}

	// Belum dijadwalkan / belum jatuh tempo: purge ditolak dan data utuh.
	if _, err := e.svc.PurgeTenant(ctx, a, victim.ID, name); !errors.Is(err, ErrPurgeNotDue) {
		t.Fatalf("purge tanpa jadwal: %v", err)
	}
	if _, err := e.svc.ScheduleDelete(ctx, a, victim.ID, 30); err != nil {
		t.Fatal(err)
	}
	if sch, err := e.svc.DeleteSchedule(ctx, a, victim.ID); err != nil || sch == nil || sch.Due {
		t.Fatalf("jadwal 30 hari: %+v %v", sch, err)
	}
	if _, err := e.svc.PurgeTenant(ctx, a, victim.ID, name); !errors.Is(err, ErrPurgeNotDue) {
		t.Fatalf("purge sebelum jatuh tempo: %v", err)
	}
	if n := e.svc.purgeDue(ctx, slog.New(slog.NewTextHandler(io.Discard, nil))); n != 0 && e.count(t, "outlets", victim.ID) == 0 {
		t.Fatalf("penjadwal menghapus tenant yang belum jatuh tempo")
	}

	// Batalkan jadwal; mengaktifkan kembali juga membatalkan jadwal.
	if err := e.svc.CancelDelete(ctx, a, victim.ID); err != nil {
		t.Fatal(err)
	}
	if err := e.svc.CancelDelete(ctx, a, victim.ID); !errors.Is(err, ErrNotScheduledDel) {
		t.Errorf("batal dua kali: %v", err)
	}
	if _, err := e.svc.ScheduleDelete(ctx, a, victim.ID, 5); err != nil {
		t.Fatal(err)
	}
	if err := e.svc.SetTenantActive(ctx, a, victim.ID, true); err != nil {
		t.Fatal(err)
	}
	if sch, _ := e.svc.DeleteSchedule(ctx, a, victim.ID); sch != nil {
		t.Fatalf("jadwal harusnya hilang setelah tenant diaktifkan: %+v", sch)
	}
	if err := e.svc.SetTenantActive(ctx, a, victim.ID, false); err != nil {
		t.Fatal(err)
	}

	// Masa tunggu 0 = langsung jatuh tempo; nama konfirmasi harus cocok.
	if _, err := e.svc.ScheduleDelete(ctx, a, victim.ID, 0); err != nil {
		t.Fatal(err)
	}
	if _, err := e.svc.PurgeTenant(ctx, a, victim.ID, "salah"); !errors.Is(err, ErrNameMismatch) {
		t.Fatalf("nama salah: %v", err)
	}
	if e.count(t, "outlets", victim.ID) != 2 {
		t.Fatal("data hilang padahal purge ditolak")
	}
	rows, err := e.svc.PurgeTenant(ctx, a, victim.ID, name)
	if err != nil {
		t.Fatal(err)
	}
	if rows["outlets"] != 2 {
		t.Errorf("rincian baris: %v", rows)
	}
	if e.count(t, "outlets", victim.ID) != 0 || e.count(t, "audit_log", victim.ID) != 0 {
		t.Error("data tenant masih ada")
	}
	var n int
	if err := e.admin.QueryRow(ctx, `SELECT count(*) FROM tenants WHERE id = $1`, victim.ID).Scan(&n); err != nil || n != 0 {
		t.Errorf("baris tenant masih ada: %d %v", n, err)
	}
	// Tenant lain tak tersentuh; jejak audit platform bertahan.
	if e.count(t, "outlets", other.ID) != 2 {
		t.Error("data tenant lain ikut terhapus")
	}
	if e.count(t, "platform_audit_log", victim.ID) == 0 {
		t.Error("audit platform harusnya bertahan")
	}
	if _, err := e.svc.PurgeTenant(ctx, a, victim.ID, name); !errors.Is(err, ErrNotFound) {
		t.Errorf("purge ulang: %v", err)
	}
}

// Penjadwal otomatis menghapus tenant nonaktif yang jatuh tempo, dan hanya itu.
func TestPenjadwalHapusTenant(t *testing.T) {
	e := newEnv(t, "")
	ctx := context.Background()
	adminID, _, _ := e.newAdmin(t, "Penghapus")
	a := Actor{ID: adminID, Name: "Penghapus"}
	due, later := e.newTenant(t), e.newTenant(t)
	for _, f := range []tenantFixture{due, later} {
		if err := e.svc.SetTenantActive(ctx, a, f.ID, false); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := e.svc.ScheduleDelete(ctx, a, due.ID, 0); err != nil {
		t.Fatal(err)
	}
	if _, err := e.svc.ScheduleDelete(ctx, a, later.ID, 10); err != nil {
		t.Fatal(err)
	}
	e.svc.purgeDue(ctx, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if e.count(t, "outlets", due.ID) != 0 {
		t.Error("tenant jatuh tempo belum terhapus")
	}
	if e.count(t, "outlets", later.ID) != 2 {
		t.Error("tenant yang belum jatuh tempo ikut terhapus")
	}
}
