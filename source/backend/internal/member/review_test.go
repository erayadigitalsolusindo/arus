package member

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"aciraba/internal/authz"
	pauth "aciraba/internal/platform/auth"
	"aciraba/internal/platform/db"
)

func TestPhoneQuery(t *testing.T) {
	for in, want := range map[string]string{
		"081234567890":   "+6281234567890",
		"0812-3456":      "+62812" + "3456",
		"+62 812 345":    "+62812345",
		"62812345":       "+62812345",
		"812345":         "812345",
		"08":             "",
		"budi":           "",
		"MBR-000001":     "",
		"(0812) 3456789": "+628123456789",
	} {
		if got := phoneQuery(in); got != want {
			t.Errorf("phoneQuery(%q) = %q, want %q", in, got, want)
		}
	}
}

// Nomor disimpan ternormalisasi (+62…); kasir mengetik 08… dan harus tetap ketemu (daftar + pencarian kasir).
func TestSearchByPhoneInLocalFormat(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	m, err := f.svc.Create(ctx, f.a1, Input{Name: "Budi", Phone: "081234567890"})
	if err != nil || m.Phone != "+6281234567890" {
		t.Fatalf("buat: %+v err=%v", m, err)
	}
	if _, err := f.svc.Create(ctx, f.a1, Input{Name: "Siti", Phone: "085500001111"}); err != nil {
		t.Fatal(err)
	}
	for _, q := range []string{"081234567890", "0812345", "+62 812 345", "62812345", "6281234567890"} {
		rows, _, err := f.svc.List(ctx, f.a1, ListParams{Q: q})
		if err != nil || len(rows) != 1 || rows[0].Name != "Budi" {
			t.Errorf("List(%q) = %v err=%v", q, rows, err)
		}
		look, err := f.svc.Lookup(ctx, f.a1, q)
		if err != nil || len(look) != 1 || look[0].Name != "Budi" {
			t.Errorf("Lookup(%q) = %v err=%v", q, look, err)
		}
	}
	if rows, _, _ := f.svc.List(ctx, f.a1, ListParams{Q: "089999"}); len(rows) != 0 {
		t.Errorf("nomor yang tidak ada ikut cocok: %v", rows)
	}
}

// PUT tanpa `active` tidak boleh mengaktifkan kembali member yang diarsipkan.
func TestUpdateWithoutActiveKeepsStatus(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	m, _ := f.svc.Create(ctx, f.a1, in("Dewi"))
	if _, err := f.svc.SetActive(ctx, f.a1, m.ID, false); err != nil {
		t.Fatal(err)
	}
	got, err := f.svc.Update(ctx, f.a1, m.ID, Input{Code: m.Code, Name: "Dewi Baru"})
	if err != nil || got.Active || got.Name != "Dewi Baru" {
		t.Fatalf("update tanpa active: %+v err=%v", got, err)
	}
	if got, err = f.svc.Update(ctx, f.a1, m.ID, Input{Code: m.Code, Name: "Dewi Baru", Active: ptr(true)}); err != nil || !got.Active {
		t.Fatalf("update dengan active=true: %+v err=%v", got, err)
	}
}

// Pembalikan yang hanya menyentuh lifetime (saldo sudah habis) tetap tercatat di ledger: SUM(lifetime_delta) = lifetime_points.
func TestReverseLifetimeOnlyIsLedgered(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	m, _ := f.svc.Create(ctx, f.a1, in("Rani"))
	s1 := uuid.New()
	if err := f.sale(ctx, m.ID, s1, 0, 10); err != nil { // +10 poin, lifetime 10
		t.Fatal(err)
	}
	if err := f.sale(ctx, m.ID, uuid.New(), 10, 0); err != nil { // habiskan saldo
		t.Fatal(err)
	}
	if err := db.WithTenant(ctx, f.app, f.t1, func(tx pgx.Tx) error { return ReverseSale(ctx, tx, f.a1, s1, "void") }); err != nil {
		t.Fatal(err)
	}
	got, _ := f.svc.Get(ctx, f.a1, m.ID)
	if got.Points != 0 || got.LifetimePoints != 0 {
		t.Fatalf("setelah pembalikan: points=%d lifetime=%d, want 0/0", got.Points, got.LifetimePoints)
	}
	var sum int
	if err := f.admin.QueryRow(ctx, `SELECT coalesce(sum(lifetime_delta),0) FROM member_point_movements WHERE member_id = $1`, m.ID).Scan(&sum); err != nil || sum != got.LifetimePoints {
		t.Errorf("SUM(lifetime_delta) = %d (err %v), want %d", sum, err, got.LifetimePoints)
	}
	moves, _, _ := f.svc.Points(ctx, f.a1, m.ID, 20, 0)
	if len(moves) == 0 || moves[0].Kind != KindReversal || moves[0].Points != 0 {
		t.Errorf("baris pembalikan lifetime-saja tidak tercatat: %+v", moves)
	}
}

