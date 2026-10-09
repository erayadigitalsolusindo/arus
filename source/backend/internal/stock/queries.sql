-- name: StockItemInfo :one
SELECT kind, allow_negative_stock FROM items WHERE tenant_id = $1 AND id = $2;

-- name: StockAddDelta :one
-- Penambahan (atau pengurangan pada item yang boleh minus): buat saldo bila belum ada, lalu tambah. Baris saldo
-- terkunci sampai commit sehingga transaksi bersamaan antre.
INSERT INTO stock_balances (tenant_id, outlet_id, item_id, bucket, qty)
VALUES (@tenant_id, @outlet_id, @item_id, @bucket, @delta)
ON CONFLICT (tenant_id, outlet_id, item_id, bucket) DO UPDATE SET qty = stock_balances.qty + EXCLUDED.qty
RETURNING qty;

-- name: StockSubtractGuarded :one
-- Pengurangan yang tidak boleh minus: satu UPDATE atomik dengan syarat saldo cukup. Tanpa baris hasil (saldo kurang
-- atau belum ada) = stok tidak cukup; tidak ada baris saldo yang dibuat.
UPDATE stock_balances SET qty = qty + @delta
WHERE tenant_id = @tenant_id AND outlet_id = @outlet_id AND item_id = @item_id AND bucket = @bucket AND qty + @delta >= 0
RETURNING qty;

-- name: StockMovementInsert :one
INSERT INTO stock_movements (tenant_id, outlet_id, item_id, bucket, qty_delta, balance_after, ref_type, ref_id, note, actor_id)
VALUES (@tenant_id, @outlet_id, @item_id, @bucket, @qty_delta, @balance_after, @ref_type, @ref_id, @note, @actor_id)
RETURNING id;

-- name: StockBalancesByItem :many
SELECT bucket, qty FROM stock_balances WHERE tenant_id = $1 AND outlet_id = $2 AND item_id = $3 ORDER BY bucket;

-- name: StockOutletLockState :one
-- FOR SHARE: penguncian (UPDATE) menunggu movement OPENING yang sedang berjalan, dan sebaliknya.
SELECT stock_locked_at, ops_start_date FROM outlets WHERE tenant_id = $1 AND id = $2 FOR SHARE;

-- name: StockOutletLock :one
UPDATE outlets SET stock_locked_at = now(), ops_start_date = @start_date
WHERE tenant_id = @tenant_id AND id = @outlet_id AND stock_locked_at IS NULL
RETURNING stock_locked_at, ops_start_date;

-- name: StockItemLock :one
-- Mengunci baris item: dua perubahan saldo awal pada item yang sama tidak saling menimpa.
SELECT sku, name, kind FROM items WHERE tenant_id = $1 AND id = $2 FOR UPDATE;

-- name: StockBalanceGet :one
SELECT qty FROM stock_balances WHERE tenant_id = $1 AND outlet_id = $2 AND item_id = $3 AND bucket = $4;

-- name: StockOpeningList :many
-- Barang bertipe goods yang aktif beserta stok outlet aktif per bucket.
SELECT i.id, i.sku, i.name, u.name AS unit_name,
       coalesce(sb.display, 0)::numeric AS display, coalesce(sb.warehouse, 0)::numeric AS warehouse,
       coalesce(sb.returns, 0)::numeric AS returns,
       count(*) OVER () AS total
FROM items i
JOIN units u ON u.tenant_id = i.tenant_id AND u.id = i.unit_id
LEFT JOIN LATERAL (
    SELECT sum(qty) FILTER (WHERE bucket = 'display') AS display,
           sum(qty) FILTER (WHERE bucket = 'warehouse') AS warehouse,
           sum(qty) FILTER (WHERE bucket = 'returns') AS returns
    FROM stock_balances s
    WHERE s.tenant_id = i.tenant_id AND s.outlet_id = @outlet_id AND s.item_id = i.id
) sb ON true
WHERE i.tenant_id = @tenant_id AND i.kind = 'goods' AND i.active
  AND (@q::text = '' OR i.name ILIKE '%' || @q || '%' OR i.sku ILIKE '%' || @q || '%' OR coalesce(i.barcode, '') ILIKE '%' || @q || '%')
ORDER BY lower(i.name), i.id
LIMIT @page_limit OFFSET @page_offset;

