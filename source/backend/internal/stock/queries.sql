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
