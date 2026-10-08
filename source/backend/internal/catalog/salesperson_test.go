package catalog

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/google/uuid"
)

func TestValidateSalesperson(t *testing.T) {
	c, f := validateSalesperson(SalespersonInput{Code: "S-01", Name: "  Budi   Santoso ", Phone: "0812 3456 7890", CommissionPct: "2.5"})
	if f != nil || c.name != "Budi Santoso" || c.phone != "+6281234567890" || c.commission.StringFixed(2) != "2.50" || c.code.String != "S-01" {
		t.Fatalf("valid: %+v %v", c, f)
	}
	if c, f := validateSalesperson(SalespersonInput{Name: "X"}); f != nil || c.code.Valid || !c.commission.IsZero() {
		t.Errorf("opsional kosong: %+v %v", c, f)
	}
	for name, in := range map[string]SalespersonInput{
		"nama kosong":      {Name: " "},
		"nama markup":      {Name: "<b>x</b>"},
		"kode berspasi":    {Name: "X", Code: "A B"},
		"hp salah":         {Name: "X", Phone: "abc"},
		"catatan panjang":  {Name: "X", Note: strings.Repeat("a", maxNote+1)},
		"komisi negatif":   {Name: "X", CommissionPct: "-1"},
		"komisi > 100":     {Name: "X", CommissionPct: "100.01"},
		"komisi 3 desimal": {Name: "X", CommissionPct: "1.234"},
	} {
		if _, f := validateSalesperson(in); f == nil {
			t.Errorf("%s: seharusnya ditolak", name)
		}
	}
}

func TestSalespersonLifecycle(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	s, err := e.svc.CreateSalesperson(ctx, e.a, SalespersonInput{Code: "S-01", Name: "Budi", CommissionPct: "1.5"})
	if err != nil || !s.Active || s.CommissionPct != "1.50" {
		t.Fatalf("buat: %+v %v", s, err)
	}
	// Nama & kode unik per tenant (tanpa membedakan huruf); tenant lain bebas.
	if _, err := e.svc.CreateSalesperson(ctx, e.a, SalespersonInput{Name: "BUDI"}); !errors.Is(err, ErrNameTaken) {
		t.Errorf("nama ganda: %v", err)
	}
	if _, err := e.svc.CreateSalesperson(ctx, e.a, SalespersonInput{Code: "s-01", Name: "Ani"}); !errors.Is(err, ErrCodeTaken) {
		t.Errorf("kode ganda: %v", err)
	}
	if _, err := e.svc.CreateSalesperson(ctx, e.b, SalespersonInput{Code: "S-01", Name: "Budi"}); err != nil {
		t.Errorf("tenant lain: %v", err)
	}
	// Ubah, arsip, aktifkan, filter.
	u, err := e.svc.UpdateSalesperson(ctx, e.a, s.ID, SalespersonInput{Code: "S-01", Name: "Budi S", Phone: "08123456789"})
	if err != nil || u.Name != "Budi S" || u.CommissionPct != "0.00" {
		t.Fatalf("ubah: %+v %v", u, err)
	}
	if _, err := e.svc.UpdateSalesperson(ctx, e.b, s.ID, SalespersonInput{Name: "Curi"}); !errors.Is(err, ErrNotFound) {
		t.Errorf("ubah lintas tenant: %v", err)
	}
	if _, err := e.svc.UpdateSalesperson(ctx, e.a, uuid.New(), SalespersonInput{Name: "X"}); !errors.Is(err, ErrNotFound) {
		t.Errorf("ubah id acak: %v", err)
	}
	if r, err := e.svc.SetSalespersonActive(ctx, e.a, s.ID, false); err != nil || r.Active {
		t.Fatalf("arsip: %+v %v", r, err)
	}
	f := false
	if l, total, err := e.svc.ListSalespeople(ctx, e.a, ListParams{Active: &f}); err != nil || total != 1 || len(l) != 1 {
		t.Errorf("filter arsip: %d %v", total, err)
	}
	if l, _, _ := e.svc.ListSalespeople(ctx, e.b, ListParams{}); len(l) != 1 || l[0].ID == s.ID {
		t.Errorf("isolasi tenant: %+v", l)
	}
	if l, _, _ := e.svc.ListSalespeople(ctx, e.a, ListParams{Q: "budi"}); len(l) != 1 {
		t.Errorf("cari: %+v", l)
	}
}
