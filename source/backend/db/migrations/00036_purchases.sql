-- +goose Up

-- Pembelian (Fase 6.2). Satu nota pembelian = satu transaksi DB: header, baris, biaya lain, hutang (bila kredit), movement
-- stok `PURCHASE` (ledger) dan HPP rata-rata tertimbang per cabang (`item_outlet_costs`) berubah bersama atau batal bersama.
--
-- Hitung (tanpa pembulatan antar langkah selain 2 desimal per baris):
--   line_total = qty × harga satuan × Π(1 − diskon_i/100)          (diskon 4 tingkat, bertingkat)
--   subtotal   = Σ line_total
--   PPN masukan = subtotal × tax_pct/100                             (dipisah dari HPP)
--   total      = subtotal + PPN masukan + biaya lain                  (= yang harus dibayar ke pemasok)
--   HPP baris  = (line_total + alokasi biaya lain) ÷ qty              (biaya lain dibagi proporsional nilai baris)
-- Satuan qty/harga selalu satuan dasar item. `qty_display` + `qty_warehouse` = pembagian stok masuk per bucket.
-- Tabel baris/biaya/hutang append-only; header masih bisa berubah status (edit/batal nota, Fase 6.7).

CREATE TABLE purchases (
    id                  uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id           uuid          NOT NULL,
    outlet_id           uuid          NOT NULL,
    doc_no              text          NOT NULL CHECK (char_length(doc_no) BETWEEN 1 AND 64),
    idempotency_key     text          NOT NULL CHECK (char_length(idempotency_key) BETWEEN 8 AND 100),
    request_hash        text          NOT NULL,
    supplier_id         uuid          NOT NULL,
    supplier_invoice_no text          NOT NULL DEFAULT '' CHECK (char_length(supplier_invoice_no) <= 60),
    purchase_date       date          NOT NULL,                 -- hari bisnis (zona waktu outlet)
    payment_type        text          NOT NULL CHECK (payment_type IN ('cash', 'credit')),
    due_date            date,
    status              text          NOT NULL DEFAULT 'completed' CHECK (status IN ('completed')),
    note                text          NOT NULL DEFAULT '' CHECK (char_length(note) <= 500),
    subtotal            numeric(18,2) NOT NULL CHECK (subtotal >= 0),
    tax_pct             numeric(5,2)  NOT NULL DEFAULT 0 CHECK (tax_pct >= 0 AND tax_pct <= 100),
    tax_amount          numeric(18,2) NOT NULL DEFAULT 0 CHECK (tax_amount >= 0),
    other_cost          numeric(18,2) NOT NULL DEFAULT 0 CHECK (other_cost >= 0),
    total               numeric(18,2) NOT NULL CHECK (total >= 0),
    created_by          uuid,
    created_at          timestamptz   NOT NULL DEFAULT now(),
    CONSTRAINT purchases_total_chk CHECK (total = subtotal + tax_amount + other_cost),
    CONSTRAINT purchases_due_chk   CHECK (payment_type = 'credit' OR due_date IS NULL),
    CONSTRAINT purchases_tenant_id_id_key UNIQUE (tenant_id, id),
    CONSTRAINT purchases_doc_no_key UNIQUE (tenant_id, doc_no),
    CONSTRAINT purchases_idem_key UNIQUE (tenant_id, idempotency_key),
    CONSTRAINT purchases_outlet_fk   FOREIGN KEY (tenant_id, outlet_id)   REFERENCES outlets   (tenant_id, id) ON DELETE RESTRICT,
    CONSTRAINT purchases_supplier_fk FOREIGN KEY (tenant_id, supplier_id) REFERENCES suppliers (tenant_id, id) ON DELETE RESTRICT,
    CONSTRAINT purchases_creator_fk  FOREIGN KEY (tenant_id, created_by)  REFERENCES users     (tenant_id, id) ON DELETE RESTRICT
);
-- Nomor faktur pemasok unik per pemasok (menolak input ganda dari faktur yang sama); dijaga DB, bukan SELECT-lalu-INSERT.
CREATE UNIQUE INDEX purchases_supplier_invoice_key ON purchases (tenant_id, supplier_id, lower(supplier_invoice_no))
    WHERE supplier_invoice_no <> '' AND status = 'completed';
CREATE INDEX purchases_list_idx     ON purchases (tenant_id, outlet_id, purchase_date DESC, created_at DESC);
CREATE INDEX purchases_supplier_idx ON purchases (tenant_id, supplier_id);

CREATE TABLE purchase_counters (
    tenant_id uuid   NOT NULL,
    outlet_id uuid   NOT NULL,
    day       date   NOT NULL,                                  -- hari menurut zona waktu outlet
    last_no   bigint NOT NULL,
    PRIMARY KEY (tenant_id, outlet_id, day),
    CONSTRAINT purchase_counters_outlet_fk FOREIGN KEY (tenant_id, outlet_id) REFERENCES outlets (tenant_id, id) ON DELETE RESTRICT
);

