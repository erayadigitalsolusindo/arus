-- +goose Up

-- Edit & batal pembelian (Fase 6.7). Nota TIDAK pernah ditimpa:
--   edit  = nota lama jadi 'superseded' (superseded_by menunjuk revisi) + nota revisi baru tertaut (supersedes_id);
--   batal = 'void' + alasan. Dampak stok & HPP dibalik lewat movement PURCHASE_VOID (aturan ada di service Go).
-- Batas hari edit/batal memakai tenants.sale_edit_window_days (diatur operator platform), sama seperti penjualan.

ALTER TABLE purchases DROP CONSTRAINT purchases_status_check;
ALTER TABLE purchases ADD CONSTRAINT purchases_status_check CHECK (status IN ('completed', 'superseded', 'void'));

ALTER TABLE purchases
    ADD COLUMN root_id         uuid,                       -- NULL = nota asli (akar rantai revisi)
    ADD COLUMN revision        integer NOT NULL DEFAULT 1 CHECK (revision >= 1),
    ADD COLUMN supersedes_id   uuid,                       -- nota yang digantikan nota ini
    ADD COLUMN superseded_by   uuid,                       -- revisi yang menggantikan nota ini
    ADD COLUMN revision_reason text        NOT NULL DEFAULT '' CHECK (char_length(revision_reason) <= 200),
    ADD COLUMN revised_at      timestamptz,
    ADD COLUMN revised_by      uuid,
    ADD COLUMN void_reason     text        NOT NULL DEFAULT '' CHECK (char_length(void_reason) <= 200),
    ADD COLUMN voided_at       timestamptz,
    ADD COLUMN voided_by       uuid,
    -- superseded_by DEFERRABLE: nota lama ditandai SEBELUM revisi (yang menjadi tujuannya) di-insert, agar indeks
    -- unik nomor faktur (hanya status 'completed') tidak bentrok. Diperiksa saat commit.
    ADD CONSTRAINT purchases_root_fk       FOREIGN KEY (tenant_id, root_id)       REFERENCES purchases (tenant_id, id),
    ADD CONSTRAINT purchases_supersedes_fk FOREIGN KEY (tenant_id, supersedes_id) REFERENCES purchases (tenant_id, id),
    ADD CONSTRAINT purchases_superseded_by_fk FOREIGN KEY (tenant_id, superseded_by) REFERENCES purchases (tenant_id, id)
        DEFERRABLE INITIALLY DEFERRED,
    ADD CONSTRAINT purchases_revised_by_fk FOREIGN KEY (tenant_id, revised_by) REFERENCES users (tenant_id, id),
    ADD CONSTRAINT purchases_voided_by_fk  FOREIGN KEY (tenant_id, voided_by)  REFERENCES users (tenant_id, id),
    ADD CONSTRAINT purchases_state_chk CHECK (
        (status = 'completed'  AND superseded_by IS NULL AND voided_at IS NULL)
     OR (status = 'superseded' AND superseded_by IS NOT NULL AND revised_at IS NOT NULL AND revised_by IS NOT NULL)
     OR (status = 'void'       AND voided_at IS NOT NULL AND voided_by IS NOT NULL AND char_length(void_reason) >= 3)
    );

-- Hutang nota yang dibatalkan/digantikan ditandai (jumlahnya tetap; hanya penanda waktu). Hak UPDATE hanya pada kolom ini.
ALTER TABLE payables ADD COLUMN voided_at timestamptz;
GRANT UPDATE (voided_at) ON payables TO aciraba_app;

-- Jenis referensi movement baru untuk pembalikan pembelian.
ALTER TABLE stock_movements DROP CONSTRAINT stock_movements_ref_type_check;
ALTER TABLE stock_movements ADD CONSTRAINT stock_movements_ref_type_check CHECK (ref_type IN (
    'OPENING', 'SALE', 'SALE_VOID', 'SALE_RETURN', 'PURCHASE', 'PURCHASE_RETURN', 'PURCHASE_VOID',
    'OPNAME', 'TRANSFER_OUT', 'TRANSFER_IN', 'UNIT_CONVERSION', 'ADJUSTMENT'));

-- +goose Down

ALTER TABLE stock_movements DROP CONSTRAINT stock_movements_ref_type_check;
ALTER TABLE stock_movements ADD CONSTRAINT stock_movements_ref_type_check CHECK (ref_type IN (
    'OPENING', 'SALE', 'SALE_VOID', 'SALE_RETURN', 'PURCHASE', 'PURCHASE_RETURN',
    'OPNAME', 'TRANSFER_OUT', 'TRANSFER_IN', 'UNIT_CONVERSION', 'ADJUSTMENT'));

REVOKE UPDATE (voided_at) ON payables FROM aciraba_app;
ALTER TABLE payables DROP COLUMN voided_at;

ALTER TABLE purchases
    DROP CONSTRAINT purchases_state_chk,
    DROP CONSTRAINT purchases_voided_by_fk,
    DROP CONSTRAINT purchases_revised_by_fk,
    DROP CONSTRAINT purchases_superseded_by_fk,
    DROP CONSTRAINT purchases_supersedes_fk,
    DROP CONSTRAINT purchases_root_fk,
    DROP COLUMN voided_by, DROP COLUMN voided_at, DROP COLUMN void_reason,
    DROP COLUMN revised_by, DROP COLUMN revised_at, DROP COLUMN revision_reason,
    DROP COLUMN superseded_by, DROP COLUMN supersedes_id, DROP COLUMN revision, DROP COLUMN root_id;
UPDATE purchases SET status = 'completed' WHERE status <> 'completed';
ALTER TABLE purchases DROP CONSTRAINT purchases_status_check;
ALTER TABLE purchases ADD CONSTRAINT purchases_status_check CHECK (status IN ('completed'));
