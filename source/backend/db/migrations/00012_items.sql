-- +goose Up

-- Daftar item (Fase 3.2, irisan B): master barang per tenant + harga jual per cabang.
-- Harga jual `items.sell_price` = harga default tenant; `item_outlet_prices` = harga khusus cabang. Cabang tanpa
-- baris sendiri memakai harga default (menambah cabang baru tidak perlu mengisi ulang harga).
-- HPP: `last_cost`/`avg_cost` diisi HPP awal saat barang dibuat dan hanya diubah oleh transaksi pembelian/stok
-- (Fase 4/6), bukan oleh form item. HPP per cabang menyusul bersama stok per cabang.
-- Tidak ada hapus permanen: item dinonaktifkan (active = false) karena transaksi akan mereferensikannya.

CREATE TABLE items (
    id                   uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id            uuid          NOT NULL REFERENCES tenants (id) ON DELETE RESTRICT,
    sku                  text          NOT NULL CHECK (char_length(sku) BETWEEN 1 AND 40),
    -- Nilai yang dibaca pemindai (barcode/QR). Opsional; bila terisi unik per tenant.
    barcode              text          CHECK (barcode IS NULL OR char_length(barcode) BETWEEN 1 AND 200),
    name                 text          NOT NULL CHECK (char_length(name) BETWEEN 1 AND 200),
    weight_grams         numeric(12,3) NOT NULL DEFAULT 0 CHECK (weight_grams >= 0),
    last_cost            numeric(18,2) NOT NULL DEFAULT 0 CHECK (last_cost >= 0),
    avg_cost             numeric(18,2) NOT NULL DEFAULT 0 CHECK (avg_cost >= 0),
    sell_price           numeric(18,2) NOT NULL DEFAULT 0 CHECK (sell_price >= 0),
    unit_id              uuid          NOT NULL,
    category_id          uuid,
    brand_id             uuid,
    principal_id         uuid,
    supplier_id          uuid,
    kind                 text          NOT NULL DEFAULT 'goods' CHECK (kind IN ('goods', 'service')),
    allow_negative_stock boolean       NOT NULL DEFAULT false,
    sell_below_cost      boolean       NOT NULL DEFAULT false,
    description          text          NOT NULL DEFAULT '' CHECK (char_length(description) <= 5000),
    active               boolean       NOT NULL DEFAULT true,
    created_at           timestamptz   NOT NULL DEFAULT now(),
    updated_at           timestamptz   NOT NULL DEFAULT now(),
    CONSTRAINT items_tenant_id_id_key UNIQUE (tenant_id, id),
    -- FK komposit: master yang dirujuk wajib milik tenant yang sama. Kolom opsional kosong (NULL) tidak diperiksa.
    CONSTRAINT items_unit_fk      FOREIGN KEY (tenant_id, unit_id)      REFERENCES units      (tenant_id, id) ON DELETE RESTRICT,
    CONSTRAINT items_category_fk  FOREIGN KEY (tenant_id, category_id)  REFERENCES categories (tenant_id, id) ON DELETE RESTRICT,
    CONSTRAINT items_brand_fk     FOREIGN KEY (tenant_id, brand_id)     REFERENCES brands     (tenant_id, id) ON DELETE RESTRICT,
    CONSTRAINT items_principal_fk FOREIGN KEY (tenant_id, principal_id) REFERENCES principals (tenant_id, id) ON DELETE RESTRICT,
    CONSTRAINT items_supplier_fk  FOREIGN KEY (tenant_id, supplier_id)  REFERENCES suppliers  (tenant_id, id) ON DELETE RESTRICT
);
CREATE UNIQUE INDEX items_tenant_sku_key     ON items (tenant_id, lower(sku));
CREATE UNIQUE INDEX items_tenant_barcode_key ON items (tenant_id, barcode) WHERE barcode IS NOT NULL;
CREATE INDEX items_tenant_name_idx     ON items (tenant_id, lower(name));
CREATE INDEX items_tenant_category_idx ON items (tenant_id, category_id);

CREATE TABLE item_outlet_prices (
    tenant_id  uuid          NOT NULL,
    item_id    uuid          NOT NULL,
    outlet_id  uuid          NOT NULL,
    sell_price numeric(18,2) NOT NULL CHECK (sell_price >= 0),
    updated_at timestamptz   NOT NULL DEFAULT now(),
    PRIMARY KEY (item_id, outlet_id),
    CONSTRAINT item_outlet_prices_item_fk   FOREIGN KEY (tenant_id, item_id)   REFERENCES items   (tenant_id, id) ON DELETE CASCADE,
    CONSTRAINT item_outlet_prices_outlet_fk FOREIGN KEY (tenant_id, outlet_id) REFERENCES outlets (tenant_id, id) ON DELETE CASCADE
);
CREATE INDEX item_outlet_prices_outlet_idx ON item_outlet_prices (tenant_id, outlet_id);

-- Penomoran kode barang otomatis per tenant (ITM-000001, …). Atomik: UPDATE ... RETURNING mengunci barisnya.
CREATE TABLE item_counters (
    tenant_id uuid   PRIMARY KEY REFERENCES tenants (id) ON DELETE RESTRICT,
    last_no   bigint NOT NULL DEFAULT 0
);

-- +goose StatementBegin
DO $$
DECLARE t text;
BEGIN
    FOREACH t IN ARRAY ARRAY['items', 'item_outlet_prices', 'item_counters'] LOOP
        EXECUTE format('ALTER TABLE %I ENABLE ROW LEVEL SECURITY', t);
        EXECUTE format('CREATE POLICY tenant_isolation ON %I USING (tenant_id = app_tenant_id()) WITH CHECK (tenant_id = app_tenant_id())', t);
    END LOOP;
    EXECUTE 'CREATE TRIGGER items_set_updated_at BEFORE UPDATE ON items FOR EACH ROW EXECUTE FUNCTION set_updated_at()';
    EXECUTE 'CREATE TRIGGER item_outlet_prices_set_updated_at BEFORE UPDATE ON item_outlet_prices FOR EACH ROW EXECUTE FUNCTION set_updated_at()';
END
$$;
-- +goose StatementEnd

-- +goose Down
DROP TABLE item_counters;
DROP TABLE item_outlet_prices;
DROP TABLE items;
