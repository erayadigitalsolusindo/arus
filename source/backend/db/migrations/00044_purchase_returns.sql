-- +goose Up

-- Retur pembelian (Fase 6.6, keputusan pengguna 2026-10-09). Satu dokumen retur = satu transaksi DB: header, baris, movement
-- stok PURCHASE_RETURN (keluar HANYA dari bucket Retur di outlet nota) dan HPP rata-rata cabang dihitung mundur.
--
-- Nilai retur per baris = nilai baris nota (setelah diskon) × qty retur ÷ qty beli (retur terakhir yang menghabiskan baris
-- mengambil sisa nilainya, agar Σ retur = nilai baris tepat); PPN = nilai × tarif PPN nota. Biaya lain nota TIDAK dikembalikan.
-- total = payable_cut (memotong sisa hutang nota) + refund (dana dikembalikan pemasok, dicatat dengan metode bayar).
-- Saldo hutang = payables.amount − Σ pembayaran − Σ payable_cut retur aktif (dihitung, bukan kolom yang dimutasi).
-- Batal retur = status 'void' + alasan; dampak stok/HPP dibalik di service Go. Baris retur append-only.

CREATE TABLE purchase_returns (
    id                 uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id          uuid          NOT NULL,
    outlet_id          uuid          NOT NULL,
    purchase_id        uuid          NOT NULL,
    supplier_id        uuid          NOT NULL,
    doc_no             text          NOT NULL CHECK (char_length(doc_no) BETWEEN 1 AND 64),
    idempotency_key    text          NOT NULL CHECK (char_length(idempotency_key) BETWEEN 8 AND 100),
    request_hash       text          NOT NULL,
    return_date        date          NOT NULL,                   -- hari bisnis (zona waktu outlet)
    status             text          NOT NULL DEFAULT 'completed' CHECK (status IN ('completed', 'void')),
    note               text          NOT NULL DEFAULT '' CHECK (char_length(note) <= 500),
    subtotal           numeric(18,2) NOT NULL CHECK (subtotal >= 0),
    tax_amount         numeric(18,2) NOT NULL DEFAULT 0 CHECK (tax_amount >= 0),
    total              numeric(18,2) NOT NULL CHECK (total >= 0),
    payable_cut        numeric(18,2) NOT NULL DEFAULT 0 CHECK (payable_cut >= 0),
    refund             numeric(18,2) NOT NULL DEFAULT 0 CHECK (refund >= 0),
    refund_method      text          CHECK (refund_method IN ('cash', 'debit', 'credit_card', 'ewallet', 'transfer')),
    refund_method_id   uuid,
    refund_method_name text          NOT NULL DEFAULT '',
    refund_ref         text          NOT NULL DEFAULT '' CHECK (char_length(refund_ref) <= 100),
    created_by         uuid,
    created_at         timestamptz   NOT NULL DEFAULT now(),
    void_reason        text          NOT NULL DEFAULT '' CHECK (char_length(void_reason) <= 200),
    voided_at          timestamptz,
    voided_by          uuid,
    CONSTRAINT purchase_returns_total_chk  CHECK (total = subtotal + tax_amount AND total = payable_cut + refund),
    CONSTRAINT purchase_returns_refund_chk CHECK ((refund = 0) = (refund_method_id IS NULL) AND (refund_method_id IS NULL) = (refund_method IS NULL)),
    CONSTRAINT purchase_returns_state_chk  CHECK (
        (status = 'completed' AND voided_at IS NULL)
     OR (status = 'void' AND voided_at IS NOT NULL AND voided_by IS NOT NULL AND char_length(void_reason) >= 3)),
    CONSTRAINT purchase_returns_tenant_id_id_key UNIQUE (tenant_id, id),
    CONSTRAINT purchase_returns_doc_no_key UNIQUE (tenant_id, doc_no),
    CONSTRAINT purchase_returns_idem_key   UNIQUE (tenant_id, idempotency_key),
    CONSTRAINT purchase_returns_outlet_fk   FOREIGN KEY (tenant_id, outlet_id)        REFERENCES outlets   (tenant_id, id) ON DELETE RESTRICT,
    CONSTRAINT purchase_returns_purchase_fk FOREIGN KEY (tenant_id, purchase_id)      REFERENCES purchases (tenant_id, id) ON DELETE RESTRICT,
    CONSTRAINT purchase_returns_supplier_fk FOREIGN KEY (tenant_id, supplier_id)      REFERENCES suppliers (tenant_id, id) ON DELETE RESTRICT,
    CONSTRAINT purchase_returns_method_fk   FOREIGN KEY (tenant_id, refund_method_id) REFERENCES payment_methods (tenant_id, id) ON DELETE RESTRICT,
    CONSTRAINT purchase_returns_creator_fk  FOREIGN KEY (tenant_id, created_by)       REFERENCES users     (tenant_id, id) ON DELETE RESTRICT,
    CONSTRAINT purchase_returns_voider_fk   FOREIGN KEY (tenant_id, voided_by)        REFERENCES users     (tenant_id, id) ON DELETE RESTRICT
);
CREATE INDEX purchase_returns_purchase_idx ON purchase_returns (tenant_id, purchase_id);
CREATE INDEX purchase_returns_list_idx     ON purchase_returns (tenant_id, outlet_id, return_date DESC, created_at DESC, id DESC);

