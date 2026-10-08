-- +goose Up

-- Rincian biaya lain-lain per nota (ongkir, bungkus, dll.). `sales.other_cost` tetap total resmi (= Σ amount bila ada rincian);
-- tabel ini hanya menjelaskan isinya. Append-only seperti sale_lines: koreksi nota = dokumen baru, bukan ubah baris.
CREATE TABLE sale_costs (
    tenant_id uuid          NOT NULL,
    sale_id   uuid          NOT NULL,
    position  int           NOT NULL,
    name      text          NOT NULL DEFAULT '' CHECK (char_length(name) <= 80),
    amount    numeric(18,2) NOT NULL CHECK (amount > 0),
    PRIMARY KEY (sale_id, position),
    CONSTRAINT sale_costs_sale_fk FOREIGN KEY (tenant_id, sale_id) REFERENCES sales (tenant_id, id) ON DELETE RESTRICT
);

ALTER TABLE sale_costs ENABLE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON sale_costs USING (tenant_id = app_tenant_id()) WITH CHECK (tenant_id = app_tenant_id());
REVOKE UPDATE, DELETE, TRUNCATE ON sale_costs FROM aciraba_app;

-- +goose Down
DROP TABLE sale_costs;
