-- name: ReceivableMemberTerms :one
-- Syarat kredit member (limit 0 = tanpa batas; jatuh tempo 0 hari = tanpa jatuh tempo).
SELECT credit_limit, due_days FROM members WHERE tenant_id = $1 AND id = $2;

-- name: ReceivableOutstanding :one
-- Sisa piutang member dari nota yang masih berlaku + saldo awal yang tidak dibatalkan, di luar nota `exclude_sale_id`
-- (revisi nota membuang piutang lamanya).
SELECT coalesce(sum(greatest(0, r.amount - coalesce(p.paid, 0) - coalesce(rr.returned, 0))), 0)::numeric AS outstanding
FROM receivables r
LEFT JOIN sales s ON s.tenant_id = r.tenant_id AND s.id = r.sale_id
LEFT JOIN LATERAL (
    SELECT sum(rp.amount) AS paid FROM receivable_payments rp
    WHERE rp.tenant_id = r.tenant_id AND rp.receivable_id = r.id
) p ON true
LEFT JOIN LATERAL (
    SELECT sum(sr.receivable_cut) AS returned FROM sales_returns sr
    WHERE sr.tenant_id = r.tenant_id AND sr.sale_id = r.sale_id AND sr.status = 'completed'
) rr ON true
WHERE r.tenant_id = @tenant_id AND r.member_id = @member_id AND r.voided_at IS NULL
  AND (r.sale_id IS NULL OR s.status = 'completed')
  AND r.sale_id IS DISTINCT FROM @exclude_sale_id::uuid;

-- name: ReceivableInsert :exec
INSERT INTO receivables (tenant_id, outlet_id, sale_id, member_id, amount, due_date)
VALUES (@tenant_id, @outlet_id, @sale_id, @member_id, @amount, sqlc.narg('due_date'));

-- name: ReceivablePaymentCountForSale :one
SELECT count(*) FROM receivable_payments rp
JOIN receivables r ON r.tenant_id = rp.tenant_id AND r.id = rp.receivable_id
WHERE r.tenant_id = $1 AND r.sale_id = $2;

-- name: ReceivableList :many
-- Daftar piutang nota yang masih berlaku (completed) + saldo awal yang tidak dibatalkan. Filter: member, status
-- (open|overdue|paid|all), cari (no. nota / no. saldo awal / no. referensi lama / kode / nama member).
-- created_at = tanggal piutang: waktu nota, atau tanggal dokumen lama untuk saldo awal.
SELECT r.id, r.sale_id, r.outlet_id, r.member_id, r.amount, r.due_date,
       coalesce(s.created_at, r.doc_date::timestamp AT TIME ZONE o.timezone, r.created_at)::timestamptz AS created_at, r.kind, r.ref_no,
       coalesce(s.doc_no, r.doc_no)::text AS doc_no, coalesce(s.total, r.amount)::numeric AS sale_total, m.code AS member_code, m.name AS member_name,
    coalesce(p.paid, 0)::numeric AS paid, coalesce(rr.returned, 0)::numeric AS returned,
       (now() AT TIME ZONE o.timezone)::date AS today
FROM receivables r
LEFT JOIN sales s ON s.tenant_id = r.tenant_id AND s.id = r.sale_id
JOIN members m ON m.tenant_id = r.tenant_id AND m.id = r.member_id
JOIN outlets o ON o.tenant_id = r.tenant_id AND o.id = r.outlet_id
LEFT JOIN LATERAL (
    SELECT sum(rp.amount) AS paid FROM receivable_payments rp
    WHERE rp.tenant_id = r.tenant_id AND rp.receivable_id = r.id
) p ON true
LEFT JOIN LATERAL (
    SELECT sum(sr.receivable_cut) AS returned FROM sales_returns sr
    WHERE sr.tenant_id = r.tenant_id AND sr.sale_id = r.sale_id AND sr.status = 'completed'
) rr ON true
WHERE r.tenant_id = @tenant_id
  AND r.outlet_id = ANY(@outlet_ids::uuid[])
  AND r.voided_at IS NULL AND (r.sale_id IS NULL OR s.status = 'completed')
  AND (sqlc.narg('member_id')::uuid IS NULL OR r.member_id = sqlc.narg('member_id')::uuid)
  AND (@q::text = '' OR coalesce(s.doc_no, r.doc_no) ILIKE '%' || @q || '%' OR r.ref_no ILIKE '%' || @q || '%' OR m.code ILIKE '%' || @q || '%' OR m.name ILIKE '%' || @q || '%')
  AND (
        @status::text = 'all'
    OR (@status::text = 'paid'    AND r.amount - coalesce(p.paid, 0) - coalesce(rr.returned, 0) <= 0)
    OR (@status::text = 'open'    AND r.amount - coalesce(p.paid, 0) - coalesce(rr.returned, 0) > 0)
    OR (@status::text = 'overdue' AND r.amount - coalesce(p.paid, 0) - coalesce(rr.returned, 0) > 0 AND r.due_date IS NOT NULL
                                    AND r.due_date < (now() AT TIME ZONE o.timezone)::date)
  )
