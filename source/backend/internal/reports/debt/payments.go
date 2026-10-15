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

// PaymentRow = satu pembayaran hutang / piutang.
type PaymentRow struct {
	ID           string `json:"id"`
	DocNo        string `json:"doc_no"`
	Date         string `json:"date"`
	Time         string `json:"time"`
	OutletCode   string `json:"outlet_code"`
	TargetDocNo  string `json:"target_doc_no"` // faktur pembelian / nota penjualan yang dibayar
	PartyName    string `json:"party_name"`
	Method       string `json:"method"`
	RefNo        string `json:"ref_no"`
	Amount       string `json:"amount"`
	Fee          string `json:"fee"` // biaya metode bayar (piutang saja)
	UserName     string `json:"user_name"`
	Note         string `json:"note"`
	SettlementNo string `json:"settlement_no"` // dokumen pelunasan massal, bila ada
}

// MethodTotal = rekap per metode bayar.
type MethodTotal struct {
	Name   string `json:"name"`
	Count  int    `json:"count"`
	Amount string `json:"amount"`
}

// PaymentSummary = total seluruh hasil filter.
type PaymentSummary struct {
	Count    int           `json:"count"`
	Amount   string        `json:"amount"`
	Fee      string        `json:"fee"`
	ByMethod []MethodTotal `json:"by_method"`
}

// PaymentResult hasil laporan pembayaran.
type PaymentResult struct {
	Header
	Data       []PaymentRow   `json:"data"`
	Summary    PaymentSummary `json:"summary"`
	HasSummary bool           `json:"has_summary"`
	NextCursor string         `json:"next_cursor,omitempty"`
	Truncated  bool           `json:"truncated,omitempty"`
}

// outletsCTE: outlet yang dibaca + batas waktu absolut rentang tanggal lokal ([from 00:00, to+1 00:00)
// di zona waktu outlet). $1 tenant, $2 outlet[], $3 from, $4 to.
const outletsCTE = `
WITH ol AS (
  SELECT o.id, o.code, o.name,
         ($3::date)::timestamp AT TIME ZONE o.timezone AS lo,
         (($4::date) + 1)::timestamp AT TIME ZONE o.timezone AS hi
  FROM outlets o
  WHERE o.tenant_id = $1 AND o.id = ANY($2::uuid[])
)
`

// payFilter: $5 pihak, $6 metode, $7 cari (nomor pembayaran / nomor dokumen), alias x = pembayaran, d = dokumen.
func (k sqlSet) payFilter(flat bool) string {
	f := `AND ($6::uuid = ` + zeroUUIDLit + ` OR x.method_id = $6::uuid)`
	if flat {
		return f + `
      AND ($5::uuid = ` + zeroUUIDLit + ` OR d.` + k.partyCol + ` = $5::uuid)
      AND ($7::text = '' OR x.doc_no ILIKE '%' || $7::text || '%' OR ` + k.docNo + ` ILIKE '%' || $7::text || '%')`
	}
	return f + ` AND $5::uuid = ` + zeroUUIDLit + ` AND $7::text = ''`
}

// payDocJoin = join pembayaran → dokumen (+ tabel pendukung tanggal/nomor dokumen) dan outlet dokumen.
func (k sqlSet) payDocJoin() string {
	return `JOIN ` + k.docTable + ` d ON d.tenant_id = x.tenant_id AND d.id = x.` + k.payFK + `
` + k.extraJoin + `
JOIN outlets o ON o.tenant_id = d.tenant_id AND o.id = d.outlet_id`
}

// $8 hasCursor, $9 cursor created_at, $10 cursor id, $11 limit.
const payCursor = `AND ($8::bool = false OR (x.created_at, x.id) < ($9::timestamptz, $10::uuid))`

// paymentsSQL: bentuk LATERAL (top-N per outlet) untuk daftar polos; bentuk datar untuk filter pihak / cari.
func (k sqlSet) paymentsSQL(flat bool) string {
	cols := `x.id, x.outlet_id, x.` + k.payFK + ` AS doc_id, x.doc_no, x.method_name, x.ref_no, x.amount, ` + k.payFee + ` AS fee,
          x.note, x.created_at, x.` + k.payUser + ` AS user_id, x.settlement_id`
	var inner string
	if flat {
		inner = `
  SELECT ` + cols + `
  FROM ol
  JOIN ` + k.payTable + ` x ON x.tenant_id = $1 AND x.outlet_id = ol.id AND x.created_at >= ol.lo AND x.created_at < ol.hi
  JOIN ` + k.docTable + ` d ON d.tenant_id = x.tenant_id AND d.id = x.` + k.payFK + `
  ` + k.extraJoin + `
  JOIN outlets o ON o.tenant_id = d.tenant_id AND o.id = d.outlet_id
  WHERE true ` + k.payFilter(true) + ` ` + payCursor + `
  ORDER BY x.created_at DESC, x.id DESC
  LIMIT $11`
	} else {
		inner = `
  SELECT l.* FROM ol
  CROSS JOIN LATERAL (
    SELECT ` + cols + `
    FROM ` + k.payTable + ` x
    WHERE x.tenant_id = $1 AND x.outlet_id = ol.id AND x.created_at >= ol.lo AND x.created_at < ol.hi
      ` + k.payFilter(false) + ` ` + payCursor + `
    ORDER BY x.created_at DESC, x.id DESC
    LIMIT $11
  ) l
  ORDER BY l.created_at DESC, l.id DESC
  LIMIT $11`
	}
	return outletsCTE + `
SELECT r.id, r.doc_no, r.created_at, (r.created_at AT TIME ZONE o.timezone)::date, to_char(r.created_at AT TIME ZONE o.timezone, 'HH24:MI'),
       ol.code, ` + k.docNo + `, coalesce(pt.name, ''), r.method_name, r.ref_no, r.amount, r.fee,
       coalesce(u.name, ''), r.note, coalesce(st.doc_no, '')
FROM (` + inner + `
) r
JOIN ol ON ol.id = r.outlet_id
JOIN outlets o ON o.tenant_id = $1 AND o.id = ol.id
JOIN ` + k.docTable + ` d ON d.tenant_id = $1 AND d.id = r.doc_id
` + k.extraJoin + `
LEFT JOIN ` + k.partyTable + ` pt ON pt.tenant_id = $1 AND pt.id = d.` + k.partyCol + `
LEFT JOIN users u ON u.tenant_id = $1 AND u.id = r.user_id
LEFT JOIN ` + k.settleTbl + ` st ON st.tenant_id = $1 AND st.id = r.settlement_id
ORDER BY r.created_at DESC, r.id DESC
`
}

