-- +goose Up

-- Siapa yang menanggung biaya metode: 'store' = toko (dipotong dari uang masuk, total nota tetap) atau 'customer' =
-- pelanggan (biaya tambahan di atas total nota: pelanggan membayar jumlah dibayar + biaya). Diatur per metode.
ALTER TABLE payment_methods
    ADD COLUMN fee_bearer text NOT NULL DEFAULT 'store' CHECK (fee_bearer IN ('store', 'customer')),
    ADD CONSTRAINT payment_methods_cash_store_bearer CHECK (kind <> 'cash' OR fee_bearer = 'store');

-- Snapshot per pembayaran + total biaya yang dibebankan ke pelanggan pada nota (di luar `total`: stok, pajak, poin,
-- dan kupon tidak terpengaruh). Ditagih ke pelanggan = total + surcharge.
ALTER TABLE sale_payments ADD COLUMN fee_bearer text NOT NULL DEFAULT 'store' CHECK (fee_bearer IN ('store', 'customer'));
ALTER TABLE sales ADD COLUMN surcharge numeric(18,2) NOT NULL DEFAULT 0 CHECK (surcharge >= 0);

-- +goose Down
ALTER TABLE sales DROP COLUMN surcharge;
ALTER TABLE sale_payments DROP COLUMN fee_bearer;
ALTER TABLE payment_methods DROP CONSTRAINT payment_methods_cash_store_bearer, DROP COLUMN fee_bearer;
