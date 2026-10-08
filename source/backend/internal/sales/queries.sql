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

-- name: SalesList :many
SELECT s.id, s.doc_no, s.status, s.total, s.paid, s.created_at,
       coalesce(u.name, '')::text AS cashier_name,
       coalesce(mb.name, '')::text AS member_name,
       (SELECT count(*) FROM sale_lines l WHERE l.tenant_id = s.tenant_id AND l.sale_id = s.id)::int AS line_count,
       -- Tunai dihitung bersih (diterima - kembalian); metode lain apa adanya.
       coalesce((SELECT string_agg(m.method || ':' || m.amt::text, ',' ORDER BY m.method)
                 FROM (SELECT p.method, sum(p.amount) - CASE WHEN p.method = 'cash' THEN s.change ELSE 0 END AS amt
                       FROM sale_payments p WHERE p.tenant_id = s.tenant_id AND p.sale_id = s.id GROUP BY p.method) m), '')::text AS pay_amounts
FROM sales s
JOIN outlets o ON o.tenant_id = s.tenant_id AND o.id = s.outlet_id
LEFT JOIN users u ON u.tenant_id = s.tenant_id AND u.id = s.cashier_id
LEFT JOIN members mb ON mb.tenant_id = s.tenant_id AND mb.id = s.member_id
WHERE s.tenant_id = @tenant_id AND s.outlet_id = @outlet_id
  AND (@all_cashiers::bool OR s.cashier_id = @cashier_id)
  AND (s.created_at AT TIME ZONE o.timezone)::date BETWEEN @from_day::date AND @to_day::date
  AND (@q::text = '' OR s.doc_no ILIKE '%' || @q::text || '%')
ORDER BY s.created_at DESC, s.doc_no DESC
LIMIT 500;

-- name: SalesVoucherInsert :exec
INSERT INTO sale_vouchers (tenant_id, sale_id, voucher_id, position, code, name, kind, value, amount)
VALUES (@tenant_id, @sale_id, @voucher_id, @position, @code, @name, @kind, @value, @amount);

-- name: SalesVouchers :many
SELECT code, name, kind, value, amount FROM sale_vouchers
WHERE tenant_id = @tenant_id AND sale_id = @sale_id ORDER BY position;

-- name: SalesListAll :many
-- Daftar penjualan lintas kasir untuk pemegang sales_list.view: satu atau beberapa outlet (outlet_ids = yang boleh diakses
-- pemanggil), semua kasir. Keyset: urut (created_at, id) menurun, @has_cursor + (cursor_at, cursor_id) = halaman berikutnya.
-- Nilai HPP (cost) selalu dibaca; yang memutuskan dikirim ke klien adalah service (izin sales_cost).
SELECT s.id, s.doc_no, s.status, s.created_at, s.outlet_id, o.code AS outlet_code, o.name AS outlet_name,
       coalesce(u.name, '')::text AS cashier_name,
       coalesce(mb.name, '')::text AS member_name,
       coalesce(sp.name, '')::text AS salesperson_name,
       s.subtotal, s.discount, s.tax_store, s.tax_gov, s.other_cost, s.total, s.change,
       s.points_earned, s.points_redeemed, s.redeem_amount,
       lc.line_count, lc.line_discount, lc.cost, lc.override_count,
       vc.voucher_amount, vc.voucher_codes,
       coalesce((SELECT string_agg(m.method || ':' || m.amt::text, ',' ORDER BY m.method)
                 FROM (SELECT p.method, sum(p.amount) - CASE WHEN p.method = 'cash' THEN s.change ELSE 0 END AS amt
                       FROM sale_payments p WHERE p.tenant_id = s.tenant_id AND p.sale_id = s.id GROUP BY p.method) m), '')::text AS pay_amounts
