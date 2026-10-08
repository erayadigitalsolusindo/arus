-- name: PaymentMethodList :many
SELECT id, name, kind, is_system, active, fee_pct, fee_flat, fee_bearer, created_at,
       count(*) OVER () AS total
FROM payment_methods
WHERE tenant_id = @tenant_id
  AND (@q::text = '' OR name ILIKE '%' || @q || '%')
  AND (sqlc.narg('active')::boolean IS NULL OR active = sqlc.narg('active'))
ORDER BY is_system DESC, lower(name), id
LIMIT @page_limit OFFSET @page_offset;

-- name: PaymentMethodGetForUpdate :one
SELECT id, name, kind, is_system, active, fee_pct, fee_flat, fee_bearer, created_at
FROM payment_methods WHERE tenant_id = $1 AND id = $2 FOR UPDATE;

-- name: PaymentMethodCount :one
SELECT count(*) FROM payment_methods WHERE tenant_id = $1;

-- name: PaymentMethodCreate :one
INSERT INTO payment_methods (tenant_id, name, kind, is_system, fee_pct, fee_flat, fee_bearer)
VALUES ($1, $2, $3, $4, $5, $6, $7)
RETURNING id, name, kind, is_system, active, fee_pct, fee_flat, fee_bearer, created_at;

-- name: PaymentMethodUpdate :one
UPDATE payment_methods SET name = $3, fee_pct = $4, fee_flat = $5, fee_bearer = $6, kind = $7
WHERE tenant_id = $1 AND id = $2
RETURNING id, name, kind, is_system, active, fee_pct, fee_flat, fee_bearer, created_at;

-- name: PaymentMethodSetActive :one
UPDATE payment_methods SET active = $3
WHERE tenant_id = $1 AND id = $2
RETURNING id, name, kind, is_system, active, fee_pct, fee_flat, fee_bearer, created_at;

-- name: PaymentMethodActiveList :many
SELECT id, name, kind, is_system, fee_pct, fee_flat, fee_bearer
FROM payment_methods
WHERE tenant_id = $1 AND active
ORDER BY is_system DESC, lower(name), id;
