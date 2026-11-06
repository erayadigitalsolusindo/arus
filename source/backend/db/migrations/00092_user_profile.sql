-- +goose Up

-- Profil pengguna: jabatan, bio, lokasi, foto profil, dan banner. Foto disimpan di storage (kunci dari id gambar,
-- pola sama dengan members.cover_image_id); ganti foto = id baru sehingga URL boleh di-cache lama.
ALTER TABLE users
    ADD COLUMN job_title       text,
    ADD COLUMN bio             text,
    ADD COLUMN location        text,
    ADD COLUMN avatar_image_id uuid,
    ADD COLUMN banner_image_id uuid,
    ADD CONSTRAINT users_job_title_len CHECK (job_title IS NULL OR char_length(job_title) BETWEEN 1 AND 80),
    ADD CONSTRAINT users_bio_len       CHECK (bio IS NULL OR char_length(bio) BETWEEN 1 AND 300),
    ADD CONSTRAINT users_location_len  CHECK (location IS NULL OR char_length(location) BETWEEN 1 AND 80);

-- Status tautan Google untuk halaman profil (user_identities tidak dapat dibaca langsung oleh aciraba_app).
-- +goose StatementBegin
CREATE FUNCTION auth_identity_has(p_provider text, p_user_id uuid)
RETURNS boolean
LANGUAGE sql STABLE SECURITY DEFINER SET search_path = public, pg_temp AS
$$
    SELECT EXISTS (SELECT 1 FROM user_identities WHERE provider = p_provider AND user_id = p_user_id)
$$;
-- +goose StatementEnd

REVOKE ALL ON FUNCTION auth_identity_has(text, uuid) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION auth_identity_has(text, uuid) TO aciraba_app;

-- +goose Down
DROP FUNCTION auth_identity_has(text, uuid);
ALTER TABLE users
    DROP CONSTRAINT users_location_len, DROP CONSTRAINT users_bio_len, DROP CONSTRAINT users_job_title_len,
    DROP COLUMN banner_image_id, DROP COLUMN avatar_image_id, DROP COLUMN location, DROP COLUMN bio, DROP COLUMN job_title;
