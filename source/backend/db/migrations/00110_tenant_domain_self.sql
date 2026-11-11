-- +goose Up
-- +goose StatementBegin

-- Tenant memilih sendiri host landing/aplikasi dari domain yang sudah didaftarkan & diaktifkan Platform Admin. Tenant TIDAK
-- bisa mendaftarkan domain baru: hanya menambah sub-host di bawah domain aktifnya (tenant_made = true), mengatur peran host
-- aktifnya, dan menghapus sub-host buatannya sendiri. Tenant dari token server (p_tenant), bukan dari body request.
ALTER TABLE tenant_domains ADD COLUMN tenant_made boolean NOT NULL DEFAULT false;

CREATE FUNCTION tenant_domain_list(p_tenant uuid) RETURNS TABLE (id uuid, host text, status text, app_enabled boolean, landing boolean, tenant_made boolean)
LANGUAGE sql STABLE SECURITY DEFINER SET search_path = public, pg_temp AS
$$
    SELECT d.id, d.host, d.status, d.app_enabled, d.landing, d.tenant_made
      FROM tenant_domains d WHERE d.tenant_id = p_tenant ORDER BY d.tenant_made, d.host
$$;

-- Tambah sub-host label.<p_base>. p_base wajib domain AKTIF milik tenant yang didaftarkan admin (bukan buatan tenant).
-- Galat: DOMAIN_BASE (domain induk tidak sah), DOMAIN_LABEL (label tak valid), DOMAIN_LIMIT (maks 10), 23505 (host dipakai).
CREATE FUNCTION tenant_domain_add_sub(p_tenant uuid, p_base text, p_label text) RETURNS uuid
LANGUAGE plpgsql SECURITY DEFINER SET search_path = public, pg_temp AS
$$
DECLARE
    v_id uuid;
BEGIN
    IF p_label !~ '^[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?$' THEN RAISE EXCEPTION 'DOMAIN_LABEL'; END IF;
    IF NOT EXISTS (SELECT 1 FROM tenant_domains WHERE tenant_id = p_tenant AND host = p_base AND status = 'active' AND NOT tenant_made) THEN
        RAISE EXCEPTION 'DOMAIN_BASE';
    END IF;
    IF (SELECT count(*) FROM tenant_domains WHERE tenant_id = p_tenant AND tenant_made) >= 10 THEN RAISE EXCEPTION 'DOMAIN_LIMIT'; END IF;
    INSERT INTO tenant_domains (tenant_id, host, status, verified_at, tenant_made)
    VALUES (p_tenant, p_label || '.' || p_base, 'active', now(), true) RETURNING id INTO v_id;
    RETURN v_id;
END
$$;

-- Peran host aktif milik tenant. false jika bukan miliknya / belum aktif; (false,false) ditolak CHECK (23514).
CREATE FUNCTION tenant_domain_set_role(p_tenant uuid, p_id uuid, p_app boolean, p_landing boolean) RETURNS boolean
LANGUAGE plpgsql SECURITY DEFINER SET search_path = public, pg_temp AS
$$
BEGIN
    UPDATE tenant_domains SET app_enabled = p_app, landing = p_landing WHERE id = p_id AND tenant_id = p_tenant AND status = 'active';
    RETURN FOUND;
END
$$;

CREATE FUNCTION tenant_domain_delete_sub(p_tenant uuid, p_id uuid) RETURNS boolean
LANGUAGE plpgsql SECURITY DEFINER SET search_path = public, pg_temp AS
$$
BEGIN
    DELETE FROM tenant_domains WHERE id = p_id AND tenant_id = p_tenant AND tenant_made;
    RETURN FOUND;
END
$$;

REVOKE ALL ON FUNCTION tenant_domain_list(uuid), tenant_domain_add_sub(uuid, text, text), tenant_domain_set_role(uuid, uuid, boolean, boolean), tenant_domain_delete_sub(uuid, uuid) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION tenant_domain_list(uuid), tenant_domain_add_sub(uuid, text, text), tenant_domain_set_role(uuid, uuid, boolean, boolean), tenant_domain_delete_sub(uuid, uuid) TO aciraba_app;

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP FUNCTION tenant_domain_delete_sub(uuid, uuid);
DROP FUNCTION tenant_domain_set_role(uuid, uuid, boolean, boolean);
DROP FUNCTION tenant_domain_add_sub(uuid, text, text);
DROP FUNCTION tenant_domain_list(uuid);
ALTER TABLE tenant_domains DROP COLUMN tenant_made;
-- +goose StatementEnd
