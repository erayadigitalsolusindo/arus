-- +goose Up

-- Shift kasir (FR-POS-16): kasir wajib membuka shift (modal awal) sebelum menyimpan nota; saat tutup, kasir mengisi
-- uang/bukti fisik per metode. Nilai "seharusnya" dibekukan (snapshot) saat tutup sehingga edit nota sesudahnya tidak
-- mengubah shift yang sudah ditutup. Selisih ≠ 0 wajib catatan + persetujuan PIN (izin shift_close.approve).
CREATE TABLE cash_shift_counters (
    tenant_id uuid   NOT NULL,
    outlet_id uuid   NOT NULL,
    day       date   NOT NULL,                    -- hari menurut zona waktu outlet
    last_no   bigint NOT NULL,
    PRIMARY KEY (tenant_id, outlet_id, day),
    CONSTRAINT cash_shift_counters_outlet_fk FOREIGN KEY (tenant_id, outlet_id) REFERENCES outlets (tenant_id, id) ON DELETE RESTRICT
);

CREATE TABLE cash_shifts (
    id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id       uuid          NOT NULL,
    outlet_id       uuid          NOT NULL,
    user_id         uuid          NOT NULL,       -- kasir pemilik shift
    doc_no          text          NOT NULL CHECK (char_length(doc_no) BETWEEN 1 AND 64),
    status          text          NOT NULL DEFAULT 'open' CHECK (status IN ('open', 'closed')),
    opening_cash    numeric(18,2) NOT NULL CHECK (opening_cash >= 0 AND opening_cash < 1e15),
    opened_at       timestamptz   NOT NULL,
    closed_at       timestamptz,
    closed_by       uuid,                         -- yang menutup (biasanya kasir itu sendiri)
    expected_total  numeric(18,2),                -- Σ seharusnya (semua metode, termasuk modal awal)
    counted_total   numeric(18,2),                -- Σ fisik
    diff_total      numeric(18,2),                -- Σ (fisik − seharusnya)
    diff_abs        numeric(18,2),                -- Σ |selisih| per metode (selisih yang saling menutup tetap terhitung)
    sale_count      int,
    void_count      int,
    sales_total     numeric(18,2),                -- Σ total nota completed (omzet)
    receivable_total numeric(18,2),               -- Σ bagian nota yang jadi piutang (bukan uang laci)
    note            text          NOT NULL DEFAULT '' CHECK (char_length(note) <= 500),
    approved_by     uuid,                         -- penyetuju PIN bila ada selisih
    approved_name   text,
    close_key       text          CHECK (close_key IS NULL OR char_length(close_key) BETWEEN 8 AND 100),
    created_at      timestamptz   NOT NULL DEFAULT now(),
    CONSTRAINT cash_shifts_tenant_id_id_key UNIQUE (tenant_id, id),
    CONSTRAINT cash_shifts_doc_key UNIQUE (tenant_id, doc_no),
    CONSTRAINT cash_shifts_outlet_fk   FOREIGN KEY (tenant_id, outlet_id)   REFERENCES outlets (tenant_id, id) ON DELETE RESTRICT,
    CONSTRAINT cash_shifts_user_fk     FOREIGN KEY (tenant_id, user_id)     REFERENCES users (tenant_id, id) ON DELETE RESTRICT,
    CONSTRAINT cash_shifts_closer_fk   FOREIGN KEY (tenant_id, closed_by)   REFERENCES users (tenant_id, id) ON DELETE RESTRICT,
    CONSTRAINT cash_shifts_approver_fk FOREIGN KEY (tenant_id, approved_by) REFERENCES users (tenant_id, id) ON DELETE RESTRICT,
    CONSTRAINT cash_shifts_state_chk CHECK (
        (status = 'open' AND closed_at IS NULL AND closed_by IS NULL AND expected_total IS NULL AND counted_total IS NULL
            AND diff_total IS NULL AND diff_abs IS NULL AND approved_by IS NULL AND close_key IS NULL)
        OR (status = 'closed' AND closed_at IS NOT NULL AND closed_at >= opened_at AND closed_by IS NOT NULL
            AND expected_total IS NOT NULL AND counted_total IS NOT NULL AND diff_total IS NOT NULL AND diff_abs IS NOT NULL
            AND sale_count IS NOT NULL AND void_count IS NOT NULL AND sales_total IS NOT NULL AND receivable_total IS NOT NULL
            AND close_key IS NOT NULL
            AND (diff_abs = 0 OR (approved_by IS NOT NULL AND char_length(btrim(note)) >= 3)))
    )
);
-- Satu shift terbuka per kasir per outlet.
CREATE UNIQUE INDEX cash_shifts_one_open_idx ON cash_shifts (tenant_id, outlet_id, user_id) WHERE status = 'open';
CREATE UNIQUE INDEX cash_shifts_close_key_idx ON cash_shifts (tenant_id, close_key) WHERE close_key IS NOT NULL;
-- Daftar shift: per outlet, terbaru dulu (keyset opened_at, id).
CREATE INDEX cash_shifts_outlet_time_idx ON cash_shifts (tenant_id, outlet_id, opened_at DESC, id DESC);
CREATE INDEX cash_shifts_user_time_idx   ON cash_shifts (tenant_id, user_id, opened_at DESC);

