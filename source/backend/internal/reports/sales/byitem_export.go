package sales

// Ekspor laporan penjualan PER BARANG: Excel (.xlsx, dialirkan) dan PDF
// (portrait A4) dengan kop yang sama seperti laporan penjualan lain.

import (
	"bytes"
	"fmt"
	"strings"

	"github.com/jung-kurt/gofpdf"
	"github.com/shopspring/decimal"
	"github.com/xuri/excelize/v2"
)

// qtyText: qty gaya Indonesia tanpa nol di belakang koma ("1234.500" → "1.234,5").
func qtyText(s string) string {
	d, err := decimal.NewFromString(strings.TrimSpace(s))
	if err != nil {
		return "0"
	}
	intPart, frac, _ := strings.Cut(d.String(), ".")
	neg := strings.HasPrefix(intPart, "-")
	intPart = strings.TrimPrefix(intPart, "-")
	var b strings.Builder
	for i, r := range intPart {
		if i > 0 && (len(intPart)-i)%3 == 0 {
			b.WriteByte('.')
		}
		b.WriteRune(r)
	}
	out := b.String()
	if neg {
		out = "-" + out
	}
	if frac != "" {
		out += "," + frac
	}
	return out
}

func itemKop(res ItemResult) (title, tenant string, meta []string) {
	outlet := res.OutletName
	if res.OutletCode != "" {
		outlet = res.OutletCode + " - " + res.OutletName
	}
	meta = []string{
		fmt.Sprintf("Periode: %s s.d. %s", pdfDate(res.From), pdfDate(res.To)),
		"Outlet: " + outlet,
		"Nominal Harga Dalam Rupiah (IDR); Qty dalam satuan dasar",
	}
	if res.CashierName != "" {
		meta = append(meta, "Kasir: "+res.CashierName)
	}
	if res.CategoryName != "" {
		meta = append(meta, "Kategori: "+res.CategoryName)
	}
	if res.SortLabel != "" {
		meta = append(meta, "Urut: "+res.SortLabel)
	}
	return "LAPORAN PENJUALAN PER BARANG", strings.ToUpper(res.TenantName), meta
}

// ItemToXLSX: kop + tabel; dialirkan per baris (memori tetap kecil).
func ItemToXLSX(res ItemResult) ([]byte, error) {
	f := excelize.NewFile()
	const sheet = "Per Barang"
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
	qtyStyle, err := f.NewStyle(&excelize.Style{NumFmt: 3})
	if err != nil {
		return nil, err
	}
	for i, w := range []float64{5, 18, 40, 22, 10, 14, 10, 18, 16, 18} {
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
	title, tenant, meta := itemKop(res)
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
		if err := put(fmt.Sprintf("Catatan: hasil dipotong pada %d barang dari %d; persempit rentang/filter.", len(res.Data), res.Summary.ItemCount)); err != nil {
			return nil, err
		}
	}
	if err := put(); err != nil {
		return nil, err
	}

	headers := []string{"No", "Kode", "Nama Barang", "Kategori", "Satuan", "Qty", "Frekuensi",
		"Penjualan Kotor", "Diskon", "Penjualan Bersih"}
	hrow := make([]any, len(headers))
	for i, h := range headers {
		hrow[i] = excelize.Cell{StyleID: bold, Value: h}
	}
	if err := put(hrow...); err != nil {
		return nil, err
	}
	money := func(s string) any { return excelize.Cell{StyleID: num, Value: moneyFloat(s)} }
	for i, x := range res.Data {
		if err := put(i+1, x.SKU, x.Name, x.CategoryName, x.UnitName,
			excelize.Cell{StyleID: qtyStyle, Value: moneyFloat(x.Qty)}, x.LineCount,
			money(x.Gross), money(x.Discount), money(x.Net)); err != nil {
			return nil, err
		}
	}
	if err := put(); err != nil {
		return nil, err
	}
	if err := put("", "", excelize.Cell{StyleID: bold, Value: fmt.Sprintf("GRAND TOTAL — %d Barang", res.Summary.ItemCount)},
		"", "", "", "", money(res.Summary.Gross), money(res.Summary.Discount), money(res.Summary.Net)); err != nil {
		return nil, err
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

// ItemToPDF: portrait A4 — kop, tabel barang, grand total. Lebar tabel 194 mm.
func ItemToPDF(res ItemResult) ([]byte, error) {
	pdf := gofpdf.New("P", "mm", "A4", "")
	pdf.SetTitle("Laporan Penjualan Per Barang "+res.From+" - "+res.To, false)
	pdf.SetAuthor("ARUS", false)
	pdf.SetMargins(8, 8, 8)
	pdf.SetAutoPageBreak(false, 0)
	pdf.AddPage()

	title, tenant, meta := itemKop(res)
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
	if res.Truncated {
		pdf.SetFont("Helvetica", "B", 8)
		pdf.CellFormat(0, 4.5, fmt.Sprintf("Catatan: hasil dipotong pada %d barang dari %d; persempit rentang/filter.",
			len(res.Data), res.Summary.ItemCount), "", 1, "L", false, 0, "")
	}
	pdf.Ln(1.5)

	type col struct {
		head  string
		w     float64
		align string
	}
	cols := []col{
		{"No", 9, "R"}, {"Kode", 22, "L"}, {"Nama Barang", 56, "L"}, {"Qty", 20, "R"},
		{"Satuan", 14, "L"}, {"Kotor", 26, "R"}, {"Diskon", 20, "R"}, {"Bersih", 27, "R"},
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
		if pdf.GetY()+h > 284 {
			pdf.AddPage()
			header()
		}
	}
	header()

	// fit memotong teks agar muat di sel (satu baris per barang).
	fit := func(s string, w float64) string {
		r := []rune(s)
		for len(r) > 1 && pdf.GetStringWidth(string(r)) > w-2 {
			r = r[:len(r)-1]
		}
		return string(r)
	}
	for i, x := range res.Data {
		needPage(5)
		cells := []string{fmt.Sprintf("%d", i+1), fit(x.SKU, cols[1].w), fit(x.Name, cols[2].w), qtyText(x.Qty),
			fit(x.UnitName, cols[4].w), pdfMoney(x.Gross), pdfMoney(x.Discount), pdfMoney(x.Net)}
		for j, txt := range cells {
			pdf.CellFormat(cols[j].w, 5, txt, "1", 0, cols[j].align, false, 0, "")
		}
		pdf.Ln(-1)
	}

	needPage(7)
	pdf.SetFont("Helvetica", "B", 8)
	pdf.SetFillColor(225, 232, 240)
	label := cols[0].w + cols[1].w + cols[2].w + cols[3].w + cols[4].w
	pdf.CellFormat(label, 6.5, fmt.Sprintf("GRAND TOTAL — %d Barang", res.Summary.ItemCount), "1", 0, "L", true, 0, "")
	pdf.CellFormat(cols[5].w, 6.5, pdfMoney(res.Summary.Gross), "1", 0, "R", true, 0, "")
	pdf.CellFormat(cols[6].w, 6.5, pdfMoney(res.Summary.Discount), "1", 0, "R", true, 0, "")
	pdf.CellFormat(cols[7].w, 6.5, pdfMoney(res.Summary.Net), "1", 1, "R", true, 0, "")

	var buf bytes.Buffer
	if err := pdf.Output(&buf); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
