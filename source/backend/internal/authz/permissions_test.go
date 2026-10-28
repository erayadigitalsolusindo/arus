package authz

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

func TestNormalizePosOnlyIsFixedBundle(t *testing.T) {
	p, err := Normalize(map[string][]string{"pos_only": {"view"}, "users": {"view", "update"}, "items": {"view", "create"}})
	if err != nil {
		t.Fatal(err)
	}
	want := PosOnlyGrants()
	if len(p.Grants) != len(want) {
		t.Fatalf("grants = %v, want exactly %v", p.Grants, want)
	}
	for m, acts := range want {
		if !p.Has(m, acts[0]) || len(p.Grants[m]) != len(acts) {
			t.Errorf("modul %s = %v, want %v", m, p.Grants[m], acts)
		}
	}
	if p.Has("users", ActView) {
		t.Error("Hanya Kasir tidak boleh membawa izin lain")
	}
	if _, err := Normalize(map[string][]string{"pos_only": {"view"}, "tidak_ada": {"view"}}); err == nil {
		t.Error("modul tak dikenal harus tetap ditolak")
	}
}

func TestPlanLockedModules(t *testing.T) {
	if LockedFor(nil) != nil {
		t.Fatal("paket tanpa daftar modul = semua modul, tidak boleh ada yang terkunci")
	}
	locked := LockedFor([]string{"items"})
	if locked["items"] || locked["subscription"] {
		t.Fatalf("items (dalam paket) dan subscription (selalu terbuka) tidak boleh terkunci: %v", locked)
	}
	if !locked["journals"] || !locked["users"] {
		t.Fatal("modul di luar paket harus terkunci")
	}

	// Owner (`*`) pun tidak menembus kunci paket; modul lain tetap lolos.
	owner := Permissions{All: true, Locked: locked}
	if owner.Has("journals", ActView) || !owner.Has("items", ActDelete) || !owner.Has("subscription", ActView) {
		t.Fatal("Owner: modul terkunci harus ditolak, modul paket dan subscription tetap boleh")
	}
	// Role biasa: izin role ∩ modul paket.
	role := Permissions{Grants: map[string][]string{"items": {ActView}, "journals": {ActView}}, Locked: locked}
	if !role.Has("items", ActView) || role.Has("journals", ActView) {
		t.Fatal("role: izin harus dipotong oleh paket")
	}

	raw, err := json.Marshal(role)
	if err != nil {
		t.Fatal(err)
	}
	var out map[string]json.RawMessage
	_ = json.Unmarshal(raw, &out)
	if _, ok := out["journals"]; ok {
		t.Error("modul terkunci tidak boleh muncul sebagai izin di JSON klien")
	}
	var lockedIDs []string
	_ = json.Unmarshal(out["_locked"], &lockedIDs)
	if !slices.Equal(lockedIDs, []string{"journals"}) {
		t.Errorf("_locked = %v, want [journals] (hanya modul terkunci yang role-nya mengizinkan)", lockedIDs)
	}
	if !ValidModuleIDs([]string{"items", "users"}) || ValidModuleIDs([]string{"items", "nope"}) {
		t.Error("ValidModuleIDs salah")
	}
}
