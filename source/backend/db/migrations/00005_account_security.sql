-- +goose Up

-- Keamanan akun, akses outlet per pengguna, dan audit log.
-- Semua tabel bertenant baru mengikuti aturan 00004: tenant_id + RLS tenant_isolation + role aciraba_app.

-- ---- users: verifikasi email, persetujuan syarat layanan, pencabutan token akses ----
ALTER TABLE users
    ADD COLUMN email_verified_at  timestamptz,
    ADD COLUMN terms_accepted_at  timestamptz,
    ADD COLUMN terms_version      text,
    -- Token akses yang diterbitkan SEBELUM waktu ini ditolak (reset password, penonaktifan): mencabut
    -- token akses 15 menit yang masih hidup, bukan hanya refresh token.
    ADD COLUMN tokens_valid_after timestamptz NOT NULL DEFAULT to_timestamp(0);

-- ---- akses outlet per pengguna ----
-- Pemilik (role sistem) otomatis mengakses semua outlet; pengguna lain hanya outlet yang ditugaskan di sini.
CREATE TABLE user_outlets (
    tenant_id uuid NOT NULL,
    user_id   uuid NOT NULL,
    outlet_id uuid NOT NULL,
    PRIMARY KEY (user_id, outlet_id),
    -- FK komposit: pengguna dan outlet wajib milik tenant yang sama.
    CONSTRAINT user_outlets_user_fk   FOREIGN KEY (tenant_id, user_id)   REFERENCES users   (tenant_id, id) ON DELETE CASCADE,
    CONSTRAINT user_outlets_outlet_fk FOREIGN KEY (tenant_id, outlet_id) REFERENCES outlets (tenant_id, id) ON DELETE CASCADE
);
CREATE INDEX user_outlets_outlet_idx ON user_outlets (outlet_id);

ALTER TABLE user_outlets ENABLE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON user_outlets USING (tenant_id = app_tenant_id()) WITH CHECK (tenant_id = app_tenant_id());

-- Pengguna yang sudah ada tetap bisa masuk ke semua outlet tenantnya.
INSERT INTO user_outlets (tenant_id, user_id, outlet_id)
SELECT u.tenant_id, u.id, o.id FROM users u JOIN outlets o ON o.tenant_id = u.tenant_id;

-- ---- audit log (append-only) ----
CREATE TABLE audit_log (
    id         bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    tenant_id  uuid        NOT NULL REFERENCES tenants (id) ON DELETE RESTRICT,
    outlet_id  uuid,
    -- Tanpa FK ke users: catatan audit harus bertahan walau akun berubah; nama disimpan sebagai snapshot.
    actor_id   uuid,
    actor_name text        NOT NULL DEFAULT '',
    action     text        NOT NULL,
    entity     text        NOT NULL,
    entity_id  text        NOT NULL DEFAULT '',
    details    jsonb       NOT NULL DEFAULT '{}'::jsonb,
    ip         text        NOT NULL DEFAULT '',
    request_id text        NOT NULL DEFAULT '',
    created_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT audit_log_action_format CHECK (action ~ '^[a-z][a-z0-9_.]{1,63}$'),
    CONSTRAINT audit_log_entity_format CHECK (entity ~ '^[a-z][a-z0-9_]{1,63}$')
);
CREATE INDEX audit_log_tenant_time_idx   ON audit_log (tenant_id, created_at DESC, id DESC);
CREATE INDEX audit_log_tenant_entity_idx ON audit_log (tenant_id, entity, entity_id);
CREATE INDEX audit_log_tenant_actor_idx  ON audit_log (tenant_id, actor_id, created_at DESC);

ALTER TABLE audit_log ENABLE ROW LEVEL SECURITY;
CREATE POLICY tenant_read   ON audit_log FOR SELECT USING (tenant_id = app_tenant_id());
CREATE POLICY tenant_insert ON audit_log FOR INSERT WITH CHECK (tenant_id = app_tenant_id());
-- Append-only: aplikasi tidak boleh mengubah atau menghapus catatan audit.
REVOKE UPDATE, DELETE, TRUNCATE ON audit_log FROM aciraba_app;

-- ---- fungsi pencarian akun (sebelum tenant diketahui): tambah verifikasi email dan outlet yang boleh diakses ----
-- Outlet yang dikembalikan: p_outlet_id bila boleh diakses pengguna, selain itu outlet aktif pertama yang boleh
-- diakses. outlet_* NULL bila pengguna tidak punya outlet aktif (login ditolak dengan pesan khusus).
DROP FUNCTION auth_account_by_email(text);
DROP FUNCTION auth_account_by_id(uuid);

