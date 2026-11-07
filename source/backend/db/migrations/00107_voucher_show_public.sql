-- +goose Up

-- Saklar "tampil di landing page" per kupon. Bawaan false: kode kupon tidak bocor ke halaman publik tanpa disengaja.
-- Indeks parsial kecil untuk pembacaan landing page (hanya kupon aktif yang dipublikasikan).
ALTER TABLE vouchers ADD COLUMN show_public boolean NOT NULL DEFAULT false;
CREATE INDEX vouchers_public_idx ON vouchers (tenant_id, ends_on NULLS LAST, id) WHERE show_public AND active;

-- +goose Down
DROP INDEX vouchers_public_idx;
ALTER TABLE vouchers DROP COLUMN show_public;
