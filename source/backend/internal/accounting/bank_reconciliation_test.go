package accounting_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/google/uuid"

	"aciraba/internal/accounting"
)

func bankJournal(t *testing.T, e *env, date, bankDebit, bankCredit string) accounting.Journal {
	t.Helper()
	j, err := e.svc.SaveDraft(context.Background(), e.owner, uuid.Nil, accounting.JournalInput{
		Date: date, Type: "JU", Narration: "Mutasi bank uji",
		Lines: []accounting.LineInput{
			ln(e.acc["1120"], bankDebit, bankCredit),
			ln(e.acc["4100"], bankCredit, bankDebit),
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	posted, err := e.svc.Post(context.Background(), e.owner, j.ID)
	if err != nil {
		t.Fatal(err)
	}
	return posted
}

func TestBankReconciliationMatchesAndCompletes(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	bankJournal(t, e, "2024-03-05", "100000", "0")
	bankJournal(t, e, "2024-03-10", "0", "25000")
	bankJournal(t, e, "2024-03-15", "50000", "0")
	bankJournal(t, e, "2024-04-01", "90000", "0")
	if _, err := e.svc.StartBankReconciliation(ctx, e.owner, accounting.BankReconciliationInput{
		AccountID: e.acc["1120"], Month: "2024-03", StatementOpening: json.Number(""), StatementClosing: json.Number("75000"),
	}); err == nil {
		t.Fatal("saldo awal rekening koran kosong harus ditolak")
	}

	recon, err := e.svc.StartBankReconciliation(ctx, e.owner, accounting.BankReconciliationInput{
		AccountID: e.acc["1120"], Month: "2024-03", StatementOpening: json.Number("0"), StatementClosing: json.Number("75000"),
	})
	if err != nil {
		t.Fatal(err)
	}
	if recon.Status != "draft" || len(recon.Lines) != 3 || recon.BookClosing != "125000.00" || recon.Balanced {
		t.Fatalf("rekonsiliasi awal: %+v", recon)
	}
	if _, err := e.svc.GetBankReconciliation(ctx, e.otherOwner, recon.ID); !errors.Is(err, accounting.ErrNotFound) {
		t.Fatalf("tenant lain dapat membaca rekonsiliasi: %v", err)
	}

	matched := []uuid.UUID{recon.Lines[0].LineID, recon.Lines[1].LineID}
	updated, err := e.svc.UpdateBankReconciliation(ctx, e.owner, recon.ID, accounting.BankReconciliationUpdate{
		StatementOpening: json.Number("0"), StatementClosing: json.Number("75000"), MatchedLineIDs: matched,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !updated.Balanced || updated.Difference != "0.00" || updated.AdjustedBank != "125000.00" || updated.OutstandingDebit != "50000.00" || updated.OutstandingCredit != "0.00" {
		t.Fatalf("hasil pencocokan: %+v", updated)
	}
	if !updated.Lines[0].Matched || !updated.Lines[1].Matched || updated.Lines[2].Matched || updated.Lines[2].Debit != "50000.00" {
		t.Fatalf("status cocok/outstanding tak sesuai urutan: %+v", updated.Lines)
	}

	completed, err := e.svc.CompleteBankReconciliation(ctx, e.owner, recon.ID)
	if err != nil {
		t.Fatal(err)
	}
	if completed.Status != "completed" || completed.CompletedAt == nil {
		t.Fatalf("status final: %+v", completed)
	}
	if _, err := e.svc.UpdateBankReconciliation(ctx, e.owner, recon.ID, accounting.BankReconciliationUpdate{
		StatementOpening: json.Number("0"), StatementClosing: json.Number("75000"), MatchedLineIDs: matched,
	}); !errors.Is(err, accounting.ErrBankReconCompleted) {
		t.Fatalf("ubah rekonsiliasi selesai: %v", err)
	}
	if _, err := e.svc.StartBankReconciliation(ctx, e.owner, accounting.BankReconciliationInput{
		AccountID: e.acc["1120"], Month: "2024-03", StatementOpening: json.Number("0"), StatementClosing: json.Number("75000"),
	}); !errors.Is(err, accounting.ErrBankReconCompleted) {
		t.Fatalf("mulai ulang periode selesai: %v", err)
	}
}

func TestBankReconciliationCannotCompleteWithDifference(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	bankJournal(t, e, "2024-03-05", "100000", "0")
	recon, err := e.svc.StartBankReconciliation(ctx, e.owner, accounting.BankReconciliationInput{
		AccountID: e.acc["1120"], Month: "2024-03", StatementOpening: json.Number("0"), StatementClosing: json.Number("99000"),
	})
	if err != nil {
		t.Fatal(err)
	}
	updated, err := e.svc.UpdateBankReconciliation(ctx, e.owner, recon.ID, accounting.BankReconciliationUpdate{
		StatementOpening: json.Number("0"), StatementClosing: json.Number("99000"), MatchedLineIDs: []uuid.UUID{recon.Lines[0].LineID},
	})
	if err != nil {
		t.Fatal(err)
	}
	if updated.Balanced || updated.Difference != "-1000.00" {
		t.Fatalf("selisih tidak terdeteksi: %+v", updated)
	}
	if _, err := e.svc.CompleteBankReconciliation(ctx, e.owner, recon.ID); !errors.Is(err, accounting.ErrBankReconUnbalanced) {
		t.Fatalf("selesaikan dengan selisih: %v", err)
	}
}

func TestBankReconciliationCarriesOutstandingIntoNextMonth(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	bankJournal(t, e, "2024-03-20", "50000", "0")

	march, err := e.svc.StartBankReconciliation(ctx, e.owner, accounting.BankReconciliationInput{
		AccountID: e.acc["1120"], Month: "2024-03", StatementOpening: json.Number("0"), StatementClosing: json.Number("0"),
	})
	if err != nil {
		t.Fatal(err)
	}
	if !march.Balanced || march.OutstandingDebit != "50000.00" {
		t.Fatalf("setoran bulan Maret: %+v", march)
	}
	if _, err := e.svc.CompleteBankReconciliation(ctx, e.owner, march.ID); err != nil {
		t.Fatal(err)
	}

	april, err := e.svc.StartBankReconciliation(ctx, e.owner, accounting.BankReconciliationInput{
		AccountID: e.acc["1120"], Month: "2024-04", StatementOpening: json.Number("0"), StatementClosing: json.Number("50000"),
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(april.Lines) != 1 || april.Lines[0].Date != "2024-03-20" || april.Lines[0].Matched || april.BookOpening != "50000.00" {
		t.Fatalf("outstanding tidak dibawa ke April: %+v", april)
	}
	matched, err := e.svc.UpdateBankReconciliation(ctx, e.owner, april.ID, accounting.BankReconciliationUpdate{
		StatementOpening: json.Number("0"), StatementClosing: json.Number("50000"), MatchedLineIDs: []uuid.UUID{april.Lines[0].LineID},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !matched.Balanced || matched.OutstandingDebit != "0.00" || matched.BookClosing != "50000.00" {
		t.Fatalf("setoran yang sudah masuk bank: %+v", matched)
	}
	may, err := e.svc.StartBankReconciliation(ctx, e.owner, accounting.BankReconciliationInput{
		AccountID: e.acc["1120"], Month: "2024-05", StatementOpening: json.Number("50000"), StatementClosing: json.Number("50000"),
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(may.Lines) != 0 || !may.Balanced || may.BookOpening != "50000.00" {
		t.Fatalf("transaksi yang sudah dicocokkan terulang pada Mei: %+v", may)
	}
}
