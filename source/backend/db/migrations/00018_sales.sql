-- +goose Up

-- Penjualan (Fase 5.1): nota tunai/split. Total dihitung SERVER (AGENTS.md §3.3); baris menyimpan snapshot nama, harga,
-- dan HPP saat transaksi. Nota tidak pernah dihapus (void = status, menyusul): DELETE dicabut dari role aplikasi.
-- Urutan hitung (diputuskan 2026-10-08): Σ(baris − potongan baris) − potongan global → +pajak toko & negara (dasar =
-- subtotal setelah potongan) → +biaya lain = total. Tanpa pembulatan.

CREATE TABLE sale_counters (
    tenant_id uuid   NOT NULL,
    outlet_id uuid   NOT NULL,
    day       date   NOT NULL,                 -- hari menurut zona waktu outlet
    last_no   bigint NOT NULL,
    PRIMARY KEY (tenant_id, outlet_id, day),
    CONSTRAINT sale_counters_outlet_fk FOREIGN KEY (tenant_id, outlet_id) REFERENCES outlets (tenant_id, id) ON DELETE RESTRICT
);

CREATE TABLE sales (
    id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id       uuid          NOT NULL,
    outlet_id       uuid          NOT NULL,
    doc_no          text          NOT NULL CHECK (char_length(doc_no) BETWEEN 1 AND 64),
    idempotency_key text          NOT NULL CHECK (char_length(idempotency_key) BETWEEN 8 AND 100),
    request_hash    text          NOT NULL,
    cashier_id      uuid,
    status          text          NOT NULL DEFAULT 'completed' CHECK (status IN ('completed', 'void')),
    note            text          NOT NULL DEFAULT '' CHECK (char_length(note) <= 500),
    subtotal        numeric(18,2) NOT NULL CHECK (subtotal >= 0),   -- Σ baris setelah potongan baris
    discount        numeric(18,2) NOT NULL CHECK (discount >= 0),   -- potongan global
    tax_store_pct   numeric(5,2)  NOT NULL,
    tax_gov_pct     numeric(5,2)  NOT NULL,
    tax_store       numeric(18,2) NOT NULL CHECK (tax_store >= 0),
    tax_gov         numeric(18,2) NOT NULL CHECK (tax_gov >= 0),
    other_cost      numeric(18,2) NOT NULL CHECK (other_cost >= 0),
    total           numeric(18,2) NOT NULL CHECK (total >= 0),
    paid            numeric(18,2) NOT NULL CHECK (paid >= 0),       -- Σ pembayaran yang diterima
    change          numeric(18,2) NOT NULL CHECK (change >= 0),     -- kembalian (paid − total), hanya dari tunai
    created_at      timestamptz   NOT NULL DEFAULT now(),
    CONSTRAINT sales_tenant_id_id_key UNIQUE (tenant_id, id),
    CONSTRAINT sales_doc_no_key UNIQUE (tenant_id, doc_no),
    CONSTRAINT sales_idem_key UNIQUE (tenant_id, idempotency_key),
    CONSTRAINT sales_outlet_fk  FOREIGN KEY (tenant_id, outlet_id)  REFERENCES outlets (tenant_id, id) ON DELETE RESTRICT,
    CONSTRAINT sales_cashier_fk FOREIGN KEY (tenant_id, cashier_id) REFERENCES users   (tenant_id, id) ON DELETE RESTRICT,
    CONSTRAINT sales_total_chk CHECK (total = subtotal - discount + tax_store + tax_gov + other_cost),
    CONSTRAINT sales_change_chk CHECK (change = paid - total)
);
CREATE INDEX sales_outlet_time_idx ON sales (tenant_id, outlet_id, created_at DESC);

CREATE TABLE sale_lines (
    id         uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id  uuid          NOT NULL,
    sale_id    uuid          NOT NULL,
    position   int           NOT NULL,
    item_id    uuid          NOT NULL,
    sku        text          NOT NULL,
    name       text          NOT NULL,
    unit_id    uuid          NOT NULL,
    unit_name  text          NOT NULL,
    factor     numeric(18,6) NOT NULL CHECK (factor > 0),          -- satuan dasar per 1 satuan jual
    qty        numeric(18,3) NOT NULL CHECK (qty > 0),             -- dalam satuan jual
    unit_price numeric(18,2) NOT NULL CHECK (unit_price >= 0),     -- per satuan jual (setelah grosir)
    unit_cost  numeric(18,2) NOT NULL CHECK (unit_cost >= 0),      -- snapshot HPP per satuan jual
    discount   numeric(18,2) NOT NULL DEFAULT 0 CHECK (discount >= 0),
    line_total numeric(18,2) NOT NULL CHECK (line_total >= 0),
    note       text          NOT NULL DEFAULT '' CHECK (char_length(note) <= 200),
    UNIQUE (sale_id, position),
    CONSTRAINT sale_lines_sale_fk FOREIGN KEY (tenant_id, sale_id) REFERENCES sales (tenant_id, id) ON DELETE RESTRICT,
    CONSTRAINT sale_lines_item_fk FOREIGN KEY (tenant_id, item_id) REFERENCES items (tenant_id, id) ON DELETE RESTRICT,
    CONSTRAINT sale_lines_unit_fk FOREIGN KEY (tenant_id, unit_id) REFERENCES units (tenant_id, id) ON DELETE RESTRICT
);
CREATE INDEX sale_lines_item_idx ON sale_lines (tenant_id, item_id);

CREATE TABLE sale_payments (
    id        uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id uuid          NOT NULL,
    sale_id   uuid          NOT NULL,
    position  int           NOT NULL,
    method    text          NOT NULL CHECK (method IN ('cash', 'debit', 'credit_card', 'ewallet', 'transfer')),
    amount    numeric(18,2) NOT NULL CHECK (amount > 0),           -- yang diterima (tunai boleh melebihi total)
    ref_no    text          NOT NULL DEFAULT '' CHECK (char_length(ref_no) <= 100),
    UNIQUE (sale_id, position),
    CONSTRAINT sale_payments_sale_fk FOREIGN KEY (tenant_id, sale_id) REFERENCES sales (tenant_id, id) ON DELETE RESTRICT
);

ALTER TABLE sale_counters ENABLE ROW LEVEL SECURITY;
ALTER TABLE sales         ENABLE ROW LEVEL SECURITY;
ALTER TABLE sale_lines    ENABLE ROW LEVEL SECURITY;
ALTER TABLE sale_payments ENABLE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON sale_counters USING (tenant_id = app_tenant_id()) WITH CHECK (tenant_id = app_tenant_id());
CREATE POLICY tenant_isolation ON sales         USING (tenant_id = app_tenant_id()) WITH CHECK (tenant_id = app_tenant_id());
CREATE POLICY tenant_isolation ON sale_lines    USING (tenant_id = app_tenant_id()) WITH CHECK (tenant_id = app_tenant_id());
CREATE POLICY tenant_isolation ON sale_payments USING (tenant_id = app_tenant_id()) WITH CHECK (tenant_id = app_tenant_id());

-- Nota tidak dihapus; baris & pembayaran juga tidak diubah (koreksi = void/retur/edit sebagai dokumen baru).
REVOKE DELETE, TRUNCATE ON sales, sale_lines, sale_payments FROM aciraba_app;
REVOKE UPDATE ON sale_lines, sale_payments FROM aciraba_app;

-- +goose Down
DROP TABLE sale_payments;
DROP TABLE sale_lines;
DROP TABLE sales;
DROP TABLE sale_counters;
