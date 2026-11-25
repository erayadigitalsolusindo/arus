-- +goose Up

-- Hak akses Platform Admin: role dengan izin per modul (CRUD) seperti role tenant, tetapi tabelnya terpisah total.
-- Role dikaitkan lewat tabel sendiri (bukan kolom di platform_admins) agar baris admin dan kueri login tidak berubah.
-- Pertahanan di DB sama dengan platform_admins: aciraba_app hanya boleh MEMBACA; penulisan lewat fungsi SECURITY DEFINER
-- yang memverifikasi pelaku adalah Platform Admin aktif. Pembatasan "siapa boleh apa" ditegakkan aplikasi (izin `roles`/`admins`).

CREATE TABLE platform_roles (
    id          uuid        PRIMARY KEY DEFAULT gen_random_uuid(),
    name        text        NOT NULL,
    permissions jsonb       NOT NULL DEFAULT '{}'::jsonb,
    is_system   boolean     NOT NULL DEFAULT false,
    created_at  timestamptz NOT NULL DEFAULT now(),
    updated_at  timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT platform_roles_name_len CHECK (char_length(name) BETWEEN 1 AND 50),
    CONSTRAINT platform_roles_perms_object CHECK (jsonb_typeof(permissions) = 'object'),
    -- Akses penuh ("*") hanya milik role sistem Owner; tidak pernah bisa dibuat lewat API.
    CONSTRAINT platform_roles_wildcard_only_system CHECK (is_system OR NOT (permissions ? '*'))
);
CREATE UNIQUE INDEX platform_roles_name_key ON platform_roles (lower(name));
CREATE UNIQUE INDEX platform_roles_one_system ON platform_roles (is_system) WHERE is_system;
CREATE TRIGGER platform_roles_set_updated_at BEFORE UPDATE ON platform_roles
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- Satu admin = satu role. Tanpa baris = tanpa izin sama sekali (tertutup).
CREATE TABLE platform_admin_roles (
    admin_id uuid PRIMARY KEY REFERENCES platform_admins (id) ON DELETE CASCADE,
    role_id  uuid NOT NULL REFERENCES platform_roles (id) ON DELETE RESTRICT
);
CREATE INDEX platform_admin_roles_role_idx ON platform_admin_roles (role_id);

REVOKE ALL ON platform_roles, platform_admin_roles FROM aciraba_app;
GRANT SELECT ON platform_roles, platform_admin_roles TO aciraba_app;

-- Role sistem Owner; semua admin yang ada sekarang menjadi Owner (perilaku lama: semuanya "dewa").
INSERT INTO platform_roles (name, permissions, is_system) VALUES ('Owner', '{"*": true}', true);
INSERT INTO platform_admin_roles (admin_id, role_id)
SELECT a.id, (SELECT id FROM platform_roles WHERE is_system) FROM platform_admins a;

-- Audit per admin (halaman audit memfilter admin_id; tabel ini append-only dan terus membesar).
CREATE INDEX platform_audit_admin_idx ON platform_audit_log (admin_id, id DESC);

-- ---- Jenis & lokasi tenant (penanda saja) ----
ALTER TABLE tenants
    ADD COLUMN tenant_type text NOT NULL DEFAULT '' CONSTRAINT tenants_type_len CHECK (char_length(tenant_type) <= 50),
    ADD COLUMN location    text NOT NULL DEFAULT '' CONSTRAINT tenants_location_len CHECK (char_length(location) <= 100);
CREATE INDEX tenants_type_idx ON tenants (lower(tenant_type)) WHERE tenant_type <> '';

-- ---- fungsi ----

-- Apakah ada Owner aktif selain p_target? Dipakai agar Owner aktif terakhir tidak bisa hilang/turun peran.
-- +goose StatementBegin
CREATE FUNCTION platform_other_owner_exists(p_target uuid) RETURNS boolean
LANGUAGE sql STABLE SECURITY DEFINER SET search_path = public, pg_temp AS
$$
    SELECT EXISTS (
        SELECT 1 FROM platform_admins a
          JOIN platform_admin_roles ar ON ar.admin_id = a.id
          JOIN platform_roles r ON r.id = ar.role_id AND r.is_system
         WHERE a.active AND a.id <> p_target)
$$;

CREATE FUNCTION platform_is_owner(p_target uuid) RETURNS boolean
LANGUAGE sql STABLE SECURITY DEFINER SET search_path = public, pg_temp AS
$$
    SELECT EXISTS (
        SELECT 1 FROM platform_admin_roles ar JOIN platform_roles r ON r.id = ar.role_id AND r.is_system
         WHERE ar.admin_id = p_target)
