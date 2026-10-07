-- +goose Up

-- Pengaturan tingkat platform (bukan data tenant, jadi tanpa tenant_id): angka kebijakan yang bisa diubah
-- lewat SQL/admin tanpa deploy ulang. Aplikasi memvalidasi isi `value` saat memuat dan memakai bawaan bila rusak.
CREATE TABLE app_settings (
    key         text PRIMARY KEY,
    value       jsonb       NOT NULL,
    description text        NOT NULL DEFAULT '',
    updated_at  timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT app_settings_key_format CHECK (key ~ '^[a-z][a-z0-9_.]{1,63}$')
);

CREATE TRIGGER app_settings_set_updated_at BEFORE UPDATE ON app_settings FOR EACH ROW EXECUTE FUNCTION set_updated_at();

INSERT INTO app_settings (key, value, description) VALUES (
    'auth.login_lockout',
    '{"max_failures": 5, "lockout_minutes": [1, 5, 15, 60], "reset_after_hours": 24, "email_max_failures_per_hour": 50}',
    'Kunci login: max_failures = gagal berturut-turut (per IP+email) sebelum dikunci; lockout_minutes = durasi kunci ke-1, ke-2, ... (terakhir dipakai berulang); reset_after_hours = tingkat kunci turun setelah selama itu tanpa kegagalan; email_max_failures_per_hour = batas gagal per email dari semua IP.'
);

-- +goose Down
DROP TABLE app_settings;
