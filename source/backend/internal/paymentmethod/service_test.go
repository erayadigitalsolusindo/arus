package paymentmethod

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/shopspring/decimal"

	"aciraba/internal/authz"
	"aciraba/internal/platform/db"
)

func fieldCode(err error, field string) string {
	var f FieldErrors
	if errors.As(err, &f) {
		return f[field]
	}
	return ""
}

func TestIsKind(t *testing.T) {
	for _, k := range Kinds {
		if !IsKind(k) {
			t.Errorf("%s harus sah", k)
		}
	}
	for _, k := range []string{"", "receivable", "CASH", "qris"} {
		if IsKind(k) {
			t.Errorf("%q tidak boleh sah", k)
		}
	}
}

func TestLifecycle(t *testing.T) {
	appURL, adminURL := os.Getenv("TEST_DATABASE_URL"), os.Getenv("TEST_ADMIN_DATABASE_URL")
	if appURL == "" || adminURL == "" {
		t.Skip("TEST_DATABASE_URL/TEST_ADMIN_DATABASE_URL tidak di-set")
	}
	ctx := context.Background()
	app, err := pgxpool.New(ctx, appURL)
	if err != nil {
		t.Fatal(err)
	}
	defer app.Close()
	admin, err := pgxpool.New(ctx, adminURL)
	if err != nil {
		t.Fatal(err)
	}
	defer admin.Close()
	tenant, other := uuid.New(), uuid.New()
	for _, id := range []uuid.UUID{tenant, other} {
		if _, err := admin.Exec(ctx, `INSERT INTO tenants (id, code, name) VALUES ($1, $2, 'UJI-METODE')`, id, "pm-"+id.String()[:8]); err != nil {
			t.Fatal(err)
		}
		if err := db.WithTenant(ctx, app, id, func(tx pgx.Tx) error { return SeedDefaults(ctx, tx, id) }); err != nil {
			t.Fatal(err)
		}
	}
	t.Cleanup(func() {
		for _, id := range []uuid.UUID{tenant, other} {
			_, _ = admin.Exec(ctx, `DELETE FROM audit_log WHERE tenant_id = $1`, id)
			_, _ = admin.Exec(ctx, `DELETE FROM payment_methods WHERE tenant_id = $1`, id)
			_, _ = admin.Exec(ctx, `DELETE FROM tenants WHERE id = $1`, id)
		}
	})
	svc := NewService(app)
	a, b := authz.Actor{TenantID: tenant}, authz.Actor{TenantID: other}

	// Bawaan: lima metode, Tunai di urutan pertama dan bersifat sistem.
	list, total, err := svc.List(ctx, a, ListParams{})
	if err != nil || total != 5 || list[0].Name != "Tunai" || !list[0].System || list[0].Kind != KindCash {
		t.Fatalf("bawaan: %+v %d %v", list, total, err)
	}
	cash := list[0]

	// Tambah metode bebas; nama unik tanpa membedakan huruf; Tunai kedua ditolak; jenis wajib & sah.
	qris, err := svc.Create(ctx, a, Input{Name: "QRIS BCA", Kind: KindEWallet})
	if err != nil || qris.Kind != KindEWallet || qris.System || !qris.Active {
		t.Fatalf("create: %+v %v", qris, err)
	}
	if _, err = svc.Create(ctx, a, Input{Name: "qris bca", Kind: KindDebit}); !errors.Is(err, ErrNameTaken) {
		t.Fatalf("nama ganda: %v", err)
	}
	if _, err = svc.Create(ctx, a, Input{Name: "Kas Kecil", Kind: KindCash}); fieldCode(err, "kind") != "INVALID" {
		t.Fatalf("tunai kedua: %v", err)
	}
	if _, err = svc.Create(ctx, a, Input{Name: "X", Kind: "bogus"}); fieldCode(err, "kind") != "INVALID" {
		t.Fatalf("jenis asing: %v", err)
	}
	if _, err = svc.Create(ctx, a, Input{Name: "X"}); fieldCode(err, "kind") != "REQUIRED" {
		t.Fatalf("jenis kosong: %v", err)
	}
	if _, err = svc.Create(ctx, a, Input{Name: "<b>", Kind: KindDebit}); fieldCode(err, "name") == "" {
		t.Fatalf("nama berisi markup harus ditolak: %v", err)
	}
	// Tenant lain bebas memakai nama yang sama dan tidak melihat metode ini.
	if _, err = svc.Create(ctx, b, Input{Name: "QRIS BCA", Kind: KindEWallet}); err != nil {
		t.Fatalf("tenant lain: %v", err)
	}
	if _, err = svc.Update(ctx, b, qris.ID, Input{Name: "Curi"}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("ubah milik tenant lain: %v", err)
	}
	if _, err = svc.SetActive(ctx, b, qris.ID, false); !errors.Is(err, ErrNotFound) {
		t.Fatalf("arsip milik tenant lain: %v", err)
	}

	// Ganti nama boleh; ganti jenis tidak.
	r, err := svc.Update(ctx, a, qris.ID, Input{Name: "QRIS Mandiri", Kind: KindEWallet})
	if err != nil || r.Name != "QRIS Mandiri" {
		t.Fatalf("rename: %+v %v", r, err)
	}
	// Jenis boleh diganti antar non-tunai (id tetap), tetapi tidak dari/ke Tunai.
	if r, err = svc.Update(ctx, a, qris.ID, Input{Name: "QRIS Mandiri", Kind: KindDebit}); err != nil || r.Kind != KindDebit || r.ID != qris.ID {
		t.Fatalf("ganti jenis non-tunai: %+v %v", r, err)
	}
	if _, err = svc.Update(ctx, a, qris.ID, Input{Name: "QRIS Mandiri", Kind: KindCash}); fieldCode(err, "kind") != "KIND_LOCKED" {
		t.Fatalf("jadi tunai ditolak: %v", err)
	}
	if _, err = svc.Update(ctx, a, cash.ID, Input{Name: "Tunai", Kind: KindDebit}); fieldCode(err, "kind") != "KIND_LOCKED" {
		t.Fatalf("Tunai terkunci: %v", err)
	}
	if _, err = svc.Update(ctx, a, qris.ID, Input{Name: "Transfer"}); !errors.Is(err, ErrNameTaken) {
		t.Fatalf("rename ke nama yang ada: %v", err)
	}
	if r, err = svc.Update(ctx, a, cash.ID, Input{Name: "Kas"}); err != nil || r.Name != "Kas" || !r.System {
		t.Fatalf("Tunai boleh diganti nama: %+v %v", r, err)
	}

	// Arsip: Tunai tak bisa; lainnya bisa dan hilang dari lookup, lalu bisa diaktifkan lagi.
	if _, err = svc.SetActive(ctx, a, cash.ID, false); !errors.Is(err, ErrLocked) {
		t.Fatalf("arsip Tunai: %v", err)
	}
	if r, err = svc.SetActive(ctx, a, qris.ID, false); err != nil || r.Active {
		t.Fatalf("arsip: %+v %v", r, err)
	}
	look, err := svc.Lookup(ctx, a)
	if err != nil || len(look) != 5 || look[0].Kind != KindCash {
		t.Fatalf("lookup tanpa arsip, Tunai pertama: %+v %v", look, err)
	}
	for _, m := range look {
		if m.ID == qris.ID {
			t.Fatal("metode terarsip tidak boleh muncul di lookup")
		}
	}
	if r, err = svc.SetActive(ctx, a, qris.ID, true); err != nil || !r.Active {
		t.Fatalf("aktifkan kembali: %+v %v", r, err)
	}
	inactive := false
	if _, total, _ = svc.List(ctx, a, ListParams{Active: &inactive}); total != 0 {
		t.Fatalf("filter nonaktif: %d", total)
	}
	if list, total, _ = svc.List(ctx, a, ListParams{Q: "qris"}); total != 1 || list[0].ID != qris.ID {
		t.Fatalf("cari: %+v %d", list, total)
	}

	// Pagar DB: dua metode tunai dalam satu tenant ditolak walau lewat SQL langsung.
	if _, err = admin.Exec(ctx, `INSERT INTO payment_methods (tenant_id, name, kind) VALUES ($1, 'Tunai 2', 'cash')`, tenant); err == nil {
		t.Fatal("DB harus menolak tunai kedua")
	}
	// Metode tidak bisa dihapus oleh role aplikasi.
	if _, err = app.Exec(ctx, `DELETE FROM payment_methods WHERE tenant_id = $1`, tenant); err == nil {
		t.Fatal("role aplikasi tidak boleh menghapus metode")
	}

	// Batas jumlah per tenant (30 termasuk arsip).
	for i := 0; ; i++ {
		_, err = svc.Create(ctx, a, Input{Name: fmt.Sprintf("Metode %d", i), Kind: KindTransfer})
		if err != nil {
			break
		}
		if i > 40 {
			t.Fatal("batas jumlah metode tidak berlaku")
		}
	}
	if !errors.Is(err, ErrTooMany) {
		t.Fatalf("batas: %v", err)
	}

	// Audit tercatat untuk buat / ubah / arsip.
	var n int
	if err := admin.QueryRow(ctx, `SELECT count(*) FROM audit_log WHERE tenant_id = $1 AND entity = 'payment_method'`, tenant).Scan(&n); err != nil || n < 5 {
		t.Fatalf("audit = %d (%v)", n, err)
	}
}

