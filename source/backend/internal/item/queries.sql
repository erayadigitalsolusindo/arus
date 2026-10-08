-- name: ItemList :many
-- Harga efektif untuk outlet aktif: harga cabang bila ada, selain itu harga default tenant.
SELECT i.id, i.sku, i.barcode, i.name, i.origin, i.kind, i.active,
       i.sell_price AS default_price, op.sell_price AS outlet_price, i.avg_cost, i.last_cost,
       u.name AS unit_name, c.name AS category_name, b.name AS brand_name,
       mi.id AS main_image_id,
       coalesce(sb.display, 0)::numeric AS stock_display, coalesce(sb.warehouse, 0)::numeric AS stock_warehouse,
       coalesce(sb.returns, 0)::numeric AS stock_returns,
       count(*) OVER () AS total
FROM items i
LEFT JOIN item_images mi ON mi.tenant_id = i.tenant_id AND mi.item_id = i.id AND mi.is_main
JOIN units u ON u.tenant_id = i.tenant_id AND u.id = i.unit_id
LEFT JOIN categories c ON c.tenant_id = i.tenant_id AND c.id = i.category_id
LEFT JOIN brands b ON b.tenant_id = i.tenant_id AND b.id = i.brand_id
LEFT JOIN item_outlet_prices op ON op.tenant_id = i.tenant_id AND op.item_id = i.id AND op.outlet_id = @outlet_id
-- Stok per bucket dijumlahkan atas outlet_ids (satu outlet aktif, atau semua outlet yang boleh diakses); tanpa baris saldo = 0.
LEFT JOIN LATERAL (
    SELECT sum(qty) FILTER (WHERE bucket = 'display') AS display,
           sum(qty) FILTER (WHERE bucket = 'warehouse') AS warehouse,
           sum(qty) FILTER (WHERE bucket = 'returns') AS returns
    FROM stock_balances s
    WHERE s.tenant_id = i.tenant_id AND s.outlet_id = ANY(@outlet_ids::uuid[]) AND s.item_id = i.id
) sb ON true
WHERE i.tenant_id = @tenant_id
  AND (@q::text = '' OR i.name ILIKE '%' || @q || '%' OR i.sku ILIKE '%' || @q || '%' OR coalesce(i.barcode, '') ILIKE '%' || @q || '%')
  AND (sqlc.narg('active')::boolean IS NULL OR i.active = sqlc.narg('active'))
  AND (sqlc.narg('category_id')::uuid IS NULL OR i.category_id = sqlc.narg('category_id'))
ORDER BY lower(i.name), i.id
LIMIT @page_limit OFFSET @page_offset;

-- name: ItemOutletBreakdown :many
-- Rincian stok + harga jual efektif per outlet untuk sekumpulan item (mode "semua cabang").
SELECT i.id AS item_id, o.id AS outlet_id, o.code AS outlet_code, o.name AS outlet_name,
       coalesce(sum(s.qty) FILTER (WHERE s.bucket = 'display'), 0)::numeric AS stock_display,
       coalesce(sum(s.qty) FILTER (WHERE s.bucket = 'warehouse'), 0)::numeric AS stock_warehouse,
       coalesce(sum(s.qty) FILTER (WHERE s.bucket = 'returns'), 0)::numeric AS stock_returns,
       coalesce(op.sell_price, i.sell_price)::numeric AS price
FROM items i
CROSS JOIN outlets o
LEFT JOIN item_outlet_prices op ON op.tenant_id = i.tenant_id AND op.item_id = i.id AND op.outlet_id = o.id
LEFT JOIN stock_balances s ON s.tenant_id = i.tenant_id AND s.item_id = i.id AND s.outlet_id = o.id
WHERE i.tenant_id = @tenant_id AND o.tenant_id = @tenant_id
  AND i.id = ANY(@item_ids::uuid[]) AND o.id = ANY(@outlet_ids::uuid[])
GROUP BY i.id, o.id, op.sell_price, i.sell_price
ORDER BY lower(o.name), o.id;

