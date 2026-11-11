-- +goose Up
-- +goose StatementBegin

-- Backup terjadwal ke Google Drive, diatur Platform Admin dari panel. Satu baris konfigurasi; rahasia (client secret, refresh token,
-- passphrase enkripsi) tersimpan terenkripsi AES-GCM dan tabelnya tidak bisa dibaca langsung oleh aplikasi: akses lewat fungsi.
CREATE TABLE backup_config (
    id                boolean PRIMARY KEY DEFAULT true CHECK (id),
    drive_client_id   text NOT NULL DEFAULT '' CHECK (char_length(drive_client_id) <= 300),
    client_secret_enc text NOT NULL DEFAULT '',
    refresh_token_enc text NOT NULL DEFAULT '',
    account_email     text NOT NULL DEFAULT '',
    folder_id         text NOT NULL DEFAULT '',
    passphrase_enc    text NOT NULL DEFAULT '',
    enabled           boolean NOT NULL DEFAULT false,
    run_hour          int NOT NULL DEFAULT 2 CHECK (run_hour BETWEEN 0 AND 23),
    keep_count        int NOT NULL DEFAULT 14 CHECK (keep_count BETWEEN 1 AND 365),
    include_uploads   boolean NOT NULL DEFAULT true,
    updated_by        uuid,
    updated_at        timestamptz NOT NULL DEFAULT now()
);
ALTER TABLE backup_config ENABLE ROW LEVEL SECURITY;
REVOKE ALL ON backup_config FROM aciraba_app;
INSERT INTO backup_config (id) VALUES (true);

-- Riwayat eksekusi (tanpa rahasia). Tidak bertenant; hanya dipakai layanan backup dan panel platform.
CREATE TABLE backup_runs (
    id               uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    kind             text NOT NULL CHECK (kind IN ('auto', 'manual', 'restore_test')),
    status           text NOT NULL DEFAULT 'running' CHECK (status IN ('running', 'ok', 'failed')),
    started_at       timestamptz NOT NULL DEFAULT now(),
    finished_at      timestamptz,
    db_name          text NOT NULL DEFAULT '',
    db_file_id       text NOT NULL DEFAULT '',
    db_bytes         bigint NOT NULL DEFAULT 0,
    uploads_name     text NOT NULL DEFAULT '',
    uploads_file_id  text NOT NULL DEFAULT '',
    uploads_bytes    bigint NOT NULL DEFAULT 0,
    deleted_at       timestamptz,
    detail           text NOT NULL DEFAULT '' CHECK (char_length(detail) <= 1000)
);
CREATE INDEX backup_runs_started_idx ON backup_runs (started_at DESC, id DESC);
GRANT SELECT, INSERT, UPDATE ON backup_runs TO aciraba_app;

CREATE FUNCTION platform_backup_get(p_actor uuid) RETURNS TABLE (
    client_id text, has_secret boolean, connected boolean, account_email text, folder_id text, has_passphrase boolean,
    enabled boolean, run_hour int, keep_count int, include_uploads boolean, updated_at timestamptz
)
LANGUAGE plpgsql STABLE SECURITY DEFINER SET search_path = public, pg_temp AS
$$
BEGIN
    PERFORM platform_assert_admin(p_actor);
    RETURN QUERY
    SELECT c.drive_client_id, c.client_secret_enc <> '', c.refresh_token_enc <> '', c.account_email, c.folder_id,
           c.passphrase_enc <> '', c.enabled, c.run_hour, c.keep_count, c.include_uploads, c.updated_at
      FROM backup_config c;
END
$$;

-- p_secret_enc kosong = pertahankan yang lama. Ganti client ID memutus koneksi Drive (token milik klien lama).
CREATE FUNCTION platform_backup_save(p_actor uuid, p_client_id text, p_secret_enc text, p_enabled boolean, p_hour int, p_keep int,
                                     p_uploads boolean) RETURNS void
