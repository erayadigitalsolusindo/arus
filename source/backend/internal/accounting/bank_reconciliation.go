package accounting

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/shopspring/decimal"

	"aciraba/internal/audit"
	"aciraba/internal/authz"
	"aciraba/internal/platform/db"
	"aciraba/internal/platform/sanitize"
)

const bankReconLineMax = 1000

type BankReconciliationInput struct {
	AccountID        uuid.UUID   `json:"account_id"`
	Month            string      `json:"month"`
	StatementOpening json.Number `json:"statement_opening"`
	StatementClosing json.Number `json:"statement_closing"`
}

type BankReconciliationUpdate struct {
	StatementOpening json.Number `json:"statement_opening"`
	StatementClosing json.Number `json:"statement_closing"`
	MatchedLineIDs   []uuid.UUID `json:"matched_line_ids"`
}

type BankReconciliationLine struct {
	LineID    uuid.UUID `json:"line_id"`
	Date      string    `json:"date"`
	DocNo     string    `json:"doc_no"`
	Type      string    `json:"type"`
	Narration string    `json:"narration"`
	Memo      string    `json:"memo"`
	Debit     string    `json:"debit"`
	Credit    string    `json:"credit"`
	Matched   bool      `json:"matched"`
}

type BankReconciliation struct {
	ID                uuid.UUID                `json:"id"`
	AccountID         uuid.UUID                `json:"account_id"`
	AccountCode       string                   `json:"account_code"`
	AccountName       string                   `json:"account_name"`
	OutletID          uuid.UUID                `json:"outlet_id"`
	Month             string                   `json:"month"`
	Status            string                   `json:"status"`
	StatementOpening  string                   `json:"statement_opening"`
	StatementClosing  string                   `json:"statement_closing"`
	BookOpening       string                   `json:"book_opening"`
	BookClosing       string                   `json:"book_closing"`
	MatchedDebit      string                   `json:"matched_debit"`
	MatchedCredit     string                   `json:"matched_credit"`
	OutstandingDebit  string                   `json:"outstanding_debit"`
	OutstandingCredit string                   `json:"outstanding_credit"`
	AdjustedBank      string                   `json:"adjusted_bank"`
	Difference        string                   `json:"difference"`
	Balanced          bool                     `json:"balanced"`
	CompletedAt       *time.Time               `json:"completed_at,omitempty"`
	Lines             []BankReconciliationLine `json:"lines"`
}

type bankReconHeader struct {
	id, accountID, outletID uuid.UUID
	month                   time.Time
	status                  string
	statementOpening        decimal.Decimal
	statementClosing        decimal.Decimal
	completedAt             *time.Time
}

func bankReconMonth(raw string) (time.Time, error) {
	month, ok := parseMonth(raw)
	if !ok || month.Year() < 2000 || month.After(monthStart(time.Now())) {
		return time.Time{}, FieldErrors{"month": sanitize.Invalid}
	}
	return month, nil
}

func bankReconAmount(raw json.Number, field string) (decimal.Decimal, error) {
	if strings.TrimSpace(raw.String()) == "" {
		return decimal.Zero, FieldErrors{field: sanitize.Required}
	}
	amount, ok := parseMoney(raw)
	if !ok {
		return decimal.Zero, FieldErrors{field: sanitize.Invalid}
	}
	return amount, nil
}

func loadBankReconHeader(ctx context.Context, tx pgx.Tx, tenant, id uuid.UUID) (bankReconHeader, error) {
	var h bankReconHeader
	var completedAt *time.Time
	err := tx.QueryRow(ctx, `SELECT id, account_id, outlet_id, period_month, status, statement_opening, statement_closing, completed_at
		FROM bank_reconciliations WHERE tenant_id = $1 AND id = $2`, tenant, id).Scan(
		&h.id, &h.accountID, &h.outletID, &h.month, &h.status, &h.statementOpening, &h.statementClosing, &completedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return h, ErrNotFound
	}
	h.completedAt = completedAt
	return h, err
}

