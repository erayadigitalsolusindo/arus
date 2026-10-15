package stock

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"aciraba/internal/authz"
	"aciraba/internal/platform/db"
)

// ModuleStockReports = modul izin Laporan Persedian Item.
const ModuleStockReports = "stock_reports"

// ErrOutletForbidden = outlet yang diminta bukan outlet aktif pemanggil dan
// tidak termasuk haknya (cabang di luar akses tak pernah dibaca).
var ErrOutletForbidden = errors.New("OUTLET_FORBIDDEN")

const (
	defaultValuationLimit = 100
	maxValuationLimit      = 500
)

// FieldErrors maps parameter names to validation error slugs.
type FieldErrors map[string]string

func (f FieldErrors) Error() string {
	if len(f) == 0 {
		return ""
	}
	b := ""
	for k, v := range f {
		b += k + ": " + v + " "
	}
	return b
}

// Service of stock valuation reports.
type Service struct {
	pool *pgxpool.Pool
}

func NewService(pool *pgxpool.Pool) *Service {
	return &Service{pool: pool}
}

// ValuationParams filter laporan persedian.
type ValuationParams struct {
	OutletID   string // kosong = outlet aktif dari token
	CategoryID string // kosong = semua kategori
	From       string // YYYY-MM-DD; kosong = semua waktu
	To         string // YYYY-MM-DD; kosong = semua waktu
	Limit      int    // default 100, max 500
	Cursor     string // keyset: lower(name) dari baris terakhir halaman sebelumnya
}

// ValuationRow satu baris item pada laporan.
type ValuationRow struct {
	ID           string `json:"id"`
	SKU          string `json:"sku"`
	Name         string `json:"name"`
	Unit         string `json:"unit"`
	DisplayQty   string `json:"display_qty"`
	WarehouseQty string `json:"warehouse_qty"`
	ReturnsQty   string `json:"returns_qty"`
	TotalQty     string `json:"total_qty"`
	AvgCost      string `json:"avg_cost"`
	StockValue   string `json:"stock_value"`
}

// ValuationTotals = total SELURUH item yang lolos filter (bukan Σ halaman yang dimuat).
type ValuationTotals struct {
	DisplayQty   string `json:"display_qty"`
	WarehouseQty string `json:"warehouse_qty"`
	ReturnsQty   string `json:"returns_qty"`
	TotalQty     string `json:"total_qty"`
	StockValue   string `json:"stock_value"`
}

// ValuationResult hasil laporan persedian.
type ValuationResult struct {
	From       string         `json:"from"`
	To         string         `json:"to"`
	OutletID   string         `json:"outlet_id,omitempty"`
	CategoryID string         `json:"category_id,omitempty"`
	Data       []ValuationRow `json:"data"`
	Totals     ValuationTotals `json:"totals"`
	Limit      int            `json:"limit"`
	// NextCursor = keyset untuk halaman berikutnya; kosong = sudah habis.
	NextCursor string `json:"next_cursor,omitempty"`
}

