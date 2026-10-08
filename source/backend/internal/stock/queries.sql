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
-- Mengunci baris barang (dipanggil terurut menurut id) dan membaca HPP + satuan dasarnya.
SELECT i.id, i.sku, i.name, i.kind, i.avg_cost, u.name AS unit_name
FROM items i JOIN units u ON u.tenant_id = i.tenant_id AND u.id = i.unit_id
WHERE i.tenant_id = $1 AND i.id = $2 FOR UPDATE OF i;

-- name: StockTotalQty :one
-- Total stok barang di semua outlet & bucket (dasar keputusan memasang HPP hasil pecah satuan).
SELECT coalesce(sum(qty), 0)::numeric AS qty FROM stock_balances WHERE tenant_id = $1 AND item_id = $2;

-- name: StockSetCost :exec
UPDATE items SET avg_cost = @cost, last_cost = @cost WHERE tenant_id = @tenant_id AND id = @item_id;

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
