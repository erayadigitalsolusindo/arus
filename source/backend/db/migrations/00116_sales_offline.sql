-- +goose Up
-- Nota yang dibuat kasir saat offline lalu disinkronkan (created_at = waktu transaksi di perangkat).
ALTER TABLE sales ADD COLUMN offline boolean NOT NULL DEFAULT false;

-- +goose Down
ALTER TABLE sales DROP COLUMN offline;
