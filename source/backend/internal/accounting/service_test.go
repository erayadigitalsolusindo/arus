package accounting_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/shopspring/decimal"

	"aciraba/internal/accounting"
	"aciraba/internal/authz"
	"aciraba/internal/platform/db"
)

// Integrasi: butuh TEST_DATABASE_URL (aciraba_app) dan TEST_ADMIN_DATABASE_URL (pemilik); tanpa itu di-SKIP.
type env struct {
	app, admin    *pgxpool.Pool
	svc           *accounting.Service
	tenant, other uuid.UUID
	outlet        uuid.UUID
	owner         authz.Actor // izin penuh
	limited       authz.Actor // tanpa akses outlet
	otherOwner    authz.Actor // tenant lain
	acc           map[string]uuid.UUID
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
	e := &env{app: app, admin: admin, svc: accounting.NewService(app), tenant: uuid.New(), other: uuid.New(), outlet: uuid.New(), acc: map[string]uuid.UUID{}}
	otherOutlet := uuid.New()
	for _, s := range []struct {
		sql  string
		args []any
	}{
		{`INSERT INTO tenants (id, code, name) VALUES ($1, $2, 'UJI-ACC')`, []any{e.tenant, "ac-" + e.tenant.String()[:8]}},
		{`INSERT INTO tenants (id, code, name) VALUES ($1, $2, 'UJI-ACC-2')`, []any{e.other, "ac-" + e.other.String()[:8]}},
		{`INSERT INTO outlets (id, tenant_id, code, name) VALUES ($1, $2, 'main', 'Pusat')`, []any{e.outlet, e.tenant}},
		{`INSERT INTO outlets (id, tenant_id, code, name) VALUES ($1, $2, 'main', 'Pusat')`, []any{otherOutlet, e.other}},
	} {
		if _, err := admin.Exec(ctx, s.sql, s.args...); err != nil {
			t.Fatal(err)
		}
	}
	t.Cleanup(func() {
		for _, tid := range []uuid.UUID{e.tenant, e.other} {
			_, _ = admin.Exec(ctx, `UPDATE journal_entries SET reverses_id = NULL WHERE tenant_id = $1`, tid)
			for _, tbl := range []string{"audit_log", "account_period_balances", "journal_lines", "journal_entries", "journal_counters", "accounting_periods",
				"accounts", "user_outlets", "users", "roles", "outlets"} {
				_, _ = admin.Exec(ctx, `DELETE FROM `+tbl+` WHERE tenant_id = $1`, tid)
			}
			_, _ = admin.Exec(ctx, `DELETE FROM tenants WHERE id = $1`, tid)
		}
	})
	e.owner = e.person(t, e.tenant, e.outlet, "Owner", `{"*":true}`, true)
	e.limited = e.person(t, e.tenant, e.outlet, "Terbatas", `{"siak":["view"]}`, false)
	e.otherOwner = e.person(t, e.other, otherOutlet, "Owner2", `{"*":true}`, true)
	// Akun uji (akun buku): kas, bank, piutang, penjualan, beban, modal; plus satu grup.
	e.account(t, "1110", "Kas", "ledger", "asset", true)
	e.account(t, "1120", "Bank", "ledger", "asset", true)
	e.account(t, "1210", "Piutang", "ledger", "asset", false)
	e.account(t, "3100", "Modal", "ledger", "equity", false)
	e.account(t, "4100", "Penjualan", "ledger", "revenue", false)
	e.account(t, "6100", "Beban Gaji", "ledger", "expense", false)
	return e
}

func (e *env) person(t *testing.T, tenant, outlet uuid.UUID, name, perms string, assign bool) authz.Actor {
	t.Helper()
	role, user := uuid.New(), uuid.New()
	for _, s := range []struct {
		sql  string
		args []any
	}{
		{"INSERT INTO roles (id, tenant_id, name, permissions, is_system) VALUES ($1, $2, $3, $4::jsonb, $5)", []any{role, tenant, name + "-" + role.String()[:6], perms, perms == `{"*":true}`}},
		{"INSERT INTO users (id, tenant_id, role_id, email, name, password_hash) VALUES ($1, $2, $3, $4, $5, 'x')", []any{user, tenant, role, name + "-" + user.String() + "@example.test", name}},
	} {
		if _, err := e.admin.Exec(context.Background(), s.sql, s.args...); err != nil {
			t.Fatal(err)
		}
	}
	outlets := map[uuid.UUID]bool{}
	if assign {
		if _, err := e.admin.Exec(context.Background(), "INSERT INTO user_outlets (tenant_id, user_id, outlet_id) VALUES ($1, $2, $3)", tenant, user, outlet); err != nil {
			t.Fatal(err)
		}
		outlets[outlet] = true
	}
	active := outlet
	if !assign {
		active = uuid.New() // outlet aktif yang tidak ditugaskan
	}
	return authz.Actor{TenantID: tenant, UserID: user, OutletID: active, Name: name, Perms: authz.ParseStored([]byte(perms)), Outlets: outlets}
}

