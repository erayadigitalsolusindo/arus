package xtable

// Package xtable = renderer ekspor tabel generik untuk laporan: Excel (.xlsx, dialirkan)
// dan PDF (landscape LEGAL). Tiap laporan membangun satu Table; renderer XLSX/PDF-nya
// sama, sehingga kop, format angka, dan batas baris konsisten.

import (
	"bytes"
	"fmt"
	"strings"

	"github.com/jung-kurt/gofpdf"
	"github.com/shopspring/decimal"
	"github.com/xuri/excelize/v2"
)

// Column = satu kolom tabel.
type Column struct {
	Head  string
	Width float64 // lebar PDF (mm); total ≤ 339
	XW    float64 // lebar kolom Excel
	Money bool    // nilai desimal → angka (Excel) / format Indonesia (PDF)
}

// Table = isi satu berkas ekspor.
type Table struct {
	Title, Tenant string
	Meta          []string
	Note          string // mis. catatan pemotongan
	Cols          []Column
	Rows          [][]string
	Total         []string // baris grand total (kosong = tanpa)
}

// PDFMoney memformat desimal ke gaya Indonesia (1.234.567,50).
func PDFMoney(s string) string {
	d, err := decimal.NewFromString(strings.TrimSpace(s))
	if err != nil {
		return "0,00"
	}
	neg := d.IsNegative()
	d = d.Abs()
	intPart := d.IntPart()
	frac := d.Sub(decimal.NewFromInt(intPart)).Mul(decimal.NewFromInt(100)).Round(0).IntPart()
	var groups []string
	for intPart >= 1000 {
		groups = append([]string{fmt.Sprintf("%03d", intPart%1000)}, groups...)
		intPart /= 1000
	}
	groups = append([]string{fmt.Sprintf("%d", intPart)}, groups...)
	out := strings.Join(groups, ".") + fmt.Sprintf(",%02d", frac)
	if neg {
		out = "-" + out
	}
	return out
}

// PDFDate mengubah YYYY-MM-DD menjadi DD-MM-YYYY.
func PDFDate(ymd string) string {
	if len(ymd) == 10 && ymd[4] == '-' && ymd[7] == '-' {
		return ymd[8:10] + "-" + ymd[5:7] + "-" + ymd[0:4]
	}
	return ymd
}

func moneyFloat(s string) float64 {
	d, err := decimal.NewFromString(strings.TrimSpace(s))
	if err != nil {
		return 0
	}
	f, _ := d.Float64()
	return f
}

