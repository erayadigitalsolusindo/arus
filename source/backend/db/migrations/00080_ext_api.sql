-- +goose Up

-- API eksternal (gateway HMAC) + portal dokumentasi white label. Semua dikelola Platform Admin.
--
-- Kredensial API = pasangan key_id (publik) + secret (HMAC, terenkripsi AES-GCM di kolom secret_enc). Setiap kredensial
-- terikat pada SATU akun layanan (user) dan SATU outlet di tenant; izinnya = izin role user itu (dibaca terkini dari DB
-- oleh authz.Authenticate), jadi pencabutan/penurunan role langsung berlaku.
--
-- Tabel TIDAK diberi hak apa pun bagi aciraba_app: seluruh akses lewat fungsi SECURITY DEFINER di bawah (pola yang sama
-- dengan tabel platform_*). RLS tetap dinyalakan sebagai lapis kedua sesuai aturan tabel bertenant.

CREATE TABLE api_credentials (
    id              uuid        PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id       uuid        NOT NULL REFERENCES tenants (id) ON DELETE CASCADE,
    user_id         uuid        NOT NULL,
    outlet_id       uuid        NOT NULL,
    label           text        NOT NULL,
    key_id          text        NOT NULL,
    secret_enc      text        NOT NULL,
    prev_secret_enc text,                         -- secret lama tetap sah sampai prev_expires_at (rotasi tanpa putus layanan)
    prev_expires_at timestamptz,
    ip_allowlist    text[]      NOT NULL DEFAULT '{}',   -- IP / CIDR; kosong = semua IP ditolak (wajib diisi)
    rate_limit_min  int         NOT NULL DEFAULT 120,
    active          boolean     NOT NULL DEFAULT true,
    expires_at      timestamptz,
    last_used_at    timestamptz,
    last_used_ip    text,
    created_by      uuid,
    created_at      timestamptz NOT NULL DEFAULT now(),
    updated_at      timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT api_credentials_key_id_key UNIQUE (key_id),
    CONSTRAINT api_credentials_user_fk   FOREIGN KEY (tenant_id, user_id)   REFERENCES users   (tenant_id, id) ON DELETE CASCADE,
    CONSTRAINT api_credentials_outlet_fk FOREIGN KEY (tenant_id, outlet_id) REFERENCES outlets (tenant_id, id) ON DELETE CASCADE,
    CONSTRAINT api_credentials_label_len CHECK (char_length(label) BETWEEN 1 AND 100),
    CONSTRAINT api_credentials_rate_chk  CHECK (rate_limit_min BETWEEN 1 AND 100000)
);
CREATE INDEX api_credentials_tenant_idx ON api_credentials (tenant_id, created_at DESC);
CREATE TRIGGER api_credentials_set_updated_at BEFORE UPDATE ON api_credentials
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();
ALTER TABLE api_credentials ENABLE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON api_credentials USING (tenant_id = app_tenant_id()) WITH CHECK (tenant_id = app_tenant_id());
REVOKE ALL ON api_credentials FROM aciraba_app;

