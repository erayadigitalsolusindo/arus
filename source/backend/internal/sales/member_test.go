package sales

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/google/uuid"

	"aciraba/internal/member"
)

// newMember membuat member beserta level dasar (Rp10.000/poin, 1 poin = Rp100). Level dasar saja agar hitungan mudah.
func (e *env) newMember(t *testing.T, points int, active bool) uuid.UUID {
	t.Helper()
	e.exec(t, `INSERT INTO member_levels (tenant_id, name, min_points, spend_per_point, point_value) VALUES ($1, 'Reguler', 0, 10000, 100) ON CONFLICT DO NOTHING`, e.tenant)
	id := uuid.New()
	e.exec(t, `INSERT INTO members (id, tenant_id, code, name, active, points, lifetime_points) VALUES ($1, $2, $3, 'Budi', $4, $5, $5)`,
		id, e.tenant, "M-"+id.String()[:8], active, points)
	return id
}

func (e *env) memberPoints(t *testing.T, id uuid.UUID) (points, lifetime int) {
	t.Helper()
	if err := e.admin.QueryRow(context.Background(), `SELECT points, lifetime_points FROM members WHERE id = $1`, id).Scan(&points, &lifetime); err != nil {
		t.Fatal(err)
	}
	return
}

func TestMemberEarnAndRedeem(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	a := e.actor(e.tenant)
	it := e.item(t, "goods", "50000", "30000", 20, false)
	m := e.newMember(t, 0, true)

	// 3 × 50.000 = 150.000 → floor(150.000 ÷ 10.000) = 15 poin.
	s1, _, err := e.svc.Create(ctx, a, key(), Request{Lines: []LineIn{line(it, "3")}, Payments: []PaymentIn{pay("cash", "150000")}, MemberID: &m})
	if err != nil {
		t.Fatal(err)
	}
	if s1.PointsEarned != 15 || s1.Member == nil || s1.Member.ID != m || s1.PointsRedeemed != 0 {
		t.Fatalf("nota 1: %+v", s1)
	}
	if p, l := e.memberPoints(t, m); p != 15 || l != 15 {
		t.Fatalf("poin setelah nota 1 = %d/%d, want 15/15", p, l)
	}

	// Quote tidak mengubah apa pun dan menampilkan hasil tukar + poin yang akan diperoleh.
	q, err := e.svc.Quote(ctx, a, Request{Lines: []LineIn{line(it, "1")}, MemberID: &m, RedeemPoints: 10})
	if err != nil || q.RedeemAmount != "1000.00" || q.Total != "49000.00" || q.PointsEarn != 4 || q.Member == nil || q.Member.Points != 15 {
		t.Fatalf("quote: %+v err=%v", q, err)
	}
	if p, _ := e.memberPoints(t, m); p != 15 {
		t.Fatalf("quote mengubah poin: %d", p)
	}

	// Tukar 10 poin = potongan Rp1.000 → total 49.000, peroleh floor(49.000 ÷ 10.000) = 4 poin. Saldo 15 − 10 + 4 = 9.
	k := key()
	s2, _, err := e.svc.Create(ctx, a, k, Request{Lines: []LineIn{line(it, "1")}, Payments: []PaymentIn{pay("cash", "49000")}, MemberID: &m, RedeemPoints: 10})
	if err != nil {
		t.Fatal(err)
	}
	if s2.Total != "49000.00" || s2.Discount != "1000.00" || s2.RedeemAmount != "1000.00" || s2.PointsRedeemed != 10 || s2.PointsEarned != 4 {
		t.Fatalf("nota 2: %+v", s2)
	}
	if p, l := e.memberPoints(t, m); p != 9 || l != 19 {
		t.Fatalf("poin setelah nota 2 = %d/%d, want 9/19", p, l)
	}
	// Kirim ulang dengan kunci sama → nota yang sama, poin tidak dihitung dua kali.
	r, replayed, err := e.svc.Create(ctx, a, k, Request{Lines: []LineIn{line(it, "1")}, Payments: []PaymentIn{pay("cash", "49000")}, MemberID: &m, RedeemPoints: 10})
	if err != nil || !replayed || r.ID != s2.ID {
		t.Fatalf("replay: replayed=%v err=%v", replayed, err)
	}
	if p, _ := e.memberPoints(t, m); p != 9 {
		t.Fatalf("replay mengubah poin: %d", p)
	}
	// Riwayat transaksi member: dua nota, terbaru dulu, dengan ringkasan barang dan poin.
	hist, total, err := member.NewService(e.app, nil).Sales(ctx, a, m, 20, 0)
	if err != nil || total != 2 || len(hist) != 2 || hist[0].ID != s2.ID || hist[0].PointsEarned != 4 || hist[0].PointsRedeemed != 10 || hist[0].Items == "" || hist[0].LineCount != 1 || hist[1].ID != s1.ID {
		t.Fatalf("riwayat: total=%d %+v err=%v", total, hist, err)
	}
	// Nota dapat dibaca kembali lengkap dengan member.
	g, err := e.svc.Get(ctx, a, s2.ID)
	if err != nil || g.Member == nil || g.Member.Code == "" || g.PointsEarned != 4 || g.PointsRedeemed != 10 {
		t.Fatalf("get: %+v err=%v", g, err)
	}
}

