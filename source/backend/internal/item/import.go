package item

import (
	"bytes"
	"context"
	"encoding/csv"
	"errors"
	"io"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/shopspring/decimal"
	"github.com/xuri/excelize/v2"

	"aciraba/internal/authz"
	gen "aciraba/internal/gen"
	"aciraba/internal/plan"
	"aciraba/internal/platform/db"
	"aciraba/internal/platform/sanitize"
	"aciraba/internal/stock"
)

// Import barang dari Excel/CSV (FR-ONB-01) + saldo awal stok outlet aktif (FR-ONB-03).
// Stateless: pratinjau dan simpan sama-sama menerima berkas; pratinjau tidak menulis apa pun. Baris valid disimpan
// lewat Create (batas paket, audit, validasi sama dengan input manual), baris tidak valid dilewati dan dilaporkan.

const (
	maxImportRows  = 3000
	maxImportBytes = 5 << 20
	maxErrorRows   = 1000 // baris bermasalah yang dikembalikan; sisanya hanya dihitung

	codeDuplicateInFile = "DUPLICATE_IN_FILE"
	codeNotFound        = "NOT_FOUND"
	codeCodeTaken       = "CODE_TAKEN"
	codeForbidden       = "FORBIDDEN"
	codeLocked          = "LOCKED"
	codeFailed          = "FAILED"
	codePlanLimit       = "PLAN_ITEM_LIMIT_REACHED"
)

var (
	ErrImportFormat   = errors.New("format berkas tidak didukung")
	ErrImportEmpty    = errors.New("berkas tidak berisi baris data")
	ErrImportTooMany  = errors.New("jumlah baris melebihi batas")
	ErrImportNoHeader = errors.New("kolom wajib tidak ditemukan")
)

// importCols = kolom kanonik → nama header yang diterima (huruf kecil, "_" dianggap spasi).
var importCols = map[string][]string{
	"sku":           {"kode", "sku", "code", "kode barang"},
	"barcode":       {"barcode", "barkod"},
	"name":          {"nama", "name", "nama barang"},
	"unit":          {"satuan", "unit"},
	"category":      {"kategori", "category"},
	"brand":         {"merek", "brand"},
	"cost":          {"hpp", "harga beli", "cost"},
	"sell_price":    {"harga jual", "sell price", "price"},
	"min_stock":     {"stok minimum", "min stock", "stok min"},
	"opening_stock": {"saldo awal", "stok awal", "opening stock"},
}

// importHeader = urutan kolom pada template unduhan.
var importHeader = []string{"Kode", "Barcode", "Nama", "Satuan", "Kategori", "Merek", "HPP", "Harga Jual", "Stok Minimum", "Saldo Awal"}

// ImportRow = satu baris bermasalah (atau tersimpan sebagian). Errors: kolom kanonik → kode galat.
type ImportRow struct {
	Line   int               `json:"line"` // nomor baris di berkas (header = 1)
	SKU    string            `json:"sku"`
	Name   string            `json:"name"`
	Errors map[string]string `json:"errors"`
}

type ImportResult struct {
	Total   int         `json:"total"`
	Valid   int         `json:"valid"`
	Invalid int         `json:"invalid"`
	Created int         `json:"created"` // 0 pada pratinjau
	Errors  []ImportRow `json:"errors"`
}