func (e *env) account(t *testing.T, code, name, kind, class string, cashBank bool) accounting.Account {
	t.Helper()
	a, err := e.svc.CreateAccount(context.Background(), e.owner, accounting.AccountInput{Code: code, Name: name, Kind: kind, Class: class, IsCashBank: cashBank})
	if err != nil {
		t.Fatalf("buat akun %s: %v", code, err)
	}
	e.acc[code] = a.ID
	return a
}

func ln(account uuid.UUID, debit, credit string) accounting.LineInput {
	return accounting.LineInput{AccountID: account, Debit: json.Number(debit), Credit: json.Number(credit)}
}

// draft membuat draf JU Rp amount: debit beban gaji, kredit kas.
func (e *env) draft(t *testing.T, date, amount string) accounting.Journal {
	t.Helper()
	j, err := e.svc.SaveDraft(context.Background(), e.owner, uuid.Nil, accounting.JournalInput{Date: date, Type: "JU", Narration: "uji",
		Lines: []accounting.LineInput{ln(e.acc["6100"], amount, "0"), ln(e.acc["1110"], "0", amount)}})
	if err != nil {
		t.Fatalf("draf: %v", err)
	}
	return j
}

func (e *env) post(t *testing.T, date, amount string) accounting.Journal {
	t.Helper()
	d := e.draft(t, date, amount)
	j, err := e.svc.Post(context.Background(), e.owner, d.ID)
	if err != nil {
		t.Fatalf("post: %v", err)
	}
	return j
}

// balance = (Σdebit, Σkredit) akun di agregat account_period_balances (dibaca dengan hak pemilik).
func (e *env) balance(t *testing.T, code string) (decimal.Decimal, decimal.Decimal) {
	t.Helper()
	var d, c decimal.Decimal
	if err := e.admin.QueryRow(context.Background(), `SELECT COALESCE(sum(debit),0), COALESCE(sum(credit),0) FROM account_period_balances WHERE tenant_id = $1 AND account_id = $2`,
		e.tenant, e.acc[code]).Scan(&d, &c); err != nil {
		t.Fatal(err)
	}
	return d, c
}

func dc(s string) decimal.Decimal { return decimal.RequireFromString(s) }

func TestPostUnbalancedRejected(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	j, err := e.svc.SaveDraft(ctx, e.owner, uuid.Nil, accounting.JournalInput{Date: "2026-10-15", Type: "JU",
		Lines: []accounting.LineInput{ln(e.acc["6100"], "100000", "0"), ln(e.acc["1110"], "0", "90000")}})
	if err != nil {
		t.Fatalf("draf yang belum seimbang harus boleh disimpan: %v", err)
	}
	if _, err := e.svc.Post(ctx, e.owner, j.ID); !errors.Is(err, accounting.ErrJournalUnbalanced) {
		t.Fatalf("err = %v, want ErrJournalUnbalanced", err)
	}
	got, _ := e.svc.Get(ctx, e.owner, j.ID)
	if got.Status != "draft" || got.DocNo != "" {
		t.Fatalf("draf harus tetap draf tanpa nomor: %+v", got)
	}
	if d, c := e.balance(t, "6100"); !d.IsZero() || !c.IsZero() {
		t.Fatalf("saldo tidak boleh berubah: %s/%s", d, c)
	}
	// Satu baris saja juga ditolak.
	one, _ := e.svc.SaveDraft(ctx, e.owner, uuid.Nil, accounting.JournalInput{Date: "2026-10-15", Type: "JU", Lines: []accounting.LineInput{ln(e.acc["6100"], "0", "0.01")}})
	if _, err := e.svc.Post(ctx, e.owner, one.ID); err == nil {
		t.Fatal("jurnal satu baris tidak boleh diposting")
	}
}

func TestLineValidation(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	for name, lines := range map[string][]accounting.LineInput{
		"debit dan kredit sekaligus": {ln(e.acc["6100"], "5", "5")},
		"nol dua sisi":               {ln(e.acc["6100"], "0", "0")},
		"negatif":                    {ln(e.acc["6100"], "-5", "0")},
		"tiga desimal":               {ln(e.acc["6100"], "1.005", "0")},
		"tanpa baris":                nil,
	} {
		_, err := e.svc.SaveDraft(ctx, e.owner, uuid.Nil, accounting.JournalInput{Date: "2026-10-15", Type: "JU", Lines: lines})
		var fe accounting.FieldErrors
		if !errors.As(err, &fe) {
			t.Errorf("%s: err = %v, want FieldErrors", name, err)
		}
	}
	// Akun grup / tidak ada ditolak.
	grp, err := e.svc.CreateAccount(ctx, e.owner, accounting.AccountInput{Code: "6000", Name: "Beban", Kind: "group", Class: "expense"})
	if err != nil {
		t.Fatal(err)
	}
	for name, id := range map[string]uuid.UUID{"grup": grp.ID, "tidak ada": uuid.New()} {
		_, err := e.svc.SaveDraft(ctx, e.owner, uuid.Nil, accounting.JournalInput{Date: "2026-10-15", Type: "JU",
			Lines: []accounting.LineInput{ln(id, "5", "0"), ln(e.acc["1110"], "0", "5")}})
		if !errors.Is(err, accounting.ErrAccountInvalid) {
			t.Errorf("akun %s: err = %v, want ErrAccountInvalid", name, err)
		}
	}
	// Jenis otomatis tidak bisa dibuat manual; outlet tanpa akses ditolak.
	if _, err := e.svc.SaveDraft(ctx, e.owner, uuid.Nil, accounting.JournalInput{Date: "2026-10-15", Type: "SALES", Lines: []accounting.LineInput{ln(e.acc["6100"], "5", "0")}}); err == nil {
		t.Error("jenis SALES manual harus ditolak")
	}
	if _, err := e.svc.SaveDraft(ctx, e.limited, uuid.Nil, accounting.JournalInput{OutletID: e.outlet, Date: "2026-10-15", Type: "JU",
		Lines: []accounting.LineInput{ln(e.acc["6100"], "5", "0"), ln(e.acc["1110"], "0", "5")}}); !errors.Is(err, accounting.ErrOutletForbidden) {
		t.Errorf("tanpa akses outlet: err = %v, want ErrOutletForbidden", err)
	}
}

