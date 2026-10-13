-- +goose Up

CREATE TABLE bank_reconciliations (
    id                  uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id           uuid NOT NULL,
    outlet_id           uuid NOT NULL,
    account_id          uuid NOT NULL,
    period_month        date NOT NULL CHECK (period_month = date_trunc('month', period_month)::date),
    statement_opening   numeric(18,2) NOT NULL CHECK (statement_opening >= 0 AND statement_opening < 1e15),
    statement_closing   numeric(18,2) NOT NULL CHECK (statement_closing >= 0 AND statement_closing < 1e15),
    status              text NOT NULL DEFAULT 'draft' CHECK (status IN ('draft', 'completed')),
    created_by          uuid NOT NULL,
    completed_by        uuid,
    completed_at        timestamptz,
    created_at          timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT bank_reconciliations_tenant_id_id_key UNIQUE (tenant_id, id),
    CONSTRAINT bank_reconciliations_scope_key UNIQUE (tenant_id, outlet_id, account_id, period_month),
    CONSTRAINT bank_reconciliations_outlet_fk FOREIGN KEY (tenant_id, outlet_id) REFERENCES outlets (tenant_id, id) ON DELETE RESTRICT,
    CONSTRAINT bank_reconciliations_account_fk FOREIGN KEY (tenant_id, account_id) REFERENCES accounts (tenant_id, id) ON DELETE RESTRICT,
    CONSTRAINT bank_reconciliations_creator_fk FOREIGN KEY (tenant_id, created_by) REFERENCES users (tenant_id, id) ON DELETE RESTRICT,
    CONSTRAINT bank_reconciliations_completer_fk FOREIGN KEY (tenant_id, completed_by) REFERENCES users (tenant_id, id) ON DELETE RESTRICT,
    CONSTRAINT bank_reconciliations_state_chk CHECK (
        (status = 'draft' AND completed_by IS NULL AND completed_at IS NULL)
        OR (status = 'completed' AND completed_by IS NOT NULL AND completed_at IS NOT NULL))
);
CREATE INDEX bank_reconciliations_account_idx ON bank_reconciliations (tenant_id, outlet_id, account_id, period_month DESC);

CREATE TABLE bank_reconciliation_matches (
    tenant_id           uuid NOT NULL,
    reconciliation_id   uuid NOT NULL,
    journal_line_id     uuid NOT NULL,
    matched_at          timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (tenant_id, reconciliation_id, journal_line_id),
    CONSTRAINT bank_reconciliation_matches_once UNIQUE (tenant_id, journal_line_id),
    CONSTRAINT bank_reconciliation_matches_recon_fk FOREIGN KEY (tenant_id, reconciliation_id)
        REFERENCES bank_reconciliations (tenant_id, id) ON DELETE CASCADE,
    CONSTRAINT bank_reconciliation_matches_line_fk FOREIGN KEY (journal_line_id)
        REFERENCES journal_lines (id) ON DELETE RESTRICT
);
CREATE INDEX bank_reconciliation_matches_line_idx ON bank_reconciliation_matches (tenant_id, journal_line_id);

ALTER TABLE bank_reconciliations ENABLE ROW LEVEL SECURITY;
ALTER TABLE bank_reconciliation_matches ENABLE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON bank_reconciliations USING (tenant_id = app_tenant_id()) WITH CHECK (tenant_id = app_tenant_id());
CREATE POLICY tenant_isolation ON bank_reconciliation_matches USING (tenant_id = app_tenant_id()) WITH CHECK (tenant_id = app_tenant_id());
CREATE POLICY draft_only_update ON bank_reconciliations AS RESTRICTIVE FOR UPDATE USING (status = 'draft') WITH CHECK (true);
CREATE POLICY draft_only_insert ON bank_reconciliation_matches AS RESTRICTIVE FOR INSERT
    WITH CHECK (EXISTS (SELECT 1 FROM bank_reconciliations r WHERE r.tenant_id = bank_reconciliation_matches.tenant_id
        AND r.id = bank_reconciliation_matches.reconciliation_id AND r.status = 'draft'));
CREATE POLICY draft_only_delete ON bank_reconciliation_matches AS RESTRICTIVE FOR DELETE
    USING (EXISTS (SELECT 1 FROM bank_reconciliations r WHERE r.tenant_id = bank_reconciliation_matches.tenant_id
        AND r.id = bank_reconciliation_matches.reconciliation_id AND r.status = 'draft'));

REVOKE DELETE, TRUNCATE ON bank_reconciliations FROM aciraba_app;
REVOKE UPDATE, TRUNCATE ON bank_reconciliation_matches FROM aciraba_app;
GRANT SELECT, INSERT ON bank_reconciliations TO aciraba_app;
GRANT UPDATE (statement_opening, statement_closing, status, completed_by, completed_at) ON bank_reconciliations TO aciraba_app;
GRANT SELECT, INSERT, DELETE ON bank_reconciliation_matches TO aciraba_app;

-- +goose Down
DROP TABLE bank_reconciliation_matches;
DROP TABLE bank_reconciliations;