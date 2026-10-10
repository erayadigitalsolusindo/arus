-- +goose Up

-- Deposit member & kredit pemasok (keputusan pengguna 2026-10-10) + batal retur penjualan.
--
-- Dua jenis metode bayar baru, masing-masing SATU metode sistem per tenant, tanpa biaya:
--   deposit         = "Deposit Member": dipakai bayar nota/piutang member, tujuan dana kembali retur penjualan.
--   supplier_credit = "Kredit Pemasok": dipakai bayar hutang pemasok, tujuan dana kembali retur pembelian.
-- Saldo masing-masing DIHITUNG dari ledger append-only (balance_after baris terakhir), bukan kolom yang dimutasi
-- (AGENTS.md §8). Penulis ledger mengambil advisory lock per pemilik saldo lalu menulis baris dengan balance_after ≥ 0.

ALTER TABLE payment_methods DROP CONSTRAINT payment_methods_kind_check;
ALTER TABLE payment_methods ADD CONSTRAINT payment_methods_kind_check
    CHECK (kind IN ('cash', 'debit', 'credit_card', 'ewallet', 'transfer', 'deposit', 'supplier_credit'));
ALTER TABLE payment_methods DROP CONSTRAINT payment_methods_system_cash;
ALTER TABLE payment_methods ADD CONSTRAINT payment_methods_system_kinds
    CHECK (NOT is_system OR (kind IN ('cash', 'deposit', 'supplier_credit') AND active));
ALTER TABLE payment_methods ADD CONSTRAINT payment_methods_internal_no_fee
    CHECK (kind NOT IN ('deposit', 'supplier_credit') OR (fee_pct = 0 AND fee_flat = 0 AND fee_bearer = 'store'));
ALTER TABLE payment_methods ADD CONSTRAINT payment_methods_internal_system
    CHECK (kind NOT IN ('deposit', 'supplier_credit') OR is_system);
CREATE UNIQUE INDEX payment_methods_one_deposit_key ON payment_methods (tenant_id) WHERE kind = 'deposit';
CREATE UNIQUE INDEX payment_methods_one_supplier_credit_key ON payment_methods (tenant_id) WHERE kind = 'supplier_credit';

INSERT INTO payment_methods (tenant_id, name, kind, is_system)
SELECT t.id, d.name, d.kind, true
FROM tenants t CROSS JOIN (VALUES ('Deposit Member', 'deposit'), ('Kredit Pemasok', 'supplier_credit')) AS d (name, kind)
WHERE NOT EXISTS (SELECT 1 FROM payment_methods m WHERE m.tenant_id = t.id AND m.kind = d.kind)
ON CONFLICT DO NOTHING;

ALTER TABLE sale_payments DROP CONSTRAINT sale_payments_method_check;
ALTER TABLE sale_payments ADD CONSTRAINT sale_payments_method_check
    CHECK (method IN ('cash', 'debit', 'credit_card', 'ewallet', 'transfer', 'deposit'));
ALTER TABLE receivable_payments DROP CONSTRAINT receivable_payments_method_check;
ALTER TABLE receivable_payments ADD CONSTRAINT receivable_payments_method_check
    CHECK (method IN ('cash', 'debit', 'credit_card', 'ewallet', 'transfer', 'deposit'));
ALTER TABLE receivable_settlements DROP CONSTRAINT receivable_settlements_method_check;
ALTER TABLE receivable_settlements ADD CONSTRAINT receivable_settlements_method_check
    CHECK (method IN ('cash', 'debit', 'credit_card', 'ewallet', 'transfer', 'deposit'));
ALTER TABLE payable_payments DROP CONSTRAINT payable_payments_method_check;
ALTER TABLE payable_payments ADD CONSTRAINT payable_payments_method_check
    CHECK (method IN ('cash', 'debit', 'credit_card', 'ewallet', 'transfer', 'supplier_credit'));
ALTER TABLE payable_settlements DROP CONSTRAINT payable_settlements_method_check;
ALTER TABLE payable_settlements ADD CONSTRAINT payable_settlements_method_check
    CHECK (method IN ('cash', 'debit', 'credit_card', 'ewallet', 'transfer', 'supplier_credit'));
ALTER TABLE sales_returns DROP CONSTRAINT sales_returns_refund_method_check;
ALTER TABLE sales_returns ADD CONSTRAINT sales_returns_refund_method_check
    CHECK (refund_method IN ('cash', 'debit', 'credit_card', 'ewallet', 'transfer', 'deposit'));
