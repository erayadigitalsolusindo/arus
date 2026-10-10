package sales

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	"aciraba/internal/receivable"
)

// Saldo awal piutang (FR-ONB-04): tercatat tanpa nota, dibayar/dicicil seperti piutang nota, ikut limit kredit,
// ikut pelunasan kolektif (FIFO menurut tanggal dokumen lama), dan bisa dibatalkan selama belum dibayar.
func TestReceivableOpeningLifecycle(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	a := e.scoped()
	rs := receivable.NewService(e.app)
	m := e.creditMember(t, "100000")
	cash := e.methodID(t, "cash")
	today := time.Now().Format("2006-01-02")
	old := time.Now().AddDate(0, -2, 0).Format("2006-01-02")

	var fe receivable.FieldErrors
	bad := []receivable.OpeningInput{
		{MemberID: m, DocDate: old}, // jumlah kosong
		{MemberID: m, DocDate: time.Now().AddDate(0, 0, 2).Format("2006-01-02"), Amount: jn("1000")},                // tanggal masa depan
		{MemberID: uuid.New(), DocDate: old, Amount: jn("1000")},                                                    // member asing
		{MemberID: m, DocDate: old, Amount: jn("1000"), DueDate: time.Now().AddDate(0, -3, 0).Format("2006-01-02")}, // jatuh tempo sebelum dokumen
	}
	for i, in := range bad {
		if _, _, err := rs.CreateOpening(ctx, a, key(), in); !errors.As(err, &fe) {
			t.Fatalf("kasus %d harus ditolak: %v", i, err)
		}
	}

	k := key()
	in := receivable.OpeningInput{MemberID: m, RefNo: "BON-77", DocDate: old, Amount: jn("60000"), DueDate: today, Note: "catatan buku lama"}
	d, replayed, err := rs.CreateOpening(ctx, a, k, in)
	if err != nil || replayed || d.Kind != "opening" || d.SaleID != nil || d.Balance != "60000.00" || d.RefNo != "BON-77" || d.DocDate == nil || *d.DocDate != old {
		t.Fatalf("buat: %+v %v", d, err)
	}
	if d2, rp, err := rs.CreateOpening(ctx, a, k, in); err != nil || !rp || d2.ID != d.ID {
		t.Fatalf("replay: %v rp=%v", err, rp)
	}
	if _, _, err := rs.CreateOpening(ctx, a, k, receivable.OpeningInput{MemberID: m, DocDate: old, Amount: jn("1")}); !errors.Is(err, receivable.ErrKeyMismatch) {
		t.Fatalf("kunci sama isi beda: %v", err)
	}
	// No. referensi yang sama untuk member yang sama = salah input ganda.
	if _, _, err := rs.CreateOpening(ctx, a, key(), receivable.OpeningInput{MemberID: m, RefNo: "bon-77", DocDate: old, Amount: jn("5")}); !errors.As(err, &fe) || fe["ref_no"] != "DUPLICATE" {
		t.Fatalf("ref ganda: %v", err)
	}

	// Ikut daftar & ringkasan, dan ikut limit kredit: piutang lama 60.000 + nota kredit 50.000 > limit 100.000.
	l, err := rs.List(ctx, a, receivable.ListParams{Q: "BON-77"})
	if err != nil || len(l.Data) != 1 || l.Data[0].Kind != "opening" || l.Summary.Outstanding != "60000.00" {
		t.Fatalf("daftar: %+v %v", l, err)
	}
	it := e.item(t, "goods", "50000", "30000", 10, false)
	var cl *CreditLimitError
	if _, _, err := e.svc.Create(ctx, a, key(), creditReq(it, "1", &m, "", nil)); !errors.As(err, &cl) {
		t.Fatalf("limit kredit harus menghitung saldo awal: %v", err)
	}

	// Cicil lalu tidak bisa dibatalkan.
	if d, _, err = rs.Pay(ctx, a, d.ID, key(), receivable.PayInput{MethodID: cash, Amount: jn("10000")}); err != nil || d.Balance != "50000.00" {
		t.Fatalf("cicil: %+v %v", d, err)
	}
	if _, err := rs.VoidOpening(ctx, a, d.ID, "salah input"); !errors.Is(err, receivable.ErrHasPayments) {
		t.Fatalf("batal setelah dibayar: %v", err)
	}

	// Saldo awal kedua (lebih lama) + nota kredit: pelunasan kolektif membayar yang paling lama dulu.
	older := time.Now().AddDate(0, -6, 0).Format("2006-01-02")
	o2, _, err := rs.CreateOpening(ctx, a, key(), receivable.OpeningInput{MemberID: m, DocDate: older, Amount: jn("20000")})
	if err != nil {
		t.Fatal(err)
	}
	q, err := rs.SettleQuote(ctx, a, receivable.SettleInput{MemberID: m, Mode: "auto", Amount: jn("30000")})
	if err != nil || q.Outstanding != "70000.00" || len(q.Allocations) != 2 || q.Allocations[0].ReceivableID != o2.ID || q.Allocations[0].Amount != "20000.00" ||
		q.Allocations[1].ReceivableID != d.ID || q.Allocations[1].Amount != "10000.00" {
		t.Fatalf("pelunasan FIFO: %+v %v", q, err)
	}
	s, _, err := rs.Settle(ctx, a, key(), receivable.SettleInput{MemberID: m, Mode: "auto", MethodID: cash, Amount: jn("30000")})
	if err != nil || len(s.Allocations) != 2 || s.Allocations[0].SaleDocNo == "" {
		t.Fatalf("pelunasan: %+v %v", s, err)
	}

	// Saldo awal yang belum dibayar bisa dibatalkan (alasan wajib); hilang dari daftar dan dari hitungan limit.
	o3, _, err := rs.CreateOpening(ctx, a, key(), receivable.OpeningInput{MemberID: m, RefNo: "SALAH", DocDate: old, Amount: jn("999")})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := rs.VoidOpening(ctx, a, o3.ID, "x"); !errors.As(err, &fe) {
		t.Fatalf("alasan pendek: %v", err)
	}
	v, err := rs.VoidOpening(ctx, a, o3.ID, "salah input nominal")
	if err != nil || v.Status != "void" || v.VoidedAt == nil {
		t.Fatalf("batal: %+v %v", v, err)
	}
	if _, err := rs.VoidOpening(ctx, a, o3.ID, "lagi"); !errors.Is(err, receivable.ErrVoided) {
		t.Fatalf("batal dua kali: %v", err)
	}
	if _, _, err := rs.Pay(ctx, a, o3.ID, key(), receivable.PayInput{MethodID: cash, Amount: jn("1")}); !errors.Is(err, receivable.ErrNotFound) {
		t.Fatalf("bayar saldo awal batal: %v", err)
	}
	// No. referensi boleh dipakai lagi setelah yang lama dibatalkan.
	if _, _, err := rs.CreateOpening(ctx, a, key(), receivable.OpeningInput{MemberID: m, RefNo: "SALAH", DocDate: old, Amount: jn("99")}); err != nil {
		t.Fatalf("ref setelah batal: %v", err)
	}
	// Piutang nota tidak bisa dibatalkan lewat jalur saldo awal.
	sale, _, err := e.svc.Create(ctx, a, key(), creditReq(it, "1", ptr(e.creditMember(t, "0")), "", nil))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := rs.VoidOpening(ctx, a, sale.Credit.ID, "bukan saldo awal"); !errors.Is(err, receivable.ErrNotOpening) {
		t.Fatalf("batal piutang nota: %v", err)
	}
	// Tenant lain tidak melihat saldo awal ini.
	stranger := e.actor(e.other)
	stranger.Outlets = map[uuid.UUID]bool{e.outlet: true}
	if _, err := rs.Get(ctx, stranger, d.ID); !errors.Is(err, receivable.ErrNotFound) {
		t.Fatalf("tenant lain: %v", err)
	}
}

