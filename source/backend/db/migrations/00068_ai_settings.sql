-- +goose Up

-- Pengaturan Asisten AI (HalloArus) per tenant. Tenant membawa API key Claude sendiri (BYOK): kunci dienkripsi
-- AES-256-GCM di aplikasi (terikat ke tenant lewat AAD) dan TIDAK PERNAH dikirim balik ke klien; UI hanya menerima `key_hint`
-- (4 karakter terakhir). Asisten hanya membaca data (transaksi READ ONLY di sisi server), tabel ini satu-satunya yang ditulis modul AI.
CREATE TABLE ai_settings (
    tenant_id   uuid PRIMARY KEY REFERENCES tenants (id) ON DELETE CASCADE,
    api_key_enc text        NOT NULL CHECK (char_length(api_key_enc) BETWEEN 20 AND 2000),
    key_hint    text        NOT NULL CHECK (char_length(key_hint) BETWEEN 1 AND 8),
    model       text        NOT NULL CHECK (model IN ('claude-fable-5-1', 'claude-opus-5-5', 'claude-sonnet-5-5', 'claude-haiku-5-5')),
    updated_by  uuid        NOT NULL,
    updated_at  timestamptz NOT NULL DEFAULT now()
);

ALTER TABLE ai_settings ENABLE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON ai_settings USING (tenant_id = app_tenant_id()) WITH CHECK (tenant_id = app_tenant_id());

-- +goose Down
DROP TABLE ai_settings;