ORDER BY (r.amount - coalesce(p.paid, 0) - coalesce(rr.returned, 0) <= 0), r.due_date NULLS LAST, r.created_at DESC, r.id
LIMIT @lim OFFSET @off;

-- name: ReceivableSummary :one
-- Total sisa piutang, yang lewat jatuh tempo, dan jumlah nota yang masih terbuka (sesuai cakupan outlet + member).
SELECT coalesce(sum(greatest(0, r.amount - coalesce(p.paid, 0) - coalesce(rr.returned, 0))) FILTER (WHERE r.amount - coalesce(p.paid, 0) - coalesce(rr.returned, 0) > 0), 0)::numeric AS outstanding,
       coalesce(sum(greatest(0, r.amount - coalesce(p.paid, 0) - coalesce(rr.returned, 0))) FILTER (WHERE r.amount - coalesce(p.paid, 0) - coalesce(rr.returned, 0) > 0 AND r.due_date IS NOT NULL
                                                             AND r.due_date < (now() AT TIME ZONE o.timezone)::date), 0)::numeric AS overdue,
       count(*) FILTER (WHERE r.amount - coalesce(p.paid, 0) - coalesce(rr.returned, 0) > 0) AS open_count,
       count(*) AS total_count
FROM receivables r
LEFT JOIN sales s ON s.tenant_id = r.tenant_id AND s.id = r.sale_id
JOIN outlets o ON o.tenant_id = r.tenant_id AND o.id = r.outlet_id
LEFT JOIN LATERAL (
    SELECT sum(rp.amount) AS paid FROM receivable_payments rp
    WHERE rp.tenant_id = r.tenant_id AND rp.receivable_id = r.id
) p ON true
LEFT JOIN LATERAL (
    SELECT sum(sr.receivable_cut) AS returned FROM sales_returns sr
    WHERE sr.tenant_id = r.tenant_id AND sr.sale_id = r.sale_id AND sr.status = 'completed'
) rr ON true
WHERE r.tenant_id = @tenant_id AND r.outlet_id = ANY(@outlet_ids::uuid[])
  AND r.voided_at IS NULL AND (r.sale_id IS NULL OR s.status = 'completed')
  AND (sqlc.narg('member_id')::uuid IS NULL OR r.member_id = sqlc.narg('member_id')::uuid);

-- name: ReceivableGet :one
-- Satu piutang: nota completed, atau saldo awal (termasuk yang sudah dibatalkan, untuk riwayat).
SELECT r.id, r.sale_id, r.outlet_id, r.member_id, r.amount, r.due_date, r.created_at, r.kind, r.ref_no, r.doc_date, r.note,
       r.voided_at, r.void_reason, coalesce(cu.name, '')::text AS created_by_name, coalesce(vu.name, '')::text AS voided_by_name,
       coalesce(s.doc_no, r.doc_no)::text AS doc_no, coalesce(s.total, r.amount)::numeric AS sale_total,
       coalesce(s.created_at, (r.doc_date::timestamp AT TIME ZONE o.timezone))::timestamptz AS sale_at, m.code AS member_code, m.name AS member_name,
    coalesce(p.paid, 0)::numeric AS paid, coalesce(rr.returned, 0)::numeric AS returned,
       (now() AT TIME ZONE o.timezone)::date AS today
