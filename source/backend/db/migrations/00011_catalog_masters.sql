-- +goose Up

-- Master pendukung katalog (Fase 3.1): satuan, kategori, brand, principal, supplier.
-- Tidak ada hapus permanen: master dinonaktifkan (active = false) karena item/transaksi akan mereferensikannya.
-- Nama unik per tenant tanpa membedakan huruf besar/kecil; dijaga DB (bukan SELECT-lalu-INSERT).
-- FK komposit (tenant_id, id) agar baris anak tidak bisa menunjuk master milik tenant lain.

CREATE TABLE units (
    id         uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id  uuid        NOT NULL REFERENCES tenants (id) ON DELETE RESTRICT,
    name       text        NOT NULL CHECK (char_length(name) BETWEEN 1 AND 100),
    active     boolean     NOT NULL DEFAULT true,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT units_tenant_id_id_key UNIQUE (tenant_id, id)
);
CREATE UNIQUE INDEX units_tenant_name_key ON units (tenant_id, lower(name));

CREATE TABLE categories (
    id         uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id  uuid        NOT NULL REFERENCES tenants (id) ON DELETE RESTRICT,
    name       text        NOT NULL CHECK (char_length(name) BETWEEN 1 AND 100),
    active     boolean     NOT NULL DEFAULT true,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT categories_tenant_id_id_key UNIQUE (tenant_id, id)
);
CREATE UNIQUE INDEX categories_tenant_name_key ON categories (tenant_id, lower(name));

CREATE TABLE brands (
    id         uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id  uuid        NOT NULL REFERENCES tenants (id) ON DELETE RESTRICT,
    name       text        NOT NULL CHECK (char_length(name) BETWEEN 1 AND 100),
    active     boolean     NOT NULL DEFAULT true,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT brands_tenant_id_id_key UNIQUE (tenant_id, id)
);
CREATE UNIQUE INDEX brands_tenant_name_key ON brands (tenant_id, lower(name));

CREATE TABLE principals (
    id         uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id  uuid        NOT NULL REFERENCES tenants (id) ON DELETE RESTRICT,
    name       text        NOT NULL CHECK (char_length(name) BETWEEN 1 AND 100),
    active     boolean     NOT NULL DEFAULT true,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT principals_tenant_id_id_key UNIQUE (tenant_id, id)
);
CREATE UNIQUE INDEX principals_tenant_name_key ON principals (tenant_id, lower(name));

CREATE TABLE suppliers (
    id           uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id    uuid        NOT NULL REFERENCES tenants (id) ON DELETE RESTRICT,
    code         text        CHECK (code IS NULL OR char_length(code) BETWEEN 1 AND 30),
    name         text        NOT NULL CHECK (char_length(name) BETWEEN 1 AND 100),
    contact_name text        NOT NULL DEFAULT '' CHECK (char_length(contact_name) <= 100),
    phone        text        NOT NULL DEFAULT '' CHECK (char_length(phone) <= 20),
    email        text        NOT NULL DEFAULT '' CHECK (char_length(email) <= 254),
    address      text        NOT NULL DEFAULT '' CHECK (char_length(address) <= 500),
    note         text        NOT NULL DEFAULT '' CHECK (char_length(note) <= 500),
    active       boolean     NOT NULL DEFAULT true,
    created_at   timestamptz NOT NULL DEFAULT now(),
    updated_at   timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT suppliers_tenant_id_id_key UNIQUE (tenant_id, id)
);
CREATE UNIQUE INDEX suppliers_tenant_name_key ON suppliers (tenant_id, lower(name));
-- Kode opsional; bila terisi, unik per tenant.
CREATE UNIQUE INDEX suppliers_tenant_code_key ON suppliers (tenant_id, lower(code)) WHERE code IS NOT NULL;

-- RLS + trigger updated_at untuk kelimanya (aturan tabel bertenant, lihat 00004_rls.sql).
-- +goose StatementBegin
DO $$
DECLARE t text;
BEGIN
    FOREACH t IN ARRAY ARRAY['units', 'categories', 'brands', 'principals', 'suppliers'] LOOP
        EXECUTE format('ALTER TABLE %I ENABLE ROW LEVEL SECURITY', t);
        EXECUTE format('CREATE POLICY tenant_isolation ON %I USING (tenant_id = app_tenant_id()) WITH CHECK (tenant_id = app_tenant_id())', t);
        EXECUTE format('CREATE TRIGGER %I BEFORE UPDATE ON %I FOR EACH ROW EXECUTE FUNCTION set_updated_at()', t || '_set_updated_at', t);
    END LOOP;
END
$$;
-- +goose StatementEnd

-- +goose Down
DROP TABLE suppliers;
DROP TABLE principals;
DROP TABLE brands;
DROP TABLE categories;
DROP TABLE units;
