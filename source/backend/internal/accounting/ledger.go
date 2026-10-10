package accounting

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/shopspring/decimal"

	"aciraba/internal/authz"
	"aciraba/internal/platform/db"
	"aciraba/internal/platform/sanitize"
)

const (
	maxRangeDays = 366
	pageDefault  = 100
	pageMax      = 200
)

// scopeOutlets = outlet yang boleh dibaca. Pemilik tanpa filter = semua outlet (nil); lainnya dibatasi ke outlet yang ditugaskan.
func scopeOutlets(a authz.Actor, outlet uuid.UUID) ([]uuid.UUID, error) {
	if outlet != uuid.Nil {
		if !outletAllowed(a, outlet) {
			return nil, ErrOutletForbidden
		}
		return []uuid.UUID{outlet}, nil
	}
	if a.Perms.All {
		return nil, nil
	}
	ids := []uuid.UUID{a.OutletID}
	for id, ok := range a.Outlets {
		if ok && id != a.OutletID {
			ids = append(ids, id)
		}
	}
	return ids, nil
}

func parseRange(from, to string) (f, t time.Time, err error) {
	f, okF := parseDate(from)
	t, okT := parseDate(to)
	fe := FieldErrors{}
	if !okF {
		fe["from"] = sanitize.Invalid
	}
	if !okT {
		fe["to"] = sanitize.Invalid
	}
	if len(fe) == 0 && (t.Before(f) || t.Sub(f) > maxRangeDays*24*time.Hour) {
		fe["to"] = sanitize.Invalid
	}
	if len(fe) > 0 {
		return f, t, fe
	}
	return f, t, nil
}

func pageSize(n int) int {
	if n <= 0 {
		return pageDefault
	}
	return min(n, pageMax)
}

// ---------------------------------------------------------------- buku besar

type LedgerParams struct {
	AccountID uuid.UUID
	OutletID  uuid.UUID // kosong = semua outlet yang boleh dibaca
	From, To  string
	Cursor    string
	Limit     int
}

type LedgerRow struct {
	LineID    uuid.UUID `json:"line_id"`
	Seq       int64     `json:"-"` // kunci keyset
	EntryID   uuid.UUID `json:"entry_id"`
	Date      string    `json:"date"`
	DocNo     string    `json:"doc_no"`
	Type      string    `json:"type"`
	Narration string    `json:"narration"`
	Memo      string    `json:"memo"`
	Debit     string    `json:"debit"`
	Credit    string    `json:"credit"`
	Balance   string    `json:"balance"` // saldo berjalan menurut saldo normal akun
}

type Ledger struct {
	Account        Account     `json:"account"`
	OpeningBalance string      `json:"opening_balance"` // saldo sebelum `from`
	Rows           []LedgerRow `json:"rows"`
	NextCursor     string      `json:"next_cursor,omitempty"`
	// Total periode (hanya halaman pertama): Σ seluruh baris dalam rentang.
	TotalDebit  *string `json:"total_debit,omitempty"`
	TotalCredit *string `json:"total_credit,omitempty"`
}

// cursor buku besar = "tanggal|line_id|saldo berjalan sebelum baris berikutnya" (saldo ikut agar halaman berikutnya
// tidak menjumlah ulang dari awal; hanya memengaruhi tampilan saldo milik pemanggil sendiri).
func encodeLedgerCursor(date string, seq int64, running decimal.Decimal) string {
	return base64.RawURLEncoding.EncodeToString([]byte(fmt.Sprintf("%s|%d|%s", date, seq, running.StringFixed(2))))
}

func decodeLedgerCursor(c string) (time.Time, int64, decimal.Decimal, bool) {
	raw, err := base64.RawURLEncoding.DecodeString(c)
	parts := strings.Split(string(raw), "|")
	if err != nil || len(parts) != 3 {
		return time.Time{}, 0, decimal.Zero, false
	}
	d, okD := parseDate(parts[0])
	seq, errID := strconv.ParseInt(parts[1], 10, 64)
	run, errR := decimal.NewFromString(parts[2])
	return d, seq, run, okD && errID == nil && errR == nil
}