-- ---- Pecah satuan (Fase 4.3) ----

-- name: StockConvOutletInfo :one
SELECT code, active, (now() AT TIME ZONE timezone)::date AS local_day
FROM outlets WHERE tenant_id = $1 AND id = $2;

-- name: StockConvItemLock :one
-- Mengunci baris barang (dipanggil terurut menurut id) dan membaca HPP outlet + satuan dasarnya.
-- HPP = HPP cabang bila ada, selain itu HPP awal barang (items.avg_cost).
SELECT i.id, i.sku, i.name, i.kind, coalesce(oc.avg_cost, i.avg_cost)::numeric AS avg_cost, u.name AS unit_name
FROM items i JOIN units u ON u.tenant_id = i.tenant_id AND u.id = i.unit_id
LEFT JOIN item_outlet_costs oc ON oc.tenant_id = i.tenant_id AND oc.item_id = i.id AND oc.outlet_id = @outlet_id
WHERE i.tenant_id = @tenant_id AND i.id = @id FOR UPDATE OF i;

-- name: StockOutletQty :one
-- Total stok barang di satu outlet (semua bucket): dasar keputusan memasang HPP hasil pecah satuan dan rata-rata tertimbang.
SELECT coalesce(sum(qty), 0)::numeric AS qty FROM stock_balances WHERE tenant_id = $1 AND outlet_id = $2 AND item_id = $3;

-- name: StockSetCost :exec
INSERT INTO item_outlet_costs (tenant_id, outlet_id, item_id, avg_cost, last_cost)
VALUES (@tenant_id, @outlet_id, @item_id, @avg_cost, @last_cost)
ON CONFLICT (tenant_id, outlet_id, item_id) DO UPDATE SET avg_cost = EXCLUDED.avg_cost, last_cost = EXCLUDED.last_cost;

-- name: StockConvByIdemKey :one
SELECT id, request_hash FROM stock_conversions WHERE tenant_id = $1 AND idempotency_key = $2;

-- name: StockConvNextNo :one
INSERT INTO stock_conversion_counters (tenant_id, outlet_id, day, last_no) VALUES (@tenant_id, @outlet_id, @day, 1)
ON CONFLICT (tenant_id, outlet_id, day) DO UPDATE SET last_no = stock_conversion_counters.last_no + 1
RETURNING last_no;

-- name: StockConvInsert :one
-- DO NOTHING pada kunci idempotensi: pengiriman ganda bersamaan tidak membuat dokumen ganda.
INSERT INTO stock_conversions (id, tenant_id, outlet_id, doc_no, idempotency_key, request_hash, from_item_id, from_qty, to_item_id, to_qty,
                               from_unit_cost, to_unit_cost, cost_applied, note, actor_id)
VALUES (@id, @tenant_id, @outlet_id, @doc_no, @idempotency_key, @request_hash, @from_item_id, @from_qty, @to_item_id, @to_qty,
        @from_unit_cost, @to_unit_cost, @cost_applied, @note, sqlc.narg('actor_id'))
ON CONFLICT (tenant_id, idempotency_key) DO NOTHING
RETURNING id;

-- name: StockConvGet :one
SELECT c.id, c.outlet_id, c.doc_no, c.from_qty, c.to_qty, c.from_unit_cost, c.to_unit_cost, c.cost_applied, c.note, c.created_at,
       fi.id AS from_id, fi.sku AS from_sku, fi.name AS from_name, fu.name AS from_unit,
       ti.id AS to_id,   ti.sku AS to_sku,   ti.name AS to_name,   tu.name AS to_unit,
       coalesce(us.name, '') AS actor_name
FROM stock_conversions c
JOIN items fi ON fi.tenant_id = c.tenant_id AND fi.id = c.from_item_id
JOIN units fu ON fu.tenant_id = fi.tenant_id AND fu.id = fi.unit_id
JOIN items ti ON ti.tenant_id = c.tenant_id AND ti.id = c.to_item_id
JOIN units tu ON tu.tenant_id = ti.tenant_id AND tu.id = ti.unit_id
LEFT JOIN users us ON us.tenant_id = c.tenant_id AND us.id = c.actor_id
WHERE c.tenant_id = $1 AND c.id = $2;

