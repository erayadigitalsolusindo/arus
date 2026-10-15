-- +goose Up

-- Indeks untuk laporan hutang / piutang (internal/reports/debt): daftar beredar diurut dokumen terbaru
-- (keyset created_at, id) tanpa memindai seluruh tenant. Indeks pembayaran per outlet + waktu sudah dibuat di 00065
-- (payable_payments_outlet_time_idx, receivable_payments_outlet_time_idx), jadi tidak diulang di sini.
CREATE INDEX payables_created_idx             ON payables (tenant_id, created_at DESC, id DESC);
CREATE INDEX receivables_created_idx          ON receivables (tenant_id, created_at DESC, id DESC);

-- +goose Down

DROP INDEX receivables_created_idx;
DROP INDEX payables_created_idx;