func TestFees(t *testing.T) {
	for _, c := range []struct {
		name string
		in   Input
		kind string
		want string // field galat yang diharapkan ("" = sah)
	}{
		{"persen saja", Input{FeePct: "0.7"}, KindDebit, ""},
		{"rupiah saja", Input{FeeFlat: "2500"}, KindEWallet, ""},
		{"keduanya", Input{FeePct: "0.7", FeeFlat: "500"}, KindCreditCard, ""},
		{"kosong", Input{}, KindTransfer, ""},
		{"persen > 100", Input{FeePct: "100.01"}, KindDebit, "fee_pct"},
		{"persen negatif", Input{FeePct: "-1"}, KindDebit, "fee_pct"},
		{"3 desimal", Input{FeePct: "0.123"}, KindDebit, "fee_pct"},
		{"rupiah teks", Input{FeeFlat: "abc"}, KindDebit, "fee_flat"},
		{"rupiah negatif", Input{FeeFlat: "-5"}, KindDebit, "fee_flat"},
		{"rupiah kelewat besar", Input{FeeFlat: "9999999999"}, KindDebit, "fee_flat"},
		{"tunai tak boleh berbiaya", Input{FeePct: "1"}, KindCash, "fee_pct"},
	} {
		f := FieldErrors{}
		fees(c.in, c.kind, f)
		if c.want == "" && len(f) > 0 || c.want != "" && f[c.want] == "" {
			t.Errorf("%s: dapat %v, ingin galat %q", c.name, f, c.want)
		}
	}
}

