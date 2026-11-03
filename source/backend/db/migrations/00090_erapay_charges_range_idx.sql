-- +goose Up

-- Layar Settlement EraPay membaca tagihan per rentang waktu (tenant, created_at). Tanpa indeks ini kueri memindai semua
-- tagihan tenant; dengan indeks tetap cepat walau tagihan mencapai jutaan baris.
CREATE INDEX erapay_charges_tenant_created_idx ON erapay_charges (tenant_id, created_at DESC);

-- +goose Down
DROP INDEX erapay_charges_tenant_created_idx;
