package purchasing

import (
	"context"
	"errors"
	"testing"
)

// Keyset: tiga halaman (3+3+1) dari tujuh nota tanpa duplikat dan tanpa terlewat; cursor rusak ditolak.
func TestListKeysetPagination(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	a := e.actor(e.outlet)
	it := e.item(t, "goods", "0", 0)
	seen := map[string]bool{}
	for i := 0; i < 7; i++ {
		r := e.req(line(it, "1", "0", "1000"))
		p, _, err := e.svc.Create(ctx, a, key(), r)
		if err != nil {
			t.Fatal(err)
		}
		seen[p.ID.String()] = false
	}

	cursor := ""
	pages := 0
	for {
		res, err := e.svc.List(ctx, a, ListParams{Limit: 3, Cursor: cursor})
		if err != nil {
			t.Fatalf("halaman %d: %v", pages+1, err)
		}
		pages++
		for _, r := range res.Data {
			if _, dup := seen[r.ID.String()]; !dup {
				t.Fatalf("baris tak dikenal: %s", r.ID)
			}
			if seen[r.ID.String()] {
				t.Fatalf("duplikat di halaman %d: %s", pages, r.ID)
			}
			seen[r.ID.String()] = true
		}
		if !res.HasMore {
			if len(res.Data) != 1 || res.NextCursor != "" {
				t.Errorf("halaman terakhir: %d baris, cursor %q", len(res.Data), res.NextCursor)
			}
			break
		}
		if len(res.Data) != 3 || res.NextCursor == "" {
			t.Fatalf("halaman %d: %d baris, cursor %q", pages, len(res.Data), res.NextCursor)
		}
		cursor = res.NextCursor
	}
	if pages != 3 {
		t.Errorf("jumlah halaman = %d, mau 3", pages)
	}
	for id, ok := range seen {
		if !ok {
			t.Errorf("nota %s terlewat", id)
		}
	}

	var fe FieldErrors
	if _, err := e.svc.List(ctx, a, ListParams{Cursor: "bukan-cursor"}); !errors.As(err, &fe) || fe["cursor"] != "INVALID" {
		t.Errorf("cursor rusak: %v", err)
	}
}

// Riwayat: Get memuat catatan audit "purchase.create" beserta pelaku dan waktunya.
func TestGetReturnsAuditEvents(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	a := e.actor(e.outlet)
	it := e.item(t, "goods", "0", 0)
	p, _, err := e.svc.Create(ctx, a, key(), e.req(line(it, "1", "0", "1000")))
	if err != nil {
		t.Fatal(err)
	}
	got, err := e.svc.Get(ctx, a, p.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Events) != 1 || got.Events[0].Action != "purchase.create" || got.Events[0].At.IsZero() {
		t.Fatalf("riwayat: %+v", got.Events)
	}
}