// Ambang level hanya unik di antara level AKTIF.
func TestLevelThresholdUniqueAmongActiveOnly(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	levels, _ := f.svc.Levels(ctx, f.a1, nil)
	silver := levels[1]
	if _, err := f.svc.SetLevelActive(ctx, f.a1, silver.ID, false); err != nil {
		t.Fatal(err)
	}
	nl, err := f.svc.CreateLevel(ctx, f.a1, LevelInput{Name: "Silver Baru", MinPoints: "100", SpendPerPoint: "7000", PointValue: "100"})
	if err != nil || nl.MinPoints != 100 {
		t.Fatalf("ambang level terarsip masih menahan: %v", err)
	}
	if _, err := f.svc.SetLevelActive(ctx, f.a1, silver.ID, true); !isField(err, "min_points", "DUPLICATE") {
		t.Errorf("mengaktifkan kembali level yang bentrok: err = %v", err)
	}
	// member_count: satu member di level Reguler.
	if _, err := f.svc.Create(ctx, f.a1, in("Hitung")); err != nil {
		t.Fatal(err)
	}
	levels, _ = f.svc.Levels(ctx, f.a1, nil)
	var reguler int64
	for _, l := range levels {
		if l.Name == "Reguler" {
			reguler = l.MemberCount
		}
	}
	if reguler != 1 {
		t.Errorf("member_count Reguler = %d, want 1", reguler)
	}
}

