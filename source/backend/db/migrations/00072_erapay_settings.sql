-- +goose Up

-- API key EraPay per tenant (tiap toko punya merchant/key sendiri di EraPay). Dienkripsi AES-256-GCM di aplikasi (terikat ke
-- tenant lewat AAD) dan TIDAK PERNAH dikirim balik ke klien; UI hanya menerima `key_hint` (4 karakter terakhir).
CREATE TABLE erapay_settings (
    tenant_id   uuid PRIMARY KEY REFERENCES tenants (id) ON DELETE CASCADE,
    api_key_enc text        NOT NULL CHECK (char_length(api_key_enc) BETWEEN 20 AND 2000),
    key_hint    text        NOT NULL CHECK (char_length(key_hint) BETWEEN 1 AND 8),
    environment text        NOT NULL CHECK (environment IN ('sandbox', 'production')),
    updated_by  uuid        NOT NULL,
    updated_at  timestamptz NOT NULL DEFAULT now()
);

ALTER TABLE erapay_settings ENABLE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON erapay_settings USING (tenant_id = app_tenant_id()) WITH CHECK (tenant_id = app_tenant_id());

-- +goose Down
DROP TABLE erapay_settings;