-- Profil white label portal dokumentasi. Dipilih menurut Host permintaan (domain khusus); tanpa kecocokan dipakai profil bawaan.
CREATE TABLE api_doc_brandings (
    id              uuid        PRIMARY KEY DEFAULT gen_random_uuid(),
    slug            text        NOT NULL,
    domain          text,                        -- mis. docs.mitra.com (huruf kecil, tanpa port)
    is_default      boolean     NOT NULL DEFAULT false,
    active          boolean     NOT NULL DEFAULT true,
    name            text        NOT NULL,        -- nama produk di header
    tagline         text        NOT NULL DEFAULT '',
    logo_url        text        NOT NULL DEFAULT '',
    accent          text        NOT NULL DEFAULT '#06b6d4',
    support_email   text        NOT NULL DEFAULT '',
    support_url     text        NOT NULL DEFAULT '',
    footer          text        NOT NULL DEFAULT '',
    base_url        text        NOT NULL DEFAULT '',   -- alamat API yang ditampilkan di contoh (mis. https://api.mitra.com)
    hidden_modules  text[]      NOT NULL DEFAULT '{}', -- grup endpoint yang disembunyikan dari portal ini
    sandbox_note    text        NOT NULL DEFAULT '',
    created_at      timestamptz NOT NULL DEFAULT now(),
    updated_at      timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT api_doc_brandings_slug_chk   CHECK (slug ~ '^[a-z0-9][a-z0-9-]{0,40}$'),
    CONSTRAINT api_doc_brandings_domain_chk CHECK (domain IS NULL OR domain ~ '^[a-z0-9]([a-z0-9.-]{0,251}[a-z0-9])?$'),
    CONSTRAINT api_doc_brandings_accent_chk CHECK (accent ~ '^#[0-9a-fA-F]{6}$'),
    CONSTRAINT api_doc_brandings_name_len   CHECK (char_length(name) BETWEEN 1 AND 80),
    CONSTRAINT api_doc_brandings_logo_len   CHECK (char_length(logo_url) <= 300000)
);
CREATE UNIQUE INDEX api_doc_brandings_slug_key   ON api_doc_brandings (slug);
CREATE UNIQUE INDEX api_doc_brandings_domain_key ON api_doc_brandings (domain) WHERE domain IS NOT NULL;
CREATE UNIQUE INDEX api_doc_brandings_default_key ON api_doc_brandings (is_default) WHERE is_default;
CREATE TRIGGER api_doc_brandings_set_updated_at BEFORE UPDATE ON api_doc_brandings
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();
REVOKE ALL ON api_doc_brandings FROM aciraba_app;

INSERT INTO api_doc_brandings (slug, is_default, name, tagline, footer)
VALUES ('default', true, 'ARUS API', 'Integrasikan ARUS ke aplikasi Anda dengan REST API yang aman.', '');

-- ---- fungsi Platform Admin ----

-- +goose StatementBegin
-- Pilihan akun layanan & outlet sebuah tenant untuk form kredensial.
CREATE FUNCTION platform_ext_tenant_actors(p_actor uuid, p_tenant uuid) RETURNS jsonb
LANGUAGE plpgsql STABLE SECURITY DEFINER SET search_path = public, pg_temp AS
$$
BEGIN
    PERFORM platform_assert_admin(p_actor);
    RETURN jsonb_build_object(
        'users', COALESCE((SELECT jsonb_agg(jsonb_build_object('id', u.id, 'name', u.name, 'email', u.email, 'role', r.name) ORDER BY u.name)
                             FROM users u JOIN roles r ON r.tenant_id = u.tenant_id AND r.id = u.role_id
                            WHERE u.tenant_id = p_tenant AND u.active), '[]'::jsonb),
        'outlets', COALESCE((SELECT jsonb_agg(jsonb_build_object('id', o.id, 'name', o.name) ORDER BY o.name)
                               FROM outlets o WHERE o.tenant_id = p_tenant AND o.active), '[]'::jsonb));
END
$$;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE FUNCTION platform_ext_cred_create(p_actor uuid, p_tenant uuid, p_user uuid, p_outlet uuid, p_label text,
    p_key_id text, p_secret_enc text, p_ips text[], p_rate int, p_expires timestamptz) RETURNS uuid
LANGUAGE plpgsql SECURITY DEFINER SET search_path = public, pg_temp AS
$$
DECLARE v_id uuid;
BEGIN
    PERFORM platform_assert_admin(p_actor);
    IF NOT EXISTS (SELECT 1 FROM users WHERE tenant_id = p_tenant AND id = p_user AND active) THEN
        RAISE EXCEPTION 'akun layanan tidak valid' USING ERRCODE = 'P0002';
    END IF;
    IF NOT EXISTS (SELECT 1 FROM outlets WHERE tenant_id = p_tenant AND id = p_outlet AND active) THEN
        RAISE EXCEPTION 'outlet tidak valid' USING ERRCODE = 'P0002';
    END IF;
    INSERT INTO api_credentials (tenant_id, user_id, outlet_id, label, key_id, secret_enc, ip_allowlist, rate_limit_min, expires_at, created_by)
    VALUES (p_tenant, p_user, p_outlet, p_label, p_key_id, p_secret_enc, p_ips, p_rate, p_expires, p_actor)
    RETURNING id INTO v_id;
    RETURN v_id;
END
$$;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE FUNCTION platform_ext_cred_list(p_actor uuid, p_tenant uuid) RETURNS TABLE (
    id uuid, tenant_id uuid, tenant_name text, user_id uuid, user_name text, outlet_id uuid, outlet_name text,
    label text, key_id text, ip_allowlist text[], rate_limit_min int, active boolean, expires_at timestamptz,
    last_used_at timestamptz, last_used_ip text, rotating boolean, created_at timestamptz)
LANGUAGE plpgsql STABLE SECURITY DEFINER SET search_path = public, pg_temp AS
$$
BEGIN
    PERFORM platform_assert_admin(p_actor);
    RETURN QUERY
    SELECT c.id, c.tenant_id, t.name, c.user_id, u.name, c.outlet_id, o.name,
           c.label, c.key_id, c.ip_allowlist, c.rate_limit_min, c.active, c.expires_at,
           c.last_used_at, c.last_used_ip, (c.prev_secret_enc IS NOT NULL AND c.prev_expires_at > now()), c.created_at
      FROM api_credentials c
      JOIN tenants t ON t.id = c.tenant_id
      JOIN users u   ON u.tenant_id = c.tenant_id AND u.id = c.user_id
      JOIN outlets o ON o.tenant_id = c.tenant_id AND o.id = c.outlet_id
     WHERE p_tenant IS NULL OR c.tenant_id = p_tenant
     ORDER BY c.created_at DESC, c.id
     LIMIT 500;
END
$$;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE FUNCTION platform_ext_cred_update(p_actor uuid, p_id uuid, p_label text, p_ips text[], p_rate int, p_active boolean, p_expires timestamptz) RETURNS boolean
LANGUAGE plpgsql SECURITY DEFINER SET search_path = public, pg_temp AS
$$
BEGIN
    PERFORM platform_assert_admin(p_actor);
    UPDATE api_credentials SET label = p_label, ip_allowlist = p_ips, rate_limit_min = p_rate, active = p_active, expires_at = p_expires
     WHERE id = p_id;
    RETURN FOUND;
END
$$;
-- +goose StatementEnd

-- +goose StatementBegin
-- Rotasi: secret lama tetap sah sampai p_grace_until, secret baru langsung sah.
CREATE FUNCTION platform_ext_cred_rotate(p_actor uuid, p_id uuid, p_secret_enc text, p_grace_until timestamptz) RETURNS boolean
LANGUAGE plpgsql SECURITY DEFINER SET search_path = public, pg_temp AS
$$
BEGIN
    PERFORM platform_assert_admin(p_actor);
    UPDATE api_credentials
       SET prev_secret_enc = CASE WHEN p_grace_until IS NULL THEN NULL ELSE secret_enc END,
           prev_expires_at = p_grace_until, secret_enc = p_secret_enc
     WHERE id = p_id;
    RETURN FOUND;
END
$$;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE FUNCTION platform_ext_cred_delete(p_actor uuid, p_id uuid) RETURNS boolean
LANGUAGE plpgsql SECURITY DEFINER SET search_path = public, pg_temp AS
$$
BEGIN
    PERFORM platform_assert_admin(p_actor);
    DELETE FROM api_credentials WHERE id = p_id;
    RETURN FOUND;
END
$$;
-- +goose StatementEnd

-- +goose StatementBegin
-- Dipakai gateway SEBELUM autentikasi (belum ada tenant): hanya mengembalikan bahan verifikasi untuk satu key_id.
-- Akun layanan/tenant nonaktif => active=false.
CREATE FUNCTION ext_cred_lookup(p_key_id text) RETURNS TABLE (
    id uuid, tenant_id uuid, user_id uuid, outlet_id uuid, secret_enc text, prev_secret_enc text, prev_expires_at timestamptz,
    ip_allowlist text[], rate_limit_min int, active boolean, expires_at timestamptz)
LANGUAGE sql STABLE SECURITY DEFINER SET search_path = public, pg_temp AS
$$
    SELECT c.id, c.tenant_id, c.user_id, c.outlet_id, c.secret_enc, c.prev_secret_enc, c.prev_expires_at,
           c.ip_allowlist, c.rate_limit_min,
           (c.active AND t.active AND u.active), c.expires_at
      FROM api_credentials c
      JOIN tenants t ON t.id = c.tenant_id
      JOIN users u   ON u.tenant_id = c.tenant_id AND u.id = c.user_id
     WHERE c.key_id = p_key_id
$$;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE FUNCTION ext_cred_touch(p_id uuid, p_ip text) RETURNS void
LANGUAGE sql SECURITY DEFINER SET search_path = public, pg_temp AS
$$ UPDATE api_credentials SET last_used_at = now(), last_used_ip = p_ip
    WHERE id = p_id AND (last_used_at IS NULL OR last_used_at < now() - interval '1 minute') $$;
-- +goose StatementEnd

-- ---- white label ----

-- +goose StatementBegin
CREATE FUNCTION platform_ext_brand_list(p_actor uuid) RETURNS SETOF api_doc_brandings
LANGUAGE plpgsql STABLE SECURITY DEFINER SET search_path = public, pg_temp AS
$$
BEGIN
    PERFORM platform_assert_admin(p_actor);
    RETURN QUERY SELECT * FROM api_doc_brandings ORDER BY is_default DESC, created_at;
END
$$;
-- +goose StatementEnd

-- +goose StatementBegin
-- Buat (p_id NULL) atau ubah satu profil. Menjadikan profil bawaan melepas status bawaan profil lain.
CREATE FUNCTION platform_ext_brand_save(p_actor uuid, p_id uuid, p_slug text, p_domain text, p_is_default boolean, p_active boolean,
    p_name text, p_tagline text, p_logo_url text, p_accent text, p_support_email text, p_support_url text, p_footer text,
    p_base_url text, p_hidden text[], p_sandbox_note text) RETURNS uuid
LANGUAGE plpgsql SECURITY DEFINER SET search_path = public, pg_temp AS
$$
DECLARE v_id uuid;
BEGIN
    PERFORM platform_assert_admin(p_actor);
    IF p_is_default THEN
        UPDATE api_doc_brandings SET is_default = false WHERE is_default AND id IS DISTINCT FROM p_id;
    END IF;
    IF p_id IS NULL THEN
        INSERT INTO api_doc_brandings (slug, domain, is_default, active, name, tagline, logo_url, accent, support_email, support_url, footer, base_url, hidden_modules, sandbox_note)
        VALUES (p_slug, p_domain, p_is_default, p_active, p_name, p_tagline, p_logo_url, p_accent, p_support_email, p_support_url, p_footer, p_base_url, p_hidden, p_sandbox_note)
        RETURNING id INTO v_id;
    ELSE
        UPDATE api_doc_brandings SET slug = p_slug, domain = p_domain, is_default = p_is_default, active = p_active, name = p_name,
               tagline = p_tagline, logo_url = p_logo_url, accent = p_accent, support_email = p_support_email, support_url = p_support_url,
               footer = p_footer, base_url = p_base_url, hidden_modules = p_hidden, sandbox_note = p_sandbox_note
         WHERE id = p_id RETURNING id INTO v_id;
    END IF;
    RETURN v_id;
END
$$;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE FUNCTION platform_ext_brand_delete(p_actor uuid, p_id uuid) RETURNS boolean
LANGUAGE plpgsql SECURITY DEFINER SET search_path = public, pg_temp AS
$$
BEGIN
    PERFORM platform_assert_admin(p_actor);
    IF EXISTS (SELECT 1 FROM api_doc_brandings WHERE id = p_id AND is_default) THEN
        RAISE EXCEPTION 'profil bawaan tidak dapat dihapus' USING ERRCODE = 'P0001';
    END IF;
    DELETE FROM api_doc_brandings WHERE id = p_id;
    RETURN FOUND;
END
$$;
-- +goose StatementEnd

-- +goose StatementBegin
-- Publik: profil untuk Host tertentu (domain cocok, aktif) atau profil bawaan. Kolom internal tidak ikut.
CREATE FUNCTION ext_brand_resolve(p_host text) RETURNS TABLE (
    slug text, name text, tagline text, logo_url text, accent text, support_email text, support_url text, footer text,
    base_url text, hidden_modules text[], sandbox_note text)
LANGUAGE sql STABLE SECURITY DEFINER SET search_path = public, pg_temp AS
$$
    SELECT b.slug, b.name, b.tagline, b.logo_url, b.accent, b.support_email, b.support_url, b.footer, b.base_url, b.hidden_modules, b.sandbox_note
      FROM api_doc_brandings b
     WHERE b.active AND (b.domain = lower(p_host) OR b.is_default)
     ORDER BY (b.domain = lower(p_host)) DESC NULLS LAST, b.is_default DESC
     LIMIT 1
$$;
-- +goose StatementEnd

-- +goose Down

DROP FUNCTION IF EXISTS ext_brand_resolve(text);
DROP FUNCTION IF EXISTS platform_ext_brand_delete(uuid, uuid);
DROP FUNCTION IF EXISTS platform_ext_brand_save(uuid, uuid, text, text, boolean, boolean, text, text, text, text, text, text, text, text, text[], text);
DROP FUNCTION IF EXISTS platform_ext_brand_list(uuid);
DROP FUNCTION IF EXISTS ext_cred_touch(uuid, text);
DROP FUNCTION IF EXISTS ext_cred_lookup(text);
DROP FUNCTION IF EXISTS platform_ext_cred_delete(uuid, uuid);
DROP FUNCTION IF EXISTS platform_ext_cred_rotate(uuid, uuid, text, timestamptz);
DROP FUNCTION IF EXISTS platform_ext_cred_update(uuid, uuid, text, text[], int, boolean, timestamptz);
DROP FUNCTION IF EXISTS platform_ext_cred_list(uuid, uuid);
DROP FUNCTION IF EXISTS platform_ext_cred_create(uuid, uuid, uuid, uuid, text, text, text, text[], int, timestamptz);
DROP FUNCTION IF EXISTS platform_ext_tenant_actors(uuid, uuid);
DROP TABLE IF EXISTS api_doc_brandings;
DROP TABLE IF EXISTS api_credentials;
