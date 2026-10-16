-- name: PaymentMethodList :many
SELECT m.id, m.name, m.kind, m.is_system, m.active, m.fee_pct, m.fee_flat, m.fee_bearer, m.small_tx_limit, m.small_tx_flat, m.created_at,
       m.account_id, a.code AS account_code, a.name AS account_name,
       count(*) OVER () AS total
FROM payment_methods m
LEFT JOIN accounts a ON a.tenant_id = m.tenant_id AND a.id = m.account_id
WHERE m.tenant_id = @tenant_id
  AND (@q::text = '' OR m.name ILIKE '%' || @q || '%')
  AND (sqlc.narg('active')::boolean IS NULL OR m.active = sqlc.narg('active'))
ORDER BY (m.kind <> 'cash'), m.is_system DESC, lower(m.name), m.id
LIMIT @page_limit OFFSET @page_offset;

-- name: PaymentMethodGetForUpdate :one
SELECT id, name, kind, is_system, active, fee_pct, fee_flat, fee_bearer, small_tx_limit, small_tx_flat, created_at
FROM payment_methods WHERE tenant_id = $1 AND id = $2 FOR UPDATE;

-- name: PaymentMethodCount :one
SELECT count(*) FROM payment_methods WHERE tenant_id = $1;

-- name: PaymentMethodCreate :one
INSERT INTO payment_methods (tenant_id, name, kind, is_system, fee_pct, fee_flat, fee_bearer, small_tx_limit, small_tx_flat)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
RETURNING id, name, kind, is_system, active, fee_pct, fee_flat, fee_bearer, small_tx_limit, small_tx_flat, created_at;

-- name: PaymentMethodUpdate :one
UPDATE payment_methods SET name = $3, fee_pct = $4, fee_flat = $5, fee_bearer = $6, kind = $7, small_tx_limit = $8, small_tx_flat = $9
WHERE tenant_id = $1 AND id = $2
RETURNING id, name, kind, is_system, active, fee_pct, fee_flat, fee_bearer, small_tx_limit, small_tx_flat, created_at;

-- name: PaymentMethodSetActive :one
UPDATE payment_methods SET active = $3
WHERE tenant_id = $1 AND id = $2
RETURNING id, name, kind, is_system, active, fee_pct, fee_flat, fee_bearer, small_tx_limit, small_tx_flat, created_at;

-- name: PaymentMethodActiveList :many
SELECT id, name, kind, is_system, fee_pct, fee_flat, fee_bearer, small_tx_limit, small_tx_flat
FROM payment_methods
WHERE tenant_id = $1 AND active
ORDER BY (kind <> 'cash'), is_system DESC, lower(name), id;

-- name: PaymentMethodAccountGet :one
SELECT m.account_id, a.code AS account_code, a.name AS account_name
FROM payment_methods m
LEFT JOIN accounts a ON a.tenant_id = m.tenant_id AND a.id = m.account_id
WHERE m.tenant_id = $1 AND m.id = $2;

-- name: PaymentMethodSetAccount :exec
UPDATE payment_methods SET account_id = $3 WHERE tenant_id = $1 AND id = $2;

-- Berapa metode LAIN yang memakai akun yang sama (akun bersama tidak ikut diganti nama / dinonaktifkan).
-- name: PaymentMethodAccountOtherUsers :one
SELECT count(*) FROM payment_methods WHERE tenant_id = $1 AND account_id = $2 AND id <> $3;

-- name: PaymentMethodSetAccountActive :exec
UPDATE accounts SET active = $3 WHERE tenant_id = $1 AND id = $2 AND NOT is_system;

-- name: PaymentMethodRenameAccount :exec
UPDATE accounts SET name = $3 WHERE tenant_id = $1 AND id = $2 AND NOT is_system;

-- Dipakai saat template retail dipasang: hubungkan metode yang belum punya akun ke akun bawaan menurut jenis.
-- name: PaymentMethodLinkDefaultAccounts :exec
UPDATE payment_methods m SET account_id = a.id
FROM accounts a
WHERE m.tenant_id = $1 AND m.account_id IS NULL AND a.tenant_id = m.tenant_id AND a.kind = 'ledger' AND a.code = CASE m.kind
    WHEN 'cash' THEN '1110' WHEN 'transfer' THEN '1120' WHEN 'debit' THEN '1120' WHEN 'ewallet' THEN '1130'
    WHEN 'credit_card' THEN '1140' WHEN 'deposit' THEN '2140' WHEN 'supplier_credit' THEN '1320' END;
