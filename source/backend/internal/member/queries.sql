-- ===== Level member =====

-- name: MemberLevelList :many
-- member_count = member aktif yang saat ini berada di level itu (level tiap member ditetapkan sekali lewat LATERAL,
-- lalu diagregasi; tidak ada subquery berkorelasi per level).
SELECT l.id, l.name, l.min_points, l.spend_per_point, l.point_value, l.active, l.created_at,
       coalesce(c.n, 0)::bigint AS member_count
FROM member_levels l
LEFT JOIN (
    SELECT lv.id, count(*) AS n
    FROM members m
    JOIN LATERAL (
        SELECT h.id FROM member_levels h
        WHERE h.tenant_id = m.tenant_id AND h.active AND h.min_points <= m.lifetime_points
        ORDER BY h.min_points DESC LIMIT 1
    ) lv ON true
    WHERE m.tenant_id = @tenant_id AND m.active
    GROUP BY lv.id
) c ON c.id = l.id
WHERE l.tenant_id = @tenant_id AND (sqlc.narg('active')::boolean IS NULL OR l.active = sqlc.narg('active'))
ORDER BY l.min_points;

-- name: MemberLevelGetForUpdate :one
SELECT id, name, min_points, spend_per_point, point_value, active, created_at
FROM member_levels WHERE tenant_id = $1 AND id = $2 FOR UPDATE;

-- name: MemberLevelCreate :one
INSERT INTO member_levels (tenant_id, name, min_points, spend_per_point, point_value)
VALUES ($1, $2, $3, $4, $5)
RETURNING id, name, min_points, spend_per_point, point_value, active, created_at;

-- name: MemberLevelUpdate :one
UPDATE member_levels SET name = $3, min_points = $4, spend_per_point = $5, point_value = $6
WHERE tenant_id = $1 AND id = $2
RETURNING id, name, min_points, spend_per_point, point_value, active, created_at;

-- name: MemberLevelSetActive :one
UPDATE member_levels SET active = $3
WHERE tenant_id = $1 AND id = $2
RETURNING id, name, min_points, spend_per_point, point_value, active, created_at;

-- ===== Member =====

-- name: MemberNextNo :one
INSERT INTO member_counters (tenant_id, last_no) VALUES ($1, 1)
ON CONFLICT (tenant_id) DO UPDATE SET last_no = member_counters.last_no + 1
RETURNING last_no;

-- name: MemberCodeExists :one
-- Lewat fungsi SECURITY DEFINER (00052), alasan sama dengan ItemSkuExists. Tenant = app_tenant_id() transaksi.
SELECT member_code_taken(@code::text)::boolean AS taken;

-- name: MemberList :many
SELECT m.id, m.code, m.name, m.phone, m.city, m.active, m.valid_until, m.points, m.lifetime_points, m.cover_image_id,
       coalesce(lv.name, '')::text AS level_name, count(*) OVER () AS total
FROM members m
LEFT JOIN LATERAL (
    SELECT l.name FROM member_levels l
    WHERE l.tenant_id = m.tenant_id AND l.active AND l.min_points <= m.lifetime_points
    ORDER BY l.min_points DESC LIMIT 1
) lv ON true
WHERE m.tenant_id = @tenant_id
  AND (@q::text = '' OR m.name ILIKE '%' || @q || '%' OR m.code ILIKE '%' || @q || '%' OR m.phone ILIKE '%' || @q || '%'
       OR (@q_phone::text <> '' AND m.phone ILIKE '%' || @q_phone || '%'))
  AND (sqlc.narg('active')::boolean IS NULL OR m.active = sqlc.narg('active'))
ORDER BY lower(m.name), m.id
LIMIT @page_limit OFFSET @page_offset;

-- name: MemberGet :one
SELECT m.id, m.code, m.name, m.gender, m.phone, m.email, m.address, m.district, m.city, m.province, m.postal_code,
       m.credit_limit, m.due_days, m.valid_until, m.active, m.notes, m.cover_image_id, m.points, m.lifetime_points,
       m.created_at, m.updated_at,
       lv.id AS level_id, coalesce(lv.name, '')::text AS level_name, coalesce(lv.min_points, 0)::int AS level_min_points,
       coalesce(lv.spend_per_point, 0)::numeric AS level_spend_per_point, coalesce(lv.point_value, 0)::numeric AS level_point_value,
       nx.id AS next_level_id, coalesce(nx.name, '')::text AS next_level_name, coalesce(nx.min_points, 0)::int AS next_level_min_points,
       coalesce(st.total_sales, 0)::numeric AS total_sales, coalesce(st.total_trx, 0)::bigint AS total_trx,
       coalesce((SELECT d.balance_after FROM member_deposit_movements d WHERE d.tenant_id = m.tenant_id AND d.member_id = m.id
                 ORDER BY d.id DESC LIMIT 1), 0)::numeric AS deposit
