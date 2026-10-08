-- +goose Up

-- Metode pembayaran (master per tenant). `kind` = jenis dasar yang menentukan perilaku di kasir (hanya tunai yang boleh
-- menghasilkan kembalian; non-tunai tidak boleh melebihi total). Nama bebas ("QRIS BCA", "GoPay"). Tidak dihapus:
-- diarsipkan, karena menempel di nota lama. Tunai (is_system) satu per tenant: tak bisa diarsipkan / diganti jenisnya.
CREATE TABLE payment_methods (
    id         uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id  uuid        NOT NULL REFERENCES tenants (id) ON DELETE RESTRICT,
    name       text        NOT NULL CHECK (char_length(name) BETWEEN 1 AND 60),
    kind       text        NOT NULL CHECK (kind IN ('cash', 'debit', 'credit_card', 'ewallet', 'transfer')),
    is_system  boolean     NOT NULL DEFAULT false,
    active     boolean     NOT NULL DEFAULT true,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT payment_methods_tenant_id_id_key UNIQUE (tenant_id, id),
    CONSTRAINT payment_methods_system_cash CHECK (NOT is_system OR (kind = 'cash' AND active))
);
CREATE UNIQUE INDEX payment_methods_tenant_name_key ON payment_methods (tenant_id, lower(name));
-- Hanya satu metode berjenis tunai per tenant (kembalian hanya dari tunai; dua "tunai" membingungkan laci kasir).
CREATE UNIQUE INDEX payment_methods_one_cash_key ON payment_methods (tenant_id) WHERE kind = 'cash';

ALTER TABLE payment_methods ENABLE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON payment_methods USING (tenant_id = app_tenant_id()) WITH CHECK (tenant_id = app_tenant_id());
CREATE TRIGGER payment_methods_set_updated_at BEFORE UPDATE ON payment_methods FOR EACH ROW EXECUTE FUNCTION set_updated_at();
REVOKE DELETE, TRUNCATE ON payment_methods FROM aciraba_app;

-- Metode bawaan untuk tenant yang sudah ada (tenant baru: payment.SeedDefaults saat register).
INSERT INTO payment_methods (tenant_id, name, kind, is_system)
SELECT t.id, d.name, d.kind, d.kind = 'cash'
FROM tenants t
CROSS JOIN (VALUES ('Tunai', 'cash'), ('Transfer', 'transfer'), ('Debit', 'debit'),
                   ('Kartu Kredit', 'credit_card'), ('E-Wallet', 'ewallet')) AS d (name, kind);

-- Pembayaran nota menyimpan metode yang dipilih + snapshot namanya (nota lama tidak berubah bila metode diganti nama).
-- Kolom `method` tetap = jenis dasar, jadi laporan per jenis tidak berubah.
ALTER TABLE sale_payments ADD COLUMN method_id uuid, ADD COLUMN method_name text;
UPDATE sale_payments p SET method_id = m.id, method_name = m.name
FROM payment_methods m WHERE m.tenant_id = p.tenant_id AND m.kind = p.method AND m.name = CASE p.method
    WHEN 'cash' THEN 'Tunai' WHEN 'transfer' THEN 'Transfer' WHEN 'debit' THEN 'Debit'
    WHEN 'credit_card' THEN 'Kartu Kredit' ELSE 'E-Wallet' END;
ALTER TABLE sale_payments ALTER COLUMN method_id SET NOT NULL, ALTER COLUMN method_name SET NOT NULL;
ALTER TABLE sale_payments ADD CONSTRAINT sale_payments_method_name_len CHECK (char_length(method_name) BETWEEN 1 AND 60);
ALTER TABLE sale_payments ADD CONSTRAINT sale_payments_method_fk FOREIGN KEY (tenant_id, method_id)
    REFERENCES payment_methods (tenant_id, id) ON DELETE RESTRICT;
CREATE INDEX sale_payments_method_idx ON sale_payments (tenant_id, method_id);

-- +goose Down
ALTER TABLE sale_payments DROP CONSTRAINT sale_payments_method_fk;
ALTER TABLE sale_payments DROP CONSTRAINT sale_payments_method_name_len;
ALTER TABLE sale_payments DROP COLUMN method_id, DROP COLUMN method_name;
DROP TABLE payment_methods;