$$;

-- Simpan role (p_id NULL = baru). Role sistem tidak bisa diubah (P0001). Nama kembar = 23505. Mengembalikan id; NULL bila tak ada.
CREATE FUNCTION platform_role_save(p_actor uuid, p_id uuid, p_name text, p_perms jsonb) RETURNS uuid
LANGUAGE plpgsql SECURITY DEFINER SET search_path = public, pg_temp AS
$$
DECLARE v_id uuid;
BEGIN
    PERFORM platform_assert_admin(p_actor);
    IF p_id IS NULL THEN
        INSERT INTO platform_roles (name, permissions) VALUES (p_name, p_perms) RETURNING id INTO v_id;
        RETURN v_id;
    END IF;
    IF EXISTS (SELECT 1 FROM platform_roles WHERE id = p_id AND is_system) THEN
        RAISE EXCEPTION 'role sistem' USING ERRCODE = 'P0001';
    END IF;
    UPDATE platform_roles SET name = p_name, permissions = p_perms WHERE id = p_id RETURNING id INTO v_id;
    RETURN v_id;
END
$$;

-- Hapus role. Role sistem ditolak (P0001); role yang masih dipakai admin tertahan FK (23503).
CREATE FUNCTION platform_role_delete(p_actor uuid, p_id uuid) RETURNS boolean
LANGUAGE plpgsql SECURITY DEFINER SET search_path = public, pg_temp AS
$$
BEGIN
    PERFORM platform_assert_admin(p_actor);
    IF EXISTS (SELECT 1 FROM platform_roles WHERE id = p_id AND is_system) THEN
        RAISE EXCEPTION 'role sistem' USING ERRCODE = 'P0001';
    END IF;
    DELETE FROM platform_roles WHERE id = p_id;
    RETURN FOUND;
END
$$;

-- Ganti role admin. Owner aktif terakhir tidak boleh diturunkan (P0001). Role tak ada = 23503. false bila admin tak ada.
CREATE FUNCTION platform_admin_set_role(p_actor uuid, p_target uuid, p_role uuid) RETURNS boolean
LANGUAGE plpgsql SECURITY DEFINER SET search_path = public, pg_temp AS
$$
BEGIN
    PERFORM platform_assert_admin(p_actor);
    PERFORM pg_advisory_xact_lock(80080001);
    IF NOT EXISTS (SELECT 1 FROM platform_admins WHERE id = p_target) THEN
        RETURN false;
    END IF;
    IF platform_is_owner(p_target)
       AND NOT EXISTS (SELECT 1 FROM platform_roles WHERE id = p_role AND is_system)
       AND NOT platform_other_owner_exists(p_target) THEN
        RAISE EXCEPTION 'owner aktif terakhir' USING ERRCODE = 'P0001';
    END IF;
    INSERT INTO platform_admin_roles (admin_id, role_id) VALUES (p_target, p_role)
    ON CONFLICT (admin_id) DO UPDATE SET role_id = EXCLUDED.role_id;
    RETURN true;
END
$$;

-- Admin pertama selalu Owner.
CREATE OR REPLACE FUNCTION platform_admin_bootstrap(p_email text, p_name text, p_hash text) RETURNS uuid
LANGUAGE plpgsql SECURITY DEFINER SET search_path = public, pg_temp AS
$$
DECLARE v_id uuid;
BEGIN
    PERFORM pg_advisory_xact_lock(80080001);
    IF EXISTS (SELECT 1 FROM platform_admins) THEN
        RETURN NULL;
    END IF;
    INSERT INTO platform_admins (email, name, password_hash) VALUES (p_email, p_name, p_hash) RETURNING id INTO v_id;
    INSERT INTO platform_admin_roles (admin_id, role_id) SELECT v_id, id FROM platform_roles WHERE is_system;
    RETURN v_id;
END
$$;

-- Buat admin beserta role-nya dalam satu transaksi. p_role NULL = tanpa izin (tertutup).
DROP FUNCTION platform_admin_create(uuid, text, text, text);
CREATE FUNCTION platform_admin_create(p_actor uuid, p_email text, p_name text, p_hash text, p_role uuid DEFAULT NULL) RETURNS uuid
LANGUAGE plpgsql SECURITY DEFINER SET search_path = public, pg_temp AS
$$
DECLARE v_id uuid;
BEGIN
    PERFORM platform_assert_admin(p_actor);
    INSERT INTO platform_admins (email, name, password_hash, created_by) VALUES (p_email, p_name, p_hash, p_actor)
    RETURNING id INTO v_id;
    IF p_role IS NOT NULL THEN
        INSERT INTO platform_admin_roles (admin_id, role_id) VALUES (v_id, p_role);
    END IF;
    RETURN v_id;
