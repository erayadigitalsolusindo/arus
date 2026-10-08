-- +goose Up

-- Biaya metode pembayaran (MDR / biaya admin), ditanggung TOKO: dipotong dari uang yang masuk, TIDAK menambah total
-- belanja pelanggan. Biaya = jumlah dibayar × fee_pct% + fee_flat (boleh salah satu atau keduanya). Hanya non-tunai.
ALTER TABLE payment_methods
    ADD COLUMN fee_pct  numeric(5,2)  NOT NULL DEFAULT 0 CHECK (fee_pct >= 0 AND fee_pct <= 100),
    ADD COLUMN fee_flat numeric(18,2) NOT NULL DEFAULT 0 CHECK (fee_flat >= 0),
    ADD CONSTRAINT payment_methods_cash_no_fee CHECK (kind <> 'cash' OR (fee_pct = 0 AND fee_flat = 0));

-- Snapshot per pembayaran: tarif saat nota dibuat + biaya hasil hitung (mengubah tarif tidak mengubah nota lama).
ALTER TABLE sale_payments
    ADD COLUMN fee_pct    numeric(5,2)  NOT NULL DEFAULT 0,
    ADD COLUMN fee_flat   numeric(18,2) NOT NULL DEFAULT 0,
    ADD COLUMN fee_amount numeric(18,2) NOT NULL DEFAULT 0 CHECK (fee_amount >= 0);

-- +goose Down
ALTER TABLE sale_payments DROP COLUMN fee_pct, DROP COLUMN fee_flat, DROP COLUMN fee_amount;
ALTER TABLE payment_methods DROP CONSTRAINT payment_methods_cash_no_fee, DROP COLUMN fee_pct, DROP COLUMN fee_flat;
