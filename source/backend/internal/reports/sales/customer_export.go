package sales

// Ekspor laporan per pelanggan & per faktur detail: Excel (.xlsx, dialirkan) dan
// PDF. Semua membaca hasil yang sama dengan tampilan web (satu sumber kebenaran).

import (
	"bytes"
	"fmt"
	"strings"

	"github.com/jung-kurt/gofpdf"
	"github.com/xuri/excelize/v2"
)

// fitText memotong teks agar muat di sel PDF selebar w mm (satu baris).
func fitText(pdf *gofpdf.Fpdf, s string, w float64) string {
	r := []rune(s)
	for len(r) > 1 && pdf.GetStringWidth(string(r)) > w-2 {
		r = r[:len(r)-1]
	}
	return string(r)
}

func kopMeta(from, to, outletCode, outletName, cashier string, extra ...string) []string {
	outlet := outletName
	if outletCode != "" {
		outlet = outletCode + " - " + outletName
	}
	meta := []string{
		fmt.Sprintf("Periode: %s s.d. %s", pdfDate(from), pdfDate(to)),
		"Outlet: " + outlet,
		"Nominal Harga Dalam Rupiah (IDR)",
	}
	if cashier != "" {
		meta = append(meta, "Kasir: "+cashier)
	}
	return append(meta, extra...)
}

func pdfKop(pdf *gofpdf.Fpdf, title, tenant string, meta []string) {
	pdf.SetFont("Helvetica", "B", 13)
	pdf.CellFormat(0, 6, title, "", 1, "C", false, 0, "")
	if tenant != "" {
		pdf.SetFont("Helvetica", "B", 10)
		pdf.CellFormat(0, 5, strings.ToUpper(tenant), "", 1, "C", false, 0, "")
	}
	pdf.SetFont("Helvetica", "", 9)
	for _, line := range meta {
		pdf.CellFormat(0, 4.5, line, "", 1, "L", false, 0, "")
	}
}

