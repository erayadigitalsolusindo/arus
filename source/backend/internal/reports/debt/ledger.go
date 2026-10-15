package debt

import (
	"context"
	"strconv"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/shopspring/decimal"

	"aciraba/internal/authz"
	"aciraba/internal/platform/db"
)

// Jenis baris buku bantu.
const (
	EventInvoice = "invoice" // dokumen hutang / piutang bertambah
	EventPayment = "payment" // pembayaran
	EventReturn  = "return"  // potongan retur
)

// LedgerRow = satu mutasi pada buku bantu satu pihak, dengan saldo berjalan sesudahnya.
type LedgerRow struct {
	Date     string `json:"date"`
	Type     string `json:"type"`
	DocNo    string `json:"doc_no"`
	RefNo    string `json:"ref_no"` // dokumen yang dibayar / diretur, atau no. faktur pemasok
	Memo     string `json:"memo"`
	Increase string `json:"increase"`
	Decrease string `json:"decrease"`
	Balance  string `json:"balance"`
}

// LedgerSummary = saldo awal, mutasi, dan saldo akhir pada periode.
type LedgerSummary struct {
	Opening  string `json:"opening"`
	Increase string `json:"increase"`
	Decrease string `json:"decrease"`
	Closing  string `json:"closing"`
}

// LedgerResult hasil buku bantu.
type LedgerResult struct {
	Header
	Opening    string        `json:"opening"` // saldo berjalan sebelum baris pertama halaman ini
	Data       []LedgerRow   `json:"data"`
	Summary    LedgerSummary `json:"summary"`
	HasSummary bool          `json:"has_summary"`
	NextCursor string        `json:"next_cursor,omitempty"`
	Truncated  bool          `json:"truncated,omitempty"`
}

// ledgerUnion = mutasi pihak ($5) pada [$3, $4]: dokumen, pembayaran, potongan retur.
// withPage: tambahkan kursor ($6 hasCursor, $7 tanggal, $8 waktu, $9 urutan, $10 id) dan LIMIT $11 per cabang.
func (k sqlSet) ledgerUnion(withPage bool) string {
	branch := func(sel, dateExpr, atExpr, rank, idExpr, from, where string) string {
		cond, tail := ``, ``
		if withPage {
			cond = `AND ($6::bool = false OR (` + dateExpr + `, ` + atExpr + `, ` + rank + `, ` + idExpr + `) > ($7::date, $8::timestamptz, $9::int, $10::uuid))`
			tail = `ORDER BY ` + dateExpr + `, ` + atExpr + `, ` + idExpr + ` LIMIT $11`
		}
		q := `SELECT ` + sel + ` FROM ` + from + ` WHERE ` + where + ` ` + cond + ` ` + tail
		if withPage {
			return `(` + q + `)`
		}
		return q
	}
	base := `d.tenant_id = $1 AND d.outlet_id = ANY($2::uuid[]) AND d.` + k.partyCol + ` = $5::uuid AND d.voided_at IS NULL ` + k.validity
	docJoin := k.docTable + ` d ` + k.extraJoin + ` JOIN outlets o ON o.tenant_id = d.tenant_id AND o.id = d.outlet_id`

	inv := branch(
		k.docDate+` AS ev_date, d.created_at AS ev_at, 0::int AS ev_rank, d.id AS ev_id, '`+EventInvoice+`'::text AS ev_type, `+
			k.docNo+` AS doc_no, `+k.refNo+` AS ref_no, ''::text AS memo, d.amount AS inc_amt, 0::numeric AS dec_amt`,
		k.docDate, `d.created_at`, `0`, `d.id`, docJoin,
		base+` AND `+k.docDate+` BETWEEN $3::date AND $4::date`)

	payLocal := `(x.created_at AT TIME ZONE o.timezone)::date`
	pay := branch(
		payLocal+` AS ev_date, x.created_at AS ev_at, 1::int AS ev_rank, x.id AS ev_id, '`+EventPayment+`'::text AS ev_type, `+
			`x.doc_no AS doc_no, `+k.docNo+` AS ref_no, trim(x.method_name || ' ' || x.ref_no) AS memo, 0::numeric AS inc_amt, x.amount AS dec_amt`,
		payLocal, `x.created_at`, `1`, `x.id`,
		k.payTable+` x JOIN `+k.docTable+` d ON d.tenant_id = x.tenant_id AND d.id = x.`+k.payFK+` `+k.extraJoin+
			` JOIN outlets o ON o.tenant_id = d.tenant_id AND o.id = d.outlet_id`,
		base+` AND `+k.docDate+` <= $4::date AND `+payLocal+` BETWEEN $3::date AND $4::date`)

	ret := branch(
		`r.return_date AS ev_date, r.created_at AS ev_at, 2::int AS ev_rank, r.id AS ev_id, '`+EventReturn+`'::text AS ev_type, `+
			`r.doc_no AS doc_no, `+k.docNo+` AS ref_no, ''::text AS memo, 0::numeric AS inc_amt, r.`+k.retCutCol+` AS dec_amt`,
		`r.return_date`, `r.created_at`, `2`, `r.id`,
		k.retTable+` r JOIN `+k.docTable+` d ON d.tenant_id = r.tenant_id AND d.`+k.retDocFK+` = r.`+k.retFK+` `+k.extraJoin+
			` JOIN outlets o ON o.tenant_id = d.tenant_id AND o.id = d.outlet_id`,
		base+` AND `+k.docDate+` <= $4::date AND r.status = 'completed' AND r.`+k.retCutCol+` > 0 AND r.return_date BETWEEN $3::date AND $4::date`)

	return inv + ` UNION ALL ` + pay + ` UNION ALL ` + ret
}