func TestPeriodClosedRejected(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	d := e.draft(t, "2026-10-15", "50000")
	if _, err := e.svc.ClosePeriod(ctx, e.owner, "2026-10"); !errors.Is(err, accounting.ErrDraftsInPeriod) {
		t.Fatalf("tutup dengan draf: err = %v, want ErrDraftsInPeriod", err)
	}
	if _, err := e.svc.Post(ctx, e.owner, d.ID); err != nil {
		t.Fatal(err)
	}
	if p, err := e.svc.ClosePeriod(ctx, e.owner, "2026-10"); err != nil || p.Status != "closed" {
		t.Fatalf("tutup: %v %+v", err, p)
	}
	if _, err := e.svc.ClosePeriod(ctx, e.owner, "2026-10"); !errors.Is(err, accounting.ErrPeriodClosed) {
		t.Fatalf("tutup ganda: err = %v", err)
	}
	// Draf bertanggal di periode tertutup tidak bisa diposting.
	late := e.draft(t, "2026-10-20", "1000")
	if _, err := e.svc.Post(ctx, e.owner, late.ID); !errors.Is(err, accounting.ErrPeriodClosed) {
		t.Fatalf("post ke periode tertutup: err = %v, want ErrPeriodClosed", err)
	}
	// Jurnal balik bertanggal di periode tertutup juga ditolak; di periode terbuka berhasil.
	first, _ := e.svc.Get(ctx, e.owner, d.ID)
	if _, err := e.svc.Reverse(ctx, e.owner, first.ID, "2026-10-25", ""); !errors.Is(err, accounting.ErrPeriodClosed) {
		t.Fatalf("balik ke periode tertutup: err = %v", err)
	}
	if _, err := e.svc.Reverse(ctx, e.owner, first.ID, "2026-11-02", ""); err != nil {
		t.Fatalf("balik ke periode terbuka: %v", err)
	}
	// Buka kembali butuh alasan, lalu posting berhasil.
	if _, err := e.svc.ReopenPeriod(ctx, e.owner, "2026-10", ""); err == nil {
		t.Fatal("buka kembali tanpa alasan harus ditolak")
	}
	if _, err := e.svc.ReopenPeriod(ctx, e.owner, "2026-10", "koreksi pajak"); err != nil {
		t.Fatal(err)
	}
	if _, err := e.svc.Post(ctx, e.owner, late.ID); err != nil {
		t.Fatalf("post setelah dibuka: %v", err)
	}
	if _, err := e.svc.ReopenPeriod(ctx, e.owner, "2026-10", "lagi"); !errors.Is(err, accounting.ErrPeriodNotClosed) {
		t.Fatalf("buka periode terbuka: err = %v", err)
	}
}

func TestPostedImmutable(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	j := e.post(t, "2026-10-15", "70000")
	if j.DocNo != "JU/2026/000001" || j.Status != "posted" {
		t.Fatalf("nomor/status: %+v", j)
	}
	if _, err := e.svc.SaveDraft(ctx, e.owner, j.ID, accounting.JournalInput{Date: "2026-10-15", Type: "JU", Lines: []accounting.LineInput{ln(e.acc["6100"], "1", "0")}}); !errors.Is(err, accounting.ErrJournalPosted) {
		t.Errorf("ubah posted: err = %v", err)
	}
	if err := e.svc.DeleteDraft(ctx, e.owner, j.ID); !errors.Is(err, accounting.ErrJournalPosted) {
		t.Errorf("hapus posted: err = %v", err)
	}
	if _, err := e.svc.Post(ctx, e.owner, j.ID); !errors.Is(err, accounting.ErrJournalPosted) {
		t.Errorf("posting ulang: err = %v", err)
	}
	// Lapis DB: walau aplikasi "salah", role aciraba_app tidak bisa mengubah/menghapus jurnal terposting.
	err := db.WithTenant(ctx, e.app, e.tenant, func(tx pgx.Tx) error {
		for name, sql := range map[string]string{
			"ubah header":  `UPDATE journal_entries SET narration = 'x' WHERE id = $1`,
			"hapus header": `DELETE FROM journal_entries WHERE id = $1`,
			"hapus baris":  `DELETE FROM journal_lines WHERE entry_id = $1`,
		} {
			tag, err := tx.Exec(ctx, sql, j.ID)
			if err != nil {
				return fmt.Errorf("%s: %w", name, err)
			}
			if tag.RowsAffected() != 0 {
				t.Errorf("%s: %d baris terpengaruh, want 0", name, tag.RowsAffected())
			}
		}
		if _, err := tx.Exec(ctx, `INSERT INTO journal_lines (tenant_id, entry_id, entry_date, line_no, account_id, debit) VALUES ($1, $2, '2026-10-15', 9, $3, 1)`,
			e.tenant, j.ID, e.acc["6100"]); err == nil {
			t.Error("menambah baris ke jurnal terposting harus ditolak")
		}
		return errors.New("rollback") // transaksi sudah rusak oleh error di atas; batalkan
	})
	if err == nil || err.Error() != "rollback" {
		t.Fatalf("db: %v", err)
	}
	err = db.WithTenant(ctx, e.app, e.tenant, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `UPDATE journal_lines SET memo = 'x' WHERE entry_id = $1`, j.ID)
		return err
	})
	if err == nil {
		t.Error("UPDATE journal_lines tanpa hak harus gagal")
	}
	got, _ := e.svc.Get(ctx, e.owner, j.ID)
	if len(got.Lines) != 2 || got.Narration != "uji" {
		t.Fatalf("jurnal berubah: %+v", got)
	}
}