// ParseImport membaca xlsx (lembar pertama) atau csv (koma/titik koma) menjadi baris-baris sel; baris pertama = header.
func ParseImport(filename string, data []byte) ([][]string, error) {
	var rows [][]string
	switch {
	case strings.HasSuffix(strings.ToLower(filename), ".xlsx"):
		f, err := excelize.OpenReader(bytes.NewReader(data))
		if err != nil {
			return nil, ErrImportFormat
		}
		defer f.Close()
		sheets := f.GetSheetList()
		if len(sheets) == 0 {
			return nil, ErrImportEmpty
		}
		if rows, err = f.GetRows(sheets[0], excelize.Options{RawCellValue: true}); err != nil {
			return nil, ErrImportFormat
		}
	case strings.HasSuffix(strings.ToLower(filename), ".csv"):
		data = bytes.TrimPrefix(data, []byte("\xef\xbb\xbf"))
		first, _, _ := strings.Cut(string(data), "\n")
		r := csv.NewReader(bytes.NewReader(data))
		r.FieldsPerRecord, r.LazyQuotes = -1, true
		if strings.Count(first, ";") > strings.Count(first, ",") {
			r.Comma = ';'
		}
		for {
			rec, err := r.Read()
			if errors.Is(err, io.EOF) {
				break
			}
			if err != nil {
				return nil, ErrImportFormat
			}
			rows = append(rows, rec)
		}
	default:
		return nil, ErrImportFormat
	}
	if len(rows) < 2 {
		return nil, ErrImportEmpty
	}
	if len(rows)-1 > maxImportRows {
		return nil, ErrImportTooMany
	}
	return rows, nil
}

// headerIndex memetakan kolom kanonik → indeks kolom pada berkas. name dan unit wajib ada.
func headerIndex(header []string) (map[string]int, error) {
	idx := map[string]int{}
	for i, h := range header {
		h = strings.ToLower(strings.TrimSpace(strings.ReplaceAll(h, "_", " ")))
		for col, aliases := range importCols {
			for _, a := range aliases {
				if h == a {
					if _, dup := idx[col]; !dup {
						idx[col] = i
					}
				}
			}
		}
	}
	for _, need := range []string{"name", "unit"} {
		if _, ok := idx[need]; !ok {
			return nil, ErrImportNoHeader
		}
	}
	return idx, nil
}

// importLine = satu baris yang sudah divalidasi; In dan Opening dipakai saat simpan.
type importLine struct {
	line    int
	in      Input
	opening decimal.Decimal
	errs    map[string]string
}

func cell(row []string, idx map[string]int, col string) string {
	i, ok := idx[col]
	if !ok || i >= len(row) {
		return ""
	}
	return strings.TrimSpace(row[i])
}

// masterIDs memuat nama (huruf kecil) → id master aktif tenant.
func masterIDs(ctx context.Context, tx pgx.Tx, tenant uuid.UUID, table string) (map[string]uuid.UUID, error) {
	rows, err := tx.Query(ctx, "SELECT lower(name), id FROM "+table+" WHERE tenant_id = $1 AND active", tenant)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	m := map[string]uuid.UUID{}
	for rows.Next() {
		var n string
		var id uuid.UUID
		if err := rows.Scan(&n, &id); err != nil {
			return nil, err
		}
		m[n] = id
	}
	return m, rows.Err()
}

// resolve mengubah nama master dari sel menjadi id; "" = tidak diisi, nama tak dikenal = NOT_FOUND.
func resolve(raw string, m map[string]uuid.UUID, col string, errs map[string]string, required bool) string {
	name, ok := sanitize.Text(raw)
	switch {
	case !ok:
		errs[col] = sanitize.Invalid
	case name == "":
		if required {
			errs[col] = sanitize.Required
		}
	default:
		if id, found := m[strings.ToLower(name)]; found {
			return id.String()
		}
		errs[col] = codeNotFound
	}
	return ""
}

