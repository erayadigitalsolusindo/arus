-- +goose Up

-- Sales returns are immutable documents. Point balances may become negative when returned sales had already-earned
-- points that were spent; lifetime points remain non-negative and are reduced only by the returned sale's earned points.
ALTER TABLE members DROP CONSTRAINT members_points_check;
ALTER TABLE member_point_movements DROP CONSTRAINT member_point_movements_balance_after_check;
ALTER TABLE member_point_movements DROP CONSTRAINT member_point_movements_ref_type_check;
ALTER TABLE member_point_movements ADD CONSTRAINT member_point_movements_ref_type_check
    CHECK (ref_type IN ('SALE', 'SALE_RETURN', 'MANUAL'));

CREATE TABLE sales_return_counters (
    tenant_id uuid   NOT NULL,
    outlet_id uuid   NOT NULL,
    day       date   NOT NULL,
    last_no   bigint NOT NULL,
    PRIMARY KEY (tenant_id, outlet_id, day),
    CONSTRAINT sales_return_counters_outlet_fk FOREIGN KEY (tenant_id, outlet_id) REFERENCES outlets (tenant_id, id) ON DELETE RESTRICT
);

CREATE TABLE sales_returns (
    id                 uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id          uuid          NOT NULL,
    outlet_id          uuid          NOT NULL,
    sale_id            uuid          NOT NULL,
    member_id          uuid,
    doc_no             text          NOT NULL CHECK (char_length(doc_no) BETWEEN 1 AND 64),
    idempotency_key    text          NOT NULL CHECK (char_length(idempotency_key) BETWEEN 8 AND 100),
    request_hash       text          NOT NULL,
    return_date        date          NOT NULL,
    note               text          NOT NULL DEFAULT '' CHECK (char_length(note) <= 500),
    subtotal           numeric(18,2) NOT NULL CHECK (subtotal >= 0),
    discount           numeric(18,2) NOT NULL CHECK (discount >= 0),
    tax_amount         numeric(18,2) NOT NULL CHECK (tax_amount >= 0),
    surcharge          numeric(18,2) NOT NULL DEFAULT 0 CHECK (surcharge >= 0),
    total              numeric(18,2) NOT NULL CHECK (total >= 0),
    receivable_cut     numeric(18,2) NOT NULL DEFAULT 0 CHECK (receivable_cut >= 0),
    refund             numeric(18,2) NOT NULL DEFAULT 0 CHECK (refund >= 0),
    refund_method      text          CHECK (refund_method IN ('cash', 'debit', 'credit_card', 'ewallet', 'transfer')),
    refund_method_id   uuid,
    refund_method_name text          NOT NULL DEFAULT '',
    refund_ref         text          NOT NULL DEFAULT '' CHECK (char_length(refund_ref) <= 100),
    points_earned_reversed int       NOT NULL DEFAULT 0 CHECK (points_earned_reversed >= 0),
    points_redeemed_restored int     NOT NULL DEFAULT 0 CHECK (points_redeemed_restored >= 0),
    created_by         uuid,
    created_at         timestamptz   NOT NULL DEFAULT now(),
    CONSTRAINT sales_returns_total_chk CHECK (total = subtotal - discount + tax_amount + surcharge),
    CONSTRAINT sales_returns_allocation_chk CHECK (total = receivable_cut + refund),
    CONSTRAINT sales_returns_refund_chk CHECK ((refund = 0) = (refund_method_id IS NULL) AND (refund_method_id IS NULL) = (refund_method IS NULL)),
    CONSTRAINT sales_returns_tenant_id_id_key UNIQUE (tenant_id, id),
    CONSTRAINT sales_returns_doc_no_key UNIQUE (tenant_id, doc_no),
    CONSTRAINT sales_returns_idem_key UNIQUE (tenant_id, idempotency_key),
    CONSTRAINT sales_returns_outlet_fk FOREIGN KEY (tenant_id, outlet_id) REFERENCES outlets (tenant_id, id) ON DELETE RESTRICT,
    CONSTRAINT sales_returns_sale_fk FOREIGN KEY (tenant_id, sale_id) REFERENCES sales (tenant_id, id) ON DELETE RESTRICT,
    CONSTRAINT sales_returns_member_fk FOREIGN KEY (tenant_id, member_id) REFERENCES members (tenant_id, id) ON DELETE RESTRICT,
    CONSTRAINT sales_returns_method_fk FOREIGN KEY (tenant_id, refund_method_id) REFERENCES payment_methods (tenant_id, id) ON DELETE RESTRICT,
    CONSTRAINT sales_returns_creator_fk FOREIGN KEY (tenant_id, created_by) REFERENCES users (tenant_id, id) ON DELETE RESTRICT
);
CREATE INDEX sales_returns_sale_idx ON sales_returns (tenant_id, sale_id);
CREATE INDEX sales_returns_list_idx ON sales_returns (tenant_id, outlet_id, return_date DESC, created_at DESC, id DESC);

