package storefront

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
)

// Tenant memilih host landing dari domain yang didaftarkan admin: tambah sub-host, atur peran, hapus; tak bisa menyentuh tenant lain.
func TestTenantDomains(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	base := "uji" + uuid.NewString()[:8] + ".com"
	t.Cleanup(func() { _, _ = e.admin.Exec(ctx, `DELETE FROM tenant_domains WHERE host LIKE '%' || $1`, base) })

	var rootID, otherID uuid.UUID
	if err := e.admin.QueryRow(ctx, `INSERT INTO tenant_domains (tenant_id, host, status, verified_at) VALUES ($1, $2, 'active', now()) RETURNING id`, e.a.TenantID, base).Scan(&rootID); err != nil {
		t.Fatal(err)
	}
	if err := e.admin.QueryRow(ctx, `INSERT INTO tenant_domains (tenant_id, host, status) VALUES ($1, $2, 'pending') RETURNING id`, e.b.TenantID, "b."+base).Scan(&otherID); err != nil {
		t.Fatal(err)
	}

	sub, err := e.svc.AddSubdomain(ctx, e.a, base, "Landing")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := e.svc.AddSubdomain(ctx, e.a, base, "landing"); !errors.Is(err, ErrDomainTaken) {
		t.Fatalf("duplikat = %v", err)
	}
	for _, label := range []string{"-x", "a_b", "a.b", ""} {
		if _, err := e.svc.AddSubdomain(ctx, e.a, base, label); !errors.Is(err, ErrDomainLabel) {
			t.Errorf("label %q = %v, want ErrDomainLabel", label, err)
		}
	}
	// Domain induk milik tenant lain / tidak aktif / bukan buatan admin tidak bisa dipakai.
	if _, err := e.svc.AddSubdomain(ctx, e.b, base, "x"); !errors.Is(err, ErrDomainBase) {
		t.Fatalf("induk tenant lain = %v", err)
	}
	if _, err := e.svc.AddSubdomain(ctx, e.a, "landing."+base, "x"); !errors.Is(err, ErrDomainBase) {
		t.Fatalf("induk buatan tenant = %v", err)
	}

	if err := e.svc.SetDomainRole(ctx, e.a, sub, false, true); err != nil {
		t.Fatal(err)
	}
	if err := e.svc.SetDomainRole(ctx, e.a, sub, false, false); !errors.Is(err, ErrDomainRole) {
		t.Fatalf("tanpa peran = %v", err)
	}
	if err := e.svc.SetDomainRole(ctx, e.a, otherID, true, true); !errors.Is(err, ErrNotFound) {
		t.Fatalf("host tenant lain = %v", err)
	}
	if err := e.svc.DeleteSubdomain(ctx, e.a, rootID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("hapus domain admin = %v", err)
	}
	if err := e.svc.DeleteSubdomain(ctx, e.b, sub); !errors.Is(err, ErrNotFound) {
		t.Fatalf("hapus sub tenant lain = %v", err)
	}

	list, err := e.svc.Domains(ctx, e.a)
	if err != nil || len(list) != 2 {
		t.Fatalf("daftar = %+v %v", list, err)
	}
	if list[0].Host != base || list[1].Host != "landing."+base || list[1].App || !list[1].Landing || !list[1].TenantMade {
		t.Fatalf("isi daftar = %+v", list)
	}
	if err := e.svc.DeleteSubdomain(ctx, e.a, sub); err != nil {
		t.Fatal(err)
	}

	for i := 0; i < 10; i++ {
		if _, err := e.svc.AddSubdomain(ctx, e.a, base, "s"+string(rune('a'+i))); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := e.svc.AddSubdomain(ctx, e.a, base, "extra"); !errors.Is(err, ErrDomainLimit) {
		t.Fatalf("batas = %v", err)
	}
}
