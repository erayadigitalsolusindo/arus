-- +goose Up

-- Stok opname (Fase 4.3). Satu sesi = satu outlet + satu bucket. Alur: draf (tambah barang, isi hasil hitung) → selesai
-- (selisih menjadi movement OPNAME) atau dibatalkan. Stok sistem saat barang ditambahkan disimpan sebagai snapshot;
-- saat selesai stok disesuaikan sebesar (hasil hitung − snapshot) terhadap saldo TERKINI, sehingga penjualan yang terjadi
-- di sela penghitungan tidak tertimpa. Sesi selesai/batal tidak dapat diubah lagi (dijaga service); koreksi = sesi baru.

CREATE TABLE stock_count_counters (
    tenant_id uuid   NOT NULL,
    outlet_id uuid   NOT NULL,
    day       date   NOT NULL,                 -- hari menurut zona waktu outlet
    last_no   bigint NOT NULL,
    PRIMARY KEY (tenant_id, outlet_id, day),
    CONSTRAINT stock_count_counters_outlet_fk FOREIGN KEY (tenant_id, outlet_id) REFERENCES outlets (tenant_id, id) ON DELETE RESTRICT
);

CREATE TABLE stock_counts (
    id           uuid PRIMARY KEY,
    tenant_id    uuid        NOT NULL,
    outlet_id    uuid        NOT NULL,
    doc_no       text        NOT NULL CHECK (char_length(doc_no) BETWEEN 1 AND 64),
    bucket       text        NOT NULL CHECK (bucket IN ('display', 'warehouse', 'returns')),
    status       text        NOT NULL DEFAULT 'draft' CHECK (status IN ('draft', 'completed', 'cancelled')),
    note         text        NOT NULL DEFAULT '' CHECK (char_length(note) <= 1000),
    created_by   uuid,
    created_at   timestamptz NOT NULL DEFAULT now(),
    completed_by uuid,
    completed_at timestamptz,
    cancelled_by uuid,
    cancelled_at timestamptz,
    CONSTRAINT stock_counts_doc_no_key UNIQUE (tenant_id, doc_no),
    CONSTRAINT stock_counts_tenant_id_id_key UNIQUE (tenant_id, id),
    CONSTRAINT stock_counts_state_chk CHECK (
        (status = 'draft'     AND completed_at IS NULL AND cancelled_at IS NULL) OR
        (status = 'completed' AND completed_at IS NOT NULL AND cancelled_at IS NULL) OR
        (status = 'cancelled' AND cancelled_at IS NOT NULL AND completed_at IS NULL)),
    CONSTRAINT stock_counts_outlet_fk FOREIGN KEY (tenant_id, outlet_id)    REFERENCES outlets (tenant_id, id) ON DELETE RESTRICT,
    CONSTRAINT stock_counts_creator_fk FOREIGN KEY (tenant_id, created_by)   REFERENCES users   (tenant_id, id) ON DELETE RESTRICT,
    CONSTRAINT stock_counts_completer_fk FOREIGN KEY (tenant_id, completed_by) REFERENCES users (tenant_id, id) ON DELETE RESTRICT,
    CONSTRAINT stock_counts_canceller_fk FOREIGN KEY (tenant_id, cancelled_by) REFERENCES users (tenant_id, id) ON DELETE RESTRICT
);
CREATE INDEX stock_counts_outlet_time_idx ON stock_counts (tenant_id, outlet_id, created_at DESC);

CREATE TABLE stock_count_lines (
    tenant_id    uuid          NOT NULL,
    count_id     uuid          NOT NULL,
    item_id      uuid          NOT NULL,
    snapshot_qty numeric(18,3) NOT NULL,                              -- stok sistem saat barang ditambahkan (bucket sesi)
    counted_qty  numeric(18,3) CHECK (counted_qty >= 0),              -- NULL = belum dihitung (tidak ikut disesuaikan)
    diff         numeric(18,3),                                       -- counted − snapshot; diisi saat selesai
    unit_cost    numeric(18,2) CHECK (unit_cost >= 0),                -- HPP rata-rata saat selesai (nilai selisih)
    counted_by   uuid,
    counted_at   timestamptz,
    PRIMARY KEY (count_id, item_id),
    CONSTRAINT stock_count_lines_count_fk FOREIGN KEY (tenant_id, count_id) REFERENCES stock_counts (tenant_id, id) ON DELETE RESTRICT,
    CONSTRAINT stock_count_lines_item_fk  FOREIGN KEY (tenant_id, item_id)  REFERENCES items        (tenant_id, id) ON DELETE RESTRICT,
    CONSTRAINT stock_count_lines_user_fk  FOREIGN KEY (tenant_id, counted_by) REFERENCES users      (tenant_id, id) ON DELETE RESTRICT
);
CREATE INDEX stock_count_lines_item_idx ON stock_count_lines (tenant_id, item_id);

ALTER TABLE stock_count_counters ENABLE ROW LEVEL SECURITY;
ALTER TABLE stock_counts         ENABLE ROW LEVEL SECURITY;
ALTER TABLE stock_count_lines    ENABLE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON stock_count_counters USING (tenant_id = app_tenant_id()) WITH CHECK (tenant_id = app_tenant_id());
CREATE POLICY tenant_isolation ON stock_counts         USING (tenant_id = app_tenant_id()) WITH CHECK (tenant_id = app_tenant_id());
CREATE POLICY tenant_isolation ON stock_count_lines    USING (tenant_id = app_tenant_id()) WITH CHECK (tenant_id = app_tenant_id());

-- Dokumen tidak pernah dihapus (batal = status). Baris hanya boleh dihapus selagi draf (dijaga service).
REVOKE DELETE, TRUNCATE ON stock_counts, stock_count_counters FROM aciraba_app;
REVOKE TRUNCATE ON stock_count_lines FROM aciraba_app;

-- +goose Down
DROP TABLE stock_count_lines;
DROP TABLE stock_counts;
DROP TABLE stock_count_counters;