// paymentSummarySQL: rekap per metode bayar (jumlah, nominal, biaya). Parameter $1..$7 seperti paymentsSQL.
func (k sqlSet) paymentSummarySQL(flat bool) string {
	join := ``
	if flat {
		join = `JOIN ` + k.docTable + ` d ON d.tenant_id = x.tenant_id AND d.id = x.` + k.payFK + `
` + k.extraJoin + `
JOIN outlets o ON o.tenant_id = d.tenant_id AND o.id = d.outlet_id`
	}
	return outletsCTE + `
SELECT x.method_name, count(*)::int, coalesce(sum(x.amount), 0), coalesce(sum(` + k.payFee + `), 0)
FROM ol
JOIN ` + k.payTable + ` x ON x.tenant_id = $1 AND x.outlet_id = ol.id AND x.created_at >= ol.lo AND x.created_at < ol.hi
` + join + `
WHERE true ` + k.payFilter(flat) + `
GROUP BY x.method_name
ORDER BY sum(x.amount) DESC, x.method_name`
}

// Payments: daftar pembayaran hutang / piutang pada rentang tanggal, terbaru dulu.
func (s *Service) Payments(ctx context.Context, a authz.Actor, k Kind, p Params) (res PaymentResult, err error) {
	err = db.WithTenant(ctx, s.pool, a.TenantID, func(tx pgx.Tx) error {
		var e error
		res, e = payments(ctx, tx, a, k, p)
		return e
	})
	return
}

func payments(ctx context.Context, tx pgx.Tx, a authz.Actor, k Kind, p Params) (PaymentResult, error) {
	var result PaymentResult
	set := sets[k]
	err := func() error {
		rs, err := resolve(ctx, tx, a, k, p, need{period: true})
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
		// Pencarian atau filter pihak → bentuk datar (indeks pihak / nomor dokumen).
		flat := rs.search != "" || rs.party != zeroUUID

		if !hasCursor {
			var sm PaymentSummary
			sm.ByMethod = []MethodTotal{}
			var total, fee decimal.Decimal
			rows, err := tx.Query(ctx, set.paymentSummarySQL(flat), a.TenantID, rs.outlets, rs.from, rs.to, rs.party, rs.method, rs.search)
			if err != nil {
				return err
			}
			for rows.Next() {
				var m MethodTotal
				var amount, f decimal.Decimal
				if err := rows.Scan(&m.Name, &m.Count, &amount, &f); err != nil {
					rows.Close()
					return err
				}
				m.Amount = money(amount)
				sm.Count += m.Count
				total, fee = total.Add(amount), fee.Add(f)
				sm.ByMethod = append(sm.ByMethod, m)
			}
			rows.Close()
			if err := rows.Err(); err != nil {
				return err
			}
			sm.Amount, sm.Fee = money(total), money(fee)
			result.Summary, result.HasSummary = sm, true
		}

		rows, err := tx.Query(ctx, set.paymentsSQL(flat), a.TenantID, rs.outlets, rs.from, rs.to, rs.party, rs.method, rs.search,
			hasCursor, curAt, curID, rowLimit)
		if err != nil {
			return err
		}
		defer rows.Close()
		list := []PaymentRow{}
		var stamps []time.Time
		for rows.Next() {
			var r PaymentRow
			var id uuid.UUID
			var at time.Time
			var day pgtype.Date
			var amount, fee decimal.Decimal
			if err := rows.Scan(&id, &r.DocNo, &at, &day, &r.Time, &r.OutletCode, &r.TargetDocNo, &r.PartyName, &r.Method,
				&r.RefNo, &amount, &fee, &r.UserName, &r.Note, &r.SettlementNo); err != nil {
				return err
			}
			r.ID, r.Date = id.String(), dateStr(day)
			r.Amount, r.Fee = money(amount), money(fee)
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
