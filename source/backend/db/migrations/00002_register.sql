-- +goose Up

-- Nomor HP pemilik/user (hasil normalisasi: digit dengan awalan + opsional).
ALTER TABLE users
    ADD COLUMN phone text,
    ADD CONSTRAINT users_phone_format CHECK (phone IS NULL OR phone ~ '^\+?[0-9]{8,15}$'),
    -- Lapis kedua setelah validasi service: batas panjang dijaga DB.
    ADD CONSTRAINT users_email_len CHECK (char_length(email) BETWEEN 3 AND 254),
    ADD CONSTRAINT users_name_len CHECK (char_length(name) BETWEEN 1 AND 100);

ALTER TABLE tenants
    ADD COLUMN onboarding_completed_at timestamptz,
    ADD CONSTRAINT tenants_name_len CHECK (char_length(name) BETWEEN 1 AND 100);

ALTER TABLE outlets
    ADD CONSTRAINT outlets_name_len CHECK (char_length(name) BETWEEN 1 AND 100);

-- +goose Down
ALTER TABLE outlets DROP CONSTRAINT outlets_name_len;
ALTER TABLE tenants DROP CONSTRAINT tenants_name_len, DROP COLUMN onboarding_completed_at;
ALTER TABLE users
    DROP CONSTRAINT users_name_len,
    DROP CONSTRAINT users_email_len,
    DROP CONSTRAINT users_phone_format,
    DROP COLUMN phone;
