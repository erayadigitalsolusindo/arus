package sales

// Ekspor laporan penjualan DETAIL: Excel (.xlsx) dan PDF (landscape LEGAL).
// Keduanya membaca DetailResult yang sama dengan tampilan web, sehingga angka
// di layar = angka di berkas (satu sumber kebenaran).

import (
	"bytes"
	"fmt"
	"strings"

	"github.com/jung-kurt/gofpdf"
	"github.com/shopspring/decimal"
	"github.com/xuri/excelize/v2"
)

// DetailFileName dasar berkas ekspor detail (ASCII, aman untuk Content-Disposition).
func DetailFileName(from, to, ext string) string {
	return fmt.Sprintf("penjualan-detail_%s_%s.%s", from, to, ext)
}

// detailQty memformat qty tanpa nol desimal tak berarti (12.000 → 12).
func detailQty(s string) string {
	d, err := decimal.NewFromString(strings.TrimSpace(s))
	if err != nil {
		return "0"
	}
	return d.String()
}

// detailMethods meringkas metode bayar satu nota: "Tunai 50.000, QRIS 20.000".
func detailMethods(sales []DetailSale) string {
	var parts []string
	for _, s := range sales {
		for _, m := range s.Methods {
			parts = append(parts, m.Name+" "+pdfMoney(m.Amount))
		}
	}
	return strings.Join(parts, ", ")
}

// ── Excel ──────────────────────────────────────────────────────────────────────

