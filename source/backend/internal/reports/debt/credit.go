package debt

import (
	"context"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/shopspring/decimal"

	"aciraba/internal/authz"
	"aciraba/internal/platform/db"
)

// CreditRow = saldo kredit toko di satu pemasok (dana kembali retur pembelian yang ditahan pemasok → toko memegang kredit)
// pada periode: saldo awal + bertambah − dipakai bayar hutang − dicairkan − batal retur = saldo akhir.
type CreditRow struct {
	SupplierID string `json:"supplier_id"`
	Code       string `json:"code"`
	Name       string `json:"name"`
	Opening    string `json:"opening"`
	Added      string `json:"added"`      // dari retur pembelian
	Used       string `json:"used"`       // dipakai membayar hutang
	CashedOut  string `json:"cashed_out"` // dicairkan pemasok ke toko
	Voided     string `json:"voided"`     // retur yang dibatalkan
	Closing    string `json:"closing"`
}

// CreditSummary = total seluruh pemasok.
type CreditSummary struct {
	SupplierCount int    `json:"supplier_count"` // pemasok yang masih bersaldo pada akhir periode
	Opening       string `json:"opening"`
	Added         string `json:"added"`
	Used          string `json:"used"`
	CashedOut     string `json:"cashed_out"`
	Voided        string `json:"voided"`
	Closing       string `json:"closing"`
}

// CreditResult hasil laporan saldo di pemasok.
type CreditResult struct {
	Header
	Data       []CreditRow   `json:"data"`
	Summary    CreditSummary `json:"summary"`
	HasSummary bool          `json:"has_summary"`
	NextCursor string        `json:"next_cursor,omitempty"`
	Truncated  bool          `json:"truncated,omitempty"`
}

// Batas waktu: [from 00:00, to+1 00:00) di zona waktu outlet aktif ($4 = zona). Saldo kredit tenant-wide
// per pemasok (tidak per outlet), sehingga laporan tidak menyaring outlet.
const creditBounds = `($2::date)::timestamp AT TIME ZONE $4::text`
const creditEnd = `(($3::date) + 1)::timestamp AT TIME ZONE $4::text`

// creditAgg = agregat gerakan kredit satu pemasok (alias m), alias hasil a.
const creditAgg = `
  SELECT coalesce(sum(m.amount) FILTER (WHERE m.created_at < ` + creditBounds + `), 0) AS opening,
         coalesce(sum(m.amount) FILTER (WHERE m.kind = 'PURCHASE_RETURN' AND m.created_at >= ` + creditBounds + `), 0) AS added,
         coalesce(-sum(m.amount) FILTER (WHERE m.kind = 'PAYABLE_PAYMENT' AND m.created_at >= ` + creditBounds + `), 0) AS used,
         coalesce(-sum(m.amount) FILTER (WHERE m.kind = 'CASH_OUT' AND m.created_at >= ` + creditBounds + `), 0) AS cashed_out,
         coalesce(-sum(m.amount) FILTER (WHERE m.kind = 'PURCHASE_RETURN_VOID' AND m.created_at >= ` + creditBounds + `), 0) AS voided,
         coalesce(sum(m.amount), 0) AS closing`

// creditPageSQL: $1 tenant, $2 from, $3 to, $4 zona, $5 cari, $6 hasCursor, $7 cursor lower(name), $8 cursor id, $9 limit.
const creditPageSQL = `
SELECT s.id, coalesce(s.code, ''), s.name, a.opening, a.added, a.used, a.cashed_out, a.voided, a.closing, lower(s.name)
FROM suppliers s
CROSS JOIN LATERAL (` + creditAgg + `
  FROM supplier_credit_movements m
  WHERE m.tenant_id = $1 AND m.supplier_id = s.id AND m.created_at < ` + creditEnd + `
) a
WHERE s.tenant_id = $1
  AND (a.opening <> 0 OR a.added <> 0 OR a.used <> 0 OR a.cashed_out <> 0 OR a.voided <> 0)
  AND ($5::text = '' OR s.name ILIKE '%' || $5::text || '%' OR coalesce(s.code, '') ILIKE '%' || $5::text || '%')
  AND ($6::bool = false OR (lower(s.name), s.id) > ($7::text, $8::uuid))
ORDER BY lower(s.name), s.id
LIMIT $9`

