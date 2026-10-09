-- +goose Up

-- HPP per cabang (Fase 6.1; keputusan 2026-10-09). `items.avg_cost`/`last_cost` kini hanya HPP AWAL (diisi saat barang
-- dibuat) dan dipakai sebagai nilai bawaan bagi cabang yang belum punya baris di sini — sama seperti harga jual
-- (`item_outlet_prices`). Baris dibuat/diubah oleh transaksi yang menentukan HPP (pembelian, pecah satuan), di dalam
-- transaksi yang sama dengan movement stoknya.
CREATE TABLE item_outlet_costs (
    tenant_id  uuid          NOT NULL,
    outlet_id  uuid          NOT NULL,
    item_id    uuid          NOT NULL,
    avg_cost   numeric(18,2) NOT NULL DEFAULT 0 CHECK (avg_cost >= 0),
    last_cost  numeric(18,2) NOT NULL DEFAULT 0 CHECK (last_cost >= 0),
    updated_at timestamptz   NOT NULL DEFAULT now(),
    PRIMARY KEY (tenant_id, outlet_id, item_id),
    CONSTRAINT item_outlet_costs_outlet_fk FOREIGN KEY (tenant_id, outlet_id) REFERENCES outlets (tenant_id, id) ON DELETE RESTRICT,
    CONSTRAINT item_outlet_costs_item_fk   FOREIGN KEY (tenant_id, item_id)   REFERENCES items   (tenant_id, id) ON DELETE RESTRICT
);
CREATE INDEX item_outlet_costs_item_idx ON item_outlet_costs (tenant_id, item_id);

ALTER TABLE item_outlet_costs ENABLE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON item_outlet_costs USING (tenant_id = app_tenant_id()) WITH CHECK (tenant_id = app_tenant_id());

CREATE TRIGGER item_outlet_costs_set_updated_at BEFORE UPDATE ON item_outlet_costs FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- +goose Down
DROP TABLE item_outlet_costs;