-- name: ItemGet :one
SELECT i.id, i.sku, i.barcode, i.name, i.origin, i.weight_grams, i.last_cost, i.avg_cost, i.sell_price, i.kind,
       i.allow_negative_stock, i.sell_below_cost, i.description, i.active, i.created_at, i.updated_at,
       i.unit_id, u.name AS unit_name,
       i.category_id, c.name AS category_name,
       i.brand_id, b.name AS brand_name,
       i.principal_id, p.name AS principal_name,
       i.supplier_id, s.name AS supplier_name
FROM items i
JOIN units u ON u.tenant_id = i.tenant_id AND u.id = i.unit_id
LEFT JOIN categories c ON c.tenant_id = i.tenant_id AND c.id = i.category_id
LEFT JOIN brands b ON b.tenant_id = i.tenant_id AND b.id = i.brand_id
LEFT JOIN principals p ON p.tenant_id = i.tenant_id AND p.id = i.principal_id
LEFT JOIN suppliers s ON s.tenant_id = i.tenant_id AND s.id = i.supplier_id
WHERE i.tenant_id = $1 AND i.id = $2;

-- name: ItemGetForUpdate :one
-- Kunci baris item selama transaksi (ubah bersamaan tidak saling menimpa; audit "sebelum" selalu benar).
SELECT id, sku, barcode, name, origin, weight_grams, sell_price, kind, allow_negative_stock, sell_below_cost, active,
       unit_id, category_id, brand_id, principal_id, supplier_id
FROM items WHERE tenant_id = $1 AND id = $2 FOR UPDATE;

-- name: ItemCreate :one
INSERT INTO items (tenant_id, sku, barcode, name, origin, weight_grams, last_cost, avg_cost, sell_price, unit_id, category_id,
                   brand_id, principal_id, supplier_id, kind, allow_negative_stock, sell_below_cost, description)
VALUES (@tenant_id, @sku, @barcode, @name, @origin, @weight_grams, @cost, @cost, @sell_price, @unit_id, @category_id,
        @brand_id, @principal_id, @supplier_id, @kind, @allow_negative_stock, @sell_below_cost, @description)
RETURNING id;

-- name: ItemUpdate :exec
-- HPP (last_cost/avg_cost) sengaja tidak ada di sini: hanya diubah transaksi pembelian/stok.
UPDATE items SET sku = @sku, barcode = @barcode, name = @name, origin = @origin, weight_grams = @weight_grams, sell_price = @sell_price,
       unit_id = @unit_id, category_id = @category_id, brand_id = @brand_id, principal_id = @principal_id,
       supplier_id = @supplier_id, kind = @kind, allow_negative_stock = @allow_negative_stock,
       sell_below_cost = @sell_below_cost, description = @description
WHERE tenant_id = @tenant_id AND id = @id;

-- name: ItemSetActive :one
UPDATE items SET active = $3 WHERE tenant_id = $1 AND id = $2 RETURNING id, sku, name, active;

-- name: ItemNextNo :one
INSERT INTO item_counters (tenant_id, last_no) VALUES ($1, 1)
ON CONFLICT (tenant_id) DO UPDATE SET last_no = item_counters.last_no + 1
RETURNING last_no;

-- name: ItemSkuExists :one
SELECT EXISTS (SELECT 1 FROM items WHERE tenant_id = $1 AND lower(sku) = lower($2));

-- name: ItemOutletPrices :many
-- Harga khusus cabang untuk outlet yang boleh diakses pemanggil (satu baris per outlet; harga NULL = pakai default).
SELECT o.id AS outlet_id, o.name AS outlet_name, op.sell_price
FROM outlets o
LEFT JOIN item_outlet_prices op ON op.tenant_id = o.tenant_id AND op.outlet_id = o.id AND op.item_id = @item_id
WHERE o.tenant_id = @tenant_id AND o.id = ANY(@outlet_ids::uuid[])
ORDER BY o.created_at, o.code;

-- name: ItemOutletPriceUpsert :exec
INSERT INTO item_outlet_prices (tenant_id, item_id, outlet_id, sell_price) VALUES ($1, $2, $3, $4)
ON CONFLICT (item_id, outlet_id) DO UPDATE SET sell_price = EXCLUDED.sell_price;

