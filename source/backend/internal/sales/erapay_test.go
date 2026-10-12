package sales

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"aciraba/internal/erapay"
	"aciraba/internal/paymentmethod"
)

// Key uji berformat sah (epk_<env>_<8 hex>_<64 hex>); fake EraPay hanya mengenali key yang didaftarkan.
const (
	keyA    = "epk_prd_aaaaaaaa_" + "0000000000000000000000000000000000000000000000000000000000000001"
	keyB    = "epk_prd_bbbbbbbb_" + "0000000000000000000000000000000000000000000000000000000000000002"
	keySbx  = "epk_sbx_cccccccc_" + "0000000000000000000000000000000000000000000000000000000000000003"
	keyNoSc = "epk_prd_dddddddd_" + "0000000000000000000000000000000000000000000000000000000000000004"
	keyBad  = "epk_prd_eeeeeeee_" + "0000000000000000000000000000000000000000000000000000000000000005"
)

// fakeEraPay meniru API EraPay: GET /v1/api-keys/self, POST /v1/payments (pending + QR), GET /v1/payments/{id} (status bisa dibalik jadi lunas).
type fakeEraPay struct {
	srv     *httptest.Server
	creates atomic.Int32
	mu      sync.Mutex
	status  string
	usedBy  map[string]string // id pembayaran -> key pembuat
	keys    map[string]struct {
		env    string
		scopes []string
	}
}

func newFakeEraPay(t *testing.T) *fakeEraPay {
	f := &fakeEraPay{status: "pending", usedBy: map[string]string{}, keys: map[string]struct {
		env    string
		scopes []string
	}{
		keyA:    {"production", []string{"payment:create", "payment:read"}},
		keyB:    {"production", []string{"payment:create", "payment:read"}},
		keySbx:  {"sandbox", []string{"payment:create", "payment:read"}},
		keyNoSc: {"production", []string{"payment:read"}},
	}}
	f.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		key := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
		info, ok := f.keys[key]
		if !ok {
			http.Error(w, "{}", http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/v1/api-keys/self" {
			_ = json.NewEncoder(w).Encode(map[string]any{"environment": info.env, "scopes": info.scopes})
			return
		}
		f.mu.Lock()
		st := f.status
		f.mu.Unlock()
		id := "pay-" + r.Header.Get("Idempotency-Key")
		if r.Method == http.MethodGet {
			id = strings.TrimPrefix(r.URL.Path, "/v1/payments/")
		}
		if r.Method == http.MethodPost {
			if r.Header.Get("Idempotency-Key") == "" {
				http.Error(w, "{}", http.StatusBadRequest)
				return
			}
			f.creates.Add(1)
			f.mu.Lock()
			f.usedBy[id] = key
			f.mu.Unlock()
			w.WriteHeader(http.StatusCreated)
		} else {
			f.mu.Lock()
			owner := f.usedBy[id]
			f.mu.Unlock()
			if owner != key { // pembayaran milik merchant lain tidak terlihat
				http.Error(w, "{}", http.StatusNotFound)
				return
			}
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"id": id, "status": st, "qr_code": map[string]string{"type": "provider_defined", "value": "000201QRIS"}})
	}))
	t.Cleanup(f.srv.Close)
	return f
}

func (f *fakeEraPay) set(s string) { f.mu.Lock(); f.status = s; f.mu.Unlock() }

