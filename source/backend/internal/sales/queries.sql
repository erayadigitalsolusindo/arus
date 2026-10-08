-- name: SalesOutletInfo :one
SELECT code, name, tax_store_pct, tax_gov_pct, active,
       (now() AT TIME ZONE timezone)::date AS local_day
FROM outlets WHERE tenant_id = $1 AND id = $2;

-- name: SalesItemsForPricing :many
-- Data harga/stok semua barang dalam satu nota (satu kueri). outlet_price NULL = pakai harga default.
SELECT i.id, i.sku, i.name, i.kind, i.active, i.sell_below_cost, i.allow_negative_stock, i.avg_cost, i.sell_price,
       i.unit_id, u.name AS unit_name, op.sell_price AS outlet_price,
       coalesce(sb.qty, 0)::numeric AS stock_display
FROM items i
JOIN units u ON u.tenant_id = i.tenant_id AND u.id = i.unit_id
LEFT JOIN item_outlet_prices op ON op.tenant_id = i.tenant_id AND op.item_id = i.id AND op.outlet_id = @outlet_id
LEFT JOIN stock_balances sb ON sb.tenant_id = i.tenant_id AND sb.outlet_id = @outlet_id AND sb.item_id = i.id AND sb.bucket = 'display'
WHERE i.tenant_id = @tenant_id AND i.id = ANY(@ids::uuid[]);

-- name: SalesTiers :many
-- Tier grosir default (outlet_id NULL) dan milik outlet ini untuk barang-barang nota.
SELECT item_id, outlet_id, min_qty, price FROM item_wholesale_tiers
WHERE tenant_id = @tenant_id AND item_id = ANY(@ids::uuid[]) AND (outlet_id IS NULL OR outlet_id = @outlet_id)
ORDER BY item_id, outlet_id NULLS FIRST, min_qty;

-- name: SalesAltUnits :many
SELECT iu.item_id, iu.unit_id, u.name AS unit_name, iu.factor, iu.sell_price
FROM item_units iu
JOIN units u ON u.tenant_id = iu.tenant_id AND u.id = iu.unit_id
WHERE iu.tenant_id = @tenant_id AND iu.item_id = ANY(@ids::uuid[]);

-- name: SalesNextNo :one
INSERT INTO sale_counters (tenant_id, outlet_id, day, last_no) VALUES (@tenant_id, @outlet_id, @day, 1)
ON CONFLICT (tenant_id, outlet_id, day) DO UPDATE SET last_no = sale_counters.last_no + 1
RETURNING last_no;

-- name: SalesInsert :one
-- DO NOTHING pada kunci idempotensi: pengiriman ulang tidak membuat nota ganda (pemanggil lalu membaca nota lama).
INSERT INTO sales (tenant_id, outlet_id, doc_no, idempotency_key, request_hash, cashier_id, approved_by, note, subtotal, discount,
                   tax_store_pct, tax_gov_pct, tax_store, tax_gov, other_cost, total, paid, change,
                   member_id, points_earned, points_redeemed, redeem_amount, salesperson_id)
VALUES (@tenant_id, @outlet_id, @doc_no, @idempotency_key, @request_hash, sqlc.narg('cashier_id'), sqlc.narg('approved_by'), @note, @subtotal, @discount,
        @tax_store_pct, @tax_gov_pct, @tax_store, @tax_gov, @other_cost, @total, @paid, @change,
        sqlc.narg('member_id'), @points_earned, @points_redeemed, @redeem_amount, sqlc.narg('salesperson_id'))
ON CONFLICT (tenant_id, idempotency_key) DO NOTHING
RETURNING id, created_at;

-- name: SalesByIdemKey :one
SELECT id, request_hash FROM sales WHERE tenant_id = $1 AND idempotency_key = $2;

-- name: SalesLineInsert :exec
INSERT INTO sale_lines (tenant_id, sale_id, position, item_id, sku, name, unit_id, unit_name, factor, qty, unit_price,
                        unit_cost, discount, line_total, note, list_price, price_override)
VALUES (@tenant_id, @sale_id, @position, @item_id, @sku, @name, @unit_id, @unit_name, @factor, @qty, @unit_price,
        @unit_cost, @discount, @line_total, @note, @list_price, @price_override);

-- name: SalesPaymentInsert :exec
INSERT INTO sale_payments (tenant_id, sale_id, position, method, amount, ref_no)
VALUES (@tenant_id, @sale_id, @position, @method, @amount, @ref_no);

-- name: SalesGet :one
SELECT s.id, s.outlet_id, s.doc_no, s.status, s.note, s.subtotal, s.discount, s.tax_store_pct, s.tax_gov_pct,
       s.tax_store, s.tax_gov, s.other_cost, s.total, s.paid, s.change, s.created_at,
       s.cashier_id, coalesce(u.name, '')::text AS cashier_name, coalesce(ap.name, '')::text AS approver_name,
       s.member_id, coalesce(mb.code, '')::text AS member_code, coalesce(mb.name, '')::text AS member_name,
       s.points_earned, s.points_redeemed, s.redeem_amount,
       s.salesperson_id, coalesce(sp.name, '')::text AS salesperson_name
FROM sales s LEFT JOIN salespeople sp ON sp.tenant_id = s.tenant_id AND sp.id = s.salesperson_id
LEFT JOIN users u ON u.tenant_id = s.tenant_id AND u.id = s.cashier_id
LEFT JOIN members mb ON mb.tenant_id = s.tenant_id AND mb.id = s.member_id
LEFT JOIN users ap ON ap.tenant_id = s.tenant_id AND ap.id = s.approved_by
WHERE s.tenant_id = $1 AND s.id = $2;

-- name: SalesLines :many
SELECT item_id, sku, name, unit_id, unit_name, factor, qty, unit_price, unit_cost, discount, line_total, note, list_price, price_override
FROM sale_lines WHERE tenant_id = $1 AND sale_id = $2 ORDER BY position;

-- name: SalesPayments :many
SELECT method, amount, ref_no FROM sale_payments WHERE tenant_id = $1 AND sale_id = $2 ORDER BY position;

-- name: SalesSalespersonState :one
SELECT active, name FROM salespeople WHERE tenant_id = $1 AND id = $2;