// RLS tabel member: tenant lain tidak terlihat dan tidak bisa ditulis.
func TestRLSIsolationMemberTables(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	m2, err := f.svc.Create(ctx, f.a2, in("Milik Tenant 2"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.svc.Adjust(ctx, f.a2, m2.ID, 5, "awal"); err != nil {
		t.Fatal(err)
	}
	for _, tbl := range []string{"members", "member_levels", "member_counters", "member_point_movements"} {
		var n int
		err := db.WithTenant(ctx, f.app, f.t1, func(tx pgx.Tx) error {
			return tx.QueryRow(ctx, `SELECT count(*) FROM `+tbl+` WHERE tenant_id = $1`, f.t2).Scan(&n)
		})
		if err != nil || n != 0 {
			t.Errorf("%s: tenant 1 melihat %d baris milik tenant 2 (err %v)", tbl, n, err)
		}
		var own int
		_ = db.WithTenant(ctx, f.app, f.t2, func(tx pgx.Tx) error {
			return tx.QueryRow(ctx, `SELECT count(*) FROM `+tbl+` WHERE tenant_id = $1`, f.t2).Scan(&own)
		})
		if own == 0 {
			t.Errorf("%s: tenant 2 tidak melihat datanya sendiri", tbl)
		}
	}
	writes := map[string]string{
		"member_levels":          `INSERT INTO member_levels (tenant_id, name, min_points, spend_per_point, point_value) VALUES ($1, 'X', 7, 1, 1)`,
		"member_counters":        `UPDATE member_counters SET last_no = 99 WHERE tenant_id = $1`,
		"member_point_movements": `INSERT INTO member_point_movements (tenant_id, member_id, kind, points, balance_after, ref_type) VALUES ($1, $2, 'ADJUST', 1, 1, 'MANUAL')`,
	}
	for tbl, q := range writes {
		err := db.WithTenant(ctx, f.app, f.t1, func(tx pgx.Tx) error {
			if strings.Contains(q, "$2") {
				_, err := tx.Exec(ctx, q, f.t2, m2.ID)
				return err
			}
			tag, err := tx.Exec(ctx, q, f.t2)
			if err == nil && tag.RowsAffected() > 0 {
				return errors.New("baris tenant lain berubah")
			}
			return err
		})
		if tbl != "member_counters" && err == nil {
			t.Errorf("%s: tenant 1 berhasil menulis ke tenant 2", tbl)
		}
		if tbl == "member_counters" && err != nil {
			t.Errorf("%s: %v", tbl, err)
		}
	}
	var life int
	_ = f.admin.QueryRow(ctx, `SELECT lifetime_points FROM members WHERE id = $1`, m2.ID).Scan(&life)
	if life != 5 {
		t.Errorf("data tenant 2 berubah: lifetime = %d", life)
	}
}

// Otorisasi HTTP: lookup kasir, foto, penyesuaian poin (approve), dan status tak berubah oleh PUT.
func TestHTTPAuthorization(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	exec := func(sql string, args ...any) {
		t.Helper()
		if _, err := f.admin.Exec(ctx, sql, args...); err != nil {
			t.Fatal(err)
		}
	}
	mkUser := func(perms string) string {
		role, user := uuid.New(), uuid.New()
		exec(`INSERT INTO roles (id, tenant_id, name, permissions) VALUES ($1, $2, $3, $4::jsonb)`, role, f.t1, "r-"+role.String()[:6], perms)
		exec(`INSERT INTO users (id, tenant_id, role_id, email, name, password_hash) VALUES ($1, $2, $3, $4, 'U', 'x')`, user, f.t1, role, user.String()+"@http.test")
		exec(`INSERT INTO user_outlets (tenant_id, user_id, outlet_id) VALUES ($1, $2, $3)`, f.t1, user, f.a1.OutletID)
		return user.String()
	}
	t.Cleanup(func() { _, _ = f.admin.Exec(ctx, `DELETE FROM user_outlets WHERE tenant_id = $1`, f.t1) })
	users := map[string]string{
		"kasir":    mkUser(`{"sales_orders":["create"]}`),
		"viewer":   mkUser(`{"members":["view"]}`),
		"approver": mkUser(`{"members":["view","approve"]}`),
		"none":     mkUser(`{}`),
	}
	issuer := pauth.NewTokenIssuer("rahasia-uji-rahasia-uji-rahasia-uji-123")
	r := chi.NewRouter()
	NewHandler(f.svc, authz.NewResolver(f.app), issuer, slog.New(slog.NewTextHandler(io.Discard, nil))).Routes(r)

	call := func(who, method, path, body string) (int, string) {
		t.Helper()
		tok, err := issuer.Issue(users[who], f.t1.String(), f.a1.OutletID.String(), "r", time.Now())
		if err != nil {
			t.Fatal(err)
		}
		req := httptest.NewRequest(method, path, strings.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+tok)
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)
		return rec.Code, rec.Body.String()
	}
	m, err := f.svc.Create(ctx, f.a1, Input{Name: "Budi", Phone: "081234567890"})
	if err != nil {
		t.Fatal(err)
	}
	id := m.ID.String()

	// Pencarian kasir: izin kasir atau members.view; tanpa izin ditolak.
	for who, want := range map[string]int{"kasir": 200, "viewer": 200, "approver": 200, "none": 403} {
		if code, body := call(who, "GET", "/members/lookup?q=0812345", ""); code != want {
			t.Errorf("lookup oleh %s: %d (%s), want %d", who, code, body, want)
		}
	}
	// Daftar member hanya untuk pemegang members.view.
	for who, want := range map[string]int{"kasir": 403, "viewer": 200, "none": 403} {
		if code, _ := call(who, "GET", "/members/", ""); code != want {
			t.Errorf("daftar oleh %s: %d, want %d", who, code, want)
		}
	}
	// Foto cover: kasir tidak boleh 403/401 (tanpa storage di test → 503 atau 404, bukan penolakan izin).
	if code, _ := call("kasir", "GET", "/members/"+id+"/cover?size=thumb", ""); code == 403 || code == 401 {
		t.Errorf("kasir ditolak mengambil foto: %d", code)
	}
	if code, _ := call("none", "GET", "/members/"+id+"/cover", ""); code != 403 {
		t.Errorf("tanpa izin foto: %d, want 403", code)
	}
	// Penyesuaian poin butuh aksi approve; riwayat transaksi butuh view.
	adj := `{"points": 25, "note": "uji"}`
	if code, _ := call("viewer", "POST", "/members/"+id+"/points/adjust", adj); code != 403 {
		t.Errorf("viewer menyesuaikan poin: %d, want 403", code)
	}
	if code, body := call("approver", "POST", "/members/"+id+"/points/adjust", adj); code != 200 {
		t.Errorf("approver menyesuaikan poin: %d (%s)", code, body)
	}
	if code, _ := call("kasir", "GET", "/members/"+id+"/sales", ""); code != 403 {
		t.Errorf("kasir membaca riwayat transaksi: %d, want 403", code)
	}
	if code, _ := call("viewer", "GET", "/members/"+id+"/sales", ""); code != 200 {
		t.Errorf("viewer membaca riwayat transaksi: %d, want 200", code)
	}
	// Isian angka kosong dari form tidak membuat permintaan ditolak, dan PUT tanpa active tidak mengubah status.
	if _, err := f.svc.SetActive(ctx, f.a1, m.ID, false); err != nil {
		t.Fatal(err)
	}
	editor := mkUser(`{"members":["view","update"]}`)
	users["editor"] = editor
	code, body := call("editor", "PUT", "/members/"+id, `{"code":"`+m.Code+`","name":"Budi B","credit_limit":"","due_days":""}`)
	var out Member
	_ = json.Unmarshal([]byte(body), &out)
	if code != 200 || out.Active || out.Name != "Budi B" {
		t.Errorf("PUT form: %d %s", code, body)
	}
	var _ = http.StatusOK
}
