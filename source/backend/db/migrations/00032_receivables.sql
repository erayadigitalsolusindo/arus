-- +goose Up

-- Penjualan kredit & piutang member (Fase 5.1/5.3). Nota kredit = bayar di muka (DP) lewat metode biasa + sisa jadi piutang
-- (sales.receivable). Piutang hanya untuk member. Saldo piutang DIHITUNG (amount − Σ receivable_payments), tidak ada kolom
-- saldo yang dimutasi (AGENTS.md §8). Semua tabel append-only: koreksi = nota diedit/dibatalkan (hanya selama belum ada
-- pembayaran) atau, nanti, dokumen pembatalan pembayaran.

ALTER TABLE sales ADD COLUMN receivable numeric(18,2) NOT NULL DEFAULT 0 CHECK (receivable >= 0);
ALTER TABLE sales DROP CONSTRAINT sales_change_chk;
ALTER TABLE sales ADD CONSTRAINT sales_settle_chk CHECK (
    (receivable = 0 AND change = paid - total)
    OR (receivable > 0 AND change = 0 AND paid + receivable = total AND member_id IS NOT NULL)
);

CREATE TABLE receivables (
    id         uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id  uuid          NOT NULL,
    outlet_id  uuid          NOT NULL,
    sale_id    uuid          NOT NULL,
    member_id  uuid          NOT NULL,
    amount     numeric(18,2) NOT NULL CHECK (amount > 0),
    due_date   date,                              -- NULL = tanpa jatuh tempo (member.due_days = 0)
    created_at timestamptz   NOT NULL DEFAULT now(),
    CONSTRAINT receivables_tenant_id_id_key UNIQUE (tenant_id, id),
    CONSTRAINT receivables_sale_key UNIQUE (tenant_id, sale_id),
    CONSTRAINT receivables_outlet_fk FOREIGN KEY (tenant_id, outlet_id) REFERENCES outlets (tenant_id, id) ON DELETE RESTRICT,
    CONSTRAINT receivables_sale_fk   FOREIGN KEY (tenant_id, sale_id)   REFERENCES sales   (tenant_id, id) ON DELETE RESTRICT,
    CONSTRAINT receivables_member_fk FOREIGN KEY (tenant_id, member_id) REFERENCES members (tenant_id, id) ON DELETE RESTRICT
);
CREATE INDEX receivables_member_idx ON receivables (tenant_id, member_id);
CREATE INDEX receivables_due_idx    ON receivables (tenant_id, due_date) WHERE due_date IS NOT NULL;

CREATE TABLE receivable_payment_counters (
    tenant_id uuid   NOT NULL,
    outlet_id uuid   NOT NULL,
    day       date   NOT NULL,                    -- hari menurut zona waktu outlet
    last_no   bigint NOT NULL,
    PRIMARY KEY (tenant_id, outlet_id, day),
    CONSTRAINT receivable_payment_counters_outlet_fk FOREIGN KEY (tenant_id, outlet_id) REFERENCES outlets (tenant_id, id) ON DELETE RESTRICT
);

CREATE TABLE receivable_payments (
    id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id       uuid          NOT NULL,
    receivable_id   uuid          NOT NULL,
    outlet_id       uuid          NOT NULL,
    doc_no          text          NOT NULL CHECK (char_length(doc_no) BETWEEN 1 AND 64),
    idempotency_key text          NOT NULL CHECK (char_length(idempotency_key) BETWEEN 8 AND 100),
    request_hash    text          NOT NULL,
    method          text          NOT NULL CHECK (method IN ('cash', 'debit', 'credit_card', 'ewallet', 'transfer')),
    method_id       uuid          NOT NULL,
    method_name     text          NOT NULL,       -- snapshot nama metode saat dibayar
    amount          numeric(18,2) NOT NULL CHECK (amount > 0),   -- bagian yang MELUNASI piutang
    ref_no          text          NOT NULL DEFAULT '' CHECK (char_length(ref_no) <= 100),
    fee_pct         numeric(5,2)  NOT NULL DEFAULT 0,
    fee_flat        numeric(18,2) NOT NULL DEFAULT 0,
    fee_amount      numeric(18,2) NOT NULL DEFAULT 0 CHECK (fee_amount >= 0),
    fee_bearer      text          NOT NULL DEFAULT 'store' CHECK (fee_bearer IN ('store', 'customer')),
    note            text          NOT NULL DEFAULT '' CHECK (char_length(note) <= 200),
    received_by     uuid,
    created_at      timestamptz   NOT NULL DEFAULT now(),
    CONSTRAINT receivable_payments_doc_no_key UNIQUE (tenant_id, doc_no),
    CONSTRAINT receivable_payments_idem_key   UNIQUE (tenant_id, idempotency_key),
    CONSTRAINT receivable_payments_recv_fk   FOREIGN KEY (tenant_id, receivable_id) REFERENCES receivables (tenant_id, id) ON DELETE RESTRICT,
    CONSTRAINT receivable_payments_outlet_fk FOREIGN KEY (tenant_id, outlet_id)     REFERENCES outlets     (tenant_id, id) ON DELETE RESTRICT,
    CONSTRAINT receivable_payments_method_fk FOREIGN KEY (tenant_id, method_id)     REFERENCES payment_methods (tenant_id, id) ON DELETE RESTRICT,
    CONSTRAINT receivable_payments_user_fk   FOREIGN KEY (tenant_id, received_by)   REFERENCES users       (tenant_id, id) ON DELETE RESTRICT
);
CREATE INDEX receivable_payments_recv_idx ON receivable_payments (tenant_id, receivable_id);

ALTER TABLE receivables                 ENABLE ROW LEVEL SECURITY;
ALTER TABLE receivable_payment_counters ENABLE ROW LEVEL SECURITY;
ALTER TABLE receivable_payments         ENABLE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON receivables                 USING (tenant_id = app_tenant_id()) WITH CHECK (tenant_id = app_tenant_id());
CREATE POLICY tenant_isolation ON receivable_payment_counters USING (tenant_id = app_tenant_id()) WITH CHECK (tenant_id = app_tenant_id());
CREATE POLICY tenant_isolation ON receivable_payments         USING (tenant_id = app_tenant_id()) WITH CHECK (tenant_id = app_tenant_id());

REVOKE DELETE, TRUNCATE, UPDATE ON receivables, receivable_payments FROM aciraba_app;
REVOKE DELETE, TRUNCATE ON receivable_payment_counters FROM aciraba_app;

-- +goose Down
DROP TABLE receivable_payments;
DROP TABLE receivable_payment_counters;
DROP TABLE receivables;
ALTER TABLE sales DROP CONSTRAINT sales_settle_chk;
ALTER TABLE sales ADD CONSTRAINT sales_change_chk CHECK (change = paid - total);
ALTER TABLE sales DROP COLUMN receivable;
