package sales

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"aciraba/internal/authz"
	"aciraba/internal/platform/db"
	"aciraba/internal/receivable"
)

func jn(s string) json.Number { return json.Number(s) }

// creditMember = member dengan limit piutang `limit` (0 = tanpa batas) dan jatuh tempo 30 hari.
func (e *env) creditMember(t *testing.T, limit string) uuid.UUID {
	t.Helper()
	m := e.newMember(t, 0, true)
	e.exec(t, `UPDATE members SET credit_limit = $2, due_days = 30 WHERE id = $1`, m, limit)
	return m
}

func (e *env) scoped() authz.Actor {
	a := e.actor(e.tenant)
	a.Outlets = map[uuid.UUID]bool{e.outlet: true}
	return a
}

func creditReq(it uuid.UUID, qty string, m *uuid.UUID, dp string, ap *ApprovalIn) Request {
	r := Request{Lines: []LineIn{line(it, qty)}, MemberID: m, Credit: true, Approval: ap}
	if dp != "" {
		r.Payments = []PaymentIn{pay("cash", dp)}
	}
	return r
}

func (e *env) methodID(t *testing.T, kind string) uuid.UUID {
	t.Helper()
	var id uuid.UUID
	if err := e.admin.QueryRow(context.Background(), `SELECT id FROM payment_methods WHERE tenant_id = $1 AND kind = $2 ORDER BY is_system DESC LIMIT 1`, e.tenant, kind).Scan(&id); err != nil {
		t.Fatal(err)
	}
	return id
}

func (e *env) creditApprover(t *testing.T) *ApprovalIn {
	t.Helper()
	spv := e.person(t, "SpvKredit", `{"credit_limit":["approve"]}`, true)
	if err := e.svc.approvals.SetPin(context.Background(), spv, testPassword, "482915"); err != nil {
		t.Fatal(err)
	}
	return &ApprovalIn{UserID: spv.UserID, PIN: "482915"}
}

func TestCreditSaleCreatesReceivable(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	a := e.scoped()
	it := e.item(t, "goods", "50000", "30000", 10, false)
	m := e.creditMember(t, "0")

	// 2 × 50.000 = 100.000, bayar di muka 20.000 → piutang 80.000; tanpa kembalian; stok tetap berkurang.
	s, _, err := e.svc.Create(ctx, a, key(), creditReq(it, "2", &m, "20000", nil))
	if err != nil {
		t.Fatal(err)
	}
	if s.Total != "100000.00" || s.Paid != "20000.00" || s.Receivable != "80000.00" || s.Change != "0.00" {
		t.Fatalf("nota: %+v", s)
	}
	if s.Credit == nil || s.Credit.Amount != "80000.00" || s.Credit.Balance != "80000.00" || s.Credit.Status != "open" || s.Credit.DueDate == nil {
		t.Fatalf("piutang pada nota: %+v", s.Credit)
	}
	var due string
	if err := e.admin.QueryRow(ctx, `SELECT (r.due_date - (s.created_at AT TIME ZONE o.timezone)::date)::text FROM receivables r JOIN sales s ON s.id = r.sale_id JOIN outlets o ON o.id = s.outlet_id WHERE r.sale_id = $1`, s.ID).Scan(&due); err != nil || due != "30" {
		t.Fatalf("jatuh tempo = %s hari (err %v), want 30", due, err)
	}
	if got := e.stockOf(t, it); got != "8" {
		t.Fatalf("stok = %s, want 8", got)
	}

	// Kredit penuh tanpa DP juga sah.
	s2, _, err := e.svc.Create(ctx, a, key(), creditReq(it, "1", &m, "", nil))
	if err != nil || s2.Paid != "0.00" || s2.Receivable != "50000.00" || len(s2.Payments) != 0 {
		t.Fatalf("kredit penuh: %+v err=%v", s2, err)
	}
	// Quote menampilkan syarat kredit + piutang berjalan, tanpa menyimpan apa pun.
	q, err := e.svc.Quote(ctx, a, Request{Lines: []LineIn{line(it, "1")}, MemberID: &m})
	if err != nil || q.Credit == nil || q.Credit.Outstanding != "130000.00" || q.Credit.Limit != "0.00" || q.Credit.DueDays != 30 {
		t.Fatalf("quote: %+v err=%v", q.Credit, err)
	}
}

