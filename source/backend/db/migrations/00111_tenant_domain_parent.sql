-- +goose Up
-- +goose StatementBegin

-- Domain induk otomatis: tenant yang didaftarkan admin untuk arus.kotakcantik.com boleh memakai kotakcantik.com (dan
-- sub-domainnya) SELAMA tidak ada tenant lain yang punya host di bawah kotakcantik.com. Induk dari akhiran dua-tingkat umum
-- (co.id, com.au, …) dan induk dari alamat aplikasi ARUS sendiri tidak pernah ditawarkan.
CREATE FUNCTION domain_parent(p_host text) RETURNS text
LANGUAGE sql IMMUTABLE AS
$$
    SELECT CASE WHEN position('.' in p_host) = 0 THEN NULL
                ELSE nullif(CASE WHEN position('.' in substring(p_host from position('.' in p_host) + 1)) = 0
                                   OR substring(p_host from position('.' in p_host) + 1) ~ '^(co|com|net|org|or|go|ac|web|my|sch|biz)\.[a-z]{2}$'
                                 THEN '' ELSE substring(p_host from position('.' in p_host) + 1) END, '') END
$$;

CREATE FUNCTION tenant_domain_parents(p_tenant uuid, p_target text) RETURNS TABLE (host text)
LANGUAGE sql STABLE SECURITY DEFINER SET search_path = public, pg_temp AS
$$
    SELECT DISTINCT x.pr
      FROM (SELECT domain_parent(d.host) AS pr FROM tenant_domains d
             WHERE d.tenant_id = p_tenant AND d.status = 'active' AND NOT d.tenant_made) x
     WHERE x.pr IS NOT NULL AND x.pr IS DISTINCT FROM domain_parent(p_target)
       AND NOT EXISTS (SELECT 1 FROM tenant_domains o WHERE o.tenant_id = p_tenant AND o.host = x.pr)
       AND NOT EXISTS (SELECT 1 FROM tenant_domains o WHERE o.tenant_id <> p_tenant AND (o.host = x.pr OR o.host LIKE '%.' || x.pr))
$$;

-- Tambahkan induk itu sendiri sebagai host milik tenant (langsung aktif; DNS diarahkan klien). DOMAIN_BASE bila tak ditawarkan.
CREATE FUNCTION tenant_domain_add_parent(p_tenant uuid, p_host text, p_target text) RETURNS uuid
LANGUAGE plpgsql SECURITY DEFINER SET search_path = public, pg_temp AS
$$
DECLARE
    v_id uuid;
BEGIN
    IF NOT EXISTS (SELECT 1 FROM tenant_domain_parents(p_tenant, p_target) t WHERE t.host = p_host) THEN RAISE EXCEPTION 'DOMAIN_BASE'; END IF;
    INSERT INTO tenant_domains (tenant_id, host, status, verified_at) VALUES (p_tenant, p_host, 'active', now()) RETURNING id INTO v_id;
    RETURN v_id;
END
$$;

REVOKE ALL ON FUNCTION tenant_domain_parents(uuid, text), tenant_domain_add_parent(uuid, text, text) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION tenant_domain_parents(uuid, text), tenant_domain_add_parent(uuid, text, text) TO aciraba_app;

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP FUNCTION tenant_domain_add_parent(uuid, text, text);
DROP FUNCTION tenant_domain_parents(uuid, text);
DROP FUNCTION domain_parent(text);
-- +goose StatementEnd