FROM members m
LEFT JOIN LATERAL (
    SELECT l.id, l.name, l.min_points, l.spend_per_point, l.point_value FROM member_levels l
    WHERE l.tenant_id = m.tenant_id AND l.active AND l.min_points <= m.lifetime_points
    ORDER BY l.min_points DESC LIMIT 1
) lv ON true
LEFT JOIN LATERAL (
    SELECT l.id, l.name, l.min_points FROM member_levels l
    WHERE l.tenant_id = m.tenant_id AND l.active AND l.min_points > m.lifetime_points
    ORDER BY l.min_points LIMIT 1
) nx ON true
LEFT JOIN LATERAL (
    SELECT sum(s.total) AS total_sales, count(*) AS total_trx FROM sales s
    WHERE s.tenant_id = m.tenant_id AND s.member_id = m.id AND s.status = 'completed'
) st ON true
WHERE m.tenant_id = $1 AND m.id = $2;

-- name: MemberGetForUpdate :one
SELECT id, code, name, gender, phone, email, address, district, city, province, postal_code, credit_limit, due_days,
       valid_until, active, notes, cover_image_id, points, lifetime_points
FROM members WHERE tenant_id = $1 AND id = $2 FOR UPDATE;

-- name: MemberCreate :one
INSERT INTO members (tenant_id, code, name, gender, phone, email, address, district, city, province, postal_code,
                     credit_limit, due_days, valid_until, notes, active)
VALUES (@tenant_id, @code, @name, @gender, @phone, @email, @address, @district, @city, @province, @postal_code,
        @credit_limit, @due_days, sqlc.narg('valid_until'), @notes, @active)
RETURNING id;

-- name: MemberUpdate :exec
UPDATE members SET code = @code, name = @name, gender = @gender, phone = @phone, email = @email, address = @address,
       district = @district, city = @city, province = @province, postal_code = @postal_code,
       credit_limit = @credit_limit, due_days = @due_days, valid_until = sqlc.narg('valid_until'), notes = @notes, active = @active
WHERE tenant_id = @tenant_id AND id = @id;

-- name: MemberSetCover :exec
UPDATE members SET cover_image_id = sqlc.narg('cover_image_id') WHERE tenant_id = @tenant_id AND id = @id;

-- name: MemberLookup :many
-- Pencarian cepat untuk kasir: member aktif yang belum kedaluwarsa (local_day = hari menurut zona waktu outlet).
SELECT m.id, m.code, m.name, m.phone, m.points, m.lifetime_points, m.valid_until, m.cover_image_id,
       coalesce(lv.name, '')::text AS level_name, coalesce(lv.spend_per_point, 0)::numeric AS spend_per_point,
       coalesce(lv.point_value, 0)::numeric AS point_value
FROM members m
LEFT JOIN LATERAL (
    SELECT l.name, l.spend_per_point, l.point_value FROM member_levels l
    WHERE l.tenant_id = m.tenant_id AND l.active AND l.min_points <= m.lifetime_points
    ORDER BY l.min_points DESC LIMIT 1
) lv ON true
WHERE m.tenant_id = @tenant_id AND m.active AND (m.valid_until IS NULL OR m.valid_until >= @local_day::date)
  AND (@q::text = '' OR m.name ILIKE '%' || @q || '%' OR m.code ILIKE '%' || @q || '%' OR m.phone ILIKE '%' || @q || '%'
       OR (@q_phone::text <> '' AND m.phone ILIKE '%' || @q_phone || '%'))
ORDER BY (lower(m.code) = lower(@q::text) OR m.phone = @q::text OR (@q_phone::text <> '' AND m.phone = @q_phone::text)) DESC, lower(m.name), m.id
LIMIT 20;

-- name: MemberLockForSale :one
-- Kunci baris member selama transaksi penjualan; level dihitung dari lifetime_points saat ini.
SELECT m.id, m.code, m.name, m.active, m.valid_until, m.points, m.lifetime_points,
       coalesce(lv.spend_per_point, 0)::numeric AS spend_per_point, coalesce(lv.point_value, 0)::numeric AS point_value
FROM members m
LEFT JOIN LATERAL (
    SELECT l.spend_per_point, l.point_value FROM member_levels l
    WHERE l.tenant_id = m.tenant_id AND l.active AND l.min_points <= m.lifetime_points
    ORDER BY l.min_points DESC LIMIT 1
) lv ON true
WHERE m.tenant_id = $1 AND m.id = $2
FOR UPDATE OF m;

-- ===== Poin =====

