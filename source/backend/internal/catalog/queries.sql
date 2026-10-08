-- name: SupplierList :many
SELECT id, code, name, contact_name, phone, email, address, note, active, created_at,
       count(*) OVER () AS total
FROM suppliers
WHERE tenant_id = @tenant_id
  AND (@q::text = '' OR name ILIKE '%' || @q || '%' OR coalesce(code, '') ILIKE '%' || @q || '%')
  AND (sqlc.narg('active')::boolean IS NULL OR active = sqlc.narg('active'))
ORDER BY lower(name), id
LIMIT @page_limit OFFSET @page_offset;

-- name: SupplierGet :one
SELECT id, code, name, contact_name, phone, email, address, note, active, created_at
FROM suppliers WHERE tenant_id = $1 AND id = $2;

-- name: SupplierCreate :one
INSERT INTO suppliers (tenant_id, code, name, contact_name, phone, email, address, note)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
RETURNING id, code, name, contact_name, phone, email, address, note, active, created_at;

-- name: SupplierUpdate :one
UPDATE suppliers SET code = $3, name = $4, contact_name = $5, phone = $6, email = $7, address = $8, note = $9
WHERE tenant_id = $1 AND id = $2
RETURNING id, code, name, contact_name, phone, email, address, note, active, created_at;

-- name: SupplierSetActive :one
UPDATE suppliers SET active = $3
WHERE tenant_id = $1 AND id = $2
RETURNING id, code, name, contact_name, phone, email, address, note, active, created_at;

-- name: SalespersonList :many
SELECT id, code, name, phone, note, commission_pct, active, created_at,
       count(*) OVER () AS total
FROM salespeople
WHERE tenant_id = @tenant_id
  AND (@q::text = '' OR name ILIKE '%' || @q || '%' OR coalesce(code, '') ILIKE '%' || @q || '%')
  AND (sqlc.narg('active')::boolean IS NULL OR active = sqlc.narg('active'))
ORDER BY lower(name), id
LIMIT @page_limit OFFSET @page_offset;

-- name: SalespersonGet :one
SELECT id, code, name, phone, note, commission_pct, active, created_at
FROM salespeople WHERE tenant_id = $1 AND id = $2;

-- name: SalespersonCreate :one
INSERT INTO salespeople (tenant_id, code, name, phone, note, commission_pct)
VALUES ($1, $2, $3, $4, $5, $6)
RETURNING id, code, name, phone, note, commission_pct, active, created_at;

-- name: SalespersonUpdate :one
UPDATE salespeople SET code = $3, name = $4, phone = $5, note = $6, commission_pct = $7
WHERE tenant_id = $1 AND id = $2
RETURNING id, code, name, phone, note, commission_pct, active, created_at;

-- name: SalespersonSetActive :one
UPDATE salespeople SET active = $3
WHERE tenant_id = $1 AND id = $2
RETURNING id, code, name, phone, note, commission_pct, active, created_at;