CREATE TABLE sales_return_lines (
    tenant_id       uuid          NOT NULL,
    return_id       uuid          NOT NULL,
    position        int           NOT NULL,
    sale_position   int           NOT NULL,
    item_id         uuid          NOT NULL,
    item_sku        text          NOT NULL,
    item_name       text          NOT NULL,
    unit_name       text          NOT NULL,
    factor          numeric(18,6) NOT NULL CHECK (factor > 0),
    qty             numeric(18,3) NOT NULL CHECK (qty > 0),
    base_qty        numeric(18,3) NOT NULL CHECK (base_qty > 0),
    value           numeric(18,2) NOT NULL CHECK (value >= 0),
    discount        numeric(18,2) NOT NULL CHECK (discount >= 0),
    tax_amount      numeric(18,2) NOT NULL CHECK (tax_amount >= 0),
    unit_cost       numeric(18,2) NOT NULL CHECK (unit_cost >= 0),
    PRIMARY KEY (return_id, position),
    CONSTRAINT sales_return_lines_return_fk FOREIGN KEY (tenant_id, return_id) REFERENCES sales_returns (tenant_id, id) ON DELETE RESTRICT,
    CONSTRAINT sales_return_lines_item_fk FOREIGN KEY (tenant_id, item_id) REFERENCES items (tenant_id, id) ON DELETE RESTRICT
);

ALTER TABLE sales_return_counters ENABLE ROW LEVEL SECURITY;
ALTER TABLE sales_returns ENABLE ROW LEVEL SECURITY;
ALTER TABLE sales_return_lines ENABLE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON sales_return_counters USING (tenant_id = app_tenant_id()) WITH CHECK (tenant_id = app_tenant_id());
CREATE POLICY tenant_isolation ON sales_returns USING (tenant_id = app_tenant_id()) WITH CHECK (tenant_id = app_tenant_id());
CREATE POLICY tenant_isolation ON sales_return_lines USING (tenant_id = app_tenant_id()) WITH CHECK (tenant_id = app_tenant_id());

REVOKE UPDATE, DELETE, TRUNCATE ON sales_returns FROM aciraba_app;
REVOKE DELETE, TRUNCATE ON sales_return_counters FROM aciraba_app;
REVOKE UPDATE, DELETE, TRUNCATE ON sales_return_lines FROM aciraba_app;

-- +goose Down
DROP TABLE sales_return_lines;
DROP TABLE sales_returns;
DROP TABLE sales_return_counters;
ALTER TABLE member_point_movements DROP CONSTRAINT member_point_movements_ref_type_check;
ALTER TABLE member_point_movements ADD CONSTRAINT member_point_movements_ref_type_check CHECK (ref_type IN ('SALE', 'MANUAL'));
ALTER TABLE member_point_movements ADD CONSTRAINT member_point_movements_balance_after_check CHECK (balance_after >= 0);
ALTER TABLE members ADD CONSTRAINT members_points_check CHECK (points >= 0);