-- +goose Up

-- HalloArus mendukung banyak penyedia AI: satu baris per (tenant, penyedia) dengan API key sendiri-sendiri, tepat satu yang aktif.
-- Daftar model dijaga di aplikasi (berubah cepat), bukan CHECK di DB.
ALTER TABLE ai_settings DROP CONSTRAINT ai_settings_model_check;
ALTER TABLE ai_settings DROP CONSTRAINT ai_settings_pkey;
ALTER TABLE ai_settings ADD COLUMN provider text NOT NULL DEFAULT 'anthropic' CHECK (provider IN ('anthropic', 'openai', 'gemini', 'qwen'));
ALTER TABLE ai_settings ADD COLUMN active boolean NOT NULL DEFAULT true;
ALTER TABLE ai_settings ADD PRIMARY KEY (tenant_id, provider);
ALTER TABLE ai_settings ADD CONSTRAINT ai_settings_model_len CHECK (char_length(model) BETWEEN 1 AND 100);
CREATE UNIQUE INDEX ai_settings_one_active ON ai_settings (tenant_id) WHERE active;
ALTER TABLE ai_settings ALTER COLUMN active SET DEFAULT false;

-- +goose Down
DROP INDEX ai_settings_one_active;
DELETE FROM ai_settings WHERE provider <> 'anthropic';
ALTER TABLE ai_settings DROP CONSTRAINT ai_settings_model_len;
ALTER TABLE ai_settings DROP CONSTRAINT ai_settings_pkey;
ALTER TABLE ai_settings DROP COLUMN active;
ALTER TABLE ai_settings DROP COLUMN provider;
ALTER TABLE ai_settings ADD PRIMARY KEY (tenant_id);
ALTER TABLE ai_settings ADD CONSTRAINT ai_settings_model_check CHECK (model IN ('claude-fable-5-1', 'claude-opus-5-5', 'claude-sonnet-5-5', 'claude-haiku-5-5'));
