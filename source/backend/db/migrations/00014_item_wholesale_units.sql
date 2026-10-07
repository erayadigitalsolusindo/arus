-- +goose Up

-- Harga grosir dan konversi satuan item (Fase 3.2, irisan D).
--
-- item_wholesale_tiers: tangga harga berdasarkan jumlah. Satu baris = "mulai min_qty (dalam satuan dasar item),
-- harga per satuan = price". Harga tier berlaku untuk SELURUH jumlah (bukan potongan bertingkat); batas atas
-- tier diturunkan dari tier berikutnya, sehingga tidak ada celah/tumpang-tindih. Tier terakhir berlaku sampai
-- stok habis. outlet_id NULL = set default semua cabang; cabang yang punya set sendiri memakai set itu SEPENUHNYA
-- (tidak digabung dengan default). Jumlah di bawah tier pertama memakai harga jual biasa.
--
-- item_units: satuan tambahan item. factor = berapa satuan DASAR (items.unit_id) dalam satu satuan ini
-- (mis. Dus: factor 12 bila satuan dasar Pcs). Barcode dan harga per satuan opsional; harga NULL = factor × harga
-- satuan dasar saat dijual. Barcode harus unik per tenant di items DAN item_units — lintas tabel dijaga service
-- (advisory lock) dan indeks unik per tabel sebagai pengaman.

CREATE TABLE item_wholesale_tiers (
    id        uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id uuid          NOT NULL,
    item_id   uuid          NOT NULL,
    outlet_id uuid,
    min_qty   numeric(18,3) NOT NULL CHECK (min_qty > 0),
    price     numeric(18,2) NOT NULL CHECK (price >= 0),
    CONSTRAINT item_wholesale_tiers_item_fk   FOREIGN KEY (tenant_id, item_id)   REFERENCES items   (tenant_id, id) ON DELETE CASCADE,
    CONSTRAINT item_wholesale_tiers_outlet_fk FOREIGN KEY (tenant_id, outlet_id) REFERENCES outlets (tenant_id, id) ON DELETE CASCADE
);
CREATE UNIQUE INDEX item_wholesale_tiers_key
    ON item_wholesale_tiers (item_id, coalesce(outlet_id, '00000000-0000-0000-0000-000000000000'::uuid), min_qty);
CREATE INDEX item_wholesale_tiers_item_idx ON item_wholesale_tiers (tenant_id, item_id);

CREATE TABLE item_units (
    id         uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id  uuid          NOT NULL,
    item_id    uuid          NOT NULL,
    unit_id    uuid          NOT NULL,
    factor     numeric(18,6) NOT NULL CHECK (factor > 0),
    barcode    text          CHECK (barcode IS NULL OR char_length(barcode) BETWEEN 1 AND 200),
    sell_price numeric(18,2) CHECK (sell_price IS NULL OR sell_price >= 0),
    position   integer       NOT NULL CHECK (position >= 1),
    CONSTRAINT item_units_item_fk FOREIGN KEY (tenant_id, item_id) REFERENCES items (tenant_id, id) ON DELETE CASCADE,
    CONSTRAINT item_units_unit_fk FOREIGN KEY (tenant_id, unit_id) REFERENCES units (tenant_id, id) ON DELETE RESTRICT,
    CONSTRAINT item_units_item_unit_key UNIQUE (item_id, unit_id)
);
CREATE UNIQUE INDEX item_units_tenant_barcode_key ON item_units (tenant_id, barcode) WHERE barcode IS NOT NULL;
CREATE INDEX item_units_item_idx ON item_units (tenant_id, item_id, position);

-- +goose StatementBegin
DO $$
DECLARE t text;
BEGIN
    FOREACH t IN ARRAY ARRAY['item_wholesale_tiers', 'item_units'] LOOP
        EXECUTE format('ALTER TABLE %I ENABLE ROW LEVEL SECURITY', t);
        EXECUTE format('CREATE POLICY tenant_isolation ON %I USING (tenant_id = app_tenant_id()) WITH CHECK (tenant_id = app_tenant_id())', t);
    END LOOP;
END
$$;
-- +goose StatementEnd

-- +goose Down
DROP TABLE item_units;
DROP TABLE item_wholesale_tiers;