// creditSummarySQL: $1 tenant, $2 from, $3 to, $4 zona.
const creditSummarySQL = `
SELECT count(*) FILTER (WHERE g.closing > 0)::int,
       coalesce(sum(g.opening), 0), coalesce(sum(g.added), 0), coalesce(sum(g.used), 0),
       coalesce(sum(g.cashed_out), 0), coalesce(sum(g.voided), 0), coalesce(sum(g.closing), 0)
FROM (
  SELECT m.supplier_id,
         coalesce(sum(m.amount) FILTER (WHERE m.created_at < ` + creditBounds + `), 0) AS opening,
         coalesce(sum(m.amount) FILTER (WHERE m.kind = 'PURCHASE_RETURN' AND m.created_at >= ` + creditBounds + `), 0) AS added,
         coalesce(-sum(m.amount) FILTER (WHERE m.kind = 'PAYABLE_PAYMENT' AND m.created_at >= ` + creditBounds + `), 0) AS used,
         coalesce(-sum(m.amount) FILTER (WHERE m.kind = 'CASH_OUT' AND m.created_at >= ` + creditBounds + `), 0) AS cashed_out,
         coalesce(-sum(m.amount) FILTER (WHERE m.kind = 'PURCHASE_RETURN_VOID' AND m.created_at >= ` + creditBounds + `), 0) AS voided,
         coalesce(sum(m.amount), 0) AS closing
  FROM supplier_credit_movements m
  WHERE m.tenant_id = $1 AND m.created_at < ` + creditEnd + `
  GROUP BY m.supplier_id
) g`

// Credit: saldo kredit toko di tiap pemasok pada rentang tanggal (hanya yang punya saldo awal atau gerakan).
func (s *Service) Credit(ctx context.Context, a authz.Actor, p Params) (res CreditResult, err error) {
	err = db.WithTenant(ctx, s.pool, a.TenantID, func(tx pgx.Tx) error {
		var e error
		res, e = credit(ctx, tx, a, p)
		return e
	})
	return
}

func credit(ctx context.Context, tx pgx.Tx, a authz.Actor, p Params) (CreditResult, error) {
	var result CreditResult
	err := func() error {
		rs, err := resolve(ctx, tx, a, KindPayable, p, need{period: true})
		if err != nil {
			return err
		}
		rs.hdr.OutletName, rs.hdr.OutletCode = "Semua Outlet", ""
		result.Header = rs.hdr

		hasCursor := p.Cursor != "" && !p.ForExport
		var curName string
		curID := zeroUUID
		if hasCursor {
			parts, ok := decodeCursor(p.Cursor, 2)
			if !ok {
				return FieldErrors{"cursor": "INVALID"}
			}
			id, err := uuid.Parse(parts[0])
			if err != nil {
				return FieldErrors{"cursor": "INVALID"}
			}
			curID, curName = id, parts[1]
		}
		limit := clampLimit(p.Limit)
		rowLimit := limit + 1
		if p.ForExport {
			rowLimit = exportMax(p) + 1
		}

		if !hasCursor {
			var sm CreditSummary
			var d [6]decimal.Decimal
			if err := tx.QueryRow(ctx, creditSummarySQL, a.TenantID, rs.from, rs.to, rs.tz).
				Scan(&sm.SupplierCount, &d[0], &d[1], &d[2], &d[3], &d[4], &d[5]); err != nil {
				return err
			}
			sm.Opening, sm.Added, sm.Used, sm.CashedOut, sm.Voided, sm.Closing =
				money(d[0]), money(d[1]), money(d[2]), money(d[3]), money(d[4]), money(d[5])
			result.Summary, result.HasSummary = sm, true
		}

		rows, err := tx.Query(ctx, creditPageSQL, a.TenantID, rs.from, rs.to, rs.tz, rs.search,
			hasCursor, curName, curID, rowLimit)
		if err != nil {
			return err
		}
		defer rows.Close()
		list := []CreditRow{}
		var lowerNames []string
		for rows.Next() {
			var r CreditRow
			var id uuid.UUID
			var d [6]decimal.Decimal
			var lname string
			if err := rows.Scan(&id, &r.Code, &r.Name, &d[0], &d[1], &d[2], &d[3], &d[4], &d[5], &lname); err != nil {
				return err
			}
			r.SupplierID = id.String()
			r.Opening, r.Added, r.Used, r.CashedOut, r.Voided, r.Closing =
				money(d[0]), money(d[1]), money(d[2]), money(d[3]), money(d[4]), money(d[5])
			list = append(list, r)
			lowerNames = append(lowerNames, lname)
		}
		if err := rows.Err(); err != nil {
			return err
		}
		if p.ForExport {
			if m := exportMax(p); len(list) > m {
				list = list[:m]
				result.Truncated = true
			}
		} else if len(list) > limit {
			list = list[:limit]
			result.NextCursor = encodeCursor(list[limit-1].SupplierID, lowerNames[limit-1])
		}
		result.Data = list
		return nil
	}()
	return result, err
}
