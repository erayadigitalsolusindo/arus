package catalog

import (
	"context"
	"errors"
	"os"
	"strings"
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"aciraba/internal/authz"
)

func TestValidateSupplier(t *testing.T) {
	c, f := validateSupplier(SupplierInput{Code: "SUP-01", Name: "  PT   Maju  ", Phone: "0812 3456 7890", Email: "A@B.co", Address: "Jl. Mawar <1>"})
	if f != nil || c.name != "PT Maju" || c.phone != "+6281234567890" || c.email != "a@b.co" || !c.code.Valid || c.code.String != "SUP-01" {
		t.Fatalf("input valid: %+v %v", c, f)
	}
	if c, f := validateSupplier(SupplierInput{Name: "X"}); f != nil || c.code.Valid || c.phone != "" || c.email != "" {
		t.Errorf("field opsional kosong: %+v %v", c, f)
	}
	for name, in := range map[string]SupplierInput{
		"nama kosong":       {Name: " "},
		"nama markup":       {Name: "<b>x</b>"},
		"kode berspasi":     {Name: "X", Code: "A B"},
		"kode terlalu 31":   {Name: "X", Code: strings.Repeat("a", 31)},
		"hp salah":          {Name: "X", Phone: "abc"},
		"email salah":       {Name: "X", Email: "bukan-email"},
		"kontak markup":     {Name: "X", ContactName: "<script>"},
		"catatan panjang":   {Name: "X", Note: strings.Repeat("a", maxNote+1)},
		"alamat kontrol":    {Name: "X", Address: "a\x00b"},
		"nama 101 karakter": {Name: strings.Repeat("a", 101)},
	} {
		if _, f := validateSupplier(in); f == nil {
			t.Errorf("%s: seharusnya ditolak", name)
		}
	}
}

func TestLikeEscape(t *testing.T) {
	if got := likeEscape(`50%_a\b`); got != `50\%\_a\\b` {
		t.Errorf("likeEscape = %q", got)
	}
}

type env struct {
	svc    *Service
	admin  *pgxpool.Pool
	a, b   authz.Actor
	tenant [2]uuid.UUID
}

func newEnv(t *testing.T) *env {
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

	e := &env{admin: admin, svc: NewService(app)}
	actors := make([]authz.Actor, 2)
	for i := range actors {
		tid, uid, oid := uuid.New(), uuid.New(), uuid.New()
		e.tenant[i] = tid
		sfx := tid.String()[:8]
		roleID := uuid.New()
		for _, q := range []struct {
			sql  string
			args []any
		}{
			{`INSERT INTO tenants (id, code, name) VALUES ($1, $2, $3)`, []any{tid, "ct-" + sfx, "UJI-KATALOG-" + sfx}},
			{`INSERT INTO outlets (id, tenant_id, code, name) VALUES ($1, $2, 'main', 'Pusat')`, []any{oid, tid}},
			{`INSERT INTO roles (id, tenant_id, name, permissions, is_system) VALUES ($1, $2, 'Owner', '{"*":true}', true)`, []any{roleID, tid}},
			{`INSERT INTO users (id, tenant_id, role_id, email, name, password_hash) VALUES ($1, $2, $3, $4, 'Pemilik', 'x')`, []any{uid, tid, roleID, "ct-" + sfx + "@ct.test"}},
		} {
			if _, err := admin.Exec(ctx, q.sql, q.args...); err != nil {
				t.Fatal(err)
			}
		}
		actors[i] = authz.Actor{TenantID: tid, UserID: uid, OutletID: oid, Name: "Pemilik", Perms: authz.Permissions{All: true}, Outlets: map[uuid.UUID]bool{oid: true}}
	}
	e.a, e.b = actors[0], actors[1]
	t.Cleanup(func() {
		for _, tid := range e.tenant {
			for _, tbl := range []string{"audit_log", "units", "categories", "brands", "principals", "suppliers", "user_outlets", "users", "roles", "outlets"} {
				_, _ = admin.Exec(ctx, `DELETE FROM `+tbl+` WHERE tenant_id = $1`, tid)
			}
			_, _ = admin.Exec(ctx, `DELETE FROM tenants WHERE id = $1`, tid)
		}
	})
	return e
}

