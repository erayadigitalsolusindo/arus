package accounting_test

import (
	"context"
	"testing"

	"github.com/google/uuid"

	"aciraba/internal/accounting"
)

func TestFinancialReports(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	mk := func(date, dr, cr, amount string) {
		t.Helper()
		d, err := e.svc.SaveDraft(ctx, e.owner, uuid.Nil, journalIn(date, e.acc[dr], e.acc[cr], amount))
		if err != nil {
			t.Fatal(err)
		}
		if _, err := e.svc.Post(ctx, e.owner, d.ID); err != nil {
			t.Fatal(err)
		}
	}
	mk("2026-09-10", "1110", "3100", "1000000")
	mk("2026-10-05", "1210", "4100", "500000")
	mk("2026-10-06", "6100", "1110", "200000")

	tb, err := e.svc.TrialBalance(ctx, e.owner, uuid.Nil, "2026-10-01", "2026-10-31")
	if err != nil {
		t.Fatal(err)
	}
	if !tb.Balanced {
		t.Fatalf("neraca saldo harus seimbang: %+v", tb.Totals)
	}
	byCode := map[string]string{}
	for _, r := range tb.Rows {
		byCode[r.Code] = r.OpeningDebit + "|" + r.Debit + "|" + r.Credit + "|" + r.ClosingDebit + "|" + r.ClosingCredit
	}
	if got, want := byCode["1110"], "1000000.00|0.00|200000.00|800000.00|0.00"; got != want {
		t.Fatalf("kas = %s, want %s", got, want)
	}
	if got, want := byCode["3100"], "0.00|0.00|0.00|0.00|1000000.00"; got != want {
		t.Fatalf("modal = %s, want %s", got, want)
	}

	is, err := e.svc.IncomeStatement(ctx, e.owner, uuid.Nil, "2026-10-01", "2026-10-31")
	if err != nil {
		t.Fatal(err)
	}
	if is.Revenue.Total != "500000.00" || is.Expenses.Total != "200000.00" || is.NetProfit != "300000.00" {
		t.Fatalf("laba rugi: %+v", is)
	}
	// Rentang September tidak memuat penjualan Oktober.
	if sep, _ := e.svc.IncomeStatement(ctx, e.owner, uuid.Nil, "2026-09-01", "2026-09-30"); sep.NetProfit != "0.00" {
		t.Fatalf("laba September = %s", sep.NetProfit)
	}

	bs, err := e.svc.BalanceSheet(ctx, e.owner, uuid.Nil, "2026-10-31")
	if err != nil {
		t.Fatal(err)
	}
	if !bs.Balanced || bs.TotalAssets != "1300000.00" || bs.UnclosedProfit != "300000.00" || bs.Equity.Total != "1000000.00" {
		t.Fatalf("neraca: %+v", bs)
	}
	// Posisi per 30 September: belum ada laba.
	if old, _ := e.svc.BalanceSheet(ctx, e.owner, uuid.Nil, "2026-09-30"); !old.Balanced || old.TotalAssets != "1000000.00" {
		t.Fatalf("neraca 30 Sep: %+v", old)
	}

	cb, err := e.svc.CashBank(ctx, e.owner, uuid.Nil, "2026-10-01", "2026-10-31")
	if err != nil {
		t.Fatal(err)
	}
	if cb.Total.Opening != "1000000.00" || cb.Total.Credit != "200000.00" || cb.Total.Closing != "800000.00" {
		t.Fatalf("kas/bank: %+v", cb.Total)
	}

	gj, err := e.svc.GeneralJournal(ctx, e.owner, uuid.Nil, "2026-09-01", "2026-10-31", "", 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(gj.Items) != 2 || gj.NextCursor == "" || gj.Items[0].Date != "2026-09-10" || len(gj.Items[0].Lines) != 2 {
		t.Fatalf("jurnal umum hal. 1: %+v", gj)
	}
	next, err := e.svc.GeneralJournal(ctx, e.owner, uuid.Nil, "2026-09-01", "2026-10-31", gj.NextCursor, 2)
	if err != nil || len(next.Items) != 1 || next.NextCursor != "" || next.Items[0].Date != "2026-10-06" {
		t.Fatalf("jurnal umum hal. 2: %+v err=%v", next, err)
	}

	// Draf tidak ikut laporan; tenant lain tidak melihat data.
	if _, err := e.svc.SaveDraft(ctx, e.owner, uuid.Nil, journalIn("2026-10-07", e.acc["6100"], e.acc["1110"], "999")); err != nil {
		t.Fatal(err)
	}
	if is2, _ := e.svc.IncomeStatement(ctx, e.owner, uuid.Nil, "2026-10-01", "2026-10-31"); is2.NetProfit != "300000.00" {
		t.Fatalf("draf ikut laporan: %s", is2.NetProfit)
	}
	if other, _ := e.svc.TrialBalance(ctx, e.otherOwner, uuid.Nil, "2026-10-01", "2026-10-31"); len(other.Rows) != 0 {
		t.Fatalf("tenant lain melihat %d baris", len(other.Rows))
	}
}

// journalIn = jurnal JU dua baris: debit akun dr, kredit akun cr.
func journalIn(date string, dr, cr uuid.UUID, amount string) accounting.JournalInput {
	return accounting.JournalInput{Date: date, Type: "JU", Narration: "uji laporan",
		Lines: []accounting.LineInput{ln(dr, amount, "0"), ln(cr, "0", amount)}}
}