END
$$;

-- Nonaktifkan: bukan diri sendiri, bukan admin aktif terakhir, dan bukan Owner aktif terakhir.
CREATE OR REPLACE FUNCTION platform_admin_set_active(p_actor uuid, p_target uuid, p_active boolean, p_now timestamptz) RETURNS boolean
LANGUAGE plpgsql SECURITY DEFINER SET search_path = public, pg_temp AS
$$
BEGIN
    PERFORM platform_assert_admin(p_actor);
    PERFORM pg_advisory_xact_lock(80080001);
    IF NOT p_active THEN
        IF p_actor = p_target THEN
            RAISE EXCEPTION 'tidak boleh menonaktifkan diri sendiri' USING ERRCODE = 'P0001';
        END IF;
        IF NOT EXISTS (SELECT 1 FROM platform_admins WHERE active AND id <> p_target)
           OR (platform_is_owner(p_target) AND NOT platform_other_owner_exists(p_target)) THEN
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

CREATE OR REPLACE FUNCTION platform_admin_delete(p_actor uuid, p_target uuid) RETURNS boolean
LANGUAGE plpgsql SECURITY DEFINER SET search_path = public, pg_temp AS
$$
BEGIN
    PERFORM platform_assert_admin(p_actor);
    PERFORM pg_advisory_xact_lock(80080001);
    IF p_actor = p_target THEN
        RAISE EXCEPTION 'tidak boleh menghapus diri sendiri' USING ERRCODE = 'P0001';
    END IF;
    IF NOT EXISTS (SELECT 1 FROM platform_admins WHERE active AND id <> p_target)
       OR (platform_is_owner(p_target) AND NOT platform_other_owner_exists(p_target)) THEN
        RAISE EXCEPTION 'admin aktif terakhir' USING ERRCODE = 'P0001';
    END IF;
    DELETE FROM platform_admins WHERE id = p_target;
    RETURN FOUND;
END
$$;

-- Daftar tenant + jenis & lokasi (filter jenis). Parameter baru ber-DEFAULT di ujung.
DROP FUNCTION platform_list_tenants(uuid, text, int, int, text, uuid, text, timestamptz, timestamptz);
CREATE FUNCTION platform_list_tenants(
    p_admin uuid, p_q text, p_limit int, p_offset int,
    p_status text DEFAULT NULL, p_plan uuid DEFAULT NULL, p_login text DEFAULT NULL,
    p_from timestamptz DEFAULT NULL, p_to timestamptz DEFAULT NULL,
    p_type text DEFAULT NULL
)
RETURNS TABLE (
    id uuid, code text, name text, active boolean, created_at timestamptz,
    outlet_count bigint, user_count bigint, last_login_at timestamptz, owner_email text,
    plan_id uuid, plan_name text, plan_expires_at timestamptz,
    tenant_type text, location text, total bigint
)
LANGUAGE plpgsql STABLE SECURITY DEFINER SET search_path = public, pg_temp AS
$$
BEGIN
    PERFORM platform_assert_admin(p_admin);
    RETURN QUERY
    WITH base AS (
        SELECT t.id, t.code, t.name, t.active, t.created_at,
               (SELECT count(*) FROM outlets o WHERE o.tenant_id = t.id) AS outlet_count,
               (SELECT count(*) FROM users u WHERE u.tenant_id = t.id) AS user_count,
               (SELECT max(u.last_login_at) FROM users u WHERE u.tenant_id = t.id) AS last_login_at,
               (SELECT u.email FROM users u JOIN roles r ON r.tenant_id = u.tenant_id AND r.id = u.role_id
                 WHERE u.tenant_id = t.id AND r.is_system ORDER BY u.created_at LIMIT 1) AS owner_email,
               p.id AS plan_id, p.name AS plan_name, s.expires_at AS plan_expires_at,
               t.tenant_type, t.location
          FROM tenants t
          LEFT JOIN tenant_subscriptions s ON s.tenant_id = t.id AND (s.expires_at IS NULL OR s.expires_at > now())
          JOIN plans p ON p.id = coalesce(s.plan_id, (SELECT d.id FROM plans d WHERE d.is_default))
         WHERE (COALESCE(p_q, '') = ''
            OR position(lower(p_q) in lower(t.name)) > 0
            OR position(lower(p_q) in lower(t.code)) > 0
            OR position(lower(p_q) in lower(t.location)) > 0
            OR EXISTS (SELECT 1 FROM users u WHERE u.tenant_id = t.id AND position(lower(p_q) in lower(u.email)) > 0))
           AND (p_status IS NULL OR p_status = '' OR t.active = (p_status = 'active'))
           AND (p_plan IS NULL OR p.id = p_plan)
           AND (p_from IS NULL OR t.created_at >= p_from)
           AND (p_to IS NULL OR t.created_at < p_to)
           AND (COALESCE(p_type, '') = '' OR lower(t.tenant_type) = lower(p_type))
    )
    SELECT b.id, b.code, b.name, b.active, b.created_at, b.outlet_count, b.user_count, b.last_login_at, b.owner_email,
           b.plan_id, b.plan_name, b.plan_expires_at, b.tenant_type, b.location, count(*) OVER ()
      FROM base b
     WHERE CASE COALESCE(p_login, '')
             WHEN 'never'   THEN b.last_login_at IS NULL
             WHEN 'stale30' THEN b.last_login_at IS NULL OR b.last_login_at < now() - interval '30 days'
             WHEN 'recent7' THEN b.last_login_at >= now() - interval '7 days'
             ELSE true END
     ORDER BY b.created_at DESC, b.id
     LIMIT LEAST(GREATEST(p_limit, 1), 200) OFFSET GREATEST(p_offset, 0);
