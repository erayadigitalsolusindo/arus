-- +goose Up
-- +goose StatementBegin

-- "API ARUS": penyedia AI milik platform untuk akun gratis HalloArus (OpenAI-compatible: base URL + model + API key), diatur Platform
-- Admin dari panel tanpa restart server. Satu baris saja. Key disimpan terenkripsi (AES-GCM, diikat ke konteks "platform"). Tabel tidak
-- bisa dibaca langsung oleh aplikasi; akses hanya lewat fungsi. Kosong = akun gratis memakai AI_PLATFORM_KEY dari env (cadangan).
CREATE TABLE ai_platform_provider (
    id          boolean PRIMARY KEY DEFAULT true CHECK (id),
    base_url    text NOT NULL CHECK (char_length(base_url) BETWEEN 1 AND 300),
    model       text NOT NULL CHECK (char_length(model) BETWEEN 1 AND 100),
    api_key_enc text NOT NULL,
    key_hint    text NOT NULL DEFAULT '',
    updated_by  uuid,
    updated_at  timestamptz NOT NULL DEFAULT now()
);
ALTER TABLE ai_platform_provider ENABLE ROW LEVEL SECURITY;
REVOKE ALL ON ai_platform_provider FROM aciraba_app;

-- Untuk panel operator (tanpa key).
CREATE FUNCTION platform_ai_provider_get(p_actor uuid) RETURNS TABLE (
    base_url text, model text, key_hint text, updated_at timestamptz, author text
)
LANGUAGE plpgsql STABLE SECURITY DEFINER SET search_path = public, pg_temp AS
$$
BEGIN
    PERFORM platform_assert_admin(p_actor);
    RETURN QUERY
    SELECT p.base_url, p.model, p.key_hint, p.updated_at, COALESCE(a.name, '')
      FROM ai_platform_provider p LEFT JOIN platform_admins a ON a.id = p.updated_by;
END
$$;

CREATE FUNCTION platform_ai_provider_save(p_actor uuid, p_base text, p_model text, p_enc text, p_hint text) RETURNS void
LANGUAGE plpgsql SECURITY DEFINER SET search_path = public, pg_temp AS
$$
BEGIN
    PERFORM platform_assert_admin(p_actor);
    INSERT INTO ai_platform_provider (id, base_url, model, api_key_enc, key_hint, updated_by)
    VALUES (true, p_base, p_model, p_enc, p_hint, p_actor)
    ON CONFLICT (id) DO UPDATE SET base_url = EXCLUDED.base_url, model = EXCLUDED.model, api_key_enc = EXCLUDED.api_key_enc,
        key_hint = EXCLUDED.key_hint, updated_by = EXCLUDED.updated_by, updated_at = now();
END
$$;

CREATE FUNCTION platform_ai_provider_clear(p_actor uuid) RETURNS boolean
LANGUAGE plpgsql SECURITY DEFINER SET search_path = public, pg_temp AS
$$
DECLARE
    v_n int;
BEGIN
    PERFORM platform_assert_admin(p_actor);
    DELETE FROM ai_platform_provider;
    GET DIAGNOSTICS v_n = ROW_COUNT;
    RETURN v_n > 0;
END
$$;

-- Dibaca layanan HalloArus saat melayani akun gratis (key tetap terenkripsi; dibuka di aplikasi).
CREATE FUNCTION ai_platform_provider_load() RETURNS TABLE (base_url text, model text, api_key_enc text)
LANGUAGE sql STABLE SECURITY DEFINER SET search_path = public, pg_temp AS
$$ SELECT base_url, model, api_key_enc FROM ai_platform_provider $$;

REVOKE ALL ON FUNCTION platform_ai_provider_get(uuid), platform_ai_provider_save(uuid, text, text, text, text),
    platform_ai_provider_clear(uuid), ai_platform_provider_load() FROM PUBLIC;
GRANT EXECUTE ON FUNCTION platform_ai_provider_get(uuid), platform_ai_provider_save(uuid, text, text, text, text),
    platform_ai_provider_clear(uuid), ai_platform_provider_load() TO aciraba_app;

-- +goose StatementEnd

-- +goose Down
DROP FUNCTION ai_platform_provider_load();
DROP FUNCTION platform_ai_provider_clear(uuid);
DROP FUNCTION platform_ai_provider_save(uuid, text, text, text, text);
DROP FUNCTION platform_ai_provider_get(uuid);
DROP TABLE ai_platform_provider;
