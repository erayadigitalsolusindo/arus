package sales

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
)

func TestSaleSalesperson(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	a := e.actor(e.tenant)
	it := e.item(t, "goods", "50000", "30000", 20, false)
	sp, off := uuid.New(), uuid.New()
	e.exec(t, `INSERT INTO salespeople (id, tenant_id, name) VALUES ($1, $2, 'Budi')`, sp, e.tenant)
	e.exec(t, `INSERT INTO salespeople (id, tenant_id, name, active) VALUES ($1, $2, 'Lama', false)`, off, e.tenant)

	s, _, err := e.svc.Create(ctx, a, key(), Request{Lines: []LineIn{line(it, "1")}, Payments: []PaymentIn{pay("cash", "50000")}, SalespersonID: &sp})
	if err != nil || s.Salesperson == nil || s.Salesperson.ID != sp || s.Salesperson.Name != "Budi" {
		t.Fatalf("dengan salesman: %+v %v", s, err)
	}
	// Tanpa salesman = Umum.
	s2, _, err := e.svc.Create(ctx, a, key(), Request{Lines: []LineIn{line(it, "1")}, Payments: []PaymentIn{pay("cash", "50000")}})
	if err != nil || s2.Salesperson != nil {
		t.Fatalf("tanpa salesman: %+v %v", s2, err)
	}
	// Tidak aktif / tidak ada / milik tenant lain ditolak dan tidak ada nota/stok berubah.
	for name, id := range map[string]uuid.UUID{"nonaktif": off, "acak": uuid.New()} {
		id := id
		_, _, err := e.svc.Create(ctx, a, key(), Request{Lines: []LineIn{line(it, "1")}, Payments: []PaymentIn{pay("cash", "50000")}, SalespersonID: &id})
		var fe FieldErrors
		if !errors.As(err, &fe) || fe["salesperson_id"] == "" {
			t.Errorf("%s: err = %v, want galat salesperson_id", name, err)
		}
	}
	// Kunci idempotensi sama dengan salesman berbeda = isi beda.
	k := key()
	if _, _, err := e.svc.Create(ctx, a, k, Request{Lines: []LineIn{line(it, "1")}, Payments: []PaymentIn{pay("cash", "50000")}, SalespersonID: &sp}); err != nil {
		t.Fatal(err)
	}
	if _, _, err := e.svc.Create(ctx, a, k, Request{Lines: []LineIn{line(it, "1")}, Payments: []PaymentIn{pay("cash", "50000")}}); !errors.Is(err, ErrKeyMismatch) {
		t.Errorf("hash salesman: err = %v, want ErrKeyMismatch", err)
	}
}
