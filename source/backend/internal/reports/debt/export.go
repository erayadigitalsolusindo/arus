package debt

// Ekspor laporan hutang / piutang: Excel dan PDF lewat renderer bersama xtable.

import (
	"fmt"
	"strings"

	"aciraba/internal/reports/xtable"
)

type table = xtable.Table

// labels = istilah per jenis.
type labels struct {
	title, file, party, doc, unit string
}

var kindLabels = map[Kind]labels{
	KindPayable:    {"HUTANG", "hutang", "Pemasok", "Faktur", "hutang"},
	KindReceivable: {"PIUTANG", "piutang", "Member", "Nota", "piutang"},
}

// View = nama laporan (dipakai nama berkas & rute).
const (
	ViewOutstanding = "outstanding"
	ViewPayments    = "payments"
	ViewBalances    = "balances"
	ViewLedger      = "ledger"
	ViewCredit      = "credit"
)

// FileName nama berkas ekspor.
func FileName(k Kind, view, stamp, ext string) string {
	if view == ViewCredit {
		return fmt.Sprintf("saldo-di-pemasok_%s.%s", stamp, ext)
	}
	return fmt.Sprintf("%s-%s_%s.%s", kindLabels[k].file, view, stamp, ext)
}

func (h Header) outletLine() string {
	if h.OutletCode != "" {
		return h.OutletCode + " - " + h.OutletName
	}
	return h.OutletName
}

func period(h Header) string {
	return fmt.Sprintf("Periode: %s s.d. %s", xtable.PDFDate(h.From), xtable.PDFDate(h.To))
}

func truncNote(truncated bool, n int) string {
	if !truncated {
		return ""
	}
	return fmt.Sprintf("Catatan: hasil dipotong pada %d baris; persempit filter.", n)
}

func agingLine(b Buckets) string {
	f := func(s string) string { return xtable.PDFMoney(s) }
	return fmt.Sprintf("Umur: belum jatuh tempo %s | 1-30 hari %s | 31-60 hari %s | 61-90 hari %s | >90 hari %s",
		f(b.Current), f(b.D1To30), f(b.D31To60), f(b.D61To90), f(b.Over90))
}

func dateOrDash(s string) string {
	if s == "" {
		return "-"
	}
	return xtable.PDFDate(s)
}

// OutstandingTable: satu baris per dokumen yang masih punya sisa.
func OutstandingTable(k Kind, res OutstandingResult) table {
	l := kindLabels[k]
	h := res.Header
	meta := []string{"Posisi per tanggal: " + xtable.PDFDate(h.AsOf), "Outlet: " + h.outletLine(), "Nominal Harga Dalam Rupiah (IDR)"}
	if h.PartyName != "" {
		meta = append(meta, l.party+": "+h.PartyName)
	}
	if res.HasSummary {
		meta = append(meta, agingLine(res.Summary.Buckets))
	}
	t := table{Title: "LAPORAN " + l.title + " BEREDAR", Tenant: strings.ToUpper(h.TenantName), Meta: meta, Note: truncNote(res.Truncated, len(res.Data))}
	t.Cols = []xtable.Column{
		xtable.Col("Tanggal", 22, 12, false), xtable.Col("No "+l.doc, 38, 24, false), xtable.Col("Referensi", 32, 20, false),
		xtable.Col(l.party, 56, 28, false), xtable.Col("Jatuh Tempo", 24, 13, false), xtable.Col("Total", 32, 16, true),
		xtable.Col("Dibayar", 32, 16, true), xtable.Col("Retur", 28, 14, true), xtable.Col("Sisa", 32, 16, true),
		xtable.Col("Terlambat", 22, 11, false),
	}
	for _, x := range res.Data {
		late := "-"
		if x.DaysOverdue > 0 {
			late = fmt.Sprintf("%d hari", x.DaysOverdue)
		}
		t.Rows = append(t.Rows, []string{xtable.PDFDate(x.DocDate), x.DocNo, x.RefNo, x.PartyName, dateOrDash(x.DueDate),
			x.Amount, x.Paid, x.Returned, x.Balance, late})
	}
	if res.HasSummary {
		s := res.Summary
		t.Total = []string{"GRAND TOTAL", fmt.Sprintf("%d dokumen", s.DocCount), "", "", "", s.Amount, s.Paid, s.Returned, s.Balance, ""}
	}
	return t
}