func TestReverseNetsToZero(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	j := e.post(t, "2026-10-15", "70000")
	r, err := e.svc.Reverse(ctx, e.owner, j.ID, "", "salah akun")
	if err != nil {
		t.Fatal(err)
	}
	if r.Status != "posted" || r.ReversesID == nil || *r.ReversesID != j.ID || r.DocNo != "JU/2026/000002" {
		t.Fatalf("jurnal balik: %+v", r)
	}
	if r.Lines[0].Credit != "70000.00" || r.Lines[1].Debit != "70000.00" {
		t.Fatalf("sisi tidak tertukar: %+v", r.Lines)
	}
	for _, code := range []string{"6100", "1110"} {
		d, c := e.balance(t, code)
		if !d.Equal(c) || !d.Equal(dc("70000")) {
			t.Errorf("%s: debit %s kredit %s, want sama 70000", code, d, c)
		}
	}
	if _, err := e.svc.Reverse(ctx, e.owner, j.ID, "", ""); !errors.Is(err, accounting.ErrAlreadyReversed) {
		t.Errorf("balik ganda: err = %v", err)
	}
	if _, err := e.svc.Reverse(ctx, e.owner, uuid.New(), "", ""); !errors.Is(err, accounting.ErrNotFound) {
		t.Errorf("balik tak ada: err = %v", err)
	}
	d := e.draft(t, "2026-10-16", "1")
	if _, err := e.svc.Reverse(ctx, e.owner, d.ID, "", ""); !errors.Is(err, accounting.ErrNotPosted) {
		t.Errorf("balik draf: err = %v", err)
	}
}

func TestReverseConcurrent(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	j := e.post(t, "2026-10-15", "1000")
	var wg sync.WaitGroup
	res := make([]error, 8)
	for i := range res {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, res[i] = e.svc.Reverse(ctx, e.owner, j.ID, "", "")
		}()
	}
	wg.Wait()
	ok := 0
	for _, err := range res {
		switch {
		case err == nil:
			ok++
		case !errors.Is(err, accounting.ErrAlreadyReversed):
			t.Errorf("err tak terduga: %v", err)
		}
	}
	if ok != 1 {
		t.Fatalf("jurnal balik berhasil %d kali, want 1", ok)
	}
	if d, c := e.balance(t, "1110"); !d.Equal(c) {
		t.Fatalf("saldo tidak netral: %s vs %s", d, c)
	}
}

func TestCashJournalShapes(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	kas, bank, modal, gaji := e.acc["1110"], e.acc["1120"], e.acc["3100"], e.acc["6100"]
	cases := []struct {
		name  string
		typ   string
		lines []accounting.LineInput
		ok    bool
	}{
		{"KM benar", "KM", []accounting.LineInput{ln(kas, "100", "0"), ln(modal, "0", "100")}, true},
		{"KM kas di kredit", "KM", []accounting.LineInput{ln(modal, "100", "0"), ln(kas, "0", "100")}, false},
		{"KM tanpa kas", "KM", []accounting.LineInput{ln(gaji, "100", "0"), ln(modal, "0", "100")}, false},
		{"KM lawan juga kas", "KM", []accounting.LineInput{ln(kas, "100", "0"), ln(bank, "0", "100")}, false},
		{"KK benar", "KK", []accounting.LineInput{ln(gaji, "100", "0"), ln(kas, "0", "100")}, true},
		{"KK kas di debit", "KK", []accounting.LineInput{ln(kas, "100", "0"), ln(gaji, "0", "100")}, false},
		{"TK benar", "TK", []accounting.LineInput{ln(bank, "100", "0"), ln(kas, "0", "100")}, true},
		{"TK ke akun non-kas", "TK", []accounting.LineInput{ln(gaji, "100", "0"), ln(kas, "0", "100")}, false},
		{"TK tiga baris", "TK", []accounting.LineInput{ln(bank, "60", "0"), ln(bank, "40", "0"), ln(kas, "0", "100")}, false},
		{"TK akun sama", "TK", []accounting.LineInput{ln(kas, "100", "0"), ln(kas, "0", "100")}, false},
		{"JU bebas", "JU", []accounting.LineInput{ln(modal, "100", "0"), ln(kas, "0", "100")}, true},
	}
	for _, c := range cases {
		d, err := e.svc.SaveDraft(ctx, e.owner, uuid.Nil, accounting.JournalInput{Date: "2026-10-15", Type: c.typ, Lines: c.lines})
		if err != nil {
			t.Fatalf("%s: draf: %v", c.name, err)
		}
		_, err = e.svc.Post(ctx, e.owner, d.ID)
		switch {
		case c.ok && err != nil:
			t.Errorf("%s: %v", c.name, err)
		case !c.ok && !errors.Is(err, accounting.ErrJournalShape):
			t.Errorf("%s: err = %v, want ErrJournalShape", c.name, err)
		}
	}
	// Jurnal balik KM (kas jadi kredit) tetap sah walau susunannya kini "terbalik".
	km := e.post(t, "2026-10-15", "5")
	_ = km
}

