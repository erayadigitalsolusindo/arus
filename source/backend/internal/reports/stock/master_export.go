package stock

// Ekspor laporan master (member, supplier, sales): Excel (.xlsx, dialirkan) dan PDF landscape.
// Isi diambil dengan paginasi keyset yang sama dengan tampilan web (satu sumber kebenaran),
// dibatasi agar memori/ukuran berkas tetap wajar.

import (
	"bytes"
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jung-kurt/gofpdf"
	"github.com/xuri/excelize/v2"

	"aciraba/internal/authz"
	"aciraba/internal/platform/db"
)

const (
	maxMasterXLSXRows = 50000
	maxMasterPDFRows  = 5000
)

// MasterKinds = jenis laporan master yang dikenal.
const (
	MasterKindMembers     = "members"
	MasterKindSuppliers   = "suppliers"
	MasterKindSalespeople = "salespeople"
)

// Format angka sel Excel / PDF kolom.
const (
	fmtText = 0
	fmtInt  = 3 // #,##0
	fmtDec  = 4 // #,##0.00
)

type masterCol struct {
	Head  string
	W     float64 // lebar kolom PDF (mm); 0 = hanya di Excel
	Align string  // L | R | C
	Fmt   int
}

// MasterTable = bentuk generik laporan master untuk penulis berkas.
type MasterTable struct {
	Title     string
	Tenant    string
	Sheet     string
	FileBase  string
	Meta      []string
	Cols      []masterCol
	Rows      [][]string
	Truncated bool
}

func statusLabel(active bool) string {
	if active {
		return "Aktif"
	}
	return "Nonaktif"
}

func dmy(s string) string {
	if t, err := time.Parse("2006-01-02", s); err == nil {
		return t.Format("02-01-2006")
	}
	return s
}

// masterPage mengambil satu halaman sebagai baris teks + cursor berikutnya.
type masterPage func(ctx context.Context, a authz.Actor, p MasterParams) (rows [][]string, next string, err error)

