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

-- name: MappingList :many
SELECT m.key, a.id AS account_id, a.code, a.name, a.class, a.active
FROM account_mappings m JOIN accounts a ON a.tenant_id = m.tenant_id AND a.id = m.account_id
WHERE m.tenant_id = $1 ORDER BY m.key;

-- name: MappingUpsert :exec
INSERT INTO account_mappings (tenant_id, key, account_id) VALUES ($1, $2, $3)
ON CONFLICT (tenant_id, key) DO UPDATE SET account_id = EXCLUDED.account_id, updated_at = now();

-- name: MappingInsertDefault :exec
INSERT INTO account_mappings (tenant_id, key, account_id)
SELECT a.tenant_id, $2::text, a.id FROM accounts a WHERE a.tenant_id = $1 AND a.code = $3::text AND a.kind = 'ledger'
ON CONFLICT DO NOTHING;

-- name: AccountIsMapped :one
SELECT EXISTS (SELECT 1 FROM account_mappings WHERE tenant_id = $1 AND account_id = $2);

-- ---- Jurnal otomatis: penjualan harian per outlet ----

-- name: AutoOutletWindow :one
SELECT o.name, o.active,
       ((sqlc.arg(day)::date)::timestamp AT TIME ZONE o.timezone)::timestamptz AS t0,
       ((((sqlc.arg(day)::date) + 1)::timestamp) AT TIME ZONE o.timezone)::timestamptz AS t1,
       (now() AT TIME ZONE o.timezone)::date AS today
FROM outlets o WHERE o.tenant_id = sqlc.arg(tenant_id) AND o.id = sqlc.arg(outlet_id);

-- name: AutoSalesTotals :one
SELECT count(*)::bigint AS n,
       coalesce(sum(subtotal - discount), 0)::numeric(18,2) AS revenue,
       coalesce(sum(tax_store + tax_gov), 0)::numeric(18,2) AS tax,
       coalesce(sum(other_cost), 0)::numeric(18,2) AS other_cost,
       coalesce(sum(change), 0)::numeric(18,2) AS change_given
FROM sales
WHERE tenant_id = $1 AND outlet_id = $2 AND status = 'completed' AND created_at >= $3 AND created_at < $4;

-- name: AutoSalesPayments :many
SELECT m.id AS method_id, m.name, m.kind, m.account_id,
       coalesce(sum(p.amount), 0)::numeric(18,2) AS amount,
       coalesce(sum(p.fee_amount), 0)::numeric(18,2) AS fee,
       coalesce(sum(CASE WHEN p.fee_bearer = 'customer' THEN p.fee_amount ELSE 0 END), 0)::numeric(18,2) AS surcharge
FROM sale_payments p
JOIN sales s ON s.tenant_id = p.tenant_id AND s.id = p.sale_id
JOIN payment_methods m ON m.tenant_id = p.tenant_id AND m.id = p.method_id
WHERE s.tenant_id = $1 AND s.outlet_id = $2 AND s.status = 'completed' AND s.created_at >= $3 AND s.created_at < $4
GROUP BY m.id, m.name, m.kind, m.account_id ORDER BY m.name;

-- name: AutoSalesReceivable :one
SELECT coalesce(sum(receivable), 0)::numeric(18,2) FROM sales
WHERE tenant_id = $1 AND outlet_id = $2 AND status = 'completed' AND created_at >= $3 AND created_at < $4;

-- name: AutoSalesCogs :one
SELECT coalesce(sum(l.unit_cost * l.qty), 0)::numeric(18,2)
FROM sale_lines l JOIN sales s ON s.tenant_id = l.tenant_id AND s.id = l.sale_id
WHERE s.tenant_id = $1 AND s.outlet_id = $2 AND s.status = 'completed' AND s.created_at >= $3 AND s.created_at < $4;

-- name: AutoJournalByRef :one
SELECT id, status FROM journal_entries WHERE tenant_id = $1 AND source_type = $2 AND source_ref = $3;

-- name: AutoJournalPostedNet :many
SELECT l.account_id, sum(l.debit - l.credit)::numeric(18,2) AS net
FROM journal_entries e JOIN journal_lines l ON l.tenant_id = e.tenant_id AND l.entry_id = e.id
WHERE e.tenant_id = $1 AND e.source_type = $2 AND e.status = 'posted'
  AND (e.source_ref = $3::text OR e.source_ref LIKE $3::text || ':adj:%')
GROUP BY l.account_id;

-- name: AutoJournalPendingAdj :one
SELECT id FROM journal_entries
WHERE tenant_id = $1 AND source_type = $2 AND status = 'draft' AND source_ref LIKE $3::text || ':adj:%' LIMIT 1;

-- name: AutoJournalAdjCount :one
SELECT count(*)::bigint FROM journal_entries WHERE tenant_id = $1 AND source_type = $2 AND source_ref LIKE $3::text || ':adj:%';

-- name: PeriodStatusGet :one
SELECT status FROM accounting_periods WHERE tenant_id = $1 AND start_date = $2;

-- ---- Jurnal otomatis: sumber lain (retur, pembelian, pembayaran piutang/hutang, deposit) ----

-- name: AutoCashMethod :one
SELECT id, name, account_id FROM payment_methods WHERE tenant_id = $1 AND kind = 'cash' LIMIT 1;

-- name: AutoSalesReturnTotals :one
SELECT count(*)::bigint AS n,
       coalesce(sum(subtotal - discount), 0)::numeric(18,2) AS revenue,
       coalesce(sum(tax_amount), 0)::numeric(18,2) AS tax,
       coalesce(sum(receivable_cut), 0)::numeric(18,2) AS receivable_cut
