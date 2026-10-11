-- +goose Up

-- Kas kecil kasir: uang keluar/masuk laci di luar penjualan (beli galon, parkir, tambah receh). Terikat shift terbuka
-- kasir; ikut menjadi "arus lain" di rekap tutup shift sehingga uang laci yang seharusnya tetap cocok.
-- Pembatalan = baris voided_at (append-only; tidak ada DELETE), hanya selama shift masih terbuka.
CREATE TABLE cash_movements (
    id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id       uuid          NOT NULL,
    outlet_id       uuid          NOT NULL,
    shift_id        uuid          NOT NULL,
    method_id       uuid          NOT NULL,             -- metode tunai sistem
    direction       text          NOT NULL CHECK (direction IN ('in', 'out')),
    amount          numeric(18,2) NOT NULL CHECK (amount > 0 AND amount < 1e15),
    category        text          NOT NULL CHECK (char_length(category) BETWEEN 1 AND 40),
    note            text          NOT NULL DEFAULT '' CHECK (char_length(note) <= 300),
    created_by      uuid          NOT NULL,
    approved_by     uuid,
    approved_name   text,
    idem_key        text          NOT NULL CHECK (char_length(idem_key) BETWEEN 8 AND 100),
    created_at      timestamptz   NOT NULL DEFAULT clock_timestamp(),
    voided_at       timestamptz,
    voided_by       uuid,
    CONSTRAINT cash_movements_tenant_id_id_key UNIQUE (tenant_id, id),
    CONSTRAINT cash_movements_idem_key UNIQUE (tenant_id, idem_key),
    CONSTRAINT cash_movements_shift_fk  FOREIGN KEY (tenant_id, shift_id)  REFERENCES cash_shifts (tenant_id, id) ON DELETE RESTRICT,
    CONSTRAINT cash_movements_outlet_fk FOREIGN KEY (tenant_id, outlet_id) REFERENCES outlets (tenant_id, id) ON DELETE RESTRICT,
    CONSTRAINT cash_movements_method_fk FOREIGN KEY (tenant_id, method_id) REFERENCES payment_methods (tenant_id, id) ON DELETE RESTRICT,
    CONSTRAINT cash_movements_user_fk   FOREIGN KEY (tenant_id, created_by) REFERENCES users (tenant_id, id) ON DELETE RESTRICT,
    CONSTRAINT cash_movements_appr_fk   FOREIGN KEY (tenant_id, approved_by) REFERENCES users (tenant_id, id) ON DELETE RESTRICT,
    CONSTRAINT cash_movements_void_chk CHECK ((voided_at IS NULL) = (voided_by IS NULL))
);
-- Rekap shift: per kasir + outlet + rentang waktu; daftar per shift.
CREATE INDEX cash_movements_recap_idx ON cash_movements (tenant_id, outlet_id, created_by, created_at);
CREATE INDEX cash_movements_shift_idx ON cash_movements (tenant_id, shift_id, created_at, id);

ALTER TABLE cash_movements ENABLE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON cash_movements USING (tenant_id = app_tenant_id()) WITH CHECK (tenant_id = app_tenant_id());
REVOKE DELETE, TRUNCATE ON cash_movements FROM aciraba_app;
REVOKE UPDATE ON cash_movements FROM aciraba_app;
GRANT UPDATE (voided_at, voided_by) ON cash_movements TO aciraba_app;

-- Batas nominal kas keluar yang butuh PIN penyetuju (0 = semua kas keluar butuh PIN).
ALTER TABLE outlets ADD COLUMN petty_cash_approval_limit numeric(18,2) NOT NULL DEFAULT 100000
    CHECK (petty_cash_approval_limit >= 0 AND petty_cash_approval_limit < 1e15);

-- +goose Down
ALTER TABLE outlets DROP COLUMN petty_cash_approval_limit;
DROP TABLE cash_movements;
