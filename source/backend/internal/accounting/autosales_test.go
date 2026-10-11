package accounting_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"aciraba/internal/accounting"
)

// salesEnv = tenant e.other dengan template retail, tiga metode bayar, satu barang, dan nota uji.
type salesEnv struct {
	*env
	outlet, item, unit, member uuid.UUID
	cash, qris, qrisCust       uuid.UUID
	deposit, supCredit         uuid.UUID
	n                          int
}

func newSalesEnv(t *testing.T) *salesEnv {
	t.Helper()
	e := newEnv(t)
	ctx := context.Background()
	s := &salesEnv{env: e, outlet: e.otherOwner.OutletID, item: uuid.New(), unit: uuid.New(), member: uuid.New(),
		cash: uuid.New(), qris: uuid.New(), qrisCust: uuid.New(), deposit: uuid.New(), supCredit: uuid.New()}
	for _, st := range []struct {
		sql  string
		args []any
	}{
		{`INSERT INTO units (id, tenant_id, name) VALUES ($1, $2, 'Pcs')`, []any{s.unit, e.other}},
		{`INSERT INTO items (id, tenant_id, sku, name, unit_id) VALUES ($1, $2, 'SKU-1', 'Barang Uji', $3)`, []any{s.item, e.other, s.unit}},
		{`INSERT INTO members (id, tenant_id, code, name) VALUES ($1, $2, 'M1', 'Budi')`, []any{s.member, e.other}},
		{`INSERT INTO payment_methods (id, tenant_id, name, kind, is_system) VALUES ($1, $2, 'Tunai', 'cash', true)`, []any{s.cash, e.other}},
		{`INSERT INTO payment_methods (id, tenant_id, name, kind, fee_pct, fee_bearer) VALUES ($1, $2, 'QRIS', 'ewallet', 1, 'store')`, []any{s.qris, e.other}},
		{`INSERT INTO payment_methods (id, tenant_id, name, kind, fee_pct, fee_bearer) VALUES ($1, $2, 'QRIS Pelanggan', 'ewallet', 1, 'customer')`, []any{s.qrisCust, e.other}},
		{`INSERT INTO payment_methods (id, tenant_id, name, kind, is_system) VALUES ($1, $2, 'Deposit Member', 'deposit', true)`, []any{s.deposit, e.other}},
		{`INSERT INTO payment_methods (id, tenant_id, name, kind, is_system) VALUES ($1, $2, 'Kredit Pemasok', 'supplier_credit', true)`, []any{s.supCredit, e.other}},
	} {
		if _, err := e.admin.Exec(ctx, st.sql, st.args...); err != nil {
			t.Fatal(err)
		}
	}
	t.Cleanup(func() { // berjalan sebelum cleanup newEnv (LIFO)
		for _, tbl := range []string{"supplier_credit_movements", "member_deposit_movements", "payable_payments", "receivable_payments", "payables", "receivables", "purchase_return_lines", "purchase_returns", "sales_return_lines", "sales_returns", "purchase_lines", "purchases", "suppliers", "sale_payments", "sale_lines", "sales", "payment_methods", "members", "items", "units"} {
			_, _ = e.admin.Exec(ctx, `DELETE FROM `+tbl+` WHERE tenant_id = $1`, e.other)
		}
	})
	if _, err := e.svc.SeedRetailTemplate(ctx, e.otherOwner); err != nil { // memetakan peran dan akun metode bayar
		t.Fatal(err)
	}
	return s
}

type payRow struct {
	method uuid.UUID
	kind   string
	name   string
	amount string
	fee    string
	bearer string
}

