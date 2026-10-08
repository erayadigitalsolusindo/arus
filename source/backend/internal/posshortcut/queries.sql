-- name: PosShortcutList :many
-- Pintasan milik satu kasir beserta data barang; harga = harga efektif outlet aktif (harga cabang bila ada, selain itu default).
SELECT s.slot, i.id, i.sku, i.name, i.kind, i.active, i.sell_price AS default_price, op.sell_price AS outlet_price,
       u.name AS unit_name, mi.id AS main_image_id
FROM pos_shortcuts s
JOIN items i ON i.tenant_id = s.tenant_id AND i.id = s.item_id
JOIN units u ON u.tenant_id = i.tenant_id AND u.id = i.unit_id
LEFT JOIN item_images mi ON mi.tenant_id = i.tenant_id AND mi.item_id = i.id AND mi.is_main
LEFT JOIN item_outlet_prices op ON op.tenant_id = i.tenant_id AND op.item_id = i.id AND op.outlet_id = @outlet_id
WHERE s.tenant_id = @tenant_id AND s.user_id = @user_id
ORDER BY s.slot;

-- name: PosShortcutSet :exec
INSERT INTO pos_shortcuts (tenant_id, user_id, slot, item_id) VALUES ($1, $2, $3, $4)
ON CONFLICT (tenant_id, user_id, slot) DO UPDATE SET item_id = EXCLUDED.item_id, updated_at = now();

-- name: PosShortcutClear :exec
DELETE FROM pos_shortcuts WHERE tenant_id = $1 AND user_id = $2 AND slot = $3;
