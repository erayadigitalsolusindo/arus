-- +goose Up

-- Salesman (FR-MD-06, FR-POS-17): master label per tenant, BUKAN akun login. Dipilih kasir per nota untuk laporan/komisi.
-- Tanpa hapus permanen (arsip), karena menempel di nota lama. commission_pct hanya disimpan; belum ada hitung komisi.
CREATE TABLE salespeople (
    id             uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id      uuid         NOT NULL REFERENCES tenants (id) ON DELETE RESTRICT,
    code           text         CHECK (code IS NULL OR char_length(code) BETWEEN 1 AND 30),
    name           text         NOT NULL CHECK (char_length(name) BETWEEN 1 AND 100),
    phone          text         NOT NULL DEFAULT '' CHECK (char_length(phone) <= 20),
    note           text         NOT NULL DEFAULT '' CHECK (char_length(note) <= 500),
    commission_pct numeric(5,2) NOT NULL DEFAULT 0 CHECK (commission_pct >= 0 AND commission_pct <= 100),
    active         boolean      NOT NULL DEFAULT true,
    created_at     timestamptz  NOT NULL DEFAULT now(),
    updated_at     timestamptz  NOT NULL DEFAULT now(),
    CONSTRAINT salespeople_tenant_id_id_key UNIQUE (tenant_id, id)
);
CREATE UNIQUE INDEX salespeople_tenant_name_key ON salespeople (tenant_id, lower(name));
CREATE UNIQUE INDEX salespeople_tenant_code_key ON salespeople (tenant_id, lower(code)) WHERE code IS NOT NULL;

ALTER TABLE salespeople ENABLE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON salespeople USING (tenant_id = app_tenant_id()) WITH CHECK (tenant_id = app_tenant_id());
CREATE TRIGGER salespeople_set_updated_at BEFORE UPDATE ON salespeople FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- Nota menyimpan salesman (nullable = Umum). FK komposit agar tak menunjuk tenant lain; RESTRICT karena salesman diarsipkan, bukan dihapus.
ALTER TABLE sales ADD COLUMN salesperson_id uuid;
ALTER TABLE sales ADD CONSTRAINT sales_salesperson_fk FOREIGN KEY (tenant_id, salesperson_id)
    REFERENCES salespeople (tenant_id, id) ON DELETE RESTRICT;

-- +goose Down
ALTER TABLE sales DROP CONSTRAINT sales_salesperson_fk;
ALTER TABLE sales DROP COLUMN salesperson_id;
DROP TABLE salespeople;
