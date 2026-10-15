package debt

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/shopspring/decimal"

	"aciraba/internal/authz"
	"aciraba/internal/platform/db"
)

// OutstandingRow = satu dokumen yang masih punya sisa pada tanggal as-of.
type OutstandingRow struct {
	ID          string `json:"id"`
	DocNo       string `json:"doc_no"`
	RefNo       string `json:"ref_no"` // no. faktur pemasok / no. referensi saldo awal
	OutletCode  string `json:"outlet_code"`
	PartyCode   string `json:"party_code"`
	PartyName   string `json:"party_name"`
	DocDate     string `json:"doc_date"`
	DueDate     string `json:"due_date"`
	Amount      string `json:"amount"`
	Paid        string `json:"paid"`
	Returned    string `json:"returned"` // dipotong retur
	Balance     string `json:"balance"`
	DaysOverdue int    `json:"days_overdue"`
	Bucket      int    `json:"bucket"` // 0 belum jatuh tempo, 1: 1-30, 2: 31-60, 3: 61-90, 4: >90 hari
}

// Summary = total seluruh hasil filter (bukan hanya halaman yang tampil).
type Summary struct {
	DocCount   int     `json:"doc_count"`
	PartyCount int     `json:"party_count,omitempty"`
	Amount     string  `json:"amount"`
	Paid       string  `json:"paid"`
	Returned   string  `json:"returned"`
	Balance    string  `json:"balance"`
	Overdue    string  `json:"overdue"`
	Buckets    Buckets `json:"buckets"`
}

// OutstandingResult hasil laporan beredar.
type OutstandingResult struct {
	Header
	Data       []OutstandingRow `json:"data"`
	Summary    Summary          `json:"summary"`
	HasSummary bool             `json:"has_summary"`
	NextCursor string           `json:"next_cursor,omitempty"`
	Truncated  bool             `json:"truncated,omitempty"`
}

const zeroUUIDLit = `'00000000-0000-0000-0000-000000000000'`

// partyCond: $4 = pihak (UUID nol = semua).
func (k sqlSet) partyCond() string {
	return `AND ($4::uuid = ` + zeroUUIDLit + ` OR d.` + k.partyCol + ` = $4::uuid)`
}

// searchCond: $5 = potongan nomor dokumen / referensi.
func (k sqlSet) searchCond() string {
	return `AND ($5::text = '' OR ` + k.docNo + ` ILIKE '%' || $5::text || '%' OR ` + k.refNo + ` ILIKE '%' || $5::text || '%')`
}

// outstandingFilter = syarat beredar: sisa > 0 + pihak + pencarian.
func (k sqlSet) outstandingFilter() string {
	return `AND ` + balExpr + ` > 0 ` + k.partyCond() + ` ` + k.searchCond()
}

// outstandingPageSQL: $1 tenant, $2 outlet[], $3 as-of, $4 pihak, $5 cari, $6 hasCursor,
// $7 cursor created_at, $8 cursor id, $9 limit. Relasi pihak di-join setelah dipotong.
func (k sqlSet) outstandingPageSQL() string {
	inner := k.docRows(k.outstandingFilter()+`
  AND ($6::bool = false OR (d.created_at, d.id) < ($7::timestamptz, $8::uuid))`,
		`ORDER BY d.created_at DESC, d.id DESC LIMIT $9`)
	return `SELECT r.id, o.code, r.doc_no, r.ref_no, coalesce(pt.code, ''), pt.name, r.doc_date, r.due_date,
       r.amount, r.paid, r.cut, r.bal, r.created_at
FROM (` + inner + `) r
JOIN outlets o ON o.tenant_id = $1 AND o.id = r.outlet_id
JOIN ` + k.partyTable + ` pt ON pt.tenant_id = $1 AND pt.id = r.party_id
ORDER BY r.created_at DESC, r.id DESC`
}

// summarySQL: $1..$5 saja. Dipakai juga oleh laporan saldo per pihak (tanpa filter pihak).
func (k sqlSet) summarySQL() string {
	return `SELECT ` + aggCols + `, count(DISTINCT x.party_id)::int
FROM (SELECT x.*, ` + bucketExpr + ` AS bk FROM (` + k.docRows(k.outstandingFilter(), ``) + `) x) x`
}

