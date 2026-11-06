-- +goose Up

-- 2FA (TOTP, RFC 6238) opsional untuk pengguna tenant. Rahasia disimpan terenkripsi (AES-GCM, kunci turunan JWT_SECRET
-- dengan label khusus pengguna, terikat ke id pengguna). Berbeda dengan Platform Admin (00010), tabel ini bertenant biasa:
-- dibaca/ditulis lewat db.WithTenant karena tenant sudah diketahui (password terbukti dulu) sebelum kode diperiksa.
ALTER TABLE users
    ADD COLUMN totp_secret_enc text,
    ADD COLUMN totp_enabled_at timestamptz,
    -- Periode TOTP terakhir yang dipakai: kode yang sama tidak bisa dipakai dua kali (anti-replay).
    ADD COLUMN totp_last_step  bigint NOT NULL DEFAULT 0;

CREATE TABLE user_recovery_codes (
    tenant_id uuid        NOT NULL,
    user_id   uuid        NOT NULL,
    code_hash text        NOT NULL,
    used_at   timestamptz,
    PRIMARY KEY (user_id, code_hash),
    CONSTRAINT user_recovery_codes_user_fk FOREIGN KEY (tenant_id, user_id) REFERENCES users (tenant_id, id) ON DELETE CASCADE
);
CREATE INDEX user_recovery_codes_tenant_idx ON user_recovery_codes (tenant_id);
ALTER TABLE user_recovery_codes ENABLE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON user_recovery_codes USING (tenant_id = app_tenant_id()) WITH CHECK (tenant_id = app_tenant_id());
GRANT SELECT, INSERT, UPDATE, DELETE ON user_recovery_codes TO aciraba_app;

-- Bio profil mendukung markdown ringan: batas diperlebar dari 300 ke 1000 karakter.
ALTER TABLE users DROP CONSTRAINT users_bio_len;
ALTER TABLE users ADD CONSTRAINT users_bio_len CHECK (bio IS NULL OR char_length(bio) BETWEEN 1 AND 1000);

-- +goose Down
ALTER TABLE users DROP CONSTRAINT users_bio_len;
ALTER TABLE users ADD CONSTRAINT users_bio_len CHECK (bio IS NULL OR char_length(bio) BETWEEN 1 AND 300);
DROP TABLE user_recovery_codes;
ALTER TABLE users DROP COLUMN totp_last_step, DROP COLUMN totp_enabled_at, DROP COLUMN totp_secret_enc;
