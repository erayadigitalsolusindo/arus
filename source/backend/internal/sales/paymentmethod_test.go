package sales

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/google/uuid"
)

// method membuat metode di master tenant dan mengembalikan id-nya.
func (e *env) method(t *testing.T, tenant uuid.UUID, name, kind string, active bool) uuid.UUID {
	t.Helper()
	id := uuid.New()
	e.exec(t, `INSERT INTO payment_methods (id, tenant_id, name, kind, active) VALUES ($1, $2, $3, $4, $5)`, id, tenant, name, kind, active)
	return id
}

func payID(id uuid.UUID, amount string) PaymentIn {
	return PaymentIn{MethodID: &id, Amount: json.Number(amount)}
}

func TestPaymentMethodsFromMaster(t *testing.T) {
	e := newEnv(t)
	a := e.item(t, "goods", "1000", "0", 50, false)
	ctx := context.Background()
	qris := e.method(t, e.tenant, "QRIS BCA", "ewallet", true)
	closed := e.method(t, e.tenant, "Voucher Lama", "transfer", false)
	foreign := e.method(t, e.other, "QRIS Orang Lain", "ewallet", true)

	// Metode kustom: jenis dasar (ewallet) dari master menentukan aturan; nama + id tersimpan di nota.
	s, _, err := e.svc.Create(ctx, e.actor(e.tenant), key(), Request{Lines: []LineIn{line(a, "3")}, Payments: []PaymentIn{payID(qris, "3000")}})
	if err != nil {
		t.Fatal(err)
	}
	if len(s.Payments) != 1 || s.Payments[0].MethodID != qris || s.Payments[0].MethodName != "QRIS BCA" || s.Payments[0].Method != "ewallet" {
		t.Fatalf("pembayaran tersimpan: %+v", s.Payments)
	}

	// Jenis non-tunai tetap tidak boleh melebihi total, walau lewat metode kustom.
	_, _, err = e.svc.Create(ctx, e.actor(e.tenant), key(), Request{Lines: []LineIn{line(a, "2")}, Payments: []PaymentIn{payID(qris, "2500")}})
	fieldErr(t, err, "payments", codeNonCashOver)

	// Tunai bawaan lewat id tetap menghasilkan kembalian.
	var cash uuid.UUID
	if err := e.admin.QueryRow(ctx, `SELECT id FROM payment_methods WHERE tenant_id = $1 AND kind = 'cash'`, e.tenant).Scan(&cash); err != nil {
		t.Fatal(err)
	}
	s2, _, err := e.svc.Create(ctx, e.actor(e.tenant), key(), Request{Lines: []LineIn{line(a, "2")}, Payments: []PaymentIn{payID(cash, "5000")}})
	if err != nil || s2.Change != "3000.00" || s2.Payments[0].Method != "cash" || s2.Payments[0].MethodName != "Tunai" {
		t.Fatalf("tunai: %+v %v", s2, err)
	}

	// Pemanggil lama (hanya jenis dasar) dipetakan ke metode aktif tertua berjenis itu.
	s3, _, err := e.svc.Create(ctx, e.actor(e.tenant), key(), Request{Lines: []LineIn{line(a, "1")}, Payments: []PaymentIn{pay("debit", "1000")}})
	if err != nil || s3.Payments[0].MethodName != "Debit" || s3.Payments[0].MethodID == uuid.Nil {
		t.Fatalf("jenis saja: %+v %v", s3.Payments, err)
	}

	// Ditolak: metode terarsip, milik tenant lain, id tak dikenal, id nol. Tak ada nota/stok yang berubah.
	before, stock := e.count(t, "sales"), e.stockOf(t, a)
	unknown := uuid.New()
	nilID := uuid.Nil
	for _, c := range []struct {
		id   uuid.UUID
		code string
	}{{closed, "METHOD_INACTIVE"}, {foreign, "INVALID"}, {unknown, "INVALID"}, {nilID, "INVALID"}} {
		_, _, err = e.svc.Create(ctx, e.actor(e.tenant), key(), Request{Lines: []LineIn{line(a, "1")}, Payments: []PaymentIn{payID(c.id, "1000")}})
		fieldErr(t, err, "payments.0.method_id", c.code)
	}
	if e.count(t, "sales") != before || e.stockOf(t, a) != stock {
		t.Fatal("penolakan tidak boleh meninggalkan nota atau mengubah stok")
	}

	// Mengganti nama / mengarsipkan metode TIDAK mengubah nota lama (snapshot nama), dan metode tak bisa dihapus.
	e.exec(t, `UPDATE payment_methods SET name = 'QRIS Baru', active = false WHERE id = $1`, qris)
	got, err := e.svc.Get(ctx, e.actor(e.tenant), s.ID)
	if err != nil || got.Payments[0].MethodName != "QRIS BCA" {
		t.Fatalf("nama di nota lama: %+v %v", got.Payments, err)
	}
	if _, err := e.admin.Exec(ctx, `DELETE FROM payment_methods WHERE id = $1`, qris); err == nil {
		t.Fatal("metode yang dipakai nota tidak boleh terhapus (FK RESTRICT)")
	}

	// Kunci idempotensi sama + metode sama = nota yang sama; metode berbeda = isi berbeda.
	k := key()
	m2 := e.method(t, e.tenant, "GoPay", "ewallet", true)
	first, _, err := e.svc.Create(ctx, e.actor(e.tenant), k, Request{Lines: []LineIn{line(a, "1")}, Payments: []PaymentIn{payID(m2, "1000")}})
	if err != nil {
		t.Fatal(err)
	}
	again, replayed, err := e.svc.Create(ctx, e.actor(e.tenant), k, Request{Lines: []LineIn{line(a, "1")}, Payments: []PaymentIn{payID(m2, "1000")}})
	if err != nil || !replayed || again.ID != first.ID {
		t.Fatalf("replay: %v %v", replayed, err)
	}
	if _, _, err = e.svc.Create(ctx, e.actor(e.tenant), k, Request{Lines: []LineIn{line(a, "1")}, Payments: []PaymentIn{payID(cash, "1000")}}); err != ErrKeyMismatch {
		t.Fatalf("kunci sama metode beda: %v", err)
	}
}