-- name: StockConvList :many
SELECT c.id, c.outlet_id, c.doc_no, c.from_qty, c.to_qty, c.from_unit_cost, c.to_unit_cost, c.cost_applied, c.note, c.created_at,
       fi.id AS from_id, fi.sku AS from_sku, fi.name AS from_name, fu.name AS from_unit,
       ti.id AS to_id,   ti.sku AS to_sku,   ti.name AS to_name,   tu.name AS to_unit,
       coalesce(us.name, '') AS actor_name,
       count(*) OVER () AS total
FROM stock_conversions c
JOIN items fi ON fi.tenant_id = c.tenant_id AND fi.id = c.from_item_id
JOIN units fu ON fu.tenant_id = fi.tenant_id AND fu.id = fi.unit_id
JOIN items ti ON ti.tenant_id = c.tenant_id AND ti.id = c.to_item_id
JOIN units tu ON tu.tenant_id = ti.tenant_id AND tu.id = ti.unit_id
LEFT JOIN users us ON us.tenant_id = c.tenant_id AND us.id = c.actor_id
WHERE c.tenant_id = @tenant_id AND c.outlet_id = @outlet_id
ORDER BY c.created_at DESC, c.id
LIMIT @page_limit OFFSET @page_offset;

-- name: StockCardBounds :one
-- Batas periode kartu stok menurut zona waktu outlet: tanggal kosong → 30 hari terakhir sampai hari ini. end_at eksklusif.
WITH o AS (
    SELECT timezone, (now() AT TIME ZONE timezone)::date AS today FROM outlets WHERE tenant_id = @tenant_id AND id = @outlet_id
), t AS (
    SELECT o.timezone, coalesce(sqlc.narg('to_day')::date, o.today) AS to_day FROM o
), d AS (
    SELECT t.timezone, t.to_day, coalesce(sqlc.narg('from_day')::date, t.to_day - 29) AS from_day FROM t
)
SELECT d.from_day::date AS from_day, d.to_day::date AS to_day,
       (d.from_day::timestamp AT TIME ZONE d.timezone)::timestamptz AS start_at,
       ((d.to_day + 1)::timestamp AT TIME ZONE d.timezone)::timestamptz AS end_at
FROM d;

-- name: StockCardItem :one
SELECT i.id, i.sku, i.name, i.kind, u.name AS unit_name
FROM items i JOIN units u ON u.tenant_id = i.tenant_id AND u.id = i.unit_id
WHERE i.tenant_id = $1 AND i.id = $2;

-- name: StockCardItemSearch :many
-- Pemilih barang kartu stok: barang berstok (goods) termasuk yang diarsipkan, karena riwayatnya tetap perlu dibaca.
SELECT i.id, i.sku, i.name, u.name AS unit_name, i.active
FROM items i JOIN units u ON u.tenant_id = i.tenant_id AND u.id = i.unit_id
WHERE i.tenant_id = @tenant_id AND i.kind = 'goods'
  AND (@q::text = '' OR i.name ILIKE '%' || @q || '%' OR i.sku ILIKE '%' || @q || '%' OR coalesce(i.barcode, '') ILIKE '%' || @q || '%')
ORDER BY i.active DESC, lower(i.name), i.id
LIMIT 20;

-- name: StockCardOpening :one
-- Saldo sebelum periode = saldo-setelah movement terakhir tiap bucket (urutan id = urutan saldo).
SELECT coalesce(sum(b.balance_after), 0)::numeric AS opening FROM (
    SELECT DISTINCT ON (m.bucket) m.balance_after
    FROM stock_movements m
    WHERE m.tenant_id = @tenant_id AND m.outlet_id = @outlet_id AND m.item_id = @item_id
      AND (@bucket::text = '' OR m.bucket = @bucket) AND m.created_at < @start_at::timestamptz
    ORDER BY m.bucket, m.id DESC
) b;

-- name: StockCardTotals :one
SELECT coalesce(sum(qty_delta) FILTER (WHERE qty_delta > 0), 0)::numeric AS qty_in,
       coalesce(-sum(qty_delta) FILTER (WHERE qty_delta < 0), 0)::numeric AS qty_out
