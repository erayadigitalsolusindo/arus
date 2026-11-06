-- +goose Up

-- Identitas login pihak ketiga (Google). Satu pengguna boleh punya satu identitas per penyedia; satu identitas
-- (provider, subject) hanya milik satu pengguna di seluruh platform. Dicari sebelum tenant diketahui (seperti
-- auth_account_by_email), jadi tabel tanpa hak bagi aciraba_app dan diakses lewat fungsi SECURITY DEFINER.
CREATE TABLE user_identities (
    provider   text        NOT NULL CHECK (provider IN ('google')),
    subject    text        NOT NULL CHECK (char_length(subject) BETWEEN 1 AND 255),
    tenant_id  uuid        NOT NULL,
    user_id    uuid        NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (provider, subject),
    CONSTRAINT user_identities_user_fk FOREIGN KEY (tenant_id, user_id) REFERENCES users (tenant_id, id) ON DELETE CASCADE,
    CONSTRAINT user_identities_user_provider_key UNIQUE (user_id, provider)
);
CREATE INDEX user_identities_tenant_idx ON user_identities (tenant_id);

REVOKE ALL ON user_identities FROM PUBLIC;

-- +goose StatementBegin
CREATE FUNCTION auth_identity_user(p_provider text, p_subject text)
RETURNS uuid
LANGUAGE sql STABLE SECURITY DEFINER SET search_path = public, pg_temp AS
$$
    SELECT user_id FROM user_identities WHERE provider = p_provider AND subject = p_subject
$$;

-- Menautkan identitas ke pengguna. Mengembalikan true bila tertaut (baru atau sudah milik pengguna ini),
-- false bila identitas itu sudah dipakai pengguna lain atau pengguna ini sudah punya identitas lain di penyedia yang sama.
CREATE FUNCTION auth_identity_link(p_provider text, p_subject text, p_tenant_id uuid, p_user_id uuid)
RETURNS boolean
LANGUAGE plpgsql SECURITY DEFINER SET search_path = public, pg_temp AS
$$
BEGIN
    INSERT INTO user_identities (provider, subject, tenant_id, user_id)
    VALUES (p_provider, p_subject, p_tenant_id, p_user_id)
    ON CONFLICT DO NOTHING;
    RETURN EXISTS (SELECT 1 FROM user_identities WHERE provider = p_provider AND subject = p_subject AND user_id = p_user_id);
END
$$;
-- +goose StatementEnd

REVOKE ALL ON FUNCTION auth_identity_user(text, text), auth_identity_link(text, text, uuid, uuid) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION auth_identity_user(text, text), auth_identity_link(text, text, uuid, uuid) TO aciraba_app;

-- +goose Down
DROP FUNCTION auth_identity_link(text, text, uuid, uuid);
DROP FUNCTION auth_identity_user(text, text);
DROP TABLE user_identities;
