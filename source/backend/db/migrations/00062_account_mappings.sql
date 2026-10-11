-- +goose Up

-- Pemetaan peran akuntansi -> akun COA untuk jurnal otomatis (FR-ACC-02). Satu baris per peran (key) per tenant; peran dikunci
-- oleh service (internal/accounting/mappings.go). Akun per metode bayar tetap di payment_methods.account_id.
CREATE TABLE account_mappings (
    tenant_id  uuid        NOT NULL,
    key        text        NOT NULL CHECK (char_length(key) BETWEEN 1 AND 40),
    account_id uuid        NOT NULL,
    updated_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (tenant_id, key),
    CONSTRAINT account_mappings_account_fk FOREIGN KEY (tenant_id, account_id) REFERENCES accounts (tenant_id, id) ON DELETE RESTRICT
);
CREATE INDEX account_mappings_account_idx ON account_mappings (tenant_id, account_id);

ALTER TABLE account_mappings ENABLE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON account_mappings USING (tenant_id = app_tenant_id()) WITH CHECK (tenant_id = app_tenant_id());
REVOKE TRUNCATE ON account_mappings FROM aciraba_app;

-- Backfill tenant yang sudah punya bagan akun (kode sama dengan defaultMappings di internal/accounting/mappings.go).
INSERT INTO account_mappings (tenant_id, key, account_id)
SELECT a.tenant_id, d.key, a.id
FROM (VALUES ('sales_revenue', '4100'), ('other_income', '4200'), ('receivable', '1210'), ('cogs', '5100'), ('inventory', '1310'),
             ('tax_out', '2130'), ('tax_in', '1330'), ('payable', '2110'), ('fee_expense', '6160'), ('stock_variance', '5200'),
             ('member_deposit', '2140'), ('supplier_advance', '1320'), ('cash_over', '4900'), ('cash_short', '6900')) AS d (key, code)
JOIN accounts a ON a.code = d.code AND a.kind = 'ledger'
ON CONFLICT DO NOTHING;

-- +goose Down
DROP TABLE account_mappings;
