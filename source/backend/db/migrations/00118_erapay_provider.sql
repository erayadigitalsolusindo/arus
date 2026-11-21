-- +goose Up

-- Penyedia QRIS per toko: 'erapay' (default, perilaku lama) atau 'qrisgate' (API key qg_live_..., deteksi mutasi).
-- Tabel erapay_* tetap dipakai kedua penyedia supaya alur kasir, nota, dan rekonsiliasi tidak bercabang.
ALTER TABLE erapay_settings ADD COLUMN provider text NOT NULL DEFAULT 'erapay' CHECK (provider IN ('erapay', 'qrisgate'));

-- +goose Down
ALTER TABLE erapay_settings DROP COLUMN provider;