// signed = selisih menurut saldo normal akun (debit-normal: debit − kredit; kredit-normal: kredit − debit).
func signed(normal string, debit, credit decimal.Decimal) decimal.Decimal {
	if normal == "debit" {
		return debit.Sub(credit)
	}
	return credit.Sub(debit)
}

// Ledger = buku besar satu akun, paginasi keyset (tanggal, id baris). Saldo awal = agregat bulanan sebelum bulan `from`
// + baris dari awal bulan itu sampai sehari sebelum `from` (≤ 31 hari), jadi tidak memindai seluruh riwayat.
func (s *Service) Ledger(ctx context.Context, a authz.Actor, p LedgerParams) (Ledger, error) {
	from, to, err := parseRange(p.From, p.To)
	if err != nil {
		return Ledger{}, err
	}
	outlets, err := scopeOutlets(a, p.OutletID)
	if err != nil {
		return Ledger{}, err
	}
	limit := pageSize(p.Limit)
	var cd time.Time
	var cseq int64
	var running decimal.Decimal
	paged := p.Cursor != ""
	if paged {
		var ok bool
		if cd, cseq, running, ok = decodeLedgerCursor(p.Cursor); !ok {
			return Ledger{}, FieldErrors{"cursor": sanitize.Invalid}
		}
	}
	var out Ledger
	out.Rows = []LedgerRow{}
	err = db.WithTenant(ctx, s.pool, a.TenantID, func(tx pgx.Tx) error {
		acc, err := queryAccount(ctx, tx, a.TenantID, p.AccountID)
		if err != nil {
			return err
		}
		out.Account = acc
		if !paged {
			var od, oc decimal.Decimal
			// Bulan-bulan penuh sebelum bulan `from` dari agregat, sisanya (awal bulan..from-1) dari baris jurnal.
			if err := tx.QueryRow(ctx, `
				SELECT COALESCE((SELECT sum(debit)  FROM account_period_balances WHERE tenant_id = $1 AND account_id = $2 AND period_month < $3
				                   AND ($4::uuid[] IS NULL OR outlet_id = ANY($4))), 0),
				       COALESCE((SELECT sum(credit) FROM account_period_balances WHERE tenant_id = $1 AND account_id = $2 AND period_month < $3
				                   AND ($4::uuid[] IS NULL OR outlet_id = ANY($4))), 0)`,
				a.TenantID, p.AccountID, pgDate(monthStart(from)), outlets).Scan(&od, &oc); err != nil {
				return err
			}
			var pd, pc decimal.Decimal
			if err := tx.QueryRow(ctx, `
				SELECT COALESCE(sum(l.debit), 0), COALESCE(sum(l.credit), 0)
				FROM journal_lines l JOIN journal_entries h ON h.tenant_id = l.tenant_id AND h.id = l.entry_id
				WHERE l.tenant_id = $1 AND l.account_id = $2 AND l.entry_date >= $3 AND l.entry_date < $4
				  AND h.status = 'posted' AND ($5::uuid[] IS NULL OR h.outlet_id = ANY($5))`,
				a.TenantID, p.AccountID, pgDate(monthStart(from)), pgDate(from), outlets).Scan(&pd, &pc); err != nil {
				return err
			}
			running = signed(acc.NormalSide, od.Add(pd), oc.Add(pc))
			out.OpeningBalance = running.StringFixed(2)
			var td, tc decimal.Decimal
			if err := tx.QueryRow(ctx, `
				SELECT COALESCE(sum(l.debit), 0), COALESCE(sum(l.credit), 0)
				FROM journal_lines l JOIN journal_entries h ON h.tenant_id = l.tenant_id AND h.id = l.entry_id
				WHERE l.tenant_id = $1 AND l.account_id = $2 AND l.entry_date >= $3 AND l.entry_date <= $4
				  AND h.status = 'posted' AND ($5::uuid[] IS NULL OR h.outlet_id = ANY($5))`,
				a.TenantID, p.AccountID, pgDate(from), pgDate(to), outlets).Scan(&td, &tc); err != nil {
				return err
			}
			ts, cs := td.StringFixed(2), tc.StringFixed(2)
			out.TotalDebit, out.TotalCredit = &ts, &cs
		}
		// Keyset: (entry_date, id) > (cd, cid). Tanpa kursor → mulai dari `from`.
		after, afterSeq := from, int64(0)
		if paged {
			after, afterSeq = cd, cseq
		}
		rows, err := tx.Query(ctx, `
			SELECT l.id, l.seq, l.entry_id, l.entry_date, COALESCE(h.doc_no, ''), h.type, h.narration, l.memo, l.debit, l.credit
			FROM journal_lines l JOIN journal_entries h ON h.tenant_id = l.tenant_id AND h.id = l.entry_id
			WHERE l.tenant_id = $1 AND l.account_id = $2 AND l.entry_date <= $3
			  AND ($6::bool = false AND l.entry_date >= $4 OR $6::bool AND (l.entry_date, l.seq) > ($4, $5))
			  AND h.status = 'posted' AND ($7::uuid[] IS NULL OR h.outlet_id = ANY($7))
			ORDER BY l.entry_date, l.seq LIMIT $8`,
			a.TenantID, p.AccountID, pgDate(to), pgDate(after), afterSeq, paged, outlets, limit+1)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var r LedgerRow
			var date time.Time
			var d, c decimal.Decimal
			if err := rows.Scan(&r.LineID, &r.Seq, &r.EntryID, &date, &r.DocNo, &r.Type, &r.Narration, &r.Memo, &d, &c); err != nil {
				return err
			}
			r.Date, r.Debit, r.Credit = date.Format(dateLayout), d.StringFixed(2), c.StringFixed(2)
			if len(out.Rows) == limit { // baris ke-(limit+1) hanya penanda masih ada halaman berikutnya
				last := out.Rows[len(out.Rows)-1]
				out.NextCursor = encodeLedgerCursor(last.Date, last.Seq, running)
				break
			}
			running = running.Add(signed(acc.NormalSide, d, c))
			r.Balance = running.StringFixed(2)
			out.Rows = append(out.Rows, r)
		}
		return rows.Err()
	})
	return out, err
}

