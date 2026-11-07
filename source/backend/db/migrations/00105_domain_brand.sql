-- +goose Up
-- +goose StatementBegin

-- White label penuh per domain kustom: nama aplikasi, teks footer, logo penuh, dan ikon (juga favicon). Gambar kecil disimpan
-- di database (maks 512 KB) agar ikut backup dan tidak butuh penyimpanan berkas. Hanya admin platform yang mengatur.
ALTER TABLE tenant_domains
    ADD COLUMN brand_name   text NOT NULL DEFAULT '' CHECK (char_length(brand_name) <= 60),
    ADD COLUMN brand_footer text NOT NULL DEFAULT '' CHECK (char_length(brand_footer) <= 200),
    ADD COLUMN brand_ver    bigint NOT NULL DEFAULT 0; -- naik tiap perubahan gambar (cache-buster URL)

CREATE TABLE tenant_domain_assets (
    domain_id    uuid        NOT NULL REFERENCES tenant_domains (id) ON DELETE CASCADE,
    kind         text        NOT NULL CHECK (kind IN ('logo', 'icon')),
    content_type text        NOT NULL,
    data         bytea       NOT NULL CHECK (octet_length(data) <= 524288),
    updated_at   timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (domain_id, kind)
);
REVOKE ALL ON tenant_domain_assets FROM aciraba_app;

DROP FUNCTION platform_domain_list(uuid, uuid, uuid);
CREATE FUNCTION platform_domain_list(p_actor uuid, p_tenant uuid, p_id uuid) RETURNS TABLE (
    id uuid, tenant_id uuid, tenant_code text, tenant_name text, host text, status text,
    verified_at timestamptz, last_check_at timestamptz, last_check_error text, created_at timestamptz, landing boolean,
    google_client_id text, brand_name text, brand_footer text, brand_ver bigint, has_logo boolean, has_icon boolean
)
LANGUAGE plpgsql STABLE SECURITY DEFINER SET search_path = public, pg_temp AS
$$
BEGIN
    PERFORM platform_assert_admin(p_actor);
    RETURN QUERY
    SELECT d.id, d.tenant_id, t.code, t.name, d.host, d.status, d.verified_at, d.last_check_at, d.last_check_error, d.created_at,
           d.landing, d.google_client_id, d.brand_name, d.brand_footer, d.brand_ver,
           EXISTS (SELECT 1 FROM tenant_domain_assets a WHERE a.domain_id = d.id AND a.kind = 'logo'),
           EXISTS (SELECT 1 FROM tenant_domain_assets a WHERE a.domain_id = d.id AND a.kind = 'icon')
      FROM tenant_domains d JOIN tenants t ON t.id = d.tenant_id
     WHERE (p_tenant IS NULL OR d.tenant_id = p_tenant) AND (p_id IS NULL OR d.id = p_id)
     ORDER BY d.created_at DESC, d.id;
END
$$;

CREATE FUNCTION platform_domain_set_brand(p_actor uuid, p_id uuid, p_name text, p_footer text) RETURNS boolean
LANGUAGE plpgsql SECURITY DEFINER SET search_path = public, pg_temp AS
$$
BEGIN
    PERFORM platform_assert_admin(p_actor);
    UPDATE tenant_domains SET brand_name = p_name, brand_footer = p_footer WHERE id = p_id;
    RETURN FOUND;
END
$$;

-- p_data NULL = hapus gambar.
CREATE FUNCTION platform_domain_set_asset(p_actor uuid, p_id uuid, p_kind text, p_type text, p_data bytea) RETURNS boolean
LANGUAGE plpgsql SECURITY DEFINER SET search_path = public, pg_temp AS
$$
BEGIN
    PERFORM platform_assert_admin(p_actor);
    IF NOT EXISTS (SELECT 1 FROM tenant_domains WHERE id = p_id) THEN
        RETURN false;
    END IF;
    IF p_data IS NULL THEN
        DELETE FROM tenant_domain_assets WHERE domain_id = p_id AND kind = p_kind;
    ELSE
        INSERT INTO tenant_domain_assets (domain_id, kind, content_type, data) VALUES (p_id, p_kind, p_type, p_data)
        ON CONFLICT (domain_id, kind) DO UPDATE SET content_type = EXCLUDED.content_type, data = EXCLUDED.data, updated_at = now();
    END IF;
    UPDATE tenant_domains SET brand_ver = (extract(epoch FROM clock_timestamp()) * 1000)::bigint WHERE id = p_id;
    RETURN true;
END
$$;

-- Publik: tampilan white label host aktif ini (kosong = pakai tampilan ARUS).
CREATE FUNCTION public_domain_brand(p_host text) RETURNS TABLE (brand_name text, brand_footer text, brand_ver bigint, has_logo boolean, has_icon boolean)
LANGUAGE sql STABLE SECURITY DEFINER SET search_path = public, pg_temp AS
$$
    SELECT d.brand_name, d.brand_footer, d.brand_ver,
           EXISTS (SELECT 1 FROM tenant_domain_assets a WHERE a.domain_id = d.id AND a.kind = 'logo'),
           EXISTS (SELECT 1 FROM tenant_domain_assets a WHERE a.domain_id = d.id AND a.kind = 'icon')
      FROM tenant_domains d JOIN tenants t ON t.id = d.tenant_id
     WHERE d.host = lower(p_host) AND d.status = 'active' AND t.active
$$;

CREATE FUNCTION public_domain_asset(p_host text, p_kind text) RETURNS TABLE (content_type text, data bytea, updated_at timestamptz)
LANGUAGE sql STABLE SECURITY DEFINER SET search_path = public, pg_temp AS
$$
    SELECT a.content_type, a.data, a.updated_at
      FROM tenant_domains d JOIN tenants t ON t.id = d.tenant_id JOIN tenant_domain_assets a ON a.domain_id = d.id
     WHERE d.host = lower(p_host) AND d.status = 'active' AND t.active AND a.kind = p_kind
$$;

REVOKE ALL ON FUNCTION platform_domain_list(uuid, uuid, uuid), platform_domain_set_brand(uuid, uuid, text, text),
    platform_domain_set_asset(uuid, uuid, text, text, bytea), public_domain_brand(text), public_domain_asset(text, text) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION platform_domain_list(uuid, uuid, uuid), platform_domain_set_brand(uuid, uuid, text, text),
    platform_domain_set_asset(uuid, uuid, text, text, bytea), public_domain_brand(text), public_domain_asset(text, text) TO aciraba_app;

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP FUNCTION public_domain_asset(text, text);
DROP FUNCTION public_domain_brand(text);
DROP FUNCTION platform_domain_set_asset(uuid, uuid, text, text, bytea);
DROP FUNCTION platform_domain_set_brand(uuid, uuid, text, text);
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
REVOKE ALL ON FUNCTION platform_domain_list(uuid, uuid, uuid) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION platform_domain_list(uuid, uuid, uuid) TO aciraba_app;
DROP TABLE tenant_domain_assets;
ALTER TABLE tenant_domains DROP COLUMN brand_name, DROP COLUMN brand_footer, DROP COLUMN brand_ver;
-- +goose StatementEnd
