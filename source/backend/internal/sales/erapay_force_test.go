package sales

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"testing"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"aciraba/internal/approval"
	"aciraba/internal/erapay"
	"aciraba/internal/paymentmethod"
)

// TestEraPayForceSave menguji alur penyimpanan paksa nota QRIS saat EraPay/DOKU tidak dapat dihubungi (webhook tidak jalan):
// flag "perlu dicek EraPay" di transaksi + penyetuju (qris_force) + PIN + audit pemakaian PIN, lalu rekonsiliasi otomatis.
func TestEraPayForceSave(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	fake := newFakeEraPay(t)
	era, err := erapay.NewService(e.app, fake.srv.URL, "uji-rahasia-uji-rahasia-uji-rahasia", true, paymentmethod.NewService(e.app))
	if err != nil {
		t.Fatal(err)
	}
	e.svc.WithPayVerifier(era)
	actor := e.actor(e.tenant)
	cfg, err := era.SaveKey(ctx, actor, keyA)
	if err != nil || cfg.MethodID == nil {
		t.Fatalf("simpan key: %+v %v", cfg, err)
	}
	m := *cfg.MethodID

	approver := e.person(t, "PenyetujuQris", `{"qris_force":["approve"],"price_override":["approve"]}`, true)
	noQris := e.person(t, "PenyetujuHarga", `{"price_override":["approve"]}`, true) // tanpa izin qris_force
	if err := e.svc.approvals.SetPin(ctx, approver, testPassword, "482915"); err != nil {
		t.Fatal(err)
	}
	if err := e.svc.approvals.SetPin(ctx, noQris, testPassword, "135790"); err != nil {
		t.Fatal(err)
	}

	item := e.item(t, "goods", "1000", "400", 50, false)
	// Charge dibuat (pending) — webhook tidak sempat tiba karena penyedia tidak dapat dihubungi.
	c, err := era.CreateCharge(ctx, actor, "pos-force-0001", decimal.NewFromInt(3000))
	if err != nil || c.Status != "pending" {
		t.Fatalf("charge: %+v %v", c, err)
	}

	force := func(item uuid.UUID, ref, qty, amount string, ap *ApprovalIn) (Sale, bool, error) {
		return e.svc.Create(ctx, actor, key(), Request{Lines: []LineIn{line(item, qty)},
			Payments: []PaymentIn{{MethodID: &m, Amount: json.Number(amount), RefNo: ref}}, ForceEraPay: true, Approval: ap})
	}

	// (a) Tanpa force, charge pending → ditolak (perilaku lama bertahan).
	_, _, err = e.svc.Create(ctx, actor, key(), Request{Lines: []LineIn{line(item, "3")},
		Payments: []PaymentIn{{MethodID: &m, Amount: "3000", RefNo: c.ID.String()}}})
	fieldErr(t, err, "payments.0.ref_no", codeEraPayNotPaid)

	// (b) Force tanpa PIN → wajib PIN penyetuju.
	if _, _, err = force(item, c.ID.String(), "3", "3000", nil); !errors.Is(err, approval.ErrPinRequired) {
		t.Fatalf("force tanpa PIN: %v", err)
	}

	// (c) Force dengan PIN penyetuju yang TIDAK berizin qris_force → ditolak.
	if _, _, err = force(item, c.ID.String(), "3", "3000", &ApprovalIn{UserID: noQris.UserID, PIN: "135790"}); !errors.Is(err, approval.ErrInvalidPin) {
		t.Fatalf("force PIN tanpa izin qris_force: %v", err)
	}

	// (d) Force dengan PIN penyetuju berizin qris_force → nota tersimpan berflag "perlu dicek EraPay", stok berkurang,
	//     charge ter-link ke nota, dan pemakaian PIN dicatat di AUDIT LOG beserta tujuannya.
	before := e.count(t, "sales")
	stockNow := e.stockOf(t, item)
	ap := &ApprovalIn{UserID: approver.UserID, PIN: "482915"}
	s, _, err := force(item, c.ID.String(), "3", "3000", ap)
	if err != nil {
		t.Fatalf("force simpan: %v", err)
	}
	if !s.EraPayPending {
		t.Fatal("nota harus berflag erapay_pending")
	}
	if e.count(t, "sales") != before+1 || e.stockOf(t, item) != decimal.RequireFromString(stockNow).Sub(decimal.NewFromInt(3)).String() {
		t.Fatalf("stok/nota tidak konsisten: nota=%d stok=%s", e.count(t, "sales")-before, e.stockOf(t, item))
	}
	var saleID *uuid.UUID
	_ = saleID
	var linked uuid.UUID
	if err := e.admin.QueryRow(ctx, `SELECT sale_id FROM erapay_charges WHERE id = $1 AND tenant_id = $2`, c.ID, e.tenant).Scan(&linked); err != nil || linked != s.ID {
		t.Fatalf("charge tidak ter-link: %v %v", linked, err)
	}
	var n int
	if err := e.admin.QueryRow(ctx, `SELECT count(*) FROM audit_log WHERE tenant_id = $1 AND action = 'sale.erapay_force' AND entity_id = $2`, e.tenant, s.ID.String()).Scan(&n); err != nil || n != 1 {
		t.Fatalf("audit sale.erapay_force: %d %v", n, err)
	}

	// (e) Rekonsiliasi: charge ternyata lunas → penanda dihapus (tidak ada void).
	fake.set("succeeded")
	r := NewEraPayReconciler(e.svc, e.app, slog.New(slog.NewTextHandler(io.Discard, nil)))
	r.OnlyTenant = e.tenant
	if got := r.Tick(ctx); got != 1 {
		t.Fatalf("rekonsiliasi lunas: %d", got)
	}
	re, err := e.svc.Get(ctx, actor, s.ID)
	if err != nil || re.EraPayPending {
		t.Fatalf("penanda harus sudah dihapus: %+v %v", re, err)
	}
	if err := e.admin.QueryRow(ctx, `SELECT count(*) FROM audit_log WHERE tenant_id = $1 AND action = 'sale.erapay_paid' AND entity_id = $2`, e.tenant, s.ID.String()).Scan(&n); err != nil || n != 1 {
		t.Fatalf("audit erapay_paid: %d %v", n, err)
	}

	// (f) Force-save kedua; EraPay menyatakan tidak lunas → nota otomatis VOID + stok kembali + audit.
	c2, err := era.CreateCharge(ctx, actor, "pos-force-0002", decimal.NewFromInt(2000))
	if err != nil {
		t.Fatal(err)
	}
	stock2 := e.stockOf(t, item)
	s2, _, err := force(item, c2.ID.String(), "2", "2000", ap)
	if err != nil {
		t.Fatalf("force simpan kedua: %v", err)
	}
	if !s2.EraPayPending {
		t.Fatal("nota kedua harus berflag erapay_pending")
	}
	fake.set("failed")
	r2 := NewEraPayReconciler(e.svc, e.app, slog.New(slog.NewTextHandler(io.Discard, nil)))
	r2.OnlyTenant = e.tenant
	if got := r2.Tick(ctx); got != 1 {
		t.Fatalf("rekonsiliasi gagal: %d", got)
	}
	v, err := e.svc.Get(ctx, actor, s2.ID)
	if err != nil || v.Status != "void" || v.EraPayPending {
		t.Fatalf("nota kedua harus void tanpa flag: %+v %v", v, err)
	}
	if e.stockOf(t, item) != stock2 {
		t.Fatalf("stok tidak dikembalikan setelah void: %s (ingin %s)", e.stockOf(t, item), stock2)
	}
	if err := e.admin.QueryRow(ctx, `SELECT count(*) FROM audit_log WHERE tenant_id = $1 AND action IN ('sale.erapay_unpaid','sale.void') AND entity_id = $2`, e.tenant, s2.ID.String()).Scan(&n); err != nil || n < 2 {
		t.Fatalf("audit unpaid/void: %d %v", n, err)
	}
}
