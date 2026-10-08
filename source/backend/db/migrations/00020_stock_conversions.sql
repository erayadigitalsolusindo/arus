-- +goose Up

-- Pecah satuan (Fase 4.3): memindahkan stok dari satu barang ke barang lain di outlet yang sama (mis. 1 Karung → 12 PCS,
-- keduanya barang terpisah). Satu dokumen = dua movement UNIT_CONVERSION (keluar dari asal, masuk ke tujuan) pada bucket
-- display, ditulis di transaksi yang sama dengan dokumen. Dokumen tidak diubah/dihapus; koreksi = dokumen kebalikannya.
-- HPP: dokumen menyimpan HPP asal per satuan dasar dan HPP hasil (asal × qty asal ÷ qty tujuan). HPP hasil hanya
-- dipasang ke barang tujuan bila HPP-nya kosong atau stok totalnya ≤ 0 (rata-rata tertimbang penuh menyusul Fase 6).

CREATE TABLE stock_conversion_counters (
    tenant_id uuid   NOT NULL,
    outlet_id uuid   NOT NULL,
    day       date   NOT NULL,                 -- hari menurut zona waktu outlet
    last_no   bigint NOT NULL,
    PRIMARY KEY (tenant_id, outlet_id, day),
    CONSTRAINT stock_conversion_counters_outlet_fk FOREIGN KEY (tenant_id, outlet_id) REFERENCES outlets (tenant_id, id) ON DELETE RESTRICT
);

CREATE TABLE stock_conversions (
    id              uuid PRIMARY KEY,
    tenant_id       uuid          NOT NULL,
    outlet_id       uuid          NOT NULL,
    doc_no          text          NOT NULL CHECK (char_length(doc_no) BETWEEN 1 AND 64),
    idempotency_key text          NOT NULL CHECK (char_length(idempotency_key) BETWEEN 8 AND 100),
    request_hash    text          NOT NULL,
    from_item_id    uuid          NOT NULL,
    from_qty        numeric(18,3) NOT NULL CHECK (from_qty > 0),   -- satuan dasar barang asal
    to_item_id      uuid          NOT NULL,
    to_qty          numeric(18,3) NOT NULL CHECK (to_qty > 0),     -- satuan dasar barang tujuan
    from_unit_cost  numeric(18,2) NOT NULL CHECK (from_unit_cost >= 0),
    to_unit_cost    numeric(18,2) NOT NULL CHECK (to_unit_cost >= 0),
    cost_applied    boolean       NOT NULL,                        -- HPP hasil dipasang ke barang tujuan
    note            text          NOT NULL DEFAULT '' CHECK (char_length(note) <= 200),
    actor_id        uuid,
    created_at      timestamptz   NOT NULL DEFAULT now(),
    CONSTRAINT stock_conversions_doc_no_key UNIQUE (tenant_id, doc_no),
    CONSTRAINT stock_conversions_idem_key UNIQUE (tenant_id, idempotency_key),
    CONSTRAINT stock_conversions_tenant_id_id_key UNIQUE (tenant_id, id),
    CONSTRAINT stock_conversions_diff_chk CHECK (from_item_id <> to_item_id),
    CONSTRAINT stock_conversions_outlet_fk FOREIGN KEY (tenant_id, outlet_id)    REFERENCES outlets (tenant_id, id) ON DELETE RESTRICT,
    CONSTRAINT stock_conversions_from_fk   FOREIGN KEY (tenant_id, from_item_id) REFERENCES items   (tenant_id, id) ON DELETE RESTRICT,
    CONSTRAINT stock_conversions_to_fk     FOREIGN KEY (tenant_id, to_item_id)   REFERENCES items   (tenant_id, id) ON DELETE RESTRICT,
    CONSTRAINT stock_conversions_actor_fk  FOREIGN KEY (tenant_id, actor_id)     REFERENCES users   (tenant_id, id) ON DELETE RESTRICT
);
CREATE INDEX stock_conversions_outlet_time_idx ON stock_conversions (tenant_id, outlet_id, created_at DESC);

ALTER TABLE stock_conversion_counters ENABLE ROW LEVEL SECURITY;
ALTER TABLE stock_conversions         ENABLE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON stock_conversion_counters USING (tenant_id = app_tenant_id()) WITH CHECK (tenant_id = app_tenant_id());
CREATE POLICY tenant_isolation ON stock_conversions         USING (tenant_id = app_tenant_id()) WITH CHECK (tenant_id = app_tenant_id());

REVOKE UPDATE, DELETE, TRUNCATE ON stock_conversions FROM aciraba_app;
REVOKE DELETE, TRUNCATE ON stock_conversion_counters FROM aciraba_app;

-- +goose Down
DROP TABLE stock_conversions;
DROP TABLE stock_conversion_counters;