func TestFeesLifecycle(t *testing.T) {
	appURL, adminURL := os.Getenv("TEST_DATABASE_URL"), os.Getenv("TEST_ADMIN_DATABASE_URL")
	if appURL == "" || adminURL == "" {
		t.Skip("TEST_DATABASE_URL/TEST_ADMIN_DATABASE_URL tidak di-set")
	}
	ctx := context.Background()
	app, err := pgxpool.New(ctx, appURL)
	if err != nil {
		t.Fatal(err)
	}
	defer app.Close()
	admin, err := pgxpool.New(ctx, adminURL)
	if err != nil {
		t.Fatal(err)
	}
	defer admin.Close()
	tenant := uuid.New()
	if _, err := admin.Exec(ctx, `INSERT INTO tenants (id, code, name) VALUES ($1, $2, 'UJI-MDR')`, tenant, "mdr-"+tenant.String()[:8]); err != nil {
		t.Fatal(err)
	}
	if err := db.WithTenant(ctx, app, tenant, func(tx pgx.Tx) error { return SeedDefaults(ctx, tx, tenant) }); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = admin.Exec(ctx, `DELETE FROM audit_log WHERE tenant_id = $1`, tenant)
		_, _ = admin.Exec(ctx, `DELETE FROM payment_methods WHERE tenant_id = $1`, tenant)
		_, _ = admin.Exec(ctx, `DELETE FROM tenants WHERE id = $1`, tenant)
	})
	svc := NewService(app)
	a := authz.Actor{TenantID: tenant}

	m, err := svc.Create(ctx, a, Input{Name: "QRIS BCA", Kind: KindEWallet, FeePct: "0.7", FeeFlat: "100"})
	if err != nil || m.FeePct != "0.70" || m.FeeFlat != "100.00" {
		t.Fatalf("create dengan biaya: %+v %v", m, err)
	}
	if _, err = svc.Create(ctx, a, Input{Name: "Debit X", Kind: KindDebit, FeePct: "101"}); fieldCode(err, "fee_pct") != "INVALID" {
		t.Fatalf("persen > 100: %v", err)
	}
	// Ubah biaya saja (nama sama) tersimpan dan tercatat di audit; kirim sama persis = tanpa perubahan.
	r, err := svc.Update(ctx, a, m.ID, Input{Name: "QRIS BCA", FeePct: "0.5", FeeFlat: ""})
	if err != nil || r.FeePct != "0.50" || r.FeeFlat != "0.00" {
		t.Fatalf("ubah biaya: %+v %v", r, err)
	}
	var before int
	_ = admin.QueryRow(ctx, `SELECT count(*) FROM audit_log WHERE tenant_id = $1 AND action = 'payment_method.update'`, tenant).Scan(&before)
	if _, err = svc.Update(ctx, a, m.ID, Input{Name: "QRIS BCA", FeePct: "0.5"}); err != nil {
		t.Fatal(err)
	}
	var after int
	_ = admin.QueryRow(ctx, `SELECT count(*) FROM audit_log WHERE tenant_id = $1 AND action = 'payment_method.update'`, tenant).Scan(&after)
	if before != 1 || after != 1 {
		t.Fatalf("audit ubah: %d → %d, ingin 1 → 1", before, after)
	}
	// Tunai tidak boleh diberi biaya (aplikasi maupun DB).
	list, _, _ := svc.List(ctx, a, ListParams{})
	if _, err = svc.Update(ctx, a, list[0].ID, Input{Name: "Tunai", FeePct: "1"}); fieldCode(err, "fee_pct") != "CASH_NO_FEE" {
		t.Fatalf("tunai berbiaya: %v", err)
	}
	if _, err = admin.Exec(ctx, `UPDATE payment_methods SET fee_flat = 5 WHERE id = $1`, list[0].ID); err == nil {
		t.Fatal("DB harus menolak biaya pada tunai")
	}
}

