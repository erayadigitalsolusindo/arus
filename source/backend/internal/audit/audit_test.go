package audit

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"aciraba/internal/platform/db"
)

func TestScrubRemovesSecretLookingKeys(t *testing.T) {
	got := scrub(map[string]any{
		"name": "Budi", "password": "x", "new_password_hash": "y", "access_token": "z", "pin": "1",
		"nested": map[string]any{"secret_key": "s", "ok": 1}, "shipping": "tetap",
	})
	for _, k := range []string{"password", "new_password_hash", "access_token", "pin"} {
		if _, ok := got[k]; ok {
			t.Errorf("kunci %q seharusnya dibuang", k)
		}
	}
	nested := got["nested"].(map[string]any)
	if _, ok := nested["secret_key"]; ok || nested["ok"] != 1 {
		t.Errorf("scrub bersarang salah: %v", nested)
	}
	if got["name"] != "Budi" || got["shipping"] != "tetap" {
		t.Errorf("kunci biasa ikut terbuang: %v", got) // "shipping" mengandung "pin" tetapi bukan awalan
	}
}

func TestCursorRoundTripAndTamper(t *testing.T) {
	at := time.Unix(1700000000, 123456000).UTC()
	c := encodeCursor(at, 42)
	gotAt, gotID, err := decodeCursor(c)
	if err != nil || !gotAt.Equal(at) || gotID != 42 {
		t.Fatalf("round trip: %v %v %v", gotAt, gotID, err)
	}
	for _, bad := range []string{"", "bukan-base64!!", "YWJj", "MToxOjE"} {
		if bad == "" {
			continue
		}
		if _, _, err := decodeCursor(bad); err == nil {
			t.Errorf("kursor %q seharusnya ditolak", bad)
		}
	}
}

func TestEscapeLike(t *testing.T) {
	if got := escapeLike(`a_b%c\d`); got != `a\_b\%c\\d` {
		t.Errorf("escapeLike = %q", got)
	}
}

type fixture struct {
	app, admin *pgxpool.Pool
	svc        *Service
	tenant     uuid.UUID
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	appURL, adminURL := os.Getenv("TEST_DATABASE_URL"), os.Getenv("TEST_ADMIN_DATABASE_URL")
	if appURL == "" || adminURL == "" {
		t.Skip("TEST_DATABASE_URL/TEST_ADMIN_DATABASE_URL tidak di-set")
	}
	ctx := context.Background()
	app, err := pgxpool.New(ctx, appURL)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(app.Close)
	admin, err := pgxpool.New(ctx, adminURL)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(admin.Close)
	f := &fixture{app: app, admin: admin, svc: NewService(app), tenant: uuid.New()}
	if _, err := admin.Exec(ctx, `INSERT INTO tenants (id, code, name) VALUES ($1, $2, $3)`, f.tenant, "au-"+f.tenant.String()[:8], "UJI-AUDIT"); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = admin.Exec(ctx, `DELETE FROM audit_log WHERE tenant_id = $1`, f.tenant)
		_, _ = admin.Exec(ctx, `DELETE FROM tenants WHERE id = $1`, f.tenant)
	})
	return f
}

