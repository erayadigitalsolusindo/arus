-- name: AccountList :many
SELECT id, parent_id, code, name, kind, class, normal_side, is_cash_bank, active, is_system
FROM accounts WHERE tenant_id = $1 ORDER BY code;

-- name: AccountGet :one
SELECT id, parent_id, code, name, kind, class, normal_side, is_cash_bank, active, is_system
FROM accounts WHERE tenant_id = $1 AND id = $2;

-- name: AccountGetByCode :one
SELECT id, parent_id, code, name, kind, class, normal_side, is_cash_bank, active, is_system
FROM accounts WHERE tenant_id = $1 AND code = $2;

-- name: AccountCount :one
SELECT count(*) FROM accounts WHERE tenant_id = $1;

-- name: AccountCreate :one
INSERT INTO accounts (tenant_id, parent_id, code, name, kind, class, normal_side, is_cash_bank, is_system)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
RETURNING id, parent_id, code, name, kind, class, normal_side, is_cash_bank, active, is_system;

-- name: AccountUpdate :one
UPDATE accounts SET parent_id = $3, code = $4, name = $5, class = $6, normal_side = $7, is_cash_bank = $8, active = $9
WHERE tenant_id = $1 AND id = $2
RETURNING id, parent_id, code, name, kind, class, normal_side, is_cash_bank, active, is_system;

-- name: AccountDelete :execrows
DELETE FROM accounts WHERE tenant_id = $1 AND id = $2;

-- name: AccountHasChildren :one
SELECT EXISTS (SELECT 1 FROM accounts WHERE tenant_id = $1 AND parent_id = $2);

-- name: AccountHasLines :one
SELECT EXISTS (SELECT 1 FROM journal_lines WHERE tenant_id = $1 AND account_id = $2);

-- name: AccountsByIDs :many
SELECT id, kind, active, is_cash_bank FROM accounts WHERE tenant_id = $1 AND id = ANY($2::uuid[]);

-- name: PeriodList :many
SELECT id, start_date, end_date, status, closed_at, closed_by FROM accounting_periods
WHERE tenant_id = $1 ORDER BY start_date DESC LIMIT 240;

-- name: PeriodEnsure :exec
INSERT INTO accounting_periods (tenant_id, start_date, end_date)
VALUES ($1, $2, ($2::date + interval '1 month' - interval '1 day')::date)
ON CONFLICT (tenant_id, start_date) DO NOTHING;

-- name: PeriodLockShare :one
-- Posting menahan periode FOR SHARE; tutup buku (FOR UPDATE) menunggu posting yang sedang berjalan.
SELECT id, start_date, end_date, status, closed_at, closed_by FROM accounting_periods
WHERE tenant_id = $1 AND start_date = $2 FOR SHARE;

-- name: PeriodLockUpdate :one
SELECT id, start_date, end_date, status, closed_at, closed_by FROM accounting_periods
WHERE tenant_id = $1 AND start_date = $2 FOR UPDATE;

-- name: PeriodSetStatus :exec
UPDATE accounting_periods SET status = $3, closed_at = $4, closed_by = $5 WHERE tenant_id = $1 AND id = $2;

-- name: PeriodDraftCount :one
SELECT count(*) FROM journal_entries
WHERE tenant_id = $1 AND status = 'draft' AND entry_date >= $2 AND entry_date <= $3;

-- name: JournalNextNo :one
INSERT INTO journal_counters (tenant_id, type, year, last_no) VALUES ($1, $2, $3, 1)
ON CONFLICT (tenant_id, type, year) DO UPDATE SET last_no = journal_counters.last_no + 1
RETURNING last_no;

-- name: JournalInsert :one
INSERT INTO journal_entries (tenant_id, outlet_id, entry_date, type, narration, source_type, source_ref, reverses_id, created_by)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
RETURNING id;

-- name: JournalGet :one
SELECT id, outlet_id, doc_no, entry_date, type, status, narration, reverses_id, created_by, created_at, posted_by, posted_at
FROM journal_entries WHERE tenant_id = $1 AND id = $2;

-- name: JournalLockGet :one
SELECT id, outlet_id, doc_no, entry_date, type, status, narration, reverses_id, created_by, created_at, posted_by, posted_at
FROM journal_entries WHERE tenant_id = $1 AND id = $2 FOR UPDATE;

-- name: JournalUpdateDraft :execrows
UPDATE journal_entries SET outlet_id = $3, entry_date = $4, narration = $5
WHERE tenant_id = $1 AND id = $2 AND status = 'draft';

-- name: JournalMarkPosted :execrows
UPDATE journal_entries SET status = 'posted', doc_no = $3, posted_by = $4, posted_at = now()
WHERE tenant_id = $1 AND id = $2 AND status = 'draft';

-- name: JournalDeleteDraft :execrows
DELETE FROM journal_entries WHERE tenant_id = $1 AND id = $2 AND status = 'draft';

-- name: JournalReversalExists :one
SELECT EXISTS (SELECT 1 FROM journal_entries WHERE tenant_id = $1 AND reverses_id = $2);

-- name: JournalOpeningExists :one
SELECT EXISTS (
    SELECT 1 FROM journal_entries o WHERE o.tenant_id = $1 AND o.type = 'OPENING' AND o.reverses_id IS NULL
      AND NOT EXISTS (SELECT 1 FROM journal_entries r WHERE r.tenant_id = o.tenant_id AND r.reverses_id = o.id));

-- name: JournalLineInsert :exec
INSERT INTO journal_lines (tenant_id, entry_id, entry_date, line_no, account_id, debit, credit, memo)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8);

-- name: JournalLinesDelete :exec
DELETE FROM journal_lines WHERE tenant_id = $1 AND entry_id = $2;

-- name: JournalLines :many
SELECT l.line_no, l.account_id, a.code AS account_code, a.name AS account_name, l.debit, l.credit, l.memo
FROM journal_lines l JOIN accounts a ON a.tenant_id = l.tenant_id AND a.id = l.account_id
WHERE l.tenant_id = $1 AND l.entry_id = $2 ORDER BY l.line_no;

-- name: BalanceAdd :exec
INSERT INTO account_period_balances (tenant_id, outlet_id, account_id, period_month, debit, credit)
VALUES ($1, $2, $3, $4, $5, $6)
ON CONFLICT (tenant_id, outlet_id, account_id, period_month)
DO UPDATE SET debit = account_period_balances.debit + EXCLUDED.debit, credit = account_period_balances.credit + EXCLUDED.credit;

-- name: BalanceList :many
SELECT outlet_id, account_id, period_month, debit, credit FROM account_period_balances
WHERE tenant_id = $1 AND account_id = $2 ORDER BY period_month, outlet_id;