// Bayar dan batal saldo awal bersamaan: tepat satu yang menang (berbagi kunci baris piutang).
func TestReceivableOpeningPayVsVoid(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	a := e.scoped()
	rs := receivable.NewService(e.app)
	m := e.creditMember(t, "0")
	cash := e.methodID(t, "cash")
	old := time.Now().AddDate(0, -1, 0).Format("2006-01-02")
	for round := 0; round < 5; round++ {
		o, _, err := rs.CreateOpening(ctx, a, key(), receivable.OpeningInput{MemberID: m, DocDate: old, Amount: jn("1000")})
		if err != nil {
			t.Fatal(err)
		}
		var wg sync.WaitGroup
		var payErr, voidErr error
		start := make(chan struct{})
		wg.Add(2)
		go func() {
			defer wg.Done()
			<-start
			_, _, payErr = rs.Pay(ctx, a, o.ID, key(), receivable.PayInput{MethodID: cash, Amount: jn("1000")})
		}()
		go func() {
			defer wg.Done()
			<-start
			_, voidErr = rs.VoidOpening(ctx, a, o.ID, "uji serentak")
		}()
		close(start)
		wg.Wait()
		if (payErr == nil) == (voidErr == nil) {
			t.Fatalf("ronde %d: bayar=%v batal=%v (harus tepat satu)", round, payErr, voidErr)
		}
		d, err := rs.Get(ctx, a, o.ID)
		if err != nil || (payErr == nil && (d.Status != "paid" || d.VoidedAt != nil)) || (voidErr == nil && (d.Status != "void" || d.Paid != "0.00")) {
			t.Fatalf("ronde %d: hasil %+v %v", round, d, err)
		}
	}
}