-- name: ItemOutletPriceDelete :exec
-- Menghapus harga khusus cabang (kembali ke harga default) untuk outlet yang boleh diakses pemanggil.
DELETE FROM item_outlet_prices
WHERE tenant_id = @tenant_id AND item_id = @item_id AND outlet_id = ANY(@outlet_ids::uuid[]);

-- name: ItemRefState :one
-- Status master yang dirujuk item (satu kueri untuk lima master): 'active' | 'archived' | 'missing'.
SELECT
  coalesce((SELECT CASE WHEN x.active THEN 'active' ELSE 'archived' END FROM units      x WHERE x.tenant_id = @tenant_id AND x.id = sqlc.narg('unit_id')),      'missing')::text AS unit_state,
  coalesce((SELECT CASE WHEN x.active THEN 'active' ELSE 'archived' END FROM categories x WHERE x.tenant_id = @tenant_id AND x.id = sqlc.narg('category_id')),  'missing')::text AS category_state,
  coalesce((SELECT CASE WHEN x.active THEN 'active' ELSE 'archived' END FROM brands     x WHERE x.tenant_id = @tenant_id AND x.id = sqlc.narg('brand_id')),     'missing')::text AS brand_state,
  coalesce((SELECT CASE WHEN x.active THEN 'active' ELSE 'archived' END FROM principals x WHERE x.tenant_id = @tenant_id AND x.id = sqlc.narg('principal_id')), 'missing')::text AS principal_state,
  coalesce((SELECT CASE WHEN x.active THEN 'active' ELSE 'archived' END FROM suppliers  x WHERE x.tenant_id = @tenant_id AND x.id = sqlc.narg('supplier_id')),  'missing')::text AS supplier_state;

-- name: ItemImageList :many
SELECT id, position, is_main, width, height, full_bytes, thumb_bytes, created_at
FROM item_images WHERE tenant_id = $1 AND item_id = $2
ORDER BY is_main DESC, position, created_at;

-- name: ItemImageCount :one
SELECT count(*) FROM item_images WHERE tenant_id = $1 AND item_id = $2;

-- name: ItemImageInsert :one
-- Gambar pertama item otomatis menjadi gambar utama; posisi = urutan setelah yang terakhir.
INSERT INTO item_images (id, tenant_id, item_id, position, is_main, width, height, full_bytes, thumb_bytes)
SELECT @id, @tenant_id, @item_id,
       coalesce((SELECT max(x.position) FROM item_images x WHERE x.tenant_id = @tenant_id AND x.item_id = @item_id), 0) + 1,
       NOT EXISTS (SELECT 1 FROM item_images y WHERE y.tenant_id = @tenant_id AND y.item_id = @item_id),
       @width, @height, @full_bytes, @thumb_bytes
RETURNING id, position, is_main, width, height, full_bytes, thumb_bytes, created_at;

-- name: ItemImageGet :one
SELECT id, position, is_main, width, height, full_bytes, thumb_bytes, created_at
FROM item_images WHERE tenant_id = $1 AND item_id = $2 AND id = $3;

-- name: ItemImageClearMain :exec
UPDATE item_images SET is_main = false WHERE tenant_id = $1 AND item_id = $2 AND is_main;

-- name: ItemImageSetMain :exec
UPDATE item_images SET is_main = true WHERE tenant_id = $1 AND item_id = $2 AND id = $3;

-- name: ItemImageDelete :one
DELETE FROM item_images WHERE tenant_id = $1 AND item_id = $2 AND id = $3 RETURNING is_main;

-- name: ItemImagePromote :exec
-- Setelah gambar utama dihapus: gambar dengan posisi terkecil menjadi utama.
UPDATE item_images SET is_main = true
WHERE id = (SELECT z.id FROM item_images z WHERE z.tenant_id = @tenant_id AND z.item_id = @item_id ORDER BY z.position, z.created_at LIMIT 1);

-- name: ItemTierList :many
SELECT outlet_id, min_qty, price FROM item_wholesale_tiers
WHERE tenant_id = $1 AND item_id = $2
ORDER BY outlet_id NULLS FIRST, min_qty;

