-- +goose Up

-- Laporan penjualan per tanggal: indeks untuk aggregasi per metode pembayaran.
-- sale_payments tidak punya indeks di luar PK; breakdown per metode pada rentang tanggal
-- memerlukan full-scan tanpa ini (430 rb nota × multiple payments = jutaan baris).
CREATE INDEX IF NOT EXISTS sale_payments_tenant_sale_idx ON sale_payments (tenant_id, sale_id);

-- +goose Down
DROP INDEX IF EXISTS sale_payments_tenant_sale_idx;