// BalanceTable: satu baris per pihak dengan sisa menurut umur.
func BalanceTable(k Kind, res BalanceResult) table {
	l := kindLabels[k]
	h := res.Header
	meta := []string{"Posisi per tanggal: " + xtable.PDFDate(h.AsOf), "Outlet: " + h.outletLine(), "Nominal Harga Dalam Rupiah (IDR)"}
	if res.HasSummary {
		meta = append(meta, agingLine(res.Summary.Buckets))
	}
	t := table{Title: "LAPORAN SALDO " + l.title + " " + strings.ToUpper(l.party), Tenant: strings.ToUpper(h.TenantName), Meta: meta,
		Note: truncNote(res.Truncated, len(res.Data))}
	t.Cols = []xtable.Column{
		xtable.Col("Kode", 20, 12, false), xtable.Col(l.party, 54, 30, false), xtable.Col("Dok", 12, 7, false),
		xtable.Col("Sisa", 30, 16, true), xtable.Col("Belum JT", 28, 15, true), xtable.Col("1-30 hr", 26, 14, true),
		xtable.Col("31-60 hr", 26, 14, true), xtable.Col("61-90 hr", 26, 14, true), xtable.Col(">90 hr", 26, 14, true),
		xtable.Col("Total Awal", 30, 16, true), xtable.Col("Dibayar", 30, 16, true), xtable.Col("JT Tertua", 21, 13, false),
	}
	for _, x := range res.Data {
		t.Rows = append(t.Rows, []string{x.Code, x.Name, fmt.Sprint(x.DocCount), x.Balance, x.Buckets.Current, x.Buckets.D1To30,
			x.Buckets.D31To60, x.Buckets.D61To90, x.Buckets.Over90, x.Amount, x.Paid, dateOrDash(x.OldestDue)})
	}
	if res.HasSummary {
		s := res.Summary
		b := s.Buckets
		t.Total = []string{"GRAND TOTAL", fmt.Sprintf("%d pihak", s.PartyCount), fmt.Sprint(s.DocCount), s.Balance, b.Current, b.D1To30,
			b.D31To60, b.D61To90, b.Over90, s.Amount, s.Paid, ""}
	}
	return t
}

// PaymentTable: satu baris per pembayaran.
func PaymentTable(k Kind, res PaymentResult) table {
	l := kindLabels[k]
	h := res.Header
	meta := []string{period(h), "Outlet: " + h.outletLine(), "Nominal Harga Dalam Rupiah (IDR)"}
	if h.PartyName != "" {
		meta = append(meta, l.party+": "+h.PartyName)
	}
	if res.HasSummary && len(res.Summary.ByMethod) > 0 {
		parts := make([]string, 0, len(res.Summary.ByMethod))
		for _, m := range res.Summary.ByMethod {
			parts = append(parts, fmt.Sprintf("%s %s", m.Name, xtable.PDFMoney(m.Amount)))
		}
		meta = append(meta, "Per metode: "+strings.Join(parts, " | "))
	}
	t := table{Title: "LAPORAN PEMBAYARAN " + l.title, Tenant: strings.ToUpper(h.TenantName), Meta: meta, Note: truncNote(res.Truncated, len(res.Data))}
	t.Cols = []xtable.Column{
		xtable.Col("Tanggal", 22, 12, false), xtable.Col("Jam", 12, 7, false), xtable.Col("No Bayar", 38, 24, false),
		xtable.Col(l.doc+" Dibayar", 38, 24, false), xtable.Col(l.party, 50, 28, false), xtable.Col("Metode", 30, 16, false),
		xtable.Col("Referensi", 28, 16, false), xtable.Col("Jumlah", 32, 16, true), xtable.Col("Pelunasan", 34, 20, false),
		xtable.Col("Diterima Oleh", 55, 18, false),
	}
	if k == KindReceivable {
		t.Cols = append(t.Cols[:8], append([]xtable.Column{xtable.Col("Biaya", 24, 14, true)}, t.Cols[8:]...)...)
		t.Cols[9].Width = 20
		t.Cols[len(t.Cols)-1].Width = 33
	}
	for _, x := range res.Data {
		row := []string{xtable.PDFDate(x.Date), x.Time, x.DocNo, x.TargetDocNo, x.PartyName, x.Method, x.RefNo, x.Amount}
		if k == KindReceivable {
			row = append(row, x.Fee)
		}
		row = append(row, x.SettlementNo, x.UserName)
		t.Rows = append(t.Rows, row)
	}
	if res.HasSummary {
		s := res.Summary
		row := []string{"GRAND TOTAL", "", fmt.Sprintf("%d bayar", s.Count), "", "", "", "", s.Amount}
		if k == KindReceivable {
			row = append(row, s.Fee)
		}
		t.Total = append(row, "", "")
	}
	return t
}