func TestCreditRejections(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	a := e.scoped()
	it := e.item(t, "goods", "50000", "30000", 10, false)
	m := e.creditMember(t, "0")

	// Pelanggan umum tidak boleh berkredit.
	_, _, err := e.svc.Create(ctx, a, key(), creditReq(it, "1", nil, "10000", nil))
	fieldErr(t, err, "credit", "MEMBER_REQUIRED")
	// Sudah lunas penuh → tidak ada yang dikreditkan.
	_, _, err = e.svc.Create(ctx, a, key(), creditReq(it, "1", &m, "50000", nil))
	fieldErr(t, err, "credit", "CREDIT_NOT_NEEDED")
	// Member nonaktif tetap ditolak.
	off := e.newMember(t, 0, false)
	_, _, err = e.svc.Create(ctx, a, key(), creditReq(it, "1", &off, "", nil))
	fieldErr(t, err, "member_id", "MEMBER_INACTIVE")
	// Tanpa tanda kredit, bayar kurang tetap ditolak seperti biasa.
	_, _, err = e.svc.Create(ctx, a, key(), Request{Lines: []LineIn{line(it, "1")}, Payments: []PaymentIn{pay("cash", "10000")}, MemberID: &m})
	fieldErr(t, err, "payments", "PAYMENT_SHORT")
	if n := e.count(t, "sales"); n != 0 {
		t.Fatalf("nota tersimpan = %d, want 0", n)
	}
	if n := e.count(t, "receivables"); n != 0 {
		t.Fatalf("piutang tersimpan = %d, want 0", n)
	}
}

func TestCreditLimitNeedsApproval(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	a := e.scoped()
	it := e.item(t, "goods", "50000", "30000", 20, false)
	m := e.creditMember(t, "100000")

	if _, _, err := e.svc.Create(ctx, a, key(), creditReq(it, "1", &m, "", nil)); err != nil { // 50.000 ≤ 100.000
		t.Fatal(err)
	}
	// 50.000 + 60.000 > 100.000 → butuh persetujuan.
	over := creditReq(it, "2", &m, "40000", nil) // piutang 60.000
	_, _, err := e.svc.Create(ctx, a, key(), over)
	var cl *CreditLimitError
	if !errors.As(err, &cl) || cl.Limit.String() != "100000" || cl.Outstanding.String() != "50000" || cl.Receivable.String() != "60000" {
		t.Fatalf("err = %v, want CreditLimitError", err)
	}
	// Penyetuju tanpa izin credit_limit (hanya price_override) ditolak.
	over.Approval = e.approverIn(t)
	if _, _, err := e.svc.Create(ctx, a, key(), over); err == nil || errors.As(err, &cl) {
		t.Fatalf("penyetuju tanpa izin credit_limit harus ditolak (INVALID_PIN), err = %v", err)
	}
	if n := e.count(t, "sales"); n != 1 {
		t.Fatalf("nota = %d, want 1", n)
	}
	// Penyetuju sah → lolos dan tercatat di audit.
	ap := e.creditApprover(t)
	over.Approval = ap
	s, _, err := e.svc.Create(ctx, a, key(), over)
	if err != nil || s.Receivable != "60000.00" {
		t.Fatalf("disetujui: %+v err=%v", s, err)
	}
	var n int
	if err := e.admin.QueryRow(ctx, `SELECT count(*) FROM audit_log WHERE tenant_id = $1 AND action = 'sale.create' AND details->>'credit_approver_id' = $2`, e.tenant, ap.UserID.String()).Scan(&n); err != nil || n != 1 {
		t.Fatalf("audit persetujuan limit = %d (err %v), want 1", n, err)
	}
	// Limit 0 = tanpa batas.
	free := e.creditMember(t, "0")
	if _, _, err := e.svc.Create(ctx, a, key(), creditReq(it, "5", &free, "", nil)); err != nil {
		t.Fatalf("tanpa batas: %v", err)
	}
}

