-- +goose Up

-- 2FA (TOTP) untuk Platform Admin. Rahasia disimpan terenkripsi (AES-GCM, kunci turunan JWT_SECRET; terikat ke id admin).
-- Seperti 00008: aplikasi hanya SELECT, penulisan lewat fungsi SECURITY DEFINER.

ALTER TABLE platform_admins
    ADD COLUMN totp_secret_enc text,
    ADD COLUMN totp_enabled_at timestamptz,
    -- Periode TOTP terakhir yang dipakai: kode yang sama tidak bisa dipakai dua kali (anti-replay).
    ADD COLUMN totp_last_step  bigint NOT NULL DEFAULT 0;

CREATE TABLE platform_recovery_codes (
    admin_id  uuid        NOT NULL REFERENCES platform_admins (id) ON DELETE CASCADE,
    code_hash text        NOT NULL,
    used_at   timestamptz,
    PRIMARY KEY (admin_id, code_hash)
);
REVOKE ALL ON platform_recovery_codes FROM aciraba_app;
GRANT SELECT ON platform_recovery_codes TO aciraba_app;

-- +goose StatementBegin
-- Simpan rahasia yang BELUM aktif (pendaftaran). Hanya bila 2FA belum aktif.
CREATE FUNCTION platform_admin_totp_begin(p_admin uuid, p_secret_enc text) RETURNS boolean
LANGUAGE plpgsql SECURITY DEFINER SET search_path = public, pg_temp AS
$$
BEGIN
    PERFORM platform_assert_admin(p_admin);
    UPDATE platform_admins SET totp_secret_enc = p_secret_enc, totp_last_step = 0
     WHERE id = p_admin AND totp_enabled_at IS NULL;
    RETURN FOUND;
END
$$;

CREATE FUNCTION platform_admin_recovery_replace(p_admin uuid, p_hashes text[]) RETURNS void
LANGUAGE plpgsql SECURITY DEFINER SET search_path = public, pg_temp AS
$$
BEGIN
    PERFORM platform_assert_admin(p_admin);
    DELETE FROM platform_recovery_codes WHERE admin_id = p_admin;
    INSERT INTO platform_recovery_codes (admin_id, code_hash) SELECT p_admin, unnest(p_hashes);
END
$$;

-- Aktifkan setelah kode pertama terbukti benar (p_step = periodenya) sekaligus menyimpan kode pemulihan.
CREATE FUNCTION platform_admin_totp_enable(p_admin uuid, p_step bigint, p_hashes text[], p_now timestamptz) RETURNS boolean
LANGUAGE plpgsql SECURITY DEFINER SET search_path = public, pg_temp AS
$$
BEGIN
    PERFORM platform_assert_admin(p_admin);
    UPDATE platform_admins SET totp_enabled_at = p_now, totp_last_step = p_step
     WHERE id = p_admin AND totp_secret_enc IS NOT NULL AND totp_enabled_at IS NULL;
    IF NOT FOUND THEN
        RETURN false;
    END IF;
    PERFORM platform_admin_recovery_replace(p_admin, p_hashes);
    RETURN true;
END
$$;

-- Tandai periode dipakai; false bila periode itu (atau yang lebih baru) sudah pernah dipakai.
CREATE FUNCTION platform_admin_totp_step(p_admin uuid, p_step bigint) RETURNS boolean
LANGUAGE sql SECURITY DEFINER SET search_path = public, pg_temp AS
$$
    WITH u AS (UPDATE platform_admins SET totp_last_step = p_step
               WHERE id = p_admin AND totp_enabled_at IS NOT NULL AND totp_last_step < p_step RETURNING 1)
    SELECT EXISTS (SELECT 1 FROM u)
$$;

CREATE FUNCTION platform_admin_use_recovery(p_admin uuid, p_hash text) RETURNS boolean
LANGUAGE sql SECURITY DEFINER SET search_path = public, pg_temp AS
$$
    WITH u AS (UPDATE platform_recovery_codes SET used_at = now()
               WHERE admin_id = p_admin AND code_hash = p_hash AND used_at IS NULL RETURNING 1)
    SELECT EXISTS (SELECT 1 FROM u)
$$;

-- Nonaktifkan 2FA (diri sendiri setelah verifikasi, atau admin lain untuk pemulihan perangkat hilang). Token akses
-- target dicabut (p_now = jam aplikasi); admin itu wajib mendaftar ulang 2FA saat masuk berikutnya.
CREATE FUNCTION platform_admin_totp_reset(p_actor uuid, p_target uuid, p_now timestamptz) RETURNS boolean
LANGUAGE plpgsql SECURITY DEFINER SET search_path = public, pg_temp AS
$$
BEGIN
    PERFORM platform_assert_admin(p_actor);
    UPDATE platform_admins
       SET totp_secret_enc = NULL, totp_enabled_at = NULL, totp_last_step = 0, tokens_valid_after = p_now
     WHERE id = p_target;
    IF NOT FOUND THEN
        RETURN false;
    END IF;
    DELETE FROM platform_recovery_codes WHERE admin_id = p_target;
    RETURN true;
END
$$;
-- +goose StatementEnd

REVOKE ALL ON FUNCTION platform_admin_totp_begin(uuid, text), platform_admin_recovery_replace(uuid, text[]),
    platform_admin_totp_enable(uuid, bigint, text[], timestamptz), platform_admin_totp_step(uuid, bigint),
    platform_admin_use_recovery(uuid, text), platform_admin_totp_reset(uuid, uuid, timestamptz) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION platform_admin_totp_begin(uuid, text), platform_admin_recovery_replace(uuid, text[]),
    platform_admin_totp_enable(uuid, bigint, text[], timestamptz), platform_admin_totp_step(uuid, bigint),
    platform_admin_use_recovery(uuid, text), platform_admin_totp_reset(uuid, uuid, timestamptz) TO aciraba_app;

-- +goose Down
DROP FUNCTION platform_admin_totp_reset(uuid, uuid, timestamptz);
DROP FUNCTION platform_admin_use_recovery(uuid, text);
DROP FUNCTION platform_admin_totp_step(uuid, bigint);
DROP FUNCTION platform_admin_totp_enable(uuid, bigint, text[], timestamptz);
DROP FUNCTION platform_admin_recovery_replace(uuid, text[]);
DROP FUNCTION platform_admin_totp_begin(uuid, text);
DROP TABLE platform_recovery_codes;
ALTER TABLE platform_admins DROP COLUMN totp_last_step, DROP COLUMN totp_enabled_at, DROP COLUMN totp_secret_enc;
