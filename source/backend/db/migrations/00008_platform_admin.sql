-- +goose Up

-- Platform Admin = operator ACIRABA (pemilik aplikasi), BUKAN pengguna tenant. Terpisah total dari `users`/`roles`:
-- tidak bisa dibuat atau diberikan lewat UI role tenant mana pun.
--
-- Pertahanan di level DB: role aplikasi (aciraba_app) hanya boleh MEMBACA platform_admins. Semua penulisan lewat
-- fungsi SECURITY DEFINER yang memverifikasi pelaku adalah Platform Admin aktif (kecuali bootstrap admin pertama,
-- yang hanya jalan selama tabel masih kosong). Dengan begitu celah SQL di aplikasi tidak bisa menambah "dewa" baru.

CREATE TABLE platform_admins (
    id                 uuid        PRIMARY KEY DEFAULT gen_random_uuid(),
    email              text        NOT NULL,
    name               text        NOT NULL,
    password_hash      text        NOT NULL,
    active             boolean     NOT NULL DEFAULT true,
    -- Token akses yang terbit SEBELUM waktu ini ditolak (reset password, penonaktifan). Diisi jam aplikasi.
    tokens_valid_after timestamptz NOT NULL DEFAULT to_timestamp(0),
    last_login_at      timestamptz,
    created_by         uuid,
    created_at         timestamptz NOT NULL DEFAULT now(),
    updated_at         timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT platform_admins_email_format CHECK (char_length(email) BETWEEN 3 AND 254 AND email = lower(email)),
    CONSTRAINT platform_admins_name_len     CHECK (char_length(name) BETWEEN 1 AND 100)
);
CREATE UNIQUE INDEX platform_admins_email_key ON platform_admins (lower(email));
CREATE TRIGGER platform_admins_set_updated_at BEFORE UPDATE ON platform_admins
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

REVOKE ALL ON platform_admins FROM aciraba_app;
GRANT SELECT ON platform_admins TO aciraba_app;

-- Audit tingkat platform (append-only). Tanpa FK: catatan harus bertahan walau data lain berubah.
CREATE TABLE platform_audit_log (
    id          bigint      GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    admin_id    uuid,
    admin_name  text        NOT NULL DEFAULT '',
    action      text        NOT NULL,
    tenant_id   uuid,
    tenant_name text        NOT NULL DEFAULT '',
    details     jsonb       NOT NULL DEFAULT '{}'::jsonb,
    ip          text        NOT NULL DEFAULT '',
    request_id  text        NOT NULL DEFAULT '',
    created_at  timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT platform_audit_action_format CHECK (action ~ '^[a-z][a-z0-9_.]{1,63}$')
);
CREATE INDEX platform_audit_time_idx   ON platform_audit_log (created_at DESC, id DESC);
CREATE INDEX platform_audit_tenant_idx ON platform_audit_log (tenant_id, id DESC);

REVOKE ALL ON platform_audit_log FROM aciraba_app;
GRANT SELECT, INSERT ON platform_audit_log TO aciraba_app;

-- ---- fungsi ----

-- Pelaku harus Platform Admin aktif; selain itu galat (SQLSTATE 42501 = insufficient_privilege).
-- +goose StatementBegin
CREATE FUNCTION platform_assert_admin(p_admin uuid) RETURNS void
LANGUAGE plpgsql STABLE SECURITY DEFINER SET search_path = public, pg_temp AS
$$
BEGIN
    IF p_admin IS NULL OR NOT EXISTS (SELECT 1 FROM platform_admins WHERE id = p_admin AND active) THEN
        RAISE EXCEPTION 'bukan platform admin aktif' USING ERRCODE = '42501';
    END IF;
END
$$;

-- Daftar semua tenant (melewati RLS) untuk Platform Admin. `total` = jumlah baris sebelum LIMIT/OFFSET.
CREATE FUNCTION platform_list_tenants(p_admin uuid, p_q text, p_limit int, p_offset int)
RETURNS TABLE (
    id uuid, code text, name text, active boolean, created_at timestamptz,
    outlet_count bigint, user_count bigint, last_login_at timestamptz, owner_email text, total bigint
)
LANGUAGE plpgsql STABLE SECURITY DEFINER SET search_path = public, pg_temp AS
$$
BEGIN
    PERFORM platform_assert_admin(p_admin);
    RETURN QUERY
    SELECT t.id, t.code, t.name, t.active, t.created_at,
           (SELECT count(*) FROM outlets o WHERE o.tenant_id = t.id),
           (SELECT count(*) FROM users u WHERE u.tenant_id = t.id),
           (SELECT max(u.last_login_at) FROM users u WHERE u.tenant_id = t.id),
           (SELECT u.email FROM users u JOIN roles r ON r.tenant_id = u.tenant_id AND r.id = u.role_id
             WHERE u.tenant_id = t.id AND r.is_system ORDER BY u.created_at LIMIT 1),
           count(*) OVER ()
    FROM tenants t
    WHERE COALESCE(p_q, '') = ''
       OR position(lower(p_q) in lower(t.name)) > 0
       OR position(lower(p_q) in lower(t.code)) > 0
       OR EXISTS (SELECT 1 FROM users u WHERE u.tenant_id = t.id AND position(lower(p_q) in lower(u.email)) > 0)
    ORDER BY t.created_at DESC, t.id
    LIMIT LEAST(GREATEST(p_limit, 1), 200) OFFSET GREATEST(p_offset, 0);