// Delapan kasir berkredit ke member yang sama bersamaan: limit 100.000, tiap nota 30.000 → tepat 3 yang lolos.
func TestCreditLimitHoldsUnderConcurrency(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	a := e.scoped()
	it := e.item(t, "goods", "30000", "10000", 50, false)
	m := e.creditMember(t, "100000")
	var ok, limited atomic.Int32
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _, err := e.svc.Create(ctx, a, key(), creditReq(it, "1", &m, "", nil))
			var cl *CreditLimitError
			switch {
			case err == nil:
				ok.Add(1)
			case errors.As(err, &cl):
				limited.Add(1)
			default:
				t.Errorf("err tak terduga: %v", err)
			}
		}()
	}
	wg.Wait()
	if ok.Load() != 3 || limited.Load() != 5 {
		t.Fatalf("lolos=%d ditolak=%d, want 3/5", ok.Load(), limited.Load())
	}
}

func TestReceivablePayments(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	a := e.scoped()
	it := e.item(t, "goods", "50000", "30000", 10, false)
	m := e.creditMember(t, "0")
	sale, _, err := e.svc.Create(ctx, a, key(), creditReq(it, "2", &m, "20000", nil)) // piutang 80.000
	if err != nil {
		t.Fatal(err)
	}
	rs := receivable.NewService(e.app)
	rid := sale.Credit.ID
	cash := e.methodID(t, "cash")
	pay := func(amount string, k string) (receivable.Detail, bool, error) {
		return rs.Pay(ctx, a, rid, k, receivable.PayInput{MethodID: cash, Amount: jn(amount)})
	}

	// Cicilan.
	k1 := key()
	d, replayed, err := pay("30000", k1)
	if err != nil || replayed || d.Balance != "50000.00" || d.Paid != "30000.00" || d.Status != "open" || len(d.Payments) != 1 {
		t.Fatalf("cicilan 1: %+v replayed=%v err=%v", d, replayed, err)
	}
	// Kunci yang sama → hasil yang sama, tidak mencatat ganda; isi beda → ditolak.
	d2, replayed, err := pay("30000", k1)
	if err != nil || !replayed || d2.Paid != "30000.00" || len(d2.Payments) != 1 {
		t.Fatalf("replay: %+v replayed=%v err=%v", d2, replayed, err)
	}
	if _, _, err := pay("31000", k1); !errors.Is(err, receivable.ErrKeyMismatch) {
		t.Fatalf("kunci sama isi beda: %v", err)
	}
	// Tidak boleh melebihi sisa; jumlah tak valid; metode tak ada.
	_, _, err = pay("50000.01", key())
	var fe receivable.FieldErrors
	if !errors.As(err, &fe) || fe["amount"] != "OVERPAID" {
		t.Fatalf("lebih bayar: %v", err)
	}
	if _, _, err = pay("0", key()); !errors.As(err, &fe) || fe["amount"] != "INVALID" {
		t.Fatalf("nol: %v", err)
	}
	if _, _, err = rs.Pay(ctx, a, rid, key(), receivable.PayInput{MethodID: uuid.New(), Amount: "1000"}); !errors.As(err, &fe) || fe["method_id"] != "INVALID" {
		t.Fatalf("metode asing: %v", err)
	}
	// Nota yang piutangnya sudah dicicil tidak boleh diedit maupun dibatalkan.
	_, ap := e.editor(t)
	_, _, err = e.svc.Edit(ctx, a, sale.ID, key(), editReq(it, "1", "50000", ap))
	if !errors.Is(err, ErrReceivablePaid) {
		t.Fatalf("edit nota yang sudah dicicil: %v", err)
	}
	if _, err = e.svc.Void(ctx, a, sale.ID, VoidInput{Reason: "salah", Approval: ap}); !errors.Is(err, ErrReceivablePaid) {
		t.Fatalf("batal nota yang sudah dicicil: %v", err)
	}
	// Lunas, lalu tidak bisa dibayar lagi.
	if d, _, err = pay("50000", key()); err != nil || d.Balance != "0.00" || d.Status != "paid" {
		t.Fatalf("pelunasan: %+v err=%v", d, err)
	}
	if _, _, err = pay("1000", key()); !errors.As(err, &fe) || fe["amount"] != "SETTLED" {
		t.Fatalf("sudah lunas: %v", err)
	}
	// Daftar: default hanya yang terbuka; status=paid menampilkan yang lunas.
	l, err := rs.List(ctx, a, receivable.ListParams{})
	if err != nil || len(l.Data) != 0 || l.Summary.Outstanding != "0.00" {
		t.Fatalf("daftar terbuka: %+v err=%v", l, err)
	}
	if l, err = rs.List(ctx, a, receivable.ListParams{Status: "paid"}); err != nil || len(l.Data) != 1 || l.Data[0].DocNo != sale.DocNo {
		t.Fatalf("daftar lunas: %+v err=%v", l, err)
	}
	// Piutang dari tenant/outlet lain tidak terlihat.
	other := authz.Actor{TenantID: e.other, UserID: uuid.New(), OutletID: uuid.New(), Outlets: map[uuid.UUID]bool{}}
	if _, err := rs.Get(ctx, other, rid); !errors.Is(err, receivable.ErrNotFound) {
		t.Fatalf("tenant lain: %v", err)
	}
	noOutlet := e.actor(e.tenant) // tanpa akses outlet
	if _, err := rs.Get(ctx, noOutlet, rid); !errors.Is(err, receivable.ErrNotFound) {
		t.Fatalf("tanpa akses outlet: %v", err)
	}
}

