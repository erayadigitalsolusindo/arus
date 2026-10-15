package sales

// Laporan penjualan PER BARANG: satu baris per barang (agregat seluruh nota
// berstatus completed pada rentang tanggal), tanpa isi nota.
//
// Siap big data (AGENTS.md backend):
//   - agregasi satu kali jalan: nota di rentang dibaca lewat indeks
//     sales_outlet_time_idx (batas waktu per outlet, sargable), barisnya lewat
//     UNIQUE (sale_id, position); GROUP BY item_id. Biaya ∝ jumlah baris
//     penjualan di rentang (tak terhindarkan untuk agregat), bukan ∝ katalog;
//   - jumlah baris hasil dibatasi jumlah BARANG TERJUAL (bukan jumlah nota) dan
//     dipotong LIMIT (default 100, maks 1000; ekspor maks 20.000, PDF 3.000);
//   - total keseluruhan dihitung lewat window function pada agregat yang sama
//     (tanpa pemindaian kedua); master barang/satuan/kategori di-join SETELAH
//     agregasi, hanya untuk barang yang ditemukan;
//   - pencarian (nama+kode+barcode, ILIKE per kata) memfilter hasil SETELAH agregasi,
//     jadi hanya menyentuh barang yang terjual di rentang, bukan seluruh katalog;
//   - qty dinyatakan dalam satuan dasar barang (qty × faktor satuan jual).

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/shopspring/decimal"

	"aciraba/internal/authz"
	"aciraba/internal/platform/db"
)

const (
	defaultItemLimit = 100
	maxItemLimit     = 1000
	// maxItemExportRows = batas barang pada ekspor Excel; PDF memakai maxItemPDFRows.
	maxItemExportRows = 20000
	maxItemPDFRows    = 3000
)

// ItemParams filter laporan penjualan per barang.
type ItemParams struct {
	AllOutlets bool
	OutletID   string
	From, To   string
	CashierID  string
	CategoryID string
	Search     string // potongan nama/kode/barcode barang
	Sort       string // "total" (default), "qty", "name"
	Limit      int
	ForExport  bool
	// ExportMax = batas barang ekspor (0 = maxItemExportRows); PDF memakai batas lebih kecil.
	ExportMax int
}

// ItemRow satu barang.
type ItemRow struct {
	ItemID       string `json:"item_id"`
	SKU          string `json:"sku"`
	Name         string `json:"name"`
	CategoryName string `json:"category_name"`
	UnitName     string `json:"unit_name"` // satuan dasar
	Qty          string `json:"qty"`       // dalam satuan dasar
	LineCount    int    `json:"line_count"` // kemunculan di baris nota
	Gross        string `json:"gross"`    // Σ qty × harga (sebelum diskon baris)
	Discount     string `json:"discount"` // Σ diskon baris
	Net          string `json:"net"`      // Σ line_total
}

// ItemSummary total seluruh barang (bukan hanya yang tampil).
type ItemSummary struct {
	ItemCount int    `json:"item_count"`
	Gross     string `json:"gross"`
	Discount  string `json:"discount"`
	Net       string `json:"net"`
}

// ItemResult hasil laporan per barang.
type ItemResult struct {
	From         string      `json:"from"`
	To           string      `json:"to"`
	TenantName   string      `json:"tenant_name"`
	OutletName   string      `json:"outlet_name"`
	OutletCode   string      `json:"outlet_code"`
	CashierName  string      `json:"cashier_name"`
	CategoryName string      `json:"category_name"`
	SortLabel    string      `json:"sort_label"`
	Data         []ItemRow   `json:"data"`
	Summary      ItemSummary `json:"summary"`
	// Truncated = barang yang tampil lebih sedikit dari seluruh barang terjual (LIMIT).
	Truncated bool `json:"truncated,omitempty"`
}

// itemSortSQL = whitelist ORDER BY di atas kolom agregat (nilai tak pernah dari input mentah).
var itemSortSQL = map[string]string{
	"total": "a.net DESC, a.qty DESC, a.item_id",
	"qty":   "a.qty DESC, a.net DESC, a.item_id",
	"name":  "lower(a.name), a.item_id",
}

var itemSortLabel = map[string]string{"total": "Total Penjualan", "qty": "Qty Terjual", "name": "Nama Barang"}

// itemAggCTE ($1 tenant, $2 outlet[], $3 from, $4 to, $5 cashier, $7 cari?,
// $8 pola ILIKE cari): agregat per barang, dihitung sekali. Nota dibaca lewat
// LATERAL per outlet agar batas waktu per outlet (ol.lo/ol.hi) dipakai sebagai
// kondisi indeks sales_outlet_time_idx (bukan Join Filter setelah seq scan).
const itemAggCTE = invoiceOutletsCTE + `,
a AS (
  SELECT sl.item_id,
         sum(sl.qty * sl.factor)    AS qty,
         count(*)                   AS sales,
         sum(sl.qty * sl.unit_price) AS gross,
         sum(sl.discount)           AS discount,
         sum(sl.line_total)         AS net
  FROM ol
  CROSS JOIN LATERAL (
    SELECT s.id
    FROM sales s
    WHERE s.tenant_id = $1 AND s.outlet_id = ol.id
      AND s.created_at >= ol.lo AND s.created_at < ol.hi
      AND s.status = 'completed'
      AND ($5::uuid = '00000000-0000-0000-0000-000000000000' OR s.cashier_id = $5::uuid)
    OFFSET 0 -- pagar: cegah planner meratakan LATERAL menjadi hash join + filter
  ) s
  JOIN sale_lines sl ON sl.tenant_id = $1 AND sl.sale_id = s.id
  GROUP BY sl.item_id
)
`

