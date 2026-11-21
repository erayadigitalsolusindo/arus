-- +goose Up

-- Nominal yang benar-benar dibayar pelanggan di QR (QRISGate: harga + kode unik). Kosong = sama dengan `amount`.
-- `amount` tetap harga nota; selisihnya (kode unik) masuk ke rekening toko dan tidak dicatat di nota.
ALTER TABLE erapay_charges ADD COLUMN pay_amount numeric(18, 2);

-- +goose Down
ALTER TABLE erapay_charges DROP COLUMN pay_amount;
