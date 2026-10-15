package returns

// Ekspor laporan retur: Excel dan PDF lewat renderer bersama xtable.

import (
	"fmt"
	"strings"

	"aciraba/internal/reports/xtable"
)

type table = xtable.Table

// labels = istilah per jenis retur.
type labels struct {
	title, file, party, ref, user, offset string
}

var kindLabels = map[Kind]labels{
	KindSale:     {"LAPORAN RETUR PENJUALAN", "retur-penjualan", "Pelanggan", "Nota Asal", "Kasir", "Potong Piutang"},
	KindPurchase: {"LAPORAN RETUR PEMBELIAN", "retur-pembelian", "Pemasok", "Faktur Asal", "Dibuat Oleh", "Potong Hutang"},
}

// FileName nama berkas ekspor.
func FileName(k Kind, from, to string, detail bool, ext string) string {
	kind := "perfaktur"
	if detail {
		kind = "perfaktur-detail"
	}
	return fmt.Sprintf("%s-%s_%s_%s.%s", kindLabels[k].file, kind, from, to, ext)
}

func statusMark(st string) string {
	if st == "void" {
		return "BATAL"
	}
	return ""
}

// kop = judul, usaha, dan baris keterangan dari Header (+ rasio retur bila ada ringkasan).
func kop(k Kind, suffix string, res Result) (string, string, []string) {
	l := kindLabels[k]
	h := res.Header
	outlet := h.OutletName
	if h.OutletCode != "" {
		outlet = h.OutletCode + " - " + h.OutletName
	}
	meta := []string{
		fmt.Sprintf("Periode: %s s.d. %s", xtable.PDFDate(h.From), xtable.PDFDate(h.To)),
		"Outlet: " + outlet,
		"Nominal Harga Dalam Rupiah (IDR)",
	}
	if h.PartyName != "" {
		meta = append(meta, l.party+": "+h.PartyName)
	}
	if h.StatusLabel != "" {
		meta = append(meta, "Status: "+h.StatusLabel)
	}
	if h.SettleLabel != "" {
		meta = append(meta, "Penyelesaian: "+h.SettleLabel)
	}
	if res.HasSummary && res.Summary.BaseTotal != "0.00" {
		basis := "penjualan"
		if k == KindPurchase {
			basis = "pembelian"
		}
		meta = append(meta, fmt.Sprintf("Rasio retur: %s%% dari total %s periode (%s)",
			strings.ReplaceAll(res.Summary.RatioPct, ".", ","), basis, xtable.PDFMoney(res.Summary.BaseTotal)))
	}
	return l.title + suffix, strings.ToUpper(h.TenantName), meta
}

func truncNote(truncated bool, n int) string {
	if !truncated {
		return ""
	}
	return fmt.Sprintf("Catatan: hasil dipotong pada %d dokumen; persempit rentang tanggal.", n)
}

// InvoiceTable: satu baris per dokumen retur.
func InvoiceTable(k Kind, res Result) table {
	l := kindLabels[k]
	title, tenant, meta := kop(k, " PER FAKTUR", res)
	t := table{Title: title, Tenant: tenant, Meta: meta, Note: truncNote(res.Truncated, len(res.Data))}
	if k == KindSale {
		t.Cols = []xtable.Column{
			xtable.Col("Tanggal", 20, 12, false), xtable.Col("No Retur", 32, 22, false), xtable.Col(l.ref, 32, 22, false),
			xtable.Col(l.party, 36, 24, false), xtable.Col(l.user, 24, 16, false), xtable.Col("Status", 12, 8, false),
			xtable.Col("Subtotal", 26, 16, true), xtable.Col("Diskon", 22, 14, true), xtable.Col("PPN", 20, 14, true),
			xtable.Col("Biaya Lain", 20, 14, true), xtable.Col("Total", 26, 16, true), xtable.Col(l.offset, 26, 16, true),
			xtable.Col("Dikembalikan", 26, 16, true), xtable.Col("Metode", 17, 14, false),
		}
		for _, x := range res.Data {
			t.Rows = append(t.Rows, []string{xtable.PDFDate(x.Date), x.DocNo, x.RefDocNo, x.PartyName, x.UserName,
				statusMark(x.Status), x.Subtotal, x.Discount, x.TaxAmount, x.Surcharge, x.Total, x.Offset, x.Refund, x.RefundMethod})
		}
		if res.HasSummary {
			s := res.Summary
			t.Total = []string{"GRAND TOTAL", fmt.Sprintf("%d retur", s.ReturnCount), "", "", "",
				fmt.Sprintf("batal %d", s.VoidCount), s.Subtotal, s.Discount, s.TaxAmount, s.Surcharge, s.Total, s.Offset, s.Refund, ""}
		}
		return t
	}
	t.Cols = []xtable.Column{
		xtable.Col("Tanggal", 20, 12, false), xtable.Col("No Retur", 34, 22, false), xtable.Col(l.ref, 34, 22, false),
		xtable.Col(l.party, 50, 28, false), xtable.Col("Status", 14, 8, false), xtable.Col("Subtotal", 32, 16, true),
		xtable.Col("PPN", 28, 14, true), xtable.Col("Total", 32, 16, true), xtable.Col(l.offset, 32, 16, true),
		xtable.Col("Dikembalikan", 32, 16, true), xtable.Col("Metode", 31, 14, false),
	}
	for _, x := range res.Data {
		t.Rows = append(t.Rows, []string{xtable.PDFDate(x.Date), x.DocNo, x.RefDocNo, x.PartyName, statusMark(x.Status),
			x.Subtotal, x.TaxAmount, x.Total, x.Offset, x.Refund, x.RefundMethod})
	}
	if res.HasSummary {
		s := res.Summary
		t.Total = []string{"GRAND TOTAL", fmt.Sprintf("%d retur", s.ReturnCount), "", "", fmt.Sprintf("batal %d", s.VoidCount),
			s.Subtotal, s.TaxAmount, s.Total, s.Offset, s.Refund, ""}
	}
	return t
}

