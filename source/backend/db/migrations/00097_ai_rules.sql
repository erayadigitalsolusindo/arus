-- +goose Up
-- +goose StatementBegin

-- Aturan operator HalloArus dalam markdown: satu dokumen GLOBAL + satu dokumen khusus per tenant, keduanya HANYA diedit Platform Admin.
-- Berversi (riwayat + rollback = menyimpan ulang isi lama sebagai versi baru). Versi aktif = versi tertinggi per cakupan; isi kosong
-- = tidak ada aturan. Isi disisipkan ke prompt sistem sebagai aturan wajib; pembatas keras (izin, domain, kuota) tetap dijaga kode.
CREATE TABLE ai_rules (
    id         uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id  uuid REFERENCES tenants (id) ON DELETE CASCADE,
    version    int  NOT NULL CHECK (version > 0),
    body       text NOT NULL DEFAULT '' CHECK (char_length(body) <= 6000),
    created_by uuid,
    created_at timestamptz NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX ai_rules_global_ver ON ai_rules (version) WHERE tenant_id IS NULL;
CREATE UNIQUE INDEX ai_rules_tenant_ver ON ai_rules (tenant_id, version DESC) WHERE tenant_id IS NOT NULL;

ALTER TABLE ai_rules ENABLE ROW LEVEL SECURITY;
CREATE POLICY read_global_or_own ON ai_rules FOR SELECT USING (tenant_id IS NULL OR tenant_id = app_tenant_id());
REVOKE ALL ON ai_rules FROM aciraba_app;
GRANT SELECT ON ai_rules TO aciraba_app;

-- Riwayat satu cakupan (p_tenant NULL = global), terbaru dulu.
CREATE FUNCTION platform_ai_rules_list(p_actor uuid, p_tenant uuid, p_limit int) RETURNS TABLE (
    version int, body text, created_at timestamptz, created_by uuid, author text
)
LANGUAGE plpgsql STABLE SECURITY DEFINER SET search_path = public, pg_temp AS
$$
BEGIN
    PERFORM platform_assert_admin(p_actor);
    RETURN QUERY
    SELECT r.version, r.body, r.created_at, r.created_by, COALESCE(a.name, '')
      FROM ai_rules r LEFT JOIN platform_admins a ON a.id = r.created_by
     WHERE r.tenant_id IS NOT DISTINCT FROM p_tenant
     ORDER BY r.version DESC LIMIT LEAST(GREATEST(p_limit, 1), 50);
END
$$;

-- Tenant yang punya dokumen aktif (versi terbaru tidak kosong), untuk daftar di panel.
CREATE FUNCTION platform_ai_rules_tenants(p_actor uuid) RETURNS TABLE (
    tenant_id uuid, tenant_code text, tenant_name text, version int, size int, created_at timestamptz
)
LANGUAGE plpgsql STABLE SECURITY DEFINER SET search_path = public, pg_temp AS
$$
BEGIN
    PERFORM platform_assert_admin(p_actor);
    RETURN QUERY
    SELECT t.id, t.code, t.name, r.version, char_length(r.body), r.created_at
      FROM (SELECT DISTINCT ON (x.tenant_id) x.* FROM ai_rules x WHERE x.tenant_id IS NOT NULL ORDER BY x.tenant_id, x.version DESC) r
      JOIN tenants t ON t.id = r.tenant_id
     WHERE r.body <> ''
     ORDER BY t.name;
END
$$;

-- Simpan versi baru (isi kosong = hapus aturan). Dikunci per cakupan agar nomor versi tidak bentrok.
CREATE FUNCTION platform_ai_rules_save(p_actor uuid, p_tenant uuid, p_body text) RETURNS int
LANGUAGE plpgsql SECURITY DEFINER SET search_path = public, pg_temp AS
$$
DECLARE
    v_ver int;
BEGIN
    PERFORM platform_assert_admin(p_actor);
    PERFORM pg_advisory_xact_lock(hashtextextended('ai_rules:' || COALESCE(p_tenant::text, 'global'), 0));
    SELECT COALESCE(max(version), 0) + 1 INTO v_ver FROM ai_rules WHERE tenant_id IS NOT DISTINCT FROM p_tenant;
    INSERT INTO ai_rules (tenant_id, version, body, created_by) VALUES (p_tenant, v_ver, p_body, p_actor);
    -- Simpan paling banyak 50 versi terakhir per cakupan.
    DELETE FROM ai_rules WHERE tenant_id IS NOT DISTINCT FROM p_tenant AND version <= v_ver - 50;
    RETURN v_ver;
END
$$;

REVOKE ALL ON FUNCTION platform_ai_rules_list(uuid, uuid, int), platform_ai_rules_tenants(uuid), platform_ai_rules_save(uuid, uuid, text) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION platform_ai_rules_list(uuid, uuid, int), platform_ai_rules_tenants(uuid), platform_ai_rules_save(uuid, uuid, text) TO aciraba_app;

-- +goose StatementEnd

-- +goose Down
DROP FUNCTION platform_ai_rules_save(uuid, uuid, text);
DROP FUNCTION platform_ai_rules_tenants(uuid);
DROP FUNCTION platform_ai_rules_list(uuid, uuid, int);
DROP TABLE ai_rules;