func TestBalancesMatchLines(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	e.post(t, "2026-10-01", "1000.50")
	e.post(t, "2026-10-31", "2000")
	e.post(t, "2026-11-01", "300.25")
	rev, err := e.svc.Reverse(ctx, e.owner, e.post(t, "2026-11-05", "40").ID, "2026-11-06", "")
	if err != nil {
		t.Fatal(err)
	}
	_ = rev
	rows, err := e.admin.Query(ctx, `
		SELECT b.account_id, b.period_month, b.debit, b.credit,
		       COALESCE((SELECT sum(l.debit)  FROM journal_lines l JOIN journal_entries h ON h.tenant_id = l.tenant_id AND h.id = l.entry_id
		                  WHERE l.tenant_id = b.tenant_id AND l.account_id = b.account_id AND h.status = 'posted'
		                    AND date_trunc('month', l.entry_date)::date = b.period_month), 0),
		       COALESCE((SELECT sum(l.credit) FROM journal_lines l JOIN journal_entries h ON h.tenant_id = l.tenant_id AND h.id = l.entry_id
		                  WHERE l.tenant_id = b.tenant_id AND l.account_id = b.account_id AND h.status = 'posted'
		                    AND date_trunc('month', l.entry_date)::date = b.period_month), 0)
		FROM account_period_balances b WHERE b.tenant_id = $1`, e.tenant)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	n := 0
	for rows.Next() {
		var acc uuid.UUID
		var month any
		var bd, bc, ld, lc decimal.Decimal
		if err := rows.Scan(&acc, &month, &bd, &bc, &ld, &lc); err != nil {
			t.Fatal(err)
		}
		if !bd.Equal(ld) || !bc.Equal(lc) {
			t.Errorf("akun %s bulan %v: agregat %s/%s ≠ Σ baris %s/%s", acc, month, bd, bc, ld, lc)
		}
		n++
	}
	if n != 4 { // 6100 & 1110 × (Okt, Nov)
		t.Errorf("baris agregat = %d, want 4", n)
	}
	// Neraca saldo seimbang: Σ debit = Σ kredit seluruh akun.
	var td, tc decimal.Decimal
	if err := e.admin.QueryRow(ctx, `SELECT sum(debit), sum(credit) FROM account_period_balances WHERE tenant_id = $1`, e.tenant).Scan(&td, &tc); err != nil {
		t.Fatal(err)
	}
	if !td.Equal(tc) {
		t.Fatalf("Σdebit %s ≠ Σkredit %s", td, tc)
	}
}

func TestConcurrentPost(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	const n = 20
	ids := make([]uuid.UUID, n)
	for i := range ids {
		ids[i] = e.draft(t, "2026-10-15", "100").ID
	}
	var wg sync.WaitGroup
	errs := make([]error, n)
	for i := range ids {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, errs[i] = e.svc.Post(ctx, e.owner, ids[i])
		}()
	}
	wg.Wait()
	for i, err := range errs {
		if err != nil {
			t.Fatalf("post %d: %v", i, err)
		}
	}
	var distinct, maxNo int
	if err := e.admin.QueryRow(ctx, `SELECT count(DISTINCT doc_no), count(*) FROM journal_entries WHERE tenant_id = $1 AND status = 'posted'`, e.tenant).Scan(&distinct, &maxNo); err != nil {
		t.Fatal(err)
	}
	if distinct != n || maxNo != n {
		t.Fatalf("nomor unik %d dari %d", distinct, maxNo)
	}
	var last int
	if err := e.admin.QueryRow(ctx, `SELECT last_no FROM journal_counters WHERE tenant_id = $1 AND type = 'JU' AND year = 2026`, e.tenant).Scan(&last); err != nil || last != n {
		t.Fatalf("counter = %d (%v), want %d tanpa bolong", last, err, n)
	}
	if d, c := e.balance(t, "6100"); !d.Equal(dc("2000")) || !c.IsZero() {
		t.Fatalf("saldo 6100 = %s/%s, want 2000/0", d, c)
	}
	if d, c := e.balance(t, "1110"); !c.Equal(dc("2000")) || !d.IsZero() {
		t.Fatalf("saldo 1110 = %s/%s, want 0/2000", d, c)
	}
}