// Sepuluh pembayaran 20.000 bersamaan atas sisa 50.000 → tepat 2 yang diterima, sisa 10.000.
func TestReceivablePaymentsNeverOverpayUnderConcurrency(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	a := e.scoped()
	it := e.item(t, "goods", "50000", "30000", 10, false)
	m := e.creditMember(t, "0")
	sale, _, err := e.svc.Create(ctx, a, key(), creditReq(it, "1", &m, "", nil))
	if err != nil {
		t.Fatal(err)
	}
	rs := receivable.NewService(e.app)
	cash := e.methodID(t, "cash")
	var ok atomic.Int32
	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _, err := rs.Pay(ctx, a, sale.Credit.ID, key(), receivable.PayInput{MethodID: cash, Amount: "20000"})
			var fe receivable.FieldErrors
			switch {
			case err == nil:
				ok.Add(1)
			case errors.As(err, &fe):
			default:
				t.Errorf("err tak terduga: %v", err)
			}
		}()
	}
	wg.Wait()
	d, err := rs.Get(ctx, a, sale.Credit.ID)
	if err != nil || ok.Load() != 2 || d.Paid != "40000.00" || d.Balance != "10000.00" {
		t.Fatalf("diterima=%d paid=%s balance=%s err=%v", ok.Load(), d.Paid, d.Balance, err)
	}
}

func TestEditAndVoidCreditSale(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	a := e.scoped()
	it := e.item(t, "goods", "50000", "30000", 10, false)
	m := e.creditMember(t, "100000")
	_, ap := e.editor(t)
	rs := receivable.NewService(e.app)

	orig, _, err := e.svc.Create(ctx, a, key(), creditReq(it, "2", &m, "", nil)) // piutang 100.000 = tepat limit
	if err != nil {
		t.Fatal(err)
	}
	// Edit ke 1 barang: piutang lama tidak dihitung dobel terhadap limit; piutang baru menggantikan.
	edit := EditRequest{Request: creditReq(it, "1", &m, "10000", ap), Reason: "kurangi barang"}
	rev, _, err := e.svc.Edit(ctx, a, orig.ID, key(), edit)
	if err != nil || rev.Receivable != "40000.00" || rev.Credit == nil || rev.Credit.Amount != "40000.00" {
		t.Fatalf("revisi: %+v err=%v", rev, err)
	}
	l, err := rs.List(ctx, a, receivable.ListParams{})
	if err != nil || len(l.Data) != 1 || l.Data[0].SaleID != rev.ID || l.Summary.Outstanding != "40000.00" {
		t.Fatalf("setelah edit: %+v err=%v", l, err)
	}
	// Batal nota kredit yang belum dibayar → piutang hilang dari daftar dan dari total.
	if _, err := e.svc.Void(ctx, a, rev.ID, VoidInput{Reason: "dibatalkan", Approval: ap}); err != nil {
		t.Fatal(err)
	}
	if l, err = rs.List(ctx, a, receivable.ListParams{Status: "all"}); err != nil || len(l.Data) != 0 || l.Summary.Outstanding != "0.00" {
		t.Fatalf("setelah batal: %+v err=%v", l, err)
	}
	if got := e.stockOf(t, it); got != "10" {
		t.Fatalf("stok = %s, want 10", got)
	}
}

