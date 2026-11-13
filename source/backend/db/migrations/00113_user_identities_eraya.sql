-- +goose Up

-- Login SSO Eraya (OIDC) memakai tabel identitas yang sama dengan Google.
ALTER TABLE user_identities DROP CONSTRAINT user_identities_provider_check;
ALTER TABLE user_identities ADD CONSTRAINT user_identities_provider_check CHECK (provider IN ('google', 'eraya'));

-- +goose Down
DELETE FROM user_identities WHERE provider = 'eraya';
ALTER TABLE user_identities DROP CONSTRAINT user_identities_provider_check;
ALTER TABLE user_identities ADD CONSTRAINT user_identities_provider_check CHECK (provider IN ('google'));