ALTER TABLE purchase_returns DROP CONSTRAINT purchase_returns_refund_method_check;
ALTER TABLE purchase_returns ADD CONSTRAINT purchase_returns_refund_method_check
    CHECK (refund_method IN ('cash', 'debit', 'credit_card', 'ewallet', 'transfer', 'supplier_credit'));

-- ---- Ledger deposit member ----
-- TOPUP/WITHDRAW = dokumen sendiri (nomor DM-…, metode bayar + idempotensi). Baris lain menunjuk dokumen sumbernya (ref_id).
CREATE TABLE member_deposit_movements (
    id              bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    tenant_id       uuid          NOT NULL,
    member_id       uuid          NOT NULL,
    outlet_id       uuid          NOT NULL,
    kind            text          NOT NULL CHECK (kind IN ('TOPUP', 'WITHDRAW', 'SALE_PAYMENT', 'SALE_REVERSAL', 'RECEIVABLE_PAYMENT',
                                                           'SALE_RETURN', 'SALE_RETURN_VOID')),
    amount          numeric(18,2) NOT NULL CHECK (amount <> 0),
    balance_after   numeric(18,2) NOT NULL CHECK (balance_after >= 0),
    ref_id          uuid,
    doc_no          text          NOT NULL DEFAULT '' CHECK (char_length(doc_no) <= 64),
    method          text          CHECK (method IN ('cash', 'debit', 'credit_card', 'ewallet', 'transfer')),
    method_id       uuid,
    method_name     text          NOT NULL DEFAULT '',
    ref_no          text          NOT NULL DEFAULT '' CHECK (char_length(ref_no) <= 100),
    note            text          NOT NULL DEFAULT '' CHECK (char_length(note) <= 200),
    idempotency_key text          CHECK (char_length(idempotency_key) BETWEEN 8 AND 100),
    request_hash    text,
    actor_id        uuid,
    created_at      timestamptz   NOT NULL DEFAULT now(),
    CONSTRAINT member_deposit_sign_chk CHECK (
        (kind IN ('TOPUP', 'SALE_REVERSAL', 'SALE_RETURN') AND amount > 0)
     OR (kind IN ('WITHDRAW', 'SALE_PAYMENT', 'RECEIVABLE_PAYMENT', 'SALE_RETURN_VOID') AND amount < 0)),
    CONSTRAINT member_deposit_cash_doc_chk CHECK ((kind IN ('TOPUP', 'WITHDRAW')) = (method_id IS NOT NULL AND idempotency_key IS NOT NULL)),
    CONSTRAINT member_deposit_member_fk FOREIGN KEY (tenant_id, member_id) REFERENCES members (tenant_id, id) ON DELETE RESTRICT,
    CONSTRAINT member_deposit_outlet_fk FOREIGN KEY (tenant_id, outlet_id) REFERENCES outlets (tenant_id, id) ON DELETE RESTRICT,
    CONSTRAINT member_deposit_method_fk FOREIGN KEY (tenant_id, method_id) REFERENCES payment_methods (tenant_id, id) ON DELETE RESTRICT,
    CONSTRAINT member_deposit_actor_fk  FOREIGN KEY (tenant_id, actor_id)  REFERENCES users (tenant_id, id) ON DELETE RESTRICT
);
CREATE INDEX member_deposit_member_idx ON member_deposit_movements (tenant_id, member_id, id DESC);
CREATE INDEX member_deposit_actor_idx  ON member_deposit_movements (tenant_id, actor_id, created_at) WHERE method_id IS NOT NULL;
CREATE INDEX member_deposit_ref_idx    ON member_deposit_movements (tenant_id, ref_id) WHERE ref_id IS NOT NULL;
CREATE UNIQUE INDEX member_deposit_idem_key ON member_deposit_movements (tenant_id, idempotency_key) WHERE idempotency_key IS NOT NULL;

