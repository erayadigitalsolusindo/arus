-- +goose Up
-- +goose StatementBegin

-- Kebijakan "Batasi topik usaha": HalloArus menolak pertanyaan di luar data usaha/ARUS. Bawaan global = aktif.
ALTER TABLE ai_policies ADD COLUMN topic_strict boolean;
UPDATE ai_policies SET topic_strict = true WHERE tenant_id IS NULL;
ALTER TABLE ai_policies DROP CONSTRAINT ai_policies_global_complete;
ALTER TABLE ai_policies ADD CONSTRAINT ai_policies_global_complete CHECK (tenant_id IS NOT NULL OR (
    enabled IS NOT NULL AND domains IS NOT NULL AND allow_write IS NOT NULL AND allow_cost IS NOT NULL AND allow_byok IS NOT NULL
    AND allow_custom IS NOT NULL AND allow_images IS NOT NULL AND allow_history IS NOT NULL AND allow_mcp IS NOT NULL
    AND free_enabled IS NOT NULL AND chat_limit IS NOT NULL AND max_range_days IS NOT NULL AND topic_strict IS NOT NULL));

DROP FUNCTION platform_ai_policy_list(uuid);
DROP FUNCTION platform_ai_policy_save(uuid, uuid, jsonb, text);

CREATE FUNCTION platform_ai_policy_list(p_actor uuid) RETURNS TABLE (
    tenant_id uuid, tenant_code text, tenant_name text,
    enabled boolean, domains text[], allow_write boolean, allow_cost boolean, allow_byok boolean, allow_custom boolean,
    allow_images boolean, allow_history boolean, allow_mcp boolean, free_enabled boolean, free_daily_limit int, free_model text,
    chat_limit int, max_range_days int, topic_strict boolean, note text, updated_at timestamptz
)
LANGUAGE plpgsql STABLE SECURITY DEFINER SET search_path = public, pg_temp AS
$$
BEGIN
    PERFORM platform_assert_admin(p_actor);
    RETURN QUERY
    SELECT p.tenant_id, t.code, t.name, p.enabled, p.domains, p.allow_write, p.allow_cost, p.allow_byok, p.allow_custom, p.allow_images,
           p.allow_history, p.allow_mcp, p.free_enabled, p.free_daily_limit, p.free_model, p.chat_limit, p.max_range_days, p.topic_strict, p.note, p.updated_at
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
            chat_limit = (p ->> 'chat_limit')::int, max_range_days = (p ->> 'max_range_days')::int, topic_strict = (p ->> 'topic_strict')::boolean, note = p_note, updated_by = p_actor
         WHERE tenant_id IS NULL;
    ELSE
        INSERT INTO ai_policies (tenant_id, enabled, domains, allow_write, allow_cost, allow_byok, allow_custom, allow_images, allow_history, allow_mcp,
                                 free_enabled, free_daily_limit, free_model, chat_limit, max_range_days, topic_strict, note, updated_by)
        VALUES (p_tenant, (p ->> 'enabled')::boolean, v_domains, (p ->> 'allow_write')::boolean, (p ->> 'allow_cost')::boolean,
                (p ->> 'allow_byok')::boolean, (p ->> 'allow_custom')::boolean, (p ->> 'allow_images')::boolean, (p ->> 'allow_history')::boolean,
                (p ->> 'allow_mcp')::boolean, (p ->> 'free_enabled')::boolean, (p ->> 'free_daily_limit')::int, NULLIF(p ->> 'free_model', ''),
                (p ->> 'chat_limit')::int, (p ->> 'max_range_days')::int, (p ->> 'topic_strict')::boolean, p_note, p_actor)
        ON CONFLICT (tenant_id) WHERE tenant_id IS NOT NULL DO UPDATE SET
            enabled = EXCLUDED.enabled, domains = EXCLUDED.domains, allow_write = EXCLUDED.allow_write, allow_cost = EXCLUDED.allow_cost,
            allow_byok = EXCLUDED.allow_byok, allow_custom = EXCLUDED.allow_custom, allow_images = EXCLUDED.allow_images,
            allow_history = EXCLUDED.allow_history, allow_mcp = EXCLUDED.allow_mcp, free_enabled = EXCLUDED.free_enabled,
            free_daily_limit = EXCLUDED.free_daily_limit, free_model = EXCLUDED.free_model, chat_limit = EXCLUDED.chat_limit,
            max_range_days = EXCLUDED.max_range_days, topic_strict = EXCLUDED.topic_strict, note = EXCLUDED.note, updated_by = EXCLUDED.updated_by;
    END IF;
    RETURN FOUND;
END
$$;

REVOKE ALL ON FUNCTION platform_ai_policy_list(uuid), platform_ai_policy_save(uuid, uuid, jsonb, text) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION platform_ai_policy_list(uuid), platform_ai_policy_save(uuid, uuid, jsonb, text) TO aciraba_app;

-- +goose StatementEnd

-- +goose Down
-- Down tidak mengembalikan fungsi lama; kolom dibiarkan (nullable, aman).