-- name: MemberPointsApply :one
-- Guard atomik: saldo/lifetime tidak boleh minus (baris terkunci selama transaksi).
UPDATE members SET points = points + @delta, lifetime_points = lifetime_points + @lifetime_delta
WHERE tenant_id = @tenant_id AND id = @id AND points + @delta >= 0 AND lifetime_points + @lifetime_delta >= 0
RETURNING points, lifetime_points;

-- name: MemberPointsApplyReturn :one
-- Retur boleh membuat saldo poin spendable negatif; lifetime earned tetap dijaga tidak negatif.
UPDATE members SET points = points + @delta, lifetime_points = lifetime_points + @lifetime_delta
WHERE tenant_id = @tenant_id AND id = @id AND lifetime_points + @lifetime_delta >= 0
RETURNING points, lifetime_points;

-- name: MemberPointInsert :one
INSERT INTO member_point_movements (tenant_id, member_id, kind, points, lifetime_delta, balance_after, ref_type, ref_id, note, actor_id)
VALUES (@tenant_id, @member_id, @kind, @points, @lifetime_delta, @balance_after, @ref_type, sqlc.narg('ref_id'), @note, sqlc.narg('actor_id'))
RETURNING id, created_at;

-- name: MemberPointList :many
SELECT p.id, p.kind, p.points, p.balance_after, p.ref_type, p.ref_id, p.note, p.created_at,
    coalesce(u.name, '')::text AS actor_name, coalesce(sr.doc_no, s.doc_no, '')::text AS doc_no,
       count(*) OVER () AS total
FROM member_point_movements p
LEFT JOIN users u ON u.tenant_id = p.tenant_id AND u.id = p.actor_id
LEFT JOIN sales s ON s.tenant_id = p.tenant_id AND s.id = p.ref_id AND p.ref_type = 'SALE'
LEFT JOIN sales_returns sr ON sr.tenant_id = p.tenant_id AND sr.id = p.ref_id AND p.ref_type = 'SALE_RETURN'
WHERE p.tenant_id = @tenant_id AND p.member_id = @member_id
ORDER BY p.id DESC
LIMIT @page_limit OFFSET @page_offset;

-- name: MemberPointsBySale :many
SELECT member_id, kind, points, lifetime_delta FROM member_point_movements
WHERE tenant_id = $1 AND ref_type = 'SALE' AND ref_id = $2 ORDER BY id;

-- name: MemberLocalDay :one
-- Hari ini menurut zona waktu outlet aktif.
SELECT (now() AT TIME ZONE timezone)::date AS local_day FROM outlets WHERE tenant_id = $1 AND id = $2;

-- name: MemberSetActive :exec
UPDATE members SET active = $3 WHERE tenant_id = $1 AND id = $2;

-- name: MemberReadForSale :one
-- Sama dengan MemberLockForSale tanpa kunci baris (pratinjau/quote kasir).
SELECT m.id, m.code, m.name, m.active, m.valid_until, m.points, m.lifetime_points,
       coalesce(lv.spend_per_point, 0)::numeric AS spend_per_point, coalesce(lv.point_value, 0)::numeric AS point_value
FROM members m
LEFT JOIN LATERAL (
    SELECT l.spend_per_point, l.point_value FROM member_levels l
    WHERE l.tenant_id = m.tenant_id AND l.active AND l.min_points <= m.lifetime_points
    ORDER BY l.min_points DESC LIMIT 1
) lv ON true
WHERE m.tenant_id = $1 AND m.id = $2;

-- name: MemberSaleList :many
-- Riwayat transaksi satu member (terbaru dulu), dengan ringkasan barang (3 pertama) dan jumlah baris.
SELECT s.id, s.doc_no, s.status, s.created_at, s.total, s.points_earned, s.points_redeemed,
       o.name AS outlet_name, coalesce(u.name, '')::text AS cashier,
       (SELECT count(*) FROM sale_lines l WHERE l.tenant_id = s.tenant_id AND l.sale_id = s.id)::int AS line_count,
       coalesce((SELECT string_agg(x.name, ', ' ORDER BY x.position)
                 FROM (SELECT l.name, l.position FROM sale_lines l WHERE l.tenant_id = s.tenant_id AND l.sale_id = s.id ORDER BY l.position LIMIT 3) x), '')::text AS items,
       count(*) OVER () AS total_rows
FROM sales s
JOIN outlets o ON o.tenant_id = s.tenant_id AND o.id = s.outlet_id
LEFT JOIN users u ON u.tenant_id = s.tenant_id AND u.id = s.cashier_id
WHERE s.tenant_id = @tenant_id AND s.member_id = @member_id
ORDER BY s.created_at DESC, s.id DESC
LIMIT @page_limit OFFSET @page_offset;

-- name: MemberExists :one
SELECT EXISTS (SELECT 1 FROM members WHERE tenant_id = $1 AND id = $2);
