-- +goose Up

-- Izin penuh (`{"*":true}`) hanya milik role sistem Owner (pemilik tenant, tingkat tertinggi di dalam tenant).
-- Akses lintas tenant ada di Platform Admin (00008), bukan role tenant. Role non-sistem yang sempat berizin `*`
-- (fitur "Administrator dinamis" yang dicabut) diturunkan menjadi tanpa izin; pengguna harus diberi role baru.
UPDATE roles SET permissions = '{}'::jsonb WHERE NOT is_system AND permissions ? '*';

ALTER TABLE roles ADD CONSTRAINT roles_wildcard_only_system CHECK (is_system OR NOT (permissions ? '*'));

-- +goose Down
ALTER TABLE roles DROP CONSTRAINT roles_wildcard_only_system;