-- ---- Ledger kredit pemasok ----
-- CASH_OUT = pemasok mencairkan kreditnya (uang masuk ke toko, dokumen KP-…).
CREATE TABLE supplier_credit_movements (
    id              bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    tenant_id       uuid          NOT NULL,
    supplier_id     uuid          NOT NULL,
    outlet_id       uuid          NOT NULL,
    kind            text          NOT NULL CHECK (kind IN ('PURCHASE_RETURN', 'PURCHASE_RETURN_VOID', 'PAYABLE_PAYMENT', 'CASH_OUT')),
    amount          numeric(18,2) NOT NULL CHECK (amount <> 0),
    balance_after   numeric(18,2) NOT NULL CHECK (balance_after >= 0),
    ref_id          uuid,
    doc_no          text          NOT NULL DEFAULT '' CHECK (char_length(doc_no) <= 64),
    method          text          CHECK (method IN ('cash', 'debit', 'credit_card', 'ewallet', 'transfer')),
    method_id       uuid,
    method_name     text          NOT NULL DEFAULT '',
    ref_no          text          NOT NULL DEFAULT '' CHECK (char_length(ref_no) <= 100),
    note            text          NOT NULL DEFAULT '' CHECK (char_length(note) <= 200),
    idempotency_key text          CHECK (char_length(idempotency_key) BETWEEN 8 AND 100),
    request_hash    text,
    actor_id        uuid,
    created_at      timestamptz   NOT NULL DEFAULT now(),
    CONSTRAINT supplier_credit_sign_chk CHECK ((kind = 'PURCHASE_RETURN' AND amount > 0) OR (kind <> 'PURCHASE_RETURN' AND amount < 0)),
    CONSTRAINT supplier_credit_cash_doc_chk CHECK ((kind = 'CASH_OUT') = (method_id IS NOT NULL AND idempotency_key IS NOT NULL)),
    CONSTRAINT supplier_credit_supplier_fk FOREIGN KEY (tenant_id, supplier_id) REFERENCES suppliers (tenant_id, id) ON DELETE RESTRICT,
    CONSTRAINT supplier_credit_outlet_fk   FOREIGN KEY (tenant_id, outlet_id)   REFERENCES outlets (tenant_id, id) ON DELETE RESTRICT,
    CONSTRAINT supplier_credit_method_fk   FOREIGN KEY (tenant_id, method_id)   REFERENCES payment_methods (tenant_id, id) ON DELETE RESTRICT,
    CONSTRAINT supplier_credit_actor_fk    FOREIGN KEY (tenant_id, actor_id)    REFERENCES users (tenant_id, id) ON DELETE RESTRICT
);
CREATE INDEX supplier_credit_supplier_idx ON supplier_credit_movements (tenant_id, supplier_id, id DESC);
CREATE INDEX supplier_credit_actor_idx    ON supplier_credit_movements (tenant_id, actor_id, created_at) WHERE method_id IS NOT NULL;
CREATE INDEX supplier_credit_ref_idx      ON supplier_credit_movements (tenant_id, ref_id) WHERE ref_id IS NOT NULL;
CREATE UNIQUE INDEX supplier_credit_idem_key ON supplier_credit_movements (tenant_id, idempotency_key) WHERE idempotency_key IS NOT NULL;

-- Nomor dokumen top-up/tarik deposit (DM-…) dan pencairan kredit pemasok (KP-…) per outlet per hari.
CREATE TABLE wallet_doc_counters (
    tenant_id uuid   NOT NULL,
    outlet_id uuid   NOT NULL,
    prefix    text   NOT NULL CHECK (prefix IN ('DM', 'KP')),
    day       date   NOT NULL,
    last_no   bigint NOT NULL,
    PRIMARY KEY (tenant_id, outlet_id, prefix, day),
    CONSTRAINT wallet_doc_counters_outlet_fk FOREIGN KEY (tenant_id, outlet_id) REFERENCES outlets (tenant_id, id) ON DELETE RESTRICT
);

ALTER TABLE member_deposit_movements  ENABLE ROW LEVEL SECURITY;
ALTER TABLE supplier_credit_movements ENABLE ROW LEVEL SECURITY;
ALTER TABLE wallet_doc_counters       ENABLE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON member_deposit_movements  USING (tenant_id = app_tenant_id()) WITH CHECK (tenant_id = app_tenant_id());
CREATE POLICY tenant_isolation ON supplier_credit_movements USING (tenant_id = app_tenant_id()) WITH CHECK (tenant_id = app_tenant_id());
CREATE POLICY tenant_isolation ON wallet_doc_counters       USING (tenant_id = app_tenant_id()) WITH CHECK (tenant_id = app_tenant_id());
REVOKE UPDATE, DELETE, TRUNCATE ON member_deposit_movements, supplier_credit_movements FROM aciraba_app;
REVOKE DELETE, TRUNCATE ON wallet_doc_counters FROM aciraba_app;

-- ---- Batal retur penjualan ----
ALTER TABLE sales_returns
    ADD COLUMN status      text        NOT NULL DEFAULT 'completed' CHECK (status IN ('completed', 'void')),
    ADD COLUMN void_reason text        NOT NULL DEFAULT '' CHECK (char_length(void_reason) <= 200),
    ADD COLUMN voided_at   timestamptz,
    ADD COLUMN voided_by   uuid,
    ADD CONSTRAINT sales_returns_voider_fk FOREIGN KEY (tenant_id, voided_by) REFERENCES users (tenant_id, id) ON DELETE RESTRICT,
    ADD CONSTRAINT sales_returns_state_chk CHECK (
        (status = 'completed' AND voided_at IS NULL)
     OR (status = 'void' AND voided_at IS NOT NULL AND voided_by IS NOT NULL AND char_length(void_reason) >= 3));