func TestBearer(t *testing.T) {
	for _, c := range []struct {
		name, in, kind, def string
		wantField           string
		want                string
	}{
		{"kosong -> bawaan", "", KindDebit, BearerStore, "", BearerStore},
		{"kosong -> tetap yang sekarang", "", KindDebit, BearerCustomer, "", BearerCustomer},
		{"pelanggan", "customer", KindEWallet, BearerStore, "", BearerCustomer},
		{"toko", "store", KindEWallet, BearerCustomer, "", BearerStore},
		{"asing", "bank", KindDebit, BearerStore, "fee_bearer", ""},
		{"tunai tak boleh pelanggan", "customer", KindCash, BearerStore, "fee_bearer", ""},
	} {
		f := FieldErrors{}
		got := bearer(Input{FeeBearer: c.in}, c.kind, c.def, f)
		if c.wantField != "" && f[c.wantField] == "" || c.wantField == "" && (len(f) > 0 || got != c.want) {
			t.Errorf("%s: dapat %q %v", c.name, got, f)
		}
	}
}

func TestBearerLifecycle(t *testing.T) {
	appURL, adminURL := os.Getenv("TEST_DATABASE_URL"), os.Getenv("TEST_ADMIN_DATABASE_URL")
	if appURL == "" || adminURL == "" {
		t.Skip("TEST_DATABASE_URL/TEST_ADMIN_DATABASE_URL tidak di-set")
	}
	ctx := context.Background()
	app, err := pgxpool.New(ctx, appURL)
	if err != nil {
		t.Fatal(err)
	}
	defer app.Close()
	admin, err := pgxpool.New(ctx, adminURL)
	if err != nil {
		t.Fatal(err)
	}
	defer admin.Close()
	tenant := uuid.New()
	if _, err := admin.Exec(ctx, `INSERT INTO tenants (id, code, name) VALUES ($1, $2, 'UJI-BEARER')`, tenant, "br-"+tenant.String()[:8]); err != nil {
		t.Fatal(err)
	}
	if err := db.WithTenant(ctx, app, tenant, func(tx pgx.Tx) error { return SeedDefaults(ctx, tx, tenant) }); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = admin.Exec(ctx, `DELETE FROM audit_log WHERE tenant_id = $1`, tenant)
		_, _ = admin.Exec(ctx, `DELETE FROM payment_methods WHERE tenant_id = $1`, tenant)
		_, _ = admin.Exec(ctx, `DELETE FROM tenants WHERE id = $1`, tenant)
	})
	svc := NewService(app)
	a := authz.Actor{TenantID: tenant}

	m, err := svc.Create(ctx, a, Input{Name: "QRIS BCA", Kind: KindEWallet, FeePct: "0.7", FeeBearer: "customer"})
	if err != nil || m.FeeBearer != "customer" {
		t.Fatalf("create pelanggan: %+v %v", m, err)
	}
	d, err := svc.Create(ctx, a, Input{Name: "Debit BCA", Kind: KindDebit, FeePct: "1"})
	if err != nil || d.FeeBearer != "store" {
		t.Fatalf("bawaan toko: %+v %v", d, err)
	}
	// Ubah penanggung biaya saja; kosongkan = tetap.
	r, err := svc.Update(ctx, a, m.ID, Input{Name: "QRIS BCA", FeePct: "0.7", FeeBearer: "store"})
	if err != nil || r.FeeBearer != "store" {
		t.Fatalf("ubah ke toko: %+v %v", r, err)
	}
	if r, err = svc.Update(ctx, a, m.ID, Input{Name: "QRIS BCA", FeePct: "0.7"}); err != nil || r.FeeBearer != "store" {
		t.Fatalf("kosong = tetap: %+v %v", r, err)
	}
	if _, err = svc.Update(ctx, a, m.ID, Input{Name: "QRIS BCA", FeePct: "0.7", FeeBearer: "bank"}); fieldCode(err, "fee_bearer") != "INVALID" {
		t.Fatalf("asing: %v", err)
	}
	look, err := svc.Lookup(ctx, a)
	if err != nil {
		t.Fatal(err)
	}
	for _, x := range look {
		if x.ID == d.ID && (x.FeePct != "1.00" || x.FeeBearer != "store") {
			t.Fatalf("lookup membawa tarif + penanggung: %+v", x)
		}
	}
	// Pagar DB: tunai tak boleh dibebankan ke pelanggan.
	list, _, _ := svc.List(ctx, a, ListParams{})
	if _, err = admin.Exec(ctx, `UPDATE payment_methods SET fee_bearer = 'customer' WHERE id = $1`, list[0].ID); err == nil {
		t.Fatal("DB harus menolak tunai dibebankan ke pelanggan")
	}
}

// Kolom biaya yang kosong / berupa angka / teks / null semuanya terbaca (bug: "" ditolak sebagai json.Number).
func TestNumDecoding(t *testing.T) {
	for body, want := range map[string]string{
		`{"fee_pct":"","fee_flat":"100"}`:    "|100",
		`{"fee_pct":0.7,"fee_flat":null}`:    "0.7|",
		`{"fee_pct":" 1.5 ","fee_flat":250}`: "1.5|250",
		`{}`:                                 "|",
	} {
		var in Input
		if err := json.Unmarshal([]byte(body), &in); err != nil {
			t.Fatalf("%s: %v", body, err)
		}
		if got := in.FeePct.String() + "|" + in.FeeFlat.String(); got != want {
			t.Errorf("%s: dapat %q, ingin %q", body, got, want)
		}
	}
	f := FieldErrors{}
	if pct, flat := fees(Input{FeePct: "", FeeFlat: "100"}, KindEWallet, f); len(f) != 0 || !pct.IsZero() || !flat.Equal(decimal.NewFromInt(100)) {
		t.Fatalf("persen kosong + tetap 100: %v %s %s", f, pct, flat)
	}
}
