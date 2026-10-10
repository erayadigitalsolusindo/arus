package sales

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/google/uuid"
)

// Model struk memuat identitas outlet + nota, jam menurut zona waktu outlet, dan jumlah cetak ulang.
func TestReceiptModelAndReprintCount(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	e.exec(t, `UPDATE outlets SET address = 'Jl. Pemuda 12'||chr(10)||'Magelang', phone = '0293-123456',
		receipt_header = 'Selamat datang', receipt_footer = 'Terima kasih', timezone = 'Asia/Jakarta' WHERE id = $1`, e.outlet)
	it := e.item(t, "goods", "1000", "0", 5, false)
	a := e.actor(e.tenant)
	s, _, err := e.svc.Create(ctx, a, key(), Request{Lines: []LineIn{line(it, "2")}, Payments: []PaymentIn{pay("cash", "5000")}})
	if err != nil {
		t.Fatal(err)
	}
	rc, err := e.svc.Receipt(ctx, a, s.ID)
	if err != nil {
		t.Fatal(err)
	}
	st := rc.Store
	if st.TenantName != "UJI-JUAL" || st.OutletName != "Pusat" || st.OutletCode != "main" || st.Address != "Jl. Pemuda 12\nMagelang" ||
		st.Phone != "0293-123456" || st.Header != "Selamat datang" || st.Footer != "Terima kasih" {
		t.Fatalf("identitas toko: %+v", st)
	}
	if rc.Sale.DocNo != s.DocNo || rc.Sale.Total != s.Total || len(rc.Sale.Lines) != 1 || rc.Reprints != 0 {
		t.Fatalf("isi nota: %+v", rc)
	}
	if want := localTime(s.CreatedAt, "Asia/Jakarta"); rc.LocalTime != want || len(want) != len("02/01/2006 15:04") {
		t.Fatalf("jam struk %q, mau %q", rc.LocalTime, want)
	}

	// Cetak ulang bersamaan tetap mendapat nomor salinan unik 1..n.
	const n = 6
	var wg sync.WaitGroup
	got := make(chan int, n)
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			c, err := e.svc.Reprint(ctx, a, s.ID)
			if err != nil {
				t.Error(err)
			}
			got <- c
		}()
	}
	wg.Wait()
	close(got)
	seen := map[int]bool{}
	for c := range got {
		seen[c] = true
	}
	for i := 1; i <= n; i++ {
		if !seen[i] {
			t.Fatalf("nomor salinan %d tidak ada: %v", i, seen)
		}
	}
	if rc, _ = e.svc.Receipt(ctx, a, s.ID); rc.Reprints != n {
		t.Fatalf("jumlah cetak ulang %d, mau %d", rc.Reprints, n)
	}
}

// Struk mengikuti aturan akses nota: kasir lain, cabang lain, dan tenant lain tidak bisa membaca/mencetak ulang.
func TestReceiptAccessFollowsSale(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	it := e.item(t, "goods", "1000", "0", 5, false)
	a := e.actor(e.tenant)
	s, _, err := e.svc.Create(ctx, a, key(), Request{Lines: []LineIn{line(it, "1")}, Payments: []PaymentIn{pay("cash", "1000")}})
	if err != nil {
		t.Fatal(err)
	}
	other := a
	other.UserID = uuid.New()
	if _, err := e.svc.Receipt(ctx, other, s.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("kasir lain: %v", err)
	}
	if _, err := e.svc.Reprint(ctx, other, s.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("cetak ulang kasir lain: %v", err)
	}
	foreign := e.actor(e.other)
	if _, err := e.svc.Receipt(ctx, foreign, s.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("tenant lain: %v", err)
	}
	outlet2 := uuid.New()
	e.exec(t, `INSERT INTO outlets (id, tenant_id, code, name) VALUES ($1, $2, 'cab2', 'Cabang Dua')`, outlet2, e.tenant)
	blind := a
	blind.OutletID = outlet2
	blind.Outlets = map[uuid.UUID]bool{outlet2: true}
	if _, err := e.svc.Reprint(ctx, blind, s.ID); !errors.Is(err, ErrOutletForbidden) {
		t.Fatalf("cabang lain: %v", err)
	}
	var n int
	if err := e.admin.QueryRow(ctx, `SELECT count(*) FROM audit_log WHERE tenant_id = $1 AND action = 'sale.reprint'`, e.tenant).Scan(&n); err != nil || n != 0 {
		t.Fatalf("penolakan tidak boleh tercatat sebagai cetak ulang: %d %v", n, err)
	}
}
