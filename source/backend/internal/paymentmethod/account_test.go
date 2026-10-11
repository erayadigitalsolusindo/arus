package paymentmethod

import (
	"context"
	"os"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"aciraba/internal/authz"
	"aciraba/internal/platform/db"
)

// Pemetaan metode -> akun COA: tanpa bagan akun kosong; setelah bagan ada, metode bawaan terhubung ke akun bawaan, metode
// baru mendapat akun sendiri (1150+), ikut berganti nama / aktif-nonaktif, dan akun bisa dipilih manual (hanya akun buku aset).
func TestAccountMapping(t *testing.T) {
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
	if _, err := admin.Exec(ctx, `INSERT INTO tenants (id, code, name) VALUES ($1, $2, 'UJI-METODE-AKUN')`, tenant, "pa-"+tenant.String()[:8]); err != nil {
		t.Fatal(err)
	}
	if err := db.WithTenant(ctx, app, tenant, func(tx pgx.Tx) error { return SeedDefaults(ctx, tx, tenant) }); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = admin.Exec(ctx, `DELETE FROM audit_log WHERE tenant_id = $1`, tenant)
		_, _ = admin.Exec(ctx, `DELETE FROM payment_methods WHERE tenant_id = $1`, tenant)
		_, _ = admin.Exec(ctx, `DELETE FROM accounts WHERE tenant_id = $1 AND parent_id IS NOT NULL`, tenant)
		_, _ = admin.Exec(ctx, `DELETE FROM accounts WHERE tenant_id = $1`, tenant)
		_, _ = admin.Exec(ctx, `DELETE FROM tenants WHERE id = $1`, tenant)
	})
	svc := NewService(app)
	a := authz.Actor{TenantID: tenant}

	// Belum ada bagan akun: metode baru tetap jadi, tanpa akun.
	m0, err := svc.Create(ctx, a, Input{Name: "QRIS Awal", Kind: KindEWallet})
	if err != nil || m0.Account != nil {
		t.Fatalf("tanpa COA: %+v %v", m0, err)
	}

	// Pasang bagan akun mini lalu hubungkan metode yang belum punya akun.
	acc := map[string]uuid.UUID{}
	for _, r := range []struct{ code, name, kind, class, parent string }{
		{"1100", "Aset Lancar", "group", "asset", ""}, {"1110", "Kas Toko", "ledger", "asset", "1100"}, {"1120", "Bank", "ledger", "asset", "1100"},
		{"1130", "QRIS / Dompet Digital", "ledger", "asset", "1100"}, {"1140", "Piutang Kartu / EDC", "ledger", "asset", "1100"},
		{"1320", "Uang Muka Pembelian", "ledger", "asset", "1100"}, {"2140", "Deposit Member", "ledger", "liability", "1100"},
	} {
		id := uuid.New()
		acc[r.code] = id
		var parent any
		if p, ok := acc[r.parent]; ok {
			parent = p
		}
		side := "debit"
		if r.class == "liability" {
			side = "credit"
		}
		if _, err := admin.Exec(ctx, `INSERT INTO accounts (id, tenant_id, parent_id, code, name, kind, class, normal_side) VALUES ($1,$2,$3,$4,$5,$6,$7,$8)`,
			id, tenant, parent, r.code, r.name, r.kind, r.class, side); err != nil {
			t.Fatal(err)
		}
	}
	if err := db.WithTenant(ctx, app, tenant, func(tx pgx.Tx) error { return LinkDefaultAccounts(ctx, tx, tenant) }); err != nil {
		t.Fatal(err)
	}
	list, _, err := svc.List(ctx, a, ListParams{Limit: 100})
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]string{"Tunai": "1110", "Transfer": "1120", "Debit": "1120", "Kartu Kredit": "1140", "E-Wallet": "1130", "QRIS Awal": "1130",
		"Deposit Member": "2140", "Kredit Pemasok": "1320"}
	for _, m := range list {
		if m.Account == nil || m.Account.Code != want[m.Name] {
			t.Fatalf("akun bawaan %s: %+v (harus %s)", m.Name, m.Account, want[m.Name])
		}
	}

	// Metode baru -> akun sendiri 1150 (kas/bank), kartu kredit -> 1151 (bukan kas/bank).
	bca, err := svc.Create(ctx, a, Input{Name: "QRIS BCA", Kind: KindEWallet})
	if err != nil || bca.Account == nil || bca.Account.Code != "1150" || bca.Account.Name != "QRIS BCA" {
		t.Fatalf("akun otomatis: %+v %v", bca, err)
	}
	cc, err := svc.Create(ctx, a, Input{Name: "Kartu BCA", Kind: KindCreditCard})
	if err != nil || cc.Account == nil || cc.Account.Code != "1151" {
		t.Fatalf("akun kartu: %+v %v", cc, err)
	}
	var cashBank bool
	var parent uuid.UUID
	if err := admin.QueryRow(ctx, `SELECT is_cash_bank, parent_id FROM accounts WHERE id = $1`, bca.Account.ID).Scan(&cashBank, &parent); err != nil || !cashBank || parent != acc["1100"] {
		t.Fatalf("atribut akun otomatis: %v %v %v", cashBank, parent, err)
	}
	if err := admin.QueryRow(ctx, `SELECT is_cash_bank FROM accounts WHERE id = $1`, cc.Account.ID).Scan(&cashBank); err != nil || cashBank {
		t.Fatalf("akun kartu bukan kas/bank: %v %v", cashBank, err)
	}

	// Ganti nama & arsip diikuti akunnya.
	if _, err := svc.Update(ctx, a, bca.ID, Input{Name: "QRIS BCA Toko"}); err != nil {
		t.Fatal(err)
	}
	var name string
	var active bool
	check := func(wantName string, wantActive bool) {
		t.Helper()
		if err := admin.QueryRow(ctx, `SELECT name, active FROM accounts WHERE id = $1`, bca.Account.ID).Scan(&name, &active); err != nil || name != wantName || active != wantActive {
			t.Fatalf("akun: %q %v %v (harus %q %v)", name, active, err, wantName, wantActive)
		}
	}
	check("QRIS BCA Toko", true)
	if _, err := svc.SetActive(ctx, a, bca.ID, false); err != nil {
		t.Fatal(err)
	}
	check("QRIS BCA Toko", false)
	if _, err := svc.SetActive(ctx, a, bca.ID, true); err != nil {
		t.Fatal(err)
	}
	check("QRIS BCA Toko", true)

	// Akun bersama (Bank 1120 dipakai Transfer & Debit) tidak ikut diubah saat salah satu diganti nama.
	var transfer Method
	for _, m := range list {
		if m.Name == "Transfer" {
			transfer = m
		}
	}
	if _, err := svc.Update(ctx, a, transfer.ID, Input{Name: "Transfer Bank"}); err != nil {
		t.Fatal(err)
	}
	if err := admin.QueryRow(ctx, `SELECT name FROM accounts WHERE id = $1`, acc["1120"]).Scan(&name); err != nil || name != "Bank" {
		t.Fatalf("akun bersama: %q %v", name, err)
	}

	// Pilih akun manual: akun buku aset aktif saja (bukan grup / kewajiban / tak dikenal); metode baru boleh langsung memilih.
	bank := acc["1120"]
	for _, bad := range []uuid.UUID{acc["1100"], acc["2140"], uuid.New()} {
		if _, err := svc.Update(ctx, a, bca.ID, Input{Name: "QRIS BCA Toko", AccountID: &bad}); fieldCode(err, "account_id") != "INVALID" {
			t.Fatalf("akun tak sah %s: %v", bad, err)
		}
	}
	if r, err := svc.Update(ctx, a, bca.ID, Input{Name: "QRIS BCA Toko", AccountID: &bank}); err != nil || r.Account == nil || r.Account.ID != bank {
		t.Fatalf("pilih akun: %+v %v", r, err)
	}
	pick, err := svc.Create(ctx, a, Input{Name: "Bayar Bank", Kind: KindTransfer, AccountID: &bank})
	if err != nil || pick.Account == nil || pick.Account.ID != bank {
		t.Fatalf("buat dengan akun pilihan: %+v %v", pick, err)
	}
}