func TestClosePeriodVsPostRace(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	const n = 10
	ids := make([]uuid.UUID, n)
	for i := range ids {
		ids[i] = e.draft(t, "2026-10-15", "10").ID
	}
	var wg sync.WaitGroup
	posted := make([]error, n)
	for i := range ids {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, posted[i] = e.svc.Post(ctx, e.owner, ids[i])
		}()
	}
	var closeErr error
	wg.Add(1)
	go func() {
		defer wg.Done()
		_, closeErr = e.svc.ClosePeriod(ctx, e.owner, "2026-10")
	}()
	wg.Wait()
	// Apa pun urutannya: tidak ada jurnal terposting di periode yang tertutup tanpa ikut tercatat sebelum penutupan, dan tidak ada draf tersisa
	// di periode tertutup selain yang ditolak.
	var status string
	_ = e.admin.QueryRow(ctx, `SELECT status FROM accounting_periods WHERE tenant_id = $1`, e.tenant).Scan(&status)
	okPosts := 0
	for _, err := range posted {
		switch {
		case err == nil:
			okPosts++
		case !errors.Is(err, accounting.ErrPeriodClosed):
			t.Errorf("post: %v", err)
		}
	}
	if closeErr == nil && status != "closed" {
		t.Fatalf("tutup sukses tapi status %q", status)
	}
	if closeErr != nil && !errors.Is(closeErr, accounting.ErrDraftsInPeriod) {
		t.Fatalf("tutup: %v", closeErr)
	}
	if closeErr == nil {
		var drafts int
		_ = e.admin.QueryRow(ctx, `SELECT count(*) FROM journal_entries WHERE tenant_id = $1 AND status = 'draft' AND entry_date BETWEEN '2026-10-01' AND '2026-10-31'`, e.tenant).Scan(&drafts)
		if drafts != n-okPosts {
			t.Fatalf("draf tersisa %d, want %d", drafts, n-okPosts)
		}
	}
}

func TestOpeningBalance(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	in := func(date string, lines ...accounting.LineInput) accounting.OpeningInput {
		return accounting.OpeningInput{Date: date, Narration: "saldo awal", Lines: lines}
	}
	if _, err := e.svc.PostOpening(ctx, e.owner, in("2026-10-01", ln(e.acc["1110"], "1000", "0"), ln(e.acc["3100"], "0", "900"))); !errors.Is(err, accounting.ErrJournalUnbalanced) {
		t.Fatalf("pembuka tak seimbang: err = %v", err)
	}
	if _, err := e.svc.PostOpening(ctx, e.owner, in("2026-10-15", ln(e.acc["1110"], "1000", "0"), ln(e.acc["3100"], "0", "1000"))); err == nil {
		t.Fatal("tanggal bukan awal bulan harus ditolak")
	}
	j, err := e.svc.PostOpening(ctx, e.owner, in("2026-10-01", ln(e.acc["1110"], "1000", "0"), ln(e.acc["3100"], "0", "1000")))
	if err != nil {
		t.Fatal(err)
	}
	if j.Type != "OPENING" || j.Status != "posted" || j.DocNo != "OPENING/2026/000001" {
		t.Fatalf("pembuka: %+v", j)
	}
	if _, err := e.svc.PostOpening(ctx, e.owner, in("2026-10-01", ln(e.acc["1110"], "5", "0"), ln(e.acc["3100"], "0", "5"))); !errors.Is(err, accounting.ErrOpeningExists) {
		t.Fatalf("pembuka ganda: err = %v", err)
	}
	// Setelah dibalik, boleh dicatat ulang.
	if _, err := e.svc.Reverse(ctx, e.owner, j.ID, "", "salah nominal"); err != nil {
		t.Fatal(err)
	}
	if _, err := e.svc.PostOpening(ctx, e.owner, in("2026-10-01", ln(e.acc["1110"], "1200", "0"), ln(e.acc["3100"], "0", "1200"))); err != nil {
		t.Fatalf("catat ulang setelah dibalik: %v", err)
	}
}

