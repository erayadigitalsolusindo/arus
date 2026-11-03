package sales

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"aciraba/internal/erapay"
	"aciraba/internal/paymentmethod"
)

// Settlement EraPay: uang ARUS (nota aktif) dibandingkan dengan uang EraPay (tagihan lunas) per tagihan.
func TestEraPaySettlementView(t *testing.T) {
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
	item := e.item(t, "goods", "1000", "400", 50, false)

	// A: lunas dan tercatat di nota. B: lunas tetapi tidak ada nota (uang masuk, penjualan belum tercatat).
	fake.set("succeeded")
	a, err := era.CreateCharge(ctx, actor, "pos-view-0001", decimal.NewFromInt(2000))
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err = e.svc.Create(ctx, actor, key(), Request{Lines: []LineIn{line(item, "2")},
		Payments: []PaymentIn{{MethodID: &m, Amount: json.Number("2000"), RefNo: a.ID.String()}}}); err != nil {
		t.Fatalf("nota A: %v", err)
	}
	if _, err = era.CreateCharge(ctx, actor, "pos-view-0002", decimal.NewFromInt(3000)); err != nil {
		t.Fatal(err)
	}
	// C: QR dibuat tetapi belum dibayar.
	fake.set("pending")
	if _, err = era.CreateCharge(ctx, actor, "pos-view-0003", decimal.NewFromInt(5000)); err != nil {
		t.Fatal(err)
	}

	r, err := era.Settlement(ctx, actor, "", "", false, false)
	if err != nil {
		t.Fatal(err)
	}
	if !r.Configured || r.Totals.Count != 3 || r.Totals.Arus != "2000.00" || r.Totals.Era != "5000.00" || r.Totals.Diff != "3000.00" ||
		r.Totals.Matched != 1 || r.Totals.Waiting != 1 || r.Totals.Attention != 1 {
		t.Fatalf("ringkasan: %+v", r.Totals)
	}
	states := map[string]int{}
	for _, row := range r.Data {
		states[row.State]++
		if row.PaymentID == "" {
			t.Fatalf("ID transaksi EraPay kosong: %+v", row)
		}
		if row.State == erapay.StateMatched && (row.DocNo == "" || row.ArusAmount != "2000.00" || row.EraAmount != "2000.00") {
			t.Fatalf("baris cocok: %+v", row)
		}
		if row.State == erapay.StateOrphan && (row.ArusAmount != "0.00" || row.EraAmount != "3000.00") {
			t.Fatalf("baris tanpa nota: %+v", row)
		}
	}
	if states[erapay.StateMatched] != 1 || states[erapay.StateOrphan] != 1 || states[erapay.StateWaiting] != 1 {
		t.Fatalf("keadaan: %v", states)
	}

	// EraPay akhirnya menjawab lunas untuk C: refresh menanyakan ulang, lalu C ikut terhitung sebagai uang EraPay.
	fake.set("succeeded")
	r, err = era.Settlement(ctx, actor, "", "", false, true)
	if err != nil || r.Refreshed != 1 || r.Totals.Era != "10000.00" || r.Totals.Waiting != 0 || r.Totals.Attention != 2 {
		t.Fatalf("setelah refresh: %+v %v", r.Totals, err)
	}

	// Rentang lain tidak berisi apa pun; rentang tidak sah ditolak.
	if r, err = era.Settlement(ctx, actor, "2020-01-01", "2020-01-02", false, false); err != nil || r.Totals.Count != 0 {
		t.Fatalf("rentang kosong: %+v %v", r.Totals, err)
	}
	var fe erapay.FieldErrors
	if _, err = era.Settlement(ctx, actor, "2026-13-40", "", false, false); !errors.As(err, &fe) {
		t.Fatalf("tanggal salah: %v", err)
	}
	// Tenant lain tidak melihat tagihan ini.
	if r, err = era.Settlement(ctx, e.actor(e.other), "", "", false, false); err == nil && r.Totals.Count != 0 {
		t.Fatalf("tenant lain melihat tagihan: %+v", r.Totals)
	}
	_ = uuid.Nil
}