func TestMemberRejections(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	a := e.actor(e.tenant)
	it := e.item(t, "goods", "50000", "30000", 20, false)
	m := e.newMember(t, 5, true)
	off := e.newMember(t, 0, false)
	expired := e.newMember(t, 0, true)
	e.exec(t, `UPDATE members SET valid_until = current_date - 2 WHERE id = $1`, expired)
	foreign := uuid.New() // member milik tenant lain
	e.exec(t, `INSERT INTO members (id, tenant_id, code, name) VALUES ($1, $2, 'ASING', 'Asing')`, foreign, e.other)

	fieldIs := func(err error, field, code string) bool {
		var fe FieldErrors
		return errors.As(err, &fe) && fe[field] == code
	}
	req := func(mid *uuid.UUID, redeem int) Request {
		return Request{Lines: []LineIn{line(it, "1")}, Payments: []PaymentIn{pay("cash", "50000")}, MemberID: mid, RedeemPoints: redeem}
	}
	cases := []struct {
		name         string
		r            Request
		field, value string
	}{
		{"tukar tanpa member", req(nil, 1), "redeem_points", "MEMBER_REQUIRED"},
		{"melebihi saldo", req(&m, 6), "redeem_points", "POINTS_INSUFFICIENT"},
		{"tukar negatif", req(&m, -1), "redeem_points", "INVALID"},
		{"member nonaktif", req(&off, 0), "member_id", "MEMBER_INACTIVE"},
		{"member kedaluwarsa", req(&expired, 0), "member_id", "MEMBER_INACTIVE"},
		{"member tenant lain", req(&foreign, 0), "member_id", "INVALID"},
	}
	for _, c := range cases {
		if _, _, err := e.svc.Create(ctx, a, key(), c.r); !fieldIs(err, c.field, c.value) {
			t.Errorf("%s: err = %v, want %s=%s", c.name, err, c.field, c.value)
		}
	}
	// Potongan tukar poin tidak boleh melebihi nilai belanja: 5 poin × Rp100 = Rp500 > Rp300.
	cheap := e.item(t, "goods", "300", "100", 5, false)
	if _, _, err := e.svc.Create(ctx, a, key(), Request{Lines: []LineIn{line(cheap, "1")}, Payments: []PaymentIn{pay("cash", "300")}, MemberID: &m, RedeemPoints: 5}); !fieldIs(err, "redeem_points", "REDEEM_TOO_HIGH") {
		t.Errorf("tukar melebihi belanja: err = %v", err)
	}
	// Semua penolakan tidak meninggalkan nota, perubahan stok, maupun poin.
	if n := e.count(t, "sales"); n != 0 {
		t.Errorf("jumlah nota = %d, want 0", n)
	}
	if p, _ := e.memberPoints(t, m); p != 5 {
		t.Errorf("poin berubah = %d", p)
	}
}

func TestConcurrentRedeemAcrossSales(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	a := e.actor(e.tenant)
	it := e.item(t, "goods", "5000", "1000", 100, false)
	m := e.newMember(t, 10, true)

	var wg sync.WaitGroup
	var ok atomic.Int32
	for range 15 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			// 5.000 − 100 (1 poin) = 4.900; belanja < Rp10.000 sehingga tidak ada poin baru.
			if _, _, err := e.svc.Create(ctx, a, key(), Request{Lines: []LineIn{line(it, "1")}, Payments: []PaymentIn{pay("cash", "4900")}, MemberID: &m, RedeemPoints: 1}); err == nil {
				ok.Add(1)
			}
		}()
	}
	wg.Wait()
	if p, _ := e.memberPoints(t, m); ok.Load() != 10 || p != 0 {
		t.Errorf("15 nota serentak menukar 1 poin dari saldo 10: berhasil=%d saldo=%d (harus 10 dan 0)", ok.Load(), p)
	}
	if n := e.count(t, "sales"); n != 10 {
		t.Errorf("jumlah nota = %d, want 10", n)
	}
}

// Tukar poin tidak boleh membuat nota di bawah HPP (tukar poin tidak memakai PIN penyetuju).
func TestRedeemCannotGoBelowCost(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	a := e.actor(e.tenant)
	it := e.item(t, "goods", "50000", "49500", 10, false) // margin tipis: hanya Rp500
	m := e.newMember(t, 100, true)

	// 10 poin = Rp1.000 > margin Rp500 → ditolak, tidak ada nota, poin utuh.
	_, _, err := e.svc.Create(ctx, a, key(), Request{Lines: []LineIn{line(it, "1")}, Payments: []PaymentIn{pay("cash", "49000")}, MemberID: &m, RedeemPoints: 10})
	var fe FieldErrors
	if !errors.As(err, &fe) || fe["redeem_points"] != "REDEEM_BELOW_COST" {
		t.Fatalf("tukar melewati HPP: err = %v", err)
	}
	if n := e.count(t, "sales"); n != 0 {
		t.Errorf("nota tersimpan: %d", n)
	}
	if p, _ := e.memberPoints(t, m); p != 100 {
		t.Errorf("poin berubah: %d", p)
	}
	// 5 poin = Rp500 pas di HPP → boleh.
	if _, _, err := e.svc.Create(ctx, a, key(), Request{Lines: []LineIn{line(it, "1")}, Payments: []PaymentIn{pay("cash", "49500")}, MemberID: &m, RedeemPoints: 5}); err != nil {
		t.Fatalf("tukar sampai tepat HPP: %v", err)
	}
	// Barang yang boleh jual rugi tidak dibatasi.
	cheap := e.item(t, "goods", "50000", "49500", 10, false)
	e.exec(t, `UPDATE items SET sell_below_cost = true WHERE id = $1`, cheap)
	if _, _, err := e.svc.Create(ctx, a, key(), Request{Lines: []LineIn{line(cheap, "1")}, Payments: []PaymentIn{pay("cash", "49000")}, MemberID: &m, RedeemPoints: 10}); err != nil {
		t.Fatalf("barang boleh jual rugi: %v", err)
	}
}
