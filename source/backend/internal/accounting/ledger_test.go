package accounting_test

import (
	"context"
	"testing"

	"github.com/google/uuid"

	"aciraba/internal/accounting"
)

func TestLedgerOpeningRunningAndKeyset(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	// Beban gaji (debit-normal): bulan lalu 1000 + 500, Okt: 100, 200, 300, 400, 500 (5 baris), draf Okt tidak ikut.
	e.post(t, "2026-08-15", "1000")
	e.post(t, "2026-09-30", "500")
	for _, a := range []string{"100", "200", "300", "400", "500"} {
		e.post(t, "2026-10-10", a)
	}
	e.draft(t, "2026-10-11", "99999")
	p := accounting.LedgerParams{AccountID: e.acc["6100"], From: "2026-10-01", To: "2026-10-31", Limit: 2}
	var rows []accounting.LedgerRow
	first, err := e.svc.Ledger(ctx, e.owner, p)
	if err != nil {
		t.Fatal(err)
	}
	if first.OpeningBalance != "1500.00" || first.TotalDebit == nil || *first.TotalDebit != "1500.00" || *first.TotalCredit != "0.00" {
		t.Fatalf("saldo awal/total halaman 1: %+v", first)
	}
	rows = append(rows, first.Rows...)
	pages := 1
	for cur := first.NextCursor; cur != ""; pages++ {
		p.Cursor = cur
		pg, err := e.svc.Ledger(ctx, e.owner, p)
		if err != nil {
			t.Fatal(err)
		}
		if pg.TotalDebit != nil || pg.OpeningBalance != "" {
			t.Fatalf("halaman lanjutan tak boleh memuat saldo awal/total")
		}
		rows = append(rows, pg.Rows...)
		cur = pg.NextCursor
	}
	if pages != 3 || len(rows) != 5 {
		t.Fatalf("halaman = %d, baris = %d, want 3 & 5", pages, len(rows))
	}
	// Urutan = urutan pencatatan (100,200,…,500); saldo berjalan 1500 + kumulatif.
	want := []string{"1600.00", "1800.00", "2100.00", "2500.00", "3000.00"}
	seen := map[uuid.UUID]bool{}
	for i, r := range rows {
		if r.Balance != want[i] || seen[r.LineID] {
			t.Errorf("baris %d: saldo %s (want %s), ganda=%v", i, r.Balance, want[i], seen[r.LineID])
		}
		seen[r.LineID] = true
	}
	// Kas (debit-normal) di kredit: saldo negatif; akun lawan tidak terpengaruh draf.
	kas, err := e.svc.Ledger(ctx, e.owner, accounting.LedgerParams{AccountID: e.acc["1110"], From: "2026-10-01", To: "2026-10-31"})
	if err != nil || kas.OpeningBalance != "-1500.00" || kas.Rows[len(kas.Rows)-1].Balance != "-3000.00" {
		t.Fatalf("buku kas: %v %+v", err, kas)
	}
	// Rentang tak sah dan outlet tak berhak.
	for _, bad := range []accounting.LedgerParams{
		{AccountID: e.acc["6100"], From: "2026-10-31", To: "2026-10-01"},
		{AccountID: e.acc["6100"], From: "2024-01-01", To: "2026-10-01"},
		{AccountID: e.acc["6100"], From: "x", To: "2026-10-01"},
	} {
		if _, err := e.svc.Ledger(ctx, e.owner, bad); err == nil {
			t.Errorf("rentang %+v harus ditolak", bad)
		}
	}
	if _, err := e.svc.Ledger(ctx, e.otherOwner, accounting.LedgerParams{AccountID: e.acc["6100"], From: "2026-10-01", To: "2026-10-31"}); err == nil {
		t.Error("buku besar akun tenant lain harus ditolak")
	}
}

func TestListJournalsKeyset(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	for i, d := range []string{"2026-10-01", "2026-10-02", "2026-10-02", "2026-10-03", "2026-10-04"} {
		e.post(t, d, "10")
		_ = i
	}
	e.draft(t, "2026-10-05", "1")
	p := accounting.JournalListParams{From: "2026-10-01", To: "2026-10-31", Limit: 2}
	var got []accounting.JournalSummary
	for pages := 0; pages < 10; pages++ {
		pg, err := e.svc.ListJournals(ctx, e.owner, p)
		if err != nil {
			t.Fatal(err)
		}
		got = append(got, pg.Items...)
		if pg.NextCursor == "" {
			break
		}
		p.Cursor = pg.NextCursor
	}
	if len(got) != 6 {
		t.Fatalf("baris = %d, want 6", len(got))
	}
	for i := 1; i < len(got); i++ {
		if got[i].Date > got[i-1].Date {
			t.Fatalf("urutan salah di %d", i)
		}
	}
	posted, _ := e.svc.ListJournals(ctx, e.owner, accounting.JournalListParams{From: "2026-10-01", To: "2026-10-31", Status: "posted"})
	if len(posted.Items) != 5 || posted.Items[0].Total != "10.00" {
		t.Fatalf("filter posted: %+v", posted.Items)
	}
}