func loadBankReconciliation(ctx context.Context, tx pgx.Tx, tenant uuid.UUID, h bankReconHeader) (BankReconciliation, error) {
	var account Account
	if err := tx.QueryRow(ctx, `SELECT id, code, name, kind, class, normal_side, is_cash_bank, active
		FROM accounts WHERE tenant_id = $1 AND id = $2`, tenant, h.accountID).Scan(
		&account.ID, &account.Code, &account.Name, &account.Kind, &account.Class, &account.NormalSide, &account.IsCashBank, &account.Active); err != nil {
		return BankReconciliation{}, err
	}
	periodEnd := h.month.AddDate(0, 1, 0)
	var opening decimal.Decimal
	if err := tx.QueryRow(ctx, `SELECT COALESCE(sum(debit - credit), 0) FROM account_period_balances
		WHERE tenant_id = $1 AND outlet_id = $2 AND account_id = $3 AND period_month < $4`,
		tenant, h.outletID, h.accountID, pgDate(h.month)).Scan(&opening); err != nil {
		return BankReconciliation{}, err
	}
	rows, err := tx.Query(ctx, `SELECT l.id, h.entry_date, COALESCE(h.doc_no, ''), h.type, h.narration, l.memo, l.debit, l.credit,
		EXISTS (SELECT 1 FROM bank_reconciliation_matches m WHERE m.tenant_id = $1 AND m.journal_line_id = l.id)
		FROM journal_lines l JOIN journal_entries h ON h.tenant_id = l.tenant_id AND h.id = l.entry_id
		WHERE l.tenant_id = $1 AND h.outlet_id = $2 AND l.account_id = $3 AND h.status = 'posted'
		AND (l.entry_date >= $4 AND l.entry_date < $5 OR (l.entry_date < $4 AND EXISTS (
			SELECT 1 FROM bank_reconciliations previous
			WHERE previous.tenant_id = $1 AND previous.outlet_id = $2 AND previous.account_id = $3 AND previous.status = 'completed'
			AND previous.period_month = date_trunc('month', l.entry_date)::date
			AND NOT EXISTS (SELECT 1 FROM bank_reconciliation_matches any_match WHERE any_match.tenant_id = $1 AND any_match.journal_line_id = l.id))))
		ORDER BY l.entry_date, l.seq LIMIT $6`, tenant, h.outletID, h.accountID, pgDate(h.month), pgDate(periodEnd), bankReconLineMax+1)
	if err != nil {
		return BankReconciliation{}, err
	}
	defer rows.Close()
	out := BankReconciliation{ID: h.id, AccountID: h.accountID, AccountCode: account.Code, AccountName: account.Name, OutletID: h.outletID,
		Month: h.month.Format("2006-01"), Status: h.status, StatementOpening: str(h.statementOpening), StatementClosing: str(h.statementClosing),
		BookOpening: str(opening), CompletedAt: h.completedAt, Lines: []BankReconciliationLine{}}
	var totalDebit, totalCredit, matchedDebit, matchedCredit, bookDebit, bookCredit decimal.Decimal
	for rows.Next() {
		if len(out.Lines) == bankReconLineMax {
			return BankReconciliation{}, ErrBankReconTooManyLines
		}
		var line BankReconciliationLine
		var date time.Time
		var debit, credit decimal.Decimal
		if err := rows.Scan(&line.LineID, &date, &line.DocNo, &line.Type, &line.Narration, &line.Memo, &debit, &credit, &line.Matched); err != nil {
			return BankReconciliation{}, err
		}
		line.Date, line.Debit, line.Credit = date.Format(dateLayout), str(debit), str(credit)
		out.Lines = append(out.Lines, line)
		if !date.Before(h.month) && date.Before(periodEnd) {
			bookDebit, bookCredit = bookDebit.Add(debit), bookCredit.Add(credit)
		}
		totalDebit, totalCredit = totalDebit.Add(debit), totalCredit.Add(credit)
		if line.Matched {
			matchedDebit, matchedCredit = matchedDebit.Add(debit), matchedCredit.Add(credit)
		}
	}
	if err := rows.Err(); err != nil {
		return BankReconciliation{}, err
	}
	outstandingDebit := totalDebit.Sub(matchedDebit)
	outstandingCredit := totalCredit.Sub(matchedCredit)
	out.BookClosing = str(opening.Add(bookDebit).Sub(bookCredit))
	out.MatchedDebit, out.MatchedCredit = str(matchedDebit), str(matchedCredit)
	out.OutstandingDebit, out.OutstandingCredit = str(outstandingDebit), str(outstandingCredit)
	adjusted := h.statementClosing.Add(outstandingDebit).Sub(outstandingCredit)
	out.AdjustedBank = str(adjusted)
	difference := adjusted.Sub(opening.Add(bookDebit).Sub(bookCredit))
	out.Difference, out.Balanced = str(difference), difference.IsZero()
	return out, nil
}

