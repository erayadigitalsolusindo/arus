-- name: VoucherList :many
SELECT id, code, name, kind, value, max_discount, min_spend, starts_on, ends_on, max_uses, used_count, active, created_at,
       count(*) OVER () AS total
FROM vouchers
WHERE tenant_id = @tenant_id
  AND (@q::text = '' OR name ILIKE '%' || @q || '%' OR code ILIKE '%' || @q || '%')
  AND (sqlc.narg('active')::boolean IS NULL OR active = sqlc.narg('active'))
ORDER BY created_at DESC, id
LIMIT @page_limit OFFSET @page_offset;

-- name: VoucherGet :one
SELECT id, code, name, kind, value, max_discount, min_spend, starts_on, ends_on, max_uses, used_count, active, created_at
FROM vouchers WHERE tenant_id = $1 AND id = $2
FOR UPDATE;

-- name: VoucherCreate :one
INSERT INTO vouchers (tenant_id, code, name, kind, value, max_discount, min_spend, starts_on, ends_on, max_uses)
VALUES (@tenant_id, @code, @name, @kind, @value, sqlc.narg('max_discount'), @min_spend, sqlc.narg('starts_on'), sqlc.narg('ends_on'), sqlc.narg('max_uses'))
RETURNING id, code, name, kind, value, max_discount, min_spend, starts_on, ends_on, max_uses, used_count, active, created_at;

-- name: VoucherUpdate :one
UPDATE vouchers SET code = @code, name = @name, kind = @kind, value = @value, max_discount = sqlc.narg('max_discount'),
       min_spend = @min_spend, starts_on = sqlc.narg('starts_on'), ends_on = sqlc.narg('ends_on'), max_uses = sqlc.narg('max_uses')
WHERE tenant_id = @tenant_id AND id = @id
RETURNING id, code, name, kind, value, max_discount, min_spend, starts_on, ends_on, max_uses, used_count, active, created_at;

-- name: VoucherSetActive :one
UPDATE vouchers SET active = $3 WHERE tenant_id = $1 AND id = $2
RETURNING id, code, name, kind, value, max_discount, min_spend, starts_on, ends_on, max_uses, used_count, active, created_at;

-- name: VoucherByCodes :many
-- Pratinjau (quote): tanpa kunci.
SELECT id, code, name, kind, value, max_discount, min_spend, starts_on, ends_on, max_uses, used_count, active
FROM vouchers WHERE tenant_id = @tenant_id AND code = ANY(@codes::text[])
ORDER BY id;

-- name: VoucherByCodesLock :many
-- Simpan nota: baris dikunci terurut id (nota yang berebut kupon sama antre, tanpa deadlock) sehingga kuota tak terlampaui.
SELECT id, code, name, kind, value, max_discount, min_spend, starts_on, ends_on, max_uses, used_count, active
FROM vouchers WHERE tenant_id = @tenant_id AND code = ANY(@codes::text[])
ORDER BY id
FOR UPDATE;

-- name: VoucherUse :execrows
UPDATE vouchers SET used_count = used_count + 1
WHERE tenant_id = @tenant_id AND id = @id AND (max_uses IS NULL OR used_count < max_uses);

-- name: VoucherRelease :execrows
-- Mengembalikan satu pemakaian kupon (nota dibatalkan/diedit). Tidak pernah di bawah 0.
UPDATE vouchers SET used_count = used_count - 1
WHERE tenant_id = @tenant_id AND id = @id AND used_count > 0;