FROM receivables r
LEFT JOIN sales s ON s.tenant_id = r.tenant_id AND s.id = r.sale_id
JOIN members m ON m.tenant_id = r.tenant_id AND m.id = r.member_id
JOIN outlets o ON o.tenant_id = r.tenant_id AND o.id = r.outlet_id
LEFT JOIN users cu ON cu.tenant_id = r.tenant_id AND cu.id = r.created_by
LEFT JOIN users vu ON vu.tenant_id = r.tenant_id AND vu.id = r.voided_by
LEFT JOIN LATERAL (
    SELECT sum(rp.amount) AS paid FROM receivable_payments rp
    WHERE rp.tenant_id = r.tenant_id AND rp.receivable_id = r.id
) p ON true
LEFT JOIN LATERAL (
    SELECT sum(sr.receivable_cut) AS returned FROM sales_returns sr
    WHERE sr.tenant_id = r.tenant_id AND sr.sale_id = r.sale_id AND sr.status = 'completed'
) rr ON true
WHERE r.tenant_id = $1 AND r.id = $2 AND (r.sale_id IS NULL OR s.status = 'completed');

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
SELECT r.id, r.sale_id, r.outlet_id, r.member_id, r.amount, s.doc_no, coalesce(rr.returned, 0)::numeric AS returned
FROM receivables r
JOIN sales s ON s.tenant_id = r.tenant_id AND s.id = r.sale_id
LEFT JOIN LATERAL (
    SELECT sum(sr.receivable_cut) AS returned FROM sales_returns sr
    WHERE sr.tenant_id = r.tenant_id AND sr.sale_id = r.sale_id AND sr.status = 'completed'
) rr ON true
WHERE r.tenant_id = $1 AND r.id = $2 AND s.status = 'completed'
FOR UPDATE OF s;

-- name: ReceivableKind :one
SELECT kind FROM receivables WHERE tenant_id = $1 AND id = $2;

-- name: ReceivableLockOpening :one
-- Saldo awal tidak punya nota: baris piutangnya sendiri yang dikunci (bayar, pelunasan kolektif, dan batal berbagi kunci ini).
SELECT r.id, r.outlet_id, r.member_id, r.amount, r.doc_no, r.voided_at
FROM receivables r
WHERE r.tenant_id = $1 AND r.id = $2 AND r.kind = 'opening'
FOR UPDATE OF r;

-- name: ReceivableOpeningByIdemKey :one
SELECT id, request_hash FROM receivables WHERE tenant_id = $1 AND idempotency_key = $2;

-- name: ReceivableOpeningNextNo :one
INSERT INTO receivable_opening_counters (tenant_id, outlet_id, day, last_no) VALUES (@tenant_id, @outlet_id, @day, 1)
ON CONFLICT (tenant_id, outlet_id, day) DO UPDATE SET last_no = receivable_opening_counters.last_no + 1
RETURNING last_no;

-- name: ReceivableOpeningInsert :one
INSERT INTO receivables (tenant_id, outlet_id, member_id, amount, due_date, kind, doc_no, ref_no, doc_date, note, created_by, idempotency_key, request_hash)
VALUES (@tenant_id, @outlet_id, @member_id, @amount, sqlc.narg('due_date'), 'opening', @doc_no, @ref_no, @doc_date, @note, @created_by, @idempotency_key, @request_hash)
RETURNING id;

-- name: ReceivableOpeningRefTaken :one
SELECT EXISTS (SELECT 1 FROM receivables WHERE tenant_id = @tenant_id AND member_id = @member_id AND kind = 'opening' AND ref_no <> ''
                 AND lower(ref_no) = lower(@ref_no::text) AND voided_at IS NULL) AS taken;

-- name: ReceivableVoid :exec
UPDATE receivables SET voided_at = now(), voided_by = @voided_by, void_reason = @void_reason
WHERE tenant_id = @tenant_id AND id = @id AND kind = 'opening' AND voided_at IS NULL;

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
SELECT r.id, r.amount, r.due_date, coalesce(p.paid, 0)::numeric AS paid, coalesce(rr.returned, 0)::numeric AS returned,
       (now() AT TIME ZONE o.timezone)::date AS today
FROM receivables r
JOIN outlets o ON o.tenant_id = r.tenant_id AND o.id = r.outlet_id
LEFT JOIN LATERAL (
    SELECT sum(rp.amount) AS paid FROM receivable_payments rp
    WHERE rp.tenant_id = r.tenant_id AND rp.receivable_id = r.id
) p ON true
LEFT JOIN LATERAL (
    SELECT sum(sr.receivable_cut) AS returned FROM sales_returns sr
    WHERE sr.tenant_id = r.tenant_id AND sr.sale_id = r.sale_id AND sr.status = 'completed'
) rr ON true
WHERE r.tenant_id = $1 AND r.sale_id = $2;