END
$$;

-- Daftar jenis tenant yang sudah dipakai (untuk saran isian & filter). Dibatasi agar tetap murah di data besar.
CREATE FUNCTION platform_tenant_types(p_admin uuid) RETURNS TABLE (tenant_type text, n bigint)
LANGUAGE plpgsql STABLE SECURITY DEFINER SET search_path = public, pg_temp AS
$$
BEGIN
    PERFORM platform_assert_admin(p_admin);
    RETURN QUERY
    SELECT min(t.tenant_type), count(*) FROM tenants t WHERE t.tenant_type <> ''
     GROUP BY lower(t.tenant_type) ORDER BY count(*) DESC, lower(t.tenant_type) LIMIT 200;
END
$$;
-- +goose StatementEnd

REVOKE ALL ON FUNCTION platform_other_owner_exists(uuid), platform_is_owner(uuid), platform_role_save(uuid, uuid, text, jsonb),
    platform_role_delete(uuid, uuid), platform_admin_set_role(uuid, uuid, uuid), platform_admin_create(uuid, text, text, text, uuid),
    platform_list_tenants(uuid, text, int, int, text, uuid, text, timestamptz, timestamptz, text), platform_tenant_types(uuid) FROM PUBLIC;
-- Fungsi bantu (owner_exists, is_owner) hanya dipanggil dari fungsi SECURITY DEFINER lain: tidak diberikan ke aciraba_app.
GRANT EXECUTE ON FUNCTION platform_role_save(uuid, uuid, text, jsonb), platform_role_delete(uuid, uuid),
    platform_admin_set_role(uuid, uuid, uuid), platform_admin_create(uuid, text, text, text, uuid),
    platform_list_tenants(uuid, text, int, int, text, uuid, text, timestamptz, timestamptz, text), platform_tenant_types(uuid) TO aciraba_app;

