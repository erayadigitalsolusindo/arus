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

// Induk otomatis: kotakcantik.com ditawarkan untuk arus.kotakcantik.com hanya bila tak ada tenant lain di bawahnya.
func TestTenantDomainParents(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	p := "uji" + uuid.NewString()[:8] + ".com"
	t.Cleanup(func() { _, _ = e.admin.Exec(ctx, `DELETE FROM tenant_domains WHERE host LIKE '%' || $1`, p) })
	add := func(tenant uuid.UUID, host string) {
		t.Helper()
		if _, err := e.admin.Exec(ctx, `INSERT INTO tenant_domains (tenant_id, host, status, verified_at) VALUES ($1, $2, 'active', now())`, tenant, host); err != nil {
			t.Fatal(err)
		}
	}
	add(e.a.TenantID, "arus."+p)
	got, err := e.svc.Parents(ctx, e.a, "app.erayadigital.co.id")
	if err != nil || len(got) != 1 || got[0] != p {
		t.Fatalf("induk = %v %v", got, err)
	}
	// Alamat aplikasi ARUS sendiri: induknya tak ditawarkan.
	if got, _ := e.svc.Parents(ctx, e.a, "x."+p); len(got) != 0 {
		t.Fatalf("induk target = %v", got)
	}
	// Tenant lain punya host di bawah induk yang sama: tak ditawarkan ke siapa pun.
	add(e.b.TenantID, "toko."+p)
	if got, _ := e.svc.Parents(ctx, e.a, "app.erayadigital.co.id"); len(got) != 0 {
		t.Fatalf("induk bersama = %v", got)
	}
	if _, err := e.svc.AddParent(ctx, e.a, p, "app.erayadigital.co.id"); !errors.Is(err, ErrDomainBase) {
		t.Fatalf("add induk bersama = %v", err)
	}
	e.sql(t, `DELETE FROM tenant_domains WHERE host = $1`, "toko."+p)

	id, err := e.svc.AddParent(ctx, e.a, p, "app.erayadigital.co.id")
	if err != nil {
		t.Fatal(err)
	}
	// Setelah jadi host: tak ditawarkan lagi, bisa dipakai sebagai induk sub-domain, dan peran bisa diubah.
	if got, _ := e.svc.Parents(ctx, e.a, "app.erayadigital.co.id"); len(got) != 0 {
		t.Fatalf("induk sudah jadi host = %v", got)
	}
	if _, err := e.svc.AddSubdomain(ctx, e.a, p, "landing"); err != nil {
		t.Fatal(err)
	}
	if err := e.svc.SetDomainRole(ctx, e.a, id, true, true); err != nil {
		t.Fatal(err)
	}
	// Akhiran dua-tingkat umum tak pernah jadi induk.
	add(e.a.TenantID, "arus.uji"+p[3:8]+".co.id")
	for _, h := range func() []string { g, _ := e.svc.Parents(ctx, e.a, "x.y.z"); return g }() {
		if h == "co.id" {
			t.Fatal("co.id tidak boleh jadi induk")
		}
	}
}

// Domain induk untuk bentuk domain umum: .com, .co.id, .id, .my.id; akhiran milik registrar tak pernah jadi induk.
func TestDomainParentForms(t *testing.T) {
	e := newEnv(t)
	for host, want := range map[string]string{
		"arus.domain.com": "domain.com", "arus.domain.co.id": "domain.co.id", "arus.domain.id": "domain.id", "arus.domain.my.id": "domain.my.id",
		"domain.com": "", "domain.co.id": "", "domain.id": "", "domain.my.id": "",
	} {
		var got *string
		if err := e.admin.QueryRow(context.Background(), `SELECT domain_parent($1)`, host).Scan(&got); err != nil {
			t.Fatal(err)
		}
		if g := ""; got != nil {
			g = *got
			if g != want {
				t.Errorf("domain_parent(%s) = %q, want %q", host, g, want)
			}
		} else if want != "" {
			t.Errorf("domain_parent(%s) = NULL, want %q", host, want)
		}
	}
}
