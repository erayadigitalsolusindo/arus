package purchases

// Ekspor laporan pembelian: Excel (.xlsx, dialirkan) dan PDF (landscape LEGAL).
// Keempat laporan membangun satu `table` generik; renderer XLSX/PDF-nya sama,
// sehingga kop, format angka, dan batas baris konsisten.

import (
	"bytes"
	"fmt"
	"strings"

	"github.com/jung-kurt/gofpdf"
	"github.com/shopspring/decimal"
	"github.com/xuri/excelize/v2"
)

type column struct {
	head  string
	width float64 // lebar PDF (mm); total ≤ 339
	xw    float64 // lebar kolom Excel
	money bool    // nilai desimal → angka (Excel) / format Indonesia (PDF)
}

// table = isi satu berkas ekspor.
type table struct {
	title, tenant string
	meta          []string
	note          string // mis. catatan pemotongan
	cols          []column
	rows          [][]string
	total         []string // baris grand total (kosong = tanpa)
}

func pdfMoney(s string) string {
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

func pdfDate(ymd string) string {
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
		fmt.Sprintf("Periode: %s s.d. %s", pdfDate(h.From), pdfDate(h.To)),
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

func (t table) toXLSX() ([]byte, error) {
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
	for i, c := range t.cols {
		if err := sw.SetColWidth(i+1, i+1, c.xw); err != nil {
			return nil, err
		}
	}
	r := 1
	put := func(vals ...any) error {
		cell, _ := excelize.CoordinatesToCellName(1, r)
		r++
		return sw.SetRow(cell, vals)
	}
	if err := put(excelize.Cell{StyleID: bold, Value: t.title}); err != nil {
		return nil, err
	}
	if t.tenant != "" {
		if err := put(excelize.Cell{StyleID: bold, Value: t.tenant}); err != nil {
			return nil, err
		}
	}
	for _, m := range t.meta {
		if err := put(m); err != nil {
			return nil, err
		}
	}
	if t.note != "" {
		if err := put(t.note); err != nil {
			return nil, err
		}
	}
	if err := put(); err != nil {
		return nil, err
	}
	head := make([]any, len(t.cols))
	for i, c := range t.cols {
		head[i] = excelize.Cell{StyleID: bold, Value: c.head}
	}
	if err := put(head...); err != nil {
		return nil, err
	}
	cell := func(i int, v string, style int) any {
		if t.cols[i].money && v != "" {
			return excelize.Cell{StyleID: num, Value: moneyFloat(v)}
		}
		if style != 0 {
			return excelize.Cell{StyleID: style, Value: v}
		}
		return v
	}
	for _, row := range t.rows {
		vals := make([]any, len(row))
		for i, v := range row {
			vals[i] = cell(i, v, 0)
		}
		if err := put(vals...); err != nil {
			return nil, err
		}
	}
	if len(t.total) > 0 {
		if err := put(); err != nil {
			return nil, err
		}
		vals := make([]any, len(t.total))
		for i, v := range t.total {
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

func (t table) toPDF() ([]byte, error) {
	pdf := gofpdf.New("L", "mm", "LEGAL", "")
	pdf.SetTitle(t.title, false)
	pdf.SetAuthor("ARUS", false)
	pdf.SetMargins(8, 6, 8)
	pdf.SetAutoPageBreak(false, 0)
	pdf.AddPage()

	pdf.SetFont("Helvetica", "B", 13)
	pdf.CellFormat(0, 6, t.title, "", 1, "C", false, 0, "")
	if t.tenant != "" {
		pdf.SetFont("Helvetica", "B", 10)
		pdf.CellFormat(0, 5, t.tenant, "", 1, "C", false, 0, "")
	}
	pdf.SetFont("Helvetica", "", 9)
	for _, m := range t.meta {
		pdf.CellFormat(0, 4.5, m, "", 1, "L", false, 0, "")
	}
	if t.note != "" {
		pdf.SetFont("Helvetica", "B", 8)
		pdf.CellFormat(0, 4.5, t.note, "", 1, "L", false, 0, "")
	}
	pdf.Ln(1)

	align := func(c column) string {
		if c.money {
			return "R"
		}
		return "L"
	}
	header := func() {
		pdf.SetFont("Helvetica", "B", 7.5)
		pdf.SetFillColor(235, 235, 235)
		for _, c := range t.cols {
			pdf.CellFormat(c.width, 6, c.head, "1", 0, align(c), true, 0, "")
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
		for i, c := range t.cols {
			v := ""
			if i < len(vals) {
				v = vals[i]
			}
			if c.money && v != "" {
				v = pdfMoney(v)
			}
			pdf.CellFormat(c.width, 5, fitText(pdf, v, c.width-1.5), "1", 0, align(c), fill, 0, "")
		}
		pdf.Ln(-1)
		if bold {
			pdf.SetFont("Helvetica", "", 7.5)
		}
	}
	header()
	for _, rv := range t.rows {
		row(rv, false, false)
	}
	if len(t.total) > 0 {
		pdf.SetFillColor(225, 232, 240)
		row(t.total, true, true)
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
	t := table{title: title, tenant: tenant, meta: meta, note: truncNote(res.Truncated, len(res.Data), "faktur"),
		cols: []column{
			{"Tanggal", 22, 12, false}, {"Faktur", 38, 22, false}, {"Faktur Pemasok", 36, 20, false},
			{"Pemasok", 50, 28, false}, {"Bayar", 18, 9, false}, {"Jatuh Tempo", 22, 12, false},
			{"Status", 16, 9, false}, {"Subtotal", 32, 16, true}, {"PPN", 28, 14, true},
			{"Biaya Lain", 28, 14, true}, {"Total", 49, 16, true},
		}}
	for _, x := range res.Data {
		t.rows = append(t.rows, []string{pdfDate(x.Date), x.DocNo, x.SupplierInvoiceNo, x.SupplierName,
			payLabel(x.PaymentType), pdfDate(x.DueDate), statusMark(x.Status),
			x.Subtotal, x.TaxAmount, x.OtherCost, x.Total})
	}
	if res.HasSummary {
		s := res.Summary
		t.total = []string{"GRAND TOTAL", fmt.Sprintf("%d faktur", s.PurchaseCount), "", "", "", "",
			fmt.Sprintf("batal %d", s.VoidCount), s.Subtotal, s.TaxAmount, s.OtherCost, s.Total}
	}
	return t
}

// InvoiceDetailTable: satu baris per barang (data faktur diulang di tiap baris).
func InvoiceDetailTable(res InvoiceResult) table {
	title, tenant, meta := kop("LAPORAN PEMBELIAN PER FAKTUR DETAIL", res.Header, statusLine(res.StatusLabel))
	t := table{title: title, tenant: tenant, meta: meta, note: truncNote(res.Truncated, len(res.Data), "faktur"),
		cols: []column{
			{"Tanggal", 22, 12, false}, {"Faktur", 36, 22, false}, {"Pemasok", 44, 28, false},
			{"Bayar", 16, 9, false}, {"SKU", 28, 14, false}, {"Barang", 62, 36, false},
			{"Satuan", 16, 9, false}, {"Qty", 20, 10, false}, {"Harga", 28, 14, true},
			{"Diskon %", 28, 14, false}, {"Jumlah", 39, 16, true},
		}}
	for _, x := range res.Data {
		doc := x.DocNo
		if x.Status == "void" {
			doc += " (BATAL)"
		}
		for _, l := range x.Lines {
			t.rows = append(t.rows, []string{pdfDate(x.Date), doc, x.SupplierName, payLabel(x.PaymentType),
				l.SKU, l.Name, l.Unit, l.Qty, l.UnitPrice, discText(l), l.LineTotal})
		}
	}
	if res.HasSummary {
		t.total = []string{"GRAND TOTAL", fmt.Sprintf("%d faktur", res.Summary.PurchaseCount), "", "", "", "", "", "", "", "", res.Summary.Subtotal}
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
	t := table{title: title, tenant: tenant, meta: meta,
		note: truncNote(res.Truncated, len(res.Data), "barang"),
		cols: []column{
			{"No", 12, 6, false}, {"SKU", 34, 16, false}, {"Barang", 100, 40, false}, {"Satuan", 20, 10, false},
			{"Faktur", 18, 9, false}, {"Qty", 28, 12, false}, {"Harga Rata-rata", 38, 16, true},
			{"Harga Terakhir", 38, 16, true}, {"Total", 51, 18, true},
		}}
	for i, x := range res.Data {
		t.rows = append(t.rows, []string{fmt.Sprintf("%d", i+1), x.SKU, x.Name, x.Unit,
			fmt.Sprintf("%d", x.PurchaseCount), x.Qty, x.AvgPrice, x.LastPrice, x.Total})
	}
	t.total = []string{"", "GRAND TOTAL", fmt.Sprintf("%d barang", res.Summary.ItemCount), "", "", "", "", "", res.Summary.Total}
	return t
}

// ByDateTable: satu baris per tanggal.
func ByDateTable(res ByDateResult) table {
	title, tenant, meta := kop("LAPORAN PEMBELIAN PER TANGGAL", res.Header)
	t := table{title: title, tenant: tenant, meta: meta,
		cols: []column{
			{"Tanggal", 24, 12, false}, {"Faktur", 16, 8, false}, {"Batal", 14, 7, false},
			{"Subtotal", 30, 16, true}, {"PPN", 26, 14, true}, {"Biaya Lain", 26, 14, true},
			{"Total", 32, 16, true}, {"Tunai", 30, 16, true}, {"Kredit", 30, 16, true},
			{"Retur", 14, 7, false}, {"Total Retur", 30, 16, true}, {"Bersih", 67, 16, true},
		}}
	add := func(label string, r DayRow) []string {
		return []string{label, fmt.Sprintf("%d", r.PurchaseCount), fmt.Sprintf("%d", r.VoidCount),
			r.Subtotal, r.TaxAmount, r.OtherCost, r.Total, r.CashTotal, r.CreditTotal,
			fmt.Sprintf("%d", r.ReturnCount), r.ReturnTotal, r.NetTotal}
	}
	for _, x := range res.Data {
		t.rows = append(t.rows, add(pdfDate(x.Date), x))
	}
	t.total = add("GRAND TOTAL", res.Totals)
	return t
}
