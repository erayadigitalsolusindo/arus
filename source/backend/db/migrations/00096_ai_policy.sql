-- +goose Up
-- +goose StatementBegin

-- Kebijakan HalloArus milik platform: satu baris GLOBAL (tenant_id NULL, semua kolom aturan terisi) + pengecualian per tenant
-- (kolom NULL = ikut global). Aturan platform selalu menang atas izin role tenant: role hanya bisa mempersempit.
-- Aplikasi hanya boleh MEMBACA (RLS: baris global + baris tenant sendiri); penulisan lewat fungsi Platform Admin di bawah.
CREATE TABLE ai_policies (
    id               uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id        uuid REFERENCES tenants (id) ON DELETE CASCADE,
    enabled          boolean,
    -- Domain data yang boleh dibaca AI: sales, stock, debts, purchases, cash, accounting.
    domains          text[],
    allow_write      boolean,  -- tool usulan aksi tulis (kas kecil, foto nota)
    allow_cost       boolean,  -- HPP, laba, nilai persediaan
    allow_byok       boolean,  -- tenant boleh memasang API key penyedia bawaan sendiri
    allow_custom     boolean,  -- tenant boleh memasang penyedia kustom (OpenAI-compatible)
    allow_images     boolean,  -- foto nota
    allow_history    boolean,  -- simpan riwayat percakapan
    allow_mcp        boolean,  -- connector MCP (Claude/ChatGPT/Gemini/Qwen di luar aplikasi)
    free_enabled     boolean,  -- akun gratis (key milik platform)
    free_daily_limit int CHECK (free_daily_limit IS NULL OR free_daily_limit BETWEEN 0 AND 1000), -- global NULL = env AI_FREE_DAILY_LIMIT
    free_model       text CHECK (free_model IS NULL OR char_length(free_model) BETWEEN 1 AND 80),   -- NULL = model murah bawaan
    chat_limit       int CHECK (chat_limit IS NULL OR chat_limit BETWEEN 1 AND 200),               -- pertanyaan per pengguna per 10 menit
    max_range_days   int CHECK (max_range_days IS NULL OR max_range_days BETWEEN 1 AND 366),       -- rentang tanggal maksimum per pertanyaan
    note             text NOT NULL DEFAULT '' CHECK (char_length(note) <= 300),
    updated_by       uuid,
    created_at       timestamptz NOT NULL DEFAULT now(),
    updated_at       timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT ai_policies_domains_valid CHECK (domains IS NULL OR domains <@ ARRAY['sales','stock','debts','purchases','cash','accounting']::text[]),
    CONSTRAINT ai_policies_global_complete CHECK (tenant_id IS NOT NULL OR (
        enabled IS NOT NULL AND domains IS NOT NULL AND allow_write IS NOT NULL AND allow_cost IS NOT NULL AND allow_byok IS NOT NULL
        AND allow_custom IS NOT NULL AND allow_images IS NOT NULL AND allow_history IS NOT NULL AND allow_mcp IS NOT NULL
        AND free_enabled IS NOT NULL AND chat_limit IS NOT NULL AND max_range_days IS NOT NULL))
);
CREATE UNIQUE INDEX ai_policies_global_one ON ai_policies ((true)) WHERE tenant_id IS NULL;
CREATE UNIQUE INDEX ai_policies_tenant_one ON ai_policies (tenant_id) WHERE tenant_id IS NOT NULL;
CREATE TRIGGER ai_policies_set_updated_at BEFORE UPDATE ON ai_policies FOR EACH ROW EXECUTE FUNCTION set_updated_at();

ALTER TABLE ai_policies ENABLE ROW LEVEL SECURITY;
CREATE POLICY read_global_or_own ON ai_policies FOR SELECT USING (tenant_id IS NULL OR tenant_id = app_tenant_id());
REVOKE ALL ON ai_policies FROM aciraba_app;
GRANT SELECT ON ai_policies TO aciraba_app;

-- Bawaan global = perilaku sebelum fitur ini ada (semua dibuka).
INSERT INTO ai_policies (tenant_id, enabled, domains, allow_write, allow_cost, allow_byok, allow_custom, allow_images, allow_history, allow_mcp,
                         free_enabled, chat_limit, max_range_days)
VALUES (NULL, true, ARRAY['sales','stock','debts','purchases','cash','accounting'], true, true, true, true, true, true, true, true, 20, 366);

-- Daftar kebijakan: global + semua pengecualian tenant (jumlahnya dibatasi jumlah tenant).
CREATE FUNCTION platform_ai_policy_list(p_actor uuid) RETURNS TABLE (
    tenant_id uuid, tenant_code text, tenant_name text,
    enabled boolean, domains text[], allow_write boolean, allow_cost boolean, allow_byok boolean, allow_custom boolean,
    allow_images boolean, allow_history boolean, allow_mcp boolean, free_enabled boolean, free_daily_limit int, free_model text,
    chat_limit int, max_range_days int, note text, updated_at timestamptz
)
LANGUAGE plpgsql STABLE SECURITY DEFINER SET search_path = public, pg_temp AS
$$
BEGIN
    PERFORM platform_assert_admin(p_actor);
    RETURN QUERY
    SELECT p.tenant_id, t.code, t.name, p.enabled, p.domains, p.allow_write, p.allow_cost, p.allow_byok, p.allow_custom, p.allow_images,
           p.allow_history, p.allow_mcp, p.free_enabled, p.free_daily_limit, p.free_model, p.chat_limit, p.max_range_days, p.note, p.updated_at
      FROM ai_policies p LEFT JOIN tenants t ON t.id = p.tenant_id
     ORDER BY (p.tenant_id IS NOT NULL), t.name, p.id;
