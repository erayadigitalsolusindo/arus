package sales

// Ekspor laporan penjualan PER KASIR: Excel (dua lembar: Ringkasan per kasir, Daftar shift) dan PDF landscape LEGAL.

import (
	"bytes"
	"fmt"
	"strings"

	"github.com/jung-kurt/gofpdf"
	"github.com/xuri/excelize/v2"
)

// CashierFileName nama berkas ekspor per kasir.
func CashierFileName(from, to, ext string) string {
	return fmt.Sprintf("penjualan-perkasir_%s_%s.%s", from, to, ext)
}

func cashierKop(res CashierResult) (title, tenant string, meta []string) {
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
		meta = append(meta, "Status shift: "+res.StatusLabel)
	}
	return "LAPORAN PENJUALAN PER KASIR", strings.ToUpper(res.TenantName), meta
}

// shiftNote = catatan shift + penyetuju selisih (satu sel).
func shiftNote(x CashierShift) string {
	n := x.Note
	if x.ApprovedBy != "" {
		if n != "" {
			n += " "
		}
		n += "(ACC: " + x.ApprovedBy + ")"
	}
	return n
}

func shiftStatus(x CashierShift) string {
	if x.Status == "open" {
		return "BERJALAN"
	}
	return "Tutup"
}

// CashierToXLSX: lembar "Per Kasir" (ringkasan + rincian metode) dan "Shift".
func CashierToXLSX(res CashierResult) ([]byte, error) {
	f := excelize.NewFile()
	bold, err := f.NewStyle(&excelize.Style{Font: &excelize.Font{Bold: true}})
	if err != nil {
		return nil, err
	}
	num, err := f.NewStyle(&excelize.Style{NumFmt: 4})
	if err != nil {
		return nil, err
	}
	title, tenant, meta := cashierKop(res)
	kop := func(set func(row int, vals ...any) error) (int, error) {
		r := 1
		if err := set(r, excelize.Cell{StyleID: bold, Value: title}); err != nil {
			return r, err
		}
		r++
		if tenant != "" {
			if err := set(r, excelize.Cell{StyleID: bold, Value: tenant}); err != nil {
				return r, err
			}
			r++
		}
		for _, m := range meta {
			if err := set(r, m); err != nil {
				return r, err
			}
			r++
		}
		if res.Truncated {
			if err := set(r, fmt.Sprintf("Catatan: hasil dipotong pada %d shift; persempit rentang tanggal.", len(res.Data))); err != nil {
				return r, err
			}
			r++
		}
		return r + 1, nil
	}
	money := func(s string) any { return excelize.Cell{StyleID: num, Value: moneyFloat(s)} }
	hdr := func(hs ...string) []any {
		out := make([]any, len(hs))
		for i, h := range hs {
			out[i] = excelize.Cell{StyleID: bold, Value: h}
		}
		return out
	}

	// ---- Lembar 1: per kasir
	const s1 = "Per Kasir"
	f.SetSheetName("Sheet1", s1)
	sw1, err := f.NewStreamWriter(s1)
	if err != nil {
		return nil, err
	}
	for i, w := range []float64{26, 14, 10, 10, 10, 17, 17, 17, 17, 17, 17, 17} {
		if err := sw1.SetColWidth(i+1, i+1, w); err != nil {
			return nil, err
		}
	}
	set1 := func(row int, vals ...any) error {
		cell, _ := excelize.CoordinatesToCellName(1, row)
		return sw1.SetRow(cell, vals)
	}
	r, err := kop(set1)
	if err != nil {
		return nil, err
	}
	if err := set1(r, hdr("Kasir", "Shift", "Berjalan", "Nota", "Batal", "Penjualan", "Laci Seharusnya", "Laci Fisik", "Selisih Laci",
		"Seharusnya (semua)", "Fisik (semua)", "Selisih (semua)")...); err != nil {
		return nil, err
	}
	r++
	row := func(c CashierTotal, name string) []any {
		return []any{name, c.Shifts, c.OpenShifts, c.SaleCount, c.VoidCount, money(c.SalesTotal), money(c.CashExpected), money(c.CashCounted),
			money(c.CashDiff), money(c.Expected), money(c.Counted), money(c.Diff)}
	}
	for _, c := range res.Cashiers {
		if err := set1(r, row(c, c.Name)...); err != nil {
			return nil, err
		}
		r++
		for _, m := range c.Methods {
			if err := set1(r, "   "+strings.ToUpper(m.Name), "", "", "", "", money(m.Sales), "", "", "", money(m.Expected), money(m.Counted), money(m.Diff)); err != nil {
				return nil, err
			}
			r++
		}
	}
	if res.HasSummary {
		r++
		a := row(res.Summary, "TOTAL")
		a[0] = excelize.Cell{StyleID: bold, Value: "TOTAL"}
		if err := set1(r, a...); err != nil {
			return nil, err
		}
	}
	if err := sw1.Flush(); err != nil {
		return nil, err
	}

	// ---- Lembar 2: shift
	const s2 = "Shift"
	if _, err := f.NewSheet(s2); err != nil {
		return nil, err
	}
	sw2, err := f.NewStreamWriter(s2)
	if err != nil {
		return nil, err
	}
	for i, w := range []float64{12, 7, 7, 22, 9, 22, 12, 7, 7, 16, 16, 16, 16, 16, 16, 30} {
		if err := sw2.SetColWidth(i+1, i+1, w); err != nil {
			return nil, err
		}
	}
	set2 := func(row int, vals ...any) error {
		cell, _ := excelize.CoordinatesToCellName(1, row)
		return sw2.SetRow(cell, vals)
	}
	r, err = kop(set2)
	if err != nil {
		return nil, err
	}
	if err := set2(r, hdr("Tanggal", "Buka", "Tutup", "No. Shift", "Outlet", "Kasir", "Status", "Nota", "Batal", "Modal Awal", "Penjualan",
		"Laci Seharusnya", "Laci Fisik", "Selisih Laci", "Selisih (semua)", "Catatan")...); err != nil {
		return nil, err
	}
	r++
	for _, x := range res.Data {
		if err := set2(r, x.OpenedDate, x.OpenedTime, x.ClosedTime, x.DocNo, x.OutletCode, x.CashierName, shiftStatus(x), x.SaleCount, x.VoidCount,
			money(x.Opening), money(x.SalesTotal), money(x.CashExpected), money(x.CashCounted), money(x.CashDiff), money(x.Diff), shiftNote(x)); err != nil {
			return nil, err
		}
		r++
	}
	if err := sw2.Flush(); err != nil {
		return nil, err
	}
	buf, err := f.WriteToBuffer()
	if err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// CashierToPDF: landscape LEGAL — kop, ringkasan per kasir, daftar shift.
func CashierToPDF(res CashierResult) ([]byte, error) {
	pdf := gofpdf.New("L", "mm", "LEGAL", "")
	pdf.SetTitle("Laporan Penjualan Per Kasir "+res.From+" - "+res.To, false)
	pdf.SetAuthor("ARUS", false)
	pdf.SetMargins(8, 6, 8)
	pdf.SetAutoPageBreak(false, 0)
	pdf.AddPage()

	title, tenant, meta := cashierKop(res)
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
		pdf.CellFormat(0, 4.5, fmt.Sprintf("Catatan: hasil dipotong pada %d shift; persempit rentang tanggal.", len(res.Data)), "", 1, "L", false, 0, "")
	}
	pdf.Ln(1)

	type col struct {
		head  string
		w     float64
		align string
	}
	needPage := func(h float64, header func()) {
		if pdf.GetY()+h > 200 {
			pdf.AddPage()
			header()
		}
	}
	table := func(cols []col) func() {
		return func() {
			pdf.SetFont("Helvetica", "B", 7.5)
			pdf.SetFillColor(235, 235, 235)
			for _, c := range cols {
				pdf.CellFormat(c.w, 6, c.head, "1", 0, c.align, true, 0, "")
			}
			pdf.Ln(-1)
			pdf.SetFont("Helvetica", "", 7.5)
		}
	}

	if res.HasSummary {
		pdf.SetFont("Helvetica", "B", 9)
		pdf.CellFormat(0, 5, "Ringkasan per Kasir", "", 1, "L", false, 0, "")
		cols := []col{{"Kasir", 50, "L"}, {"Shift", 16, "R"}, {"Nota", 16, "R"}, {"Batal", 16, "R"}, {"Penjualan", 36, "R"},
			{"Laci Seharusnya", 38, "R"}, {"Laci Fisik", 38, "R"}, {"Selisih Laci", 34, "R"}, {"Selisih (semua)", 34, "R"}, {"Shift Selisih", 24, "R"}}
		hd := table(cols)
		hd()
		line := func(c CashierTotal, name string, b bool) {
			needPage(5, hd)
			if b {
				pdf.SetFont("Helvetica", "B", 7.5)
			}
			cells := []string{name, fmt.Sprint(c.Shifts), fmt.Sprint(c.SaleCount), fmt.Sprint(c.VoidCount), pdfMoney(c.SalesTotal),
				pdfMoney(c.CashExpected), pdfMoney(c.CashCounted), pdfMoney(c.CashDiff), pdfMoney(c.Diff), fmt.Sprint(c.DiffShifts)}
			for j, t := range cells {
				pdf.CellFormat(cols[j].w, 5, t, "1", 0, cols[j].align, false, 0, "")
			}
			pdf.Ln(-1)
			pdf.SetFont("Helvetica", "", 7.5)
		}
		for _, c := range res.Cashiers {
			line(c, c.Name, false)
		}
		line(res.Summary, "TOTAL", true)
		pdf.Ln(3)
	}

	pdf.SetFont("Helvetica", "B", 9)
	pdf.CellFormat(0, 5, "Daftar Shift", "", 1, "L", false, 0, "")
	cols := []col{{"Tanggal", 24, "L"}, {"No. Shift", 38, "L"}, {"Kasir", 36, "L"}, {"Status", 18, "L"}, {"Nota", 12, "R"},
		{"Modal Awal", 28, "R"}, {"Penjualan", 30, "R"}, {"Laci Seharusnya", 30, "R"}, {"Laci Fisik", 30, "R"}, {"Selisih Laci", 26, "R"}, {"Catatan", 67, "L"}}
	hd := table(cols)
	hd()
	for _, x := range res.Data {
		needPage(5, hd)
		cells := []string{pdfDate(x.OpenedDate) + " " + x.OpenedTime, x.DocNo, x.CashierName, shiftStatus(x), fmt.Sprint(x.SaleCount),
			pdfMoney(x.Opening), pdfMoney(x.SalesTotal), pdfMoney(x.CashExpected), pdfMoney(x.CashCounted), pdfMoney(x.CashDiff), clip(shiftNote(x), 48)}
		for j, t := range cells {
			pdf.CellFormat(cols[j].w, 5, t, "1", 0, cols[j].align, false, 0, "")
		}
		pdf.Ln(-1)
	}

	var buf bytes.Buffer
	if err := pdf.Output(&buf); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// clip memotong teks ke n rune (sel PDF tidak membungkus).
func clip(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n-1]) + "…"
}
