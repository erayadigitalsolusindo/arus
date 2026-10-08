package stock

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/google/uuid"

	"aciraba/internal/authz"
)

func (e *env) cardRows(t *testing.T, svc *Service, a authz.Actor, p CardParams) CardResult {
	t.Helper()
	res, err := svc.Card(context.Background(), a, p)
	if err != nil {
		t.Fatal(err)
	}
	return res
}

func balances(rows []CardRow) string {
	out := make([]string, len(rows))
	for i, r := range rows {
		out[i] = r.Balance
	}
	return strings.Join(out, ",")
}

func TestStockCard(t *testing.T) {
	e := newEnv(t)
	svc := NewService(e.app)
	ctx := context.Background()
	a := e.convActor(t, e.outlet)
	item := e.item(t, e.tenant, "goods", false)

	// Display +10, Gudang +5 (dimundurkan 5 hari), lalu display -3 dan +2 (hari ini).
	for _, m := range []Movement{
		e.mv(item, BucketDisplay, "10", RefOpening), e.mv(item, BucketWarehouse, "5", RefOpening),
		e.mv(item, BucketDisplay, "-3", RefSale), e.mv(item, BucketDisplay, "2", RefAdjustment),
	} {
		m.ActorID = a.UserID
		if _, err := e.apply(t, m); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := e.admin.Exec(ctx, `UPDATE stock_movements SET created_at = now() - interval '5 days' WHERE item_id = $1 AND ref_type = 'OPENING'`, item); err != nil {
		t.Fatal(err)
	}

	// Periode bawaan (30 hari): semua mutasi, saldo awal 0.
	all := e.cardRows(t, svc, a, CardParams{ItemID: item})
	if all.Opening != "0" || all.In != "17" || all.Out != "3" || all.Closing != "14" || balances(all.Rows) != "10,15,12,14" {
		t.Fatalf("periode bawaan: %+v balances=%s", all, balances(all.Rows))
	}
	if all.Rows[0].Actor != "Kasir Uji" || all.Rows[0].RefType != RefOpening {
		t.Fatalf("baris: %+v", all.Rows[0])
	}
	today := all.To

	// Hanya hari ini: saldo awal = saldo akhir hari-hari sebelumnya (semua bucket).
	day := e.cardRows(t, svc, a, CardParams{ItemID: item, From: today, To: today})
	if day.Opening != "15" || day.In != "2" || day.Out != "3" || day.Closing != "14" || balances(day.Rows) != "12,14" {
		t.Fatalf("hari ini: %+v balances=%s", day, balances(day.Rows))
	}

	// Filter bucket: display saja.
	disp := e.cardRows(t, svc, a, CardParams{ItemID: item, From: today, To: today, Bucket: BucketDisplay})
	if disp.Opening != "10" || disp.Closing != "9" || balances(disp.Rows) != "7,9" {
		t.Fatalf("display: %+v balances=%s", disp, balances(disp.Rows))
	}
	wh := e.cardRows(t, svc, a, CardParams{ItemID: item, Bucket: BucketWarehouse})
	if wh.Closing != "5" || len(wh.Rows) != 1 {
		t.Fatalf("gudang: %+v", wh)
	}

	// Paginasi keyset: satu baris per halaman, saldo berjalan tersambung, tanpa baris ganda/hilang.
	var seen []CardRow
	cursor := int64(0)
	for i := 0; i < 10; i++ {
		page := e.cardRows(t, svc, a, CardParams{ItemID: item, Limit: 1, After: cursor})
		seen = append(seen, page.Rows...)
		if page.NextCursor == nil {
			break
		}
		cursor = *page.NextCursor
	}
	if len(seen) != 4 || balances(seen) != "10,15,12,14" {
		t.Fatalf("paginasi: %d baris, saldo %s", len(seen), balances(seen))
	}

	// Outlet lain tidak melihat mutasi outlet ini.
	other := e.convActor(t, e.outlet2)
	if r := e.cardRows(t, svc, other, CardParams{ItemID: item}); len(r.Rows) != 0 || r.Closing != "0" {
		t.Fatalf("outlet lain: %+v", r)
	}
}

func TestStockCardValidation(t *testing.T) {
	e := newEnv(t)
	svc := NewService(e.app)
	ctx := context.Background()
	a := e.convActor(t, e.outlet)
	item := e.item(t, e.tenant, "goods", false)

	for name, p := range map[string]CardParams{
		"tanpa barang":    {},
		"bucket asing":    {ItemID: item, Bucket: "gudang"},
		"tanggal rusak":   {ItemID: item, From: "2026-13-40"},
		"terbalik":        {ItemID: item, From: "2026-02-10", To: "2026-02-01"},
		"terlalu panjang": {ItemID: item, From: "2020-01-01", To: "2026-01-01"},
		"kursor negatif":  {ItemID: item, After: -1},
	} {
		var fe FieldErrors
		if _, err := svc.Card(ctx, a, p); !errors.As(err, &fe) {
			t.Errorf("%s: harus FieldErrors, dapat %v", name, err)
		}
	}
	if _, err := svc.Card(ctx, a, CardParams{ItemID: uuid.New()}); !errors.Is(err, ErrItemNotFound) {
		t.Errorf("barang tak ada: %v", err)
	}
	if _, err := svc.Card(ctx, a, CardParams{ItemID: e.item(t, e.other, "goods", false)}); !errors.Is(err, ErrItemNotFound) {
		t.Errorf("barang tenant lain: %v", err)
	}
	if _, err := svc.Card(ctx, a, CardParams{ItemID: e.item(t, e.tenant, "service", false)}); !errors.Is(err, ErrNotStocked) {
		t.Errorf("jasa: %v", err)
	}
}

func TestStockCardItems(t *testing.T) {
	e := newEnv(t)
	svc := NewService(e.app)
	ctx := context.Background()
	a := e.convActor(t, e.outlet)
	goods := e.item(t, e.tenant, "goods", false)
	e.item(t, e.tenant, "service", false)
	e.item(t, e.other, "goods", false)
	if _, err := e.admin.Exec(ctx, `UPDATE items SET active = false WHERE id = $1`, goods); err != nil {
		t.Fatal(err)
	}
	rows, err := svc.CardItems(ctx, a, "")
	if err != nil || len(rows) != 1 || rows[0].ID != goods {
		t.Fatalf("pemilih: %v %v", rows, err)
	}
	if rows, _ := svc.CardItems(ctx, a, "%"); len(rows) != 0 {
		t.Fatalf("wildcard harus di-escape: %v", rows)
	}
}
