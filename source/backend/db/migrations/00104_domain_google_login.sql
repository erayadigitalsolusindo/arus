-- +goose Up
-- +goose StatementBegin

-- Login Google di domain kustom. Tiap domain boleh membawa OAuth client ID Google sendiri (project Google Cloud milik
-- tenant, origin-nya = domain itu), jadi tidak terbatas satu ID dari env. Kosong = tombol Google tidak tampil di domain itu.
ALTER TABLE tenant_domains ADD COLUMN google_client_id text NOT NULL DEFAULT ''
    CHECK (google_client_id = '' OR google_client_id ~ '^[0-9]+-[a-z0-9]+\.apps\.googleusercontent\.com$');

DROP FUNCTION platform_domain_list(uuid, uuid, uuid);
CREATE FUNCTION platform_domain_list(p_actor uuid, p_tenant uuid, p_id uuid) RETURNS TABLE (
    id uuid, tenant_id uuid, tenant_code text, tenant_name text, host text, status text,
    verified_at timestamptz, last_check_at timestamptz, last_check_error text, created_at timestamptz, landing boolean,
    google_client_id text
)
LANGUAGE plpgsql STABLE SECURITY DEFINER SET search_path = public, pg_temp AS
$$
BEGIN
    PERFORM platform_assert_admin(p_actor);
    RETURN QUERY
    SELECT d.id, d.tenant_id, t.code, t.name, d.host, d.status, d.verified_at, d.last_check_at, d.last_check_error, d.created_at,
           d.landing, d.google_client_id
      FROM tenant_domains d JOIN tenants t ON t.id = d.tenant_id
     WHERE (p_tenant IS NULL OR d.tenant_id = p_tenant) AND (p_id IS NULL OR d.id = p_id)
     ORDER BY d.created_at DESC, d.id;
END
$$;

CREATE FUNCTION platform_domain_set_google(p_actor uuid, p_id uuid, p_client_id text) RETURNS boolean
LANGUAGE plpgsql SECURITY DEFINER SET search_path = public, pg_temp AS
$$
BEGIN
    PERFORM platform_assert_admin(p_actor);
    UPDATE tenant_domains SET google_client_id = p_client_id WHERE id = p_id;
    RETURN FOUND;
END
$$;

-- Publik: client ID Google milik host aktif ini ('' = tidak ada).
CREATE FUNCTION public_domain_google(p_host text) RETURNS text
LANGUAGE sql STABLE SECURITY DEFINER SET search_path = public, pg_temp AS
$$
    SELECT COALESCE(max(d.google_client_id), '')
      FROM tenant_domains d JOIN tenants t ON t.id = d.tenant_id
     WHERE d.host = lower(p_host) AND d.status = 'active' AND t.active
$$;

-- Semua client ID domain aktif: audience tambahan yang diterima server saat memverifikasi ID token Google.
CREATE FUNCTION public_google_client_ids() RETURNS SETOF text
LANGUAGE sql STABLE SECURITY DEFINER SET search_path = public, pg_temp AS
$$
    SELECT DISTINCT d.google_client_id
      FROM tenant_domains d JOIN tenants t ON t.id = d.tenant_id
     WHERE d.google_client_id <> '' AND d.status = 'active' AND t.active
$$;

REVOKE ALL ON FUNCTION platform_domain_list(uuid, uuid, uuid), platform_domain_set_google(uuid, uuid, text), public_domain_google(text), public_google_client_ids() FROM PUBLIC;
GRANT EXECUTE ON FUNCTION platform_domain_list(uuid, uuid, uuid), platform_domain_set_google(uuid, uuid, text), public_domain_google(text), public_google_client_ids() TO aciraba_app;

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP FUNCTION public_google_client_ids();
DROP FUNCTION public_domain_google(text);
DROP FUNCTION platform_domain_set_google(uuid, uuid, text);
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
REVOKE ALL ON FUNCTION platform_domain_list(uuid, uuid, uuid) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION platform_domain_list(uuid, uuid, uuid) TO aciraba_app;
ALTER TABLE tenant_domains DROP COLUMN google_client_id;
-- +goose StatementEnd