func queryAccount(ctx context.Context, tx pgx.Tx, tenant, id uuid.UUID) (Account, error) {
	var acc Account
	var parent *uuid.UUID
	err := tx.QueryRow(ctx, `SELECT id, parent_id, code, name, kind, class, normal_side, is_cash_bank, active FROM accounts WHERE tenant_id = $1 AND id = $2`,
		tenant, id).Scan(&acc.ID, &parent, &acc.Code, &acc.Name, &acc.Kind, &acc.Class, &acc.NormalSide, &acc.IsCashBank, &acc.Active)
	if errors.Is(err, pgx.ErrNoRows) {
		return acc, ErrNotFound
	}
	acc.ParentID = parent
	return acc, err
}

// ---------------------------------------------------------------- daftar jurnal

type JournalListParams struct {
	OutletID uuid.UUID
	From, To string
	Status   string // "", draft, posted
	Type     string
	Cursor   string
	Limit    int
}

type JournalSummary struct {
	ID        uuid.UUID `json:"id"`
	OutletID  uuid.UUID `json:"outlet_id"`
	DocNo     string    `json:"doc_no"`
	Date      string    `json:"date"`
	Type      string    `json:"type"`
	Status    string    `json:"status"`
	Narration string    `json:"narration"`
	Total     string    `json:"total"` // Σ debit

	Lines         int     `json:"lines"`
	CreatedBy     string  `json:"created_by"`
	PostedAt      *string `json:"posted_at,omitempty"`
	Source        string  `json:"source"`        // '' = manual; selain itu jurnal otomatis dari modul (mis. sale)
	IsReversal    bool    `json:"is_reversal"`   // jurnal ini membalik jurnal lain
	Reversed      bool    `json:"reversed"`      // jurnal ini sudah dibalik
	DebitAccount  string  `json:"debit_account"` // akun debit pertama ("kode · nama")
	CreditAccount string  `json:"credit_account"`
}

type JournalPage struct {
	Items      []JournalSummary `json:"items"`
	NextCursor string           `json:"next_cursor,omitempty"`
}

