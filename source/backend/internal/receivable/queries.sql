-- name: ReceivableMemberTerms :one
-- Syarat kredit member (limit 0 = tanpa batas; jatuh tempo 0 hari = tanpa jatuh tempo).
SELECT credit_limit, due_days FROM members WHERE tenant_id = $1 AND id = $2;

-- name: ReceivableOutstanding :one
-- Sisa piutang member dari nota yang masih berlaku, di luar nota `exclude_sale_id` (revisi nota membuang piutang lamanya).
SELECT coalesce(sum(r.amount - coalesce(p.paid, 0)), 0)::numeric AS outstanding
FROM receivables r
JOIN sales s ON s.tenant_id = r.tenant_id AND s.id = r.sale_id AND s.status = 'completed'
LEFT JOIN LATERAL (
    SELECT sum(rp.amount) AS paid FROM receivable_payments rp
    WHERE rp.tenant_id = r.tenant_id AND rp.receivable_id = r.id
) p ON true
WHERE r.tenant_id = @tenant_id AND r.member_id = @member_id AND r.sale_id <> @exclude_sale_id;

-- name: ReceivableInsert :exec
INSERT INTO receivables (tenant_id, outlet_id, sale_id, member_id, amount, due_date)
VALUES (@tenant_id, @outlet_id, @sale_id, @member_id, @amount, sqlc.narg('due_date'));

-- name: ReceivablePaymentCountForSale :one
SELECT count(*) FROM receivable_payments rp
JOIN receivables r ON r.tenant_id = rp.tenant_id AND r.id = rp.receivable_id
WHERE r.tenant_id = $1 AND r.sale_id = $2;

-- name: ReceivableList :many
-- Daftar piutang nota yang masih berlaku (completed). Filter: member, status (open|overdue|paid|all), cari (no. nota / kode / nama member).
SELECT r.id, r.sale_id, r.outlet_id, r.member_id, r.amount, r.due_date, r.created_at,
       s.doc_no, s.total AS sale_total, m.code AS member_code, m.name AS member_name,
       coalesce(p.paid, 0)::numeric AS paid,
       (now() AT TIME ZONE o.timezone)::date AS today
FROM receivables r
JOIN sales s ON s.tenant_id = r.tenant_id AND s.id = r.sale_id AND s.status = 'completed'
JOIN members m ON m.tenant_id = r.tenant_id AND m.id = r.member_id
JOIN outlets o ON o.tenant_id = r.tenant_id AND o.id = r.outlet_id
LEFT JOIN LATERAL (
    SELECT sum(rp.amount) AS paid FROM receivable_payments rp
    WHERE rp.tenant_id = r.tenant_id AND rp.receivable_id = r.id
) p ON true
WHERE r.tenant_id = @tenant_id
  AND r.outlet_id = ANY(@outlet_ids::uuid[])
  AND (sqlc.narg('member_id')::uuid IS NULL OR r.member_id = sqlc.narg('member_id')::uuid)
  AND (@q::text = '' OR s.doc_no ILIKE '%' || @q || '%' OR m.code ILIKE '%' || @q || '%' OR m.name ILIKE '%' || @q || '%')
  AND (
        @status::text = 'all'
     OR (@status::text = 'paid'    AND r.amount - coalesce(p.paid, 0) <= 0)
     OR (@status::text = 'open'    AND r.amount - coalesce(p.paid, 0) > 0)
     OR (@status::text = 'overdue' AND r.amount - coalesce(p.paid, 0) > 0 AND r.due_date IS NOT NULL
                                    AND r.due_date < (now() AT TIME ZONE o.timezone)::date)
  )
ORDER BY (r.amount - coalesce(p.paid, 0) <= 0), r.due_date NULLS LAST, r.created_at DESC, r.id
LIMIT @lim OFFSET @off;

-- name: ReceivableSummary :one
-- Total sisa piutang, yang lewat jatuh tempo, dan jumlah nota yang masih terbuka (sesuai cakupan outlet + member).
SELECT coalesce(sum(r.amount - coalesce(p.paid, 0)) FILTER (WHERE r.amount - coalesce(p.paid, 0) > 0), 0)::numeric AS outstanding,
       coalesce(sum(r.amount - coalesce(p.paid, 0)) FILTER (WHERE r.amount - coalesce(p.paid, 0) > 0 AND r.due_date IS NOT NULL
                                                             AND r.due_date < (now() AT TIME ZONE o.timezone)::date), 0)::numeric AS overdue,
       count(*) FILTER (WHERE r.amount - coalesce(p.paid, 0) > 0) AS open_count,
       count(*) AS total_count
FROM receivables r
JOIN sales s ON s.tenant_id = r.tenant_id AND s.id = r.sale_id AND s.status = 'completed'
JOIN outlets o ON o.tenant_id = r.tenant_id AND o.id = r.outlet_id
LEFT JOIN LATERAL (
    SELECT sum(rp.amount) AS paid FROM receivable_payments rp
    WHERE rp.tenant_id = r.tenant_id AND rp.receivable_id = r.id
) p ON true
WHERE r.tenant_id = @tenant_id AND r.outlet_id = ANY(@outlet_ids::uuid[])
  AND (sqlc.narg('member_id')::uuid IS NULL OR r.member_id = sqlc.narg('member_id')::uuid);