func TestEraPayQRISPayment(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	fake := newFakeEraPay(t)
	era, err := erapay.NewService(e.app, fake.srv.URL, "uji-rahasia-uji-rahasia-uji-rahasia", true, paymentmethod.NewService(e.app))
	if err != nil {
		t.Fatal(err)
	}
	e.svc.WithPayVerifier(era)
	a := e.item(t, "goods", "1000", "0", 50, false)
	actor := e.actor(e.tenant)

	// Sebelum key dipasang: tidak bisa membuat QR.
	if _, err := era.CreateCharge(ctx, actor, "pos-key-0000", decimal.NewFromInt(1000)); err != erapay.ErrNotConfigured {
		t.Fatalf("tanpa key: %v", err)
	}
	// Key ditolak: format salah, tidak dikenal EraPay, tanpa izin payment:create, key sandbox di server production.
	for key, want := range map[string]error{"abc": erapay.ErrBadKey, keyBad: erapay.ErrKeyInvalid, keyNoSc: erapay.ErrKeyScope, keySbx: erapay.ErrKeyEnvironment} {
		if _, err := era.SaveKey(ctx, actor, key); err != want {
			t.Fatalf("key %.14s: dapat %v ingin %v", key, err, want)
		}
	}
	// Key sah: tersimpan terenkripsi (tidak ada teks key di DB), metode "EraPay QRIS" otomatis siap; simpan ulang = ganti key.
	cfg, err := era.SaveKey(ctx, actor, keyA)
	if err != nil || !cfg.Enabled || cfg.MethodID == nil || cfg.KeyHint != keyA[len(keyA)-4:] || cfg.Environment != "production" {
		t.Fatalf("simpan key: %+v %v", cfg, err)
	}
	var enc string
	if err := e.admin.QueryRow(ctx, `SELECT api_key_enc FROM erapay_settings WHERE tenant_id = $1`, e.tenant).Scan(&enc); err != nil || strings.Contains(enc, "aaaaaaaa") || strings.Contains(enc, keyA[len(keyA)-12:]) {
		t.Fatalf("key tidak terenkripsi: %q %v", enc, err)
	}
	if cfg2, err := era.SaveKey(ctx, actor, keyA); err != nil || *cfg2.MethodID != *cfg.MethodID {
		t.Fatalf("simpan ulang: %+v %v", cfg2, err)
	}
	// Tenant lain belum punya key: konfigurasinya tidak ikut aktif, dan key tenant lain tidak dipakai.
	other := e.actor(e.other)
	if cfgO, err := era.Config(ctx, other); err != nil || cfgO.Configured || cfgO.Enabled {
		t.Fatalf("tenant lain: %+v %v", cfgO, err)
	}
	if _, err := era.CreateCharge(ctx, other, "pos-key-other1", decimal.NewFromInt(1000)); err != erapay.ErrNotConfigured {
		t.Fatalf("tenant lain tanpa key: %v", err)
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
	if _, err := era.GetCharge(ctx, other, c.ID); err != erapay.ErrNotFound {
		t.Fatalf("lintas tenant: %v", err)
	}

	// Tenant lain memasang key-nya sendiri: tagihannya dibuat dengan key itu, bukan key tenant pertama.
	if _, err := era.SaveKey(ctx, other, keyB); err != nil {
		t.Fatal(err)
	}
	cb, err := era.CreateCharge(ctx, other, "pos-key-other2", decimal.NewFromInt(2000))
	if err != nil {
		t.Fatal(err)
	}
	fake.mu.Lock()
	owner := fake.usedBy["pay-"+cb.ID.String()]
	fake.mu.Unlock()
	if owner != keyB {
		t.Fatalf("tagihan tenant B dibuat dengan key yang salah: %.20s", owner)
	}
	// Key dicabut dari ARUS: tidak bisa membuat QR baru; konfigurasi menunjukkan belum terpasang.
	if err := era.RemoveKey(ctx, other); err != nil {
		t.Fatal(err)
	}
	if _, err := era.CreateCharge(ctx, other, "pos-key-other3", decimal.NewFromInt(2000)); err != erapay.ErrNotConfigured {
		t.Fatalf("setelah key dicabut: %v", err)
	}
	if cfgO, err := era.Config(ctx, other); err != nil || cfgO.Configured || cfgO.Enabled {
		t.Fatalf("konfigurasi setelah cabut: %+v %v", cfgO, err)
	}
}
