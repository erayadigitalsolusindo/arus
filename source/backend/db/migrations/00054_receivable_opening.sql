-- +goose Up

-- Saldo awal piutang member (FR-ONB-04): piutang tanpa nota sumber (kind = 'opening'), dicatat saat onboarding dari
-- catatan lama toko (no. referensi + tanggal dokumen lama). Dibayar/dicicil/dilunasi kolektif seperti piutang nota dan
-- ikut dihitung dalam limit kredit member. Bisa dibatalkan (void + alasan) selama belum ada pembayaran.
ALTER TABLE receivables
    ALTER COLUMN sale_id DROP NOT NULL,
    ADD COLUMN kind            text NOT NULL DEFAULT 'sale' CHECK (kind IN ('sale', 'opening')),
    ADD COLUMN doc_no          text CHECK (doc_no IS NULL OR char_length(doc_no) BETWEEN 1 AND 64),
    ADD COLUMN ref_no          text NOT NULL DEFAULT '' CHECK (char_length(ref_no) <= 64),
    ADD COLUMN doc_date        date,
    ADD COLUMN note            text NOT NULL DEFAULT '' CHECK (char_length(note) <= 200),
    ADD COLUMN created_by      uuid,
    ADD COLUMN idempotency_key text CHECK (idempotency_key IS NULL OR char_length(idempotency_key) BETWEEN 8 AND 100),
    ADD COLUMN request_hash    text,
    ADD COLUMN voided_at       timestamptz,
    ADD COLUMN voided_by       uuid,
    ADD COLUMN void_reason     text,
    ADD CONSTRAINT receivables_creator_fk FOREIGN KEY (tenant_id, created_by) REFERENCES users (tenant_id, id) ON DELETE RESTRICT,
    ADD CONSTRAINT receivables_voider_fk  FOREIGN KEY (tenant_id, voided_by)  REFERENCES users (tenant_id, id) ON DELETE RESTRICT,
    ADD CONSTRAINT receivables_kind_chk CHECK (
        (kind = 'sale' AND sale_id IS NOT NULL AND doc_no IS NULL AND doc_date IS NULL AND idempotency_key IS NULL AND voided_at IS NULL)
        OR (kind = 'opening' AND sale_id IS NULL AND doc_no IS NOT NULL AND doc_date IS NOT NULL AND created_by IS NOT NULL
            AND idempotency_key IS NOT NULL AND request_hash IS NOT NULL AND (due_date IS NULL OR due_date >= doc_date))
    ),
    ADD CONSTRAINT receivables_void_chk CHECK (
        (voided_at IS NULL AND voided_by IS NULL AND void_reason IS NULL)
        OR (voided_at IS NOT NULL AND voided_by IS NOT NULL AND char_length(void_reason) BETWEEN 3 AND 200)
    );

CREATE UNIQUE INDEX receivables_doc_key  ON receivables (tenant_id, doc_no) WHERE doc_no IS NOT NULL;
CREATE UNIQUE INDEX receivables_idem_key ON receivables (tenant_id, idempotency_key) WHERE idempotency_key IS NOT NULL;
-- Satu nota lama tidak boleh dimasukkan dua kali untuk member yang sama (selama yang pertama tidak dibatalkan).
CREATE UNIQUE INDEX receivables_opening_ref_key ON receivables (tenant_id, member_id, lower(ref_no))
    WHERE kind = 'opening' AND ref_no <> '' AND voided_at IS NULL;

CREATE TABLE receivable_opening_counters (
    tenant_id uuid   NOT NULL,
    outlet_id uuid   NOT NULL,
    day       date   NOT NULL,                    -- hari menurut zona waktu outlet
    last_no   bigint NOT NULL,
    PRIMARY KEY (tenant_id, outlet_id, day),
    CONSTRAINT receivable_opening_counters_outlet_fk FOREIGN KEY (tenant_id, outlet_id) REFERENCES outlets (tenant_id, id) ON DELETE RESTRICT
);
ALTER TABLE receivable_opening_counters ENABLE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON receivable_opening_counters USING (tenant_id = app_tenant_id()) WITH CHECK (tenant_id = app_tenant_id());
REVOKE DELETE, TRUNCATE ON receivable_opening_counters FROM aciraba_app;

-- Piutang tetap tidak bisa diubah, kecuali penanda batal (juga memberi hak SELECT … FOR UPDATE untuk mengunci barisnya).
GRANT UPDATE (voided_at, voided_by, void_reason) ON receivables TO aciraba_app;

-- +goose Down
REVOKE UPDATE (voided_at, voided_by, void_reason) ON receivables FROM aciraba_app;
DROP TABLE receivable_opening_counters;
DELETE FROM receivable_payments WHERE receivable_id IN (SELECT id FROM receivables WHERE kind = 'opening');
DELETE FROM receivables WHERE kind = 'opening';
DROP INDEX receivables_opening_ref_key;
DROP INDEX receivables_idem_key;
DROP INDEX receivables_doc_key;
ALTER TABLE receivables
    DROP CONSTRAINT receivables_void_chk,
    DROP CONSTRAINT receivables_kind_chk,
    DROP CONSTRAINT receivables_voider_fk,
    DROP CONSTRAINT receivables_creator_fk,
    DROP COLUMN void_reason,
    DROP COLUMN voided_by,
    DROP COLUMN voided_at,
    DROP COLUMN request_hash,
    DROP COLUMN idempotency_key,
    DROP COLUMN created_by,
    DROP COLUMN note,
    DROP COLUMN doc_date,
    DROP COLUMN ref_no,
    DROP COLUMN doc_no,
    DROP COLUMN kind,
    ALTER COLUMN sale_id SET NOT NULL;