// Ringkasan Daftar Penjualan memisahkan metode berjenis sama (QRIS vs GoPay), tunai bersih dari kembalian,
// dan nota void tidak ikut dihitung.
func TestListAllBreaksDownByMethod(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	a := e.item(t, "goods", "1000", "0", 100, false)
	qris := e.method(t, e.tenant, "QRIS BCA", "ewallet", true)
	gopay := e.method(t, e.tenant, "GoPay", "ewallet", true)
	var cash uuid.UUID
	if err := e.admin.QueryRow(ctx, `SELECT id FROM payment_methods WHERE tenant_id = $1 AND kind = 'cash'`, e.tenant).Scan(&cash); err != nil {
		t.Fatal(err)
	}
	act := e.actor(e.tenant)
	for _, in := range []Request{
		{Lines: []LineIn{line(a, "3")}, Payments: []PaymentIn{payID(qris, "3000")}},
		{Lines: []LineIn{line(a, "2")}, Payments: []PaymentIn{payID(gopay, "2000")}},
		{Lines: []LineIn{line(a, "1")}, Payments: []PaymentIn{payID(qris, "1000")}},
		{Lines: []LineIn{line(a, "4")}, Payments: []PaymentIn{payID(cash, "5000")}}, // kembalian 1.000
	} {
		if _, _, err := e.svc.Create(ctx, act, key(), in); err != nil {
			t.Fatal(err)
		}
	}
	res, err := e.svc.ListAll(ctx, act, AllParams{})
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]string{}
	for _, m := range res.Summary.ByMethod {
		got[m.Name] = m.Amount
	}
	if len(res.Summary.ByMethod) != 3 || got["Tunai"] != "4000.00" || got["QRIS BCA"] != "4000.00" || got["GoPay"] != "2000.00" {
		t.Fatalf("rincian per metode: %+v", res.Summary.ByMethod)
	}
	if res.Summary.ByMethod[0].Kind != "cash" {
		t.Fatalf("Tunai harus pertama: %+v", res.Summary.ByMethod)
	}
	// Ringkasan per jenis tetap: ewallet = QRIS + GoPay.
	if res.Summary.Methods["ewallet"] != "6000.00" || res.Summary.Methods["cash"] != "4000.00" {
		t.Fatalf("per jenis: %+v", res.Summary.Methods)
	}
}