FROM stock_movements
WHERE tenant_id = @tenant_id AND outlet_id = @outlet_id AND item_id = @item_id
  AND (@bucket::text = '' OR bucket = @bucket)
  AND created_at >= @start_at::timestamptz AND created_at < @end_at::timestamptz;

-- name: StockCardBefore :one
-- Jumlah mutasi pada periode sampai (dan termasuk) id kursor: dasar saldo berjalan halaman berikutnya.
SELECT coalesce(sum(qty_delta), 0)::numeric AS delta
FROM stock_movements
WHERE tenant_id = @tenant_id AND outlet_id = @outlet_id AND item_id = @item_id
  AND (@bucket::text = '' OR bucket = @bucket)
  AND created_at >= @start_at::timestamptz AND created_at < @end_at::timestamptz AND id <= @after_id;

-- name: StockCardRows :many
SELECT m.id, m.created_at, m.bucket, m.qty_delta, m.ref_type, m.ref_id, m.note, coalesce(u.name, '')::text AS actor_name
FROM stock_movements m
LEFT JOIN users u ON u.tenant_id = m.tenant_id AND u.id = m.actor_id
WHERE m.tenant_id = @tenant_id AND m.outlet_id = @outlet_id AND m.item_id = @item_id
  AND (@bucket::text = '' OR m.bucket = @bucket)
  AND m.created_at >= @start_at::timestamptz AND m.created_at < @end_at::timestamptz AND m.id > @after_id
ORDER BY m.id
LIMIT @page_limit;

-- ---- Stok opname (Fase 4.3) ----

-- name: StockCountNextNo :one
INSERT INTO stock_count_counters (tenant_id, outlet_id, day, last_no) VALUES (@tenant_id, @outlet_id, @day, 1)
ON CONFLICT (tenant_id, outlet_id, day) DO UPDATE SET last_no = stock_count_counters.last_no + 1
RETURNING last_no;

-- name: StockCountInsert :exec
INSERT INTO stock_counts (id, tenant_id, outlet_id, doc_no, bucket, note, created_by)
VALUES (@id, @tenant_id, @outlet_id, @doc_no, @bucket, @note, sqlc.narg('created_by'));

-- name: StockCountLockUpdate :one
-- Penyelesaian/pembatalan: kunci eksklusif; hasil menentukan boleh tidaknya diproses.
SELECT outlet_id, doc_no, bucket, status FROM stock_counts WHERE tenant_id = $1 AND id = $2 FOR UPDATE;

-- name: StockCountLockShare :one
-- Perubahan baris draf: berbagi kunci, sehingga penyelesaian bersamaan menunggu/menolak.
SELECT outlet_id, doc_no, bucket, status FROM stock_counts WHERE tenant_id = $1 AND id = $2 FOR SHARE;

-- name: StockCountHeader :one
SELECT c.id, c.outlet_id, c.doc_no, c.bucket, c.status, c.note, c.created_at, c.completed_at, c.cancelled_at, c.kind, c.adjust_mode,
       coalesce(cu.name, '') AS created_name, coalesce(fu.name, '') AS completed_name, coalesce(xu.name, '') AS cancelled_name
FROM stock_counts c
LEFT JOIN users cu ON cu.tenant_id = c.tenant_id AND cu.id = c.created_by
LEFT JOIN users fu ON fu.tenant_id = c.tenant_id AND fu.id = c.completed_by
LEFT JOIN users xu ON xu.tenant_id = c.tenant_id AND xu.id = c.cancelled_by
WHERE c.tenant_id = $1 AND c.id = $2;

-- name: StockCountList :many
SELECT c.id, c.outlet_id, c.doc_no, c.bucket, c.status, c.note, c.created_at, c.completed_at, c.cancelled_at, c.kind, c.adjust_mode,
       coalesce(cu.name, '') AS created_name,
       (SELECT coalesce(sum(CASE WHEN c.status = 'completed' THEN l.diff * l.unit_cost
                                 ELSE (l.counted_qty - l.snapshot_qty) * coalesce(oc.avg_cost, i.avg_cost, 0) END), 0)
          FROM stock_count_lines l JOIN items i ON i.tenant_id = l.tenant_id AND i.id = l.item_id
          LEFT JOIN item_outlet_costs oc ON oc.tenant_id = l.tenant_id AND oc.item_id = l.item_id AND oc.outlet_id = c.outlet_id
         WHERE l.tenant_id = c.tenant_id AND l.count_id = c.id AND l.counted_qty IS NOT NULL)::numeric AS diff_value,
       (SELECT count(*) FROM stock_count_lines l WHERE l.tenant_id = c.tenant_id AND l.count_id = c.id)::bigint AS line_count,
       (SELECT count(*) FROM stock_count_lines l WHERE l.tenant_id = c.tenant_id AND l.count_id = c.id AND l.counted_qty IS NOT NULL)::bigint AS counted_count,
       (SELECT count(*) FROM stock_count_lines l WHERE l.tenant_id = c.tenant_id AND l.count_id = c.id
                AND l.counted_qty IS NOT NULL AND l.counted_qty <> l.snapshot_qty)::bigint AS diff_count,
       count(*) OVER () AS total
