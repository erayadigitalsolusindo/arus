-- +goose Up

-- Jenis jurnal otomatis untuk opname stok (STOCK) dan kas laci (CASH: selisih tutup shift, kas kecil).
ALTER TABLE journal_entries DROP CONSTRAINT journal_entries_type_check;
ALTER TABLE journal_entries ADD CONSTRAINT journal_entries_type_check
    CHECK (type IN ('JU', 'KM', 'KK', 'TK', 'SALES', 'PURCHASE', 'AR', 'AP', 'WALLET', 'STOCK', 'CASH', 'OPENING'));

-- Peran pemetaan baru: beban kas kecil dan sumber dana "tambah receh" (kode sama dengan defaultMappings di internal/accounting/mappings.go).
INSERT INTO account_mappings (tenant_id, key, account_id)
SELECT a.tenant_id, d.key, a.id
FROM (VALUES ('petty_cash_expense', '6900'), ('petty_cash_in', '1120')) AS d (key, code)
JOIN accounts a ON a.code = d.code AND a.kind = 'ledger'
ON CONFLICT DO NOTHING;

-- Indeks pembacaan harian per outlet.
CREATE INDEX stock_counts_completed_idx ON stock_counts (tenant_id, outlet_id, completed_at) WHERE status = 'completed';
CREATE INDEX cash_shifts_closed_idx ON cash_shifts (tenant_id, outlet_id, closed_at) WHERE status = 'closed';
CREATE INDEX cash_movements_outlet_time_idx ON cash_movements (tenant_id, outlet_id, created_at) WHERE voided_at IS NULL;

-- +goose Down
DROP INDEX cash_movements_outlet_time_idx;
DROP INDEX cash_shifts_closed_idx;
DROP INDEX stock_counts_completed_idx;
DELETE FROM account_mappings WHERE key IN ('petty_cash_expense', 'petty_cash_in');
ALTER TABLE journal_entries DROP CONSTRAINT journal_entries_type_check;
ALTER TABLE journal_entries ADD CONSTRAINT journal_entries_type_check
    CHECK (type IN ('JU', 'KM', 'KK', 'TK', 'SALES', 'PURCHASE', 'AR', 'AP', 'WALLET', 'OPENING'));