// Biaya metode (MDR) ditanggung toko: dicatat per pembayaran sebagai snapshot, TIDAK mengubah total nota, dan
// mengubah tarif sesudahnya tidak mengubah nota lama.
func TestPaymentMethodFeeSnapshot(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	a := e.item(t, "goods", "1000", "0", 100, false)
	qris := e.method(t, e.tenant, "QRIS BCA", "ewallet", true)
	e.exec(t, `UPDATE payment_methods SET fee_pct = 0.7, fee_flat = 100 WHERE id = $1`, qris)
	act := e.actor(e.tenant)

	// 10.000 × 0,7% = 70 + 100 = 170; total nota tetap 10.000.
	s, _, err := e.svc.Create(ctx, act, key(), Request{Lines: []LineIn{line(a, "10")}, Payments: []PaymentIn{payID(qris, "10000")}})
	if err != nil {
		t.Fatal(err)
	}
	p := s.Payments[0]
	if p.Fee != "170.00" || p.FeePct != "0.70" || s.Total != "10000.00" {
		t.Fatalf("biaya: fee=%s pct=%s total=%s", p.Fee, p.FeePct, s.Total)
	}

	// Split: biaya hanya pada bagian yang dibayar lewat metode berbiaya (3.000 × 0,7% + 100 = 121); tunai bebas biaya.
	var cash uuid.UUID
	if err := e.admin.QueryRow(ctx, `SELECT id FROM payment_methods WHERE tenant_id = $1 AND kind = 'cash'`, e.tenant).Scan(&cash); err != nil {
		t.Fatal(err)
	}
	s2, _, err := e.svc.Create(ctx, act, key(), Request{Lines: []LineIn{line(a, "5")}, Payments: []PaymentIn{payID(cash, "2000"), payID(qris, "3000")}})
	if err != nil || s2.Payments[0].Fee != "0.00" || s2.Payments[1].Fee != "121.00" {
		t.Fatalf("split: %+v %v", s2.Payments, err)
	}

	// Tarif diubah: nota lama tetap, nota baru memakai tarif baru.
	e.exec(t, `UPDATE payment_methods SET fee_pct = 2, fee_flat = 0 WHERE id = $1`, qris)
	old, err := e.svc.Get(ctx, act, s.ID)
	if err != nil || old.Payments[0].Fee != "170.00" {
		t.Fatalf("nota lama: %+v %v", old.Payments, err)
	}
	s3, _, err := e.svc.Create(ctx, act, key(), Request{Lines: []LineIn{line(a, "1")}, Payments: []PaymentIn{payID(qris, "1000")}})
	if err != nil || s3.Payments[0].Fee != "20.00" {
		t.Fatalf("tarif baru: %+v %v", s3.Payments, err)
	}

	// Ringkasan: jumlah dan biaya per metode (170 + 121 + 20 = 311 pada jumlah 10.000 + 3.000 + 1.000).
	res, err := e.svc.ListAll(ctx, act, AllParams{})
	if err != nil {
		t.Fatal(err)
	}
	for _, m := range res.Summary.ByMethod {
		if m.Name == "QRIS BCA" && (m.Amount != "14000.00" || m.Fee != "311.00") {
			t.Fatalf("ringkasan QRIS: %+v", m)
		}
		if m.Kind == "cash" && m.Fee != "0.00" {
			t.Fatalf("tunai berbiaya: %+v", m)
		}
	}
}

