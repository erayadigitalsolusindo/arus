-- +goose Up

-- Jurnal otomatis harian membaca pembayaran/mutasi per outlet per rentang waktu; tanpa indeks ini pemindaian tabel per hari.
CREATE INDEX receivable_payments_outlet_time_idx ON receivable_payments (tenant_id, outlet_id, created_at);
CREATE INDEX payable_payments_outlet_time_idx    ON payable_payments (tenant_id, outlet_id, created_at);
CREATE INDEX member_deposit_cash_time_idx  ON member_deposit_movements (tenant_id, outlet_id, created_at) WHERE method_id IS NOT NULL;
CREATE INDEX supplier_credit_cash_time_idx ON supplier_credit_movements (tenant_id, outlet_id, created_at) WHERE method_id IS NOT NULL;

-- +goose Down
DROP INDEX supplier_credit_cash_time_idx;
DROP INDEX member_deposit_cash_time_idx;
DROP INDEX payable_payments_outlet_time_idx;
DROP INDEX receivable_payments_outlet_time_idx;