CREATE TABLE purchase_return_counters (
    tenant_id uuid   NOT NULL,
    outlet_id uuid   NOT NULL,
    day       date   NOT NULL,
    last_no   bigint NOT NULL,
    PRIMARY KEY (tenant_id, outlet_id, day),
    CONSTRAINT purchase_return_counters_outlet_fk FOREIGN KEY (tenant_id, outlet_id) REFERENCES outlets (tenant_id, id) ON DELETE RESTRICT
);

CREATE TABLE purchase_return_lines (
    tenant_id         uuid          NOT NULL,
    return_id         uuid          NOT NULL,
    position          int           NOT NULL,
    purchase_position int           NOT NULL,                   -- baris nota asal (purchase_lines.position)
    item_id           uuid          NOT NULL,
    item_sku          text          NOT NULL,
    item_name         text          NOT NULL,
    unit_name         text          NOT NULL,
    qty               numeric(18,3) NOT NULL CHECK (qty > 0),
    value             numeric(18,2) NOT NULL CHECK (value >= 0),   -- nilai netto (tanpa PPN)
    unit_cost         numeric(18,2) NOT NULL CHECK (unit_cost >= 0), -- HPP baris nota asal (yang dibalik)
    PRIMARY KEY (return_id, position),
    CONSTRAINT purchase_return_lines_return_fk FOREIGN KEY (tenant_id, return_id) REFERENCES purchase_returns (tenant_id, id) ON DELETE RESTRICT,
    CONSTRAINT purchase_return_lines_item_fk   FOREIGN KEY (tenant_id, item_id)   REFERENCES items (tenant_id, id) ON DELETE RESTRICT
);

ALTER TABLE purchase_returns         ENABLE ROW LEVEL SECURITY;
ALTER TABLE purchase_return_counters ENABLE ROW LEVEL SECURITY;
ALTER TABLE purchase_return_lines    ENABLE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON purchase_returns         USING (tenant_id = app_tenant_id()) WITH CHECK (tenant_id = app_tenant_id());
CREATE POLICY tenant_isolation ON purchase_return_counters USING (tenant_id = app_tenant_id()) WITH CHECK (tenant_id = app_tenant_id());
CREATE POLICY tenant_isolation ON purchase_return_lines    USING (tenant_id = app_tenant_id()) WITH CHECK (tenant_id = app_tenant_id());

REVOKE UPDATE, DELETE, TRUNCATE ON purchase_returns FROM aciraba_app;
GRANT UPDATE (status, void_reason, voided_at, voided_by) ON purchase_returns TO aciraba_app;
REVOKE DELETE, TRUNCATE ON purchase_return_counters FROM aciraba_app;
REVOKE UPDATE, DELETE, TRUNCATE ON purchase_return_lines FROM aciraba_app;

-- +goose Down
DROP TABLE purchase_return_lines;
DROP TABLE purchase_return_counters;
DROP TABLE purchase_returns;