LANGUAGE plpgsql SECURITY DEFINER SET search_path = public, pg_temp AS
$$
BEGIN
    PERFORM platform_assert_admin(p_actor);
    UPDATE backup_config SET
        refresh_token_enc = CASE WHEN drive_client_id <> p_client_id THEN '' ELSE refresh_token_enc END,
        account_email = CASE WHEN drive_client_id <> p_client_id THEN '' ELSE account_email END,
        folder_id = CASE WHEN drive_client_id <> p_client_id THEN '' ELSE folder_id END,
        drive_client_id = p_client_id,
        client_secret_enc = CASE WHEN p_secret_enc = '' THEN client_secret_enc ELSE p_secret_enc END,
        enabled = p_enabled, run_hour = p_hour, keep_count = p_keep, include_uploads = p_uploads,
        updated_by = p_actor, updated_at = now();
END
$$;

CREATE FUNCTION platform_backup_connect(p_actor uuid, p_refresh_enc text, p_email text, p_folder text) RETURNS void
LANGUAGE plpgsql SECURITY DEFINER SET search_path = public, pg_temp AS
$$
BEGIN
    PERFORM platform_assert_admin(p_actor);
    UPDATE backup_config SET refresh_token_enc = p_refresh_enc, account_email = p_email, folder_id = p_folder,
        updated_by = p_actor, updated_at = now();
END
$$;

CREATE FUNCTION platform_backup_disconnect(p_actor uuid) RETURNS void
LANGUAGE plpgsql SECURITY DEFINER SET search_path = public, pg_temp AS
$$
BEGIN
    PERFORM platform_assert_admin(p_actor);
    UPDATE backup_config SET refresh_token_enc = '', account_email = '', folder_id = '', enabled = false,
        updated_by = p_actor, updated_at = now();
END
$$;

-- Passphrase hanya boleh dibuat sekali (menggantinya membuat salinan lama tak terbuka oleh passphrase baru).
CREATE FUNCTION platform_backup_set_passphrase(p_actor uuid, p_enc text) RETURNS boolean
LANGUAGE plpgsql SECURITY DEFINER SET search_path = public, pg_temp AS
$$
DECLARE
    v_n int;
BEGIN
    PERFORM platform_assert_admin(p_actor);
    UPDATE backup_config SET passphrase_enc = p_enc, updated_by = p_actor, updated_at = now() WHERE passphrase_enc = '';
    GET DIAGNOSTICS v_n = ROW_COUNT;
    RETURN v_n > 0;
END
$$;

-- Dibaca layanan backup (rahasia tetap terenkripsi; dibuka di aplikasi).
CREATE FUNCTION backup_config_load() RETURNS TABLE (
    client_id text, secret_enc text, refresh_enc text, folder_id text, passphrase_enc text, enabled boolean, run_hour int,
    keep_count int, include_uploads boolean
)
LANGUAGE sql STABLE SECURITY DEFINER SET search_path = public, pg_temp AS
$$ SELECT drive_client_id, client_secret_enc, refresh_token_enc, folder_id, passphrase_enc, enabled, run_hour, keep_count,
          include_uploads FROM backup_config $$;

-- Layanan backup mencatat folder Drive baru bila folder lama hilang/dibuang ke tempat sampah.
CREATE FUNCTION backup_set_folder(p_folder text) RETURNS void
LANGUAGE sql SECURITY DEFINER SET search_path = public, pg_temp AS
$$ UPDATE backup_config SET folder_id = p_folder WHERE refresh_token_enc <> '' $$;

REVOKE ALL ON FUNCTION platform_backup_get(uuid), platform_backup_save(uuid, text, text, boolean, int, int, boolean),
    platform_backup_connect(uuid, text, text, text), platform_backup_disconnect(uuid), platform_backup_set_passphrase(uuid, text),
    backup_config_load(), backup_set_folder(text) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION platform_backup_get(uuid), platform_backup_save(uuid, text, text, boolean, int, int, boolean),
    platform_backup_connect(uuid, text, text, text), platform_backup_disconnect(uuid), platform_backup_set_passphrase(uuid, text),
    backup_config_load(), backup_set_folder(text) TO aciraba_app;

-- +goose StatementEnd

-- +goose Down
DROP FUNCTION backup_set_folder(text);
DROP FUNCTION backup_config_load();
DROP FUNCTION platform_backup_set_passphrase(uuid, text);
DROP FUNCTION platform_backup_disconnect(uuid);
DROP FUNCTION platform_backup_connect(uuid, text, text, text);
DROP FUNCTION platform_backup_save(uuid, text, text, boolean, int, int, boolean);
DROP FUNCTION platform_backup_get(uuid);
DROP TABLE backup_runs;
DROP TABLE backup_config;
