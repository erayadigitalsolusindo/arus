package sales

// Ekspor laporan penjualan PER FAKTUR: Excel (.xlsx, dialirkan) dan PDF
// (landscape LEGAL) dengan kop yang sama seperti laporan penjualan lain.
// Rincian pembayaran ditulis satu baris per metode ("TUNAI 99.999,00" ⏎ "DEBIT ...").

import (
	"bytes"
	"fmt"
	"strings"

	"github.com/jung-kurt/gofpdf"
	"github.com/shopspring/decimal"
	"github.com/xuri/excelize/v2"
)

// maxInvoicePDFRows = batas faktur pada PDF (berkas dibuat di memori).
const maxInvoicePDFRows = 5000

// InvoiceFileName nama berkas ekspor per faktur.
func InvoiceFileName(from, to, ext string) string {
	return fmt.Sprintf("penjualan-perfaktur_%s_%s.%s", from, to, ext)
}

// invoiceMethodsText: satu baris per metode, nama kapital.
func invoiceMethodsText(ms []DetailMethod) string {
	lines := make([]string, 0, len(ms))
	for _, m := range ms {
		lines = append(lines, strings.ToUpper(m.Name)+" "+pdfMoney(m.Amount))
	}
	return strings.Join(lines, "\n")
}

// sumStr menjumlahkan dua string desimal (kolom Pajak = toko + negara).
func sumStr(a, b string) string {
	x, err1 := decimal.NewFromString(a)
	y, err2 := decimal.NewFromString(b)
	if err1 != nil || err2 != nil {
		return "0"
	}
	return x.Add(y).StringFixed(2)
}

// invoiceKop = judul, usaha, dan baris keterangan — dipakai Excel & PDF.
func invoiceKop(res InvoiceResult) (title, tenant string, meta []string) {
	outlet := res.OutletName
	if res.OutletCode != "" {
		outlet = res.OutletCode + " - " + res.OutletName
	}
	meta = []string{
		fmt.Sprintf("Periode: %s s.d. %s", pdfDate(res.From), pdfDate(res.To)),
		"Outlet: " + outlet,
		"Nominal Harga Dalam Rupiah (IDR)",
	}
	if res.CashierName != "" {
		meta = append(meta, "Kasir: "+res.CashierName)
	}
	if res.StatusLabel != "" {
		meta = append(meta, "Status: "+res.StatusLabel)
	}
	return "LAPORAN PENJUALAN PER FAKTUR", strings.ToUpper(res.TenantName), meta
}