func (s *Summary) scan(row pgx.Row) error {
	var amount, paid, cut, bal decimal.Decimal
	var b [5]decimal.Decimal
	if err := row.Scan(&s.DocCount, &amount, &paid, &cut, &bal, &b[0], &b[1], &b[2], &b[3], &b[4], &s.PartyCount); err != nil {
		return err
	}
	s.Amount, s.Paid, s.Returned, s.Balance = money(amount), money(paid), money(cut), money(bal)
	s.Buckets = newBuckets(b)
	s.Overdue = money(s.Buckets.overdueSum())
	return nil
}

// bucketOf = bucket umur sebuah dokumen pada tanggal as-of.
func bucketOf(due pgtype.Date, asOf time.Time) (days, bucket int) {
	if !due.Valid || !asOf.After(due.Time) {
		return 0, 0
	}
	days = int(asOf.Sub(due.Time).Hours() / 24)
	switch {
	case days <= 30:
		return days, 1
	case days <= 60:
		return days, 2
	case days <= 90:
		return days, 3
	}
	return days, 4
}

// Outstanding: dokumen hutang / piutang yang masih punya sisa pada tanggal as-of, umur dihitung dari jatuh tempo.
func (s *Service) Outstanding(ctx context.Context, a authz.Actor, k Kind, p Params) (res OutstandingResult, err error) {
	err = db.WithTenant(ctx, s.pool, a.TenantID, func(tx pgx.Tx) error {
		var e error
		res, e = outstanding(ctx, tx, a, k, p)
		return e
	})
	return
}

func outstanding(ctx context.Context, tx pgx.Tx, a authz.Actor, k Kind, p Params) (OutstandingResult, error) {
	var result OutstandingResult
	set := sets[k]
	err := func() error {
		rs, err := resolve(ctx, tx, a, k, p, need{asOf: true})
		if err != nil {
			return err
		}
		result.Header = rs.hdr

		hasCursor := p.Cursor != "" && !p.ForExport
		var curAt time.Time
		curID := zeroUUID
		if hasCursor {
			parts, ok := decodeCursor(p.Cursor, 2)
			if !ok {
				return FieldErrors{"cursor": "INVALID"}
			}
			var ok2 bool
			curAt, ok2 = parseTS(parts[0])
			id, err := uuid.Parse(parts[1])
			if !ok2 || err != nil {
				return FieldErrors{"cursor": "INVALID"}
			}
			curID = id
		}
		limit := clampLimit(p.Limit)
		rowLimit := limit + 1
		if p.ForExport {
			rowLimit = exportMax(p) + 1
		}

		if !hasCursor {
			var sm Summary
			if err := sm.scan(tx.QueryRow(ctx, set.summarySQL(), a.TenantID, rs.outlets, rs.asOf, rs.party, rs.search)); err != nil {
				return err
			}
			result.Summary, result.HasSummary = sm, true
		}

		rows, err := tx.Query(ctx, set.outstandingPageSQL(),
			a.TenantID, rs.outlets, rs.asOf, rs.party, rs.search, hasCursor, curAt, curID, rowLimit)
		if err != nil {
			return err
		}
		defer rows.Close()
		list := []OutstandingRow{}
		var stamps []time.Time
		for rows.Next() {
			var r OutstandingRow
			var id uuid.UUID
			var doc, due pgtype.Date
			var amount, paid, cut, bal decimal.Decimal
			var at time.Time
			if err := rows.Scan(&id, &r.OutletCode, &r.DocNo, &r.RefNo, &r.PartyCode, &r.PartyName, &doc, &due,
				&amount, &paid, &cut, &bal, &at); err != nil {
				return err
			}
			r.ID, r.DocDate, r.DueDate = id.String(), dateStr(doc), dateStr(due)
			r.Amount, r.Paid, r.Returned, r.Balance = money(amount), money(paid), money(cut), money(bal)
			r.DaysOverdue, r.Bucket = bucketOf(due, rs.asOf)
			list = append(list, r)
			stamps = append(stamps, at)
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
			result.NextCursor = encodeCursor(tsCursor(stamps[limit-1]), list[limit-1].ID)
		}
		result.Data = list
		return nil
	}()
	return result, err
}
