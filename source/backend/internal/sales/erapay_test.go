package sales

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"aciraba/internal/erapay"
	"aciraba/internal/paymentmethod"
)

// fakeEraPay meniru API EraPay: POST /v1/payments (status pending + QR) dan GET /v1/payments/{id} (status bisa dibalik jadi lunas).
type fakeEraPay struct {
	srv     *httptest.Server
	creates atomic.Int32
	mu      sync.Mutex
	status  string
}

func newFakeEraPay(t *testing.T) *fakeEraPay {
	f := &fakeEraPay{status: "pending"}
	f.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer epk_prd_test" {
			http.Error(w, "{}", http.StatusUnauthorized)
			return
		}
		f.mu.Lock()
		st := f.status
		f.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodPost {
			if r.Header.Get("Idempotency-Key") == "" {
				http.Error(w, "{}", http.StatusBadRequest)
				return
			}
			f.creates.Add(1)
			w.WriteHeader(http.StatusCreated)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"id": "pay-" + r.Header.Get("Idempotency-Key") + r.URL.Path[len("/v1/payments"):], "status": st, "qr_code": map[string]string{"type": "provider_defined", "value": "000201QRIS"}})
	}))
	t.Cleanup(f.srv.Close)
	return f
}

func (f *fakeEraPay) set(s string) { f.mu.Lock(); f.status = s; f.mu.Unlock() }

func TestEraPayQRISPayment(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	fake := newFakeEraPay(t)
	era := erapay.NewService(e.app, erapay.NewClient(fake.srv.URL, "epk_prd_test"), paymentmethod.NewService(e.app))
	e.svc.WithPayVerifier(era)
	a := e.item(t, "goods", "1000", "0", 50, false)
	actor := e.actor(e.tenant)

	// Metode EraPay QRIS disiapkan lewat Setup (idempotent).
	cfg, err := era.Setup(ctx, actor)
	if err != nil || !cfg.Enabled || cfg.MethodID == nil {
		t.Fatalf("setup: %+v %v", cfg, err)
	}
	if cfg2, err := era.Setup(ctx, actor); err != nil || *cfg2.MethodID != *cfg.MethodID {
		t.Fatalf("setup ulang: %+v %v", cfg2, err)
	}
	m := *cfg.MethodID
	sale := func(ref string, amount string) error {
		_, _, err := e.svc.Create(ctx, actor, key(), Request{Lines: []LineIn{line(a, "3")}, Payments: []PaymentIn{{MethodID: &m, Amount: json.Number(amount), RefNo: ref}}})
		return err
	}

	// Tanpa charge / ref asal-asalan ditolak: kasir tidak bisa mengklaim QRIS yang tak pernah terjadi.
	fieldErr(t, sale("", "3000"), "payments.0.ref_no", codeEraPayNotPaid)
	fieldErr(t, sale("QRIS-123", "3000"), "payments.0.ref_no", codeEraPayNotPaid)
	fieldErr(t, sale(uuid.NewString(), "3000"), "payments.0.ref_no", codeEraPayNotPaid)

	// Charge: kunci sama = tagihan yang sama (satu panggilan ke EraPay); nominal pecahan / lain ditolak.
	c, err := era.CreateCharge(ctx, actor, "pos-key-0001", decimal.NewFromInt(3000))
	if err != nil || c.Status != "pending" || c.QRValue == "" {
		t.Fatalf("charge: %+v %v", c, err)
	}
	if c2, err := era.CreateCharge(ctx, actor, "pos-key-0001", decimal.NewFromInt(3000)); err != nil || c2.ID != c.ID || fake.creates.Load() != 1 {
		t.Fatalf("idempotensi charge: %+v %v creates=%d", c2, err, fake.creates.Load())
	}
	if _, err := era.CreateCharge(ctx, actor, "pos-key-0001", decimal.NewFromInt(2000)); err != erapay.ErrKeyConflict {
		t.Fatalf("kunci sama nominal beda: %v", err)
	}
	if _, err := era.CreateCharge(ctx, actor, "pos-key-0002", decimal.RequireFromString("1500.5")); err != erapay.ErrAmount {
		t.Fatalf("nominal pecahan: %v", err)
	}

	// Belum dibayar di EraPay -> nota ditolak, tidak ada nota/stok berubah.
	before, stock := e.count(t, "sales"), e.stockOf(t, a)
	fieldErr(t, sale(c.ID.String(), "3000"), "payments.0.ref_no", codeEraPayNotPaid)
	if e.count(t, "sales") != before || e.stockOf(t, a) != stock {
		t.Fatal("nota/stok berubah padahal QRIS belum lunas")
	}

	// Lunas di EraPay tetapi nominal nota beda -> ditolak.
	fake.set("succeeded")
	if got, err := era.GetCharge(ctx, actor, c.ID); err != nil || got.Status != "succeeded" {
		t.Fatalf("poll: %+v %v", got, err)
	}
	// Lunas di EraPay tetapi nominal charge (2.000) beda dengan pembayaran di nota (3.000) -> ditolak.
	c2k, err := era.CreateCharge(ctx, actor, "pos-key-0003", decimal.NewFromInt(2000))
	if err != nil {
		t.Fatal(err)
	}
	fieldErr(t, sale(c2k.ID.String(), "3000"), "payments.0.ref_no", codeEraPayNotPaid)

	// Dua kasir memakai charge lunas yang sama bersamaan: tepat satu nota yang tersimpan.
	var wg sync.WaitGroup
	errs := make([]error, 4)
	for i := range errs {
		wg.Add(1)
		go func() { defer wg.Done(); errs[i] = sale(c.ID.String(), "3000") }()
	}
	wg.Wait()
	ok := 0
	for _, err := range errs {
		if err == nil {
			ok++
		}
	}
	if ok != 1 || e.count(t, "sales") != before+1 {
		t.Fatalf("pemakaian ganda charge: ok=%d errs=%v nota=%d", ok, errs, e.count(t, "sales")-before)
	}
	// Setelah terpakai, charge tidak bisa dipakai nota lain.
	fieldErr(t, sale(c.ID.String(), "3000"), "payments.0.ref_no", codeEraPayNotPaid)

	// Charge milik tenant lain tidak terbaca (RLS/tenant): ref dari tenant lain ditolak.
	other := e.actor(e.other)
	if _, err := era.GetCharge(ctx, other, c.ID); err != erapay.ErrNotFound {
		t.Fatalf("lintas tenant: %v", err)
	}
}
