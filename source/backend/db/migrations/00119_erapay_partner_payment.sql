-- +goose Up

-- Tombol "PARTNER PAYMENT" di kasir (checkout hosted DANA lewat EraPay). Dinyalakan Platform Admin per tenant; mati bawaan.
ALTER TABLE erapay_settings ADD COLUMN partner_payment boolean NOT NULL DEFAULT false;

-- +goose Down
ALTER TABLE erapay_settings DROP COLUMN partner_payment;
