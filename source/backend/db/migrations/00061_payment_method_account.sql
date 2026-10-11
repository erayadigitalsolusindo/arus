-- +goose Up

-- Pemetaan metode pembayaran -> akun COA (FR-ACC-02: pemetaan akun per metode bayar, dasar jurnal otomatis). Boleh kosong
-- selama tenant belum punya bagan akun; terisi otomatis saat metode dibuat / saat template retail dipasang.
ALTER TABLE payment_methods ADD COLUMN account_id uuid;
ALTER TABLE payment_methods ADD CONSTRAINT payment_methods_account_fk FOREIGN KEY (tenant_id, account_id)
    REFERENCES accounts (tenant_id, id) ON DELETE RESTRICT;
CREATE INDEX payment_methods_account_idx ON payment_methods (tenant_id, account_id) WHERE account_id IS NOT NULL;

-- Backfill: akun bawaan menurut jenis (sama dengan defaultAccountCode di internal/paymentmethod).
UPDATE payment_methods m SET account_id = a.id
FROM accounts a
WHERE a.tenant_id = m.tenant_id AND a.kind = 'ledger' AND a.code = CASE m.kind
    WHEN 'cash' THEN '1110' WHEN 'transfer' THEN '1120' WHEN 'debit' THEN '1120' WHEN 'ewallet' THEN '1130'
    WHEN 'credit_card' THEN '1140' WHEN 'deposit' THEN '2140' WHEN 'supplier_credit' THEN '1320' END;

-- +goose Down
ALTER TABLE payment_methods DROP CONSTRAINT payment_methods_account_fk;
DROP INDEX payment_methods_account_idx;
ALTER TABLE payment_methods DROP COLUMN account_id;
