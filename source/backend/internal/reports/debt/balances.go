package debt

import (
	"context"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/shopspring/decimal"

	"aciraba/internal/authz"
	"aciraba/internal/platform/db"
)

// BalanceRow = sisa hutang / piutang satu pihak pada tanggal as-of, dipecah menurut umur.
type BalanceRow struct {
	PartyID   string  `json:"party_id"`
	Code      string  `json:"code"`
	Name      string  `json:"name"`
	DocCount  int     `json:"doc_count"`
	Amount    string  `json:"amount"`
	Paid      string  `json:"paid"`
	Returned  string  `json:"returned"`
	Balance   string  `json:"balance"`
	Overdue   string  `json:"overdue"`
	Buckets   Buckets `json:"buckets"`
	OldestDue string  `json:"oldest_due"` // jatuh tempo tertua yang sudah lewat
}

// BalanceResult hasil laporan saldo per pihak.
type BalanceResult struct {
	Header
	Data       []BalanceRow `json:"data"`
	Summary    Summary      `json:"summary"`
	HasSummary bool         `json:"has_summary"`
	NextCursor string       `json:"next_cursor,omitempty"`
	Truncated  bool         `json:"truncated,omitempty"`
}

// balancesSQL: iterasi pihak berurut (lower(name), id) — LATERAL hanya menjumlah dokumen pihak itu,
// sehingga satu halaman cukup memindai pihak-pihak halaman itu. $1 tenant, $2 outlet[], $3 as-of,
// $4 hasCursor, $5 cursor lower(name), $6 cursor id, $7 limit, $8 cari nama/kode pihak.
func (k sqlSet) balancesSQL() string {
	docs := k.docRows(`AND d.`+k.partyCol+` = pt.id AND `+balExpr+` > 0`, ``)
	return `SELECT pt.id, coalesce(pt.code, ''), pt.name,
       a.n, a.amount, a.paid, a.cut, a.bal, a.b0, a.b1, a.b2, a.b3, a.b4, a.oldest_due, lower(pt.name)
FROM ` + k.partyTable + ` pt
CROSS JOIN LATERAL (
  SELECT ` + aggCols + `, min(x.due_date) FILTER (WHERE x.bk > 0) AS oldest_due
  FROM (SELECT x.*, ` + bucketExpr + ` AS bk FROM (` + docs + `) x) x
) a(n, amount, paid, cut, bal, b0, b1, b2, b3, b4, oldest_due)
WHERE pt.tenant_id = $1 AND a.n > 0
  AND ($8::text = '' OR pt.name ILIKE '%' || $8::text || '%' OR coalesce(pt.code, '') ILIKE '%' || $8::text || '%')
  AND ($4::bool = false OR (lower(pt.name), pt.id) > ($5::text, $6::uuid))
ORDER BY lower(pt.name), pt.id
LIMIT $7`
}

// Balances: saldo per pemasok / member pada tanggal as-of (hanya yang masih bersaldo).
func (s *Service) Balances(ctx context.Context, a authz.Actor, k Kind, p Params) (res BalanceResult, err error) {
	err = db.WithTenant(ctx, s.pool, a.TenantID, func(tx pgx.Tx) error {
		var e error
		res, e = balances(ctx, tx, a, k, p)
		return e
	})
	return
}

func balances(ctx context.Context, tx pgx.Tx, a authz.Actor, k Kind, p Params) (BalanceResult, error) {
	var result BalanceResult
	set := sets[k]
	err := func() error {
		rs, err := resolve(ctx, tx, a, k, p, need{asOf: true})
		if err != nil {
			return err
		}
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
			var sm Summary
			// ringkasan seluruh pihak; pencarian tidak memengaruhi (hanya menyaring daftar)
			if err := sm.scan(tx.QueryRow(ctx, set.summarySQL(), a.TenantID, rs.outlets, rs.asOf, zeroUUID, "")); err != nil {
				return err
			}
			result.Summary, result.HasSummary = sm, true
		}

		rows, err := tx.Query(ctx, set.balancesSQL(), a.TenantID, rs.outlets, rs.asOf, hasCursor, curName, curID, rowLimit, rs.search)
		if err != nil {
			return err
		}
		defer rows.Close()
		list := []BalanceRow{}
		var lowerNames []string
		for rows.Next() {
			var r BalanceRow
			var id uuid.UUID
			var oldest pgtype.Date
			var lname string
			var amount, paid, cut, bal decimal.Decimal
			var b [5]decimal.Decimal
			if err := rows.Scan(&id, &r.Code, &r.Name, &r.DocCount, &amount, &paid, &cut, &bal,
				&b[0], &b[1], &b[2], &b[3], &b[4], &oldest, &lname); err != nil {
				return err
			}
			r.PartyID = id.String()
			r.Amount, r.Paid, r.Returned, r.Balance = money(amount), money(paid), money(cut), money(bal)
			r.Buckets = newBuckets(b)
			r.Overdue = money(r.Buckets.overdueSum())
			r.OldestDue = dateStr(oldest)
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
			result.NextCursor = encodeCursor(list[limit-1].PartyID, lowerNames[limit-1])
		}
		result.Data = list
		return nil
	}()
	return result, err
}