func (s *Service) StartBankReconciliation(ctx context.Context, a authz.Actor, in BankReconciliationInput) (BankReconciliation, error) {
	month, err := bankReconMonth(in.Month)
	if err != nil {
		return BankReconciliation{}, err
	}
	opening, err := bankReconAmount(in.StatementOpening, "statement_opening")
	if err != nil {
		return BankReconciliation{}, err
	}
	closing, err := bankReconAmount(in.StatementClosing, "statement_closing")
	if err != nil {
		return BankReconciliation{}, err
	}
	if in.AccountID == uuid.Nil {
		return BankReconciliation{}, FieldErrors{"account_id": sanitize.Required}
	}
	if !outletAllowed(a, a.OutletID) {
		return BankReconciliation{}, ErrOutletForbidden
	}
	var out BankReconciliation
	err = db.WithTenant(ctx, s.pool, a.TenantID, func(tx pgx.Tx) error {
		if err := activeOutlet(ctx, tx, a.TenantID, a.OutletID); err != nil {
			return err
		}
		account, err := queryAccount(ctx, tx, a.TenantID, in.AccountID)
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNotFound
		}
		if err != nil {
			return err
		}
		if !account.IsCashBank || account.Kind != KindLedger || account.Class != "asset" {
			return ErrAccountInvalid
		}
		if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended('bank-recon:' || $1::text || ':' || $2::text || ':' || $3::text || ':' || $4::text, 0))`,
			a.TenantID, a.OutletID, in.AccountID, month.Format("2006-01")); err != nil {
			return err
		}
		var id uuid.UUID
		var status string
		err = tx.QueryRow(ctx, `SELECT id, status FROM bank_reconciliations
			WHERE tenant_id = $1 AND outlet_id = $2 AND account_id = $3 AND period_month = $4`, a.TenantID, a.OutletID, in.AccountID, pgDate(month)).Scan(&id, &status)
		if errors.Is(err, pgx.ErrNoRows) {
			if err := tx.QueryRow(ctx, `INSERT INTO bank_reconciliations
				(tenant_id, outlet_id, account_id, period_month, statement_opening, statement_closing, created_by)
				VALUES ($1, $2, $3, $4, $5, $6, $7) RETURNING id`, a.TenantID, a.OutletID, in.AccountID, pgDate(month), opening, closing, pgUUID(a.UserID)).Scan(&id); err != nil {
				return err
			}
			if err := audit.Record(ctx, tx, audit.FromActor(a), audit.Entry{Action: audit.ActionBankReconciliationStart, Entity: audit.EntityBankReconciliation,
				EntityID: id.String(), Details: map[string]any{"account_id": in.AccountID.String(), "month": month.Format("2006-01")}}); err != nil {
				return err
			}
		} else if err != nil {
			return err
		} else {
			if status != "draft" {
				return ErrBankReconCompleted
			}
			if _, err := tx.Exec(ctx, `UPDATE bank_reconciliations SET statement_opening = $3, statement_closing = $4
				WHERE tenant_id = $1 AND id = $2 AND status = 'draft'`, a.TenantID, id, opening, closing); err != nil {
				return err
			}
			if err := audit.Record(ctx, tx, audit.FromActor(a), audit.Entry{Action: audit.ActionBankReconciliationUpdate, Entity: audit.EntityBankReconciliation,
				EntityID: id.String(), Details: map[string]any{"reopened_draft": true, "month": month.Format("2006-01")}}); err != nil {
				return err
			}
		}
		header, err := loadBankReconHeader(ctx, tx, a.TenantID, id)
		if err != nil {
			return err
		}
		out, err = loadBankReconciliation(ctx, tx, a.TenantID, header)
		return err
	})
	return out, err
}

func (s *Service) GetBankReconciliation(ctx context.Context, a authz.Actor, id uuid.UUID) (BankReconciliation, error) {
	var out BankReconciliation
	err := db.WithTenant(ctx, s.pool, a.TenantID, func(tx pgx.Tx) error {
		header, err := loadBankReconHeader(ctx, tx, a.TenantID, id)
		if err != nil {
			return err
		}
		if !outletAllowed(a, header.outletID) {
			return ErrOutletForbidden
		}
		out, err = loadBankReconciliation(ctx, tx, a.TenantID, header)
		return err
	})
	return out, err
}

func (s *Service) FindBankReconciliation(ctx context.Context, a authz.Actor, accountID uuid.UUID, monthRaw string) (BankReconciliation, error) {
	month, err := bankReconMonth(monthRaw)
	if err != nil {
		return BankReconciliation{}, err
	}
	var out BankReconciliation
	err = db.WithTenant(ctx, s.pool, a.TenantID, func(tx pgx.Tx) error {
		account, err := queryAccount(ctx, tx, a.TenantID, accountID)
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNotFound
		}
		if err != nil {
			return err
		}
		if !account.Active || !account.IsCashBank || account.Kind != KindLedger || account.Class != "asset" {
			return ErrAccountInvalid
		}
		var id uuid.UUID
		if err := tx.QueryRow(ctx, `SELECT id FROM bank_reconciliations WHERE tenant_id = $1 AND outlet_id = $2 AND account_id = $3 AND period_month = $4`,
			a.TenantID, a.OutletID, accountID, pgDate(month)).Scan(&id); errors.Is(err, pgx.ErrNoRows) {
			return ErrNotFound
		} else if err != nil {
			return err
		}
		header, err := loadBankReconHeader(ctx, tx, a.TenantID, id)
		if err != nil {
			return err
		}
		if !outletAllowed(a, header.outletID) {
			return ErrOutletForbidden
		}
		out, err = loadBankReconciliation(ctx, tx, a.TenantID, header)
		return err
	})
	return out, err
}

func (s *Service) UpdateBankReconciliation(ctx context.Context, a authz.Actor, id uuid.UUID, in BankReconciliationUpdate) (BankReconciliation, error) {
	opening, err := bankReconAmount(in.StatementOpening, "statement_opening")
	if err != nil {
		return BankReconciliation{}, err
	}
	closing, err := bankReconAmount(in.StatementClosing, "statement_closing")
	if err != nil {
		return BankReconciliation{}, err
	}
	if len(in.MatchedLineIDs) > bankReconLineMax {
		return BankReconciliation{}, FieldErrors{"matched_line_ids": sanitize.Invalid}
	}
	ids := slices.Clone(in.MatchedLineIDs)
	slices.SortFunc(ids, func(x, y uuid.UUID) int { return strings.Compare(x.String(), y.String()) })
	if len(slices.Compact(ids)) != len(ids) {
		return BankReconciliation{}, FieldErrors{"matched_line_ids": sanitize.Invalid}
	}
	var out BankReconciliation
	err = db.WithTenant(ctx, s.pool, a.TenantID, func(tx pgx.Tx) error {
		header, err := loadBankReconHeader(ctx, tx, a.TenantID, id)
		if err != nil {
			return err
		}
		if !outletAllowed(a, header.outletID) {
			return ErrOutletForbidden
		}
		if header.status != "draft" {
			return ErrBankReconCompleted
		}
		if err := tx.QueryRow(ctx, `SELECT id FROM bank_reconciliations WHERE tenant_id = $1 AND id = $2 AND status = 'draft' FOR UPDATE`, a.TenantID, id).Scan(&header.id); err != nil {
			return err
		}
		if len(ids) > 0 {
			var count int
			if err := tx.QueryRow(ctx, `SELECT count(*) FROM journal_lines l JOIN journal_entries h ON h.tenant_id = l.tenant_id AND h.id = l.entry_id
				WHERE l.tenant_id = $1 AND l.id = ANY($2::uuid[]) AND l.account_id = $3 AND h.outlet_id = $4 AND h.status = 'posted'
				AND (l.entry_date >= $5 AND l.entry_date < $6 OR (l.entry_date < $5 AND EXISTS (
					SELECT 1 FROM bank_reconciliations previous WHERE previous.tenant_id = $1 AND previous.outlet_id = $4 AND previous.account_id = $3
					AND previous.status = 'completed' AND previous.period_month = date_trunc('month', l.entry_date)::date
					AND NOT EXISTS (SELECT 1 FROM bank_reconciliation_matches any_match WHERE any_match.tenant_id = $1 AND any_match.journal_line_id = l.id))))`, a.TenantID, ids, header.accountID, header.outletID,
				pgDate(header.month), pgDate(header.month.AddDate(0, 1, 0))).Scan(&count); err != nil {
				return err
			}
			if count != len(ids) {
				return FieldErrors{"matched_line_ids": sanitize.Invalid}
			}
		}
		if _, err := tx.Exec(ctx, `DELETE FROM bank_reconciliation_matches WHERE tenant_id = $1 AND reconciliation_id = $2`, a.TenantID, id); err != nil {
			return err
		}
		for _, lineID := range ids {
			if _, err := tx.Exec(ctx, `INSERT INTO bank_reconciliation_matches (tenant_id, reconciliation_id, journal_line_id) VALUES ($1, $2, $3)`, a.TenantID, id, lineID); err != nil {
				return err
			}
		}
		if _, err := tx.Exec(ctx, `UPDATE bank_reconciliations SET statement_opening = $3, statement_closing = $4 WHERE tenant_id = $1 AND id = $2 AND status = 'draft'`,
			a.TenantID, id, opening, closing); err != nil {
			return err
		}
		if err := audit.Record(ctx, tx, audit.FromActor(a), audit.Entry{Action: audit.ActionBankReconciliationUpdate, Entity: audit.EntityBankReconciliation,
			EntityID: id.String(), Details: map[string]any{"matched_lines": len(ids), "month": header.month.Format("2006-01")}}); err != nil {
			return err
		}
		header, err = loadBankReconHeader(ctx, tx, a.TenantID, id)
		if err != nil {
			return err
		}
		out, err = loadBankReconciliation(ctx, tx, a.TenantID, header)
		return err
	})
	return out, err
}

func (s *Service) CompleteBankReconciliation(ctx context.Context, a authz.Actor, id uuid.UUID) (BankReconciliation, error) {
	var out BankReconciliation
	err := db.WithTenant(ctx, s.pool, a.TenantID, func(tx pgx.Tx) error {
		header, err := loadBankReconHeader(ctx, tx, a.TenantID, id)
		if err != nil {
			return err
		}
		if !outletAllowed(a, header.outletID) {
			return ErrOutletForbidden
		}
		if header.status != "draft" {
			return ErrBankReconCompleted
		}
		if err := tx.QueryRow(ctx, `SELECT id FROM bank_reconciliations WHERE tenant_id = $1 AND id = $2 AND status = 'draft' FOR UPDATE`, a.TenantID, id).Scan(&header.id); err != nil {
			return err
		}
		current, err := loadBankReconciliation(ctx, tx, a.TenantID, header)
		if err != nil {
			return err
		}
		if !current.Balanced {
			return ErrBankReconUnbalanced
		}
		if _, err := tx.Exec(ctx, `UPDATE bank_reconciliations SET status = 'completed', completed_by = $3, completed_at = clock_timestamp()
			WHERE tenant_id = $1 AND id = $2 AND status = 'draft'`, a.TenantID, id, pgUUID(a.UserID)); err != nil {
			return err
		}
		if err := audit.Record(ctx, tx, audit.FromActor(a), audit.Entry{Action: audit.ActionBankReconciliationComplete, Entity: audit.EntityBankReconciliation,
			EntityID: id.String(), Details: map[string]any{"month": header.month.Format("2006-01"), "difference": current.Difference}}); err != nil {
			return err
		}
		header, err = loadBankReconHeader(ctx, tx, a.TenantID, id)
		if err != nil {
			return err
		}
		out, err = loadBankReconciliation(ctx, tx, a.TenantID, header)
		return err
	})
	return out, err
}