const itemCols = `a.item_id, i.sku, i.name, coalesce(c.name, ''), coalesce(u.name, ''),
       a.qty, a.sales::int, a.gross, a.discount, a.net,
       a.n, a.t_gross, a.t_disc, a.t_net`

const itemTotals = `count(*) OVER ()::int AS n, sum(a.gross) OVER () AS t_gross,
       sum(a.discount) OVER () AS t_disc, sum(a.net) OVER () AS t_net`

const itemJoins = `
JOIN items i ON i.tenant_id = $1 AND i.id = a.item_id
LEFT JOIN categories c ON c.tenant_id = i.tenant_id AND c.id = i.category_id
LEFT JOIN units u ON u.tenant_id = i.tenant_id AND u.id = i.unit_id`

// itemTopSQL: urut menurut nilai agregat (total/qty) tanpa filter kategori →
// top-N dipotong DI ATAS agregat; master barang di-join hanya untuk N baris.
// aggOrder = ORDER BY di atas kolom agregat. $9 = limit.
func itemTopSQL(aggOrder string) string {
	return itemAggCTE + `,
top AS (
  SELECT a.*, ` + itemTotals + `
  FROM a
  WHERE $6::uuid IS NOT NULL AND $7::bool = false AND $8::text[] IS NOT NULL
  ORDER BY ` + aggOrder + `
  LIMIT $9
)
SELECT ` + itemCols + `
FROM top a` + itemJoins + `
ORDER BY ` + aggOrder + `
`
}

// itemJoinFirstSQL: urut nama atau ada filter kategori → perlu master barang
// sebelum diurut/dipotong ($6 = kategori, $9 = limit).
func itemJoinFirstSQL(order string) string {
	return itemAggCTE + `,
b AS (
  SELECT a.*, i.sku, i.name, i.category_id, i.unit_id
  FROM a
  JOIN items i ON i.tenant_id = $1 AND i.id = a.item_id
  WHERE ($6::uuid = '00000000-0000-0000-0000-000000000000' OR i.category_id = $6::uuid)
    AND ($7::bool = false OR (i.name || ' ' || i.sku || ' ' || coalesce(i.barcode, '')) ILIKE ALL($8::text[]))
), top AS (
  SELECT a.*, ` + itemTotals + `
  FROM b a
  ORDER BY ` + order + `
  LIMIT $9
)
SELECT a.item_id, a.sku, a.name, coalesce(c.name, ''), coalesce(u.name, ''),
       a.qty, a.sales::int, a.gross, a.discount, a.net, a.n, a.t_gross, a.t_disc, a.t_net
FROM top a
LEFT JOIN categories c ON c.tenant_id = $1 AND c.id = a.category_id
LEFT JOIN units u ON u.tenant_id = $1 AND u.id = a.unit_id
ORDER BY ` + order + `
`
}

