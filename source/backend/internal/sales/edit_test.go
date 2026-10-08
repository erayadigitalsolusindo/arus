package sales

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"

	"github.com/google/uuid"

	"aciraba/internal/approval"
	"aciraba/internal/authz"
)

const editPin = "482915"

// editor membuat penyetuju (izin sale_edit + price_override, akses outlet uji) lengkap dengan PIN.
func (e *env) editor(t *testing.T) (authz.Actor, *ApprovalIn) {
	t.Helper()
	spv := e.person(t, "SpvEdit", `{"sale_edit":["approve"],"price_override":["approve"]}`, true)
	if err := e.svc.approvals.SetPin(context.Background(), spv, testPassword, editPin); err != nil {
		t.Fatal(err)
	}
	return spv, &ApprovalIn{UserID: spv.UserID, PIN: editPin}
}

func (e *env) saleStatus(t *testing.T, id uuid.UUID) string {
	t.Helper()
	var s string
	if err := e.admin.QueryRow(context.Background(), `SELECT status FROM sales WHERE id = $1`, id).Scan(&s); err != nil {
		t.Fatal(err)
	}
	return s
}

func (e *env) movements(t *testing.T, item uuid.UUID) (n int, last string) {
	t.Helper()
	err := e.admin.QueryRow(context.Background(), `SELECT count(*), coalesce((SELECT balance_after::text FROM stock_movements WHERE tenant_id=$1 AND item_id=$2 ORDER BY id DESC LIMIT 1), '')
		FROM stock_movements WHERE tenant_id=$1 AND item_id=$2`, e.tenant, item).Scan(&n, &last)
	if err != nil {
		t.Fatal(err)
	}
	return
}

func editReq(item uuid.UUID, qty, cash string, ap *ApprovalIn) EditRequest {
	return EditRequest{Request: Request{Lines: []LineIn{line(item, qty)}, Payments: []PaymentIn{pay("cash", cash)}, Approval: ap}, Reason: "salah input qty"}
}