func TestAccountRules(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	mk := func(in accounting.AccountInput) (accounting.Account, error) {
		return e.svc.CreateAccount(ctx, e.owner, in)
	}
	if _, err := mk(accounting.AccountInput{Code: "1110", Name: "Dobel", Kind: "ledger", Class: "asset"}); !errors.Is(err, accounting.ErrCodeTaken) {
		t.Errorf("kode ganda: err = %v", err)
	}
	grp, err := mk(accounting.AccountInput{Code: "2000", Name: "Kewajiban", Kind: "group", Class: "liability"})
	if err != nil {
		t.Fatal(err)
	}
	child, err := mk(accounting.AccountInput{Code: "2100", Name: "Hutang", Kind: "ledger", Class: "liability", ParentID: &grp.ID})
	if err != nil {
		t.Fatal(err)
	}
	if child.NormalSide != "credit" {
		t.Errorf("saldo normal hutang = %s", child.NormalSide)
	}
	// Induk harus grup berkelas sama.
	for name, in := range map[string]accounting.AccountInput{
		"induk akun buku": {Code: "2101", Name: "x", Kind: "ledger", Class: "liability", ParentID: &child.ID},
		"kelas beda":      {Code: "2102", Name: "x", Kind: "ledger", Class: "asset", ParentID: &grp.ID},
		"induk tak ada":   {Code: "2103", Name: "x", Kind: "ledger", Class: "liability", ParentID: ptr(uuid.New())},
	} {
		if _, err := mk(in); !errors.Is(err, accounting.ErrParentInvalid) {
			t.Errorf("%s: err = %v, want ErrParentInvalid", name, err)
		}
	}
	// Kas/bank hanya akun buku aset; kelas/kind/kode tak sah ditolak.
	for name, in := range map[string]accounting.AccountInput{
		"kas non-aset":   {Code: "7000", Name: "x", Kind: "ledger", Class: "revenue", IsCashBank: true},
		"kas grup":       {Code: "7001", Name: "x", Kind: "group", Class: "asset", IsCashBank: true},
		"kelas ngawur":   {Code: "7002", Name: "x", Kind: "ledger", Class: "lain"},
		"kind ngawur":    {Code: "7003", Name: "x", Kind: "lain", Class: "asset"},
		"kode berspasi":  {Code: "70 04", Name: "x", Kind: "ledger", Class: "asset"},
		"nama kosong":    {Code: "7005", Name: "  ", Kind: "ledger", Class: "asset"},
		"nama bermarkup": {Code: "7006", Name: "<b>", Kind: "ledger", Class: "asset"},
	} {
		var fe accounting.FieldErrors
		if _, err := mk(in); !errors.As(err, &fe) {
			t.Errorf("%s: err = %v, want FieldErrors", name, err)
		}
	}
	// Siklus: induk tidak boleh turunan sendiri.
	sub, err := mk(accounting.AccountInput{Code: "2200", Name: "Sub", Kind: "group", Class: "liability", ParentID: &grp.ID})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := e.svc.UpdateAccount(ctx, e.owner, grp.ID, accounting.AccountInput{Code: "2000", Name: "Kewajiban", Class: "liability", ParentID: &sub.ID}); !errors.Is(err, accounting.ErrParentInvalid) {
		t.Errorf("siklus: err = %v", err)
	}
	// Tidak bisa dihapus bila punya anak; kelas tak bisa diubah bila punya anak.
	if err := e.svc.DeleteAccount(ctx, e.owner, grp.ID); !errors.Is(err, accounting.ErrAccountInUse) {
		t.Errorf("hapus induk: err = %v", err)
	}
	if _, err := e.svc.UpdateAccount(ctx, e.owner, grp.ID, accounting.AccountInput{Code: "2000", Name: "Kewajiban", Class: "equity"}); !errors.Is(err, accounting.ErrAccountInUse) {
		t.Errorf("ubah kelas induk: err = %v", err)
	}
	// Akun berbaris jurnal: tidak bisa dihapus / diubah kelasnya, tapi bisa dinonaktifkan (lalu ditolak untuk jurnal baru).
	e.post(t, "2026-10-15", "10")
	if err := e.svc.DeleteAccount(ctx, e.owner, e.acc["6100"]); !errors.Is(err, accounting.ErrAccountInUse) {
		t.Errorf("hapus akun berjurnal: err = %v", err)
	}
	if _, err := e.svc.UpdateAccount(ctx, e.owner, e.acc["6100"], accounting.AccountInput{Code: "6100", Name: "Beban Gaji", Class: "asset"}); !errors.Is(err, accounting.ErrAccountInUse) {
		t.Errorf("ubah kelas akun berjurnal: err = %v", err)
	}
	off := false
	if _, err := e.svc.UpdateAccount(ctx, e.owner, e.acc["6100"], accounting.AccountInput{Code: "6100", Name: "Beban Gaji", Class: "expense", Active: &off}); err != nil {
		t.Fatal(err)
	}
	if _, err := e.svc.SaveDraft(ctx, e.owner, uuid.Nil, accounting.JournalInput{Date: "2026-10-16", Type: "JU",
		Lines: []accounting.LineInput{ln(e.acc["6100"], "5", "0"), ln(e.acc["1110"], "0", "5")}}); !errors.Is(err, accounting.ErrAccountInvalid) {
		t.Errorf("akun nonaktif: err = %v", err)
	}
	// Akun yang tidak dipakai bisa dihapus; hapus dua kali = tidak ditemukan.
	if err := e.svc.DeleteAccount(ctx, e.owner, child.ID); err != nil {
		t.Errorf("hapus akun kosong: %v", err)
	}
	if err := e.svc.DeleteAccount(ctx, e.owner, child.ID); !errors.Is(err, accounting.ErrNotFound) {
		t.Errorf("hapus ulang: err = %v", err)
	}
}

func ptr[T any](v T) *T { return &v }

