-- name: ItemList :many
-- Harga efektif untuk outlet aktif: harga cabang bila ada, selain itu harga default tenant.
SELECT i.id, i.sku, i.barcode, i.name, i.kind, i.active,
       i.sell_price AS default_price, op.sell_price AS outlet_price,
       u.name AS unit_name, c.name AS category_name, b.name AS brand_name,
       count(*) OVER () AS total
FROM items i
JOIN units u ON u.tenant_id = i.tenant_id AND u.id = i.unit_id
LEFT JOIN categories c ON c.tenant_id = i.tenant_id AND c.id = i.category_id
LEFT JOIN brands b ON b.tenant_id = i.tenant_id AND b.id = i.brand_id
LEFT JOIN item_outlet_prices op ON op.tenant_id = i.tenant_id AND op.item_id = i.id AND op.outlet_id = @outlet_id
WHERE i.tenant_id = @tenant_id
  AND (@q::text = '' OR i.name ILIKE '%' || @q || '%' OR i.sku ILIKE '%' || @q || '%' OR coalesce(i.barcode, '') ILIKE '%' || @q || '%')
  AND (sqlc.narg('active')::boolean IS NULL OR i.active = sqlc.narg('active'))
  AND (sqlc.narg('category_id')::uuid IS NULL OR i.category_id = sqlc.narg('category_id'))
ORDER BY lower(i.name), i.id
LIMIT @page_limit OFFSET @page_offset;

-- name: ItemGet :one
SELECT i.id, i.sku, i.barcode, i.name, i.weight_grams, i.last_cost, i.avg_cost, i.sell_price, i.kind,
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
SELECT id, sku, barcode, name, weight_grams, sell_price, kind, allow_negative_stock, sell_below_cost, active,
       unit_id, category_id, brand_id, principal_id, supplier_id
FROM items WHERE tenant_id = $1 AND id = $2 FOR UPDATE;

-- name: ItemCreate :one
INSERT INTO items (tenant_id, sku, barcode, name, weight_grams, last_cost, avg_cost, sell_price, unit_id, category_id,
                   brand_id, principal_id, supplier_id, kind, allow_negative_stock, sell_below_cost, description)
VALUES (@tenant_id, @sku, @barcode, @name, @weight_grams, @cost, @cost, @sell_price, @unit_id, @category_id,
        @brand_id, @principal_id, @supplier_id, @kind, @allow_negative_stock, @sell_below_cost, @description)
RETURNING id;

-- name: ItemUpdate :exec
-- HPP (last_cost/avg_cost) sengaja tidak ada di sini: hanya diubah transaksi pembelian/stok.
UPDATE items SET sku = @sku, barcode = @barcode, name = @name, weight_grams = @weight_grams, sell_price = @sell_price,
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
