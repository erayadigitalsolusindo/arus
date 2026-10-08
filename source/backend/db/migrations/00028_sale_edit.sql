-- +goose Up

-- Edit & batal nota (Fase 5.3). Nota tidak pernah ditimpa: edit = nota LAMA berstatus 'superseded' + nota BARU (revisi) yang
-- tertaut; batal = status 'void' + alasan. Dampak (stok, poin, kupon) dibalik oleh service di transaksi yang sama.
ALTER TABLE sales DROP CONSTRAINT sales_status_check;
ALTER TABLE sales ADD CONSTRAINT sales_status_check CHECK (status IN ('completed', 'void', 'superseded'));

ALTER TABLE sales
    ADD COLUMN root_id         uuid,                                        -- nota asli rantai revisi (NULL = nota ini sendiri yang asli)
    ADD COLUMN revision        int  NOT NULL DEFAULT 1 CHECK (revision >= 1),
    ADD COLUMN supersedes_id   uuid,                                        -- revisi ini menggantikan nota ...
    ADD COLUMN superseded_by   uuid,                                        -- nota ini sudah digantikan oleh revisi ...
    ADD COLUMN revision_reason text CHECK (revision_reason IS NULL OR char_length(revision_reason) BETWEEN 3 AND 200),
    ADD COLUMN revised_at      timestamptz,
    ADD COLUMN revised_by      uuid,
    ADD COLUMN void_reason     text CHECK (void_reason IS NULL OR char_length(void_reason) BETWEEN 3 AND 200),
    ADD COLUMN voided_at       timestamptz,
    ADD COLUMN voided_by       uuid;

ALTER TABLE sales
    ADD CONSTRAINT sales_supersedes_fk    FOREIGN KEY (tenant_id, supersedes_id) REFERENCES sales (tenant_id, id) ON DELETE RESTRICT,
    ADD CONSTRAINT sales_superseded_by_fk FOREIGN KEY (tenant_id, superseded_by) REFERENCES sales (tenant_id, id) ON DELETE RESTRICT,
    ADD CONSTRAINT sales_root_fk          FOREIGN KEY (tenant_id, root_id)       REFERENCES sales (tenant_id, id) ON DELETE RESTRICT,
    ADD CONSTRAINT sales_revised_by_fk    FOREIGN KEY (tenant_id, revised_by)    REFERENCES users (tenant_id, id) ON DELETE RESTRICT,
    ADD CONSTRAINT sales_voided_by_fk     FOREIGN KEY (tenant_id, voided_by)     REFERENCES users (tenant_id, id) ON DELETE RESTRICT,
    ADD CONSTRAINT sales_void_chk         CHECK ((status = 'void') = (void_reason IS NOT NULL)),
    ADD CONSTRAINT sales_superseded_chk   CHECK ((status = 'superseded') = (superseded_by IS NOT NULL)),
    ADD CONSTRAINT sales_revision_chk     CHECK ((revision = 1) = (supersedes_id IS NULL));

CREATE INDEX sales_root_idx ON sales (tenant_id, coalesce(root_id, id));

-- Batas waktu edit/batal nota, DIATUR OPERATOR PLATFORM per tenant (bukan pemilik tenant): 0 = hanya di hari nota dibuat
-- (zona waktu outlet), N = sampai N hari sesudahnya, 3650 = praktis tanpa batas.
ALTER TABLE tenants ADD COLUMN sale_edit_window_days int NOT NULL DEFAULT 0 CHECK (sale_edit_window_days BETWEEN 0 AND 3650);

-- +goose Down
ALTER TABLE tenants DROP COLUMN sale_edit_window_days;
DROP INDEX sales_root_idx;
ALTER TABLE sales
    DROP CONSTRAINT sales_revision_chk, DROP CONSTRAINT sales_superseded_chk, DROP CONSTRAINT sales_void_chk,
    DROP CONSTRAINT sales_voided_by_fk, DROP CONSTRAINT sales_revised_by_fk, DROP CONSTRAINT sales_root_fk,
    DROP CONSTRAINT sales_superseded_by_fk, DROP CONSTRAINT sales_supersedes_fk;
ALTER TABLE sales
    DROP COLUMN voided_by, DROP COLUMN voided_at, DROP COLUMN void_reason, DROP COLUMN revised_by, DROP COLUMN revised_at,
    DROP COLUMN revision_reason, DROP COLUMN superseded_by, DROP COLUMN supersedes_id, DROP COLUMN revision, DROP COLUMN root_id;
ALTER TABLE sales DROP CONSTRAINT sales_status_check;
ALTER TABLE sales ADD CONSTRAINT sales_status_check CHECK (status IN ('completed', 'void'));
