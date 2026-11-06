package aipolicy

import "testing"

func ptr[T any](v T) *T { return &v }

func TestApplyOverridesOnlySetFields(t *testing.T) {
	g := Default().Apply(Values{Domains: []string{DomainSales, DomainStock}, AllowCost: ptr(false), ChatLimit: ptr(5)})
	if g.AllowCost || g.ChatLimit != 5 || len(g.Domains) != 2 || !g.AllowWrite {
		t.Fatalf("global: %+v", g)
	}
	o := g.Apply(Values{AllowCost: ptr(true), Domains: []string{}}) // pengecualian: nil ikut global, [] = semua domain mati
	if !o.AllowCost || o.ChatLimit != 5 || len(o.Domains) != 0 {
		t.Fatalf("override: %+v", o)
	}
	if o.DomainAllowed("sales_summary") || !o.DomainAllowed("list_outlets") {
		t.Fatal("domain kosong harus memblokir tool berdomain, list_outlets tetap boleh")
	}
}

func TestBlockedOrder(t *testing.T) {
	e := Default().Apply(Values{Domains: []string{DomainStock}})
	b := e.Blocked()
	if len(b) != len(Domains)-1 || b[0] != DomainSales {
		t.Fatalf("blocked: %v", b)
	}
}

func TestValidate(t *testing.T) {
	ok := func(string) bool { return true }
	if f := (Values{}).Validate(false, ok); len(f) != 0 {
		t.Errorf("pengecualian kosong harus valid: %v", f)
	}
	if f := (Values{}).Validate(true, ok); len(f) == 0 {
		t.Error("global kosong harus ditolak")
	}
	full := Values{Enabled: ptr(true), Domains: []string{DomainSales}, AllowWrite: ptr(true), AllowCost: ptr(true), AllowBYOK: ptr(true), AllowCustom: ptr(true),
		AllowImages: ptr(true), AllowHistory: ptr(true), AllowMCP: ptr(true), FreeEnabled: ptr(true), ChatLimit: ptr(20), MaxRangeDays: ptr(30)}
	if f := full.Validate(true, ok); len(f) != 0 {
		t.Errorf("global lengkap harus valid: %v", f)
	}
	bad := full
	bad.Domains, bad.ChatLimit, bad.MaxRangeDays, bad.FreeDailyLimit = []string{"hack"}, ptr(0), ptr(400), ptr(-1)
	bad.FreeModel = ptr("x")
	f := bad.Validate(true, func(string) bool { return false })
	for _, k := range []string{"domains", "chat_limit", "max_range_days", "free_daily_limit", "free_model"} {
		if f[k] == "" {
			t.Errorf("%s harus ditolak", k)
		}
	}
}

func TestMembersDomainAndToolCatalog(t *testing.T) {
	if DomainOf("member_overview") != DomainMembers {
		t.Fatal("member_overview harus di domain members")
	}
	tools := DomainTools()
	for _, d := range Domains {
		if len(tools[d]) == 0 {
			t.Errorf("domain %s tidak punya tool", d)
		}
	}
	if len(tools[""]) != 0 {
		t.Errorf("semua tool terpetakan harus punya domain: %v", tools[""])
	}
	e := Default().Apply(Values{Domains: []string{DomainSales}})
	if e.DomainAllowed("member_overview") || !e.DomainAllowed("sales_summary") {
		t.Fatal("membuka sales tidak boleh otomatis membuka members")
	}
}
