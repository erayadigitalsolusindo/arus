-- +goose Up

-- Aturan transaksi kecil (BI): bila jumlah dibayar <= small_tx_limit, biaya metode = small_tx_flat (MDR flat) dan
-- menggantikan fee_pct + fee_flat. small_tx_limit = 0 berarti aturan nonaktif. Snapshot nota tetap di kolom fee_pct/fee_flat/
-- fee_amount yang sudah ada (tarif efektif yang dipakai), jadi sale_payments dan receivable_payments tidak berubah.
ALTER TABLE payment_methods
    ADD COLUMN small_tx_limit numeric(18,2) NOT NULL DEFAULT 0 CHECK (small_tx_limit >= 0),
    ADD COLUMN small_tx_flat  numeric(18,2) NOT NULL DEFAULT 0 CHECK (small_tx_flat >= 0),
    ADD CONSTRAINT payment_methods_cash_no_small_tx CHECK (kind <> 'cash' OR (small_tx_limit = 0 AND small_tx_flat = 0));

-- +goose Down
ALTER TABLE payment_methods DROP CONSTRAINT payment_methods_cash_no_small_tx, DROP COLUMN small_tx_limit, DROP COLUMN small_tx_flat;