func TestSimpleMasters(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	for _, k := range Kinds {
		t.Run(k.Path, func(t *testing.T) {
			u, err := e.svc.Create(ctx, e.a, k, "  Dus   Besar ")
			if err != nil || u.Name != "Dus Besar" || !u.Active {
				t.Fatalf("buat: %+v %v", u, err)
			}
			// Nama unik per tenant tanpa membedakan huruf besar/kecil; tenant lain boleh memakai nama sama.
			if _, err := e.svc.Create(ctx, e.a, k, "dus besar"); !errors.Is(err, ErrNameTaken) {
				t.Errorf("nama ganda: err = %v, want ErrNameTaken", err)
			}
			if _, err := e.svc.Create(ctx, e.b, k, "Dus Besar"); err != nil {
				t.Errorf("tenant lain memakai nama sama: %v", err)
			}
			var fe FieldErrors
			for _, bad := range []string{"", "  ", "<b>", strings.Repeat("x", 101)} {
				if _, err := e.svc.Create(ctx, e.a, k, bad); !errors.As(err, &fe) || fe["name"] == "" {
					t.Errorf("nama %q: err = %v, want validasi", bad, err)
				}
			}

			// Ubah nama: bentrok dengan baris lain ditolak.
			other, _ := e.svc.Create(ctx, e.a, k, "Pak")
			if _, err := e.svc.Rename(ctx, e.a, k, other.ID, "DUS BESAR"); !errors.Is(err, ErrNameTaken) {
				t.Errorf("ubah ke nama ganda: err = %v", err)
			}
			if r, err := e.svc.Rename(ctx, e.a, k, other.ID, "Pak Kecil"); err != nil || r.Name != "Pak Kecil" {
				t.Errorf("ubah: %+v %v", r, err)
			}
			if _, err := e.svc.Rename(ctx, e.a, k, uuid.New(), "X"); !errors.Is(err, ErrNotFound) {
				t.Errorf("ubah id acak: err = %v", err)
			}

			// Arsip: hilang dari filter aktif, tetap ada di filter nonaktif/semua; bisa diaktifkan lagi.
			if r, err := e.svc.SetActive(ctx, e.a, k, other.ID, false); err != nil || r.Active {
				t.Fatalf("arsip: %+v %v", r, err)
			}
			yes, no := true, false
			if list, total, _ := e.svc.List(ctx, e.a, k, ListParams{Active: &yes}); total != 1 || len(list) != 1 || list[0].Name != "Dus Besar" {
				t.Errorf("filter aktif: %+v total=%d", list, total)
			}
			if _, total, _ := e.svc.List(ctx, e.a, k, ListParams{Active: &no}); total != 1 {
				t.Errorf("filter nonaktif total = %d, want 1", total)
			}
			if _, total, _ := e.svc.List(ctx, e.a, k, ListParams{}); total != 2 {
				t.Errorf("semua total = %d, want 2", total)
			}
			if _, err := e.svc.SetActive(ctx, e.a, k, other.ID, true); err != nil {
				t.Errorf("aktifkan kembali: %v", err)
			}

			// Pencarian: bagian kata tanpa membedakan huruf besar/kecil; persen dan garis bawah dicocokkan apa adanya.
			if list, _, _ := e.svc.List(ctx, e.a, k, ListParams{Q: "kEcIl"}); len(list) != 1 || list[0].Name != "Pak Kecil" {
				t.Errorf("cari kEcIl: %+v", list)
			}
			if list, _, _ := e.svc.List(ctx, e.a, k, ListParams{Q: "%"}); len(list) != 0 {
				t.Errorf("persen tidak boleh jadi wildcard: %+v", list)
			}
			// Halaman: limit/offset.
			if list, total, _ := e.svc.List(ctx, e.a, k, ListParams{Limit: 1, Offset: 1}); len(list) != 1 || total != 2 || list[0].Name != "Pak Kecil" {
				t.Errorf("halaman 2: %+v total=%d", list, total)
			}

			// Isolasi tenant: tenant B tidak melihat/mengubah milik A, walau menebak id.
			if list, total, _ := e.svc.List(ctx, e.b, k, ListParams{}); total != 1 || list[0].Name != "Dus Besar" || list[0].ID == u.ID {
				t.Errorf("tenant B melihat data tenant A: %+v", list)
			}
			if _, err := e.svc.Rename(ctx, e.b, k, u.ID, "Curang"); !errors.Is(err, ErrNotFound) {
				t.Errorf("ubah lintas tenant: err = %v, want ErrNotFound", err)
			}
			if _, err := e.svc.SetActive(ctx, e.b, k, u.ID, false); !errors.Is(err, ErrNotFound) {
				t.Errorf("arsip lintas tenant: err = %v, want ErrNotFound", err)
			}
		})
	}
	var n int
	_ = e.admin.QueryRow(ctx, `SELECT count(*) FROM audit_log WHERE tenant_id = $1 AND entity IN ('unit','category','brand','principal')`, e.tenant[0]).Scan(&n)
	if n == 0 {
		t.Error("perubahan master harus tercatat di audit log")
	}
}