// Edit = revisi baru; stok akhir tepat, ledger append-only (jual 5 → balik 5 → jual 3), nota lama utuh.
func TestEditCreatesRevisionAndKeepsStockExact(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	it := e.item(t, "goods", "10000", "5000", 10, false)
	_, ap := e.editor(t)
	a := e.actor(e.tenant)
	a.Outlets = map[uuid.UUID]bool{e.outlet: true}

	orig, _, err := e.svc.Create(ctx, a, key(), Request{Lines: []LineIn{line(it, "5")}, Payments: []PaymentIn{pay("cash", "50000")}})
	if err != nil {
		t.Fatal(err)
	}
	if e.stockOf(t, it) != "5" {
		t.Fatalf("stok awal nota: %s", e.stockOf(t, it))
	}
	rev, replayed, err := e.svc.Edit(ctx, a, orig.ID, key(), editReq(it, "3", "30000", ap))
	if err != nil || replayed {
		t.Fatalf("edit: %v", err)
	}
	if rev.DocNo != orig.DocNo+"-R2" || rev.Revision != 2 || rev.Status != "completed" || rev.SupersedesID == nil || *rev.SupersedesID != orig.ID || rev.RootID != orig.ID ||
		rev.Total != "30000.00" || rev.RevisionReason != "salah input qty" || rev.CashierName != orig.CashierName || !rev.CreatedAt.Equal(orig.CreatedAt) {
		t.Fatalf("revisi: %+v", rev)
	}
	if e.stockOf(t, it) != "7" {
		t.Fatalf("stok sesudah edit harus 7, dapat %s", e.stockOf(t, it))
	}
	if e.saleStatus(t, orig.ID) != "superseded" {
		t.Fatal("nota lama harus superseded")
	}
	// Ledger: OPENING, SALE -5, SALE_VOID +5, SALE -3; saldo berjalan konsisten.
	if n, last := e.movements(t, it); n != 4 || last != "7.000" {
		t.Fatalf("ledger: %d movement, saldo akhir %s", n, last)
	}
	var kinds string
	_ = e.admin.QueryRow(ctx, `SELECT string_agg(ref_type || ':' || qty_delta::text, ',' ORDER BY id) FROM stock_movements WHERE tenant_id=$1 AND item_id=$2`, e.tenant, it).Scan(&kinds)
	if kinds != "OPENING:10.000,SALE:-5.000,SALE_VOID:5.000,SALE:-3.000" {
		t.Fatalf("urutan ledger: %s", kinds)
	}
	// Nota lama tidak berubah (baris tetap 5), hanya satu versi aktif di daftar.
	if old, err := e.svc.Get(ctx, a, orig.ID); err != nil || old.Status != "superseded" || old.Lines[0].Qty != "5" || old.SupersededBy == nil || *old.SupersededBy != rev.ID {
		t.Fatalf("nota lama: %+v %v", old, err)
	}
	if res, err := e.svc.ListAll(ctx, a, AllParams{}); err != nil || len(res.Data) != 1 || res.Data[0].ID != rev.ID || res.Summary.Count != 1 || res.Summary.Total != "30000.00" {
		t.Fatalf("daftar harus hanya versi aktif: %+v %v", res, err)
	}
	// Audit di kedua nota.
	var edits, supers int
	_ = e.admin.QueryRow(ctx, `SELECT count(*) FILTER (WHERE action='sale.edit' AND entity_id=$2), count(*) FILTER (WHERE action='sale.superseded' AND entity_id=$3) FROM audit_log WHERE tenant_id=$1`,
		e.tenant, rev.ID.String(), orig.ID.String()).Scan(&edits, &supers)
	if edits != 1 || supers != 1 {
		t.Fatalf("audit: edit=%d superseded=%d", edits, supers)
	}
	// Detail revisi memuat rantai + pembalikan stok nota lama.
	d, err := e.svc.Detail(ctx, a, rev.ID)
	if err != nil || len(d.Revisions) != 2 || len(d.Stock) != 3 {
		t.Fatalf("detail: revisions=%d stock=%d %v", len(d.Revisions), len(d.Stock), err)
	}

	// Edit lagi: penanda revisi diganti (-R3), tidak bertumpuk.
	rev3, _, err := e.svc.Edit(ctx, a, rev.ID, key(), editReq(it, "4", "40000", ap))
	if err != nil || rev3.DocNo != orig.DocNo+"-R3" || rev3.RootID != orig.ID || e.stockOf(t, it) != "6" {
		t.Fatalf("revisi 3: %+v %v stok=%s", rev3, err, e.stockOf(t, it))
	}
	// Versi lama tidak bisa diedit lagi.
	if _, _, err := e.svc.Edit(ctx, a, orig.ID, key(), editReq(it, "1", "10000", ap)); !errors.Is(err, ErrNotEditable) {
		t.Fatalf("edit nota superseded: %v", err)
	}
}

