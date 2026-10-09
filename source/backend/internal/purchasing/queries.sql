-- name: PurchaseOutletInfo :one
SELECT code, name, active, (now() AT TIME ZONE timezone)::date AS local_day
FROM outlets WHERE tenant_id = $1 AND id = $2;

-- name: PurchaseByIdemKey :one
SELECT id, request_hash FROM purchases WHERE tenant_id = $1 AND idempotency_key = $2;

-- name: PurchaseSupplierState :one
SELECT id, name, active FROM suppliers WHERE tenant_id = $1 AND id = $2;

-- name: PurchaseItemLock :one
-- Mengunci baris barang (dipanggil terurut menurut id) dan membaca data + HPP cabang. HPP = HPP cabang bila ada,
-- selain itu HPP awal barang. Penguncian ini membuat dua pembelian barang yang sama antre (HPP rata-rata tak saling menimpa).
SELECT i.id, i.sku, i.name, i.kind, i.active, u.name AS unit_name,
       coalesce(oc.avg_cost, i.avg_cost)::numeric AS avg_cost
FROM items i JOIN units u ON u.tenant_id = i.tenant_id AND u.id = i.unit_id
LEFT JOIN item_outlet_costs oc ON oc.tenant_id = i.tenant_id AND oc.item_id = i.id AND oc.outlet_id = @outlet_id
WHERE i.tenant_id = @tenant_id AND i.id = @id FOR UPDATE OF i;

-- name: PurchaseItemState :one
-- Seperti PurchaseItemLock tanpa penguncian (pratinjau/quote).
SELECT i.id, i.sku, i.name, i.kind, i.active, u.name AS unit_name,
       coalesce(oc.avg_cost, i.avg_cost)::numeric AS avg_cost
FROM items i JOIN units u ON u.tenant_id = i.tenant_id AND u.id = i.unit_id
LEFT JOIN item_outlet_costs oc ON oc.tenant_id = i.tenant_id AND oc.item_id = i.id AND oc.outlet_id = @outlet_id
WHERE i.tenant_id = @tenant_id AND i.id = @id;

-- name: PurchaseNextNo :one
INSERT INTO purchase_counters (tenant_id, outlet_id, day, last_no) VALUES (@tenant_id, @outlet_id, @day, 1)
ON CONFLICT (tenant_id, outlet_id, day) DO UPDATE SET last_no = purchase_counters.last_no + 1
RETURNING last_no;

-- name: PurchaseInsert :one
-- Pengiriman ganda bersamaan (kunci idempotensi sama): yang kalah tidak menghasilkan baris → pemanggil membatalkan transaksi.
INSERT INTO purchases (id, tenant_id, outlet_id, doc_no, idempotency_key, request_hash, supplier_id, supplier_invoice_no, purchase_date,
                       payment_type, due_date, note, subtotal, tax_pct, tax_amount, other_cost, total, created_by)
VALUES (@id, @tenant_id, @outlet_id, @doc_no, @idempotency_key, @request_hash, @supplier_id, @supplier_invoice_no, @purchase_date,
        @payment_type, sqlc.narg('due_date'), @note, @subtotal, @tax_pct, @tax_amount, @other_cost, @total, sqlc.narg('created_by'))
ON CONFLICT (tenant_id, idempotency_key) DO NOTHING
RETURNING id;

-- name: PurchaseLineInsert :exec
INSERT INTO purchase_lines (tenant_id, purchase_id, position, item_id, item_sku, item_name, unit_name, qty_display, qty_warehouse, qty,
                            unit_price, disc1, disc2, disc3, disc4, line_total, cost_alloc, unit_cost, stock_before, avg_before, avg_after)
VALUES (@tenant_id, @purchase_id, @position, @item_id, @item_sku, @item_name, @unit_name, @qty_display, @qty_warehouse, @qty,
        @unit_price, @disc1, @disc2, @disc3, @disc4, @line_total, @cost_alloc, @unit_cost, @stock_before, @avg_before, @avg_after);

-- name: PurchaseCostInsert :exec
INSERT INTO purchase_costs (tenant_id, purchase_id, position, name, amount)
VALUES (@tenant_id, @purchase_id, @position, @name, @amount);

-- name: PayableInsert :exec
INSERT INTO payables (tenant_id, outlet_id, supplier_id, purchase_id, amount, due_date)
VALUES (@tenant_id, @outlet_id, @supplier_id, @purchase_id, @amount, sqlc.narg('due_date'));

-- name: PurchaseGet :one
SELECT p.id, p.outlet_id, o.code AS outlet_code, o.name AS outlet_name, p.doc_no, p.supplier_id, s.name AS supplier_name,
       p.supplier_invoice_no, p.purchase_date, p.payment_type, p.due_date, p.status, p.note,
       p.subtotal, p.tax_pct, p.tax_amount, p.other_cost, p.total, p.created_at,
       coalesce(u.name, '') AS created_name