func (f *fixture) record(t *testing.T, tenant uuid.UUID, actor uuid.UUID, e Entry) {
	t.Helper()
	ctx := WithMeta(context.Background(), Meta{IP: "203.0.113.5", RequestID: "req-1"})
	err := db.WithTenant(ctx, f.app, tenant, func(tx pgx.Tx) error {
		return Record(ctx, tx, Actor{TenantID: tenant, UserID: actor, Name: "Penguji"}, e)
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestRecordListFilterAndPagination(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	alice, bob := uuid.New(), uuid.New()

	for i := range 5 {
		f.record(t, f.tenant, alice, Entry{Action: ActionRoleUpdate, Entity: EntityRole, EntityID: fmt.Sprint(i), Details: map[string]any{"i": i, "password": "rahasia"}})
	}
	f.record(t, f.tenant, bob, Entry{Action: ActionUserCreate, Entity: EntityUser, EntityID: "u1"})
	f.record(t, f.tenant, bob, Entry{Action: "auth.login", Entity: EntityUser})

	// Semua, terbaru dulu; metadata permintaan dan nama pelaku tersimpan; rahasia tidak ikut.
	page, err := f.svc.List(ctx, f.tenant, Filter{Limit: 100})
	if err != nil || len(page.Items) != 7 || page.NextCursor != "" {
		t.Fatalf("daftar: %d item next=%q err=%v", len(page.Items), page.NextCursor, err)
	}
	first := page.Items[0]
	if first.Action != "auth.login" || first.IP != "203.0.113.5" || first.RequestID != "req-1" || first.ActorName != "Penguji" {
		t.Errorf("item terbaru: %+v", first)
	}
	for _, it := range page.Items {
		if _, leaked := it.Details["password"]; leaked {
			t.Fatal("password bocor ke audit log")
		}
	}

	// Filter.
	if p, _ := f.svc.List(ctx, f.tenant, Filter{Entity: EntityRole}); len(p.Items) != 5 {
		t.Errorf("filter entity = %d, want 5", len(p.Items))
	}
	if p, _ := f.svc.List(ctx, f.tenant, Filter{ActionPrefix: "role."}); len(p.Items) != 5 {
		t.Errorf("filter awalan aksi = %d, want 5", len(p.Items))
	}
	if p, _ := f.svc.List(ctx, f.tenant, Filter{ActionPrefix: "role_"}); len(p.Items) != 0 {
		t.Errorf("'_' harus literal, bukan wildcard LIKE: %d item", len(p.Items))
	}
	if p, _ := f.svc.List(ctx, f.tenant, Filter{ActorID: bob}); len(p.Items) != 2 {
		t.Errorf("filter pelaku = %d, want 2", len(p.Items))
	}
	if p, _ := f.svc.List(ctx, f.tenant, Filter{EntityID: "3"}); len(p.Items) != 1 {
		t.Errorf("filter entity_id = %d, want 1", len(p.Items))
	}
	if p, _ := f.svc.List(ctx, f.tenant, Filter{From: time.Now().Add(time.Hour)}); len(p.Items) != 0 {
		t.Errorf("filter waktu masa depan = %d, want 0", len(p.Items))
	}

	// Paginasi keyset: 3 halaman (3+3+1) tanpa duplikat atau terlewat.
	seen := map[int64]bool{}
	cursor, pages := "", 0
	for {
		p, err := f.svc.List(ctx, f.tenant, Filter{Limit: 3, Cursor: cursor})
		if err != nil {
			t.Fatal(err)
		}
		pages++
		for _, it := range p.Items {
			if seen[it.ID] {
				t.Fatalf("item %d muncul dua kali", it.ID)
			}
			seen[it.ID] = true
		}
		if p.NextCursor == "" {
			break
		}
		cursor = p.NextCursor
	}
	if len(seen) != 7 || pages != 3 {
		t.Errorf("paginasi: %d item dalam %d halaman, want 7 dalam 3", len(seen), pages)
	}
	if _, err := f.svc.List(ctx, f.tenant, Filter{Cursor: "rusak!"}); err != ErrBadCursor {
		t.Errorf("kursor rusak: err = %v", err)
	}
}

func TestAuditIsAppendOnlyAndTenantIsolated(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	other := uuid.New()
	if _, err := f.admin.Exec(ctx, `INSERT INTO tenants (id, code, name) VALUES ($1, $2, 'UJI-AUDIT-2')`, other, "au-"+other.String()[:8]); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = f.admin.Exec(ctx, `DELETE FROM audit_log WHERE tenant_id = $1`, other)
		_, _ = f.admin.Exec(ctx, `DELETE FROM tenants WHERE id = $1`, other)
	})
	f.record(t, f.tenant, uuid.New(), Entry{Action: ActionLogin, Entity: EntityUser})
	f.record(t, other, uuid.New(), Entry{Action: ActionLogin, Entity: EntityUser})

	// Tenant lain tidak melihat catatan ini.
	if p, _ := f.svc.List(ctx, other, Filter{}); len(p.Items) != 1 {
		t.Errorf("tenant lain melihat %d catatan, want 1 (miliknya sendiri)", len(p.Items))
	}

	// Aplikasi tidak boleh mengubah atau menghapus catatan, bahkan di dalam tenant sendiri.
	for name, sql := range map[string]string{
		"UPDATE":   `UPDATE audit_log SET action = 'auth.hack'`,
		"DELETE":   `DELETE FROM audit_log`,
		"TRUNCATE": `TRUNCATE audit_log`,
	} {
		err := db.WithTenant(ctx, f.app, f.tenant, func(tx pgx.Tx) error {
			_, err := tx.Exec(ctx, sql)
			return err
		})
		if err == nil {
			t.Errorf("%s oleh role aplikasi seharusnya ditolak (append-only)", name)
		}
	}
	// Menyisipkan atas nama tenant lain ditolak oleh WITH CHECK.
	err := db.WithTenant(ctx, f.app, f.tenant, func(tx pgx.Tx) error {
		return Record(ctx, tx, Actor{TenantID: other}, Entry{Action: ActionLogin, Entity: EntityUser})
	})
	if err == nil {
		t.Error("catatan untuk tenant lain seharusnya ditolak")
	}
	if err := Record(ctx, nil, Actor{}, Entry{Action: ActionLogin, Entity: EntityUser}); err == nil {
		t.Error("tenant kosong harus ditolak")
	}
}