FROM stock_counts c
LEFT JOIN users cu ON cu.tenant_id = c.tenant_id AND cu.id = c.created_by
WHERE c.tenant_id = @tenant_id AND c.outlet_id = @outlet_id AND (@status::text = '' OR c.status = @status)
  AND (@kind::text = '' OR c.kind = @kind) AND (@q::text = '' OR c.doc_no ILIKE '%' || @q || '%')
ORDER BY c.created_at DESC, c.id
LIMIT @page_limit OFFSET @page_offset;

-- name: StockCountLines :many
-- Baris sesi beserta stok sistem TERKINI di bucket sesi (pratinjau hasil akhir).
SELECT l.item_id, i.sku, i.name, u.name AS unit_name, l.snapshot_qty, l.counted_qty, l.diff, l.unit_cost, l.counted_at,
       coalesce(sb.qty, 0)::numeric AS current_qty, coalesce(oc.avg_cost, i.avg_cost, 0)::numeric AS item_cost
FROM stock_count_lines l
JOIN items i ON i.tenant_id = l.tenant_id AND i.id = l.item_id
JOIN units u ON u.tenant_id = i.tenant_id AND u.id = i.unit_id
JOIN stock_counts c ON c.tenant_id = l.tenant_id AND c.id = l.count_id
LEFT JOIN item_outlet_costs oc ON oc.tenant_id = l.tenant_id AND oc.item_id = l.item_id AND oc.outlet_id = c.outlet_id
LEFT JOIN stock_balances sb ON sb.tenant_id = l.tenant_id AND sb.outlet_id = c.outlet_id AND sb.item_id = l.item_id AND sb.bucket = c.bucket
WHERE l.tenant_id = $1 AND l.count_id = $2
ORDER BY lower(i.name), i.id;

-- name: StockCountLineCount :one
SELECT count(*) FROM stock_count_lines WHERE tenant_id = $1 AND count_id = $2;

-- name: StockCountAddItems :execrows
-- Menambah barang goods aktif ke sesi dengan snapshot stok bucket sesi. Gabungan penyaring: semua, id eksplisit, kategori, brand.
INSERT INTO stock_count_lines (tenant_id, count_id, item_id, snapshot_qty)
SELECT i.tenant_id, @count_id, i.id, coalesce(sb.qty, 0)
FROM items i
LEFT JOIN stock_balances sb ON sb.tenant_id = i.tenant_id AND sb.outlet_id = @outlet_id AND sb.item_id = i.id AND sb.bucket = @bucket
WHERE i.tenant_id = @tenant_id AND i.kind = 'goods' AND i.active
  AND (@all_items::bool OR i.id = ANY(@item_ids::uuid[])
       OR i.category_id = sqlc.narg('category_id') OR i.brand_id = sqlc.narg('brand_id'))
ON CONFLICT (count_id, item_id) DO NOTHING;

-- name: StockCountRemoveItem :execrows
DELETE FROM stock_count_lines WHERE tenant_id = $1 AND count_id = $2 AND item_id = $3;

-- name: StockCountSetQty :execrows
UPDATE stock_count_lines SET counted_qty = sqlc.narg('counted_qty'),
       counted_by = CASE WHEN sqlc.narg('counted_qty')::numeric IS NULL THEN NULL ELSE sqlc.narg('actor_id')::uuid END,
       counted_at = CASE WHEN sqlc.narg('counted_qty')::numeric IS NULL THEN NULL ELSE now() END
