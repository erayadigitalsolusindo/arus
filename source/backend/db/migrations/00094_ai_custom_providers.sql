-- +goose Up

-- Penyedia AI kustom per tenant (API gaya OpenAI /chat/completions, mis. OpenKey, OpenRouter, vLLM publik): tenant mengisi base URL,
-- nama model, dan API key sendiri. Id penyedia = `custom_<8 hex>` (boleh lebih dari satu per tenant; batas di aplikasi). API key tetap
-- terenkripsi di `api_key_enc` (AAD = tenant) seperti penyedia bawaan. Base URL divalidasi ketat di aplikasi (https, bukan IP privat).
ALTER TABLE ai_settings DROP CONSTRAINT ai_settings_provider_check;
ALTER TABLE ai_settings ADD CONSTRAINT ai_settings_provider_check CHECK (provider IN ('anthropic', 'openai', 'gemini', 'qwen') OR provider ~ '^custom_[a-z0-9]{8}$');
ALTER TABLE ai_settings ADD COLUMN base_url text NOT NULL DEFAULT '' CHECK (char_length(base_url) <= 300);
ALTER TABLE ai_settings ADD COLUMN label text NOT NULL DEFAULT '' CHECK (char_length(label) <= 40);
ALTER TABLE ai_settings ADD CONSTRAINT ai_settings_custom_fields CHECK (provider NOT LIKE 'custom\_%' OR (base_url <> '' AND label <> ''));

-- +goose Down
DELETE FROM ai_settings WHERE provider LIKE 'custom\_%';
ALTER TABLE ai_settings DROP CONSTRAINT ai_settings_custom_fields;
ALTER TABLE ai_settings DROP COLUMN label;
ALTER TABLE ai_settings DROP COLUMN base_url;
ALTER TABLE ai_settings DROP CONSTRAINT ai_settings_provider_check;
ALTER TABLE ai_settings ADD CONSTRAINT ai_settings_provider_check CHECK (provider IN ('anthropic', 'openai', 'gemini', 'qwen'));
