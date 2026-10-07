-- +goose Up

-- "Administrator" = sifat role (izin `{"*":true}`), bukan nama/role sistem tertentu: role mana pun dengan izin `*`
-- mengakses semua menu/aksi DAN semua outlet aktif tanpa penugasan di user_outlets. Sebelumnya akses semua outlet
-- melekat pada `roles.is_system` (hanya Owner). Fungsi pencarian akun (sebelum tenant diketahui) disesuaikan;
-- sisi aplikasi (authz.Resolver) memakai aturan yang sama dari izin role.

DROP FUNCTION auth_account_by_email(text);
DROP FUNCTION auth_account_by_id(uuid, uuid);

-- +goose StatementBegin
CREATE FUNCTION auth_account_by_email(p_email text)
RETURNS TABLE (
    user_id uuid, user_name text, email text, password_hash text, user_active boolean, email_verified boolean,
    role_name text, tenant_id uuid, tenant_code text, tenant_name text, tenant_active boolean,
    outlet_id uuid, outlet_code text, outlet_name text
)
LANGUAGE sql STABLE SECURITY DEFINER SET search_path = public, pg_temp AS
$$
    SELECT u.id, u.name, u.email, u.password_hash, u.active, u.email_verified_at IS NOT NULL,
           r.name, t.id, t.code, t.name, t.active,
           o.id, o.code, o.name
    FROM users u
    JOIN roles r   ON r.tenant_id = u.tenant_id AND r.id = u.role_id
    JOIN tenants t ON t.id = u.tenant_id
    LEFT JOIN LATERAL (
        SELECT ox.id, ox.code, ox.name FROM outlets ox
        WHERE ox.tenant_id = u.tenant_id AND ox.active
          AND (COALESCE((r.permissions ->> '*')::boolean, false)
               OR EXISTS (SELECT 1 FROM user_outlets uo
                          WHERE uo.tenant_id = u.tenant_id AND uo.user_id = u.id AND uo.outlet_id = ox.id))
        ORDER BY ox.created_at, ox.code
        LIMIT 1
    ) o ON true
    WHERE lower(u.email) = lower(p_email)
$$;

CREATE FUNCTION auth_account_by_id(p_user_id uuid, p_outlet_id uuid DEFAULT NULL)
RETURNS TABLE (
    user_id uuid, user_name text, email text, password_hash text, user_active boolean, email_verified boolean,
    role_name text, tenant_id uuid, tenant_code text, tenant_name text, tenant_active boolean,
    outlet_id uuid, outlet_code text, outlet_name text
)
LANGUAGE sql STABLE SECURITY DEFINER SET search_path = public, pg_temp AS
$$
    SELECT u.id, u.name, u.email, u.password_hash, u.active, u.email_verified_at IS NOT NULL,
           r.name, t.id, t.code, t.name, t.active,
           o.id, o.code, o.name
    FROM users u
    JOIN roles r   ON r.tenant_id = u.tenant_id AND r.id = u.role_id
    JOIN tenants t ON t.id = u.tenant_id
    LEFT JOIN LATERAL (
        SELECT ox.id, ox.code, ox.name FROM outlets ox
        WHERE ox.tenant_id = u.tenant_id AND ox.active
          AND (COALESCE((r.permissions ->> '*')::boolean, false)
               OR EXISTS (SELECT 1 FROM user_outlets uo
                          WHERE uo.tenant_id = u.tenant_id AND uo.user_id = u.id AND uo.outlet_id = ox.id))
        ORDER BY COALESCE(ox.id = p_outlet_id, false) DESC, ox.created_at, ox.code
        LIMIT 1
    ) o ON true
    WHERE u.id = p_user_id
$$;
-- +goose StatementEnd

REVOKE ALL ON FUNCTION auth_account_by_email(text), auth_account_by_id(uuid, uuid) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION auth_account_by_email(text), auth_account_by_id(uuid, uuid) TO aciraba_app;

-- +goose Down
DROP FUNCTION auth_account_by_email(text);
DROP FUNCTION auth_account_by_id(uuid, uuid);

-- +goose StatementBegin
CREATE FUNCTION auth_account_by_email(p_email text)
RETURNS TABLE (
    user_id uuid, user_name text, email text, password_hash text, user_active boolean, email_verified boolean,
    role_name text, tenant_id uuid, tenant_code text, tenant_name text, tenant_active boolean,
    outlet_id uuid, outlet_code text, outlet_name text
)
LANGUAGE sql STABLE SECURITY DEFINER SET search_path = public, pg_temp AS
$$
    SELECT u.id, u.name, u.email, u.password_hash, u.active, u.email_verified_at IS NOT NULL,
           r.name, t.id, t.code, t.name, t.active,
           o.id, o.code, o.name
    FROM users u
    JOIN roles r   ON r.tenant_id = u.tenant_id AND r.id = u.role_id
    JOIN tenants t ON t.id = u.tenant_id
    LEFT JOIN LATERAL (
        SELECT ox.id, ox.code, ox.name FROM outlets ox
        WHERE ox.tenant_id = u.tenant_id AND ox.active
          AND (r.is_system OR EXISTS (SELECT 1 FROM user_outlets uo
                                      WHERE uo.tenant_id = u.tenant_id AND uo.user_id = u.id AND uo.outlet_id = ox.id))
        ORDER BY ox.created_at, ox.code
        LIMIT 1
    ) o ON true
    WHERE lower(u.email) = lower(p_email)
$$;

CREATE FUNCTION auth_account_by_id(p_user_id uuid, p_outlet_id uuid DEFAULT NULL)
RETURNS TABLE (
    user_id uuid, user_name text, email text, password_hash text, user_active boolean, email_verified boolean,
    role_name text, tenant_id uuid, tenant_code text, tenant_name text, tenant_active boolean,
    outlet_id uuid, outlet_code text, outlet_name text
)
LANGUAGE sql STABLE SECURITY DEFINER SET search_path = public, pg_temp AS
$$
    SELECT u.id, u.name, u.email, u.password_hash, u.active, u.email_verified_at IS NOT NULL,
           r.name, t.id, t.code, t.name, t.active,
           o.id, o.code, o.name
    FROM users u
    JOIN roles r   ON r.tenant_id = u.tenant_id AND r.id = u.role_id
    JOIN tenants t ON t.id = u.tenant_id
    LEFT JOIN LATERAL (
        SELECT ox.id, ox.code, ox.name FROM outlets ox
        WHERE ox.tenant_id = u.tenant_id AND ox.active
          AND (r.is_system OR EXISTS (SELECT 1 FROM user_outlets uo
                                      WHERE uo.tenant_id = u.tenant_id AND uo.user_id = u.id AND uo.outlet_id = ox.id))
        ORDER BY COALESCE(ox.id = p_outlet_id, false) DESC, ox.created_at, ox.code
        LIMIT 1
    ) o ON true
    WHERE u.id = p_user_id
$$;
-- +goose StatementEnd

REVOKE ALL ON FUNCTION auth_account_by_email(text), auth_account_by_id(uuid, uuid) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION auth_account_by_email(text), auth_account_by_id(uuid, uuid) TO aciraba_app;