var eventLabel = map[string]string{EventInvoice: "Tagihan", EventPayment: "Pembayaran", EventReturn: "Retur"}

// LedgerTable: buku bantu satu pihak, dimulai baris saldo awal.
func LedgerTable(k Kind, res LedgerResult) table {
	l := kindLabels[k]
	h := res.Header
	meta := []string{period(h), "Outlet: " + h.outletLine(), "Nominal Harga Dalam Rupiah (IDR)", l.party + ": " + strings.TrimSpace(h.PartyCode+" "+h.PartyName)}
	t := table{Title: "BUKU BANTU " + l.title, Tenant: strings.ToUpper(h.TenantName), Meta: meta, Note: truncNote(res.Truncated, len(res.Data))}
	t.Cols = []xtable.Column{
		xtable.Col("Tanggal", 24, 12, false), xtable.Col("Jenis", 26, 13, false), xtable.Col("No Dokumen", 42, 24, false),
		xtable.Col("Referensi", 42, 24, false), xtable.Col("Keterangan", 58, 28, false), xtable.Col("Bertambah", 38, 17, true),
		xtable.Col("Berkurang", 38, 17, true), xtable.Col("Saldo", 41, 17, true),
	}
	if res.HasSummary {
		t.Rows = append(t.Rows, []string{xtable.PDFDate(h.From), "Saldo awal", "", "", "", "", "", res.Summary.Opening})
	}
	for _, x := range res.Data {
		t.Rows = append(t.Rows, []string{xtable.PDFDate(x.Date), eventLabel[x.Type], x.DocNo, x.RefNo, x.Memo, x.Increase, x.Decrease, x.Balance})
	}
	if res.HasSummary {
		s := res.Summary
		t.Total = []string{"SALDO AKHIR", "", "", "", "", s.Increase, s.Decrease, s.Closing}
	}
	return t
}

// CreditTable: saldo kredit di pemasok per pemasok.
func CreditTable(res CreditResult) table {
	h := res.Header
	t := table{Title: "LAPORAN SALDO DI PEMASOK", Tenant: strings.ToUpper(h.TenantName),
		Meta: []string{period(h), "Kredit toko di pemasok dari dana kembali retur pembelian", "Nominal Harga Dalam Rupiah (IDR)"},
		Note: truncNote(res.Truncated, len(res.Data))}
	t.Cols = []xtable.Column{
		xtable.Col("Kode", 24, 12, false), xtable.Col("Pemasok", 70, 32, false), xtable.Col("Saldo Awal", 38, 17, true),
		xtable.Col("Retur Masuk", 38, 17, true), xtable.Col("Dipakai Bayar Hutang", 38, 20, true), xtable.Col("Dicairkan", 38, 17, true),
		xtable.Col("Batal Retur", 38, 17, true), xtable.Col("Saldo Akhir", 38, 17, true),
	}
	for _, x := range res.Data {
		t.Rows = append(t.Rows, []string{x.Code, x.Name, x.Opening, x.Added, x.Used, x.CashedOut, x.Voided, x.Closing})
	}
	if res.HasSummary {
		s := res.Summary
		t.Total = []string{"GRAND TOTAL", fmt.Sprintf("%d pemasok bersaldo", s.SupplierCount), s.Opening, s.Added, s.Used, s.CashedOut, s.Voided, s.Closing}
	}
	return t
}