func TestReceivableOverdueStatus(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	a := e.scoped()
	it := e.item(t, "goods", "50000", "30000", 10, false)
	m := e.creditMember(t, "0")
	sale, _, err := e.svc.Create(ctx, a, key(), creditReq(it, "1", &m, "", nil))
	if err != nil {
		t.Fatal(err)
	}
	e.exec(t, `UPDATE receivables SET due_date = current_date - 3 WHERE sale_id = $1`, sale.ID)
	rs := receivable.NewService(e.app)
	l, err := rs.List(ctx, a, receivable.ListParams{Status: "overdue"})
	if err != nil || len(l.Data) != 1 || l.Data[0].Status != "overdue" || l.Summary.Overdue != "50000.00" {
		t.Fatalf("lewat tempo: %+v err=%v", l, err)
	}
	if l, err = rs.List(ctx, a, receivable.ListParams{Q: "bu"}); err != nil || len(l.Data) != 1 {
		t.Fatalf("cari member: %+v err=%v", l, err)
	}
	if l, err = rs.List(ctx, a, receivable.ListParams{Q: "zzz"}); err != nil || len(l.Data) != 0 {
		t.Fatalf("cari kosong: %+v err=%v", l, err)
	}
}

// Daftar penjualan harian & semua kasir memisahkan uang yang masuk dari bagian yang dikreditkan.
func TestListsShowReceivableSeparately(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	a := e.scoped()
	it := e.item(t, "goods", "50000", "30000", 10, false)
	m := e.creditMember(t, "0")
	if _, _, err := e.svc.Create(ctx, a, key(), creditReq(it, "2", &m, "20000", nil)); err != nil { // total 100.000, DP 20.000
		t.Fatal(err)
	}
	if _, _, err := e.svc.Create(ctx, a, key(), Request{Lines: []LineIn{line(it, "1")}, Payments: []PaymentIn{pay("cash", "50000")}}); err != nil {
		t.Fatal(err)
	}
	l, err := e.svc.List(ctx, a, "", "", "")
	if err != nil || l.Total != "150000.00" || l.Totals["cash"] != "70000.00" || l.Totals["credit"] != "80000.00" {
		t.Fatalf("hari ini: total=%s totals=%v err=%v", l.Total, l.Totals, err)
	}
	var credit int
	for _, r := range l.Data {
		if r.Receivable == "80000.00" {
			credit++
		}
	}
	if credit != 1 {
		t.Fatalf("baris kredit = %d, want 1", credit)
	}
	all, err := e.svc.ListAll(ctx, a, AllParams{})
	if err != nil || all.Summary.Total != "150000.00" || all.Summary.Receivable != "80000.00" || all.Summary.Methods["cash"] != "70000.00" {
		t.Fatalf("semua kasir: %+v err=%v", all.Summary, err)
	}
}

// RLS: tanpa konteks tenant, role aplikasi tidak melihat satu pun baris piutang; dengan tenant lain pun tidak.
func TestReceivableTablesAreTenantIsolated(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	a := e.scoped()
	it := e.item(t, "goods", "50000", "30000", 10, false)
	m := e.creditMember(t, "0")
	sale, _, err := e.svc.Create(ctx, a, key(), creditReq(it, "1", &m, "", nil))
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := receivable.NewService(e.app).Pay(ctx, a, sale.Credit.ID, key(), receivable.PayInput{MethodID: e.methodID(t, "cash"), Amount: "1000"}); err != nil {
		t.Fatal(err)
	}
	for _, tbl := range []string{"receivables", "receivable_payments", "receivable_payment_counters"} {
		var n int
		if err := e.app.QueryRow(ctx, `SELECT count(*) FROM `+tbl).Scan(&n); err != nil || n != 0 {
			t.Fatalf("%s tanpa tenant: n=%d err=%v, want 0", tbl, n, err)
		}
	}
	// Ledger piutang tidak bisa diubah/dihapus oleh role aplikasi.
	err = db.WithTenant(ctx, e.app, e.tenant, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `UPDATE receivable_payments SET amount = 1 WHERE tenant_id = $1`, e.tenant)
		return err
	})
	if err == nil {
		t.Fatal("UPDATE receivable_payments seharusnya ditolak")
	}
	err = db.WithTenant(ctx, e.app, e.tenant, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `DELETE FROM receivables WHERE tenant_id = $1`, e.tenant)
		return err
	})
	if err == nil {
		t.Fatal("DELETE receivables seharusnya ditolak")
	}
}