-- name: ReceivableGet :one
SELECT r.id, r.sale_id, r.outlet_id, r.member_id, r.amount, r.due_date, r.created_at,
       s.doc_no, s.total AS sale_total, s.created_at AS sale_at, m.code AS member_code, m.name AS member_name,
       coalesce(p.paid, 0)::numeric AS paid,
       (now() AT TIME ZONE o.timezone)::date AS today
FROM receivables r
JOIN sales s ON s.tenant_id = r.tenant_id AND s.id = r.sale_id AND s.status = 'completed'
JOIN members m ON m.tenant_id = r.tenant_id AND m.id = r.member_id
JOIN outlets o ON o.tenant_id = r.tenant_id AND o.id = r.outlet_id
LEFT JOIN LATERAL (
    SELECT sum(rp.amount) AS paid FROM receivable_payments rp
    WHERE rp.tenant_id = r.tenant_id AND rp.receivable_id = r.id
) p ON true
WHERE r.tenant_id = $1 AND r.id = $2;

-- name: ReceivablePaymentsList :many
SELECT rp.id, rp.doc_no, rp.method, rp.method_id, rp.method_name, rp.amount, rp.ref_no, rp.fee_pct, rp.fee_amount, rp.fee_bearer,
       rp.note, rp.created_at, coalesce(u.name, '')::text AS received_by_name
FROM receivable_payments rp
LEFT JOIN users u ON u.tenant_id = rp.tenant_id AND u.id = rp.received_by
WHERE rp.tenant_id = $1 AND rp.receivable_id = $2
ORDER BY rp.created_at, rp.id;

-- name: ReceivableLockForPay :one
-- Mengunci NOTA asal piutang (role aplikasi tak punya UPDATE pada receivables, jadi baris piutang tidak bisa dikunci; kunci nota
-- cukup karena semua pembayaran dan edit/batal nota berbagi kunci itu). Syarat status completed dievaluasi ulang setelah
-- menunggu kunci, jadi pembayaran vs edit/batal bersamaan tepat satu yang menang.
SELECT r.id, r.sale_id, r.outlet_id, r.member_id, r.amount, s.doc_no
FROM receivables r
JOIN sales s ON s.tenant_id = r.tenant_id AND s.id = r.sale_id
WHERE r.tenant_id = $1 AND r.id = $2 AND s.status = 'completed'
FOR UPDATE OF s;

-- name: ReceivablePaidTotal :one
SELECT coalesce(sum(amount), 0)::numeric FROM receivable_payments WHERE tenant_id = $1 AND receivable_id = $2;

-- name: ReceivablePaymentByIdemKey :one
SELECT id, receivable_id, request_hash FROM receivable_payments WHERE tenant_id = $1 AND idempotency_key = $2;

-- name: ReceivablePaymentNextNo :one
INSERT INTO receivable_payment_counters (tenant_id, outlet_id, day, last_no) VALUES (@tenant_id, @outlet_id, @day, 1)
ON CONFLICT (tenant_id, outlet_id, day) DO UPDATE SET last_no = receivable_payment_counters.last_no + 1
RETURNING last_no;

-- name: ReceivablePaymentInsert :exec
INSERT INTO receivable_payments (tenant_id, receivable_id, outlet_id, doc_no, idempotency_key, request_hash, method, method_id, method_name,
                                 amount, ref_no, fee_pct, fee_flat, fee_amount, fee_bearer, note, received_by)
VALUES (@tenant_id, @receivable_id, @outlet_id, @doc_no, @idempotency_key, @request_hash, @method, @method_id, @method_name,
        @amount, @ref_no, @fee_pct, @fee_flat, @fee_amount, @fee_bearer, @note, sqlc.narg('received_by'));

-- name: ReceivablePaymentMethod :one
SELECT id, name, kind, active, fee_pct, fee_flat, fee_bearer FROM payment_methods WHERE tenant_id = $1 AND id = $2;

-- name: ReceivablePayOutlet :one
SELECT o.code, o.active, (now() AT TIME ZONE o.timezone)::date AS local_day
FROM outlets o WHERE o.tenant_id = $1 AND o.id = $2;

-- name: ReceivableForSale :one
-- Piutang milik satu nota (bila nota kredit) beserta jumlah yang sudah dibayar.
SELECT r.id, r.amount, r.due_date, coalesce(p.paid, 0)::numeric AS paid, (now() AT TIME ZONE o.timezone)::date AS today
FROM receivables r
JOIN outlets o ON o.tenant_id = r.tenant_id AND o.id = r.outlet_id
LEFT JOIN LATERAL (
    SELECT sum(rp.amount) AS paid FROM receivable_payments rp
    WHERE rp.tenant_id = r.tenant_id AND rp.receivable_id = r.id
) p ON true
WHERE r.tenant_id = $1 AND r.sale_id = $2;