GRANT UPDATE (status, void_reason, voided_at, voided_by) ON sales_returns TO aciraba_app;
-- Uang keluar/masuk per petugas (ringkasan laci kasir).
CREATE INDEX sales_returns_creator_idx    ON sales_returns (tenant_id, created_by, created_at);
CREATE INDEX purchase_returns_creator_idx ON purchase_returns (tenant_id, created_by, created_at);
CREATE INDEX receivable_payments_receiver_idx ON receivable_payments (tenant_id, received_by, created_at);
CREATE INDEX payable_payments_payer_idx   ON payable_payments (tenant_id, paid_by, created_at);

-- +goose Down
DROP INDEX IF EXISTS payable_payments_payer_idx;
DROP INDEX IF EXISTS receivable_payments_receiver_idx;
DROP INDEX IF EXISTS purchase_returns_creator_idx;
DROP INDEX IF EXISTS sales_returns_creator_idx;
REVOKE UPDATE (status, void_reason, voided_at, voided_by) ON sales_returns FROM aciraba_app;
ALTER TABLE sales_returns DROP CONSTRAINT sales_returns_state_chk, DROP CONSTRAINT sales_returns_voider_fk,
    DROP COLUMN voided_by, DROP COLUMN voided_at, DROP COLUMN void_reason, DROP COLUMN status;
DROP TABLE wallet_doc_counters;
DROP TABLE supplier_credit_movements;
DROP TABLE member_deposit_movements;
ALTER TABLE purchase_returns DROP CONSTRAINT purchase_returns_refund_method_check;
ALTER TABLE purchase_returns ADD CONSTRAINT purchase_returns_refund_method_check CHECK (refund_method IN ('cash', 'debit', 'credit_card', 'ewallet', 'transfer'));
ALTER TABLE sales_returns DROP CONSTRAINT sales_returns_refund_method_check;
ALTER TABLE sales_returns ADD CONSTRAINT sales_returns_refund_method_check CHECK (refund_method IN ('cash', 'debit', 'credit_card', 'ewallet', 'transfer'));
ALTER TABLE payable_settlements DROP CONSTRAINT payable_settlements_method_check;
ALTER TABLE payable_settlements ADD CONSTRAINT payable_settlements_method_check CHECK (method IN ('cash', 'debit', 'credit_card', 'ewallet', 'transfer'));
ALTER TABLE payable_payments DROP CONSTRAINT payable_payments_method_check;
ALTER TABLE payable_payments ADD CONSTRAINT payable_payments_method_check CHECK (method IN ('cash', 'debit', 'credit_card', 'ewallet', 'transfer'));
ALTER TABLE receivable_settlements DROP CONSTRAINT receivable_settlements_method_check;
ALTER TABLE receivable_settlements ADD CONSTRAINT receivable_settlements_method_check CHECK (method IN ('cash', 'debit', 'credit_card', 'ewallet', 'transfer'));
ALTER TABLE receivable_payments DROP CONSTRAINT receivable_payments_method_check;
ALTER TABLE receivable_payments ADD CONSTRAINT receivable_payments_method_check CHECK (method IN ('cash', 'debit', 'credit_card', 'ewallet', 'transfer'));
ALTER TABLE sale_payments DROP CONSTRAINT sale_payments_method_check;
ALTER TABLE sale_payments ADD CONSTRAINT sale_payments_method_check CHECK (method IN ('cash', 'debit', 'credit_card', 'ewallet', 'transfer'));
DELETE FROM payment_methods WHERE kind IN ('deposit', 'supplier_credit');
DROP INDEX payment_methods_one_supplier_credit_key;
DROP INDEX payment_methods_one_deposit_key;
ALTER TABLE payment_methods DROP CONSTRAINT payment_methods_internal_system;
ALTER TABLE payment_methods DROP CONSTRAINT payment_methods_internal_no_fee;
ALTER TABLE payment_methods DROP CONSTRAINT payment_methods_system_kinds;
ALTER TABLE payment_methods ADD CONSTRAINT payment_methods_system_cash CHECK (NOT is_system OR (kind = 'cash' AND active));
ALTER TABLE payment_methods DROP CONSTRAINT payment_methods_kind_check;
ALTER TABLE payment_methods ADD CONSTRAINT payment_methods_kind_check CHECK (kind IN ('cash', 'debit', 'credit_card', 'ewallet', 'transfer'));