func finishPDF(pdf *gofpdf.Fpdf) ([]byte, error) {
	var buf bytes.Buffer
	if err := pdf.Output(&buf); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// ── Omset per pelanggan ────────────────────────────────────────────────────────

func customerKop(res CustomerResult) (title, tenant string, meta []string) {
	extra := []string{"Hanya pelanggan member; nota non member tidak diikutkan"}
	if res.SortLabel != "" {
		extra = append(extra, "Urut: "+res.SortLabel)
	}
	return "LAPORAN OMSET PER PELANGGAN", strings.ToUpper(res.TenantName),
		kopMeta(res.From, res.To, res.OutletCode, res.OutletName, res.CashierName, extra...)
}

// CustomerToXLSX: kop + tabel omset per pelanggan.
func CustomerToXLSX(res CustomerResult) ([]byte, error) {
	f := excelize.NewFile()
	const sheet = "Omset Pelanggan"
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
	for i, w := range []float64{5, 14, 34, 16, 10, 18, 16, 18, 18, 14} {
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
	title, tenant, meta := customerKop(res)
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
		if err := put(fmt.Sprintf("Catatan: hasil dipotong pada %d pelanggan dari %d; persempit rentang/filter.",
			len(res.Data), res.Summary.CustomerCount)); err != nil {
			return nil, err
		}
	}
	if err := put(); err != nil {
		return nil, err
	}
	headers := []string{"No", "Kode", "Pelanggan", "Telepon", "Faktur", "Subtotal", "Diskon", "Omset", "Rata-rata/Faktur", "Terakhir"}
	hrow := make([]any, len(headers))
	for i, h := range headers {
		hrow[i] = excelize.Cell{StyleID: bold, Value: h}
	}
	if err := put(hrow...); err != nil {
		return nil, err
	}
	money := func(s string) any { return excelize.Cell{StyleID: num, Value: moneyFloat(s)} }
	for i, x := range res.Data {
		if err := put(i+1, x.Code, x.Name, x.Phone, x.InvoiceCount,
			money(x.Subtotal), money(x.Discount), money(x.Total), money(x.Average), pdfDate(x.LastDate)); err != nil {
			return nil, err
		}
	}
	if err := put(); err != nil {
		return nil, err
	}
	if err := put("", "", excelize.Cell{StyleID: bold, Value: fmt.Sprintf("GRAND TOTAL — %d Pelanggan", res.Summary.CustomerCount)},
		"", res.Summary.InvoiceCount, money(res.Summary.Subtotal), money(res.Summary.Discount), money(res.Summary.Total)); err != nil {
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

// CustomerToPDF: portrait A4 — kop, tabel omset, grand total (lebar 194 mm).
func CustomerToPDF(res CustomerResult) ([]byte, error) {
	pdf := gofpdf.New("P", "mm", "A4", "")
	pdf.SetTitle("Laporan Omset Per Pelanggan "+res.From+" - "+res.To, false)
	pdf.SetAuthor("ARUS", false)
	pdf.SetMargins(8, 8, 8)
	pdf.SetAutoPageBreak(false, 0)
	pdf.AddPage()

	title, tenant, meta := customerKop(res)
	pdfKop(pdf, title, tenant, meta)
	if res.Truncated {
		pdf.SetFont("Helvetica", "B", 8)
		pdf.CellFormat(0, 4.5, fmt.Sprintf("Catatan: hasil dipotong pada %d pelanggan dari %d; persempit rentang/filter.",
			len(res.Data), res.Summary.CustomerCount), "", 1, "L", false, 0, "")
	}
	pdf.Ln(1.5)

	type col struct {
		head  string
		w     float64
		align string
	}
	cols := []col{
		{"No", 9, "R"}, {"Kode", 20, "L"}, {"Pelanggan", 60, "L"}, {"Faktur", 14, "R"},
		{"Diskon", 24, "R"}, {"Omset", 30, "R"}, {"Rata-rata", 24, "R"}, {"Terakhir", 13, "C"},
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
	for i, x := range res.Data {
		needPage(5)
		d := pdfDate(x.LastDate)
		if len(d) == 10 {
			d = d[:6] + d[8:] // dd-mm-yy agar muat
		}
		cells := []string{fmt.Sprintf("%d", i+1), fitText(pdf, x.Code, cols[1].w), fitText(pdf, x.Name, cols[2].w),
			fmt.Sprintf("%d", x.InvoiceCount), pdfMoney(x.Discount), pdfMoney(x.Total), pdfMoney(x.Average), d}
		for j, txt := range cells {
			pdf.CellFormat(cols[j].w, 5, txt, "1", 0, cols[j].align, false, 0, "")
		}
		pdf.Ln(-1)
	}
	needPage(7)
	pdf.SetFont("Helvetica", "B", 8)
	pdf.SetFillColor(225, 232, 240)
	label := cols[0].w + cols[1].w + cols[2].w
	pdf.CellFormat(label, 6.5, fmt.Sprintf("GRAND TOTAL — %d Pelanggan", res.Summary.CustomerCount), "1", 0, "L", true, 0, "")
	pdf.CellFormat(cols[3].w, 6.5, fmt.Sprintf("%d", res.Summary.InvoiceCount), "1", 0, "R", true, 0, "")
	pdf.CellFormat(cols[4].w, 6.5, pdfMoney(res.Summary.Discount), "1", 0, "R", true, 0, "")
	pdf.CellFormat(cols[5].w, 6.5, pdfMoney(res.Summary.Total), "1", 0, "R", true, 0, "")
	pdf.CellFormat(cols[6].w+cols[7].w, 6.5, "", "1", 1, "R", true, 0, "")
	return finishPDF(pdf)
}

// ── Pelanggan per faktur (+ isi barang) ────────────────────────────────────────

func custInvKop(res CustInvResult) (title, tenant string, meta []string) {
	title = "LAPORAN PELANGGAN PER FAKTUR"
	if res.WithItems {
		title = "LAPORAN PELANGGAN DETAIL PER FAKTUR"
	}
	return title, strings.ToUpper(res.TenantName),
		kopMeta(res.From, res.To, res.OutletCode, res.OutletName, res.CashierName,
			"Hanya pelanggan member; nota non member tidak diikutkan")
}

// CustInvToXLSX: satu sheet datar — baris faktur (+ baris barang bila WithItems).
func CustInvToXLSX(res CustInvResult) ([]byte, error) {
	f := excelize.NewFile()
	const sheet = "Pelanggan Faktur"
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
	for i, w := range []float64{14, 30, 12, 8, 22, 18, 16, 34, 10, 10, 16, 14, 18} {
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
	title, tenant, meta := custInvKop(res)
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
		if err := put("Catatan: hasil dipotong oleh batas ekspor; persempit rentang/filter."); err != nil {
			return nil, err
		}
	}
	if err := put(); err != nil {
		return nil, err
	}
	headers := []string{"Kode", "Pelanggan", "Tanggal", "Jam", "Faktur", "Kasir", "Kode Barang", "Nama Barang", "Qty", "Satuan", "Harga", "Diskon", "Jumlah"}
	hrow := make([]any, len(headers))
	for i, h := range headers {
		hrow[i] = excelize.Cell{StyleID: bold, Value: h}
	}
	if err := put(hrow...); err != nil {
		return nil, err
	}
	money := func(s string) any { return excelize.Cell{StyleID: num, Value: moneyFloat(s)} }
	for _, c := range res.Customers {
		for _, sl := range c.Sales {
			// Baris faktur: Diskon & Jumlah = diskon dan total faktur.
			if err := put(c.Code, c.Name, pdfDate(sl.Date), sl.Time, sl.DocNo, sl.CashierName,
				"", "", nil, "", nil, money(sl.Discount), excelize.Cell{StyleID: num, Value: moneyFloat(sl.Total)}); err != nil {
				return nil, err
			}
			for _, l := range sl.Lines {
				if err := put(c.Code, c.Name, pdfDate(sl.Date), sl.Time, sl.DocNo, "",
					l.SKU, l.Name, excelize.Cell{StyleID: qtyStyle, Value: moneyFloat(l.Qty)}, l.UnitName,
					money(l.UnitPrice), money(l.Discount), money(l.LineTotal)); err != nil {
					return nil, err
				}
			}
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

// CustInvToPDF: portrait A4 — per pelanggan: banner, faktur, lalu (bila WithItems)
// barang tiap faktur; subtotal pelanggan; grand total di akhir.
func CustInvToPDF(res CustInvResult) ([]byte, error) {
	pdf := gofpdf.New("P", "mm", "A4", "")
	pdf.SetTitle("Laporan Pelanggan Per Faktur "+res.From+" - "+res.To, false)
	pdf.SetAuthor("ARUS", false)
	pdf.SetMargins(8, 8, 8)
	pdf.SetAutoPageBreak(false, 0)
	pdf.AddPage()

	title, tenant, meta := custInvKop(res)
	pdfKop(pdf, title, tenant, meta)
	if res.Truncated {
		pdf.SetFont("Helvetica", "B", 8)
		pdf.CellFormat(0, 4.5, "Catatan: hasil dipotong oleh batas ekspor; persempit rentang/filter.", "", 1, "L", false, 0, "")
	}
	pdf.Ln(1.5)

	const (
		wDate, wTime, wDoc, wCashier = 18.0, 11.0, 40.0, 40.0
		wDisc, wTotal                = 30.0, 55.0
	)
	invHead := func() {
		pdf.SetFont("Helvetica", "B", 7.5)
		pdf.SetFillColor(235, 235, 235)
		pdf.CellFormat(wDate, 5.5, "Tanggal", "1", 0, "C", true, 0, "")
		pdf.CellFormat(wTime, 5.5, "Jam", "1", 0, "C", true, 0, "")
		pdf.CellFormat(wDoc, 5.5, "Faktur", "1", 0, "L", true, 0, "")
		pdf.CellFormat(wCashier, 5.5, "Kasir", "1", 0, "L", true, 0, "")
		pdf.CellFormat(wDisc, 5.5, "Diskon", "1", 0, "R", true, 0, "")
		pdf.CellFormat(wTotal-4, 5.5, "Total Faktur", "1", 1, "R", true, 0, "")
		pdf.SetFont("Helvetica", "", 7.5)
	}
	needPage := func(h float64) {
		if pdf.GetY()+h > 284 {
			pdf.AddPage()
		}
	}

	itemCols := []struct {
		head  string
		w     float64
		align string
	}{{"Kode", 24, "L"}, {"Nama Barang", 70, "L"}, {"Qty", 16, "R"}, {"Satuan", 16, "L"}, {"Harga", 26, "R"}, {"Jumlah", 38, "R"}}

	totInv := 0
	for _, c := range res.Customers {
		needPage(24)
		pdf.SetFont("Helvetica", "B", 8.5)
		pdf.SetFillColor(225, 232, 240)
		pdf.CellFormat(0, 6, fmt.Sprintf("%s — %s     %d faktur     Omset: %s",
			c.Code, c.Name, c.InvoiceCount, pdfMoney(c.Total)), "1", 1, "L", true, 0, "")
		invHead()
		for _, sl := range c.Sales {
			need := 5.0
			if res.WithItems {
				need += 4.5 * float64(min(len(sl.Lines), 3)+1)
			}
			if pdf.GetY()+need > 284 {
				pdf.AddPage()
				invHead()
			}
			totInv++
			pdf.SetFont("Helvetica", "B", 7.5)
			pdf.CellFormat(wDate, 5, pdfDate(sl.Date), "1", 0, "C", false, 0, "")
			pdf.CellFormat(wTime, 5, sl.Time, "1", 0, "C", false, 0, "")
			pdf.CellFormat(wDoc, 5, fitText(pdf, sl.DocNo, wDoc), "1", 0, "L", false, 0, "")
			pdf.CellFormat(wCashier, 5, fitText(pdf, sl.CashierName, wCashier), "1", 0, "L", false, 0, "")
			pdf.CellFormat(wDisc, 5, pdfMoney(sl.Discount), "1", 0, "R", false, 0, "")
			pdf.CellFormat(wTotal-4, 5, pdfMoney(sl.Total), "1", 1, "R", false, 0, "")
			if !res.WithItems {
				continue
			}
			pdf.SetFont("Helvetica", "", 7)
			for _, l := range sl.Lines {
				if pdf.GetY()+4.5 > 284 {
					pdf.AddPage()
				}
				pdf.CellFormat(4, 4.5, "", "", 0, "L", false, 0, "")
				cells := []string{fitText(pdf, l.SKU, itemCols[0].w), fitText(pdf, l.Name, itemCols[1].w),
					detailQty(l.Qty), fitText(pdf, l.UnitName, itemCols[3].w), pdfMoney(l.UnitPrice), pdfMoney(l.LineTotal)}
				for j, txt := range cells {
					pdf.CellFormat(itemCols[j].w, 4.5, txt, "B", 0, itemCols[j].align, false, 0, "")
				}
				pdf.Ln(-1)
			}
		}
		pdf.Ln(2.5)
	}

	needPage(8)
	pdf.SetFont("Helvetica", "B", 8)
	pdf.SetFillColor(225, 232, 240)
	pdf.CellFormat(0, 6.5, fmt.Sprintf("GRAND TOTAL — %d Pelanggan     %d Faktur     Omset: %s",
		len(res.Customers), totInv, pdfMoney(res.Summary.Total)), "1", 1, "L", true, 0, "")
	return finishPDF(pdf)
}

// ── Per faktur detail (datar, tanpa pengelompokan tanggal) ─────────────────────

// InvoiceDetailFileName nama berkas ekspor per faktur detail.
func InvoiceDetailFileName(from, to, ext string) string {
	return fmt.Sprintf("penjualan-perfaktur-detail_%s_%s.%s", from, to, ext)
}

// InvoiceDetailToPDF: landscape LEGAL — kop, ringkasan, lalu tiap faktur
// (baris kop faktur + baris barang) berurutan, tanpa pengelompokan tanggal.
func InvoiceDetailToPDF(res DetailResult) ([]byte, error) {
	pdf := gofpdf.New("L", "mm", "LEGAL", "")
	pdf.SetTitle("Laporan Penjualan Per Faktur Detail "+res.From+" - "+res.To, false)
	pdf.SetAuthor("ARUS", false)
	pdf.SetMargins(8, 6, 8)
	pdf.SetAutoPageBreak(false, 0)
	pdf.AddPage()

	extra := []string{}
	if res.StatusLabel != "" {
		extra = append(extra, "Status: "+res.StatusLabel)
	}
	pdfKop(pdf, "LAPORAN PENJUALAN PER FAKTUR DETAIL", res.TenantName,
		kopMeta(res.From, res.To, res.OutletCode, res.OutletName, res.CashierName, extra...))
	pdf.SetFont("Helvetica", "", 8.5)
	pdf.CellFormat(0, 4.5, fmt.Sprintf("Total Faktur: %d     Batal: %d     Total: %s     Diterima: %s     Piutang: %s",
		res.Summary.SaleCount, res.Summary.VoidCount, pdfMoney(res.Summary.Total),
		pdfMoney(res.Summary.Paid), pdfMoney(res.Summary.Receivable)), "", 1, "L", false, 0, "")
	if res.Truncated {
		pdf.SetFont("Helvetica", "B", 8)
		pdf.CellFormat(0, 4.5, fmt.Sprintf("Catatan: hasil dipotong pada %d faktur; persempit rentang tanggal.", maxDetailExportSales),
			"", 1, "L", false, 0, "")
	}
	pdf.Ln(1)

	type col struct {
		head  string
		w     float64
		align string
	}
	cols := []col{
		{"Tanggal", 18, "C"}, {"Jam", 12, "C"}, {"Faktur", 34, "L"}, {"Kasir / Pelanggan", 48, "L"},
		{"SKU", 22, "L"}, {"Nama Barang", 82, "L"}, {"Qty", 12, "R"}, {"Satuan", 14, "C"},
		{"Harga", 24, "R"}, {"Diskon", 18, "R"}, {"Jumlah", 26, "R"}, {"Total Faktur", 29, "R"},
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
	row := func(cells []string, h float64) {
		for j, c := range cols {
			txt := ""
			if j < len(cells) {
				txt = cells[j]
			}
			pdf.CellFormat(c.w, h, txt, "1", 0, c.align, false, 0, "")
		}
		pdf.Ln(-1)
	}
	header()
	for _, day := range res.Data {
		for _, s := range day.Sales {
			need := 5.0
			if len(s.Lines) > 0 {
				need += 4.5
			}
			needPage(need)
			doc := s.DocNo
			if s.Status == "void" {
				doc += " (BATAL)"
			}
			who := s.CashierName
			if s.MemberName != "" {
				who += " / " + s.MemberName
			}
			pdf.SetFont("Helvetica", "B", 7.5)
			row([]string{pdfDate(day.Date), s.Time, fitText(pdf, doc, cols[2].w), fitText(pdf, who, cols[3].w),
				"", "", "", "", "", "", "", pdfMoney(s.Total)}, 5)
			pdf.SetFont("Helvetica", "", 7)
			for _, l := range s.Lines {
				needPage(4.5)
				row([]string{"", "", "", "", fitText(pdf, l.SKU, cols[4].w), fitText(pdf, l.Name, cols[5].w),
					detailQty(l.Qty), l.UnitName, pdfMoney(l.UnitPrice), pdfMoney(l.Discount), pdfMoney(l.LineTotal), ""}, 4.5)
			}
		}
	}
	needPage(7)
	pdf.SetFont("Helvetica", "B", 8)
	pdf.SetFillColor(225, 232, 240)
	pdf.CellFormat(0, 6.5, fmt.Sprintf("GRAND TOTAL — %d Faktur (Batal: %d)      Total: %s      Diterima: %s      Piutang: %s",
		res.Summary.SaleCount, res.Summary.VoidCount, pdfMoney(res.Summary.Total),
		pdfMoney(res.Summary.Paid), pdfMoney(res.Summary.Receivable)), "1", 1, "L", true, 0, "")
	return finishPDF(pdf)
}
