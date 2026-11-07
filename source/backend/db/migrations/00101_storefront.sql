-- +goose Up

-- Landing page publik per tenant (/s/{slug}), ditulis pemilik toko sebagai HTML + Tailwind + CSS sendiri dengan penanda
-- {namabarang} dsb. Satu baris per tenant; teks disaring di aplikasi (tanpa script) dan dilayani dengan CSP ketat.
CREATE TABLE storefronts (
    tenant_id  uuid PRIMARY KEY REFERENCES tenants (id) ON DELETE RESTRICT,
    slug       text        NOT NULL,
    enabled    boolean     NOT NULL DEFAULT false,
    title      text        NOT NULL DEFAULT '' CHECK (char_length(title) <= 80),
    html       text        NOT NULL DEFAULT '' CHECK (char_length(html) <= 60000),
    css        text        NOT NULL DEFAULT '' CHECK (char_length(css) <= 20000),
    version    int         NOT NULL DEFAULT 1 CHECK (version >= 1),
    updated_by uuid,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT storefronts_slug_format CHECK (slug ~ '^[a-z0-9][a-z0-9-]{2,39}$')
);
-- Slug unik global (satu namespace /s/{slug}).
CREATE UNIQUE INDEX storefronts_slug_key ON storefronts (slug);
CREATE TRIGGER storefronts_set_updated_at BEFORE UPDATE ON storefronts FOR EACH ROW EXECUTE FUNCTION set_updated_at();
ALTER TABLE storefronts ENABLE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON storefronts USING (tenant_id = app_tenant_id()) WITH CHECK (tenant_id = app_tenant_id());
GRANT SELECT, INSERT, UPDATE ON storefronts TO aciraba_app;

-- Riwayat versi (disimpan tiap simpan; aplikasi memangkas sisanya sampai 20 terbaru).
CREATE TABLE storefront_versions (
    tenant_id  uuid        NOT NULL REFERENCES tenants (id) ON DELETE RESTRICT,
    version    int         NOT NULL,
    title      text        NOT NULL DEFAULT '',
    html       text        NOT NULL,
    css        text        NOT NULL,
    created_by uuid,
    created_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (tenant_id, version)
);
ALTER TABLE storefront_versions ENABLE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON storefront_versions USING (tenant_id = app_tenant_id()) WITH CHECK (tenant_id = app_tenant_id());
GRANT SELECT, INSERT, DELETE ON storefront_versions TO aciraba_app;

-- Publik (tanpa konteks tenant): halaman aktif milik tenant aktif. Hanya kolom yang perlu untuk merender.
-- +goose StatementBegin
CREATE FUNCTION storefront_public_lookup(p_slug text) RETURNS TABLE (tenant_id uuid, tenant_name text, title text, html text, css text, version int)
LANGUAGE sql STABLE SECURITY DEFINER SET search_path = public, pg_temp AS
$$
    SELECT s.tenant_id, t.name, s.title, s.html, s.css, s.version
      FROM storefronts s JOIN tenants t ON t.id = s.tenant_id
     WHERE s.slug = lower(p_slug) AND s.enabled AND t.active
$$;
-- +goose StatementEnd
REVOKE ALL ON FUNCTION storefront_public_lookup(text) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION storefront_public_lookup(text) TO aciraba_app;

-- Tenant pemilik slug (halaman boleh belum diaktifkan): dipakai alamat gambar agar pratinjau di editor ikut menampilkan gambar.
-- +goose StatementBegin
CREATE FUNCTION storefront_slug_tenant(p_slug text) RETURNS uuid
LANGUAGE sql STABLE SECURITY DEFINER SET search_path = public, pg_temp AS
$$
    SELECT s.tenant_id FROM storefronts s JOIN tenants t ON t.id = s.tenant_id WHERE s.slug = lower(p_slug) AND t.active
$$;
-- +goose StatementEnd
REVOKE ALL ON FUNCTION storefront_slug_tenant(text) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION storefront_slug_tenant(text) TO aciraba_app;

-- Indeks untuk blok {#barang kategori=…}: barang aktif per kategori berurutan nama (keyset-ready, tanpa sort besar).
CREATE INDEX items_storefront_idx ON items (tenant_id, category_id, lower(name), id) WHERE active;

-- +goose Down
DROP INDEX items_storefront_idx;
DROP FUNCTION storefront_slug_tenant(text);
DROP FUNCTION storefront_public_lookup(text);
DROP TABLE storefront_versions;
DROP TABLE storefronts;