FROM purchases p
JOIN outlets o ON o.tenant_id = p.tenant_id AND o.id = p.outlet_id
JOIN suppliers s ON s.tenant_id = p.tenant_id AND s.id = p.supplier_id
LEFT JOIN users u ON u.tenant_id = p.tenant_id AND u.id = p.created_by
WHERE p.tenant_id = $1 AND p.id = $2;

-- name: PurchaseLines :many
SELECT item_id, item_sku, item_name, unit_name, qty_display, qty_warehouse, qty, unit_price, disc1, disc2, disc3, disc4,
       line_total, cost_alloc, unit_cost, stock_before, avg_before, avg_after
FROM purchase_lines WHERE tenant_id = $1 AND purchase_id = $2 ORDER BY position;

-- name: PurchaseCosts :many
SELECT name, amount FROM purchase_costs WHERE tenant_id = $1 AND purchase_id = $2 ORDER BY position;

-- name: PurchasePayable :one
SELECT id, amount, due_date FROM payables WHERE tenant_id = $1 AND purchase_id = $2;

-- name: PurchaseList :many
SELECT p.id, p.doc_no, p.supplier_id, s.name AS supplier_name, p.supplier_invoice_no, p.purchase_date, p.payment_type,
       p.due_date, p.status, p.total, p.created_at, coalesce(u.name, '') AS created_name,
       (SELECT count(*) FROM purchase_lines l WHERE l.tenant_id = p.tenant_id AND l.purchase_id = p.id)::bigint AS line_count,
       count(*) OVER () AS total_rows
FROM purchases p
JOIN suppliers s ON s.tenant_id = p.tenant_id AND s.id = p.supplier_id
LEFT JOIN users u ON u.tenant_id = p.tenant_id AND u.id = p.created_by
WHERE p.tenant_id = @tenant_id AND p.outlet_id = @outlet_id
  AND p.purchase_date >= @from_date AND p.purchase_date <= @to_date
  AND (sqlc.narg('supplier_id')::uuid IS NULL OR p.supplier_id = sqlc.narg('supplier_id'))
  AND (@payment_type::text = '' OR p.payment_type = @payment_type)
  AND (@q::text = '' OR p.doc_no ILIKE '%' || @q || '%' OR p.supplier_invoice_no ILIKE '%' || @q || '%' OR s.name ILIKE '%' || @q || '%')
ORDER BY p.purchase_date DESC, p.created_at DESC, p.id
LIMIT @page_limit OFFSET @page_offset;

-- name: PurchaseListSummary :one
SELECT count(*)::bigint AS cnt, coalesce(sum(p.total), 0)::numeric AS total,
       coalesce(sum(p.total) FILTER (WHERE p.payment_type = 'credit'), 0)::numeric AS credit_total
FROM purchases p
JOIN suppliers s ON s.tenant_id = p.tenant_id AND s.id = p.supplier_id
WHERE p.tenant_id = @tenant_id AND p.outlet_id = @outlet_id
  AND p.purchase_date >= @from_date AND p.purchase_date <= @to_date
  AND (sqlc.narg('supplier_id')::uuid IS NULL OR p.supplier_id = sqlc.narg('supplier_id'))
  AND (@payment_type::text = '' OR p.payment_type = @payment_type)
  AND (@q::text = '' OR p.doc_no ILIKE '%' || @q || '%' OR p.supplier_invoice_no ILIKE '%' || @q || '%' OR s.name ILIKE '%' || @q || '%');

-- name: PurchaseItemSearch :many
-- Pemilih barang di form pembelian: barang berstok aktif + stok total outlet aktif + HPP cabang.
SELECT i.id, i.sku, coalesce(i.barcode, '') AS barcode, i.name, u.name AS unit_name,
       coalesce(oc.avg_cost, i.avg_cost)::numeric AS avg_cost, coalesce(oc.last_cost, i.last_cost)::numeric AS last_cost,
       coalesce((SELECT sum(sb.qty) FROM stock_balances sb WHERE sb.tenant_id = i.tenant_id AND sb.item_id = i.id AND sb.outlet_id = @outlet_id), 0)::numeric AS stock_total
FROM items i
JOIN units u ON u.tenant_id = i.tenant_id AND u.id = i.unit_id
LEFT JOIN item_outlet_costs oc ON oc.tenant_id = i.tenant_id AND oc.item_id = i.id AND oc.outlet_id = @outlet_id
WHERE i.tenant_id = @tenant_id AND i.kind = 'goods' AND i.active
  AND (@q::text = '' OR i.name ILIKE '%' || @q || '%' OR i.sku ILIKE '%' || @q || '%' OR coalesce(i.barcode, '') ILIKE '%' || @q || '%')
ORDER BY lower(i.name), i.id
LIMIT @page_limit;
