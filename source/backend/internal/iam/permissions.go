// Package iam: role, permission, dan manajemen pengguna dalam satu tenant (PRD FR-AUTH-05).
package iam

import (
	"encoding/json"
	"errors"
	"fmt"
	"slices"
)

// Aksi yang dikenal. Setiap modul hanya menerima subset-nya (lihat Modules).
const (
	ActView    = "view"
	ActCreate  = "create"
	ActUpdate  = "update"
	ActDelete  = "delete"
	ActApprove = "approve"
)

// Module = satu unit izin; ID-nya sama dengan id menu di sidebar (web/src/lib/nav.ts) agar UI bisa menyaring menu.
type Module struct {
	ID      string   `json:"id"`
	Actions []string `json:"actions"`
}

var (
	crud    = []string{ActView, ActCreate, ActUpdate, ActDelete}
	viewing = []string{ActView}
)

// Modules = registri izin yang sah, berurutan seperti di UI (matriks hak akses). Menambah modul baru = tambah
// baris di sini (+ kamus i18n `perm.module.<id>`); role lama otomatis tidak punya izinnya (default tertutup).
var Modules = []Module{
	{"siak", viewing},
	{"items", crud},
	{"stock_card", viewing},
	{"coupons", crud},
	{"suppliers", crud},
	{"members", crud},
	{"salespeople", crud},
	{"units", crud},
	{"categories", crud},
	{"member_categories", crud},
	{"payment_methods", crud},
	{"brands", crud},
	{"principals", crud},
	{"sales_orders", []string{ActView, ActCreate, ActUpdate, ActDelete, ActApprove}},
	{"sales_returns", []string{ActView, ActCreate, ActApprove}},
	{"sales_list", viewing},
	{"sell_price_history", viewing},
	{"member_receivables", []string{ActView, ActCreate}},
	{"order_list", crud},
	{"purchase_invoices", []string{ActView, ActCreate, ActUpdate, ActDelete, ActApprove}},
	{"purchase_returns", []string{ActView, ActCreate, ActApprove}},
	{"purchase_list", viewing},
	{"buy_price_history", viewing},
	{"supplier_payables", []string{ActView, ActCreate}},
	{"stock_opname", []string{ActView, ActCreate, ActApprove}},
	{"stock_transfer", []string{ActView, ActCreate, ActApprove}},
	{"users", crud},
	{"roles", crud},
}

func moduleByID(id string) (Module, bool) {
	i := slices.IndexFunc(Modules, func(m Module) bool { return m.ID == id })
	if i < 0 {
		return Module{}, false
	}
	return Modules[i], true
}

// Permissions = izin efektif sebuah role. Bentuk JSON (disimpan di roles.permissions dan dikirim ke klien):
//
//	{"*": true}                                  → semua izin (hanya role sistem Owner)
//	{"items": ["view","create"], "users": [...]} → izin per modul
type Permissions struct {
	All    bool
	Grants map[string][]string
}

var ErrInvalidPermissions = errors.New("izin tidak valid")

func (p Permissions) Has(module, action string) bool {
	return p.All || slices.Contains(p.Grants[module], action)
}

// Covers: apakah p boleh memberikan `target` kepada orang lain. Mencegah eskalasi hak: pemegang izin
// "ubah role" tidak bisa membuat role (atau menaikkan dirinya) melebihi izinnya sendiri.
func (p Permissions) Covers(target Permissions) bool {
	if p.All {
		return true
	}
	if target.All {
		return false
	}
	for m, acts := range target.Grants {
		for _, a := range acts {
			if !slices.Contains(p.Grants[m], a) {
				return false
			}
		}
	}
	return true
}

func (p Permissions) MarshalJSON() ([]byte, error) {
	if p.All {
		return []byte(`{"*":true}`), nil
	}
	g := p.Grants
	if g == nil {
		g = map[string][]string{}
	}
	return json.Marshal(g)
}

// ParseStored membaca roles.permissions dari DB. Longgar terhadap modul/aksi yang sudah tidak ada (dibuang),
// agar menghapus modul dari registri tidak merusak role lama. JSON rusak = tanpa izin (tertutup).
func ParseStored(raw []byte) Permissions {
	var m map[string]json.RawMessage
	if json.Unmarshal(raw, &m) != nil {
		return Permissions{}
	}
	if v, ok := m["*"]; ok {
		var b bool
		if json.Unmarshal(v, &b) == nil && b {
			return Permissions{All: true}
		}
	}
	grants := map[string][]string{}
	for id, v := range m {
		mod, ok := moduleByID(id)
		if !ok {
			continue
		}
		var acts []string
		if json.Unmarshal(v, &acts) != nil {
			continue
		}
		for _, a := range acts {
			if slices.Contains(mod.Actions, a) && !slices.Contains(grants[id], a) {
				grants[id] = append(grants[id], a)
			}
		}
	}
	return Permissions{Grants: grants}
}

// Normalize memvalidasi izin dari klien secara ketat: modul/aksi tak dikenal ditolak, duplikat dibuang, dan
// "view" ikut diberikan bila ada aksi lain (tidak ada izin ubah tanpa izin lihat). Wildcard "*" tidak boleh
// dibuat lewat API. Hasilnya berurutan sesuai registri.
func Normalize(in map[string][]string) (Permissions, error) {
	out := map[string][]string{}
	for id, acts := range in {
		mod, ok := moduleByID(id)
		if !ok {
			return Permissions{}, fmt.Errorf("%w: modul %q tidak dikenal", ErrInvalidPermissions, id)
		}
		for _, a := range acts {
			if !slices.Contains(mod.Actions, a) {
				return Permissions{}, fmt.Errorf("%w: aksi %q tidak berlaku untuk modul %q", ErrInvalidPermissions, a, id)
			}
		}
		if len(acts) == 0 {
			continue
		}
		set := map[string]bool{ActView: true}
		for _, a := range acts {
			set[a] = true
		}
		for _, a := range mod.Actions { // urutan registri, bukan urutan masukan
			if set[a] {
				out[id] = append(out[id], a)
			}
		}
	}
	return Permissions{Grants: out}, nil
}