WHERE tenant_id = @tenant_id AND count_id = @count_id AND item_id = @item_id;

-- name: StockCountDiffLines :many
-- Baris yang sudah dihitung dengan selisih ≠ 0 terhadap snapshot.
SELECT item_id, (counted_qty - snapshot_qty)::numeric AS delta
FROM stock_count_lines
WHERE tenant_id = $1 AND count_id = $2 AND counted_qty IS NOT NULL AND counted_qty <> snapshot_qty;

-- name: StockCountFinalizeLines :exec
UPDATE stock_count_lines l SET diff = l.counted_qty - l.snapshot_qty, unit_cost = coalesce(oc.avg_cost, i.avg_cost, 0)
FROM items i
JOIN stock_counts c ON c.tenant_id = i.tenant_id AND c.id = @count_id
LEFT JOIN item_outlet_costs oc ON oc.tenant_id = i.tenant_id AND oc.item_id = i.id AND oc.outlet_id = c.outlet_id
WHERE l.tenant_id = @tenant_id AND l.count_id = @count_id AND l.counted_qty IS NOT NULL
  AND i.tenant_id = l.tenant_id AND i.id = l.item_id;

-- name: StockCountComplete :exec
UPDATE stock_counts SET status = 'completed', completed_at = now(), completed_by = sqlc.narg('actor_id')
WHERE tenant_id = @tenant_id AND id = @id;

-- name: StockCountCancel :exec
UPDATE stock_counts SET status = 'cancelled', cancelled_at = now(), cancelled_by = sqlc.narg('actor_id')
WHERE tenant_id = @tenant_id AND id = @id;

-- name: StockCountCountedLines :one
SELECT count(*) FROM stock_count_lines WHERE tenant_id = $1 AND count_id = $2 AND counted_qty IS NOT NULL;

-- ---- Opname langsung ----

-- name: StockCountInsertQuick :exec
INSERT INTO stock_counts (id, tenant_id, outlet_id, doc_no, bucket, note, kind, adjust_mode, status,
                          created_by, completed_by, completed_at)
VALUES (@id, @tenant_id, @outlet_id, @doc_no, @bucket, @note, 'quick', @adjust_mode, 'completed',
        sqlc.narg('actor_id'), sqlc.narg('actor_id'), now());

-- name: StockCountLineInsertDone :exec
INSERT INTO stock_count_lines (tenant_id, count_id, item_id, snapshot_qty, counted_qty, diff, unit_cost, counted_by, counted_at)
VALUES (@tenant_id, @count_id, @item_id, @snapshot_qty, @counted_qty, @diff, @unit_cost, sqlc.narg('actor_id'), now());

-- name: StockBalanceEnsure :exec
INSERT INTO stock_balances (tenant_id, outlet_id, item_id, bucket, qty)
VALUES (@tenant_id, @outlet_id, @item_id, @bucket, 0)
ON CONFLICT (tenant_id, outlet_id, item_id, bucket) DO NOTHING;

-- name: StockBalanceLock :one
SELECT qty FROM stock_balances WHERE tenant_id = $1 AND outlet_id = $2 AND item_id = $3 AND bucket = $4 FOR UPDATE;

-- name: StockCountSummary :one
-- Ringkasan 30 hari terakhir di outlet: sesi berjalan, dokumen selesai, dan nilai selisih (lebih / kurang).
SELECT
  (SELECT count(*) FROM stock_counts c WHERE c.tenant_id = @tenant_id AND c.outlet_id = @outlet_id AND c.status = 'draft')::bigint AS open_count,
  (SELECT count(*) FROM stock_counts c WHERE c.tenant_id = @tenant_id AND c.outlet_id = @outlet_id AND c.status = 'completed'
        AND c.completed_at >= now() - interval '30 days')::bigint AS done_count,
  coalesce(sum(CASE WHEN l.diff > 0 THEN l.diff * l.unit_cost END), 0)::numeric AS plus_value,
  coalesce(sum(CASE WHEN l.diff < 0 THEN l.diff * l.unit_cost END), 0)::numeric AS minus_value,
  (count(*) FILTER (WHERE l.diff <> 0))::bigint AS diff_lines
