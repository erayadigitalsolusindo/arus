-- +goose Up

-- Indeks untuk laporan hutang / piutang (internal/reports/debt): daftar beredar diurut dokumen terbaru
-- (keyset created_at, id) dan daftar pembayaran per outlet + waktu (keyset created_at, id) tanpa memindai seluruh tenant.
CREATE INDEX payables_created_idx    ON payables (tenant_id, created_at DESC, id DESC);
CREATE INDEX receivables_created_idx ON receivables (tenant_id, created_at DESC, id DESC);

-- Indeks outlet+waktu dari 00065 (jurnal otomatis) diganti versi yang memuat id & urutan menurun: tetap melayani
-- rentang waktu jurnal otomatis, dan juga top-N keyset (created_at, id) laporan pembayaran tanpa sort tambahan.
DROP INDEX payable_payments_outlet_time_idx;
DROP INDEX receivable_payments_outlet_time_idx;
CREATE INDEX payable_payments_outlet_time_idx    ON payable_payments (tenant_id, outlet_id, created_at DESC, id DESC);
CREATE INDEX receivable_payments_outlet_time_idx ON receivable_payments (tenant_id, outlet_id, created_at DESC, id DESC);

-- +goose Down

DROP INDEX receivable_payments_outlet_time_idx;
DROP INDEX payable_payments_outlet_time_idx;
CREATE INDEX receivable_payments_outlet_time_idx ON receivable_payments (tenant_id, outlet_id, created_at);
CREATE INDEX payable_payments_outlet_time_idx    ON payable_payments (tenant_id, outlet_id, created_at);
DROP INDEX receivables_created_idx;
DROP INDEX payables_created_idx;
