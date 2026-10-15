package sales

// Ekspor laporan per tanggal: Excel (.xlsx) dan PDF (landscape LEGAL).
// Keduanya membaca ByDateResult yang sama dengan tampilan web (dari ringkasan
// harian ≤366 baris), sehingga biayanya tidak tergantung volume tabel sales:
// ekspor 1 bulan/1 tahun tetap ringan walau sales mencapai ratusan juta baris.

import (
	"bytes"
	"fmt"
	"strings"

	"github.com/jung-kurt/gofpdf"
	"github.com/shopspring/decimal"
	"github.com/xuri/excelize/v2"
)

// pdfMoney memformat desimal gaya Indonesia (3.470.220,00) untuk PDF.
func pdfMoney(s string) string {
	d, err := decimal.NewFromString(strings.TrimSpace(s))
	if err != nil {
		return "0"
	}
	neg := d.IsNegative()
	if neg {
		d = d.Neg()
	}
	intPart := d.IntPart()
	frac := d.Sub(decimal.NewFromInt(intPart)).Mul(decimal.NewFromInt(100)).IntPart()
	var groups []string
	if intPart == 0 {
		groups = []string{"0"}
	}
	for intPart > 0 {
		groups = append([]string{fmt.Sprintf("%03d", intPart%1000)}, groups...)
		intPart /= 1000
	}
	groups[0] = strings.TrimLeft(groups[0], "0")
	if groups[0] == "" {
		groups[0] = "0"
	}
	out := strings.Join(groups, ".") + fmt.Sprintf(",%02d", frac)
	if neg {
		out = "-" + out
	}
	return out
}

// pdfDate: "2026-10-31" → "31-10-2026" (gaya kop laporan legacy).
func pdfDate(ymd string) string {
	if len(ymd) == 10 && ymd[4] == '-' && ymd[7] == '-' {
		return ymd[8:10] + "-" + ymd[5:7] + "-" + ymd[0:4]
	}
	return ymd
}

// moneyFloat mengubah string desimal ke float64 untuk Excel.
func moneyFloat(s string) float64 {
	d, err := decimal.NewFromString(strings.TrimSpace(s))
	if err != nil {
		return 0
	}
	f, _ := d.Float64()
	return f
}

// pdfCol = definisi kolom tabel PDF per tanggal.
type pdfCol struct {
	head  string
	w     float64
	align string
}

// GrandTotal menjumlahkan baris harian (dipakai layar/export agar konsisten).
type GrandTotal struct {
	SaleCount, VoidCount, ReturnCount                                                               int
	Subtotal, Discount, TaxStore, TaxGov, Total, Paid, Receivable, VoidTotal, ReturnTotal, NetTotal decimal.Decimal
}

// SumTotal menghitung grand total dari baris hasil (sudah termasuk filter hide_empty pemanggil).
func SumTotal(rows []DayRow) GrandTotal {
	var g GrandTotal
	add := func(dst *decimal.Decimal, s string) {
		d, err := decimal.NewFromString(strings.TrimSpace(s))
		if err == nil {
			*dst = dst.Add(d)
		}
	}
	for _, r := range rows {
		g.SaleCount += r.SaleCount
		g.VoidCount += r.VoidCount
		g.ReturnCount += r.ReturnCount
		add(&g.Subtotal, r.Subtotal)
		add(&g.Discount, r.Discount)
		add(&g.TaxStore, r.TaxStore)
		add(&g.TaxGov, r.TaxGov)
		add(&g.Total, r.Total)
		add(&g.Paid, r.Paid)
		add(&g.Receivable, r.Receivable)
		add(&g.VoidTotal, r.VoidTotal)
		add(&g.ReturnTotal, r.ReturnTotal)
		add(&g.NetTotal, r.NetTotal)
	}
	return g
}

