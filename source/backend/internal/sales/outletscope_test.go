package sales

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
)

// Isolasi antar cabang dalam satu tenant: RLS hanya memisahkan tenant, jadi akses cabang dijaga service.
func TestSaleReadIsScopedToAccessibleOutlets(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	it := e.item(t, "goods", "1000", "0", 5, false)
	a := e.actor(e.tenant)
	a.Outlets = map[uuid.UUID]bool{e.outlet: true}
	s, _, err := e.svc.Create(ctx, a, key(), Request{Lines: []LineIn{line(it, "1")}, Payments: []PaymentIn{pay("cash", "1000")}})
	if err != nil {
		t.Fatal(err)
	}

	outlet2 := uuid.New()
	e.exec(t, `INSERT INTO outlets (id, tenant_id, code, name) VALUES ($1, $2, 'cab2', 'Cabang Dua')`, outlet2, e.tenant)
	// Pemegang sales_list.view yang hanya punya akses cabang 2 tidak boleh membuka nota cabang 1 — walau tahu id-nya.
	blind := a
	blind.OutletID = outlet2
	blind.Outlets = map[uuid.UUID]bool{outlet2: true}
	blind.Perms.All = true
	if _, err := e.svc.Get(ctx, blind, s.ID); !errors.Is(err, ErrOutletForbidden) {
		t.Fatalf("Get lintas cabang harus ditolak: %v", err)
	}
	if _, err := e.svc.Detail(ctx, blind, s.ID); !errors.Is(err, ErrOutletForbidden) {
		t.Fatalf("Detail lintas cabang harus ditolak: %v", err)
	}
	// Punya akses ke kedua cabang (walau cabang aktif = 2) → boleh.
	both := blind
	both.Outlets = map[uuid.UUID]bool{e.outlet: true, outlet2: true}
	if got, err := e.svc.Get(ctx, both, s.ID); err != nil || got.ID != s.ID {
		t.Fatalf("akses dua cabang: %v", err)
	}
}

// Kunci idempotensi dari cabang lain tidak boleh mengembalikan nota cabang pertama.
func TestIdempotencyKeyDoesNotLeakAcrossOutlets(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	it := e.item(t, "goods", "1000", "0", 5, false)
	a := e.actor(e.tenant)
	k := key()
	req := Request{Lines: []LineIn{line(it, "1")}, Payments: []PaymentIn{pay("cash", "1000")}}
	s, _, err := e.svc.Create(ctx, a, k, req)
	if err != nil {
		t.Fatal(err)
	}
	outlet2 := uuid.New()
	e.exec(t, `INSERT INTO outlets (id, tenant_id, code, name) VALUES ($1, $2, 'cab2', 'Cabang Dua')`, outlet2, e.tenant)
	b := a
	b.OutletID = outlet2
	b.Outlets = map[uuid.UUID]bool{outlet2: true}
	got, replayed, err := e.svc.Create(ctx, b, k, req)
	if err == nil && (replayed || got.ID == s.ID) {
		t.Fatalf("kunci yang sama dari cabang lain mengembalikan nota cabang pertama (replayed=%v)", replayed)
	}
	if err != nil && !errors.Is(err, ErrKeyMismatch) {
		t.Logf("ditolak dengan: %v", err) // stok cabang 2 kosong dsb. juga diterima; yang penting bukan replay
	}
}