-- +goose StatementBegin
CREATE FUNCTION auth_account_by_email(p_email text)
RETURNS TABLE (
    user_id uuid, user_name text, email text, password_hash text, user_active boolean, email_verified boolean,
    role_name text, tenant_id uuid, tenant_code text, tenant_name text, tenant_active boolean,
    outlet_id uuid, outlet_code text, outlet_name text
)
LANGUAGE sql STABLE SECURITY DEFINER SET search_path = public, pg_temp AS
$$
    SELECT u.id, u.name, u.email, u.password_hash, u.active, u.email_verified_at IS NOT NULL,
           r.name, t.id, t.code, t.name, t.active,
           o.id, o.code, o.name
    FROM users u
    JOIN roles r   ON r.tenant_id = u.tenant_id AND r.id = u.role_id
    JOIN tenants t ON t.id = u.tenant_id
    LEFT JOIN LATERAL (
        SELECT ox.id, ox.code, ox.name FROM outlets ox
        WHERE ox.tenant_id = u.tenant_id AND ox.active
          AND (r.is_system OR EXISTS (SELECT 1 FROM user_outlets uo
                                      WHERE uo.tenant_id = u.tenant_id AND uo.user_id = u.id AND uo.outlet_id = ox.id))
        ORDER BY ox.created_at, ox.code
        LIMIT 1
    ) o ON true
    WHERE lower(u.email) = lower(p_email)
$$;

CREATE FUNCTION auth_account_by_id(p_user_id uuid, p_outlet_id uuid DEFAULT NULL)
RETURNS TABLE (
    user_id uuid, user_name text, email text, password_hash text, user_active boolean, email_verified boolean,
    role_name text, tenant_id uuid, tenant_code text, tenant_name text, tenant_active boolean,
    outlet_id uuid, outlet_code text, outlet_name text
)
LANGUAGE sql STABLE SECURITY DEFINER SET search_path = public, pg_temp AS
$$
    SELECT u.id, u.name, u.email, u.password_hash, u.active, u.email_verified_at IS NOT NULL,
           r.name, t.id, t.code, t.name, t.active,
           o.id, o.code, o.name
    FROM users u
    JOIN roles r   ON r.tenant_id = u.tenant_id AND r.id = u.role_id
    JOIN tenants t ON t.id = u.tenant_id
    LEFT JOIN LATERAL (
        SELECT ox.id, ox.code, ox.name FROM outlets ox
        WHERE ox.tenant_id = u.tenant_id AND ox.active
          AND (r.is_system OR EXISTS (SELECT 1 FROM user_outlets uo
                                      WHERE uo.tenant_id = u.tenant_id AND uo.user_id = u.id AND uo.outlet_id = ox.id))
        ORDER BY COALESCE(ox.id = p_outlet_id, false) DESC, ox.created_at, ox.code
        LIMIT 1
    ) o ON true
    WHERE u.id = p_user_id
$$;
-- +goose StatementEnd

REVOKE ALL ON FUNCTION auth_account_by_email(text), auth_account_by_id(uuid, uuid) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION auth_account_by_email(text), auth_account_by_id(uuid, uuid) TO aciraba_app;

-- +goose Down
DROP FUNCTION auth_account_by_id(uuid, uuid);
DROP FUNCTION auth_account_by_email(text);
-- +goose StatementBegin
CREATE FUNCTION auth_account_by_email(p_email text)
RETURNS TABLE (
    user_id uuid, user_name text, email text, password_hash text, user_active boolean, role_name text,
    tenant_id uuid, tenant_code text, tenant_name text, tenant_active boolean,
    outlet_id uuid, outlet_code text, outlet_name text
)
LANGUAGE sql STABLE SECURITY DEFINER SET search_path = public, pg_temp AS
$$
    SELECT u.id, u.name, u.email, u.password_hash, u.active, r.name, t.id, t.code, t.name, t.active, o.id, o.code, o.name
    FROM users u
    JOIN roles r   ON r.tenant_id = u.tenant_id AND r.id = u.role_id
    JOIN tenants t ON t.id = u.tenant_id
    JOIN outlets o ON o.tenant_id = u.tenant_id AND o.active
    WHERE lower(u.email) = lower(p_email)
    ORDER BY o.created_at, o.code
    LIMIT 1
$$;

CREATE FUNCTION auth_account_by_id(p_user_id uuid)
RETURNS TABLE (
    user_id uuid, user_name text, email text, password_hash text, user_active boolean, role_name text,
    tenant_id uuid, tenant_code text, tenant_name text, tenant_active boolean,
    outlet_id uuid, outlet_code text, outlet_name text
)
LANGUAGE sql STABLE SECURITY DEFINER SET search_path = public, pg_temp AS
$$
    SELECT u.id, u.name, u.email, u.password_hash, u.active, r.name, t.id, t.code, t.name, t.active, o.id, o.code, o.name
    FROM users u
    JOIN roles r   ON r.tenant_id = u.tenant_id AND r.id = u.role_id
    JOIN tenants t ON t.id = u.tenant_id
    JOIN outlets o ON o.tenant_id = u.tenant_id AND o.active
    WHERE u.id = p_user_id
    ORDER BY o.created_at, o.code
    LIMIT 1
$$;
-- +goose StatementEnd
REVOKE ALL ON FUNCTION auth_account_by_email(text), auth_account_by_id(uuid) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION auth_account_by_email(text), auth_account_by_id(uuid) TO aciraba_app;
DROP TABLE audit_log;
DROP TABLE user_outlets;
ALTER TABLE users DROP COLUMN tokens_valid_after, DROP COLUMN terms_version, DROP COLUMN terms_accepted_at, DROP COLUMN email_verified_at;
