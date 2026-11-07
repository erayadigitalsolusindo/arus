-- +goose Up
-- +goose StatementBegin

-- Domain kustom bisa menampilkan landing page tenant (storefronts) di root-nya (https://kotakcantik.com/), bukan SPA.
-- Hanya admin platform yang mengatur; tenant tidak punya akses ke tabel ini.
ALTER TABLE tenant_domains ADD COLUMN landing boolean NOT NULL DEFAULT false;

DROP FUNCTION platform_domain_list(uuid, uuid, uuid);
CREATE FUNCTION platform_domain_list(p_actor uuid, p_tenant uuid, p_id uuid) RETURNS TABLE (
    id uuid, tenant_id uuid, tenant_code text, tenant_name text, host text, status text,
    verified_at timestamptz, last_check_at timestamptz, last_check_error text, created_at timestamptz, landing boolean
)
LANGUAGE plpgsql STABLE SECURITY DEFINER SET search_path = public, pg_temp AS
$$
BEGIN
    PERFORM platform_assert_admin(p_actor);
    RETURN QUERY
    SELECT d.id, d.tenant_id, t.code, t.name, d.host, d.status, d.verified_at, d.last_check_at, d.last_check_error, d.created_at, d.landing
      FROM tenant_domains d JOIN tenants t ON t.id = d.tenant_id
     WHERE (p_tenant IS NULL OR d.tenant_id = p_tenant) AND (p_id IS NULL OR d.id = p_id)
     ORDER BY d.created_at DESC, d.id;
END
$$;

CREATE FUNCTION platform_domain_set_landing(p_actor uuid, p_id uuid, p_on boolean) RETURNS boolean
LANGUAGE plpgsql SECURITY DEFINER SET search_path = public, pg_temp AS
$$
BEGIN
    PERFORM platform_assert_admin(p_actor);
    UPDATE tenant_domains SET landing = p_on WHERE id = p_id;
    RETURN FOUND;
END
$$;

-- Publik (tanpa konteks tenant): slug landing page yang harus tampil di root host ini. Kosong = tidak ada (jatuh ke aplikasi).
CREATE FUNCTION public_domain_landing(p_host text) RETURNS text
LANGUAGE sql STABLE SECURITY DEFINER SET search_path = public, pg_temp AS
$$
    SELECT s.slug
      FROM tenant_domains d
      JOIN tenants t ON t.id = d.tenant_id
      JOIN storefronts s ON s.tenant_id = d.tenant_id
     WHERE d.host = lower(p_host) AND d.status = 'active' AND d.landing AND t.active AND s.enabled
$$;

REVOKE ALL ON FUNCTION platform_domain_list(uuid, uuid, uuid), platform_domain_set_landing(uuid, uuid, boolean), public_domain_landing(text) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION platform_domain_list(uuid, uuid, uuid), platform_domain_set_landing(uuid, uuid, boolean), public_domain_landing(text) TO aciraba_app;

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP FUNCTION public_domain_landing(text);
DROP FUNCTION platform_domain_set_landing(uuid, uuid, boolean);
DROP FUNCTION platform_domain_list(uuid, uuid, uuid);
CREATE FUNCTION platform_domain_list(p_actor uuid, p_tenant uuid, p_id uuid) RETURNS TABLE (
    id uuid, tenant_id uuid, tenant_code text, tenant_name text, host text, status text,
    verified_at timestamptz, last_check_at timestamptz, last_check_error text, created_at timestamptz
)
LANGUAGE plpgsql STABLE SECURITY DEFINER SET search_path = public, pg_temp AS
$$
BEGIN
    PERFORM platform_assert_admin(p_actor);
    RETURN QUERY
    SELECT d.id, d.tenant_id, t.code, t.name, d.host, d.status, d.verified_at, d.last_check_at, d.last_check_error, d.created_at
      FROM tenant_domains d JOIN tenants t ON t.id = d.tenant_id
     WHERE (p_tenant IS NULL OR d.tenant_id = p_tenant) AND (p_id IS NULL OR d.id = p_id)
     ORDER BY d.created_at DESC, d.id;
END
$$;
REVOKE ALL ON FUNCTION platform_domain_list(uuid, uuid, uuid) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION platform_domain_list(uuid, uuid, uuid) TO aciraba_app;
ALTER TABLE tenant_domains DROP COLUMN landing;
-- +goose StatementEnd
