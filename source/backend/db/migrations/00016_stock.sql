-- +goose Up

-- Stok (Fase 4.1): ledger + saldo. `stock_movements` append-only (sumber kebenaran, dibaca kartu stok);
-- `stock_balances` = saldo turunan yang diubah pada transaksi yang sama dengan movement-nya (AGENTS.md §3.4).
-- Satuan qty selalu satuan dasar item. Tiga bucket per outlet: display (dijual), warehouse (gudang), returns (retur).
-- Tidak ada CHECK qty >= 0 di saldo karena item tertentu boleh minus (items.allow_negative_stock, hanya bucket display);
-- penjaganya satu pernyataan atomik di service (internal/stock), bukan SELECT-lalu-UPDATE.

CREATE TABLE stock_balances (
    tenant_id  uuid          NOT NULL,
    outlet_id  uuid          NOT NULL,
    item_id    uuid          NOT NULL,
    bucket     text          NOT NULL CHECK (bucket IN ('display', 'warehouse', 'returns')),
    qty        numeric(18,3) NOT NULL DEFAULT 0,
    updated_at timestamptz   NOT NULL DEFAULT now(),
    PRIMARY KEY (tenant_id, outlet_id, item_id, bucket),
    CONSTRAINT stock_balances_outlet_fk FOREIGN KEY (tenant_id, outlet_id) REFERENCES outlets (tenant_id, id) ON DELETE RESTRICT,
    CONSTRAINT stock_balances_item_fk   FOREIGN KEY (tenant_id, item_id)   REFERENCES items   (tenant_id, id) ON DELETE RESTRICT
);
CREATE INDEX stock_balances_item_idx ON stock_balances (tenant_id, item_id);

CREATE TABLE stock_movements (
    id            bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    tenant_id     uuid          NOT NULL,
    outlet_id     uuid          NOT NULL,
    item_id       uuid          NOT NULL,
    bucket        text          NOT NULL CHECK (bucket IN ('display', 'warehouse', 'returns')),
    qty_delta     numeric(18,3) NOT NULL CHECK (qty_delta <> 0),
    -- Saldo bucket tepat setelah movement ini (dihitung atomik bersama pembaruan saldo; bahan kartu stok).
    balance_after numeric(18,3) NOT NULL,
    ref_type      text          NOT NULL CHECK (ref_type IN ('OPENING', 'SALE', 'SALE_VOID', 'SALE_RETURN', 'PURCHASE',
                                  'PURCHASE_RETURN', 'OPNAME', 'TRANSFER_OUT', 'TRANSFER_IN', 'UNIT_CONVERSION', 'ADJUSTMENT')),
    ref_id        uuid,
    note          text          NOT NULL DEFAULT '' CHECK (char_length(note) <= 500),
    actor_id      uuid,
    created_at    timestamptz   NOT NULL DEFAULT now(),
    CONSTRAINT stock_movements_outlet_fk FOREIGN KEY (tenant_id, outlet_id) REFERENCES outlets (tenant_id, id) ON DELETE RESTRICT,
    CONSTRAINT stock_movements_item_fk   FOREIGN KEY (tenant_id, item_id)   REFERENCES items   (tenant_id, id) ON DELETE RESTRICT
);
CREATE INDEX stock_movements_card_idx ON stock_movements (tenant_id, outlet_id, item_id, id);
CREATE INDEX stock_movements_ref_idx  ON stock_movements (tenant_id, ref_type, ref_id);

ALTER TABLE stock_balances ENABLE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON stock_balances USING (tenant_id = app_tenant_id()) WITH CHECK (tenant_id = app_tenant_id());

-- Ledger: hanya baca + tambah. Aplikasi tidak bisa mengubah/menghapus movement (koreksi = movement baru).
ALTER TABLE stock_movements ENABLE ROW LEVEL SECURITY;
CREATE POLICY tenant_read   ON stock_movements FOR SELECT USING (tenant_id = app_tenant_id());
CREATE POLICY tenant_insert ON stock_movements FOR INSERT WITH CHECK (tenant_id = app_tenant_id());
REVOKE UPDATE, DELETE, TRUNCATE ON stock_movements FROM aciraba_app;

CREATE TRIGGER stock_balances_set_updated_at BEFORE UPDATE ON stock_balances FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- +goose Down
DROP TABLE stock_movements;
DROP TABLE stock_balances;