FROM sales_returns WHERE tenant_id = $1 AND outlet_id = $2 AND status = 'completed' AND return_date = $3;

-- name: AutoSalesReturnRefunds :many
SELECT m.id AS method_id, m.name, m.account_id, coalesce(sum(r.refund), 0)::numeric(18,2) AS amount
FROM sales_returns r JOIN payment_methods m ON m.tenant_id = r.tenant_id AND m.id = r.refund_method_id
WHERE r.tenant_id = $1 AND r.outlet_id = $2 AND r.status = 'completed' AND r.return_date = $3 AND r.refund > 0
GROUP BY m.id, m.name, m.account_id ORDER BY m.name;

-- name: AutoSalesReturnCost :one
SELECT coalesce(sum(l.unit_cost * l.qty), 0)::numeric(18,2)
FROM sales_return_lines l JOIN sales_returns r ON r.tenant_id = l.tenant_id AND r.id = l.return_id
WHERE r.tenant_id = $1 AND r.outlet_id = $2 AND r.status = 'completed' AND r.return_date = $3;

-- name: AutoPurchaseTotals :one
SELECT count(*)::bigint AS n,
       coalesce(sum(subtotal + other_cost), 0)::numeric(18,2) AS inventory,
       coalesce(sum(tax_amount), 0)::numeric(18,2) AS tax,
       coalesce(sum(total) FILTER (WHERE payment_type = 'credit'), 0)::numeric(18,2) AS credit_total,
       coalesce(sum(total) FILTER (WHERE payment_type = 'cash'), 0)::numeric(18,2) AS cash_total
FROM purchases WHERE tenant_id = $1 AND outlet_id = $2 AND status = 'completed' AND purchase_date = $3;

-- name: AutoPurchaseReturnTotals :one
SELECT count(*)::bigint AS n,
       coalesce(sum(subtotal), 0)::numeric(18,2) AS subtotal,
       coalesce(sum(tax_amount), 0)::numeric(18,2) AS tax,
       coalesce(sum(payable_cut), 0)::numeric(18,2) AS payable_cut
FROM purchase_returns WHERE tenant_id = $1 AND outlet_id = $2 AND status = 'completed' AND return_date = $3;

-- name: AutoPurchaseReturnRefunds :many
SELECT m.id AS method_id, m.name, m.account_id, coalesce(sum(r.refund), 0)::numeric(18,2) AS amount
FROM purchase_returns r JOIN payment_methods m ON m.tenant_id = r.tenant_id AND m.id = r.refund_method_id
WHERE r.tenant_id = $1 AND r.outlet_id = $2 AND r.status = 'completed' AND r.return_date = $3 AND r.refund > 0
GROUP BY m.id, m.name, m.account_id ORDER BY m.name;

-- name: AutoPurchaseReturnCost :one
SELECT coalesce(sum(l.unit_cost * l.qty), 0)::numeric(18,2)
FROM purchase_return_lines l JOIN purchase_returns r ON r.tenant_id = l.tenant_id AND r.id = l.return_id
WHERE r.tenant_id = $1 AND r.outlet_id = $2 AND r.status = 'completed' AND r.return_date = $3;

-- name: AutoReceivablePayments :many
SELECT m.id AS method_id, m.name, m.account_id, count(*)::bigint AS n,
       coalesce(sum(p.amount), 0)::numeric(18,2) AS amount,
       coalesce(sum(p.fee_amount), 0)::numeric(18,2) AS fee,
       coalesce(sum(CASE WHEN p.fee_bearer = 'customer' THEN p.fee_amount ELSE 0 END), 0)::numeric(18,2) AS surcharge
FROM receivable_payments p JOIN payment_methods m ON m.tenant_id = p.tenant_id AND m.id = p.method_id
WHERE p.tenant_id = $1 AND p.outlet_id = $2 AND p.created_at >= $3 AND p.created_at < $4
GROUP BY m.id, m.name, m.account_id ORDER BY m.name;

-- name: AutoPayablePayments :many
SELECT m.id AS method_id, m.name, m.account_id, count(*)::bigint AS n, coalesce(sum(p.amount), 0)::numeric(18,2) AS amount
FROM payable_payments p JOIN payment_methods m ON m.tenant_id = p.tenant_id AND m.id = p.method_id
WHERE p.tenant_id = $1 AND p.outlet_id = $2 AND p.created_at >= $3 AND p.created_at < $4
GROUP BY m.id, m.name, m.account_id ORDER BY m.name;

-- name: AutoDepositMoves :many
SELECT m.id AS method_id, m.name, m.account_id, d.kind, count(*)::bigint AS n, coalesce(sum(d.amount), 0)::numeric(18,2) AS amount
FROM member_deposit_movements d JOIN payment_methods m ON m.tenant_id = d.tenant_id AND m.id = d.method_id
WHERE d.tenant_id = $1 AND d.outlet_id = $2 AND d.kind IN ('TOPUP', 'WITHDRAW') AND d.created_at >= $3 AND d.created_at < $4
GROUP BY m.id, m.name, m.account_id, d.kind ORDER BY m.name, d.kind;

-- name: AutoSupplierCashOuts :many
SELECT m.id AS method_id, m.name, m.account_id, count(*)::bigint AS n, coalesce(sum(-d.amount), 0)::numeric(18,2) AS amount
FROM supplier_credit_movements d JOIN payment_methods m ON m.tenant_id = d.tenant_id AND m.id = d.method_id
WHERE d.tenant_id = $1 AND d.outlet_id = $2 AND d.kind = 'CASH_OUT' AND d.created_at >= $3 AND d.created_at < $4
GROUP BY m.id, m.name, m.account_id ORDER BY m.name;
