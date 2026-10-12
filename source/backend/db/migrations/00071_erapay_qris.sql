-- +goose Up

-- Integrasi EraPay (QRIS). `erapay_methods` menandai satu metode pembayaran per tenant sebagai "EraPay QRIS"
-- (penanda memakai tabel sendiri agar tabel payment_methods tidak berubah). `erapay_charges` = satu tagihan QRIS ke EraPay;
-- nota hanya boleh memakai charge yang SUDAH LUNAS, nominalnya sama, dan belum terpakai nota lain (diperiksa saat nota disimpan).
CREATE TABLE erapay_methods (
    tenant_id  uuid PRIMARY KEY REFERENCES tenants (id) ON DELETE CASCADE,
    method_id  uuid NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE erapay_charges (
    id                uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id         uuid        NOT NULL REFERENCES tenants (id) ON DELETE CASCADE,
    outlet_id         uuid,
    method_id         uuid        NOT NULL,
    idempotency_key   text        NOT NULL CHECK (char_length(idempotency_key) BETWEEN 8 AND 100),
    amount            numeric(18, 2) NOT NULL CHECK (amount > 0),
    status            text        NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'succeeded', 'failed', 'expired', 'cancelled', 'unknown')),
    erapay_payment_id text,
    qr_value          text,
    expires_at        timestamptz,
    sale_id           uuid,
    created_by        uuid        NOT NULL,
    created_at        timestamptz NOT NULL DEFAULT now(),
    updated_at        timestamptz NOT NULL DEFAULT now(),
    UNIQUE (tenant_id, idempotency_key)
);
CREATE UNIQUE INDEX erapay_charges_payment_id_key ON erapay_charges (erapay_payment_id) WHERE erapay_payment_id IS NOT NULL;

ALTER TABLE erapay_methods ENABLE ROW LEVEL SECURITY;
ALTER TABLE erapay_charges ENABLE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON erapay_methods USING (tenant_id = app_tenant_id()) WITH CHECK (tenant_id = app_tenant_id());
CREATE POLICY tenant_isolation ON erapay_charges USING (tenant_id = app_tenant_id()) WITH CHECK (tenant_id = app_tenant_id());

REVOKE DELETE, TRUNCATE ON erapay_charges FROM aciraba_app;

-- +goose Down
DROP TABLE erapay_charges;
DROP TABLE erapay_methods;
