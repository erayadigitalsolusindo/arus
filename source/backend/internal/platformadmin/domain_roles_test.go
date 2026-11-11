package platformadmin

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
)

// Satu domain didaftarkan: sub-host bebas, tiap host punya peran (aplikasi / landing / keduanya).
func TestDomainRolesAndSubhosts(t *testing.T) {
	e := newEnv(t, "")
	a := mustActor(t, e)
	ctx := context.Background()
	tn, other := e.newTenant(t), e.newTenant(t)
	base := "uji" + uuid.NewString()[:8] + ".com"
	t.Cleanup(func() { _, _ = e.admin.Exec(ctx, `DELETE FROM tenant_domains WHERE host LIKE '%' || $1`, base) })

	rootID, err := e.svc.AddDomain(ctx, a, tn.ID, base)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := e.admin.Exec(ctx, `UPDATE tenant_domains SET status = 'active' WHERE id = $1`, rootID); err != nil {
		t.Fatal(err)
	}
	status := func(id uuid.UUID) DomainRow {
		d, err := e.svc.getDomain(ctx, a, id)
		if err != nil {
			t.Fatal(err)
		}
		return d
	}
	if d := status(rootID); !d.App || d.Landing {
		t.Fatalf("bawaan = app %v landing %v, want app saja", d.App, d.Landing)
	}

	sub, err := e.svc.AddDomain(ctx, a, tn.ID, "landing."+base)
	if err != nil {
		t.Fatal(err)
	}
	if d := status(sub); d.Status != "active" {
		t.Fatalf("sub-host dari domain aktif milik tenant sama = %s, want active", d.Status)
	}
	// Tenant lain tidak ikut menumpang pada domain induk; "xbase" (akhiran tanpa titik) juga bukan sub-host.
	for _, c := range []struct {
		tenant uuid.UUID
		host   string
	}{{other.ID, "app." + base}, {tn.ID, "x" + base}} {
		id, err := e.svc.AddDomain(ctx, a, c.tenant, c.host)
		if err != nil {
			t.Fatal(err)
		}
		if d := status(id); d.Status != "pending" {
			t.Fatalf("%s = %s, want pending", c.host, d.Status)
		}
	}

	gate := func(host string) int {
		w := httptest.NewRecorder()
		e.router.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/public/domain-gate?host="+host, nil))
		return w.Code
	}
	off, on := false, true
	if err := e.svc.SetDomainRole(ctx, a, sub, &off, &on); err != nil {
		t.Fatal(err)
	}
	if d := status(sub); d.App || !d.Landing {
		t.Fatalf("landing saja = app %v landing %v", d.App, d.Landing)
	}
	if c := gate("landing." + base); c != http.StatusFound {
		t.Fatalf("gate landing saja = %d, want 302", c)
	}
	if c := gate(base); c != http.StatusNoContent {
		t.Fatalf("gate host aplikasi = %d, want 204", c)
	}
	if err := e.svc.SetDomainRole(ctx, a, sub, nil, &off); !errors.Is(err, ErrDomainInvalid) {
		t.Fatalf("tanpa peran = %v, want ErrDomainInvalid", err)
	}
}
