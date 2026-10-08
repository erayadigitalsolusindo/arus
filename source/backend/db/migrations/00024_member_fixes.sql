-- +goose Up

-- Hasil code review Fase 3.3:
-- 1) Ambang level hanya unik di antara level AKTIF: level yang diarsipkan tidak lagi menahan ambangnya.
--    (Level dasar min_points = 0 tidak bisa diarsipkan, jadi ambang 0 tetap tunggal.)
ALTER TABLE member_levels DROP CONSTRAINT member_levels_min_points_key;
CREATE UNIQUE INDEX member_levels_min_points_key ON member_levels (tenant_id, min_points) WHERE active;

-- 2) Ledger boleh mencatat perubahan yang HANYA menyentuh lifetime_points (mis. pembalikan nota saat saldo sudah
--    habis), supaya lifetime/level selalu bisa direkonstruksi dari ledger.
ALTER TABLE member_point_movements DROP CONSTRAINT member_point_movements_points_check;
ALTER TABLE member_point_movements ADD CONSTRAINT member_point_movements_nonzero_chk CHECK (points <> 0 OR lifetime_delta <> 0);

-- +goose Down
ALTER TABLE member_point_movements DROP CONSTRAINT member_point_movements_nonzero_chk;
ALTER TABLE member_point_movements ADD CONSTRAINT member_point_movements_points_check CHECK (points <> 0);
DROP INDEX member_levels_min_points_key;
ALTER TABLE member_levels ADD CONSTRAINT member_levels_min_points_key UNIQUE (tenant_id, min_points);
