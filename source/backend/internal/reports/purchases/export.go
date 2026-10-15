package purchases

// Ekspor laporan pembelian: keempat laporan membangun satu xtable.Table
// (renderer XLSX/PDF bersama, lihat internal/reports/xtable).

import (
	"fmt"
	"strings"

	"github.com/shopspring/decimal"

	"aciraba/internal/reports/xtable"
)

type table = xtable.Table

func payLabel(t string) string {
	if t == "credit" {
		return "Kredit"
	}
	return "Tunai"
}

func statusMark(st string) string {
	if st == "void" {
		return "BATAL"
	}
	return ""
}

// kop = judul, usaha, dan baris keterangan dari Header.
func kop(title string, h Header, extra ...string) (string, string, []string) {
	outlet := h.OutletName
	if h.OutletCode != "" {
		outlet = h.OutletCode + " - " + h.OutletName
	}
	meta := []string{
		fmt.Sprintf("Periode: %s s.d. %s", xtable.PDFDate(h.From), xtable.PDFDate(h.To)),
		"Outlet: " + outlet,
		"Nominal Harga Dalam Rupiah (IDR)",
	}
	if h.SupplierName != "" {
		meta = append(meta, "Pemasok: "+h.SupplierName)
	}
	for _, e := range extra {
		if e != "" {
			meta = append(meta, e)
		}
	}
	return title, strings.ToUpper(h.TenantName), meta
}

func truncNote(truncated bool, n int, unit string) string {
	if !truncated {
		return ""
	}
	return fmt.Sprintf("Catatan: hasil dipotong pada %d %s; persempit rentang tanggal.", n, unit)
}

// ── Tabel per laporan ────────────────────────────────────────────────────────

// InvoiceFileName nama berkas per faktur / per faktur detail.
func InvoiceFileName(from, to string, detail bool, ext string) string {
	kind := "perfaktur"
	if detail {
		kind = "perfaktur-detail"
	}
	return fmt.Sprintf("pembelian-%s_%s_%s.%s", kind, from, to, ext)
}

// ItemFileName nama berkas per barang.
func ItemFileName(from, to, ext string) string {
	return fmt.Sprintf("pembelian-perbarang_%s_%s.%s", from, to, ext)
}

// ByDateFileName nama berkas per tanggal.
func ByDateFileName(from, to, ext string) string {
	return fmt.Sprintf("pembelian-pertanggal_%s_%s.%s", from, to, ext)
}

// InvoiceTable: satu baris per faktur.
func InvoiceTable(res InvoiceResult) table {
	title, tenant, meta := kop("LAPORAN PEMBELIAN PER FAKTUR", res.Header, statusLine(res.StatusLabel))
	t := table{Title: title, Tenant: tenant, Meta: meta, Note: truncNote(res.Truncated, len(res.Data), "faktur"),
		Cols: []xtable.Column{
			xtable.Col("Tanggal", 22, 12, false), xtable.Col("Faktur", 38, 22, false), xtable.Col("Faktur Pemasok", 36, 20, false),
			xtable.Col("Pemasok", 50, 28, false), xtable.Col("Bayar", 18, 9, false), xtable.Col("Jatuh Tempo", 22, 12, false),
			xtable.Col("Status", 16, 9, false), xtable.Col("Subtotal", 32, 16, true), xtable.Col("PPN", 28, 14, true),
			xtable.Col("Biaya Lain", 28, 14, true), xtable.Col("Total", 49, 16, true),
		}}
	for _, x := range res.Data {
		t.Rows = append(t.Rows, []string{xtable.PDFDate(x.Date), x.DocNo, x.SupplierInvoiceNo, x.SupplierName,
			payLabel(x.PaymentType), xtable.PDFDate(x.DueDate), statusMark(x.Status),
			x.Subtotal, x.TaxAmount, x.OtherCost, x.Total})
	}
	if res.HasSummary {
		s := res.Summary
		t.Total = []string{"GRAND TOTAL", fmt.Sprintf("%d faktur", s.PurchaseCount), "", "", "", "",
			fmt.Sprintf("batal %d", s.VoidCount), s.Subtotal, s.TaxAmount, s.OtherCost, s.Total}
	}
	return t
}