// DetailTable: satu baris per barang (data dokumen retur diulang di tiap baris).
func DetailTable(k Kind, res Result) table {
	l := kindLabels[k]
	title, tenant, meta := kop(k, " PER FAKTUR DETAIL", res)
	t := table{Title: title, Tenant: tenant, Meta: meta, Note: truncNote(res.Truncated, len(res.Data))}
	docLabel := func(x Row) string {
		if x.Status == "void" {
			return x.DocNo + " (BATAL)"
		}
		return x.DocNo
	}
	if k == KindSale {
		t.Cols = []xtable.Column{
			xtable.Col("Tanggal", 20, 12, false), xtable.Col("No Retur", 32, 22, false), xtable.Col(l.ref, 32, 22, false),
			xtable.Col(l.party, 36, 24, false), xtable.Col("SKU", 24, 14, false), xtable.Col("Barang", 56, 36, false),
			xtable.Col("Satuan", 14, 9, false), xtable.Col("Qty", 18, 10, false), xtable.Col("Nilai", 26, 14, true),
			xtable.Col("Diskon", 22, 14, true), xtable.Col("PPN", 20, 14, true), xtable.Col("Jumlah", 29, 16, true),
		}
		for _, x := range res.Data {
			for _, ln := range x.Lines {
				t.Rows = append(t.Rows, []string{xtable.PDFDate(x.Date), docLabel(x), x.RefDocNo, x.PartyName,
					ln.SKU, ln.Name, ln.Unit, ln.Qty, ln.Value, ln.Discount, ln.TaxAmount, ln.Net})
			}
		}
		if res.HasSummary {
			s := res.Summary
			t.Total = []string{"GRAND TOTAL", fmt.Sprintf("%d retur", s.ReturnCount), "", "", "", "", "", "",
				s.Subtotal, s.Discount, s.TaxAmount, sumMoney(s.Subtotal, "-"+s.Discount, s.TaxAmount)}
		}
		return t
	}
	t.Cols = []xtable.Column{
		xtable.Col("Tanggal", 20, 12, false), xtable.Col("No Retur", 34, 22, false), xtable.Col(l.ref, 34, 22, false),
		xtable.Col(l.party, 46, 28, false), xtable.Col("SKU", 28, 14, false), xtable.Col("Barang", 70, 36, false),
		xtable.Col("Satuan", 16, 9, false), xtable.Col("Qty", 20, 10, false), xtable.Col("Harga", 32, 14, true),
		xtable.Col("Jumlah", 39, 16, true),
	}
	for _, x := range res.Data {
		for _, ln := range x.Lines {
			t.Rows = append(t.Rows, []string{xtable.PDFDate(x.Date), docLabel(x), x.RefDocNo, x.PartyName,
				ln.SKU, ln.Name, ln.Unit, ln.Qty, ln.UnitPrice, ln.Net})
		}
	}
	if res.HasSummary {
		s := res.Summary
		t.Total = []string{"GRAND TOTAL", fmt.Sprintf("%d retur", s.ReturnCount), "", "", "", "", "", "", "", s.Subtotal}
	}
	return t
}
