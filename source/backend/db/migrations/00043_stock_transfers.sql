-- +goose Up

-- Mutasi stok (Fase 4.3 sisa + 6.5). Satu dokumen memindahkan barang antar CABANG dalam tenant yang sama (lintas tenant
-- mustahil: semua FK bertenant_id yang sama + RLS) dan/atau antar BUCKET (display/warehouse/returns).
--   * Antar cabang: dua tahap. Kirim = stok asal berkurang (TRANSFER_OUT), status 'sent' (dalam perjalanan);
--     Terima = stok tujuan bertambah (TRANSFER_IN) sebesar qty diterima (boleh < dikirim; selisih tercatat di dokumen);
--     Batal (hanya saat 'sent') = stok dikembalikan ke asal.
--   * Antar bucket di cabang yang sama: satu tahap, langsung 'received'.
-- Hanya kolom status/penerimaan yang boleh diubah setelah dokumen dibuat.

CREATE TABLE stock_transfer_counters (
    tenant_id uuid   NOT NULL,
    outlet_id uuid   NOT NULL,
    day       date   NOT NULL,
    last_no   bigint NOT NULL,
    PRIMARY KEY (tenant_id, outlet_id, day),
    CONSTRAINT stock_transfer_counters_outlet_fk FOREIGN KEY (tenant_id, outlet_id) REFERENCES outlets (tenant_id, id) ON DELETE RESTRICT
);

CREATE TABLE stock_transfers (
    id              uuid PRIMARY KEY,
    tenant_id       uuid        NOT NULL,
    doc_no          text        NOT NULL CHECK (char_length(doc_no) BETWEEN 1 AND 64),
    idempotency_key text        NOT NULL CHECK (char_length(idempotency_key) BETWEEN 8 AND 100),
    request_hash    text        NOT NULL,
    from_outlet_id  uuid        NOT NULL,
    from_bucket     text        NOT NULL CHECK (from_bucket IN ('display', 'warehouse', 'returns')),
    to_outlet_id    uuid        NOT NULL,
    to_bucket       text        NOT NULL CHECK (to_bucket IN ('display', 'warehouse', 'returns')),
    status          text        NOT NULL CHECK (status IN ('sent', 'received', 'cancelled')),
    note            text        NOT NULL DEFAULT '' CHECK (char_length(note) <= 1000),
    sent_by         uuid,
    sent_at         timestamptz NOT NULL DEFAULT now(),
    received_by     uuid,
    received_at     timestamptz,
    cancelled_by    uuid,
    cancelled_at    timestamptz,
    cancel_reason   text        NOT NULL DEFAULT '' CHECK (char_length(cancel_reason) <= 200),
    CONSTRAINT stock_transfers_doc_no_key UNIQUE (tenant_id, doc_no),
    CONSTRAINT stock_transfers_idem_key UNIQUE (tenant_id, idempotency_key),
    CONSTRAINT stock_transfers_tenant_id_id_key UNIQUE (tenant_id, id),
    CONSTRAINT stock_transfers_move_chk CHECK (from_outlet_id <> to_outlet_id OR from_bucket <> to_bucket),
    CONSTRAINT stock_transfers_state_chk CHECK (
        (status = 'sent'      AND received_at IS NULL AND cancelled_at IS NULL)
     OR (status = 'received'  AND received_at IS NOT NULL AND cancelled_at IS NULL)
     OR (status = 'cancelled' AND cancelled_at IS NOT NULL AND received_at IS NULL)),
    CONSTRAINT stock_transfers_from_fk FOREIGN KEY (tenant_id, from_outlet_id) REFERENCES outlets (tenant_id, id) ON DELETE RESTRICT,
    CONSTRAINT stock_transfers_to_fk   FOREIGN KEY (tenant_id, to_outlet_id)   REFERENCES outlets (tenant_id, id) ON DELETE RESTRICT,
    CONSTRAINT stock_transfers_sent_by_fk      FOREIGN KEY (tenant_id, sent_by)      REFERENCES users (tenant_id, id) ON DELETE RESTRICT,
    CONSTRAINT stock_transfers_received_by_fk  FOREIGN KEY (tenant_id, received_by)  REFERENCES users (tenant_id, id) ON DELETE RESTRICT,
    CONSTRAINT stock_transfers_cancelled_by_fk FOREIGN KEY (tenant_id, cancelled_by) REFERENCES users (tenant_id, id) ON DELETE RESTRICT
);
CREATE INDEX stock_transfers_from_idx ON stock_transfers (tenant_id, from_outlet_id, sent_at DESC, id DESC);
CREATE INDEX stock_transfers_to_idx   ON stock_transfers (tenant_id, to_outlet_id, sent_at DESC, id DESC);

CREATE TABLE stock_transfer_lines (
    tenant_id    uuid          NOT NULL,
    transfer_id  uuid          NOT NULL,
    item_id      uuid          NOT NULL,
    qty_sent     numeric(18,3) NOT NULL CHECK (qty_sent > 0),
    qty_received numeric(18,3) CHECK (qty_received >= 0 AND qty_received <= qty_sent),   -- NULL = belum diterima
    unit_cost    numeric(18,2) NOT NULL CHECK (unit_cost >= 0),                           -- HPP cabang asal saat dikirim
    PRIMARY KEY (transfer_id, item_id),
    CONSTRAINT stock_transfer_lines_transfer_fk FOREIGN KEY (tenant_id, transfer_id) REFERENCES stock_transfers (tenant_id, id) ON DELETE RESTRICT,
    CONSTRAINT stock_transfer_lines_item_fk     FOREIGN KEY (tenant_id, item_id)     REFERENCES items (tenant_id, id) ON DELETE RESTRICT
);

ALTER TABLE stock_transfer_counters ENABLE ROW LEVEL SECURITY;
ALTER TABLE stock_transfers         ENABLE ROW LEVEL SECURITY;
ALTER TABLE stock_transfer_lines    ENABLE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON stock_transfer_counters USING (tenant_id = app_tenant_id()) WITH CHECK (tenant_id = app_tenant_id());
CREATE POLICY tenant_isolation ON stock_transfers         USING (tenant_id = app_tenant_id()) WITH CHECK (tenant_id = app_tenant_id());
CREATE POLICY tenant_isolation ON stock_transfer_lines    USING (tenant_id = app_tenant_id()) WITH CHECK (tenant_id = app_tenant_id());

REVOKE DELETE, TRUNCATE ON stock_transfer_counters FROM aciraba_app;
REVOKE UPDATE, DELETE, TRUNCATE ON stock_transfers, stock_transfer_lines FROM aciraba_app;
GRANT UPDATE (status, received_by, received_at, cancelled_by, cancelled_at, cancel_reason) ON stock_transfers TO aciraba_app;
GRANT UPDATE (qty_received) ON stock_transfer_lines TO aciraba_app;

-- +goose Down
DROP TABLE stock_transfer_lines;
DROP TABLE stock_transfers;
DROP TABLE stock_transfer_counters;
