-- +goose Up

-- Pelunasan kolektif (bayar per pemasok / per member). Satu dokumen header + beberapa alokasi per nota. Alokasi tetap berupa
-- baris payable_payments / receivable_payments (settlement_id terisi), jadi saldo per nota tetap DIHITUNG dari pembayaran
-- dan bayar per nota yang sudah ada tidak berubah. Append-only.

CREATE TABLE payable_settlements (
    id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id       uuid          NOT NULL,
    outlet_id       uuid          NOT NULL,
    supplier_id     uuid          NOT NULL,
    doc_no          text          NOT NULL CHECK (char_length(doc_no) BETWEEN 1 AND 64),
    idempotency_key text          NOT NULL CHECK (char_length(idempotency_key) BETWEEN 8 AND 100),
    request_hash    text          NOT NULL,
    mode            text          NOT NULL CHECK (mode IN ('auto', 'manual')),
    method          text          NOT NULL CHECK (method IN ('cash', 'debit', 'credit_card', 'ewallet', 'transfer')),
    method_id       uuid          NOT NULL,
    method_name     text          NOT NULL,
    total           numeric(18,2) NOT NULL CHECK (total > 0),
    ref_no          text          NOT NULL DEFAULT '' CHECK (char_length(ref_no) <= 100),
    note            text          NOT NULL DEFAULT '' CHECK (char_length(note) <= 200),
    paid_by         uuid,
    created_at      timestamptz   NOT NULL DEFAULT now(),
    CONSTRAINT payable_settlements_tenant_id_id_key UNIQUE (tenant_id, id),
    CONSTRAINT payable_settlements_doc_no_key UNIQUE (tenant_id, doc_no),
    CONSTRAINT payable_settlements_idem_key   UNIQUE (tenant_id, idempotency_key),
    CONSTRAINT payable_settlements_outlet_fk   FOREIGN KEY (tenant_id, outlet_id)   REFERENCES outlets (tenant_id, id) ON DELETE RESTRICT,
    CONSTRAINT payable_settlements_supplier_fk FOREIGN KEY (tenant_id, supplier_id) REFERENCES suppliers (tenant_id, id) ON DELETE RESTRICT,
    CONSTRAINT payable_settlements_method_fk   FOREIGN KEY (tenant_id, method_id)   REFERENCES payment_methods (tenant_id, id) ON DELETE RESTRICT,
    CONSTRAINT payable_settlements_user_fk     FOREIGN KEY (tenant_id, paid_by)     REFERENCES users (tenant_id, id) ON DELETE RESTRICT
);

CREATE TABLE receivable_settlements (
    id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id       uuid          NOT NULL,
    outlet_id       uuid          NOT NULL,
    member_id       uuid          NOT NULL,
    doc_no          text          NOT NULL CHECK (char_length(doc_no) BETWEEN 1 AND 64),
    idempotency_key text          NOT NULL CHECK (char_length(idempotency_key) BETWEEN 8 AND 100),
    request_hash    text          NOT NULL,
    mode            text          NOT NULL CHECK (mode IN ('auto', 'manual')),
    method          text          NOT NULL CHECK (method IN ('cash', 'debit', 'credit_card', 'ewallet', 'transfer')),
    method_id       uuid          NOT NULL,
    method_name     text          NOT NULL,
    total           numeric(18,2) NOT NULL CHECK (total > 0),
    fee_amount      numeric(18,2) NOT NULL DEFAULT 0 CHECK (fee_amount >= 0),
    ref_no          text          NOT NULL DEFAULT '' CHECK (char_length(ref_no) <= 100),
    note            text          NOT NULL DEFAULT '' CHECK (char_length(note) <= 200),
    received_by     uuid,
    created_at      timestamptz   NOT NULL DEFAULT now(),
    CONSTRAINT receivable_settlements_tenant_id_id_key UNIQUE (tenant_id, id),
    CONSTRAINT receivable_settlements_doc_no_key UNIQUE (tenant_id, doc_no),
    CONSTRAINT receivable_settlements_idem_key   UNIQUE (tenant_id, idempotency_key),
    CONSTRAINT receivable_settlements_outlet_fk FOREIGN KEY (tenant_id, outlet_id) REFERENCES outlets (tenant_id, id) ON DELETE RESTRICT,
    CONSTRAINT receivable_settlements_member_fk FOREIGN KEY (tenant_id, member_id) REFERENCES members (tenant_id, id) ON DELETE RESTRICT,
    CONSTRAINT receivable_settlements_method_fk FOREIGN KEY (tenant_id, method_id) REFERENCES payment_methods (tenant_id, id) ON DELETE RESTRICT,
    CONSTRAINT receivable_settlements_user_fk   FOREIGN KEY (tenant_id, received_by) REFERENCES users (tenant_id, id) ON DELETE RESTRICT
);

ALTER TABLE payable_payments    ADD COLUMN settlement_id uuid;
ALTER TABLE receivable_payments ADD COLUMN settlement_id uuid;
ALTER TABLE payable_payments    ADD CONSTRAINT payable_payments_settlement_fk    FOREIGN KEY (tenant_id, settlement_id) REFERENCES payable_settlements (tenant_id, id) ON DELETE RESTRICT;
ALTER TABLE receivable_payments ADD CONSTRAINT receivable_payments_settlement_fk FOREIGN KEY (tenant_id, settlement_id) REFERENCES receivable_settlements (tenant_id, id) ON DELETE RESTRICT;
CREATE INDEX payable_payments_settlement_idx    ON payable_payments (tenant_id, settlement_id) WHERE settlement_id IS NOT NULL;
CREATE INDEX receivable_payments_settlement_idx ON receivable_payments (tenant_id, settlement_id) WHERE settlement_id IS NOT NULL;

ALTER TABLE payable_settlements    ENABLE ROW LEVEL SECURITY;
ALTER TABLE receivable_settlements ENABLE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON payable_settlements    USING (tenant_id = app_tenant_id()) WITH CHECK (tenant_id = app_tenant_id());
CREATE POLICY tenant_isolation ON receivable_settlements USING (tenant_id = app_tenant_id()) WITH CHECK (tenant_id = app_tenant_id());
REVOKE UPDATE, DELETE, TRUNCATE ON payable_settlements, receivable_settlements FROM aciraba_app;

-- +goose Down
DROP INDEX receivable_payments_settlement_idx;
DROP INDEX payable_payments_settlement_idx;
ALTER TABLE receivable_payments DROP CONSTRAINT receivable_payments_settlement_fk;
ALTER TABLE payable_payments    DROP CONSTRAINT payable_payments_settlement_fk;
ALTER TABLE receivable_payments DROP COLUMN settlement_id;
ALTER TABLE payable_payments    DROP COLUMN settlement_id;
DROP TABLE receivable_settlements;
DROP TABLE payable_settlements;