// checkImport memvalidasi semua baris data (tanpa menulis). Kunci ganda dalam berkas dan kode yang sudah dipakai ikut dicek.
func (s *Service) checkImport(ctx context.Context, a authz.Actor, rows [][]string) ([]importLine, error) {
	idx, err := headerIndex(rows[0])
	if err != nil {
		return nil, err
	}
	var lines []importLine
	err = db.WithTenant(ctx, s.pool, a.TenantID, func(tx pgx.Tx) error {
		units, err := masterIDs(ctx, tx, a.TenantID, "units")
		if err != nil {
			return err
		}
		cats, err := masterIDs(ctx, tx, a.TenantID, "categories")
		if err != nil {
			return err
		}
		brands, err := masterIDs(ctx, tx, a.TenantID, "brands")
		if err != nil {
			return err
		}
		st, err := gen.New(tx).StockOutletLockState(ctx, gen.StockOutletLockStateParams{TenantID: a.TenantID, ID: a.OutletID})
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return err
		}
		openingAllowed := a.Perms.Has(stock.OpeningModule, authz.ActCreate)
		locked := st.StockLockedAt.Valid

		inFile := map[string]bool{}
		var skus []string
		for i, row := range rows[1:] {
			if len(strings.Join(row, "")) == 0 {
				continue // baris kosong
			}
			l := importLine{line: i + 2, errs: map[string]string{}}
			in := Input{
				SKU: cell(row, idx, "sku"), Barcode: cell(row, idx, "barcode"), Name: cell(row, idx, "name"),
				Cost: cell(row, idx, "cost"), SellPrice: cell(row, idx, "sell_price"), MinStock: cell(row, idx, "min_stock"),
			}
			in.UnitID = resolve(cell(row, idx, "unit"), units, "unit", l.errs, true)
			in.CategoryID = resolve(cell(row, idx, "category"), cats, "category", l.errs, false)
			in.BrandID = resolve(cell(row, idx, "brand"), brands, "brand", l.errs, false)
			if in.UnitID == "" {
				in.UnitID = uuid.NewString() // galat satuan sudah dicatat; sisanya tetap divalidasi
			}
			if _, f := validate(in, true); f != nil {
				col := map[string]string{"sku": "sku", "barcode": "barcode", "name": "name", "cost": "cost", "sell_price": "sell_price", "min_stock": "min_stock"}
				for k, code := range f {
					if c, ok := col[k]; ok {
						l.errs[c] = code
					}
				}
			}
			if raw := cell(row, idx, "opening_stock"); raw != "" {
				q, err := decimal.NewFromString(raw)
				switch {
				case err != nil || q.IsNegative() || !q.Equal(q.Round(3)) || q.GreaterThanOrEqual(decimal.New(1, 12)):
					l.errs["opening_stock"] = sanitize.Invalid
				case !openingAllowed:
					l.errs["opening_stock"] = codeForbidden
				case locked && q.IsPositive():
					l.errs["opening_stock"] = codeLocked
				default:
					l.opening = q
				}
			}
			if in.SKU != "" {
				key := strings.ToLower(in.SKU)
				if inFile[key] {
					l.errs["sku"] = codeDuplicateInFile
				}
				inFile[key] = true
				skus = append(skus, in.SKU)
			}
			l.in = in
			lines = append(lines, l)
		}
		if len(skus) == 0 {
			return nil
		}
		taken, err := tx.Query(ctx, "SELECT sku FROM items WHERE tenant_id = $1 AND sku = ANY($2)", a.TenantID, skus)
		if err != nil {
			return err
		}
		defer taken.Close()
		used := map[string]bool{}
		for taken.Next() {
			var sku string
			if err := taken.Scan(&sku); err != nil {
				return err
			}
			used[strings.ToLower(sku)] = true
		}
		if err := taken.Err(); err != nil {
			return err
		}
		for i := range lines {
			if k := strings.ToLower(lines[i].in.SKU); k != "" && used[k] {
				lines[i].errs["sku"] = codeCodeTaken
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	if len(lines) == 0 {
		return nil, ErrImportEmpty
	}
	return lines, nil
}

func report(lines []importLine, res *ImportResult) {
	for _, l := range lines {
		if len(l.errs) == 0 {
			continue
		}
		res.Invalid++
		if len(res.Errors) < maxErrorRows {
			res.Errors = append(res.Errors, ImportRow{Line: l.line, SKU: l.in.SKU, Name: l.in.Name, Errors: l.errs})
		}
	}
}

// ImportPreview memvalidasi berkas tanpa menyimpan apa pun.
func (s *Service) ImportPreview(ctx context.Context, a authz.Actor, rows [][]string) (ImportResult, error) {
	lines, err := s.checkImport(ctx, a, rows)
	if err != nil {
		return ImportResult{}, err
	}
	res := ImportResult{Total: len(lines), Errors: []ImportRow{}}
	report(lines, &res)
	res.Valid = res.Total - res.Invalid
	return res, nil
}

// ImportCommit menyimpan baris valid satu per satu (tiap barang = satu transaksi, seperti input manual) lalu mencatat
// saldo awal bila ada. Baris yang gagal saat simpan dilaporkan; batas paket menghentikan sisa baris.
func (s *Service) ImportCommit(ctx context.Context, a authz.Actor, rows [][]string) (ImportResult, error) {
	lines, err := s.checkImport(ctx, a, rows)
	if err != nil {
		return ImportResult{}, err
	}
	res := ImportResult{Total: len(lines), Errors: []ImportRow{}}
	stk := stock.NewService(s.pool)
	limited := false
	for i := range lines {
		l := &lines[i]
		if len(l.errs) > 0 {
			continue
		}
		if limited {
			l.errs["name"] = codePlanLimit
			continue
		}
		it, err := s.Create(ctx, a, l.in)
		var fields FieldErrors
		switch {
		case err == nil:
		case errors.Is(err, plan.ErrItemLimit):
			limited = true
			l.errs["name"] = codePlanLimit
			continue
		case errors.Is(err, ErrCodeTaken):
			l.errs["sku"] = codeCodeTaken
			continue
		case errors.As(err, &fields):
			for k, c := range fields {
				l.errs[k] = c
			}
			continue
		default:
			return res, err // galat sistem: hentikan, baris sebelumnya sudah tersimpan
		}
		res.Created++
		if l.opening.IsPositive() {
			if _, err := stk.SetOpening(ctx, a, it.ID, stock.BucketDisplay, l.opening.String()); err != nil {
				l.errs["opening_stock"] = codeFailed // barang tersimpan; saldo awal diisi manual
			}
		}
	}
	report(lines, &res)
	res.Valid = res.Total - res.Invalid
	return res, nil
}

// ImportTemplate = template unduhan: header + dua contoh baris. format "xlsx" atau "csv".
func ImportTemplate(format string) ([]byte, error) {
	samples := [][]string{
		{"BRG-001", "8999999000012", "Contoh Barang A", "Pcs", "Umum", "", "8000", "10000", "5", "20"},
		{"", "", "Contoh Barang B", "Pcs", "", "", "", "15000", "", ""},
	}
	if format == "csv" {
		var b bytes.Buffer
		b.WriteString("\xef\xbb\xbf")
		w := csv.NewWriter(&b)
		_ = w.Write(importHeader)
		_ = w.WriteAll(samples)
		w.Flush()
		return b.Bytes(), w.Error()
	}
	f := excelize.NewFile()
	defer f.Close()
	sheet := f.GetSheetName(0)
	text, _ := f.NewStyle(&excelize.Style{NumFmt: 49}) // format Teks: barcode panjang tidak berubah jadi notasi ilmiah
	_ = f.SetColStyle(sheet, "A:B", text)
	all := append([][]string{importHeader}, samples...)
	for r, row := range all {
		for c, v := range row {
			ref, _ := excelize.CoordinatesToCellName(c+1, r+1)
			if r > 0 && (c >= 6) && v != "" { // kolom angka disimpan sebagai angka
				d, _ := decimal.NewFromString(v)
				f64, _ := d.Float64()
				_ = f.SetCellValue(sheet, ref, f64)
				continue
			}
			_ = f.SetCellStr(sheet, ref, v)
		}
	}
	_ = f.SetColWidth(sheet, "A", "J", 16)
	buf, err := f.WriteToBuffer()
	if err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
