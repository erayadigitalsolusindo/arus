-- +goose Up
-- +goose StatementBegin

-- Domain kustom (white label) per tenant, mis. arus.kotakcantik.com. Tenant tetap ditentukan dari token login; domain hanya
-- alamat lain ke aplikasi yang sama. Tabel milik platform: aplikasi tidak punya hak tulis/baca langsung, semua lewat fungsi
-- di bawah (admin platform) atau public_domain_lookup() (endpoint publik, tanpa konteks tenant).
CREATE TABLE tenant_domains (
    id               uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id        uuid        NOT NULL REFERENCES tenants (id) ON DELETE RESTRICT,
    host             text        NOT NULL,
    -- pending = menunggu DNS terverifikasi; active = boleh melayani + diterbitkan sertifikat TLS; disabled = dimatikan operator.
    status           text        NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'active', 'disabled')),
    verified_at      timestamptz,
    last_check_at    timestamptz,
    last_check_error text        NOT NULL DEFAULT '' CHECK (char_length(last_check_error) <= 300),
    created_at       timestamptz NOT NULL DEFAULT now(),
    updated_at       timestamptz NOT NULL DEFAULT now(),
    -- Satu host hanya boleh dimiliki satu tenant (UNIQUE global, bukan per tenant).
    CONSTRAINT tenant_domains_host_key    UNIQUE (host),
    CONSTRAINT tenant_domains_host_format CHECK (char_length(host) <= 253
        AND host ~ '^([a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?\.)+[a-z][a-z0-9-]{1,62}$')
);
CREATE INDEX tenant_domains_tenant_idx ON tenant_domains (tenant_id, created_at);
CREATE TRIGGER tenant_domains_set_updated_at BEFORE UPDATE ON tenant_domains FOR EACH ROW EXECUTE FUNCTION set_updated_at();

REVOKE ALL ON tenant_domains FROM aciraba_app;

-- Daftar domain (p_tenant / p_id NULL = tanpa filter), terbaru dulu. Tanpa keyset: jumlah domain dibatasi jumlah tenant.
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

-- Tambah domain (status awal pending). Host bentrok = unique_violation (23505), tenant tidak ada = FK (23503).
CREATE FUNCTION platform_domain_add(p_actor uuid, p_tenant uuid, p_host text) RETURNS uuid
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

-- Catat hasil verifikasi DNS. p_ok=true: pending/active -> active (verified_at diisi sekali); p_ok=false: active tetap active
-- (kegagalan sesaat tidak mematikan domain yang sudah jalan), pending tetap pending; keduanya menyimpan pesan error.
-- Domain disabled tidak disentuh. Mengembalikan false bila domain tidak ada.
CREATE FUNCTION platform_domain_mark(p_actor uuid, p_id uuid, p_ok boolean, p_error text) RETURNS boolean
LANGUAGE plpgsql SECURITY DEFINER SET search_path = public, pg_temp AS
$$
BEGIN
    PERFORM platform_assert_admin(p_actor);
    UPDATE tenant_domains
       SET status = CASE WHEN status = 'disabled' THEN status WHEN p_ok THEN 'active' ELSE status END,
           verified_at = CASE WHEN status <> 'disabled' AND p_ok THEN COALESCE(verified_at, now()) ELSE verified_at END,
           last_check_at = now(),
           last_check_error = CASE WHEN p_ok THEN '' ELSE left(p_error, 300) END
     WHERE id = p_id;
    RETURN FOUND;
END
$$;

-- Nyalakan/matikan domain secara manual. p_disabled=true -> disabled; false -> kembali ke pending (wajib verifikasi ulang).
CREATE FUNCTION platform_domain_disable(p_actor uuid, p_id uuid, p_disabled boolean) RETURNS boolean
LANGUAGE plpgsql SECURITY DEFINER SET search_path = public, pg_temp AS
$$
BEGIN
    PERFORM platform_assert_admin(p_actor);
    UPDATE tenant_domains
       SET status = CASE WHEN p_disabled THEN 'disabled' ELSE 'pending' END,
           verified_at = CASE WHEN p_disabled THEN verified_at ELSE NULL END
     WHERE id = p_id;
    RETURN FOUND;
END
$$;

CREATE FUNCTION platform_domain_delete(p_actor uuid, p_id uuid) RETURNS boolean
LANGUAGE plpgsql SECURITY DEFINER SET search_path = public, pg_temp AS
$$
BEGIN
    PERFORM platform_assert_admin(p_actor);
    DELETE FROM tenant_domains WHERE id = p_id;
    RETURN FOUND;
END
$$;

-- Publik (tanpa konteks tenant): host aktif milik tenant aktif -> identitas minimal untuk branding & izin sertifikat TLS.
CREATE FUNCTION public_domain_lookup(p_host text) RETURNS TABLE (tenant_code text, tenant_name text)
LANGUAGE sql STABLE SECURITY DEFINER SET search_path = public, pg_temp AS
$$
    SELECT t.code, t.name
      FROM tenant_domains d JOIN tenants t ON t.id = d.tenant_id
     WHERE d.host = lower(p_host) AND d.status = 'active' AND t.active
$$;

REVOKE ALL ON FUNCTION
    platform_domain_list(uuid, uuid, uuid),
    platform_domain_add(uuid, uuid, text),
    platform_domain_mark(uuid, uuid, boolean, text),
    platform_domain_disable(uuid, uuid, boolean),
    platform_domain_delete(uuid, uuid),
    public_domain_lookup(text)
FROM PUBLIC;
GRANT EXECUTE ON FUNCTION
    platform_domain_list(uuid, uuid, uuid),
    platform_domain_add(uuid, uuid, text),
    platform_domain_mark(uuid, uuid, boolean, text),
    platform_domain_disable(uuid, uuid, boolean),
    platform_domain_delete(uuid, uuid),
    public_domain_lookup(text)
TO aciraba_app;

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP FUNCTION public_domain_lookup(text);
DROP FUNCTION platform_domain_delete(uuid, uuid);
DROP FUNCTION platform_domain_disable(uuid, uuid, boolean);
DROP FUNCTION platform_domain_mark(uuid, uuid, boolean, text);
DROP FUNCTION platform_domain_add(uuid, uuid, text);
DROP FUNCTION platform_domain_list(uuid, uuid, uuid);
DROP TABLE tenant_domains;
-- +goose StatementEnd
