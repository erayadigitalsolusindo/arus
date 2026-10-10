-- +goose Up

CREATE TABLE user_feature_shortcuts (
    tenant_id  uuid        NOT NULL,
    user_id    uuid        NOT NULL,
    shortcuts  jsonb       NOT NULL DEFAULT '[]'::jsonb CHECK (jsonb_typeof(shortcuts) = 'array'),
    updated_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (tenant_id, user_id),
    FOREIGN KEY (tenant_id, user_id) REFERENCES users (tenant_id, id) ON DELETE CASCADE
);

ALTER TABLE user_feature_shortcuts ENABLE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON user_feature_shortcuts
    USING (tenant_id = app_tenant_id()) WITH CHECK (tenant_id = app_tenant_id());

-- +goose Down

DROP TABLE user_feature_shortcuts;