END
$$;

-- Admin pertama: hanya berhasil selama belum ada Platform Admin sama sekali (NULL bila sudah ada).
CREATE FUNCTION platform_admin_bootstrap(p_email text, p_name text, p_hash text) RETURNS uuid
LANGUAGE plpgsql SECURITY DEFINER SET search_path = public, pg_temp AS
$$
DECLARE v_id uuid;
BEGIN
    PERFORM pg_advisory_xact_lock(80080001);
    IF EXISTS (SELECT 1 FROM platform_admins) THEN
        RETURN NULL;
    END IF;
    INSERT INTO platform_admins (email, name, password_hash) VALUES (p_email, p_name, p_hash) RETURNING id INTO v_id;
    RETURN v_id;
END
$$;

CREATE FUNCTION platform_admin_create(p_actor uuid, p_email text, p_name text, p_hash text) RETURNS uuid
LANGUAGE plpgsql SECURITY DEFINER SET search_path = public, pg_temp AS
$$
DECLARE v_id uuid;
BEGIN
    PERFORM platform_assert_admin(p_actor);
    INSERT INTO platform_admins (email, name, password_hash, created_by) VALUES (p_email, p_name, p_hash, p_actor)
    RETURNING id INTO v_id;
    RETURN v_id;
END
$$;

-- Aktifkan/nonaktifkan. Tidak boleh menonaktifkan diri sendiri maupun admin aktif terakhir. `p_now` = jam aplikasi.
-- Mengembalikan false bila target tidak ada.
CREATE FUNCTION platform_admin_set_active(p_actor uuid, p_target uuid, p_active boolean, p_now timestamptz) RETURNS boolean
LANGUAGE plpgsql SECURITY DEFINER SET search_path = public, pg_temp AS
$$
BEGIN
    PERFORM platform_assert_admin(p_actor);
    PERFORM pg_advisory_xact_lock(80080001);
    IF NOT p_active THEN
        IF p_actor = p_target THEN
            RAISE EXCEPTION 'tidak boleh menonaktifkan diri sendiri' USING ERRCODE = 'P0001';
        END IF;
        IF NOT EXISTS (SELECT 1 FROM platform_admins WHERE active AND id <> p_target) THEN
            RAISE EXCEPTION 'admin aktif terakhir' USING ERRCODE = 'P0001';
        END IF;
    END IF;
    UPDATE platform_admins
       SET active = p_active,
           tokens_valid_after = CASE WHEN p_active THEN tokens_valid_after ELSE p_now END
     WHERE id = p_target;
    RETURN FOUND;
END
$$;

CREATE FUNCTION platform_admin_set_password(p_actor uuid, p_target uuid, p_hash text, p_now timestamptz) RETURNS boolean
LANGUAGE plpgsql SECURITY DEFINER SET search_path = public, pg_temp AS
$$
BEGIN
    PERFORM platform_assert_admin(p_actor);
    UPDATE platform_admins SET password_hash = p_hash, tokens_valid_after = p_now WHERE id = p_target;
    RETURN FOUND;
END
$$;

CREATE FUNCTION platform_admin_touch_login(p_admin uuid, p_now timestamptz) RETURNS void
LANGUAGE sql SECURITY DEFINER SET search_path = public, pg_temp AS
$$ UPDATE platform_admins SET last_login_at = p_now WHERE id = p_admin AND active $$;
-- +goose StatementEnd

REVOKE ALL ON FUNCTION platform_assert_admin(uuid), platform_list_tenants(uuid, text, int, int),
    platform_admin_bootstrap(text, text, text), platform_admin_create(uuid, text, text, text),
    platform_admin_set_active(uuid, uuid, boolean, timestamptz), platform_admin_set_password(uuid, uuid, text, timestamptz),
    platform_admin_touch_login(uuid, timestamptz) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION platform_list_tenants(uuid, text, int, int),
    platform_admin_bootstrap(text, text, text), platform_admin_create(uuid, text, text, text),
    platform_admin_set_active(uuid, uuid, boolean, timestamptz), platform_admin_set_password(uuid, uuid, text, timestamptz),
    platform_admin_touch_login(uuid, timestamptz) TO aciraba_app;

-- +goose Down
DROP FUNCTION platform_admin_touch_login(uuid, timestamptz);
DROP FUNCTION platform_admin_set_password(uuid, uuid, text, timestamptz);
DROP FUNCTION platform_admin_set_active(uuid, uuid, boolean, timestamptz);
DROP FUNCTION platform_admin_create(uuid, text, text, text);
DROP FUNCTION platform_admin_bootstrap(text, text, text);
DROP FUNCTION platform_list_tenants(uuid, text, int, int);
DROP FUNCTION platform_assert_admin(uuid);
DROP TABLE platform_audit_log;
DROP TABLE platform_admins;