// Harga & HPP baris yang sudah ada dipertahankan walau master berubah; baris baru memakai harga sekarang.
func TestEditKeepsOriginalPricesAndCost(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	it := e.item(t, "goods", "10000", "5000", 20, false)
	other := e.item(t, "goods", "7000", "3000", 20, false)
	_, ap := e.editor(t)
	a := e.actor(e.tenant)
	a.Outlets = map[uuid.UUID]bool{e.outlet: true}
	orig, _, err := e.svc.Create(ctx, a, key(), Request{Lines: []LineIn{line(it, "2")}, Payments: []PaymentIn{pay("cash", "20000")}})
	if err != nil {
		t.Fatal(err)
	}
	e.exec(t, `UPDATE items SET sell_price = 15000, avg_cost = 9000 WHERE id = $1`, it)   // master naik setelah transaksi
	e.exec(t, `UPDATE items SET sell_price = 8000, avg_cost = 4000 WHERE id = $1`, other) // barang baru: harga sekarang

	req := EditRequest{Request: Request{Lines: []LineIn{line(it, "3"), line(other, "1")}, Payments: []PaymentIn{pay("cash", "38000")}, Approval: ap}, Reason: "tambah barang"}
	rev, _, err := e.svc.Edit(ctx, a, orig.ID, key(), req)
	if err != nil {
		t.Fatal(err)
	}
	d, err := e.svc.Detail(ctx, withCostPerm(a), rev.ID)
	if err != nil || len(d.Lines) != 2 {
		t.Fatalf("detail: %v", err)
	}
	// Baris lama: harga 10.000 & HPP 5.000 (bukan 15.000/9.000). Baris baru: 8.000 & HPP 4.000.
	if d.Lines[0].UnitPrice != "10000.00" || *d.Lines[0].UnitCost != "5000.00" || d.Lines[0].LineTotal != "30000.00" {
		t.Fatalf("baris lama harus mempertahankan harga/HPP: %+v", d.Lines[0])
	}
	if d.Lines[1].UnitPrice != "8000.00" || *d.Lines[1].UnitCost != "4000.00" || rev.Total != "38000.00" {
		t.Fatalf("baris baru memakai harga sekarang: %+v total=%s", d.Lines[1], rev.Total)
	}
}

func withCostPerm(a authz.Actor) authz.Actor {
	a.Perms = authz.Permissions{Grants: map[string][]string{"sales_cost": {"view"}}}
	return a
}

// Stok revisi kurang → seluruh edit dibatalkan: nota lama utuh, stok & ledger tidak berubah.
func TestEditRollsBackWhenStockInsufficient(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	it := e.item(t, "goods", "10000", "5000", 10, false)
	_, ap := e.editor(t)
	a := e.actor(e.tenant)
	a.Outlets = map[uuid.UUID]bool{e.outlet: true}
	orig, _, err := e.svc.Create(ctx, a, key(), Request{Lines: []LineIn{line(it, "4")}, Payments: []PaymentIn{pay("cash", "40000")}})
	if err != nil {
		t.Fatal(err)
	}
	nBefore, _ := e.movements(t, it)

	_, _, err = e.svc.Edit(ctx, a, orig.ID, key(), editReq(it, "11", "110000", ap)) // stok total hanya 10
	var se *StockError
	if !errors.As(err, &se) {
		t.Fatalf("harus STOCK_INSUFFICIENT: %v", err)
	}
	if e.saleStatus(t, orig.ID) != "completed" || e.stockOf(t, it) != "6" {
		t.Fatalf("nota lama harus utuh: status=%s stok=%s", e.saleStatus(t, orig.ID), e.stockOf(t, it))
	}
	if n, _ := e.movements(t, it); n != nBefore {
		t.Fatalf("ledger tidak boleh berubah saat edit gagal: %d → %d", nBefore, n)
	}
	if e.count(t, "sales") != 1 {
		t.Fatal("tidak boleh ada nota revisi yang tertinggal")
	}
	// Edit yang menambah qty tapi masih cukup stok (dihitung dengan stok yang dikembalikan) berhasil: 4 → 10.
	if _, _, err := e.svc.Edit(ctx, a, orig.ID, key(), editReq(it, "10", "100000", ap)); err != nil || e.stockOf(t, it) != "0" {
		t.Fatalf("edit ke seluruh stok: %v stok=%s", err, e.stockOf(t, it))
	}
}

