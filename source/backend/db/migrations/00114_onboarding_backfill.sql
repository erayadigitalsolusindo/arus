-- +goose Up
-- Wizard penyiapan awal (onboarding) hanya untuk tenant baru: tenant yang sudah ada dianggap selesai.
UPDATE tenants SET onboarding_completed_at = created_at WHERE onboarding_completed_at IS NULL;

-- +goose Down
-- Tidak ada yang dikembalikan: penanda lama tidak bisa dibedakan dari yang diisi wizard.
SELECT 1;