func TestConcurrentCreateOnlyOneWins(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	var wg sync.WaitGroup
	res := make(chan error, 8)
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := e.svc.Create(ctx, e.a, Kinds[2], "Samsung")
			res <- err
		}()
	}
	wg.Wait()
	close(res)
	ok, taken := 0, 0
	for err := range res {
		switch {
		case err == nil:
			ok++
		case errors.Is(err, ErrNameTaken):
			taken++
		default:
			t.Errorf("error tak terduga: %v", err)
		}
	}
	if ok != 1 || taken != 7 {
		t.Errorf("menang=%d bentrok=%d, want 1 dan 7", ok, taken)
	}
}

func TestSuppliers(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	s, err := e.svc.CreateSupplier(ctx, e.a, SupplierInput{Code: "sup-1", Name: "PT Maju", Phone: "08123456789", Email: "x@y.id"})
	if err != nil || s.Code != "sup-1" || s.Phone != "+628123456789" {
		t.Fatalf("buat: %+v %v", s, err)
	}
	if _, err := e.svc.CreateSupplier(ctx, e.a, SupplierInput{Name: "pt maju"}); !errors.Is(err, ErrNameTaken) {
		t.Errorf("nama ganda: %v", err)
	}
	if _, err := e.svc.CreateSupplier(ctx, e.a, SupplierInput{Code: "SUP-1", Name: "Lain"}); !errors.Is(err, ErrCodeTaken) {
		t.Errorf("kode ganda (beda huruf): %v", err)
	}
	// Kode kosong boleh berulang (hanya yang terisi yang unik).
	if _, err := e.svc.CreateSupplier(ctx, e.a, SupplierInput{Name: "Tanpa Kode 1"}); err != nil {
		t.Errorf("tanpa kode #1: %v", err)
	}
	if _, err := e.svc.CreateSupplier(ctx, e.a, SupplierInput{Name: "Tanpa Kode 2"}); err != nil {
		t.Errorf("tanpa kode #2: %v", err)
	}
	if _, err := e.svc.CreateSupplier(ctx, e.b, SupplierInput{Code: "sup-1", Name: "PT Maju"}); err != nil {
		t.Errorf("tenant lain memakai kode/nama sama: %v", err)
	}

	u, err := e.svc.UpdateSupplier(ctx, e.a, s.ID, SupplierInput{Code: "sup-1", Name: "PT Maju Jaya", Address: "Jl. Mawar 1"})
	if err != nil || u.Name != "PT Maju Jaya" || u.Phone != "" || u.Address != "Jl. Mawar 1" {
		t.Fatalf("ubah: %+v %v", u, err)
	}
	if _, err := e.svc.UpdateSupplier(ctx, e.b, s.ID, SupplierInput{Name: "Curang"}); !errors.Is(err, ErrNotFound) {
		t.Errorf("ubah lintas tenant: %v", err)
	}
	if _, err := e.svc.SetSupplierActive(ctx, e.a, s.ID, false); err != nil {
		t.Fatal(err)
	}
	yes := true
	if _, total, _ := e.svc.ListSuppliers(ctx, e.a, ListParams{Active: &yes}); total != 2 {
		t.Errorf("supplier aktif = %d, want 2", total)
	}
	if list, _, _ := e.svc.ListSuppliers(ctx, e.a, ListParams{Q: "sup-1"}); len(list) != 1 {
		t.Errorf("cari berdasarkan kode: %+v", list)
	}
	if list, _, _ := e.svc.ListSuppliers(ctx, e.b, ListParams{}); len(list) != 1 {
		t.Errorf("tenant B melihat %d supplier, want 1", len(list))
	}
}