// Batal: stok kembali, kupon dikembalikan, tidak bisa dibatalkan dua kali, tidak masuk jumlah uang laci kasir.
func TestVoidReversesStockVoucherAndIsFinal(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	it := e.item(t, "goods", "100000", "30000", 10, false)
	vid := e.newVoucher(t, "HEMAT10", "percent", "10", "15000", "0", "NULL", "NULL", nil, true)
	_, ap := e.editor(t)
	a := e.actor(e.tenant)
	a.Outlets = map[uuid.UUID]bool{e.outlet: true}
	s, _, err := e.svc.Create(ctx, a, key(), Request{Lines: []LineIn{line(it, "2")}, VoucherCodes: []string{"HEMAT10"}, Payments: []PaymentIn{pay("cash", "200000")}})
	if err != nil {
		t.Fatal(err)
	}
	if e.voucherUsed(t, vid) != 1 || e.stockOf(t, it) != "8" {
		t.Fatal("prasyarat")
	}
	v, err := e.svc.Void(ctx, a, s.ID, VoidInput{Reason: "pelanggan batal", Approval: ap})
	if err != nil || v.Status != "void" || v.VoidReason != "pelanggan batal" {
		t.Fatalf("void: %+v %v", v, err)
	}
	if e.stockOf(t, it) != "10" || e.voucherUsed(t, vid) != 0 {
		t.Fatalf("dampak harus dibalik: stok=%s kupon=%d", e.stockOf(t, it), e.voucherUsed(t, vid))
	}
	if _, err := e.svc.Void(ctx, a, s.ID, VoidInput{Reason: "dua kali", Approval: ap}); !errors.Is(err, ErrNotEditable) {
		t.Fatalf("void kedua: %v", err)
	}
	if _, _, err := e.svc.Edit(ctx, a, s.ID, key(), editReq(it, "1", "100000", ap)); !errors.Is(err, ErrNotEditable) {
		t.Fatalf("edit nota batal: %v", err)
	}
	// Daftar kasir: nota batal tetap tampil tapi tidak masuk total uang.
	if l, err := e.svc.List(ctx, a, "", "", ""); err != nil || len(l.Data) != 1 || l.Data[0].Status != "void" || l.Total != "0.00" || len(l.Totals) != 0 {
		t.Fatalf("daftar kasir: %+v %v", l, err)
	}
	if all, err := e.svc.ListAll(ctx, a, AllParams{}); err != nil || all.Summary.Count != 1 || all.Summary.CompletedCount != 0 || all.Summary.Total != "0.00" {
		t.Fatalf("ringkasan: %+v %v", all.Summary, err)
	}
	// Kupon yang sama bisa dipakai lagi (kuota kembali).
	if _, _, err := e.svc.Create(ctx, a, key(), Request{Lines: []LineIn{line(it, "1")}, VoucherCodes: []string{"HEMAT10"}, Payments: []PaymentIn{pay("cash", "100000")}}); err != nil {
		t.Fatal(err)
	}
}

