-- name: OutletList :many
SELECT id, code, name, tax_store_pct, tax_gov_pct, timezone, active, created_at
FROM outlets WHERE tenant_id = $1 ORDER BY created_at, code;

-- name: OutletGet :one
SELECT id, code, name, tax_store_pct, tax_gov_pct, timezone, active, created_at
FROM outlets WHERE tenant_id = $1 AND id = $2;

-- name: OutletCreate :one
INSERT INTO outlets (tenant_id, code, name, tax_store_pct, tax_gov_pct, timezone)
VALUES ($1, $2, $3, $4, $5, $6)
RETURNING id, code, name, tax_store_pct, tax_gov_pct, timezone, active, created_at;

-- name: OutletUpdate :one
UPDATE outlets SET name = $3, tax_store_pct = $4, tax_gov_pct = $5, timezone = $6, active = $7
WHERE tenant_id = $1 AND id = $2
RETURNING id, code, name, tax_store_pct, tax_gov_pct, timezone, active, created_at;

-- name: OutletLockActive :many
-- Mengunci baris outlet aktif selama transaksi: dua penonaktifan bersamaan tidak bisa sama-sama menyisakan nol outlet.
SELECT id FROM outlets WHERE tenant_id = $1 AND active FOR UPDATE;

-- name: OutletAssignUser :exec
INSERT INTO user_outlets (tenant_id, user_id, outlet_id) VALUES ($1, $2, $3)
ON CONFLICT DO NOTHING;