// sale menyisipkan satu nota completed beserta satu baris, pembayaran, dan (bila receivable != "") piutang.
func (s *salesEnv) sale(t *testing.T, at, status string, subtotal, discount, taxGov, other, total, paid, change, surcharge, qty, unitCost, receivable string, pays ...payRow) uuid.UUID {
	t.Helper()
	ctx := context.Background()
	s.n++
	id := uuid.New()
	var member any
	recvAmount := "0"
	if receivable != "" {
		member, recvAmount = s.member, receivable
	}
	var reason any
	if status == "void" {
		reason = "uji batal"
	}
	if _, err := s.admin.Exec(ctx, `INSERT INTO sales (id, tenant_id, outlet_id, doc_no, idempotency_key, request_hash, status, subtotal, discount,
		tax_store_pct, tax_gov_pct, tax_store, tax_gov, other_cost, total, paid, change, surcharge, void_reason, member_id, receivable, created_at)
		VALUES ($1, $2, $3, $4, $5, 'h', $6, $7, $8, 0, 0, 0, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18::timestamptz)`,
		id, s.other, s.outlet, "UJI-"+id.String()[:8], "idem-"+id.String(), status, subtotal, discount, taxGov, other, total, paid, change, surcharge, reason, member, recvAmount, at); err != nil {
		t.Fatalf("sales: %v", err)
	}
	if _, err := s.admin.Exec(ctx, `INSERT INTO sale_lines (tenant_id, sale_id, position, item_id, sku, name, unit_id, unit_name, factor, qty, list_price, unit_price, unit_cost, line_total)
		VALUES ($1, $2, 1, $3, 'SKU-1', 'Barang Uji', $4, 'Pcs', 1, $5, 1, 1, $6, $7)`, s.other, id, s.item, s.unit, qty, unitCost, subtotal); err != nil {
		t.Fatalf("sale_lines: %v", err)
	}
	for i, p := range pays {
		if _, err := s.admin.Exec(ctx, `INSERT INTO sale_payments (tenant_id, sale_id, position, method, amount, method_id, method_name, fee_amount, fee_bearer)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)`, s.other, id, i+1, p.kind, p.amount, p.method, p.name, p.fee, p.bearer); err != nil {
			t.Fatalf("sale_payments: %v", err)
		}
	}
	return id
}

// net = debit − kredit per kode akun dari baris-baris jurnal.
func net(t *testing.T, j accounting.Journal) map[string]decimal.Decimal {
	t.Helper()
	out := map[string]decimal.Decimal{}
	for _, l := range j.Lines {
		d, _ := decimal.NewFromString(l.Debit)
		c, _ := decimal.NewFromString(l.Credit)
		out[l.AccountCode] = out[l.AccountCode].Add(d).Sub(c)
	}
	return out
}

func wantNet(t *testing.T, got map[string]decimal.Decimal, want map[string]string) {
	t.Helper()
	for code, w := range want {
		wd, _ := decimal.NewFromString(w)
		if !got[code].Equal(wd) {
			t.Errorf("akun %s: net %s, mau %s", code, got[code], w)
		}
	}
	for code, g := range got {
		if _, ok := want[code]; !ok && !g.IsZero() {
			t.Errorf("akun %s tak terduga: net %s", code, g)
		}
	}
}