// Persetujuan, alasan, batas hari edit, dan batas outlet/tenant.
func TestEditGuards(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	it := e.item(t, "goods", "10000", "5000", 20, false)
	_, ap := e.editor(t)
	plain := e.person(t, "Biasa", `{"sales_orders":["update"]}`, true) // bukan penyetuju
	a := e.actor(e.tenant)
	a.Outlets = map[uuid.UUID]bool{e.outlet: true}
	s, _, err := e.svc.Create(ctx, a, key(), Request{Lines: []LineIn{line(it, "2")}, Payments: []PaymentIn{pay("cash", "20000")}})
	if err != nil {
		t.Fatal(err)
	}
	do := func(act authz.Actor, r EditRequest) error {
		_, _, err := e.svc.Edit(ctx, act, s.ID, key(), r)
		return err
	}

	if err := do(a, editReq(it, "1", "10000", nil)); !errors.Is(err, approval.ErrPinRequired) {
		t.Fatalf("tanpa persetujuan: %v", err)
	}
	if err := do(a, editReq(it, "1", "10000", &ApprovalIn{UserID: ap.UserID, PIN: "999000"})); !errors.Is(err, approval.ErrInvalidPin) {
		t.Fatalf("PIN salah: %v", err)
	}
	// Penyetuju ubah harga TIDAK otomatis boleh menyetujui edit nota (izin sale_edit terpisah).
	onlyPrice := e.person(t, "Spv2", `{"price_override":["approve"]}`, true)
	if err := e.svc.approvals.SetPin(ctx, onlyPrice, testPassword, "135792"); err != nil {
		t.Fatal(err)
	}
	if err := do(a, editReq(it, "1", "10000", &ApprovalIn{UserID: onlyPrice.UserID, PIN: "135792"})); !errors.Is(err, approval.ErrInvalidPin) {
		t.Fatalf("penyetuju ubah harga bukan penyetuju edit: %v", err)
	}
	if err := do(a, editReq(it, "1", "10000", &ApprovalIn{UserID: plain.UserID, PIN: editPin})); !errors.Is(err, approval.ErrInvalidPin) {
		t.Fatalf("bukan penyetuju edit: %v", err)
	}
	bad := editReq(it, "1", "10000", ap)
	bad.Reason = " "
	fieldErr(t, do(a, bad), "reason", "REQUIRED")
	bad.Reason = "ab"
	fieldErr(t, do(a, bad), "reason", "INVALID")
	if e.saleStatus(t, s.ID) != "completed" || e.stockOf(t, it) != "18" {
		t.Fatal("penolakan tidak boleh mengubah apa pun")
	}

	// Outlet: edit harus dari outlet nota; outlet di luar akses = tidak ditemukan; tenant lain = tidak ditemukan.
	otherOutlet := uuid.New()
	e.exec(t, `INSERT INTO outlets (id, tenant_id, code, name) VALUES ($1, $2, 'cab2', 'Cabang Dua')`, otherOutlet, e.tenant)
	away := a
	away.OutletID = otherOutlet
	away.Outlets = map[uuid.UUID]bool{e.outlet: true, otherOutlet: true}
	if err := do(away, editReq(it, "1", "10000", ap)); !errors.Is(err, ErrOutletMismatch) {
		t.Fatalf("outlet lain: %v", err)
	}
	blind := a
	blind.OutletID = otherOutlet
	blind.Outlets = map[uuid.UUID]bool{otherOutlet: true}
	if err := do(blind, editReq(it, "1", "10000", ap)); !errors.Is(err, ErrNotFound) {
		t.Fatalf("tanpa akses outlet: %v", err)
	}
	if err := do(e.actor(e.other), editReq(it, "1", "10000", ap)); !errors.Is(err, ErrNotFound) {
		t.Fatalf("tenant lain: %v", err)
	}

	// Batas hari: bawaan 0 (hanya hari nota). Nota 2 hari lalu → ditolak; tenant diberi 5 hari → boleh.
	e.exec(t, `UPDATE sales SET created_at = created_at - interval '2 days' WHERE id = $1`, s.ID)
	if err := do(a, editReq(it, "1", "10000", ap)); !errors.Is(err, ErrEditWindow) {
		t.Fatalf("lewat batas hari: %v", err)
	}
	if _, err := e.svc.Void(ctx, a, s.ID, VoidInput{Reason: "terlambat", Approval: ap}); !errors.Is(err, ErrEditWindow) {
		t.Fatalf("void lewat batas hari: %v", err)
	}
	e.exec(t, `UPDATE tenants SET sale_edit_window_days = 5 WHERE id = $1`, e.tenant)
	if err := do(a, editReq(it, "1", "10000", ap)); err != nil {
		t.Fatalf("dalam batas 5 hari: %v", err)
	}
	if !strings.HasSuffix(mustDoc(t, e, s.ID), "-R2") && e.count(t, "sales") != 2 {
		t.Fatal("edit dalam batas hari harus membuat revisi")
	}
}

func mustDoc(t *testing.T, e *env, id uuid.UUID) string {
	t.Helper()
	var d string
	if err := e.admin.QueryRow(context.Background(), `SELECT doc_no FROM sales WHERE id = $1`, id).Scan(&d); err != nil {
		t.Fatal(err)
	}
	return d
}

