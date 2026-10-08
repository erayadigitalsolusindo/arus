-- +goose Up

-- Kupon belanja global (menu "Kupon Belanja"): kode buatan pemilik, potongan persen atau nominal, dipakai kasir SEBELUM bayar.
-- Berlaku untuk semua outlet tenant. Kode disimpan HURUF BESAR (dinormalkan service) sehingga UNIQUE biasa cukup.
-- Tanpa hapus permanen (arsip), karena menempel di nota lama lewat sale_vouchers.
CREATE TABLE vouchers (
    id           uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id    uuid          NOT NULL REFERENCES tenants (id) ON DELETE RESTRICT,
    code         text          NOT NULL CHECK (code ~ '^[A-Z0-9][A-Z0-9_-]{2,31}$'),
    name         text          NOT NULL CHECK (char_length(name) BETWEEN 1 AND 100),
    kind         text          NOT NULL CHECK (kind IN ('percent', 'amount')),
    value        numeric(18,2) NOT NULL CHECK (value > 0),            -- persen (0–100) atau nominal rupiah
    max_discount numeric(18,2) CHECK (max_discount IS NULL OR max_discount > 0),  -- batas potongan, hanya untuk persen
    min_spend    numeric(18,2) NOT NULL DEFAULT 0 CHECK (min_spend >= 0),         -- minimal belanja (setelah potongan baris/global)
    starts_on    date,                                                -- NULL = langsung berlaku; tanggal menurut zona waktu outlet
    ends_on      date,                                                -- NULL = tanpa batas akhir; hari ini masih berlaku
    max_uses     integer CHECK (max_uses IS NULL OR max_uses > 0),    -- NULL = tak terbatas
    used_count   integer       NOT NULL DEFAULT 0 CHECK (used_count >= 0),
    active       boolean       NOT NULL DEFAULT true,
    created_at   timestamptz   NOT NULL DEFAULT now(),
    updated_at   timestamptz   NOT NULL DEFAULT now(),
    CONSTRAINT vouchers_tenant_id_id_key UNIQUE (tenant_id, id),
    CONSTRAINT vouchers_tenant_code_key UNIQUE (tenant_id, code),
    CONSTRAINT vouchers_percent_range CHECK (kind <> 'percent' OR value <= 100),
    CONSTRAINT vouchers_cap_percent_only CHECK (kind = 'percent' OR max_discount IS NULL),
    CONSTRAINT vouchers_window CHECK (starts_on IS NULL OR ends_on IS NULL OR ends_on >= starts_on),
    CONSTRAINT vouchers_quota CHECK (max_uses IS NULL OR used_count <= max_uses)
);

ALTER TABLE vouchers ENABLE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON vouchers USING (tenant_id = app_tenant_id()) WITH CHECK (tenant_id = app_tenant_id());
CREATE TRIGGER vouchers_set_updated_at BEFORE UPDATE ON vouchers FOR EACH ROW EXECUTE FUNCTION set_updated_at();
REVOKE DELETE, TRUNCATE ON vouchers FROM aciraba_app;

-- Kupon yang dipakai sebuah nota (boleh lebih dari satu). Snapshot kode/nama/jenis/nilai agar nota tetap terbaca
-- walau kupon kemudian diubah. `amount` = potongan rupiah dari kupon itu (sudah termasuk di sales.discount).
CREATE TABLE sale_vouchers (
    tenant_id  uuid          NOT NULL,
    sale_id    uuid          NOT NULL,
    voucher_id uuid          NOT NULL,
    position   int           NOT NULL,
    code       text          NOT NULL,
    name       text          NOT NULL,
    kind       text          NOT NULL CHECK (kind IN ('percent', 'amount')),
    value      numeric(18,2) NOT NULL,
    amount     numeric(18,2) NOT NULL CHECK (amount > 0),
    PRIMARY KEY (sale_id, voucher_id),
    CONSTRAINT sale_vouchers_sale_fk FOREIGN KEY (tenant_id, sale_id) REFERENCES sales (tenant_id, id) ON DELETE RESTRICT,
    CONSTRAINT sale_vouchers_voucher_fk FOREIGN KEY (tenant_id, voucher_id) REFERENCES vouchers (tenant_id, id) ON DELETE RESTRICT
);
CREATE INDEX sale_vouchers_voucher_idx ON sale_vouchers (tenant_id, voucher_id);

ALTER TABLE sale_vouchers ENABLE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON sale_vouchers USING (tenant_id = app_tenant_id()) WITH CHECK (tenant_id = app_tenant_id());
REVOKE UPDATE, DELETE, TRUNCATE ON sale_vouchers FROM aciraba_app;

-- +goose Down
DROP TABLE sale_vouchers;
DROP TABLE vouchers;