FROM sales s
JOIN outlets o ON o.tenant_id = s.tenant_id AND o.id = s.outlet_id
LEFT JOIN users u ON u.tenant_id = s.tenant_id AND u.id = s.cashier_id
LEFT JOIN members mb ON mb.tenant_id = s.tenant_id AND mb.id = s.member_id
LEFT JOIN salespeople sp ON sp.tenant_id = s.tenant_id AND sp.id = s.salesperson_id
CROSS JOIN LATERAL (
    SELECT count(*)::int AS line_count,
           coalesce(sum(l.discount), 0)::numeric AS line_discount,
           coalesce(sum(l.unit_cost * l.qty), 0)::numeric AS cost,
           count(*) FILTER (WHERE l.price_override)::int AS override_count
    FROM sale_lines l WHERE l.tenant_id = s.tenant_id AND l.sale_id = s.id
) lc
CROSS JOIN LATERAL (
    SELECT coalesce(sum(v.amount), 0)::numeric AS voucher_amount,
           coalesce(string_agg(v.code, ',' ORDER BY v.position), '')::text AS voucher_codes
    FROM sale_vouchers v WHERE v.tenant_id = s.tenant_id AND v.sale_id = s.id
) vc
WHERE s.tenant_id = @tenant_id AND s.outlet_id = ANY(@outlet_ids::uuid[])
  AND (s.created_at AT TIME ZONE o.timezone)::date BETWEEN @from_day::date AND @to_day::date
  AND (@status::text = '' OR s.status = @status::text)
  AND (@cashier_id::uuid = '00000000-0000-0000-0000-000000000000' OR s.cashier_id = @cashier_id::uuid)
  AND (@method::text = '' OR EXISTS (SELECT 1 FROM sale_payments p WHERE p.tenant_id = s.tenant_id AND p.sale_id = s.id AND p.method = @method::text))
  AND (@q::text = '' OR s.doc_no ILIKE '%' || @q::text || '%' OR mb.name ILIKE '%' || @q::text || '%'
       OR mb.code ILIKE '%' || @q::text || '%' OR u.name ILIKE '%' || @q::text || '%')
  AND (NOT @has_cursor::bool OR (s.created_at, s.id) < (@cursor_at::timestamptz, @cursor_id::uuid))
ORDER BY s.created_at DESC, s.id DESC
LIMIT @page_limit;

-- name: SalesListAllSummary :one
-- Ringkasan atas SELURUH hasil filter (bukan hanya halaman yang tampil). Uang hanya dari nota berstatus completed.
SELECT count(*)::int AS sale_count,
       (count(*) FILTER (WHERE s.status = 'completed'))::int AS completed_count,
       coalesce(sum(s.total) FILTER (WHERE s.status = 'completed'), 0)::numeric AS total,
       coalesce(sum(s.discount + lc.line_discount) FILTER (WHERE s.status = 'completed'), 0)::numeric AS discount,
       coalesce(sum(lc.cost) FILTER (WHERE s.status = 'completed'), 0)::numeric AS cost,
       coalesce(sum(s.subtotal - s.discount) FILTER (WHERE s.status = 'completed'), 0)::numeric AS net_sales
FROM sales s
JOIN outlets o ON o.tenant_id = s.tenant_id AND o.id = s.outlet_id
LEFT JOIN members mb ON mb.tenant_id = s.tenant_id AND mb.id = s.member_id
LEFT JOIN users u ON u.tenant_id = s.tenant_id AND u.id = s.cashier_id
CROSS JOIN LATERAL (
    SELECT coalesce(sum(l.discount), 0)::numeric AS line_discount,
           coalesce(sum(l.unit_cost * l.qty), 0)::numeric AS cost
    FROM sale_lines l WHERE l.tenant_id = s.tenant_id AND l.sale_id = s.id
) lc
WHERE s.tenant_id = @tenant_id AND s.outlet_id = ANY(@outlet_ids::uuid[])
  AND (s.created_at AT TIME ZONE o.timezone)::date BETWEEN @from_day::date AND @to_day::date
  AND (@status::text = '' OR s.status = @status::text)
  AND (@cashier_id::uuid = '00000000-0000-0000-0000-000000000000' OR s.cashier_id = @cashier_id::uuid)
  AND (@method::text = '' OR EXISTS (SELECT 1 FROM sale_payments p WHERE p.tenant_id = s.tenant_id AND p.sale_id = s.id AND p.method = @method::text))
  AND (@q::text = '' OR s.doc_no ILIKE '%' || @q::text || '%' OR mb.name ILIKE '%' || @q::text || '%'
       OR mb.code ILIKE '%' || @q::text || '%' OR u.name ILIKE '%' || @q::text || '%');

