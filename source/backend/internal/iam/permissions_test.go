package iam

import (
	"encoding/json"
	"errors"
	"slices"
	"testing"
)

func TestNormalize(t *testing.T) {
	p, err := Normalize(map[string][]string{"items": {"delete", "create", "create"}, "stock_card": {}, "units": {"view"}})
	if err != nil {
		t.Fatal(err)
	}
	if got := p.Grants["items"]; !slices.Equal(got, []string{"view", "create", "delete"}) {
		t.Errorf("items = %v, want [view create delete] (view otomatis, urutan registri, tanpa duplikat)", got)
	}
	if _, ok := p.Grants["stock_card"]; ok {
		t.Error("modul tanpa aksi tidak boleh tersimpan")
	}

	for name, in := range map[string]map[string][]string{
		"modul asing":        {"gudang_rahasia": {"view"}},
		"aksi tak berlaku":   {"stock_card": {"delete"}},
		"wildcard lewat API": {"*": {"view"}},
		"aksi asing":         {"items": {"hack"}},
	} {
		if _, err := Normalize(in); !errors.Is(err, ErrInvalidPermissions) {
			t.Errorf("%s: err = %v, want ErrInvalidPermissions", name, err)
		}
	}
}

func TestHasAndCovers(t *testing.T) {
	owner := Permissions{All: true}
	kasir, _ := Normalize(map[string][]string{"items": {"view"}, "sales_orders": {"create"}})
	admin, _ := Normalize(map[string][]string{"items": {"view", "create", "update"}, "sales_orders": {"create"}, "roles": {"update"}})

	if !owner.Has("anything", "x") || !kasir.Has("items", "view") || kasir.Has("items", "create") || kasir.Has("users", "view") {
		t.Error("Has salah")
	}
	if !owner.Covers(admin) || !owner.Covers(owner) {
		t.Error("Owner harus bisa memberi apa saja")
	}
	if !admin.Covers(kasir) {
		t.Error("admin harus mencakup kasir")
	}
	if kasir.Covers(admin) {
		t.Error("kasir tidak boleh memberi izin lebih besar (eskalasi)")
	}
	if admin.Covers(owner) {
		t.Error("non-Owner tidak boleh memberi wildcard")
	}
}

func TestStoredRoundTrip(t *testing.T) {
	if p := ParseStored([]byte(`{"*":true}`)); !p.All {
		t.Error("wildcard tidak terbaca")
	}
	// Modul/aksi usang dibuang, bukan error; JSON rusak = tanpa izin (tertutup).
	p := ParseStored([]byte(`{"items":["view","bogus"],"dihapus":["view"],"stock_card":["delete"]}`))
	if !slices.Equal(p.Grants["items"], []string{"view"}) || len(p.Grants) != 1 {
		t.Errorf("parse longgar salah: %+v", p)
	}
	if p := ParseStored([]byte(`bukan json`)); p.All || len(p.Grants) != 0 {
		t.Error("JSON rusak harus tanpa izin")
	}
	b, _ := json.Marshal(Permissions{All: true})
	if string(b) != `{"*":true}` {
		t.Errorf("marshal owner = %s", b)
	}
	b, _ = json.Marshal(Permissions{})
	if string(b) != `{}` {
		t.Errorf("marshal kosong = %s", b)
	}
}

func TestRegistryIsConsistent(t *testing.T) {
	seen := map[string]bool{}
	for _, m := range Modules {
		if seen[m.ID] {
			t.Errorf("modul ganda: %s", m.ID)
		}
		seen[m.ID] = true
		if !slices.Contains(m.Actions, ActView) {
			t.Errorf("%s tidak punya aksi view", m.ID)
		}
	}
}