-- +goose Down
-- +goose StatementBegin
DROP FUNCTION platform_tenant_types(uuid);
DROP FUNCTION platform_list_tenants(uuid, text, int, int, text, uuid, text, timestamptz, timestamptz, text);
CREATE FUNCTION platform_list_tenants(
    p_admin uuid, p_q text, p_limit int, p_offset int,
    p_status text DEFAULT NULL, p_plan uuid DEFAULT NULL, p_login text DEFAULT NULL,
    p_from timestamptz DEFAULT NULL, p_to timestamptz DEFAULT NULL
)
RETURNS TABLE (
    id uuid, code text, name text, active boolean, created_at timestamptz,
    outlet_count bigint, user_count bigint, last_login_at timestamptz, owner_email text,
    plan_id uuid, plan_name text, plan_expires_at timestamptz, total bigint
)
LANGUAGE plpgsql STABLE SECURITY DEFINER SET search_path = public, pg_temp AS
$$
BEGIN
    PERFORM platform_assert_admin(p_admin);
    RETURN QUERY
    WITH base AS (
        SELECT t.id, t.code, t.name, t.active, t.created_at,
               (SELECT count(*) FROM outlets o WHERE o.tenant_id = t.id) AS outlet_count,
               (SELECT count(*) FROM users u WHERE u.tenant_id = t.id) AS user_count,
               (SELECT max(u.last_login_at) FROM users u WHERE u.tenant_id = t.id) AS last_login_at,
               (SELECT u.email FROM users u JOIN roles r ON r.tenant_id = u.tenant_id AND r.id = u.role_id
                 WHERE u.tenant_id = t.id AND r.is_system ORDER BY u.created_at LIMIT 1) AS owner_email,
               p.id AS plan_id, p.name AS plan_name, s.expires_at AS plan_expires_at
          FROM tenants t
          LEFT JOIN tenant_subscriptions s ON s.tenant_id = t.id AND (s.expires_at IS NULL OR s.expires_at > now())
          JOIN plans p ON p.id = coalesce(s.plan_id, (SELECT d.id FROM plans d WHERE d.is_default))
         WHERE (COALESCE(p_q, '') = ''
            OR position(lower(p_q) in lower(t.name)) > 0
            OR position(lower(p_q) in lower(t.code)) > 0
            OR EXISTS (SELECT 1 FROM users u WHERE u.tenant_id = t.id AND position(lower(p_q) in lower(u.email)) > 0))
           AND (p_status IS NULL OR p_status = '' OR t.active = (p_status = 'active'))
           AND (p_plan IS NULL OR p.id = p_plan)
           AND (p_from IS NULL OR t.created_at >= p_from)
           AND (p_to IS NULL OR t.created_at < p_to)
    )
    SELECT b.id, b.code, b.name, b.active, b.created_at, b.outlet_count, b.user_count, b.last_login_at, b.owner_email,
           b.plan_id, b.plan_name, b.plan_expires_at, count(*) OVER ()
      FROM base b
     WHERE CASE COALESCE(p_login, '')
             WHEN 'never'   THEN b.last_login_at IS NULL
             WHEN 'stale30' THEN b.last_login_at IS NULL OR b.last_login_at < now() - interval '30 days'
             WHEN 'recent7' THEN b.last_login_at >= now() - interval '7 days'
             ELSE true END
     ORDER BY b.created_at DESC, b.id
     LIMIT LEAST(GREATEST(p_limit, 1), 200) OFFSET GREATEST(p_offset, 0);
END
$$;

CREATE OR REPLACE FUNCTION platform_admin_delete(p_actor uuid, p_target uuid) RETURNS boolean
LANGUAGE plpgsql SECURITY DEFINER SET search_path = public, pg_temp AS
$$
BEGIN
    PERFORM platform_assert_admin(p_actor);
    PERFORM pg_advisory_xact_lock(80080001);
    IF p_actor = p_target THEN
        RAISE EXCEPTION 'tidak boleh menghapus diri sendiri' USING ERRCODE = 'P0001';
    END IF;
    IF NOT EXISTS (SELECT 1 FROM platform_admins WHERE active AND id <> p_target) THEN
        RAISE EXCEPTION 'admin aktif terakhir' USING ERRCODE = 'P0001';
    END IF;
    DELETE FROM platform_admins WHERE id = p_target;
    RETURN FOUND;
END
$$;

CREATE OR REPLACE FUNCTION platform_admin_set_active(p_actor uuid, p_target uuid, p_active boolean, p_now timestamptz) RETURNS boolean
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

DROP FUNCTION platform_admin_create(uuid, text, text, text, uuid);
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

CREATE OR REPLACE FUNCTION platform_admin_bootstrap(p_email text, p_name text, p_hash text) RETURNS uuid
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
-- +goose StatementEnd

REVOKE ALL ON FUNCTION platform_admin_create(uuid, text, text, text), platform_list_tenants(uuid, text, int, int, text, uuid, text, timestamptz, timestamptz) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION platform_admin_create(uuid, text, text, text), platform_list_tenants(uuid, text, int, int, text, uuid, text, timestamptz, timestamptz) TO aciraba_app;

DROP FUNCTION platform_admin_set_role(uuid, uuid, uuid);
DROP FUNCTION platform_role_delete(uuid, uuid);
DROP FUNCTION platform_role_save(uuid, uuid, text, jsonb);
DROP FUNCTION platform_is_owner(uuid);
DROP FUNCTION platform_other_owner_exists(uuid);
DROP INDEX tenants_type_idx;
ALTER TABLE tenants DROP COLUMN location, DROP COLUMN tenant_type;
DROP INDEX platform_audit_admin_idx;
DROP TABLE platform_admin_roles;
DROP TABLE platform_roles;