// Dua edit/batal bersamaan pada nota yang sama: tepat satu menang, stok tetap benar, tidak ada nota ganda.
func TestConcurrentEditsOnSameSale(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	it := e.item(t, "goods", "10000", "5000", 20, false)
	_, ap := e.editor(t)
	a := e.actor(e.tenant)
	a.Outlets = map[uuid.UUID]bool{e.outlet: true}
	s, _, err := e.svc.Create(ctx, a, key(), Request{Lines: []LineIn{line(it, "5")}, Payments: []PaymentIn{pay("cash", "50000")}})
	if err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	results := make([]error, 8)
	for i := range results {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if i%2 == 0 {
				_, _, results[i] = e.svc.Edit(ctx, a, s.ID, key(), editReq(it, "2", "20000", ap))
			} else {
				_, results[i] = e.svc.Void(ctx, a, s.ID, VoidInput{Reason: "bersamaan", Approval: ap})
			}
		}()
	}
	wg.Wait()
	ok := 0
	for _, r := range results {
		switch {
		case r == nil:
			ok++
		case !errors.Is(r, ErrNotEditable):
			t.Fatalf("galat tak terduga: %v", r)
		}
	}
	if ok != 1 {
		t.Fatalf("tepat satu yang boleh menang, dapat %d", ok)
	}
	// Stok: batal → 20; edit → 18. Apa pun pemenangnya, stok sesuai dan tidak ada movement ganda.
	got := e.stockOf(t, it)
	if got != "20" && got != "18" {
		t.Fatalf("stok akhir tidak masuk akal: %s", got)
	}
	if e.count(t, "sales") > 2 {
		t.Fatalf("nota ganda: %d", e.count(t, "sales"))
	}
}

// Pratinjau edit = hasil yang sama dengan edit sungguhan, dan tidak menyimpan apa pun (stok, kupon, nota tetap).
func TestQuoteEditMatchesEditAndPersistsNothing(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	it := e.item(t, "goods", "100000", "30000", 10, false)
	vid := e.newVoucher(t, "HEMAT10", "percent", "10", "15000", "0", "NULL", "NULL", nil, true)
	_, ap := e.editor(t)
	a := e.actor(e.tenant)
	a.Outlets = map[uuid.UUID]bool{e.outlet: true}
	orig, _, err := e.svc.Create(ctx, a, key(), Request{Lines: []LineIn{line(it, "4")}, VoucherCodes: []string{"HEMAT10"}, Payments: []PaymentIn{pay("cash", "400000")}})
	if err != nil {
		t.Fatal(err)
	}
	e.exec(t, `UPDATE items SET sell_price = 120000 WHERE id = $1`, it) // harga master naik: baris lama tetap 100.000

	req := Request{Lines: []LineIn{line(it, "10")}, VoucherCodes: []string{"HEMAT10"}} // total stok 10 hanya cukup bila stok nota lama kembali
	q, err := e.svc.QuoteEdit(ctx, a, orig.ID, req)
	if err != nil || q.Subtotal != "1000000.00" || q.VoucherAmount != "15000.00" || q.Lines[0].UnitPrice != "100000.00" || q.Lines[0].Issue != "" {
		t.Fatalf("quote edit: %+v %v", q, err)
	}
	if e.stockOf(t, it) != "6" || e.voucherUsed(t, vid) != 1 || e.saleStatus(t, orig.ID) != "completed" || e.count(t, "sales") != 1 {
		t.Fatalf("pratinjau tidak boleh menyimpan apa pun: stok=%s kupon=%d", e.stockOf(t, it), e.voucherUsed(t, vid))
	}
	req.Payments, req.Approval = []PaymentIn{pay("cash", q.Total)}, ap
	rev, _, err := e.svc.Edit(ctx, a, orig.ID, key(), EditRequest{Request: req, Reason: "tambah qty"})
	if err != nil || rev.Total != q.Total || rev.Discount != q.Discount {
		t.Fatalf("edit harus sama dengan pratinjau: %+v vs %+v %v", rev, q, err)
	}
	// Nota yang sudah digantikan tidak bisa dipratinjau lagi; batas hari juga berlaku di pratinjau.
	if _, err := e.svc.QuoteEdit(ctx, a, orig.ID, req); !errors.Is(err, ErrNotEditable) {
		t.Fatalf("quote nota superseded: %v", err)
	}
}