FROM stock_counts c
JOIN stock_count_lines l ON l.tenant_id = c.tenant_id AND l.count_id = c.id
WHERE c.tenant_id = @tenant_id AND c.outlet_id = @outlet_id AND c.status = 'completed'
  AND c.completed_at >= now() - interval '30 days';

-- ---- Mutasi stok (Fase 4.3 / 6.5) ----

-- name: StockTrOutlet :one
SELECT id, code, name, active, (now() AT TIME ZONE timezone)::date AS local_day
FROM outlets WHERE tenant_id = $1 AND id = $2;

-- name: StockTrDestinations :many
SELECT id, code, name FROM outlets WHERE tenant_id = $1 AND active ORDER BY lower(name), code;

-- name: StockTrItemLock :one
-- Kunci baris barang (terurut id) + data & HPP efektif di outlet yang diminta. FOR NO KEY UPDATE (bukan FOR UPDATE)
-- agar tidak bentrok dengan FOR KEY SHARE dari FK saat penjualan menulis movement (deadlock).
SELECT i.id, i.sku, i.name, i.kind, i.active, u.name AS unit_name,
       coalesce(oc.avg_cost, i.avg_cost)::numeric  AS avg_cost,
       coalesce(oc.last_cost, i.last_cost)::numeric AS last_cost
FROM items i JOIN units u ON u.tenant_id = i.tenant_id AND u.id = i.unit_id
LEFT JOIN item_outlet_costs oc ON oc.tenant_id = i.tenant_id AND oc.item_id = i.id AND oc.outlet_id = @outlet_id
WHERE i.tenant_id = @tenant_id AND i.id = @id FOR NO KEY UPDATE OF i;

-- name: StockTrByIdemKey :one
SELECT id, request_hash FROM stock_transfers WHERE tenant_id = $1 AND idempotency_key = $2;

-- name: StockTrNextNo :one
INSERT INTO stock_transfer_counters (tenant_id, outlet_id, day, last_no) VALUES (@tenant_id, @outlet_id, @day, 1)
ON CONFLICT (tenant_id, outlet_id, day) DO UPDATE SET last_no = stock_transfer_counters.last_no + 1
RETURNING last_no;

-- name: StockTrInsert :one
INSERT INTO stock_transfers (id, tenant_id, doc_no, idempotency_key, request_hash, from_outlet_id, from_bucket, to_outlet_id, to_bucket,
                             status, note, sent_by, received_by, received_at)
VALUES (@id, @tenant_id, @doc_no, @idempotency_key, @request_hash, @from_outlet_id, @from_bucket, @to_outlet_id, @to_bucket,
        @status, @note, sqlc.narg('sent_by'), sqlc.narg('received_by'), sqlc.narg('received_at'))
ON CONFLICT (tenant_id, idempotency_key) DO NOTHING
RETURNING id;

-- name: StockTrLineInsert :exec
INSERT INTO stock_transfer_lines (tenant_id, transfer_id, item_id, qty_sent, qty_received, unit_cost)
VALUES (@tenant_id, @transfer_id, @item_id, @qty_sent, sqlc.narg('qty_received'), @unit_cost);

-- name: StockTrLock :one
SELECT id, doc_no, from_outlet_id, from_bucket, to_outlet_id, to_bucket, status
FROM stock_transfers WHERE tenant_id = $1 AND id = $2 FOR UPDATE;

-- name: StockTrLinesForMove :many
SELECT item_id, qty_sent, unit_cost FROM stock_transfer_lines WHERE tenant_id = $1 AND transfer_id = $2 ORDER BY item_id;

-- name: StockTrLineSetReceived :exec
UPDATE stock_transfer_lines SET qty_received = @qty_received
WHERE tenant_id = @tenant_id AND transfer_id = @transfer_id AND item_id = @item_id;

-- name: StockTrSetReceived :exec
UPDATE stock_transfers SET status = 'received', received_by = sqlc.narg('actor_id'), received_at = now()
WHERE tenant_id = @tenant_id AND id = @id;

-- name: StockTrSetCancelled :exec
UPDATE stock_transfers SET status = 'cancelled', cancelled_by = sqlc.narg('actor_id'), cancelled_at = now(), cancel_reason = @reason
WHERE tenant_id = @tenant_id AND id = @id;

