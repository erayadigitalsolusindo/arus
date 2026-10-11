-- +goose Up

-- Connector MCP untuk Claude (HalloArus lewat aplikasi Claude milik pengguna). Server MCP ARUS hanya punya tool BACA.
-- OAuth 2.1 + PKCE: klien (mis. Claude) mendaftar sendiri (Dynamic Client Registration, RFC 7591), pengguna menyetujui di ARUS,
-- lalu klien menukar kode menjadi token. Token buram (opaque), hanya hash-nya yang disimpan.

-- Klien OAuth publik (tanpa rahasia; keamanan dari PKCE + redirect_uri persis). Bukan data tenant, jadi tanpa RLS.
CREATE TABLE oauth_clients (
    client_id     uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    name          text        NOT NULL CHECK (char_length(name) BETWEEN 1 AND 100),
    redirect_uris text[]      NOT NULL CHECK (cardinality(redirect_uris) BETWEEN 1 AND 5),
    created_at    timestamptz NOT NULL DEFAULT now()
);
REVOKE UPDATE, DELETE, TRUNCATE ON oauth_clients FROM aciraba_app;

-- Satu izin yang diberikan pengguna ke satu klien. Dicabut = revoked_at (baris tidak dihapus agar jejak tetap ada).
CREATE TABLE mcp_grants (
    id                 uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id          uuid        NOT NULL REFERENCES tenants (id) ON DELETE CASCADE,
    user_id            uuid        NOT NULL,
    outlet_id          uuid        NOT NULL,            -- outlet aktif saat menyetujui (konteks "hari ini" dan bawaan tool)
    client_id          uuid        NOT NULL REFERENCES oauth_clients (client_id),
    client_name        text        NOT NULL,
    access_hash        text        NOT NULL,
    access_expires_at  timestamptz NOT NULL,
    refresh_hash       text        NOT NULL,
    refresh_expires_at timestamptz NOT NULL,
    created_at         timestamptz NOT NULL DEFAULT now(),
    last_used_at       timestamptz,
    revoked_at         timestamptz,
    CONSTRAINT mcp_grants_user_fk FOREIGN KEY (tenant_id, user_id) REFERENCES users (tenant_id, id) ON DELETE CASCADE
);
CREATE INDEX mcp_grants_tenant_idx ON mcp_grants (tenant_id, created_at DESC);

ALTER TABLE mcp_grants ENABLE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON mcp_grants USING (tenant_id = app_tenant_id()) WITH CHECK (tenant_id = app_tenant_id());
REVOKE DELETE, TRUNCATE ON mcp_grants FROM aciraba_app;

-- +goose Down
DROP TABLE mcp_grants;
DROP TABLE oauth_clients;