// Items: laporan penjualan per barang.
func (s *Service) Items(ctx context.Context, a authz.Actor, p ItemParams) (ItemResult, error) {
	var result ItemResult

	err := db.WithTenant(ctx, s.pool, a.TenantID, func(tx pgx.Tx) error {
		var localDay pgtype.Date
		if err := tx.QueryRow(ctx, `
			SELECT (now() AT TIME ZONE timezone)::date FROM outlets WHERE tenant_id = $1 AND id = $2`,
			a.TenantID, a.OutletID).Scan(&localDay); err != nil {
			return err
		}
		today := time.Now().UTC()
		if localDay.Valid {
			today = localDay.Time
		}
		fd, td := today, today
		fe := FieldErrors{}
		if p.From != "" {
			var perr error
			if fd, perr = time.Parse("2006-01-02", p.From); perr != nil {
				fe["from"] = "INVALID"
			}
		}
		if p.To != "" {
			var perr error
			if td, perr = time.Parse("2006-01-02", p.To); perr != nil {
				fe["to"] = "INVALID"
			}
		}
		if len(fe) == 0 && td.Before(fd) {
			fe["to"] = "MUST_BE_AFTER_FROM"
		}
		if td.Sub(fd) > maxDetailDays*24*time.Hour {
			fe["to"] = "MAX_366_DAYS"
		}
		if len(fe) > 0 {
			return fe
		}

		sortKey := strings.TrimSpace(p.Sort)
		if sortKey == "" {
			sortKey = "total"
		}
		order, ok := itemSortSQL[sortKey]
		if !ok {
			return FieldErrors{"sort": "INVALID"}
		}
		result.SortLabel = itemSortLabel[sortKey]

		cashier := uuid.Nil
		if p.CashierID != "" {
			id, err := uuid.Parse(p.CashierID)
			if err != nil || id == uuid.Nil {
				return FieldErrors{"cashier_id": "INVALID"}
			}
			cashier = id
		}
		category := uuid.Nil
		if p.CategoryID != "" {
			id, err := uuid.Parse(p.CategoryID)
			if err != nil || id == uuid.Nil {
				return FieldErrors{"category_id": "INVALID"}
			}
			category = id
		}

		limit := p.Limit
		if limit <= 0 {
			limit = defaultItemLimit
		}
		limit = min(limit, maxItemLimit)
		if p.ForExport {
			limit = maxItemExportRows
			if p.ExportMax > 0 && p.ExportMax < limit {
				limit = p.ExportMax
			}
		}

		outlets := []uuid.UUID{a.OutletID}
		if p.AllOutlets && len(a.Outlets) > 0 {
			outlets = outlets[:0]
			for id := range a.Outlets {
				outlets = append(outlets, id)
			}
		}
		if p.OutletID != "" {
			id, err := uuid.Parse(p.OutletID)
			if err != nil || id == uuid.Nil {
				return FieldErrors{"outlet_id": "INVALID"}
			}
			if id != a.OutletID && !a.Outlets[id] {
				return ErrOutletForbidden
			}
			outlets = []uuid.UUID{id}
		}

		// Pencarian: difilter SETELAH agregasi (hanya barang yang terjual di rentang,
		// bukan seluruh katalog), tiap kata harus cocok pada nama + kode + barcode.
		hasSearch := false
		pats := []string{}
		if q := strings.TrimSpace(p.Search); q != "" {
			fields := strings.Fields(q)
			if len(q) > 64 || len(fields) > 8 {
				return FieldErrors{"q": "TOO_LONG"}
			}
			for _, f := range fields {
				pats = append(pats, "%"+escapeLike(f)+"%")
			}
			hasSearch = true
		}

		result.From, result.To = fd.Format("2006-01-02"), td.Format("2006-01-02")
		if cashier != uuid.Nil {
			if err := tx.QueryRow(ctx, `SELECT name FROM users WHERE tenant_id = $1 AND id = $2`,
				a.TenantID, cashier).Scan(&result.CashierName); err != nil {
				return err
			}
		}
		if category != uuid.Nil {
			if err := tx.QueryRow(ctx, `SELECT name FROM categories WHERE tenant_id = $1 AND id = $2`,
				a.TenantID, category).Scan(&result.CategoryName); err != nil {
				return err
			}
		}
		if err := tx.QueryRow(ctx, `SELECT name FROM tenants WHERE id = $1`, a.TenantID).Scan(&result.TenantName); err != nil {
			return err
		}
		if len(outlets) == 1 {
			if err := tx.QueryRow(ctx, `SELECT name, code FROM outlets WHERE tenant_id = $1 AND id = $2`,
				a.TenantID, outlets[0]).Scan(&result.OutletName, &result.OutletCode); err != nil {
				return err
			}
		} else {
			result.OutletName = "Semua Outlet"
		}

		querySQL := itemTopSQL(order)
		if sortKey == "name" || category != uuid.Nil || hasSearch {
			querySQL = itemJoinFirstSQL(order)
		}
		rows, err := tx.Query(ctx, querySQL,
			a.TenantID, outlets, fd, td, cashier, category, hasSearch, pats, limit)
		if err != nil {
			return err
		}
		defer rows.Close()
		list := []ItemRow{}
		for rows.Next() {
			var r ItemRow
			var id uuid.UUID
			var qty, gross, disc, net, tGross, tDisc, tNet decimal.Decimal
			var total int
			if err := rows.Scan(&id, &r.SKU, &r.Name, &r.CategoryName, &r.UnitName,
				&qty, &r.LineCount, &gross, &disc, &net, &total, &tGross, &tDisc, &tNet); err != nil {
				return err
			}
			r.ItemID = id.String()
			r.Qty = qty.StringFixed(3)
			r.Gross, r.Discount, r.Net = gross.StringFixed(2), disc.StringFixed(2), net.StringFixed(2)
			list = append(list, r)
			result.Summary = ItemSummary{
				ItemCount: total, Gross: tGross.StringFixed(2),
				Discount: tDisc.StringFixed(2), Net: tNet.StringFixed(2),
			}
		}
		if err := rows.Err(); err != nil {
			return err
		}
		if len(list) == 0 {
			result.Summary = ItemSummary{Gross: "0.00", Discount: "0.00", Net: "0.00"}
		}
		result.Truncated = result.Summary.ItemCount > len(list)
		result.Data = list
		return nil
	})
	return result, err
}

// ItemFileName nama berkas ekspor per barang.
func ItemFileName(from, to, ext string) string {
	return fmt.Sprintf("penjualan-perbarang_%s_%s.%s", from, to, ext)
}