-- name: StockTrGet :one
SELECT t.id, t.doc_no, t.from_outlet_id, t.from_bucket, t.to_outlet_id, t.to_bucket, t.status, t.note, t.sent_at, t.received_at, t.cancelled_at, t.cancel_reason,
       fo.code AS from_code, fo.name AS from_name, too.code AS to_code, too.name AS to_name,
       coalesce(us.name, '') AS sent_by_name, coalesce(ur.name, '') AS received_by_name, coalesce(uc.name, '') AS cancelled_by_name
FROM stock_transfers t
JOIN outlets fo  ON fo.tenant_id = t.tenant_id AND fo.id = t.from_outlet_id
JOIN outlets too ON too.tenant_id = t.tenant_id AND too.id = t.to_outlet_id
LEFT JOIN users us ON us.tenant_id = t.tenant_id AND us.id = t.sent_by
LEFT JOIN users ur ON ur.tenant_id = t.tenant_id AND ur.id = t.received_by
LEFT JOIN users uc ON uc.tenant_id = t.tenant_id AND uc.id = t.cancelled_by
WHERE t.tenant_id = $1 AND t.id = $2;

-- name: StockTrLines :many
SELECT l.item_id, l.qty_sent, l.qty_received, l.unit_cost, i.sku, i.name, u.name AS unit_name
FROM stock_transfer_lines l
JOIN items i ON i.tenant_id = l.tenant_id AND i.id = l.item_id
JOIN units u ON u.tenant_id = i.tenant_id AND u.id = i.unit_id
WHERE l.tenant_id = $1 AND l.transfer_id = $2 ORDER BY lower(i.name), i.id;

-- name: StockTrList :many
-- direction: 'out' = keluar dari outlet ini, 'in' = masuk ke outlet ini (termasuk antar-bucket di outlet yang sama untuk keduanya).
SELECT t.id, t.doc_no, t.from_outlet_id, t.from_bucket, t.to_outlet_id, t.to_bucket, t.status, t.note, t.sent_at, t.received_at, t.cancelled_at,
       fo.code AS from_code, fo.name AS from_name, too.code AS to_code, too.name AS to_name,
       coalesce(us.name, '') AS sent_by_name,
       (SELECT count(*) FROM stock_transfer_lines l WHERE l.tenant_id = t.tenant_id AND l.transfer_id = t.id) AS line_count,
       (SELECT coalesce(sum(l.qty_sent), 0) FROM stock_transfer_lines l WHERE l.tenant_id = t.tenant_id AND l.transfer_id = t.id)::numeric AS qty_sent,
       (SELECT coalesce(sum(l.qty_sent - coalesce(l.qty_received, l.qty_sent)), 0) FROM stock_transfer_lines l WHERE l.tenant_id = t.tenant_id AND l.transfer_id = t.id)::numeric AS qty_short
FROM stock_transfers t
JOIN outlets fo  ON fo.tenant_id = t.tenant_id AND fo.id = t.from_outlet_id
JOIN outlets too ON too.tenant_id = t.tenant_id AND too.id = t.to_outlet_id
LEFT JOIN users us ON us.tenant_id = t.tenant_id AND us.id = t.sent_by
WHERE t.tenant_id = @tenant_id
  AND ((@direction::text = 'out' AND t.from_outlet_id = @outlet_id) OR (@direction::text = 'in' AND t.to_outlet_id = @outlet_id))
  AND (@status::text = '' OR t.status = @status::text)
  AND (@q::text = '' OR t.doc_no ILIKE '%' || @q::text || '%')
  AND (sqlc.narg('cursor_at')::timestamptz IS NULL OR (t.sent_at, t.id) < (sqlc.narg('cursor_at')::timestamptz, sqlc.narg('cursor_id')::uuid))
ORDER BY t.sent_at DESC, t.id DESC
LIMIT @page_limit;

-- name: StockTrSummary :one
SELECT
  count(*) FILTER (WHERE status = 'sent' AND to_outlet_id = @outlet_id AND from_outlet_id <> @outlet_id) AS to_receive,
  count(*) FILTER (WHERE status = 'sent' AND from_outlet_id = @outlet_id AND to_outlet_id <> @outlet_id) AS in_transit
FROM stock_transfers WHERE tenant_id = @tenant_id AND (from_outlet_id = @outlet_id OR to_outlet_id = @outlet_id);
