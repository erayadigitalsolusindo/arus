-- +goose Up
-- Daftar tenant platform: filter lanjutan (status, paket efektif, login terakhir, rentang tanggal daftar) + kolom paket.
-- Parameter baru ber-DEFAULT agar pemanggilan 4 argumen lama tetap berlaku.
DROP FUNCTION platform_list_tenants(uuid, text, int, int);

-- +goose StatementBegin
CREATE FUNCTION platform_list_tenants(
    p_admin uuid, p_q text, p_limit int, p_offset int,
    p_status text DEFAULT NULL,      -- 'active' | 'inactive'
    p_plan uuid DEFAULT NULL,        -- paket efektif
    p_login text DEFAULT NULL,       -- 'never' | 'stale30' (tidak login >30 hari atau belum pernah) | 'recent7'
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
-- +goose StatementEnd

REVOKE ALL ON FUNCTION platform_list_tenants(uuid, text, int, int, text, uuid, text, timestamptz, timestamptz) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION platform_list_tenants(uuid, text, int, int, text, uuid, text, timestamptz, timestamptz) TO aciraba_app;

-- +goose Down
DROP FUNCTION platform_list_tenants(uuid, text, int, int, text, uuid, text, timestamptz, timestamptz);
-- +goose StatementBegin
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
-- +goose StatementEnd
GRANT EXECUTE ON FUNCTION platform_list_tenants(uuid, text, int, int) TO aciraba_app;
