-- +goose Up

-- Trigger generik: isi updated_at otomatis (AGENTS.md §1: trigger hanya untuk updated_at/audit generik).
-- +goose StatementBegin
CREATE FUNCTION set_updated_at() RETURNS trigger
LANGUAGE plpgsql AS $$
BEGIN
    NEW.updated_at = now();
    RETURN NEW;
END;
$$;
-- +goose StatementEnd

CREATE TABLE tenants (
    id         uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    code       text        NOT NULL,
    name       text        NOT NULL,
    active     boolean     NOT NULL DEFAULT true,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT tenants_code_key UNIQUE (code),
    CONSTRAINT tenants_code_format CHECK (code ~ '^[a-z0-9][a-z0-9_-]{1,31}$')
);

CREATE TABLE outlets (
    id            uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id     uuid          NOT NULL REFERENCES tenants (id) ON DELETE RESTRICT,
    code          text          NOT NULL,
    name          text          NOT NULL,
    tax_store_pct numeric(5, 2) NOT NULL DEFAULT 0 CHECK (tax_store_pct BETWEEN 0 AND 100),
    tax_gov_pct   numeric(5, 2) NOT NULL DEFAULT 0 CHECK (tax_gov_pct BETWEEN 0 AND 100),
    timezone      text          NOT NULL DEFAULT 'Asia/Jakarta',
    active        boolean       NOT NULL DEFAULT true,
    created_at    timestamptz   NOT NULL DEFAULT now(),
    updated_at    timestamptz   NOT NULL DEFAULT now(),
    CONSTRAINT outlets_tenant_code_key UNIQUE (tenant_id, code),
    -- Target FK komposit dari tabel lain agar baris anak tidak bisa menunjuk outlet milik tenant berbeda.
    CONSTRAINT outlets_tenant_id_id_key UNIQUE (tenant_id, id)
);

CREATE TABLE roles (
    id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id   uuid        NOT NULL REFERENCES tenants (id) ON DELETE RESTRICT,
    name        text        NOT NULL,
    permissions jsonb       NOT NULL DEFAULT '{}'::jsonb,
    is_system   boolean     NOT NULL DEFAULT false,
    created_at  timestamptz NOT NULL DEFAULT now(),
    updated_at  timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT roles_tenant_name_key UNIQUE (tenant_id, name),
    CONSTRAINT roles_tenant_id_id_key UNIQUE (tenant_id, id)
);

CREATE TABLE users (
    id            uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id     uuid        NOT NULL REFERENCES tenants (id) ON DELETE RESTRICT,
    role_id       uuid        NOT NULL,
    email         text        NOT NULL,
    name          text        NOT NULL,
    password_hash text        NOT NULL,
    pin_hash      text,
    active        boolean     NOT NULL DEFAULT true,
    last_login_at timestamptz,
    created_at    timestamptz NOT NULL DEFAULT now(),
    updated_at    timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT users_tenant_id_id_key UNIQUE (tenant_id, id),
    -- Role harus milik tenant yang sama dengan user.
    CONSTRAINT users_role_fk FOREIGN KEY (tenant_id, role_id) REFERENCES roles (tenant_id, id) ON DELETE RESTRICT
);

-- Login hanya memakai email + password (tanpa kode tenant), jadi email harus unik global (case-insensitive).
CREATE UNIQUE INDEX users_email_key ON users (lower(email));
CREATE INDEX users_tenant_idx ON users (tenant_id);

CREATE TRIGGER tenants_set_updated_at BEFORE UPDATE ON tenants FOR EACH ROW EXECUTE FUNCTION set_updated_at();
CREATE TRIGGER outlets_set_updated_at BEFORE UPDATE ON outlets FOR EACH ROW EXECUTE FUNCTION set_updated_at();
CREATE TRIGGER roles_set_updated_at   BEFORE UPDATE ON roles   FOR EACH ROW EXECUTE FUNCTION set_updated_at();
CREATE TRIGGER users_set_updated_at   BEFORE UPDATE ON users   FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- +goose Down
DROP TABLE users;
DROP TABLE roles;
DROP TABLE outlets;
DROP TABLE tenants;
DROP FUNCTION set_updated_at();