END
$$;

-- Simpan kebijakan (p_tenant NULL = global). p = JSON; kunci yang tidak ada/NULL = NULL (ikut global pada pengecualian).
CREATE FUNCTION platform_ai_policy_save(p_actor uuid, p_tenant uuid, p jsonb, p_note text) RETURNS boolean
LANGUAGE plpgsql SECURITY DEFINER SET search_path = public, pg_temp AS
$$
DECLARE
    v_domains text[];
BEGIN
    PERFORM platform_assert_admin(p_actor);
    IF jsonb_typeof(p -> 'domains') = 'array' THEN
        SELECT COALESCE(array_agg(x), ARRAY[]::text[]) INTO v_domains FROM jsonb_array_elements_text(p -> 'domains') AS x;
    END IF;
    IF p_tenant IS NULL THEN
        UPDATE ai_policies SET
            enabled = (p ->> 'enabled')::boolean, domains = v_domains, allow_write = (p ->> 'allow_write')::boolean,
            allow_cost = (p ->> 'allow_cost')::boolean, allow_byok = (p ->> 'allow_byok')::boolean, allow_custom = (p ->> 'allow_custom')::boolean,
            allow_images = (p ->> 'allow_images')::boolean, allow_history = (p ->> 'allow_history')::boolean, allow_mcp = (p ->> 'allow_mcp')::boolean,
            free_enabled = (p ->> 'free_enabled')::boolean, free_daily_limit = (p ->> 'free_daily_limit')::int, free_model = NULLIF(p ->> 'free_model', ''),
            chat_limit = (p ->> 'chat_limit')::int, max_range_days = (p ->> 'max_range_days')::int, note = p_note, updated_by = p_actor
         WHERE tenant_id IS NULL;
    ELSE
        INSERT INTO ai_policies (tenant_id, enabled, domains, allow_write, allow_cost, allow_byok, allow_custom, allow_images, allow_history, allow_mcp,
                                 free_enabled, free_daily_limit, free_model, chat_limit, max_range_days, note, updated_by)
        VALUES (p_tenant, (p ->> 'enabled')::boolean, v_domains, (p ->> 'allow_write')::boolean, (p ->> 'allow_cost')::boolean,
                (p ->> 'allow_byok')::boolean, (p ->> 'allow_custom')::boolean, (p ->> 'allow_images')::boolean, (p ->> 'allow_history')::boolean,
                (p ->> 'allow_mcp')::boolean, (p ->> 'free_enabled')::boolean, (p ->> 'free_daily_limit')::int, NULLIF(p ->> 'free_model', ''),
                (p ->> 'chat_limit')::int, (p ->> 'max_range_days')::int, p_note, p_actor)
        ON CONFLICT (tenant_id) WHERE tenant_id IS NOT NULL DO UPDATE SET
            enabled = EXCLUDED.enabled, domains = EXCLUDED.domains, allow_write = EXCLUDED.allow_write, allow_cost = EXCLUDED.allow_cost,
            allow_byok = EXCLUDED.allow_byok, allow_custom = EXCLUDED.allow_custom, allow_images = EXCLUDED.allow_images,
            allow_history = EXCLUDED.allow_history, allow_mcp = EXCLUDED.allow_mcp, free_enabled = EXCLUDED.free_enabled,
            free_daily_limit = EXCLUDED.free_daily_limit, free_model = EXCLUDED.free_model, chat_limit = EXCLUDED.chat_limit,
            max_range_days = EXCLUDED.max_range_days, note = EXCLUDED.note, updated_by = EXCLUDED.updated_by;
    END IF;
    RETURN FOUND;
END
$$;

-- Hapus pengecualian tenant (kembali ikut global). Baris global tidak bisa dihapus.
CREATE FUNCTION platform_ai_policy_delete(p_actor uuid, p_tenant uuid) RETURNS boolean
LANGUAGE plpgsql SECURITY DEFINER SET search_path = public, pg_temp AS
$$
BEGIN
    PERFORM platform_assert_admin(p_actor);
    IF p_tenant IS NULL THEN
        RETURN false;
    END IF;
    DELETE FROM ai_policies WHERE tenant_id = p_tenant;
    RETURN FOUND;
END
$$;

REVOKE ALL ON FUNCTION platform_ai_policy_list(uuid), platform_ai_policy_save(uuid, uuid, jsonb, text), platform_ai_policy_delete(uuid, uuid) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION platform_ai_policy_list(uuid), platform_ai_policy_save(uuid, uuid, jsonb, text), platform_ai_policy_delete(uuid, uuid) TO aciraba_app;

-- +goose StatementEnd

-- +goose Down
DROP FUNCTION platform_ai_policy_delete(uuid, uuid);
DROP FUNCTION platform_ai_policy_save(uuid, uuid, jsonb, text);
DROP FUNCTION platform_ai_policy_list(uuid);
DROP TABLE ai_policies;