func TestSalesDailyDraft(t *testing.T) {
	s := newSalesEnv(t)
	ctx := context.Background()
	const day = "2026-03-10"
	in := accounting.GenerateInput{From: day, To: day, OutletID: s.outlet}
	cash := func(amount string) payRow { return payRow{s.cash, "cash", "Tunai", amount, "0", "store"} }

	s.sale(t, "2026-03-10 10:00+07", "completed", "20000", "0", "2000", "1000", "23000", "25000", "2000", "0", "2", "6000", "", cash("25000"))
	s2 := s.sale(t, "2026-03-10 11:00+07", "completed", "10000", "1000", "0", "0", "9000", "9000", "0", "0", "1", "4000", "",
		payRow{s.qris, "ewallet", "QRIS", "9000", "90", "store"})
	s.sale(t, "2026-03-10 12:00+07", "completed", "5000", "0", "0", "0", "5000", "5000", "0", "50", "1", "2000", "",
		payRow{s.qrisCust, "ewallet", "QRIS Pelanggan", "5000", "50", "customer"})
	s.sale(t, "2026-03-10 13:00+07", "completed", "8000", "0", "0", "0", "8000", "3000", "0", "0", "1", "3000", "5000", cash("3000"))
	s.sale(t, "2026-03-10 14:00+07", "void", "99999", "0", "0", "0", "99999", "99999", "0", "0", "1", "1", "", cash("99999"))
	s.sale(t, "2026-03-10 23:30+00", "completed", "777", "0", "0", "0", "777", "777", "0", "0", "1", "1", "", cash("777")) // = 11 Maret WIB

	res, err := s.svc.GenerateDrafts(ctx, s.otherOwner, in)
	if err != nil {
		t.Fatal(err)
	}
	if res.Created != 1 || len(res.Items) != 1 || res.Items[0].JournalID == nil {
		t.Fatalf("hasil pertama: %+v", res)
	}
	jid := *res.Items[0].JournalID
	j, err := s.svc.Get(ctx, s.otherOwner, jid)
	if err != nil {
		t.Fatal(err)
	}
	if j.Type != "SALES" || j.Status != "draft" || j.Date != day || j.TotalDebit != j.TotalCredit {
		t.Fatalf("draf: %+v", j)
	}
	wantNet(t, net(t, j), map[string]string{
		"1110": "26000", "1130": "13910", "6160": "140", "1210": "5000", "5100": "21000",
		"4100": "-42000", "2130": "-2000", "4200": "-1050", "1310": "-21000"})

	// Dihitung ulang selama masih draf: tetap satu jurnal.
	res, err = s.svc.GenerateDrafts(ctx, s.otherOwner, in)
	if err != nil || res.Updated != 1 || *res.Items[0].JournalID != jid {
		t.Fatalf("ulang: %+v %v", res, err)
	}

	// Diposting akuntan; tanpa perubahan nota -> unchanged.
	if _, err := s.svc.Post(ctx, s.otherOwner, jid); err != nil {
		t.Fatal(err)
	}
	if res, err = s.svc.GenerateDrafts(ctx, s.otherOwner, in); err != nil || res.Unchanged != 1 {
		t.Fatalf("setelah posting: %+v %v", res, err)
	}

	// Nota dibatalkan setelah jurnal diposting -> draf selisih, jurnal asal tidak berubah.
	if _, err := s.admin.Exec(ctx, `UPDATE sales SET status = 'void', void_reason = 'uji' WHERE id = $1`, s2); err != nil {
		t.Fatal(err)
	}
	res, err = s.svc.GenerateDrafts(ctx, s.otherOwner, in)
	if err != nil || res.Adjusted != 1 {
		t.Fatalf("selisih: %+v %v", res, err)
	}
	adj, err := s.svc.Get(ctx, s.otherOwner, *res.Items[0].JournalID)
	if err != nil {
		t.Fatal(err)
	}
	if adj.ID == jid || adj.Status != "draft" || adj.TotalDebit != adj.TotalCredit {
		t.Fatalf("draf selisih: %+v", adj)
	}
	wantNet(t, net(t, adj), map[string]string{"4100": "9000", "1130": "-8910", "6160": "-90", "5100": "-4000", "1310": "4000"})
	if res, err = s.svc.GenerateDrafts(ctx, s.otherOwner, in); err != nil || res.Adjusted != 1 || *res.Items[0].JournalID != adj.ID {
		t.Fatalf("selisih dihitung ulang: %+v %v", res, err)
	}
	if _, err := s.svc.Post(ctx, s.otherOwner, adj.ID); err != nil {
		t.Fatal(err)
	}
	if res, err = s.svc.GenerateDrafts(ctx, s.otherOwner, in); err != nil || res.Unchanged != 1 {
		t.Fatalf("setelah selisih diposting: %+v %v", res, err)
	}
	var rev decimal.Decimal
	if err := s.admin.QueryRow(ctx, `SELECT coalesce(sum(l.credit - l.debit), 0) FROM journal_lines l JOIN journal_entries e ON e.id = l.entry_id
		WHERE e.tenant_id = $1 AND e.status = 'posted' AND e.type = 'SALES' AND l.account_id = $2`, s.other, s.acc4100(t)).Scan(&rev); err != nil || !rev.Equal(decimal.NewFromInt(33000)) {
		t.Fatalf("penjualan terposting = %s (%v), mau 33000", rev, err)
	}

	// Hari lain berisi nota 23:30 UTC = 11 Maret WIB: hanya muncul di 11 Maret.
	next := accounting.GenerateInput{From: "2026-03-11", To: "2026-03-11", OutletID: s.outlet}
	res, err = s.svc.GenerateDrafts(ctx, s.otherOwner, next)
	if err != nil || res.Created != 1 {
		t.Fatalf("11 Maret: %+v %v", res, err)
	}
	// Pemetaan hilang -> galat jelas, tidak ada draf baru.
	s.sale(t, "2026-03-12 10:00+07", "completed", "1000", "0", "0", "0", "1000", "1000", "0", "0", "1", "500", "",
		payRow{s.qris, "ewallet", "QRIS", "1000", "10", "store"})
	if _, err := s.admin.Exec(ctx, `DELETE FROM account_mappings WHERE tenant_id = $1 AND key = 'fee_expense'`, s.other); err != nil {
		t.Fatal(err)
	}
	var miss *accounting.MissingMappingError
	if _, err := s.svc.GenerateDrafts(ctx, s.otherOwner, accounting.GenerateInput{From: "2026-03-12", To: "2026-03-12", OutletID: s.outlet}); !errors.As(err, &miss) || len(miss.Keys) != 1 || miss.Keys[0] != "fee_expense" {
		t.Fatalf("pemetaan hilang: %v", err)
	}

	// Akses: pengguna tanpa outlet ditolak; rentang salah ditolak.
	if _, err := s.svc.GenerateDrafts(ctx, s.limited, accounting.GenerateInput{From: day, To: day, OutletID: s.outlet}); !errors.Is(err, accounting.ErrOutletForbidden) {
		t.Fatalf("outlet terlarang: %v", err)
	}
	var fe accounting.FieldErrors
	if _, err := s.svc.GenerateDrafts(ctx, s.otherOwner, accounting.GenerateInput{From: "2026-03-10", To: "2026-01-01"}); !errors.As(err, &fe) {
		t.Fatalf("rentang terbalik: %v", err)
	}
	if _, err := s.svc.GenerateDrafts(ctx, s.otherOwner, accounting.GenerateInput{From: "2026-01-01", To: time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC).Format("2006-01-02")}); !errors.As(err, &fe) {
		t.Fatalf("rentang terlalu panjang: %v", err)
	}
}

