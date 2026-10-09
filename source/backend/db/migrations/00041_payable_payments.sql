-- +goose Up

-- Pembayaran hutang pemasok (Fase 6.3). Saldo hutang DIHITUNG (payables.amount − Σ payable_payments.amount), tidak ada kolom
-- saldo yang dimutasi (AGENTS.md §8). Append-only: koreksi pembayaran = dokumen pembatalan (belum ada).
-- Nota pembelian yang hutangnya sudah dibayar (sebagian/seluruh) tidak boleh diedit/dibatalkan (dijaga service purchasing).

CREATE TABLE payable_payment_counters (
    tenant_id uuid   NOT NULL,
    outlet_id uuid   NOT NULL,
    day       date   NOT NULL,                    -- hari menurut zona waktu outlet
    last_no   bigint NOT NULL,
    PRIMARY KEY (tenant_id, outlet_id, day),
    CONSTRAINT payable_payment_counters_outlet_fk FOREIGN KEY (tenant_id, outlet_id) REFERENCES outlets (tenant_id, id) ON DELETE RESTRICT
);

CREATE TABLE payable_payments (
    id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id       uuid          NOT NULL,
    payable_id      uuid          NOT NULL,
    outlet_id       uuid          NOT NULL,       -- outlet tempat uang keluar
    doc_no          text          NOT NULL CHECK (char_length(doc_no) BETWEEN 1 AND 64),
    idempotency_key text          NOT NULL CHECK (char_length(idempotency_key) BETWEEN 8 AND 100),
    request_hash    text          NOT NULL,
    method          text          NOT NULL CHECK (method IN ('cash', 'debit', 'credit_card', 'ewallet', 'transfer')),
    method_id       uuid          NOT NULL,
    method_name     text          NOT NULL,       -- snapshot nama metode saat dibayar
    amount          numeric(18,2) NOT NULL CHECK (amount > 0),
    ref_no          text          NOT NULL DEFAULT '' CHECK (char_length(ref_no) <= 100),
    note            text          NOT NULL DEFAULT '' CHECK (char_length(note) <= 200),
    paid_by         uuid,
    created_at      timestamptz   NOT NULL DEFAULT now(),
    CONSTRAINT payable_payments_doc_no_key UNIQUE (tenant_id, doc_no),
    CONSTRAINT payable_payments_idem_key   UNIQUE (tenant_id, idempotency_key),
    CONSTRAINT payable_payments_payable_fk FOREIGN KEY (tenant_id, payable_id) REFERENCES payables (tenant_id, id) ON DELETE RESTRICT,
    CONSTRAINT payable_payments_outlet_fk  FOREIGN KEY (tenant_id, outlet_id)  REFERENCES outlets  (tenant_id, id) ON DELETE RESTRICT,
    CONSTRAINT payable_payments_method_fk  FOREIGN KEY (tenant_id, method_id)  REFERENCES payment_methods (tenant_id, id) ON DELETE RESTRICT,
    CONSTRAINT payable_payments_user_fk    FOREIGN KEY (tenant_id, paid_by)    REFERENCES users    (tenant_id, id) ON DELETE RESTRICT
);
CREATE INDEX payable_payments_payable_idx ON payable_payments (tenant_id, payable_id);

ALTER TABLE payable_payment_counters ENABLE ROW LEVEL SECURITY;
ALTER TABLE payable_payments         ENABLE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON payable_payment_counters USING (tenant_id = app_tenant_id()) WITH CHECK (tenant_id = app_tenant_id());
CREATE POLICY tenant_isolation ON payable_payments         USING (tenant_id = app_tenant_id()) WITH CHECK (tenant_id = app_tenant_id());

REVOKE DELETE, TRUNCATE ON payable_payment_counters FROM aciraba_app;
REVOKE UPDATE, DELETE, TRUNCATE ON payable_payments FROM aciraba_app;

-- +goose Down
DROP TABLE payable_payments;
DROP TABLE payable_payment_counters;