-- name: SalesListAllMethodTotals :many
-- Jumlah per metode bayar (tunai bersih dari kembalian) untuk nota completed pada filter yang sama.
SELECT m.method::text AS method, sum(m.amt)::numeric AS amount
FROM sales s
JOIN outlets o ON o.tenant_id = s.tenant_id AND o.id = s.outlet_id
LEFT JOIN members mb ON mb.tenant_id = s.tenant_id AND mb.id = s.member_id
LEFT JOIN users u ON u.tenant_id = s.tenant_id AND u.id = s.cashier_id
CROSS JOIN LATERAL (
    SELECT p.method, sum(p.amount) - CASE WHEN p.method = 'cash' THEN s.change ELSE 0 END AS amt
    FROM sale_payments p WHERE p.tenant_id = s.tenant_id AND p.sale_id = s.id GROUP BY p.method
) m
WHERE s.tenant_id = @tenant_id AND s.outlet_id = ANY(@outlet_ids::uuid[]) AND s.status = 'completed'
  AND (s.created_at AT TIME ZONE o.timezone)::date BETWEEN @from_day::date AND @to_day::date
  AND (@cashier_id::uuid = '00000000-0000-0000-0000-000000000000' OR s.cashier_id = @cashier_id::uuid)
  AND (@method::text = '' OR EXISTS (SELECT 1 FROM sale_payments p WHERE p.tenant_id = s.tenant_id AND p.sale_id = s.id AND p.method = @method::text))
  AND (@q::text = '' OR s.doc_no ILIKE '%' || @q::text || '%' OR mb.name ILIKE '%' || @q::text || '%'
       OR mb.code ILIKE '%' || @q::text || '%' OR u.name ILIKE '%' || @q::text || '%')
GROUP BY m.method
ORDER BY m.method;

-- name: SalesStockMovements :many
-- Gerakan stok yang ditimbulkan satu nota (jual, pembatalan, retur). Satuan = satuan dasar barang.
SELECT m.id, m.created_at, m.ref_type, m.bucket, m.qty_delta, m.balance_after, m.item_id,
       i.sku, i.name, un.name AS unit_name, coalesce(u.name, '')::text AS actor_name
FROM stock_movements m
JOIN items i ON i.tenant_id = m.tenant_id AND i.id = m.item_id
JOIN units un ON un.tenant_id = i.tenant_id AND un.id = i.unit_id
LEFT JOIN users u ON u.tenant_id = m.tenant_id AND u.id = m.actor_id
WHERE m.tenant_id = @tenant_id AND m.ref_id = @sale_id AND m.ref_type IN ('SALE', 'SALE_VOID', 'SALE_RETURN')
ORDER BY m.id;

-- name: SalesAuditEvents :many
-- Riwayat audit satu nota (buat, ubah harga/potongan disetujui PIN; edit/void nanti ikut tercatat di sini).
SELECT action, actor_name, details, created_at FROM audit_log
WHERE tenant_id = @tenant_id AND entity = 'sale' AND entity_id = @sale_id::text
ORDER BY id;

-- name: SalesCostInsert :exec
INSERT INTO sale_costs (tenant_id, sale_id, position, name, amount)
VALUES (@tenant_id, @sale_id, @position, @name, @amount);

-- name: SalesCosts :many
SELECT name, amount FROM sale_costs WHERE tenant_id = @tenant_id AND sale_id = @sale_id ORDER BY position;