CREATE TABLE purchase_lines (
    tenant_id     uuid          NOT NULL,
    purchase_id   uuid          NOT NULL,
    position      int           NOT NULL,
    item_id       uuid          NOT NULL,
    item_sku      text          NOT NULL,                       -- snapshot saat nota dibuat
    item_name     text          NOT NULL,
    unit_name     text          NOT NULL,
    qty_display   numeric(18,3) NOT NULL CHECK (qty_display >= 0),
    qty_warehouse numeric(18,3) NOT NULL CHECK (qty_warehouse >= 0),
    qty           numeric(18,3) NOT NULL CHECK (qty > 0),
    unit_price    numeric(18,2) NOT NULL CHECK (unit_price >= 0),
    disc1         numeric(5,2)  NOT NULL DEFAULT 0 CHECK (disc1 >= 0 AND disc1 < 100),
    disc2         numeric(5,2)  NOT NULL DEFAULT 0 CHECK (disc2 >= 0 AND disc2 < 100),
    disc3         numeric(5,2)  NOT NULL DEFAULT 0 CHECK (disc3 >= 0 AND disc3 < 100),
    disc4         numeric(5,2)  NOT NULL DEFAULT 0 CHECK (disc4 >= 0 AND disc4 < 100),
    line_total    numeric(18,2) NOT NULL CHECK (line_total >= 0),
    cost_alloc    numeric(18,2) NOT NULL DEFAULT 0 CHECK (cost_alloc >= 0),   -- bagian biaya lain nota
    unit_cost     numeric(18,2) NOT NULL CHECK (unit_cost >= 0),              -- HPP baris per satuan dasar
    stock_before  numeric(18,3) NOT NULL,                       -- stok cabang (semua bucket) tepat sebelum baris ini
    avg_before    numeric(18,2) NOT NULL CHECK (avg_before >= 0),
    avg_after     numeric(18,2) NOT NULL CHECK (avg_after >= 0),
    PRIMARY KEY (purchase_id, position),
    CONSTRAINT purchase_lines_qty_chk CHECK (qty = qty_display + qty_warehouse),
    CONSTRAINT purchase_lines_purchase_fk FOREIGN KEY (tenant_id, purchase_id) REFERENCES purchases (tenant_id, id) ON DELETE RESTRICT,
    CONSTRAINT purchase_lines_item_fk     FOREIGN KEY (tenant_id, item_id)     REFERENCES items     (tenant_id, id) ON DELETE RESTRICT
);
CREATE INDEX purchase_lines_item_idx ON purchase_lines (tenant_id, item_id);

CREATE TABLE purchase_costs (
    tenant_id   uuid          NOT NULL,
    purchase_id uuid          NOT NULL,
    position    int           NOT NULL,
    name        text          NOT NULL DEFAULT '' CHECK (char_length(name) <= 80),
    amount      numeric(18,2) NOT NULL CHECK (amount > 0),
    PRIMARY KEY (purchase_id, position),
    CONSTRAINT purchase_costs_purchase_fk FOREIGN KEY (tenant_id, purchase_id) REFERENCES purchases (tenant_id, id) ON DELETE RESTRICT
);

-- Hutang pemasok: satu per nota kredit. Saldo DIHITUNG (amount − Σ pembayaran, Fase 6.3), bukan kolom yang dimutasi.
CREATE TABLE payables (
    id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id   uuid          NOT NULL,
    outlet_id   uuid          NOT NULL,
    supplier_id uuid          NOT NULL,
    purchase_id uuid          NOT NULL,
    amount      numeric(18,2) NOT NULL CHECK (amount > 0),
    due_date    date,                                           -- NULL = tanpa jatuh tempo
    created_at  timestamptz   NOT NULL DEFAULT now(),
    CONSTRAINT payables_tenant_id_id_key UNIQUE (tenant_id, id),
    CONSTRAINT payables_purchase_key UNIQUE (tenant_id, purchase_id),
    CONSTRAINT payables_outlet_fk   FOREIGN KEY (tenant_id, outlet_id)   REFERENCES outlets   (tenant_id, id) ON DELETE RESTRICT,
    CONSTRAINT payables_supplier_fk FOREIGN KEY (tenant_id, supplier_id) REFERENCES suppliers (tenant_id, id) ON DELETE RESTRICT,
    CONSTRAINT payables_purchase_fk FOREIGN KEY (tenant_id, purchase_id) REFERENCES purchases (tenant_id, id) ON DELETE RESTRICT
);
CREATE INDEX payables_supplier_idx ON payables (tenant_id, supplier_id);
CREATE INDEX payables_due_idx      ON payables (tenant_id, due_date) WHERE due_date IS NOT NULL;

ALTER TABLE purchases         ENABLE ROW LEVEL SECURITY;
ALTER TABLE purchase_counters ENABLE ROW LEVEL SECURITY;
ALTER TABLE purchase_lines    ENABLE ROW LEVEL SECURITY;
ALTER TABLE purchase_costs    ENABLE ROW LEVEL SECURITY;
ALTER TABLE payables          ENABLE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON purchases         USING (tenant_id = app_tenant_id()) WITH CHECK (tenant_id = app_tenant_id());
CREATE POLICY tenant_isolation ON purchase_counters USING (tenant_id = app_tenant_id()) WITH CHECK (tenant_id = app_tenant_id());
CREATE POLICY tenant_isolation ON purchase_lines    USING (tenant_id = app_tenant_id()) WITH CHECK (tenant_id = app_tenant_id());
CREATE POLICY tenant_isolation ON purchase_costs    USING (tenant_id = app_tenant_id()) WITH CHECK (tenant_id = app_tenant_id());
CREATE POLICY tenant_isolation ON payables          USING (tenant_id = app_tenant_id()) WITH CHECK (tenant_id = app_tenant_id());

REVOKE DELETE, TRUNCATE ON purchases, purchase_counters FROM aciraba_app;
REVOKE UPDATE, DELETE, TRUNCATE ON purchase_lines, purchase_costs, payables FROM aciraba_app;

-- +goose Down
DROP TABLE payables;
DROP TABLE purchase_costs;
DROP TABLE purchase_lines;
DROP TABLE purchase_counters;
DROP TABLE purchases;