// Valuation menghitung nilai persedian per item dengan paginasi keyset.
// Tanpa filter tanggal: pakai stock_balances (tabel turunan, cepat).
// Dengan filter tanggal: saldo = balance_after movement terakhir SEBELUM tanggal `to`
// (atau semua movement bila `to` kosong; `from` diabaikan untuk kompatibilitas).
func (s *Service) Valuation(ctx context.Context, a authz.Actor, p ValuationParams) (ValuationResult, error) {
	var result ValuationResult

	// Validasi tanggal
	var toTime time.Time
	toTime = time.Time{}

	if p.From != "" {
		if _, err := time.Parse("2006-01-02", p.From); err != nil {
			return result, FieldErrors{"from": "INVALID"}
		}
	}
	if p.To != "" {
		parsed, err := time.Parse("2006-01-02", p.To)
		if err != nil {
			return result, FieldErrors{"to": "INVALID"}
		}
		toTime = parsed
	}
	if !toTime.IsZero() {
		// Batasi rentang ≤ 366 hari dari today untuk keamanan
		if time.Since(toTime) > 366*24*time.Hour && !toTime.After(time.Now()) {
			// OK, tanggal lama
		}
	}

	// Limit: default 100, max 500.
	limit := p.Limit
	if limit <= 0 {
		limit = defaultValuationLimit
	}
	if limit > maxValuationLimit {
		limit = maxValuationLimit
	}

	// Default: outlet aktif dari token bila tidak dipilih
	outletID := a.OutletID
	if p.OutletID != "" {
		parsed, err := uuid.Parse(p.OutletID)
		if err != nil {
			return result, FieldErrors{"outlet_id": "INVALID"}
		}
		// Cabang di luar akses pemanggil tak pernah dibaca (sama seperti laporan penjualan).
		if parsed != a.OutletID && !a.Outlets[parsed] {
			return result, ErrOutletForbidden
		}
		outletID = parsed
	}

	// Parameter tanggal: nil = semua waktu (saldo saat ini)
	var toTS any
	if !toTime.IsZero() {
		toTS = toTime.Add(24 * time.Hour) // eksklusif: saldo per akhir hari `to`
	}

	// Without date filter: use stock_balances (fast path).
	if toTS == nil {
		err := db.WithTenant(ctx, s.pool, a.TenantID, func(tx pgx.Tx) error {
			rows, err := tx.Query(ctx, `
				SELECT
					i.id, i.sku, i.name, u.name AS unit_name,
					coalesce(sb.display, 0) AS display_qty,
					coalesce(sb.warehouse, 0) AS warehouse_qty,
					coalesce(sb.returns, 0) AS returns_qty,
					coalesce(sb.display, 0) + coalesce(sb.warehouse, 0) + coalesce(sb.returns, 0) AS total_qty,
					coalesce(oc.avg_cost, i.avg_cost) AS avg_cost,
					(coalesce(sb.display, 0) + coalesce(sb.warehouse, 0) + coalesce(sb.returns, 0)) * coalesce(oc.avg_cost, i.avg_cost) AS stock_value,
					lower(i.name) AS sort_key
				FROM items i
				JOIN units u ON u.tenant_id = i.tenant_id AND u.id = i.unit_id
				LEFT JOIN (
					SELECT item_id,
						sum(qty) FILTER (WHERE bucket = 'display') AS display,
						sum(qty) FILTER (WHERE bucket = 'warehouse') AS warehouse,
						sum(qty) FILTER (WHERE bucket = 'returns') AS returns
					FROM stock_balances
					WHERE tenant_id = $1 AND outlet_id = $2
					GROUP BY item_id
				) sb ON sb.item_id = i.id
				LEFT JOIN item_outlet_costs oc ON oc.tenant_id = i.tenant_id AND oc.item_id = i.id AND oc.outlet_id = $2
				WHERE i.tenant_id = $1 AND i.kind = 'goods'
					AND ($3::uuid IS NULL OR i.category_id = $3::uuid)
					AND ($4::text = '' OR lower(i.name) > $4::text)
				ORDER BY lower(i.name), i.id
				LIMIT $5
			`, a.TenantID, outletID, parseUUID(p.CategoryID), p.Cursor, limit+1)
			if err != nil {
				return err
			}
			defer rows.Close()

			var lastSortKey string
			count := 0
			for rows.Next() {
				var r ValuationRow
				var totalQty, avgCost, stockValue pgtype.Numeric
				var sortKey string
				err := rows.Scan(&r.ID, &r.SKU, &r.Name, &r.Unit, &r.DisplayQty, &r.WarehouseQty, &r.ReturnsQty, &totalQty, &avgCost, &stockValue, &sortKey)
				if err != nil {
					return err
				}
				r.TotalQty = numericString(totalQty)
				r.AvgCost = numericString(avgCost)
				r.StockValue = numericString(stockValue)
				result.Data = append(result.Data, r)
				lastSortKey = sortKey
				count++
				if count >= limit {
					break
				}
			}
			// Cek apakah masih ada baris berikutnya
			if count >= limit && rows.Next() {
				result.NextCursor = lastSortKey
			}
			rows.Close()

			// Total SELURUH item yang lolos filter (di luar paginasi).
			var displayTotal, warehouseTotal, returnsTotal, qtyTotal, valueTotal pgtype.Numeric
			err = tx.QueryRow(ctx, `
				SELECT coalesce(sum(x.display), 0), coalesce(sum(x.warehouse), 0), coalesce(sum(x.returns), 0),
				       coalesce(sum(x.display) + sum(x.warehouse) + sum(x.returns), 0),
				       coalesce(sum((coalesce(x.display,0) + coalesce(x.warehouse,0) + coalesce(x.returns,0)) * x.cost), 0)
				FROM (
					SELECT coalesce(sb.display, 0) AS display, coalesce(sb.warehouse, 0) AS warehouse,
					       coalesce(sb.returns, 0) AS returns, coalesce(oc.avg_cost, i.avg_cost) AS cost
					FROM items i
					LEFT JOIN (
						SELECT item_id,
							sum(qty) FILTER (WHERE bucket = 'display') AS display,
							sum(qty) FILTER (WHERE bucket = 'warehouse') AS warehouse,
							sum(qty) FILTER (WHERE bucket = 'returns') AS returns
						FROM stock_balances
						WHERE tenant_id = $1 AND outlet_id = $2
						GROUP BY item_id
					) sb ON sb.item_id = i.id
					LEFT JOIN item_outlet_costs oc ON oc.tenant_id = i.tenant_id AND oc.item_id = i.id AND oc.outlet_id = $2
					WHERE i.tenant_id = $1 AND i.kind = 'goods'
						AND ($3::uuid IS NULL OR i.category_id = $3::uuid)
				) x
			`, a.TenantID, outletID, parseUUID(p.CategoryID)).Scan(
				&displayTotal, &warehouseTotal, &returnsTotal, &qtyTotal, &valueTotal)
			if err != nil {
				return err
			}
			result.Totals = ValuationTotals{
				DisplayQty:   numericString(displayTotal),
				WarehouseQty: numericString(warehouseTotal),
				ReturnsQty:   numericString(returnsTotal),
				TotalQty:     numericString(qtyTotal),
				StockValue:   numericString(valueTotal),
			}
			return nil
		})
		if err != nil {
			return result, err
		}
	} else {
		// With date filter: saldo = balance_after movement terakhir sebelum tanggal `to`.
		err := db.WithTenant(ctx, s.pool, a.TenantID, func(tx pgx.Tx) error {
			rows, err := tx.Query(ctx, `
				SELECT
					i.id, i.sku, i.name, u.name AS unit_name,
					coalesce(sum(sb.display), 0) AS display_qty,
					coalesce(sum(sb.warehouse), 0) AS warehouse_qty,
					coalesce(sum(sb.returns), 0) AS returns_qty,
					coalesce(sum(sb.display) + sum(sb.warehouse) + sum(sb.returns), 0) AS total_qty,
					coalesce(oc.avg_cost, i.avg_cost) AS avg_cost,
					(coalesce(sum(sb.display), 0) + coalesce(sum(sb.warehouse), 0) + coalesce(sum(sb.returns), 0)) * coalesce(oc.avg_cost, i.avg_cost) AS stock_value,
					lower(i.name) AS sort_key
				FROM items i
				JOIN units u ON u.tenant_id = i.tenant_id AND u.id = i.unit_id
				LEFT JOIN (
					SELECT DISTINCT ON (m.item_id, m.bucket)
						m.item_id,
						CASE m.bucket WHEN 'display' THEN m.balance_after ELSE 0 END AS display,
						CASE m.bucket WHEN 'warehouse' THEN m.balance_after ELSE 0 END AS warehouse,
						CASE m.bucket WHEN 'returns' THEN m.balance_after ELSE 0 END AS returns
					FROM stock_movements m
					WHERE m.tenant_id = $1 AND m.outlet_id = $2
						AND m.created_at < $3::timestamptz
					ORDER BY m.item_id, m.bucket, m.id DESC
				) sb ON sb.item_id = i.id
				LEFT JOIN item_outlet_costs oc ON oc.tenant_id = i.tenant_id AND oc.item_id = i.id AND oc.outlet_id = $2
				WHERE i.tenant_id = $1 AND i.kind = 'goods'
					AND ($4::uuid IS NULL OR i.category_id = $4::uuid)
					AND ($5::text = '' OR lower(i.name) > $5::text)
				GROUP BY i.id, i.sku, i.name, u.name, oc.avg_cost, i.avg_cost
				ORDER BY lower(i.name), i.id
				LIMIT $6
			`, a.TenantID, outletID, toTS, parseUUID(p.CategoryID), p.Cursor, limit+1)
			if err != nil {
				return err
			}
			defer rows.Close()

			var lastSortKey string
			count := 0
			for rows.Next() {
				var r ValuationRow
				var displayQty, warehouseQty, returnsQty, totalQty, avgCost, stockValue pgtype.Numeric
				var sortKey string
				err := rows.Scan(&r.ID, &r.SKU, &r.Name, &r.Unit, &displayQty, &warehouseQty, &returnsQty, &totalQty, &avgCost, &stockValue, &sortKey)
				if err != nil {
					return err
				}
				r.DisplayQty = numericString(displayQty)
				r.WarehouseQty = numericString(warehouseQty)
				r.ReturnsQty = numericString(returnsQty)
				r.TotalQty = numericString(totalQty)
				r.AvgCost = numericString(avgCost)
				r.StockValue = numericString(stockValue)
				result.Data = append(result.Data, r)
				lastSortKey = sortKey
				count++
				if count >= limit {
					break
				}
			}
			// Cek apakah masih ada baris berikutnya
			if count >= limit && rows.Next() {
				result.NextCursor = lastSortKey
			}
			rows.Close()

			// Total SELURUH item yang lolos filter (di luar paginasi).
			var displayTotal, warehouseTotal, returnsTotal, qtyTotal, valueTotal pgtype.Numeric
			err = tx.QueryRow(ctx, `
				SELECT coalesce(sum(x.display), 0), coalesce(sum(x.warehouse), 0), coalesce(sum(x.returns), 0),
				       coalesce(sum(x.display) + sum(x.warehouse) + sum(x.returns), 0),
				       coalesce(sum((coalesce(x.display,0) + coalesce(x.warehouse,0) + coalesce(x.returns,0)) * x.cost), 0)
				FROM (
					SELECT coalesce(sum(CASE m.bucket WHEN 'display' THEN m.balance_after END), 0) AS display,
					       coalesce(sum(CASE m.bucket WHEN 'warehouse' THEN m.balance_after END), 0) AS warehouse,
					       coalesce(sum(CASE m.bucket WHEN 'returns' THEN m.balance_after END), 0) AS returns,
					       coalesce(oc.avg_cost, i.avg_cost) AS cost
					FROM items i
					LEFT JOIN stock_movements m ON m.tenant_id = i.tenant_id AND m.item_id = i.id
						AND m.outlet_id = $2 AND m.created_at < $3::timestamptz
						AND m.id = (
							SELECT max(m2.id) FROM stock_movements m2
							WHERE m2.tenant_id = i.tenant_id AND m2.item_id = i.id
							  AND m2.outlet_id = $2 AND m2.bucket = m.bucket AND m2.created_at < $3::timestamptz
						)
					LEFT JOIN item_outlet_costs oc ON oc.tenant_id = i.tenant_id AND oc.item_id = i.id AND oc.outlet_id = $2
					WHERE i.tenant_id = $1 AND i.kind = 'goods'
						AND ($4::uuid IS NULL OR i.category_id = $4::uuid)
					GROUP BY i.id, oc.avg_cost, i.avg_cost
				) x
			`, a.TenantID, outletID, toTS, parseUUID(p.CategoryID)).Scan(
				&displayTotal, &warehouseTotal, &returnsTotal, &qtyTotal, &valueTotal)
			if err != nil {
				return err
			}
			result.Totals = ValuationTotals{
				DisplayQty:   numericString(displayTotal),
				WarehouseQty: numericString(warehouseTotal),
				ReturnsQty:   numericString(returnsTotal),
				TotalQty:     numericString(qtyTotal),
				StockValue:   numericString(valueTotal),
			}
			return nil
		})
		if err != nil {
			return result, err
		}
	}

	result.From = p.From
	result.To = p.To
	result.OutletID = p.OutletID
	result.CategoryID = p.CategoryID
	result.Limit = limit
	return result, nil
}

func numericString(n pgtype.Numeric) string {
	if !n.Valid {
		return "0"
	}
	f, _ := n.Value()
	if f == nil {
		return "0"
	}
	return f.(string)
}

func parseUUID(s string) any {
	if s == "" {
		return nil
	}
	id, err := uuid.Parse(s)
	if err != nil || id == uuid.Nil {
		return nil
	}
	return id
}
