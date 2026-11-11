-- +goose Up
-- +goose StatementBegin

-- Peran host: aplikasi (app_enabled), landing page (landing), atau keduanya. Host "landing saja" mengalihkan halaman aplikasi ke /.
ALTER TABLE tenant_domains ADD COLUMN app_enabled boolean NOT NULL DEFAULT true;
ALTER TABLE tenant_domains ADD CONSTRAINT tenant_domains_role CHECK (app_enabled OR landing);

CREATE FUNCTION platform_domain_roles(p_actor uuid, p_tenant uuid, p_id uuid) RETURNS TABLE (id uuid, app_enabled boolean)
LANGUAGE plpgsql STABLE SECURITY DEFINER SET search_path = public, pg_temp AS
$$
BEGIN
    PERFORM platform_assert_admin(p_actor);
    RETURN QUERY SELECT d.id, d.app_enabled FROM tenant_domains d
     WHERE (p_tenant IS NULL OR d.tenant_id = p_tenant) AND (p_id IS NULL OR d.id = p_id);
END
$$;

-- Atur kedua peran sekaligus; (false,false) ditolak CHECK (23514).
CREATE FUNCTION platform_domain_set_role(p_actor uuid, p_id uuid, p_app boolean, p_landing boolean) RETURNS boolean
LANGUAGE plpgsql SECURITY DEFINER SET search_path = public, pg_temp AS
$$
BEGIN
    PERFORM platform_assert_admin(p_actor);
    UPDATE tenant_domains SET app_enabled = p_app, landing = p_landing WHERE id = p_id;
    RETURN FOUND;
END
$$;

-- Publik: false hanya untuk host aktif yang berperan "landing saja" (halaman aplikasi tidak boleh dilayani di sana).
CREATE FUNCTION public_domain_app_allowed(p_host text) RETURNS boolean
LANGUAGE sql STABLE SECURITY DEFINER SET search_path = public, pg_temp AS
$$
    SELECT NOT EXISTS (SELECT 1 FROM tenant_domains WHERE host = lower(p_host) AND status = 'active' AND NOT app_enabled)
$$;

-- Satu domain didaftarkan, sub-host bebas: host yang berakhiran ".<domain aktif milik tenant yang sama>" langsung aktif
-- (pemilik domain induk sudah terbukti lewat DNS); DNS sub-host tetap harus diarahkan klien (atau wildcard CNAME).
CREATE OR REPLACE FUNCTION platform_domain_add(p_actor uuid, p_tenant uuid, p_host text) RETURNS uuid
LANGUAGE plpgsql SECURITY DEFINER SET search_path = public, pg_temp AS
$$
DECLARE
    v_id uuid;
    v_sub boolean;
BEGIN
    PERFORM platform_assert_admin(p_actor);
    SELECT EXISTS (SELECT 1 FROM tenant_domains
                    WHERE tenant_id = p_tenant AND status = 'active' AND p_host LIKE '%.' || replace(replace(replace(host, '\', '\'), '%', '\%'), '_', '\_'))
      INTO v_sub;
    INSERT INTO tenant_domains (tenant_id, host, status, verified_at)
    VALUES (p_tenant, p_host, CASE WHEN v_sub THEN 'active' ELSE 'pending' END, CASE WHEN v_sub THEN now() END)
    RETURNING id INTO v_id;
    RETURN v_id;
END
$$;

REVOKE ALL ON FUNCTION platform_domain_roles(uuid, uuid, uuid), platform_domain_set_role(uuid, uuid, boolean, boolean), public_domain_app_allowed(text) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION platform_domain_roles(uuid, uuid, uuid), platform_domain_set_role(uuid, uuid, boolean, boolean), public_domain_app_allowed(text) TO aciraba_app;

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
CREATE OR REPLACE FUNCTION platform_domain_add(p_actor uuid, p_tenant uuid, p_host text) RETURNS uuid
LANGUAGE plpgsql SECURITY DEFINER SET search_path = public, pg_temp AS
$$
DECLARE
    v_id uuid;
BEGIN
    PERFORM platform_assert_admin(p_actor);
    INSERT INTO tenant_domains (tenant_id, host) VALUES (p_tenant, p_host) RETURNING id INTO v_id;
    RETURN v_id;
END
$$;
DROP FUNCTION public_domain_app_allowed(text);
DROP FUNCTION platform_domain_set_role(uuid, uuid, boolean, boolean);
DROP FUNCTION platform_domain_roles(uuid, uuid, uuid);
ALTER TABLE tenant_domains DROP CONSTRAINT tenant_domains_role;
ALTER TABLE tenant_domains DROP COLUMN app_enabled;
-- +goose StatementEnd
