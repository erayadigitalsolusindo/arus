-- +goose Up

-- Row Level Security per tenant (AGENTS.md §1, §3.1).
--
-- Model peran:
--   * pemilik skema (mis. `aciraba`)  : menjalankan migration; non-superuser; TIDAK di-FORCE, jadi melewati RLS.
--   * `aciraba_app` (LOGIN)           : dipakai aplikasi saat runtime; bukan pemilik tabel, jadi RLS SELALU berlaku.
--                                       Tanpa `app.tenant_id` ia tidak melihat satu baris pun (fail closed).
-- Role `aciraba_app` dibuat sekali oleh superuser (deploy/initdb atau manual); migration tidak punya hak membuatnya.
--
-- ATURAN untuk setiap tabel bertenant berikutnya: kolom `tenant_id`, ENABLE ROW LEVEL SECURITY, policy
-- `tenant_isolation` seperti di bawah, dan test isolasi (lihat internal/platform/db/rls_test.go).

-- +goose StatementBegin
DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'aciraba_app') THEN
        RAISE EXCEPTION 'role aciraba_app belum ada: buat dulu sebagai superuser (CREATE ROLE aciraba_app LOGIN PASSWORD ''...'' NOSUPERUSER NOBYPASSRLS; GRANT CONNECT ON DATABASE <db> TO aciraba_app; GRANT USAGE ON SCHEMA public TO aciraba_app)';
    END IF;
    IF EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'aciraba_app' AND (rolsuper OR rolbypassrls)) THEN
        RAISE EXCEPTION 'role aciraba_app tidak boleh superuser atau BYPASSRLS (RLS akan terlewati)';
    END IF;
END
$$;
-- +goose StatementEnd

-- Tenant aktif untuk transaksi berjalan; diisi aplikasi lewat set_config('app.tenant_id', <uuid>, true) dari token.
-- NULLIF: setelah transaksi berakhir GUC kembali ke '' (bukan NULL), dan ''::uuid akan error.
CREATE FUNCTION app_tenant_id() RETURNS uuid
LANGUAGE sql STABLE AS
$$ SELECT NULLIF(current_setting('app.tenant_id', true), '')::uuid $$;

ALTER TABLE tenants ENABLE ROW LEVEL SECURITY;
ALTER TABLE outlets ENABLE ROW LEVEL SECURITY;
ALTER TABLE roles   ENABLE ROW LEVEL SECURITY;
ALTER TABLE users   ENABLE ROW LEVEL SECURITY;

CREATE POLICY tenant_isolation ON tenants USING (id = app_tenant_id())        WITH CHECK (id = app_tenant_id());
CREATE POLICY tenant_isolation ON outlets USING (tenant_id = app_tenant_id()) WITH CHECK (tenant_id = app_tenant_id());
CREATE POLICY tenant_isolation ON roles   USING (tenant_id = app_tenant_id()) WITH CHECK (tenant_id = app_tenant_id());
CREATE POLICY tenant_isolation ON users   USING (tenant_id = app_tenant_id()) WITH CHECK (tenant_id = app_tenant_id());

GRANT SELECT, INSERT, UPDATE, DELETE ON tenants, outlets, roles, users TO aciraba_app;
GRANT SELECT ON app_settings TO aciraba_app;
-- Tabel baru otomatis bisa diakses aplikasi; tabel tingkat platform yang tak boleh ditulis aplikasi: REVOKE di migrationnya.
ALTER DEFAULT PRIVILEGES IN SCHEMA public GRANT SELECT, INSERT, UPDATE, DELETE ON TABLES TO aciraba_app;

-- Pencarian akun SEBELUM tenant diketahui (login: email; refresh: user id dari Redis). Fungsi ini milik pemilik
-- skema (melewati RLS), sempit (satu akun per panggilan), dan search_path dikunci.
-- +goose StatementBegin
CREATE FUNCTION auth_account_by_email(p_email text)
RETURNS TABLE (
    user_id uuid, user_name text, email text, password_hash text, user_active boolean, role_name text,
    tenant_id uuid, tenant_code text, tenant_name text, tenant_active boolean,
    outlet_id uuid, outlet_code text, outlet_name text
)
LANGUAGE sql STABLE SECURITY DEFINER SET search_path = public, pg_temp AS
$$
    SELECT u.id, u.name, u.email, u.password_hash, u.active, r.name,
           t.id, t.code, t.name, t.active,
           o.id, o.code, o.name
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
    SELECT u.id, u.name, u.email, u.password_hash, u.active, r.name,
           t.id, t.code, t.name, t.active,
           o.id, o.code, o.name
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

-- +goose Down
DROP FUNCTION auth_account_by_id(uuid);
DROP FUNCTION auth_account_by_email(text);
ALTER DEFAULT PRIVILEGES IN SCHEMA public REVOKE SELECT, INSERT, UPDATE, DELETE ON TABLES FROM aciraba_app;
REVOKE ALL ON tenants, outlets, roles, users, app_settings FROM aciraba_app;
DROP POLICY tenant_isolation ON users;
DROP POLICY tenant_isolation ON roles;
DROP POLICY tenant_isolation ON outlets;
DROP POLICY tenant_isolation ON tenants;
ALTER TABLE users   DISABLE ROW LEVEL SECURITY;
ALTER TABLE roles   DISABLE ROW LEVEL SECURITY;
ALTER TABLE outlets DISABLE ROW LEVEL SECURITY;
ALTER TABLE tenants DISABLE ROW LEVEL SECURITY;
DROP FUNCTION app_tenant_id();