// Biaya ditanggung PELANGGAN: tagihan tambahan di luar total nota (stok/pajak/poin tak berubah); pembayaran
// non-tunai tetap tak boleh melebihi total; ringkasan memisahkan biaya toko dan biaya pelanggan.
func TestCustomerBorneFee(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	a := e.item(t, "goods", "1000", "0", 300, false)
	qris := e.method(t, e.tenant, "QRIS BCA", "ewallet", true)
	debit := e.method(t, e.tenant, "Debit BCA", "debit", true)
	e.exec(t, `UPDATE payment_methods SET fee_pct = 0.7, fee_flat = 0, fee_bearer = 'customer' WHERE id = $1`, qris)
	e.exec(t, `UPDATE payment_methods SET fee_pct = 1, fee_flat = 0, fee_bearer = 'store' WHERE id = $1`, debit)
	act := e.actor(e.tenant)

	// 100.000 lewat QRIS (pelanggan): biaya 700 ditagihkan tambahan; total nota tetap 100.000; stok mengikuti total.
	s, _, err := e.svc.Create(ctx, act, key(), Request{Lines: []LineIn{line(a, "100")}, Payments: []PaymentIn{payID(qris, "100000")}})
	if err != nil {
		t.Fatal(err)
	}
	p := s.Payments[0]
	if s.Total != "100000.00" || s.Surcharge != "700.00" || p.Fee != "700.00" || p.FeeBearer != "customer" || s.Change != "0.00" {
		t.Fatalf("pelanggan: total=%s surcharge=%s pay=%+v change=%s", s.Total, s.Surcharge, p, s.Change)
	}
	if e.stockOf(t, a) != "200" {
		t.Fatalf("stok = %s, ingin 200", e.stockOf(t, a))
	}

	// Biaya toko (debit 1%) tidak masuk surcharge; nota campur: tunai bebas biaya, QRIS 20.000 → 140.
	s2, _, err := e.svc.Create(ctx, act, key(), Request{Lines: []LineIn{line(a, "50")}, Payments: []PaymentIn{payID(debit, "50000")}})
	if err != nil || s2.Surcharge != "0.00" || s2.Payments[0].Fee != "500.00" || s2.Payments[0].FeeBearer != "store" {
		t.Fatalf("toko: %+v %v", s2, err)
	}
	var cash uuid.UUID
	if err := e.admin.QueryRow(ctx, `SELECT id FROM payment_methods WHERE tenant_id = $1 AND kind = 'cash'`, e.tenant).Scan(&cash); err != nil {
		t.Fatal(err)
	}
	s3, _, err := e.svc.Create(ctx, act, key(), Request{Lines: []LineIn{line(a, "30")}, Payments: []PaymentIn{payID(cash, "10000"), payID(qris, "20000")}})
	if err != nil || s3.Surcharge != "140.00" || s3.Payments[0].Fee != "0.00" || s3.Payments[1].Fee != "140.00" {
		t.Fatalf("campur: %+v %v", s3, err)
	}

	// Detail nota membawa surcharge; ringkasan memisahkan biaya toko dan biaya pelanggan per metode.
	got, err := e.svc.Get(ctx, act, s.ID)
	if err != nil || got.Surcharge != "700.00" {
		t.Fatalf("get: %+v %v", got.Surcharge, err)
	}
	res, err := e.svc.ListAll(ctx, act, AllParams{})
	if err != nil {
		t.Fatal(err)
	}
	for _, m := range res.Summary.ByMethod {
		switch m.Name {
		case "QRIS BCA":
			if m.Amount != "120000.00" || m.Surcharge != "840.00" || m.Fee != "0.00" {
				t.Fatalf("QRIS: %+v", m)
			}
		case "Debit BCA":
			if m.Fee != "500.00" || m.Surcharge != "0.00" {
				t.Fatalf("Debit: %+v", m)
			}
		}
	}

	// Mengubah penanggung sesudahnya tidak mengubah nota lama.
	e.exec(t, `UPDATE payment_methods SET fee_bearer = 'store' WHERE id = $1`, qris)
	old, err := e.svc.Get(ctx, act, s.ID)
	if err != nil || old.Surcharge != "700.00" || old.Payments[0].FeeBearer != "customer" {
		t.Fatalf("nota lama: %+v %v", old.Payments, err)
	}
}
