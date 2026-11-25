-- +goose Up
-- Platform Admin boleh dibatasi ke tenant tertentu: platform_admins.tenant_scope (NULL = semua tenant, array = hanya itu).
-- Daftar tenant, jenis, dan lokasi tenant dibatasi di DB; rute per-tenant dijaga middleware (platformadmin/scope.go).
ALTER TABLE platform_admins ADD COLUMN tenant_scope uuid[];

DROP FUNCTION platform_list_tenants(uuid, text, int, int, text, uuid, text, timestamptz, timestamptz, text);
-- +goose StatementBegin
CREATE FUNCTION platform_list_tenants(
    p_admin uuid, p_q text, p_limit int, p_offset int,
    p_status text DEFAULT NULL, p_plan uuid DEFAULT NULL, p_login text DEFAULT NULL,
    p_from timestamptz DEFAULT NULL, p_to timestamptz DEFAULT NULL,
    p_type text DEFAULT NULL, p_location text DEFAULT NULL
)
RETURNS TABLE (
    id uuid, code text, name text, active boolean, created_at timestamptz,
    outlet_count bigint, user_count bigint, last_login_at timestamptz, owner_email text,
    plan_id uuid, plan_name text, plan_expires_at timestamptz,
    tenant_type text, location text, total bigint
)
LANGUAGE plpgsql STABLE SECURITY DEFINER SET search_path = public, pg_temp AS
$$
DECLARE v_scope uuid[];
BEGIN
    PERFORM platform_assert_admin(p_admin);
    SELECT a.tenant_scope INTO v_scope FROM platform_admins a WHERE a.id = p_admin;
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
           AND (COALESCE(p_location, '') = '' OR lower(t.location) = lower(p_location))
           AND (v_scope IS NULL OR t.id = ANY (v_scope))
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

CREATE OR REPLACE FUNCTION platform_tenant_types(p_admin uuid) RETURNS TABLE (tenant_type text, n bigint)
LANGUAGE plpgsql STABLE SECURITY DEFINER SET search_path = public, pg_temp AS
$$
DECLARE v_scope uuid[];
BEGIN
    PERFORM platform_assert_admin(p_admin);
    SELECT a.tenant_scope INTO v_scope FROM platform_admins a WHERE a.id = p_admin;
    RETURN QUERY
    SELECT min(t.tenant_type), count(*) FROM tenants t WHERE t.tenant_type <> '' AND (v_scope IS NULL OR t.id = ANY (v_scope))
     GROUP BY lower(t.tenant_type) ORDER BY count(*) DESC, lower(t.tenant_type) LIMIT 200;
END
$$;

-- Lokasi tenant yang sudah dipakai (bahan filter), sama bentuknya dengan jenis tenant.
CREATE FUNCTION platform_tenant_locations(p_admin uuid) RETURNS TABLE (location text, n bigint)
LANGUAGE plpgsql STABLE SECURITY DEFINER SET search_path = public, pg_temp AS
$$
DECLARE v_scope uuid[];
BEGIN
    PERFORM platform_assert_admin(p_admin);
    SELECT a.tenant_scope INTO v_scope FROM platform_admins a WHERE a.id = p_admin;
    RETURN QUERY
    SELECT min(t.location), count(*) FROM tenants t WHERE t.location <> '' AND (v_scope IS NULL OR t.id = ANY (v_scope))
     GROUP BY lower(t.location) ORDER BY count(*) DESC, lower(t.location) LIMIT 200;
END
$$;

-- Mengatur cakupan tenant admin lain. mode: 'all' (hapus batas) | 'set' | 'add' | 'remove'. Pemanggil harus admin aktif TANPA batas
-- (admin berbatas tidak boleh melonggarkan batas siapa pun). Mengembalikan false bila target tidak ada.
CREATE FUNCTION platform_admin_set_scope(p_actor uuid, p_target uuid, p_mode text, p_ids uuid[]) RETURNS boolean
LANGUAGE plpgsql SECURITY DEFINER SET search_path = public, pg_temp AS
$$
BEGIN
    PERFORM platform_assert_admin(p_actor);
    IF EXISTS (SELECT 1 FROM platform_admins WHERE id = p_actor AND tenant_scope IS NOT NULL) THEN
        RAISE EXCEPTION 'admin berbatas tidak boleh mengatur cakupan' USING ERRCODE = '42501';
    END IF;
    p_ids := COALESCE(p_ids, '{}');
    UPDATE platform_admins SET tenant_scope = CASE p_mode
        WHEN 'all'    THEN NULL
        WHEN 'set'    THEN ARRAY(SELECT DISTINCT x FROM unnest(p_ids) x)
        WHEN 'add'    THEN ARRAY(SELECT DISTINCT x FROM unnest(COALESCE(tenant_scope, '{}') || p_ids) x)
        WHEN 'remove' THEN CASE WHEN tenant_scope IS NULL THEN NULL ELSE ARRAY(SELECT x FROM unnest(tenant_scope) x WHERE x <> ALL (p_ids)) END
        ELSE tenant_scope END
     WHERE id = p_target;
    RETURN FOUND;
END
$$;
-- +goose StatementEnd

REVOKE ALL ON FUNCTION platform_list_tenants(uuid, text, int, int, text, uuid, text, timestamptz, timestamptz, text, text),
    platform_tenant_locations(uuid), platform_admin_set_scope(uuid, uuid, text, uuid[]) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION platform_list_tenants(uuid, text, int, int, text, uuid, text, timestamptz, timestamptz, text, text),
    platform_tenant_locations(uuid), platform_admin_set_scope(uuid, uuid, text, uuid[]) TO aciraba_app;

-- +goose Down
-- +goose StatementBegin
DROP FUNCTION platform_admin_set_scope(uuid, uuid, text, uuid[]);
DROP FUNCTION platform_tenant_locations(uuid);
CREATE OR REPLACE FUNCTION platform_tenant_types(p_admin uuid) RETURNS TABLE (tenant_type text, n bigint)
LANGUAGE plpgsql STABLE SECURITY DEFINER SET search_path = public, pg_temp AS
$$
BEGIN
    PERFORM platform_assert_admin(p_admin);
    RETURN QUERY
    SELECT min(t.tenant_type), count(*) FROM tenants t WHERE t.tenant_type <> ''
     GROUP BY lower(t.tenant_type) ORDER BY count(*) DESC, lower(t.tenant_type) LIMIT 200;
END
$$;
DROP FUNCTION platform_list_tenants(uuid, text, int, int, text, uuid, text, timestamptz, timestamptz, text, text);
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
-- +goose StatementEnd
REVOKE ALL ON FUNCTION platform_list_tenants(uuid, text, int, int, text, uuid, text, timestamptz, timestamptz, text) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION platform_list_tenants(uuid, text, int, int, text, uuid, text, timestamptz, timestamptz, text) TO aciraba_app;
ALTER TABLE platform_admins DROP COLUMN tenant_scope;
