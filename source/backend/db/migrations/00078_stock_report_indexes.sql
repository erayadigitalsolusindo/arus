-- +goose Up

-- Indeks untuk Laporan Persedian Item: filter outlet + item + bucket + urutan id DESC
-- (DISTINCT ON per item_id, bucket dengan ORDER BY id DESC)
CREATE INDEX stock_movements_report_idx ON stock_movements (tenant_id, outlet_id, item_id, bucket, id DESC);

-- +goose Down

DROP INDEX stock_movements_report_idx;