// InvoiceToXLSX: kop + tabel; dialirkan per baris (memori tetap kecil).
func InvoiceToXLSX(res InvoiceResult) ([]byte, error) {
	f := excelize.NewFile()
	const sheet = "Per Faktur"
	if _, err := f.NewSheet(sheet); err != nil {
		return nil, err
	}
	f.DeleteSheet("Sheet1")
	sw, err := f.NewStreamWriter(sheet)
	if err != nil {
		return nil, err
	}
	bold, err := f.NewStyle(&excelize.Style{Font: &excelize.Font{Bold: true}})
	if err != nil {
		return nil, err
	}
	num, err := f.NewStyle(&excelize.Style{NumFmt: 4})
	if err != nil {
		return nil, err
	}
	wrap, err := f.NewStyle(&excelize.Style{Alignment: &excelize.Alignment{WrapText: true, Vertical: "top"}})
	if err != nil {
		return nil, err
	}
	widths := []float64{12, 7, 22, 9, 10, 16, 16, 16, 15, 13, 13, 13, 12, 16, 16, 13, 15, 28}
	for i, w := range widths {
		if err := sw.SetColWidth(i+1, i+1, w); err != nil {
			return nil, err
		}
	}

	r := 1
	put := func(vals ...any) error {
		cell, _ := excelize.CoordinatesToCellName(1, r)
		r++
		return sw.SetRow(cell, vals)
	}
	title, tenant, meta := invoiceKop(res)
	if err := put(excelize.Cell{StyleID: bold, Value: title}); err != nil {
		return nil, err
	}
	if tenant != "" {
		if err := put(excelize.Cell{StyleID: bold, Value: tenant}); err != nil {
			return nil, err
		}
	}
	for _, m := range meta {
		if err := put(m); err != nil {
			return nil, err
		}
	}
	if res.Truncated {
		if err := put(fmt.Sprintf("Catatan: hasil dipotong pada %d faktur; persempit rentang tanggal.", len(res.Data))); err != nil {
			return nil, err
		}
	}
	if err := put(); err != nil {
		return nil, err
	}

	headers := []string{"Tanggal", "Jam", "Faktur", "Status", "Outlet", "Kasir", "Member", "Salesperson",
		"Subtotal", "Diskon", "Pajak Toko", "Pajak Negara", "Biaya Lain", "Total", "Diterima",
		"Kembalian", "Piutang", "Pembayaran"}
	hrow := make([]any, len(headers))
	for i, h := range headers {
		hrow[i] = excelize.Cell{StyleID: bold, Value: h}
	}
	if err := put(hrow...); err != nil {
		return nil, err
	}
	money := func(s string) any { return excelize.Cell{StyleID: num, Value: moneyFloat(s)} }
	for _, x := range res.Data {
		status := x.Status
		if status == "completed" {
			status = ""
		}
		if err := put(
			x.Date, x.Time, x.DocNo, status, x.OutletCode, x.CashierName, x.MemberName, x.Salesperson,
			money(x.Subtotal), money(x.Discount), money(x.TaxStore), money(x.TaxGov), money(x.OtherCost),
			money(x.Total), money(x.Paid), money(x.Change), money(x.Receivable),
			excelize.Cell{StyleID: wrap, Value: invoiceMethodsText(x.Methods)},
		); err != nil {
			return nil, err
		}
	}
	if res.HasSummary {
		if err := put(); err != nil {
			return nil, err
		}
		if err := put(excelize.Cell{StyleID: bold, Value: "GRAND TOTAL"}, "", "", "", "", "", "", "",
			money(res.Summary.Subtotal), money(res.Summary.Discount), money(res.Summary.TaxStore),
			money(res.Summary.TaxGov), "", money(res.Summary.Total), money(res.Summary.Paid), "",
			money(res.Summary.Receivable)); err != nil {
			return nil, err
		}
	}
	if err := sw.Flush(); err != nil {
		return nil, err
	}
	buf, err := f.WriteToBuffer()
	if err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// InvoiceToPDF: landscape LEGAL — kop, ringkasan, tabel faktur (kolom Pembayaran
// satu baris per metode), grand total. Lebar tabel 339 mm.
func InvoiceToPDF(res InvoiceResult) ([]byte, error) {
	pdf := gofpdf.New("L", "mm", "LEGAL", "")
	pdf.SetTitle("Laporan Penjualan Per Faktur "+res.From+" - "+res.To, false)
	pdf.SetAuthor("ARUS", false)
	pdf.SetMargins(8, 6, 8)
	pdf.SetAutoPageBreak(false, 0)
	pdf.AddPage()

	title, tenant, meta := invoiceKop(res)
	pdf.SetFont("Helvetica", "B", 13)
	pdf.CellFormat(0, 6, title, "", 1, "C", false, 0, "")
	if tenant != "" {
		pdf.SetFont("Helvetica", "B", 10)
		pdf.CellFormat(0, 5, tenant, "", 1, "C", false, 0, "")
	}
	pdf.SetFont("Helvetica", "", 9)
	for _, line := range meta {
		pdf.CellFormat(0, 4.5, line, "", 1, "L", false, 0, "")
	}
	pdf.Ln(1)

	if res.HasSummary {
		pdf.SetFont("Helvetica", "B", 9)
		pdf.CellFormat(0, 5, "Ringkasan", "", 1, "L", false, 0, "")
		pdf.SetFont("Helvetica", "", 8.5)
		for _, line := range [][2]string{
			{"Total Faktur", fmt.Sprintf("%d", res.Summary.SaleCount)},
			{"Total Belanja", pdfMoney(res.Summary.Total)},
			{"Diskon", pdfMoney(res.Summary.Discount)},
			{"Pajak Toko", pdfMoney(res.Summary.TaxStore)},
			{"Pajak Negara", pdfMoney(res.Summary.TaxGov)},
			{"Total Diterima", pdfMoney(res.Summary.Paid)},
			{"Total Piutang", pdfMoney(res.Summary.Receivable)},
			{"Faktur Batal", fmt.Sprintf("%d", res.Summary.VoidCount)},
		} {
			pdf.CellFormat(55, 4.5, line[0], "", 0, "L", false, 0, "")
			pdf.CellFormat(0, 4.5, ": "+line[1], "", 1, "R", false, 0, "")
		}
	}
	if res.Truncated {
		pdf.SetFont("Helvetica", "B", 8)
		pdf.CellFormat(0, 4.5, fmt.Sprintf("Catatan: hasil dipotong pada %d faktur; persempit rentang tanggal.", len(res.Data)),
			"", 1, "L", false, 0, "")
	}
	pdf.Ln(1)

	type col struct {
		head  string
		w     float64
		align string
	}
	cols := []col{
		{"Tanggal", 26, "L"}, {"Faktur", 36, "L"}, {"Kasir", 30, "L"}, {"Member", 30, "L"},
		{"Subtotal", 30, "R"}, {"Diskon", 24, "R"}, {"Pajak", 26, "R"}, {"Total", 32, "R"},
		{"Piutang", 28, "R"}, {"Pembayaran", 77, "L"},
	}
	header := func() {
		pdf.SetFont("Helvetica", "B", 7.5)
		pdf.SetFillColor(235, 235, 235)
		for _, c := range cols {
			pdf.CellFormat(c.w, 6, c.head, "1", 0, c.align, true, 0, "")
		}
		pdf.Ln(-1)
		pdf.SetFont("Helvetica", "", 7.5)
	}
	needPage := func(h float64) {
		if pdf.GetY()+h > 200 {
			pdf.AddPage()
			header()
		}
	}
	header()

	const lineH = 4.0
	for _, x := range res.Data {
		h := 5.0
		if n := float64(len(x.Methods))*lineH + 1; n > h {
			h = n
		}
		needPage(h)
		doc := x.DocNo
		if x.Status == "void" {
			doc += " (BATAL)"
		}
		cells := []string{pdfDate(x.Date) + " " + x.Time, doc, x.CashierName, x.MemberName,
			pdfMoney(x.Subtotal), pdfMoney(x.Discount), pdfMoney(sumStr(x.TaxStore, x.TaxGov)),
			pdfMoney(x.Total), pdfMoney(x.Receivable)}
		for j, txt := range cells {
			pdf.CellFormat(cols[j].w, h, txt, "1", 0, cols[j].align, false, 0, "")
		}
		// Kolom Pembayaran: satu baris per metode (nama kiri, nominal kanan).
		pc := cols[len(cols)-1]
		x0, y0 := pdf.GetXY()
		pdf.Rect(x0, y0, pc.w, h, "D")
		for i, m := range x.Methods {
			pdf.SetXY(x0+1, y0+0.5+float64(i)*lineH)
			pdf.CellFormat(pc.w*0.45, lineH, strings.ToUpper(m.Name), "", 0, "L", false, 0, "")
			pdf.CellFormat(pc.w*0.55-2, lineH, pdfMoney(m.Amount), "", 0, "R", false, 0, "")
		}
		pdf.SetXY(x0+pc.w, y0)
		pdf.Ln(h)
	}

	if res.HasSummary {
		needPage(7)
		pdf.SetFont("Helvetica", "B", 8)
		pdf.SetFillColor(225, 232, 240)
		pdf.CellFormat(0, 6.5, fmt.Sprintf("GRAND TOTAL — %d Faktur (Batal: %d)      Total: %s      Diterima: %s      Piutang: %s",
			res.Summary.SaleCount, res.Summary.VoidCount,
			pdfMoney(res.Summary.Total), pdfMoney(res.Summary.Paid), pdfMoney(res.Summary.Receivable)),
			"1", 1, "L", true, 0, "")
	}

	var buf bytes.Buffer
	if err := pdf.Output(&buf); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