// MasterExport mengumpulkan seluruh baris (sampai batas) untuk ekspor format "xlsx" atau "pdf".
func (s *Service) MasterExport(ctx context.Context, a authz.Actor, kind, format string, p MasterParams) (MasterTable, error) {
	var tb MasterTable
	var page masterPage

	switch kind {
	case MasterKindMembers:
		tb = MasterTable{Title: "LAPORAN MASTER MEMBER", Sheet: "Master Member", FileBase: "master-member", Cols: []masterCol{
			{"No", 8, "R", fmtText}, {"Kode", 22, "L", fmtText}, {"Nama", 52, "L", fmtText}, {"JK", 8, "C", fmtText},
			{"Telepon", 28, "L", fmtText}, {"Email", 0, "L", fmtText}, {"Alamat", 0, "L", fmtText}, {"Kota", 30, "L", fmtText},
			{"Level", 24, "L", fmtText}, {"Poin", 16, "R", fmtInt}, {"Batas Piutang", 30, "R", fmtDec},
			{"Tempo (hari)", 14, "R", fmtInt}, {"Berlaku s/d", 24, "C", fmtText}, {"Status", 20, "L", fmtText},
		}}
		page = func(ctx context.Context, a authz.Actor, p MasterParams) ([][]string, string, error) {
			res, err := s.MasterMembers(ctx, a, p)
			out := make([][]string, 0, len(res.Data))
			for _, r := range res.Data {
				gender := ""
				switch r.Gender {
				case "M":
					gender = "L"
				case "F":
					gender = "P"
				}
				out = append(out, []string{r.Code, r.Name, gender, r.Phone, r.Email, r.Address, r.City, r.Level,
					strconv.Itoa(r.Points), r.CreditLimit, strconv.Itoa(r.DueDays), dmy(r.ValidUntil), statusLabel(r.Active)})
			}
			return out, res.NextCursor, err
		}
	case MasterKindSuppliers:
		tb = MasterTable{Title: "LAPORAN MASTER SUPPLIER", Sheet: "Master Supplier", FileBase: "master-supplier", Cols: []masterCol{
			{"No", 8, "R", fmtText}, {"Kode", 22, "L", fmtText}, {"Nama", 50, "L", fmtText}, {"Kontak", 36, "L", fmtText},
			{"Telepon", 28, "L", fmtText}, {"Email", 45, "L", fmtText}, {"Alamat", 69, "L", fmtText},
			{"Catatan", 0, "L", fmtText}, {"Status", 18, "L", fmtText},
		}}
		page = func(ctx context.Context, a authz.Actor, p MasterParams) ([][]string, string, error) {
			res, err := s.MasterSuppliers(ctx, a, p)
			out := make([][]string, 0, len(res.Data))
			for _, r := range res.Data {
				out = append(out, []string{r.Code, r.Name, r.ContactName, r.Phone, r.Email, r.Address, r.Note, statusLabel(r.Active)})
			}
			return out, res.NextCursor, err
		}
	case MasterKindSalespeople:
		tb = MasterTable{Title: "LAPORAN MASTER SALES", Sheet: "Master Sales", FileBase: "master-sales", Cols: []masterCol{
			{"No", 8, "R", fmtText}, {"Kode", 24, "L", fmtText}, {"Nama", 70, "L", fmtText}, {"Telepon", 36, "L", fmtText},
			{"Komisi (%)", 24, "R", fmtDec}, {"Catatan", 92, "L", fmtText}, {"Status", 22, "L", fmtText},
		}}
		page = func(ctx context.Context, a authz.Actor, p MasterParams) ([][]string, string, error) {
			res, err := s.MasterSalespeople(ctx, a, p)
			out := make([][]string, 0, len(res.Data))
			for _, r := range res.Data {
				out = append(out, []string{r.Code, r.Name, r.Phone, r.CommissionPct, r.Note, statusLabel(r.Active)})
			}
			return out, res.NextCursor, err
		}
	default:
		return tb, FieldErrors{"kind": "INVALID"}
	}

	max := maxMasterXLSXRows
	if format == "pdf" {
		max = maxMasterPDFRows
	}
	p.Limit = maxMasterLimit
	for {
		rows, next, err := page(ctx, a, p)
		if err != nil {
			return tb, err
		}
		if room := max - len(tb.Rows); len(rows) > room {
			rows = rows[:room]
			next = "x" // masih ada sisa
		}
		tb.Rows = append(tb.Rows, rows...)
		if next == "" {
			break
		}
		if len(tb.Rows) >= max {
			tb.Truncated = true
			break
		}
		p.Cursor = next
	}

	if err := db.WithTenant(ctx, s.pool, a.TenantID, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT name FROM tenants WHERE id = $1`, a.TenantID).Scan(&tb.Tenant)
	}); err != nil {
		return tb, err
	}
	tb.Tenant = strings.ToUpper(tb.Tenant)

	st := "Semua"
	switch p.Status {
	case "active":
		st = "Aktif"
	case "inactive":
		st = "Nonaktif"
	}
	tb.Meta = []string{"Status: " + st, "Dicetak: " + time.Now().Format("02-01-2006 15:04")}
	if strings.TrimSpace(p.Q) != "" {
		tb.Meta = append(tb.Meta, "Pencarian: "+strings.TrimSpace(p.Q))
	}
	return tb, nil
}

// FileName = nama berkas unduhan, mis. master-member_2026-10-15.xlsx.
func (t MasterTable) FileName(ext string) string {
	return t.FileBase + "_" + time.Now().Format("2006-01-02") + "." + ext
}

func (t MasterTable) truncNote() string {
	return fmt.Sprintf("Catatan: hasil dipotong pada %d baris; persempit dengan pencarian/status.", len(t.Rows))
}

// ToXLSX: kop + tabel (semua kolom, termasuk yang tak muat di PDF).
func (t MasterTable) ToXLSX() ([]byte, error) {
	f := excelize.NewFile()
	if _, err := f.NewSheet(t.Sheet); err != nil {
		return nil, err
	}
	f.DeleteSheet("Sheet1")
	sw, err := f.NewStreamWriter(t.Sheet)
	if err != nil {
		return nil, err
	}
	bold, err := f.NewStyle(&excelize.Style{Font: &excelize.Font{Bold: true}})
	if err != nil {
		return nil, err
	}
	styles := map[int]int{}
	for _, nf := range []int{fmtInt, fmtDec} {
		id, err := f.NewStyle(&excelize.Style{NumFmt: nf})
		if err != nil {
			return nil, err
		}
		styles[nf] = id
	}
	for i, c := range t.Cols {
		w := 18.0
		switch {
		case i == 0:
			w = 5
		case c.Head == "Nama":
			w = 34
		case c.Head == "Alamat" || c.Head == "Catatan" || c.Head == "Email":
			w = 40
		}
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
	if t.Truncated {
		if err := put(t.truncNote()); err != nil {
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
	for i, row := range t.Rows {
		vals := make([]any, 0, len(t.Cols))
		vals = append(vals, i+1)
		for j, v := range row {
			c := t.Cols[j+1]
			if c.Fmt != fmtText {
				if n, err := strconv.ParseFloat(v, 64); err == nil {
					vals = append(vals, excelize.Cell{StyleID: styles[c.Fmt], Value: n})
					continue
				}
			}
			vals = append(vals, v)
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

// groupThousands: "1234567.50" → "1.234.567,5" (format Indonesia, desimal nol dibuang).
func groupThousands(s string) string {
	neg := strings.HasPrefix(s, "-")
	s = strings.TrimPrefix(s, "-")
	intPart, frac, _ := strings.Cut(s, ".")
	frac = strings.TrimRight(frac, "0")
	var b strings.Builder
	for i, ch := range intPart {
		if i > 0 && (len(intPart)-i)%3 == 0 {
			b.WriteByte('.')
		}
		b.WriteRune(ch)
	}
	out := b.String()
	if frac != "" {
		out += "," + frac
	}
	if neg {
		out = "-" + out
	}
	return out
}

func fitCell(pdf *gofpdf.Fpdf, s string, w float64) string {
	r := []rune(s)
	for len(r) > 1 && pdf.GetStringWidth(string(r)) > w-2 {
		r = r[:len(r)-1]
	}
	return string(r)
}

// ToPDF: A4 landscape (lebar pakai 277 mm); kolom W=0 dilewati.
func (t MasterTable) ToPDF() ([]byte, error) {
	pdf := gofpdf.New("L", "mm", "A4", "")
	pdf.SetTitle(t.Title, false)
	pdf.SetAuthor("ARUS", false)
	pdf.SetMargins(10, 8, 10)
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
	if t.Truncated {
		pdf.SetFont("Helvetica", "B", 8)
		pdf.CellFormat(0, 4.5, t.truncNote(), "", 1, "L", false, 0, "")
	}
	pdf.Ln(1.5)

	header := func() {
		pdf.SetFont("Helvetica", "B", 7.5)
		pdf.SetFillColor(235, 235, 235)
		for _, c := range t.Cols {
			if c.W > 0 {
				pdf.CellFormat(c.W, 6, c.Head, "1", 0, c.Align, true, 0, "")
			}
		}
		pdf.Ln(-1)
		pdf.SetFont("Helvetica", "", 7.5)
	}
	header()
	for i, row := range t.Rows {
		if pdf.GetY()+5 > 200 {
			pdf.AddPage()
			header()
		}
		for j, c := range t.Cols {
			if c.W == 0 {
				continue
			}
			txt := strconv.Itoa(i + 1)
			if j > 0 {
				txt = row[j-1]
				if c.Fmt != fmtText {
					txt = groupThousands(txt)
				}
			}
			pdf.CellFormat(c.W, 5, fitCell(pdf, txt, c.W), "1", 0, c.Align, false, 0, "")
		}
		pdf.Ln(-1)
	}
	var buf bytes.Buffer
	if err := pdf.Output(&buf); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