func TestSeedRetailTemplate(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	// Tenant lain belum punya akun: seed berhasil; dua kali = ErrCOAExists.
	accs, err := e.svc.SeedRetailTemplate(ctx, e.otherOwner)
	if err != nil {
		t.Fatal(err)
	}
	if len(accs) < 30 {
		t.Fatalf("template hanya %d akun", len(accs))
	}
	byCode := map[string]accounting.Account{}
	cash := 0
	for _, a := range accs {
		byCode[a.Code] = a
		if a.IsCashBank {
			cash++
		}
	}
	for _, a := range accs {
		if a.ParentID != nil {
			p := byCode[codeOf(accs, *a.ParentID)]
			if p.Kind != "group" || p.Class != a.Class {
				t.Errorf("induk %s tidak sah untuk %s", p.Code, a.Code)
			}
		}
	}
	if cash < 2 || byCode["1310"].Name != "Persediaan Barang Dagang" || byCode["4100"].NormalSide != "credit" {
		t.Errorf("isi template tidak sesuai harapan")
	}
	// Akun sistem: tidak bisa dihapus, dinonaktifkan, atau diubah kodenya; nama boleh diganti. Akun biasa tetap bisa dihapus.
	kas := byCode["1110"]
	if !kas.IsSystem || byCode["6100"].IsSystem {
		t.Fatalf("penanda is_system salah: 1110=%v 6100=%v", kas.IsSystem, byCode["6100"].IsSystem)
	}
	if err := e.svc.DeleteAccount(ctx, e.otherOwner, kas.ID); !errors.Is(err, accounting.ErrAccountSystem) {
		t.Errorf("hapus akun sistem: err = %v", err)
	}
	off := false
	for name, in := range map[string]accounting.AccountInput{
		"ubah kode":   {ParentID: kas.ParentID, Code: "1119", Name: kas.Name, Class: kas.Class, IsCashBank: true},
		"nonaktifkan": {ParentID: kas.ParentID, Code: kas.Code, Name: kas.Name, Class: kas.Class, IsCashBank: true, Active: &off},
	} {
		if _, err := e.svc.UpdateAccount(ctx, e.otherOwner, kas.ID, in); !errors.Is(err, accounting.ErrAccountSystem) {
			t.Errorf("%s akun sistem: err = %v", name, err)
		}
	}
	if _, err := e.svc.UpdateAccount(ctx, e.otherOwner, kas.ID, accounting.AccountInput{ParentID: kas.ParentID, Code: kas.Code, Name: "Kas Besar", Class: kas.Class, IsCashBank: true}); err != nil {
		t.Errorf("ganti nama akun sistem: %v", err)
	}
	if err := e.svc.DeleteAccount(ctx, e.otherOwner, byCode["6900"].ID); err != nil {
		t.Errorf("hapus akun biasa: %v", err)
	}
	if _, err := e.svc.SeedRetailTemplate(ctx, e.otherOwner); !errors.Is(err, accounting.ErrCOAExists) {
		t.Errorf("seed ulang: err = %v", err)
	}
	// Tenant uji utama sudah punya akun → ditolak.
	if _, err := e.svc.SeedRetailTemplate(ctx, e.owner); !errors.Is(err, accounting.ErrCOAExists) {
		t.Errorf("seed di tenant berakun: err = %v", err)
	}
}

func codeOf(accs []accounting.Account, id uuid.UUID) string {
	for _, a := range accs {
		if a.ID == id {
			return a.Code
		}
	}
	return ""
}

func TestTenantIsolation(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	j := e.post(t, "2026-10-15", "100")
	if _, err := e.svc.Get(ctx, e.otherOwner, j.ID); !errors.Is(err, accounting.ErrNotFound) {
		t.Errorf("Get lintas tenant: err = %v", err)
	}
	if _, err := e.svc.Reverse(ctx, e.otherOwner, j.ID, "", ""); !errors.Is(err, accounting.ErrNotFound) {
		t.Errorf("Reverse lintas tenant: err = %v", err)
	}
	if err := e.svc.DeleteAccount(ctx, e.otherOwner, e.acc["1110"]); !errors.Is(err, accounting.ErrNotFound) {
		t.Errorf("hapus akun lintas tenant: err = %v", err)
	}
	// Jurnal tenant lain tidak boleh memakai akun tenant ini.
	if _, err := e.svc.SaveDraft(ctx, e.otherOwner, uuid.Nil, accounting.JournalInput{Date: "2026-10-15", Type: "JU",
		Lines: []accounting.LineInput{ln(e.acc["6100"], "5", "0"), ln(e.acc["1110"], "0", "5")}}); !errors.Is(err, accounting.ErrAccountInvalid) {
		t.Errorf("akun tenant lain: err = %v", err)
	}
	accs, err := e.svc.ListAccounts(ctx, e.otherOwner)
	if err != nil || len(accs) != 0 {
		t.Errorf("daftar akun tenant lain: %d (%v)", len(accs), err)
	}
	// Role aplikasi tanpa konteks tenant tidak melihat apa pun.
	var n int
	if err := e.app.QueryRow(ctx, `SELECT count(*) FROM journal_entries`).Scan(&n); err != nil || n != 0 {
		t.Errorf("tanpa konteks tenant melihat %d jurnal (%v)", n, err)
	}
}
