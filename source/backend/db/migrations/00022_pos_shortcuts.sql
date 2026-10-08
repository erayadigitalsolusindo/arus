-- +goose Up

-- Slot pintasan barang di layar kasir (16 slot per kasir). Preferensi pribadi, bukan transaksi: boleh diubah/dihapus bebas.
-- Barang tidak pernah dihapus permanen (diarsipkan), jadi FK ke items cukup RESTRICT.
CREATE TABLE pos_shortcuts (
    tenant_id  uuid        NOT NULL,
    user_id    uuid        NOT NULL,
    slot       smallint    NOT NULL CHECK (slot BETWEEN 1 AND 16),
    item_id    uuid        NOT NULL,
    updated_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (tenant_id, user_id, slot),
    CONSTRAINT pos_shortcuts_user_fk FOREIGN KEY (tenant_id, user_id) REFERENCES users (tenant_id, id) ON DELETE CASCADE,
    CONSTRAINT pos_shortcuts_item_fk FOREIGN KEY (tenant_id, item_id) REFERENCES items (tenant_id, id) ON DELETE RESTRICT
);

ALTER TABLE pos_shortcuts ENABLE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON pos_shortcuts USING (tenant_id = app_tenant_id()) WITH CHECK (tenant_id = app_tenant_id());

-- +goose Down
DROP TABLE pos_shortcuts;