// DetailToXLSX: sheet "Detail" (satu baris per barang; nota tanpa barang tetap
// satu baris) dan sheet "Ringkasan" (kop + total per tanggal + grand total).
func DetailToXLSX(res DetailResult) ([]byte, error) {
	f := excelize.NewFile()
	const detail = "Detail"
	if _, err := f.NewSheet(detail); err != nil {
		return nil, err
	}
	f.DeleteSheet("Sheet1")

	numStyle, err := f.NewStyle(&excelize.Style{NumFmt: 4}) // #,##0.00
	if err != nil {
		return nil, err
	}
	headStyle, err := f.NewStyle(&excelize.Style{Font: &excelize.Font{Bold: true}})
	if err != nil {
		return nil, err
	}

	// Kolom uang sheet Detail (1-based): Qty(14) … Piutang(26). Sisanya teks.
	isMoney := func(col int) bool { return col >= 14 && col <= 26 }

	// writeRow menulis satu baris; moneyStyle menentukan kolom uang, bold menebalkan.
	writeRow := func(sheet string, row int, vals []any, bold bool) error {
		for i, v := range vals {
			cell, _ := excelize.CoordinatesToCellName(i+1, row)
			if err := f.SetCellValue(sheet, cell, v); err != nil {
				return err
			}
			if isMoney(i + 1) {
				if err := f.SetCellStyle(sheet, cell, cell, numStyle); err != nil {
					return err
				}
			}
		}
		if bold {
			last, _ := excelize.CoordinatesToCellName(len(vals), row)
			if err := f.SetCellStyle(sheet, fmt.Sprintf("A%d", row), last, headStyle); err != nil {
				return err
			}
		}
		return nil
	}

	headers := []any{"Tanggal", "Jam", "Nota", "Status", "Outlet", "Kasir", "Member", "Salesperson",
		"No", "SKU", "Nama Barang", "Kategori", "Satuan", "Qty", "Harga", "Diskon", "Total Baris",
		"Subtotal", "Diskon Nota", "Pajak Toko", "Pajak Negara", "Biaya Lain", "Total", "Diterima",
		"Kembalian", "Piutang", "Metode Bayar"}
	if err := writeRow(detail, 1, headers, true); err != nil {
		return nil, err
	}

	// saleCells = kolom tetap nota + kolom uang nota, lalu diisi per barang.
	saleCells := func(day DetailDay, s DetailSale) []any {
		status := s.Status
		if status == "completed" {
			status = ""
		}
		return []any{
			day.Date, s.Time, s.DocNo, status, s.OutletCode,
			s.CashierName, s.MemberName, s.Salesperson,
		}
	}
	saleMoney := func(s DetailSale) []any {
		return []any{
			moneyFloat(s.Subtotal), moneyFloat(s.Discount), moneyFloat(s.TaxStore),
			moneyFloat(s.TaxGov), moneyFloat(s.OtherCost), moneyFloat(s.Total),
			moneyFloat(s.Paid), moneyFloat(s.Change), moneyFloat(s.Receivable),
		}
	}

	r := 2
	for _, day := range res.Data {
		for _, s := range day.Sales {
			base := saleCells(day, s)
			tail := saleMoney(s)
			if len(s.Lines) == 0 {
				v := append([]any{}, base...)
				v = append(v, "", "", "", "", "") // No..Satuan kosong
				v = append(v, 0.0, 0.0, 0.0, moneyFloat(s.LinesTotal))
				v = append(v, tail...)
				v = append(v, detailMethods([]DetailSale{s}))
				if err := writeRow(detail, r, v, false); err != nil {
					return nil, err
				}
				r++
				continue
			}
			// Satu baris per barang; nota diulang agar bisa difilter/pivot sendiri.
			for i, l := range s.Lines {
				v := append([]any{}, base...)
				v = append(v, i+1, l.SKU, l.Name, l.CategoryName, l.UnitName)
				v = append(v,
					moneyFloat(detailQty(l.Qty)), moneyFloat(l.UnitPrice),
					moneyFloat(l.Discount), moneyFloat(l.LineTotal))
				v = append(v, tail...)
				v = append(v, detailMethods([]DetailSale{s}))
				if err := writeRow(detail, r, v, false); err != nil {
					return nil, err
				}
				r++
			}
		}
	}

	f.SetColWidth(detail, "A", "A", 12)
	f.SetColWidth(detail, "B", "B", 7)
	f.SetColWidth(detail, "C", "C", 16)
	f.SetColWidth(detail, "D", "D", 9)
	f.SetColWidth(detail, "E", "E", 10)
	f.SetColWidth(detail, "F", "H", 16)
	f.SetColWidth(detail, "I", "I", 5)
	f.SetColWidth(detail, "J", "L", 26)
	f.SetColWidth(detail, "M", "M", 10)
	f.SetColWidth(detail, "N", "Z", 15)
	f.SetActiveSheet(0)

	// ── Sheet Ringkasan ───────────────────────────────────────────────────
	const summary = "Ringkasan"
	if _, err := f.NewSheet(summary); err != nil {
		return nil, err
	}
	// Kolom uang sheet Ringkasan (1-based): D(4) … J(10).
	isMoneySum := func(col int) bool { return col >= 4 && col <= 10 }
	writeSum := func(row int, vals []any, bold bool) error {
		for i, v := range vals {
			cell, _ := excelize.CoordinatesToCellName(i+1, row)
			if err := f.SetCellValue(summary, cell, v); err != nil {
				return err
			}
			if isMoneySum(i + 1) {
				if err := f.SetCellStyle(summary, cell, cell, numStyle); err != nil {
					return err
				}
			}
		}
		if bold {
			last, _ := excelize.CoordinatesToCellName(len(vals), row)
			if err := f.SetCellStyle(summary, fmt.Sprintf("A%d", row), last, headStyle); err != nil {
				return err
			}
		}
		return nil
	}

	meta := [][2]string{
		{"LAPORAN PENJUALAN DETAIL", ""},
		{"Tenant", res.TenantName},
		{"Periode", fmt.Sprintf("%s s.d. %s", pdfDate(res.From), pdfDate(res.To))},
		{"Outlet", res.OutletName},
	}
	if res.CashierName != "" {
		meta = append(meta, [2]string{"Kasir", res.CashierName})
	}
	if res.CategoryName != "" {
		meta = append(meta, [2]string{"Kategori", res.CategoryName})
	}
	if res.StatusLabel != "" {
		meta = append(meta, [2]string{"Status", res.StatusLabel})
	}
	for i, m := range meta {
		if err := writeSum(i+1, []any{m[0], m[1]}, false); err != nil {
			return nil, err
		}
	}
	if err := f.SetCellStyle(summary, "A1", "A1", headStyle); err != nil {
		return nil, err
	}

	start := len(meta) + 2
	if err := writeSum(start, []any{"Tanggal", "Nota", "Batal", "Subtotal", "Diskon",
		"Pajak Toko", "Pajak Negara", "Total", "Diterima", "Piutang"}, true); err != nil {
		return nil, err
	}
	rr := start + 1
	for _, day := range res.Data {
		if err := writeSum(rr, []any{day.Date, day.SaleCount, day.VoidCount,
			moneyFloat(day.Subtotal), moneyFloat(day.Discount), moneyFloat(day.TaxStore),
			moneyFloat(day.TaxGov), moneyFloat(day.Total), moneyFloat(day.Paid),
			moneyFloat(day.Receivable)}, false); err != nil {
			return nil, err
		}
		rr++
	}
	if err := writeSum(rr, []any{"GRAND TOTAL", res.Summary.SaleCount, res.Summary.VoidCount,
		moneyFloat(res.Summary.Subtotal), moneyFloat(res.Summary.Discount),
		moneyFloat(res.Summary.TaxStore), moneyFloat(res.Summary.TaxGov),
		moneyFloat(res.Summary.Total), moneyFloat(res.Summary.Paid),
		moneyFloat(res.Summary.Receivable)}, true); err != nil {
		return nil, err
	}
	f.SetColWidth(summary, "A", "A", 26)
	f.SetColWidth(summary, "B", "J", 16)

	var buf bytes.Buffer
	if err := f.Write(&buf); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// ── PDF ────────────────────────────────────────────────────────────────────────

// DetailToPDF: landscape LEGAL — kop, blok ringkasan, lalu per tanggal: baris
// tanggal, tiap nota (baris kop + baris barang), dan total per tanggal.
// Angka gaya Indonesia (1.234.567,64) rata kanan; tabel selebar kertas (339 mm).
func DetailToPDF(res DetailResult) ([]byte, error) {
	pdf := gofpdf.New("L", "mm", "LEGAL", "")
	pdf.SetTitle("Laporan Penjualan Detail "+res.From+" - "+res.To, false)
	pdf.SetAuthor("ARUS", false)
	// Margin kecil eksplisit (bawaan gofpdf boros) + jeda halaman manual penuh:
	// lebar pakai 355,6−8−8 = 339,6 mm; tabel di bawah total 339 mm.
	pdf.SetMargins(8, 6, 8)
	pdf.SetAutoPageBreak(false, 0)
	pdf.AddPage()

	// ── Kop ───────────────────────────────────────────────────────────────
	pdf.SetFont("Helvetica", "B", 13)
	pdf.CellFormat(0, 6, "LAPORAN PENJUALAN DETAIL", "", 1, "C", false, 0, "")
	if res.TenantName != "" {
		pdf.SetFont("Helvetica", "B", 10)
		pdf.CellFormat(0, 5, strings.ToUpper(res.TenantName), "", 1, "C", false, 0, "")
	}
	pdf.SetFont("Helvetica", "", 9)
	outlet := res.OutletName
	if res.OutletCode != "" {
		outlet = res.OutletCode + " - " + res.OutletName
	}
	meta := []string{
		fmt.Sprintf("Periode: %s s.d. %s", pdfDate(res.From), pdfDate(res.To)),
		"Outlet: " + outlet,
		"Nominal Harga Dalam Rupiah (IDR)",
	}
	if res.CashierName != "" {
		meta = append(meta, "Kasir: "+res.CashierName)
	}
	if res.CategoryName != "" {
		meta = append(meta, "Kategori: "+res.CategoryName)
	}
	if res.StatusLabel != "" {
		meta = append(meta, "Status: "+res.StatusLabel)
	}
	for _, line := range meta {
		pdf.CellFormat(0, 4.5, line, "", 1, "L", false, 0, "")
	}
	pdf.Ln(1)

	// ── Ringkasan (angka rata kanan) ──────────────────────────────────────
	pdf.SetFont("Helvetica", "B", 9)
	pdf.CellFormat(0, 5, "Ringkasan", "", 1, "L", false, 0, "")
	pdf.SetFont("Helvetica", "", 8.5)
	for _, line := range [][2]string{
		{"Total Nota", fmt.Sprintf("%d", res.Summary.SaleCount)},
		{"Total Belanja", pdfMoney(res.Summary.Total)},
		{"Diskon", pdfMoney(res.Summary.Discount)},
		{"Pajak Toko", pdfMoney(res.Summary.TaxStore)},
		{"Pajak Negara", pdfMoney(res.Summary.TaxGov)},
		{"Total Diterima", pdfMoney(res.Summary.Paid)},
		{"Total Piutang", pdfMoney(res.Summary.Receivable)},
		{"Nota Batal", fmt.Sprintf("%d", res.Summary.VoidCount)},
	} {
		pdf.CellFormat(55, 4.5, line[0], "", 0, "L", false, 0, "")
		pdf.CellFormat(0, 4.5, ": "+line[1], "", 1, "R", false, 0, "")
	}
	if res.Truncated {
		pdf.SetFont("Helvetica", "B", 8)
		pdf.CellFormat(0, 4.5,
			fmt.Sprintf("Catatan: hasil dipotong pada %d nota; persempit rentang tanggal.", maxDetailExportSales),
			"", 1, "L", false, 0, "")
		pdf.SetFont("Helvetica", "", 8.5)
	}
	pdf.Ln(1)

	// ── Tabel detail ──────────────────────────────────────────────────────
	type pdfCol struct {
		head  string
		w     float64
		align string
	}
	cols := []pdfCol{
		{"No", 7, "C"}, {"Jam", 12, "C"}, {"Nota", 26, "L"}, {"Kasir", 22, "L"},
		{"SKU", 20, "L"}, {"Nama Barang", 90, "L"}, {"Qty", 12, "R"}, {"Satuan", 14, "C"},
		{"Harga", 24, "R"}, {"Diskon", 18, "R"}, {"Total", 28, "R"}, {"Bayar", 26, "L"},
		{"Total Nota", 40, "R"},
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
	// row menggambar satu baris tabel; sel di luar panjang dikosongkan.
	row := func(cells []string, h float64, fill bool) {
		for j, c := range cols {
			txt := ""
			if j < len(cells) {
				txt = cells[j]
			}
			pdf.CellFormat(c.w, h, txt, "1", 0, c.align, fill, 0, "")
		}
		pdf.Ln(-1)
	}

	needPage(12)
	header()
	if res.CategoryName != "" {
		// CategoryName terisi = baris nota hanya barang kategori itu; kolom Subtotal/
		// Total di kop nota & banner tanggal tetap nilai nota PENUH. Sampaikan eksplisit
		// supaya "Σ baris" tak disalahartikan sama dengan Subtotal.
		pdf.SetFont("Helvetica", "I", 7)
		pdf.CellFormat(0, 4.5,
			"Catatan: hanya barang kategori \""+res.CategoryName+"\" yang ditampilkan per baris; kolom Subtotal/Total tetap nilai nota penuh.",
			"", 1, "L", false, 0, "")
		pdf.SetFont("Helvetica", "", 7.5)
	}

	for _, day := range res.Data {
		// Baris tanggal: satu baris penuh berisi ringkasan hari itu.
		needPage(6)
		pdf.SetFont("Helvetica", "B", 8)
		pdf.SetFillColor(225, 232, 240)
		banner := fmt.Sprintf("%s   —   %d Nota (Batal: %d)      Subtotal: %s   Diskon: %s   Pajak Toko: %s   Pajak Negara: %s   Total: %s   Diterima: %s   Piutang: %s",
			pdfDate(day.Date), day.SaleCount, day.VoidCount,
			pdfMoney(day.Subtotal), pdfMoney(day.Discount), pdfMoney(day.TaxStore),
			pdfMoney(day.TaxGov), pdfMoney(day.Total), pdfMoney(day.Paid), pdfMoney(day.Receivable))
		pdf.CellFormat(0, 6, banner, "1", 1, "L", true, 0, "")
		pdf.SetFont("Helvetica", "", 7.5)

		for _, s := range day.Sales {
			needPage(5)
			doc := s.DocNo
			if s.Status == "void" {
				doc += " (BATAL)"
			}
			kasir := s.CashierName
			if s.MemberName != "" {
				kasir += " / " + s.MemberName
			}
			pay := make([]string, 0, len(s.Methods))
			for _, m := range s.Methods {
				pay = append(pay, m.Name+" "+pdfMoney(m.Amount))
			}
			pdf.SetFont("Helvetica", "B", 7.5)
			row([]string{"", s.Time, doc, kasir, "", "", "", "", "", "", "",
				strings.Join(pay, ", "), pdfMoney(s.Total)}, 5, false)
			pdf.SetFont("Helvetica", "", 7)

			for i, l := range s.Lines {
				needPage(4.5)
				cells := make([]string, len(cols))
				cells[4] = l.SKU
				cells[5] = l.Name
				cells[6] = detailQty(l.Qty)
				cells[7] = l.UnitName
				cells[8] = pdfMoney(l.UnitPrice)
				cells[9] = pdfMoney(l.Discount)
				cells[10] = pdfMoney(l.LineTotal)
				if i == len(s.Lines)-1 {
					// Σ baris tampil (subset dari subtotal bila difilter kategori).
					cells[12] = "Σ baris " + pdfMoney(s.LinesTotal)
				}
				row(cells, 4.5, false)
			}
		}

		needPage(5)
		pdf.SetFont("Helvetica", "B", 7.5)
		pdf.SetFillColor(240, 240, 240)
		row([]string{"", "", "Total " + pdfDate(day.Date), "", "", "", "", "", "", "", "",
			"", pdfMoney(day.Total)}, 5, true)
		pdf.SetFont("Helvetica", "", 7.5)
	}

	needPage(7)
	pdf.SetFont("Helvetica", "B", 8)
	pdf.SetFillColor(225, 232, 240)
	pdf.CellFormat(0, 6.5, fmt.Sprintf("GRAND TOTAL — %d Nota (Batal: %d)      Total: %s      Diterima: %s      Piutang: %s",
		res.Summary.SaleCount, res.Summary.VoidCount,
		pdfMoney(res.Summary.Total), pdfMoney(res.Summary.Paid), pdfMoney(res.Summary.Receivable)),
		"1", 1, "L", true, 0, "")

	var buf bytes.Buffer
	if err := pdf.Output(&buf); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