// FileName dasar berkas ekspor (ASCII, aman untuk Content-Disposition).
func FileName(from, to, ext string) string {
	return fmt.Sprintf("penjualan-per-tanggal_%s_%s.%s", from, to, ext)
}

// toXLSXCols = header sheet harian (tanggal + agregat harian). Kolom uang =
// possi 5..15 (1-based) sesuai isMoneyXLSX.
var toXLSXCols = []string{
	"No", "Tanggal", "Nota", "Batal", "Subtotal", "Diskon", "Pajak Toko", "Pajak Negara",
	"Total", "Diterima", "Piutang", "Total Batal", "Jml Retur", "Total Retur", "Bersih",
}

// ToXLSX: laporan per tanggal — satu sheet "Harian" berisi kop (judul, tenant,
// periode, outlet), tabel harian bernomor, dan baris grand total. Angka uang
// bertype float dengan format #,##0.00 (sama dengan sheet detail).
func ToXLSX(res ByDateResult) ([]byte, error) {
	f := excelize.NewFile()
	const sheet = "Harian"
	if _, err := f.NewSheet(sheet); err != nil {
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
	// Kolom uang sheet Harian (1-based): Subtotal(5) … Bersih(15).
	isMoney := func(col int) bool { return col >= 5 && col <= 15 }

	writeRow := func(row int, vals []any, bold bool) error {
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

	// ── Kop ────────────────────────────────────────────────────────────────
	outlet := res.Outlet
	if res.OutletCode != "" {
		outlet = res.OutletCode + " - " + res.Outlet
	}
	meta := [][2]string{
		{"LAPORAN PENJUALAN PER TANGGAL", ""},
		{"Tenant", res.Tenant},
		{"Periode", fmt.Sprintf("%s s.d. %s", pdfDate(res.From), pdfDate(res.To))},
		{"Outlet", outlet},
	}
	for i, m := range meta {
		if err := writeRow(i+1, []any{m[0], m[1]}, false); err != nil {
			return nil, err
		}
	}
	if err := f.SetCellStyle(sheet, "A1", "A1", headStyle); err != nil {
		return nil, err
	}

	// ── Tabel harian + grand total ─────────────────────────────────────────
	headerRow := len(meta) + 2
	headers := make([]any, len(toXLSXCols))
	for i, h := range toXLSXCols {
		headers[i] = h
	}
	if err := writeRow(headerRow, headers, true); err != nil {
		return nil, err
	}
	g := SumTotal(res.Data)
	row := headerRow + 1
	for i, d := range res.Data {
		if err := writeRow(row, []any{
			i + 1, d.Date, d.SaleCount, d.VoidCount,
			moneyFloat(d.Subtotal), moneyFloat(d.Discount), moneyFloat(d.TaxStore), moneyFloat(d.TaxGov),
			moneyFloat(d.Total), moneyFloat(d.Paid), moneyFloat(d.Receivable), moneyFloat(d.VoidTotal),
			d.ReturnCount, moneyFloat(d.ReturnTotal), moneyFloat(d.NetTotal),
		}, false); err != nil {
			return nil, err
		}
		row++
	}
	if err := writeRow(row, []any{
		"", "GRAND TOTAL", g.SaleCount, g.VoidCount,
		moneyFloat(g.Subtotal.StringFixed(2)), moneyFloat(g.Discount.StringFixed(2)),
		moneyFloat(g.TaxStore.StringFixed(2)), moneyFloat(g.TaxGov.StringFixed(2)),
		moneyFloat(g.Total.StringFixed(2)), moneyFloat(g.Paid.StringFixed(2)),
		moneyFloat(g.Receivable.StringFixed(2)), moneyFloat(g.VoidTotal.StringFixed(2)),
		g.ReturnCount, moneyFloat(g.ReturnTotal.StringFixed(2)), moneyFloat(g.NetTotal.StringFixed(2)),
	}, true); err != nil {
		return nil, err
	}

	f.SetColWidth(sheet, "A", "A", 5)
	f.SetColWidth(sheet, "B", "B", 12)
	f.SetColWidth(sheet, "C", "D", 8)
	f.SetColWidth(sheet, "E", "O", 15)
	f.SetActiveSheet(0)

	var buf bytes.Buffer
	if err := f.Write(&buf); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// byDatePDFCols return the column definitions for the by-date PDF report.
// Total width = 8 + 22 + 14 + 14 + 28 + 20 + 24 + 24 + 28 + 28 + 28 + 29 + 14 + 29 + 29 = 338 mm,
// which fits the LEGAL landscape width of 339.6 mm (355.6 − 8 − 8).
func byDatePDFCols() []pdfCol {
	return []pdfCol{
		{"No", 8, "C"}, {"Tanggal", 22, "C"}, {"Nota", 14, "R"}, {"Batal", 14, "R"},
		{"Subtotal", 28, "R"}, {"Diskon", 20, "R"}, {"Pajak Toko", 24, "R"}, {"Pajak Negara", 24, "R"},
		{"Total", 28, "R"}, {"Diterima", 28, "R"}, {"Piutang", 28, "R"}, {"Total Batal", 29, "R"},
		{"Jml Retur", 14, "R"}, {"Total Retur", 29, "R"}, {"Bersih", 29, "R"},
	}
}

// toPDF: landscape LEGAL gaya laporan legacy — kop tengah (judul + tenant +
// periode/outlet/satuan), blok ringkasan, tabel harian bernomor dengan header
// berulang tiap halaman + baris grand total, lalu tabel rincian metode.
// Semua kolom layar terwakili; angka gaya Indonesia (1.234.567,64).
func ToPDF(res ByDateResult) ([]byte, error) {
	pdf := gofpdf.New("L", "mm", "LEGAL", "")
	pdf.SetTitle("Laporan Penjualan Per Tanggal "+res.From+" - "+res.To, false)
	pdf.SetAuthor("ARUS", false)
	// Margin kecil eksplisit (bawaan gofpdf boros) + jeda halaman manual penuh:
	// lebar pakai 355,6−8−8 = 339,6 mm; tabel di bawah total 339 mm.
	pdf.SetMargins(8, 6, 8)
	pdf.SetAutoPageBreak(false, 0)
	pdf.AddPage()

	// ── Kop ──────────────────────────────────────────────────────────────
	pdf.SetFont("Helvetica", "B", 13)
	pdf.CellFormat(0, 6, "LAPORAN PENJUALAN PER TANGGAL", "", 1, "C", false, 0, "")
	if res.Tenant != "" {
		pdf.SetFont("Helvetica", "B", 10)
		pdf.CellFormat(0, 5, strings.ToUpper(res.Tenant), "", 1, "C", false, 0, "")
	}
	pdf.SetFont("Helvetica", "", 9)
	outlet := res.Outlet
	if res.OutletCode != "" {
		outlet = res.OutletCode + " - " + res.Outlet
	}
	for _, line := range []string{
		fmt.Sprintf("Periode: %s s.d. %s", pdfDate(res.From), pdfDate(res.To)),
		"Outlet: " + outlet,
		"Nominal Harga Dalam Rupiah (IDR)",
	} {
		pdf.CellFormat(0, 4.5, line, "", 1, "L", false, 0, "")
	}
	pdf.Ln(1)

	// ── Ringkasan ────────────────────────────────────────────────────────
	g := SumTotal(res.Data)
	pdf.SetFont("Helvetica", "B", 9)
	pdf.CellFormat(0, 5, "Ringkasan", "", 1, "L", false, 0, "")
	pdf.SetFont("Helvetica", "", 8.5)
	var totalNota int
	for _, d := range res.Data {
		totalNota += d.SaleCount
	}
	for _, line := range [][2]string{
		{"Total Nota", fmt.Sprintf("%d", totalNota)},
		{"Total Belanja", pdfMoney(g.Total.StringFixed(2))},
		{"Total Pajak Toko", pdfMoney(g.TaxStore.StringFixed(2))},
		{"Total Pajak Negara", pdfMoney(g.TaxGov.StringFixed(2))},
		{"Total Diterima", pdfMoney(g.Paid.StringFixed(2))},
		{"Total Piutang", pdfMoney(g.Receivable.StringFixed(2))},
		{"Total Batal", pdfMoney(g.VoidTotal.StringFixed(2))},
		{"Total Retur", pdfMoney(g.ReturnTotal.StringFixed(2))},
		{"Grand Total (Bersih)", pdfMoney(g.NetTotal.StringFixed(2))},
	} {
		pdf.CellFormat(55, 4.5, line[0], "", 0, "L", false, 0, "")
		pdf.CellFormat(0, 4.5, ": "+line[1], "", 1, "R", false, 0, "")
	}
	pdf.Ln(1)

	// ── Tabel harian ─────────────────────────────────────────────────────
	header := func() {
		pdf.SetFont("Helvetica", "B", 7.5)
		pdf.SetFillColor(235, 235, 235)
		for _, c := range byDatePDFCols() {
			pdf.CellFormat(c.w, 6, c.head, "1", 0, c.align, true, 0, "")
		}
		pdf.Ln(-1)
	}
	needPage := func(h float64) {
		if pdf.GetY()+h > 200 {
			pdf.AddPage()
			header()
			pdf.SetFont("Helvetica", "", 7.5)
		}
	}
	needPage(12)
	header()
	pdf.SetFont("Helvetica", "", 7.5)
	for i, d := range res.Data {
		needPage(6)
		vals := []string{fmt.Sprintf("%d", i+1), pdfDate(d.Date), fmt.Sprintf("%d", d.SaleCount),
			fmt.Sprintf("%d", d.VoidCount), pdfMoney(d.Subtotal), pdfMoney(d.Discount),
			pdfMoney(d.TaxStore), pdfMoney(d.TaxGov), pdfMoney(d.Total), pdfMoney(d.Paid),
			pdfMoney(d.Receivable), pdfMoney(d.VoidTotal), fmt.Sprintf("%d", d.ReturnCount),
			pdfMoney(d.ReturnTotal), pdfMoney(d.NetTotal)}
		if i%2 == 1 {
			pdf.SetFillColor(248, 248, 248)
		}
		for j, c := range byDatePDFCols() {
			pdf.CellFormat(c.w, 5.5, vals[j], "1", 0, c.align, i%2 == 1, 0, "")
		}
		pdf.Ln(-1)
	}
	// Grand total menutup tabel (pindah halaman bila sisa ruang tak cukup).
	needPage(6)
	pdf.SetFont("Helvetica", "B", 7.5)
	pdf.SetFillColor(235, 235, 235)
	gt := []string{"", "Grand Total", fmt.Sprintf("%d", g.SaleCount), fmt.Sprintf("%d", g.VoidCount),
		pdfMoney(g.Subtotal.StringFixed(2)), pdfMoney(g.Discount.StringFixed(2)),
		pdfMoney(g.TaxStore.StringFixed(2)), pdfMoney(g.TaxGov.StringFixed(2)),
		pdfMoney(g.Total.StringFixed(2)), pdfMoney(g.Paid.StringFixed(2)),
		pdfMoney(g.Receivable.StringFixed(2)), pdfMoney(g.VoidTotal.StringFixed(2)),
		fmt.Sprintf("%d", g.ReturnCount), pdfMoney(g.ReturnTotal.StringFixed(2)), pdfMoney(g.NetTotal.StringFixed(2))}
	for j, c := range byDatePDFCols() {
		pdf.CellFormat(c.w, 6, gt[j], "1", 0, c.align, true, 0, "")
	}
	pdf.Ln(-1)

	var buf bytes.Buffer
	if err := pdf.Output(&buf); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}