-- Rekap per metode yang dibekukan saat tutup (append-only).
CREATE TABLE cash_shift_counts (
    tenant_id   uuid          NOT NULL,
    shift_id    uuid          NOT NULL,
    position    int           NOT NULL CHECK (position >= 1),
    method_id   uuid          NOT NULL,
    method_name text          NOT NULL,       -- snapshot nama metode saat tutup
    kind        text          NOT NULL,
    sales       numeric(18,2) NOT NULL,       -- uang dari penjualan lewat metode ini (tunai bersih kembalian)
    flows       numeric(18,2) NOT NULL,       -- arus lain (bayar piutang, deposit, retur, hutang, …), bertanda
    opening     numeric(18,2) NOT NULL,       -- modal awal (hanya baris tunai)
    expected    numeric(18,2) NOT NULL,       -- opening + sales + flows
    counted     numeric(18,2) NOT NULL CHECK (counted >= 0),
    diff        numeric(18,2) NOT NULL,       -- counted − expected
    PRIMARY KEY (tenant_id, shift_id, position),
    CONSTRAINT cash_shift_counts_method_key UNIQUE (tenant_id, shift_id, method_id),
    CONSTRAINT cash_shift_counts_shift_fk  FOREIGN KEY (tenant_id, shift_id)  REFERENCES cash_shifts (tenant_id, id) ON DELETE RESTRICT,
    CONSTRAINT cash_shift_counts_method_fk FOREIGN KEY (tenant_id, method_id) REFERENCES payment_methods (tenant_id, id) ON DELETE RESTRICT,
    CONSTRAINT cash_shift_counts_math_chk CHECK (expected = opening + sales + flows AND diff = counted - expected)
);

-- Arus lain per sumber yang dibekukan saat tutup (append-only; hanya untuk rincian struk/daftar).
CREATE TABLE cash_shift_flows (
    tenant_id   uuid          NOT NULL,
    shift_id    uuid          NOT NULL,
    position    int           NOT NULL CHECK (position >= 1),
    source      text          NOT NULL,
    method_id   uuid          NOT NULL,
    method_name text          NOT NULL,
    kind        text          NOT NULL,
    amount      numeric(18,2) NOT NULL,
    doc_count   int           NOT NULL CHECK (doc_count >= 0),
    PRIMARY KEY (tenant_id, shift_id, position),
    CONSTRAINT cash_shift_flows_shift_fk FOREIGN KEY (tenant_id, shift_id) REFERENCES cash_shifts (tenant_id, id) ON DELETE RESTRICT
);

-- Rekap per kasir pakai rentang waktu shift: indeks penjualan per kasir + waktu.
CREATE INDEX IF NOT EXISTS sales_cashier_time_idx ON sales (tenant_id, outlet_id, cashier_id, created_at);

ALTER TABLE cash_shift_counters ENABLE ROW LEVEL SECURITY;
ALTER TABLE cash_shifts         ENABLE ROW LEVEL SECURITY;
ALTER TABLE cash_shift_counts   ENABLE ROW LEVEL SECURITY;
ALTER TABLE cash_shift_flows    ENABLE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON cash_shift_counters USING (tenant_id = app_tenant_id()) WITH CHECK (tenant_id = app_tenant_id());
CREATE POLICY tenant_isolation ON cash_shifts         USING (tenant_id = app_tenant_id()) WITH CHECK (tenant_id = app_tenant_id());
CREATE POLICY tenant_isolation ON cash_shift_counts   USING (tenant_id = app_tenant_id()) WITH CHECK (tenant_id = app_tenant_id());
CREATE POLICY tenant_isolation ON cash_shift_flows    USING (tenant_id = app_tenant_id()) WITH CHECK (tenant_id = app_tenant_id());
REVOKE DELETE, TRUNCATE ON cash_shift_counters FROM aciraba_app;
REVOKE UPDATE, DELETE, TRUNCATE ON cash_shifts, cash_shift_counts, cash_shift_flows FROM aciraba_app;
-- Shift hanya boleh diubah saat ditutup (kolom penutupan); data buka shift tidak bisa diubah.
GRANT UPDATE (status, closed_at, closed_by, expected_total, counted_total, diff_total, diff_abs, sale_count, void_count, sales_total,
              receivable_total, note, approved_by, approved_name, close_key) ON cash_shifts TO aciraba_app;

-- +goose Down
REVOKE UPDATE (status, closed_at, closed_by, expected_total, counted_total, diff_total, diff_abs, sale_count, void_count, sales_total,
               receivable_total, note, approved_by, approved_name, close_key) ON cash_shifts FROM aciraba_app;
DROP INDEX IF EXISTS sales_cashier_time_idx;
DROP TABLE cash_shift_flows;
DROP TABLE cash_shift_counts;
DROP TABLE cash_shifts;
DROP TABLE cash_shift_counters;