func (t Table) ToXLSX() ([]byte, error) {
	f := excelize.NewFile()
	const sheet = "Laporan"
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
	for i, c := range t.Cols {
		if err := sw.SetColWidth(i+1, i+1, c.XW); err != nil {
			return nil, err
		}
	}
	r := 1
	put := func(vals ...any) error {
		cell, _ := excelize.CoordinatesToCellName(1, r)
		r++
		return sw.SetRow(cell, vals)
	}
	if err := put(excelize.Cell{StyleID: bold, Value: t.Title}); err != nil {
		return nil, err
	}
	if t.Tenant != "" {
		if err := put(excelize.Cell{StyleID: bold, Value: t.Tenant}); err != nil {
			return nil, err
		}
	}
	for _, m := range t.Meta {
		if err := put(m); err != nil {
			return nil, err
		}
	}
	if t.Note != "" {
		if err := put(t.Note); err != nil {
			return nil, err
		}
	}
	if err := put(); err != nil {
		return nil, err
	}
	head := make([]any, len(t.Cols))
	for i, c := range t.Cols {
		head[i] = excelize.Cell{StyleID: bold, Value: c.Head}
	}
	if err := put(head...); err != nil {
		return nil, err
	}
	cell := func(i int, v string, style int) any {
		if t.Cols[i].Money && v != "" {
			return excelize.Cell{StyleID: num, Value: moneyFloat(v)}
		}
		if style != 0 {
			return excelize.Cell{StyleID: style, Value: v}
		}
		return v
	}
	for _, row := range t.Rows {
		vals := make([]any, len(row))
		for i, v := range row {
			vals[i] = cell(i, v, 0)
		}
		if err := put(vals...); err != nil {
			return nil, err
		}
	}
	if len(t.Total) > 0 {
		if err := put(); err != nil {
			return nil, err
		}
		vals := make([]any, len(t.Total))
		for i, v := range t.Total {
			vals[i] = cell(i, v, bold)
		}
		if err := put(vals...); err != nil {
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

func (t Table) ToPDF() ([]byte, error) {
	pdf := gofpdf.New("L", "mm", "LEGAL", "")
	pdf.SetTitle(t.Title, false)
	pdf.SetAuthor("ARUS", false)
	pdf.SetMargins(8, 6, 8)
	pdf.SetAutoPageBreak(false, 0)
	pdf.AddPage()

	pdf.SetFont("Helvetica", "B", 13)
	pdf.CellFormat(0, 6, t.Title, "", 1, "C", false, 0, "")
	if t.Tenant != "" {
		pdf.SetFont("Helvetica", "B", 10)
		pdf.CellFormat(0, 5, t.Tenant, "", 1, "C", false, 0, "")
	}
	pdf.SetFont("Helvetica", "", 9)
	for _, m := range t.Meta {
		pdf.CellFormat(0, 4.5, m, "", 1, "L", false, 0, "")
	}
	if t.Note != "" {
		pdf.SetFont("Helvetica", "B", 8)
		pdf.CellFormat(0, 4.5, t.Note, "", 1, "L", false, 0, "")
	}
	pdf.Ln(1)

	align := func(c Column) string {
		if c.Money {
			return "R"
		}
		return "L"
	}
	header := func() {
		pdf.SetFont("Helvetica", "B", 7.5)
		pdf.SetFillColor(235, 235, 235)
		for _, c := range t.Cols {
			pdf.CellFormat(c.Width, 6, c.Head, "1", 0, align(c), true, 0, "")
		}
		pdf.Ln(-1)
		pdf.SetFont("Helvetica", "", 7.5)
	}
	row := func(vals []string, bold bool, fill bool) {
		if pdf.GetY()+5 > 200 {
			pdf.AddPage()
			header()
		}
		if bold {
			pdf.SetFont("Helvetica", "B", 7.5)
		}
		for i, c := range t.Cols {
			v := ""
			if i < len(vals) {
				v = vals[i]
			}
			if c.Money && v != "" {
				v = PDFMoney(v)
			}
			pdf.CellFormat(c.Width, 5, fitText(pdf, v, c.Width-1.5), "1", 0, align(c), fill, 0, "")
		}
		pdf.Ln(-1)
		if bold {
			pdf.SetFont("Helvetica", "", 7.5)
		}
	}
	header()
	for _, rv := range t.Rows {
		row(rv, false, false)
	}
	if len(t.Total) > 0 {
		pdf.SetFillColor(225, 232, 240)
		row(t.Total, true, true)
	}
	var buf bytes.Buffer
	if err := pdf.Output(&buf); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// fitText memotong teks agar muat di lebar sel (tanpa wrap; PDF tabel padat).
func fitText(pdf *gofpdf.Fpdf, s string, w float64) string {
	if pdf.GetStringWidth(s) <= w {
		return s
	}
	rs := []rune(s)
	for len(rs) > 1 && pdf.GetStringWidth(string(rs)+"..") > w {
		rs = rs[:len(rs)-1]
	}
	return string(rs) + ".."
}

// Col membuat kolom: lebar PDF (mm; total ≤ 339), lebar Excel, dan penanda kolom uang.
func Col(head string, pdfWidth, xlsxWidth float64, money bool) Column {
	return Column{Head: head, Width: pdfWidth, XW: xlsxWidth, Money: money}
}
