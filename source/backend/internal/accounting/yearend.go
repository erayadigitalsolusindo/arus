package accounting

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/shopspring/decimal"

	"aciraba/internal/audit"
	"aciraba/internal/authz"
	gen "aciraba/internal/gen"
	"aciraba/internal/platform/db"
	"aciraba/internal/platform/sanitize"
)

const yearEndSource = "year_close"

// lockYearPeriods ensures each calendar month exists, locks them in order, and requires all of them closed.
func lockYearPeriods(ctx context.Context, tx pgx.Tx, tenant uuid.UUID, year int, exclusive bool) error {
	q := gen.New(tx)
	for month := 1; month <= 12; month++ {
		start := time.Date(year, time.Month(month), 1, 0, 0, 0, 0, time.UTC)
		if err := q.PeriodEnsure(ctx, gen.PeriodEnsureParams{TenantID: tenant, StartDate: pgDate(start)}); err != nil {
			return err
		}
	}
	lock := "FOR SHARE"
	if exclusive {
		lock = "FOR UPDATE"
	}
	rows, err := tx.Query(ctx, `SELECT start_date, status FROM accounting_periods
		WHERE tenant_id = $1 AND start_date >= $2 AND start_date < $3 ORDER BY start_date `+lock,
		tenant, pgDate(time.Date(year, 1, 1, 0, 0, 0, 0, time.UTC)), pgDate(time.Date(year+1, 1, 1, 0, 0, 0, 0, time.UTC)))
	if err != nil {
		return err
	}
	defer rows.Close()
	count := 0
	for rows.Next() {
		var start time.Time
		var status string
		if err := rows.Scan(&start, &status); err != nil {
			return err
		}
		count++
		if status != "closed" {
			return ErrYearEndPeriodsOpen
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}
	if count != 12 {
		return ErrYearEndPeriodsOpen
	}
	return nil
}

func yearEndEntry(ctx context.Context, tx pgx.Tx, tenant uuid.UUID, entry gen.JournalGetRow) (bool, error) {
	var sourceType, sourceRef *string
	if err := tx.QueryRow(ctx, `SELECT source_type, source_ref FROM journal_entries WHERE tenant_id = $1 AND id = $2`, tenant, entry.ID).Scan(&sourceType, &sourceRef); err != nil {
		return false, err
	}
	if sourceType == nil || *sourceType != yearEndSource {
		return false, nil
	}
	date := entry.EntryDate.Time
	if entry.Type != "JU" || entry.ReversesID.Valid || date.Month() != time.December || date.Day() != 31 || sourceRef == nil || *sourceRef != fmt.Sprintf("%s:%d", entry.OutletID, date.Year()) {
		return false, ErrYearEndInvalid
	}
	return true, nil
}

// CreateYearEndDraft closes the temporary accounts to account 3200 after all calendar periods are closed.
func (s *Service) CreateYearEndDraft(ctx context.Context, a authz.Actor, year int) (Journal, error) {
	if year < 2000 || year >= s.now().Year() {
		return Journal{}, FieldErrors{"year": sanitize.Invalid}
	}
	if !outletAllowed(a, a.OutletID) {
		return Journal{}, ErrOutletForbidden
	}
	date := time.Date(year, time.December, 31, 0, 0, 0, 0, time.UTC)
	sourceRef := fmt.Sprintf("%s:%d", a.OutletID, year)
	var out Journal
	err := db.WithTenant(ctx, s.pool, a.TenantID, func(tx pgx.Tx) error {
		if err := activeOutlet(ctx, tx, a.TenantID, a.OutletID); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended('acc-year-close:' || $1::text || ':' || $2::text || ':' || $3::int::text, 0))`, a.TenantID, a.OutletID, year); err != nil {
			return err
		}
		if err := lockYearPeriods(ctx, tx, a.TenantID, year, true); err != nil {
			return err
		}

		var entryID uuid.UUID
		var status string
		existing := true
		err := tx.QueryRow(ctx, `SELECT id, status FROM journal_entries
			WHERE tenant_id = $1 AND source_type = $2 AND source_ref = $3`, a.TenantID, yearEndSource, sourceRef).Scan(&entryID, &status)
		if err == pgx.ErrNoRows {
			existing = false
		} else if err != nil {
			return err
		} else if status == "posted" {
			return ErrYearEndAlreadyPosted
		} else {
			entry, err := lockEntry(ctx, gen.New(tx), a.TenantID, entryID)
			if err != nil {
				return err
			}
			if entry.Status != "draft" {
				return ErrYearEndAlreadyPosted
			}
		}

		var drafts int64
		if err := tx.QueryRow(ctx, `SELECT count(*) FROM journal_entries
			WHERE tenant_id = $1 AND status = 'draft' AND entry_date >= $2 AND entry_date <= $3
			AND ($4::uuid IS NULL OR id <> $4)`, a.TenantID, pgDate(time.Date(year, 1, 1, 0, 0, 0, 0, time.UTC)), pgDate(date), nullableUUID(existing, entryID)).Scan(&drafts); err != nil {
			return err
		}
		if drafts > 0 {
			return ErrDraftsInPeriod
		}

		var retained uuid.UUID
		if err := tx.QueryRow(ctx, `SELECT id FROM accounts WHERE tenant_id = $1 AND code = '3200' AND kind = 'ledger' AND class = 'equity' AND active`, a.TenantID).Scan(&retained); err != nil {
			if err == pgx.ErrNoRows {
				return ErrYearEndRetainedEarningsMissing
			}
			return err
		}

		rows, err := tx.Query(ctx, `SELECT b.account_id, a.class, a.active, sum(b.debit), sum(b.credit)
			FROM account_period_balances b JOIN accounts a ON a.tenant_id = b.tenant_id AND a.id = b.account_id
			WHERE b.tenant_id = $1 AND b.outlet_id = $2 AND b.period_month >= $3 AND b.period_month < $4
			AND a.class IN ('revenue', 'cogs', 'expense')
			GROUP BY b.account_id, a.class, a.active ORDER BY b.account_id`, a.TenantID, a.OutletID,
			pgDate(time.Date(year, 1, 1, 0, 0, 0, 0, time.UTC)), pgDate(time.Date(year+1, 1, 1, 0, 0, 0, 0, time.UTC)))
		if err != nil {
			return err
		}
		lines := make([]normLine, 0)
		profit := decimal.Zero
		for rows.Next() {
			var account uuid.UUID
			var class string
			var active bool
			var debit, credit decimal.Decimal
			if err := rows.Scan(&account, &class, &active, &debit, &credit); err != nil {
				rows.Close()
				return err
			}
			if !active {
				rows.Close()
				return ErrAccountInvalid
			}
			net := debit.Sub(credit)
			if net.IsZero() {
				continue
			}
			profit = profit.Add(net)
			line := normLine{account: account, memo: fmt.Sprintf("Tutup saldo %s %d", class, year)}
			if net.IsPositive() {
				line.credit = net
			} else {
				line.debit = net.Abs()
			}
			lines = append(lines, line)
		}
		if err := rows.Err(); err != nil {
			rows.Close()
			return err
		}
		rows.Close()
		if len(lines) == 0 {
			return ErrYearEndNoActivity
		}
		retainedLine := normLine{account: retained, memo: fmt.Sprintf("Laba/rugi tahun %d", year)}
		if profit.IsPositive() {
			retainedLine.debit = profit
		} else if profit.IsNegative() {
			retainedLine.credit = profit.Abs()
		}
		if !profit.IsZero() {
			lines = append(lines, retainedLine)
		}
		if len(lines) < 2 || len(lines) > maxLines {
			return ErrJournalShape
		}
		if _, err := checkAccounts(ctx, gen.New(tx), a.TenantID, accountIDs(lines), true); err != nil {
			return err
		}

		narration := fmt.Sprintf("Jurnal penutup tahun %d", year)
		if existing {
			if _, err := tx.Exec(ctx, `UPDATE journal_entries SET narration = $3 WHERE tenant_id = $1 AND id = $2 AND status = 'draft'`, a.TenantID, entryID, narration); err != nil {
				return err
			}
			if err := gen.New(tx).JournalLinesDelete(ctx, gen.JournalLinesDeleteParams{TenantID: a.TenantID, EntryID: entryID}); err != nil {
				return err
			}
		} else if err := tx.QueryRow(ctx, `INSERT INTO journal_entries (tenant_id, outlet_id, entry_date, type, narration, source_type, source_ref, created_by)
			VALUES ($1, $2, $3, 'JU', $4, $5, $6, $7) RETURNING id`, a.TenantID, a.OutletID, pgDate(date), narration, yearEndSource, sourceRef, pgUUID(a.UserID)).Scan(&entryID); err != nil {
			return err
		}
		if err := insertLines(ctx, gen.New(tx), a.TenantID, entryID, date, lines); err != nil {
			return err
		}
		if err := audit.Record(ctx, tx, audit.FromActor(a), audit.Entry{Action: audit.ActionJournalSave, Entity: audit.EntityJournal,
			EntityID: entryID.String(), Details: map[string]any{"type": "JU", "year_end": year, "lines": len(lines)}}); err != nil {
			return err
		}
		out, err = getJournal(ctx, tx, a, entryID)
		return err
	})
	return out, err
}

func nullableUUID(valid bool, id uuid.UUID) any {
	if !valid {
		return nil
	}
	return id
}