// ListJournals = daftar jurnal terbaru dulu, keyset (entry_date desc, id desc), rentang tanggal wajib dan dibatasi.
func (s *Service) ListJournals(ctx context.Context, a authz.Actor, p JournalListParams) (JournalPage, error) {
	from, to, err := parseRange(p.From, p.To)
	if err != nil {
		return JournalPage{}, err
	}
	if p.Status != "" && p.Status != "draft" && p.Status != "posted" {
		return JournalPage{}, FieldErrors{"status": sanitize.Invalid}
	}
	outlets, err := scopeOutlets(a, p.OutletID)
	if err != nil {
		return JournalPage{}, err
	}
	limit := pageSize(p.Limit)
	cd, cid := to.AddDate(0, 0, 1), uuid.Nil // tanpa kursor: semua baris < (to+1 hari, ∞)
	hasCursor := p.Cursor != ""
	if hasCursor {
		raw, e := base64.RawURLEncoding.DecodeString(p.Cursor)
		parts := strings.Split(string(raw), "|")
		d, ok := time.Time{}, false
		var id uuid.UUID
		var e2 error
		if e == nil && len(parts) == 2 {
			d, ok = parseDate(parts[0])
			id, e2 = uuid.Parse(parts[1])
		}
		if !ok || e2 != nil {
			return JournalPage{}, FieldErrors{"cursor": sanitize.Invalid}
		}
		cd, cid = d, id
	}
	out := JournalPage{Items: []JournalSummary{}}
	err = db.WithTenant(ctx, s.pool, a.TenantID, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `
			SELECT h.id, h.outlet_id, COALESCE(h.doc_no, ''), h.entry_date, h.type, h.status, h.narration,
			       s.total, s.n, COALESCE(u.name, ''), h.posted_at, COALESCE(h.source_type, ''), h.reverses_id IS NOT NULL,
			       EXISTS (SELECT 1 FROM journal_entries r WHERE r.tenant_id = h.tenant_id AND r.reverses_id = h.id),
			       COALESCE((SELECT a.code || ' · ' || a.name FROM journal_lines l JOIN accounts a ON a.tenant_id = l.tenant_id AND a.id = l.account_id
			                 WHERE l.tenant_id = h.tenant_id AND l.entry_id = h.id AND l.debit > 0 ORDER BY l.line_no LIMIT 1), ''),
			       COALESCE((SELECT a.code || ' · ' || a.name FROM journal_lines l JOIN accounts a ON a.tenant_id = l.tenant_id AND a.id = l.account_id
			                 WHERE l.tenant_id = h.tenant_id AND l.entry_id = h.id AND l.credit > 0 ORDER BY l.line_no LIMIT 1), '')
			FROM journal_entries h
			CROSS JOIN LATERAL (SELECT COALESCE(sum(l.debit), 0) AS total, count(*)::int AS n FROM journal_lines l
			                    WHERE l.tenant_id = h.tenant_id AND l.entry_id = h.id) s
			LEFT JOIN users u ON u.tenant_id = h.tenant_id AND u.id = h.created_by
			WHERE h.tenant_id = $1 AND h.entry_date >= $2 AND h.entry_date <= $3
			  AND ($4::bool = false OR (h.entry_date, h.id) < ($5, $6))
			  AND ($7 = '' OR h.status = $7) AND ($8 = '' OR h.type = $8)
			  AND ($9::uuid[] IS NULL OR h.outlet_id = ANY($9))
			ORDER BY h.entry_date DESC, h.id DESC LIMIT $10`,
			a.TenantID, pgDate(from), pgDate(to), hasCursor, pgDate(cd), cid, p.Status, p.Type, outlets, limit+1)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var j JournalSummary
			var date time.Time
			var total decimal.Decimal
			var postedAt *time.Time
			if err := rows.Scan(&j.ID, &j.OutletID, &j.DocNo, &date, &j.Type, &j.Status, &j.Narration, &total, &j.Lines, &j.CreatedBy,
				&postedAt, &j.Source, &j.IsReversal, &j.Reversed, &j.DebitAccount, &j.CreditAccount); err != nil {
				return err
			}
			j.Date, j.Total = date.Format(dateLayout), total.StringFixed(2)
			if postedAt != nil {
				s := postedAt.Format(time.RFC3339)
				j.PostedAt = &s
			}
			if len(out.Items) == limit {
				last := out.Items[len(out.Items)-1]
				out.NextCursor = base64.RawURLEncoding.EncodeToString([]byte(last.Date + "|" + last.ID.String()))
				break
			}
			out.Items = append(out.Items, j)
		}
		return rows.Err()
	})
	return out, err
}
