-- +goose Up

-- Template jurnal buatan pengguna (dipakai bersama satu tenant): kerangka jurnal (jenis, keterangan, akun + sisi), tanpa nominal.
-- Template bukan data pembukuan: boleh diubah/dihapus bebas. Akun yang dihapus ikut menghapus barisnya (CASCADE).
CREATE TABLE journal_templates (
    id         uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id  uuid        NOT NULL,
    name       text        NOT NULL CHECK (char_length(name) BETWEEN 1 AND 80),
    type       text        NOT NULL CHECK (type IN ('JU', 'KM', 'KK', 'TK')),
    narration  text        NOT NULL DEFAULT '' CHECK (char_length(narration) <= 500),
    created_by uuid        NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT journal_templates_tenant_id_id_key UNIQUE (tenant_id, id),
    CONSTRAINT journal_templates_creator_fk FOREIGN KEY (tenant_id, created_by) REFERENCES users (tenant_id, id) ON DELETE RESTRICT
);
CREATE UNIQUE INDEX journal_templates_name_key ON journal_templates (tenant_id, lower(name));

CREATE TABLE journal_template_lines (
    tenant_id   uuid NOT NULL,
    template_id uuid NOT NULL,
    line_no     int  NOT NULL CHECK (line_no >= 1),
    account_id  uuid NOT NULL,
    side        text NOT NULL CHECK (side IN ('debit', 'credit')),
    memo        text NOT NULL DEFAULT '' CHECK (char_length(memo) <= 200),
    PRIMARY KEY (tenant_id, template_id, line_no),
    CONSTRAINT journal_template_lines_tpl_fk FOREIGN KEY (tenant_id, template_id) REFERENCES journal_templates (tenant_id, id) ON DELETE CASCADE,
    CONSTRAINT journal_template_lines_account_fk FOREIGN KEY (tenant_id, account_id) REFERENCES accounts (tenant_id, id) ON DELETE CASCADE
);
CREATE INDEX journal_template_lines_account_idx ON journal_template_lines (tenant_id, account_id);

ALTER TABLE journal_templates      ENABLE ROW LEVEL SECURITY;
ALTER TABLE journal_template_lines ENABLE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON journal_templates      USING (tenant_id = app_tenant_id()) WITH CHECK (tenant_id = app_tenant_id());
CREATE POLICY tenant_isolation ON journal_template_lines USING (tenant_id = app_tenant_id()) WITH CHECK (tenant_id = app_tenant_id());
REVOKE TRUNCATE ON journal_templates, journal_template_lines FROM aciraba_app;

-- +goose Down
DROP TABLE journal_template_lines;
DROP TABLE journal_templates;