func (s *salesEnv) acc4100(t *testing.T) uuid.UUID {
	t.Helper()
	var id uuid.UUID
	if err := s.admin.QueryRow(context.Background(), `SELECT id FROM accounts WHERE tenant_id = $1 AND code = '4100'`, s.other).Scan(&id); err != nil {
		t.Fatal(err)
	}
	return id
}

func TestPostBatch(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	good := e.draft(t, "2026-04-01", "100")
	bad, err := e.svc.SaveDraft(ctx, e.owner, uuid.Nil, accounting.JournalInput{Date: "2026-04-01", Type: "JU", Lines: []accounting.LineInput{ln(e.acc["6100"], "50", "0"), ln(e.acc["1110"], "0", "40")}})
	if err != nil {
		t.Fatal(err)
	}
	items, err := e.svc.PostBatch(ctx, e.owner, []uuid.UUID{good.ID, bad.ID, uuid.New()})
	if err != nil {
		t.Fatal(err)
	}
	if !items[0].OK || items[0].DocNo == "" || items[1].OK || items[1].Code != "JOURNAL_UNBALANCED" || items[2].OK || items[2].Code != "NOT_FOUND" {
		t.Fatalf("hasil: %+v", items)
	}
	var fe accounting.FieldErrors
	if _, err := e.svc.PostBatch(ctx, e.owner, nil); !errors.As(err, &fe) {
		t.Fatalf("kosong: %v", err)
	}
}