-- name: ItemTierDeleteDefault :exec
DELETE FROM item_wholesale_tiers WHERE tenant_id = $1 AND item_id = $2 AND outlet_id IS NULL;

-- name: ItemTierDeleteOutlets :exec
-- Menghapus set tier khusus cabang (kembali ke set default) untuk outlet yang boleh diakses pemanggil.
DELETE FROM item_wholesale_tiers
WHERE tenant_id = @tenant_id AND item_id = @item_id AND outlet_id = ANY(@outlet_ids::uuid[]);

-- name: ItemTierInsert :exec
INSERT INTO item_wholesale_tiers (tenant_id, item_id, outlet_id, min_qty, price)
VALUES (@tenant_id, @item_id, sqlc.narg('outlet_id'), @min_qty, @price);

-- name: ItemUnitList :many
SELECT iu.unit_id, u.name AS unit_name, iu.factor, iu.barcode, iu.sell_price, iu.position
FROM item_units iu
JOIN units u ON u.tenant_id = iu.tenant_id AND u.id = iu.unit_id
WHERE iu.tenant_id = $1 AND iu.item_id = $2
ORDER BY iu.position;

-- name: ItemUnitDeleteAll :exec
DELETE FROM item_units WHERE tenant_id = $1 AND item_id = $2;

-- name: ItemUnitInsert :exec
INSERT INTO item_units (tenant_id, item_id, unit_id, factor, barcode, sell_price, position)
VALUES (@tenant_id, @item_id, @unit_id, @factor, sqlc.narg('barcode'), sqlc.narg('sell_price'), @position);

-- name: ItemUnitStates :many
SELECT id, active FROM units WHERE tenant_id = @tenant_id AND id = ANY(@ids::uuid[]);


-- name: ItemByBarcode :many
-- Semua barang yang memakai barcode ini, sebagai barcode barang maupun barcode satuan tambahan (barcode boleh kembar).
-- exclude_id: abaikan satu item (form ubah item tidak perlu diperingatkan oleh dirinya sendiri).
SELECT m.id, m.sku, m.name, m.origin, m.active, m.matched, m.unit_id, m.unit_name, m.factor, m.unit_price, m.default_price, m.outlet_price
FROM (
    SELECT i.id, i.sku, i.name, i.origin, i.active, 'item'::text AS matched, i.unit_id, u.name AS unit_name,
           1::numeric AS factor, NULL::numeric AS unit_price,
           i.sell_price AS default_price, op.sell_price AS outlet_price
    FROM items i
    JOIN units u ON u.tenant_id = i.tenant_id AND u.id = i.unit_id
    LEFT JOIN item_outlet_prices op ON op.tenant_id = i.tenant_id AND op.item_id = i.id AND op.outlet_id = @outlet_id
    WHERE i.tenant_id = @tenant_id AND i.barcode = @barcode::text
      AND (sqlc.narg('exclude_id')::uuid IS NULL OR i.id <> sqlc.narg('exclude_id')::uuid)
    UNION ALL
    SELECT i.id, i.sku, i.name, i.origin, i.active, 'unit'::text AS matched, iu.unit_id, au.name AS unit_name,
           iu.factor::numeric AS factor, iu.sell_price::numeric AS unit_price,
           i.sell_price AS default_price, op.sell_price AS outlet_price
    FROM item_units iu
    JOIN items i ON i.tenant_id = iu.tenant_id AND i.id = iu.item_id
    JOIN units au ON au.tenant_id = iu.tenant_id AND au.id = iu.unit_id
    LEFT JOIN item_outlet_prices op ON op.tenant_id = i.tenant_id AND op.item_id = i.id AND op.outlet_id = @outlet_id
    WHERE iu.tenant_id = @tenant_id AND iu.barcode = @barcode::text
      AND (sqlc.narg('exclude_id')::uuid IS NULL OR i.id <> sqlc.narg('exclude_id')::uuid)
) m
ORDER BY m.active DESC, lower(m.name), m.sku
LIMIT 20;