// ledgerPageSQL: $1..$11 (lihat ledgerUnion).
func (k sqlSet) ledgerPageSQL() string {
	return `SELECT ev_date, ev_at, ev_rank, ev_id, ev_type, doc_no, ref_no, memo, inc_amt, dec_amt
FROM (` + k.ledgerUnion(true) + `) e
ORDER BY ev_date, ev_at, ev_rank, ev_id
LIMIT $11`
}

// ledgerTotalsSQL: $1..$5.
func (k sqlSet) ledgerTotalsSQL() string {
	return `SELECT coalesce(sum(inc_amt), 0), coalesce(sum(dec_amt), 0) FROM (` + k.ledgerUnion(false) + `) e`
}

// ledgerOpeningSQL: saldo seluruh dokumen pihak ($4) pada as-of $3.
func (k sqlSet) ledgerOpeningSQL() string {
	return `SELECT coalesce(sum(x.bal), 0) FROM (` + k.docRows(k.partyCond(), ``) + `) x`
}

// Ledger: buku bantu hutang / piutang satu pihak (wajib party_id) pada rentang tanggal, dengan saldo berjalan.
func (s *Service) Ledger(ctx context.Context, a authz.Actor, k Kind, p Params) (res LedgerResult, err error) {
	err = db.WithTenant(ctx, s.pool, a.TenantID, func(tx pgx.Tx) error {
		var e error
		res, e = ledger(ctx, tx, a, k, p)
		return e
	})
	return
}

func ledger(ctx context.Context, tx pgx.Tx, a authz.Actor, k Kind, p Params) (LedgerResult, error) {
	var result LedgerResult
	set := sets[k]
	err := func() error {
		rs, err := resolve(ctx, tx, a, k, p, need{period: true, party: true})
		if err != nil {
			return err
		}
		result.Header = rs.hdr

		var opening decimal.Decimal
		if err := tx.QueryRow(ctx, set.ledgerOpeningSQL(), a.TenantID, rs.outlets, rs.from.AddDate(0, 0, -1), rs.party).Scan(&opening); err != nil {
			return err
		}

		hasCursor := p.Cursor != "" && !p.ForExport
		var curDay, curAt time.Time
		curRank := 0
		curID := zeroUUID
		running := opening
		if hasCursor {
			parts, ok := decodeCursor(p.Cursor, 5)
			if !ok {
				return FieldErrors{"cursor": "INVALID"}
			}
			var err1, err2, err3, err4 error
			curDay, err1 = time.Parse("2006-01-02", parts[0])
			var ok2 bool
			curAt, ok2 = parseTS(parts[1])
			curRank, err2 = strconv.Atoi(parts[2])
			curID, err3 = uuid.Parse(parts[3])
			running, err4 = decimal.NewFromString(parts[4])
			if err1 != nil || !ok2 || err2 != nil || err3 != nil || err4 != nil {
				return FieldErrors{"cursor": "INVALID"}
			}
		}
		result.Opening = money(running)

		if !hasCursor {
			var inc, dec decimal.Decimal
			if err := tx.QueryRow(ctx, set.ledgerTotalsSQL(), a.TenantID, rs.outlets, rs.from, rs.to, rs.party).Scan(&inc, &dec); err != nil {
				return err
			}
			result.Summary = LedgerSummary{
				Opening: money(opening), Increase: money(inc), Decrease: money(dec),
				Closing: money(opening.Add(inc).Sub(dec)),
			}
			result.HasSummary = true
		}

		limit := clampLimit(p.Limit)
		rowLimit := limit + 1
		if p.ForExport {
			rowLimit = exportMax(p) + 1
		}
		rows, err := tx.Query(ctx, set.ledgerPageSQL(), a.TenantID, rs.outlets, rs.from, rs.to, rs.party,
			hasCursor, curDay, curAt, int32(curRank), curID, rowLimit)
		if err != nil {
			return err
		}
		defer rows.Close()

		type keyed struct {
			day  time.Time
			at   time.Time
			rank int32
			id   uuid.UUID
			bal  decimal.Decimal
		}
		list := []LedgerRow{}
		var keys []keyed
		for rows.Next() {
			var r LedgerRow
			var day pgtype.Date
			var at time.Time
			var rank int32
			var id uuid.UUID
			var inc, dec decimal.Decimal
			if err := rows.Scan(&day, &at, &rank, &id, &r.Type, &r.DocNo, &r.RefNo, &r.Memo, &inc, &dec); err != nil {
				return err
			}
			running = running.Add(inc).Sub(dec)
			r.Date, r.Increase, r.Decrease, r.Balance = dateStr(day), money(inc), money(dec), money(running)
			list = append(list, r)
			keys = append(keys, keyed{day.Time, at, rank, id, running})
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
			kk := keys[limit-1]
			result.NextCursor = encodeCursor(kk.day.Format("2006-01-02"), tsCursor(kk.at), strconv.Itoa(int(kk.rank)), kk.id.String(), money(kk.bal))
		}
		result.Data = list
		return nil
	}()
	return result, err
}