// InvoiceDetailTable: satu baris per barang (data faktur diulang di tiap baris).
func InvoiceDetailTable(res InvoiceResult) table {
	title, tenant, meta := kop("LAPORAN PEMBELIAN PER FAKTUR DETAIL", res.Header, statusLine(res.StatusLabel))
	t := table{Title: title, Tenant: tenant, Meta: meta, Note: truncNote(res.Truncated, len(res.Data), "faktur"),
		Cols: []xtable.Column{
			xtable.Col("Tanggal", 22, 12, false), xtable.Col("Faktur", 36, 22, false), xtable.Col("Pemasok", 44, 28, false),
			xtable.Col("Bayar", 16, 9, false), xtable.Col("SKU", 28, 14, false), xtable.Col("Barang", 62, 36, false),
			xtable.Col("Satuan", 16, 9, false), xtable.Col("Qty", 20, 10, false), xtable.Col("Harga", 28, 14, true),
			xtable.Col("Diskon %", 28, 14, false), xtable.Col("Jumlah", 39, 16, true),
		}}
	for _, x := range res.Data {
		doc := x.DocNo
		if x.Status == "void" {
			doc += " (BATAL)"
		}
		for _, l := range x.Lines {
			t.Rows = append(t.Rows, []string{xtable.PDFDate(x.Date), doc, x.SupplierName, payLabel(x.PaymentType),
				l.SKU, l.Name, l.Unit, l.Qty, l.UnitPrice, discText(l), l.LineTotal})
		}
	}
	if res.HasSummary {
		t.Total = []string{"GRAND TOTAL", fmt.Sprintf("%d faktur", res.Summary.PurchaseCount), "", "", "", "", "", "", "", "", res.Summary.Subtotal}
	}
	return t
}

func statusLine(label string) string {
	if label == "" {
		return ""
	}
	return "Status: " + label
}

// discText: diskon bertingkat "10+5" (tingkat 0 dilewati).
func discText(l LineRow) string {
	var parts []string
	for _, d := range []string{l.Disc1, l.Disc2, l.Disc3, l.Disc4} {
		if v, err := decimal.NewFromString(d); err == nil && !v.IsZero() {
			parts = append(parts, v.String())
		}
	}
	return strings.Join(parts, "+")
}

// ItemTable: satu baris per barang.
func ItemTable(res ItemResult) table {
	title, tenant, meta := kop("LAPORAN PEMBELIAN PER BARANG", res.Header)
	t := table{Title: title, Tenant: tenant, Meta: meta,
		Note: truncNote(res.Truncated, len(res.Data), "barang"),
		Cols: []xtable.Column{
			xtable.Col("No", 12, 6, false), xtable.Col("SKU", 34, 16, false), xtable.Col("Barang", 100, 40, false), xtable.Col("Satuan", 20, 10, false),
			xtable.Col("Faktur", 18, 9, false), xtable.Col("Qty", 28, 12, false), xtable.Col("Harga Rata-rata", 38, 16, true),
			xtable.Col("Harga Terakhir", 38, 16, true), xtable.Col("Total", 51, 18, true),
		}}
	for i, x := range res.Data {
		t.Rows = append(t.Rows, []string{fmt.Sprintf("%d", i+1), x.SKU, x.Name, x.Unit,
			fmt.Sprintf("%d", x.PurchaseCount), x.Qty, x.AvgPrice, x.LastPrice, x.Total})
	}
	t.Total = []string{"", "GRAND TOTAL", fmt.Sprintf("%d barang", res.Summary.ItemCount), "", "", "", "", "", res.Summary.Total}
	return t
}

// ByDateTable: satu baris per tanggal.
func ByDateTable(res ByDateResult) table {
	title, tenant, meta := kop("LAPORAN PEMBELIAN PER TANGGAL", res.Header)
	t := table{Title: title, Tenant: tenant, Meta: meta,
		Cols: []xtable.Column{
			xtable.Col("Tanggal", 24, 12, false), xtable.Col("Faktur", 16, 8, false), xtable.Col("Batal", 14, 7, false),
			xtable.Col("Subtotal", 30, 16, true), xtable.Col("PPN", 26, 14, true), xtable.Col("Biaya Lain", 26, 14, true),
			xtable.Col("Total", 32, 16, true), xtable.Col("Tunai", 30, 16, true), xtable.Col("Kredit", 30, 16, true),
			xtable.Col("Retur", 14, 7, false), xtable.Col("Total Retur", 30, 16, true), xtable.Col("Bersih", 67, 16, true),
		}}
	add := func(label string, r DayRow) []string {
		return []string{label, fmt.Sprintf("%d", r.PurchaseCount), fmt.Sprintf("%d", r.VoidCount),
			r.Subtotal, r.TaxAmount, r.OtherCost, r.Total, r.CashTotal, r.CreditTotal,
			fmt.Sprintf("%d", r.ReturnCount), r.ReturnTotal, r.NetTotal}
	}
	for _, x := range res.Data {
		t.Rows = append(t.Rows, add(xtable.PDFDate(x.Date), x))
	}
	t.Total = add("GRAND TOTAL", res.Totals)
	return t
}
