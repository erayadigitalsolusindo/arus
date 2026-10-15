-- +goose Up

-- Ringkasan penjualan harian (laporan per tanggal + ekspor tahan big data).
-- Laporan/export membaca ≤366 baris ringkasan (indeks penuh), bukan memindai
-- tabel sales (jutaan–ratusan juta baris). Disi agregat per (outlet, tanggal
-- lokal outlet, kasir): tanggal global akan salah untuk outlet beda zona waktu.
-- Kasir NULL (nota lama tanpa kasir) memakai sentinel all-zero agar PK terisi;
-- filter kasir spesifik tidak pernah cocok sentinel (sama seperti live query
-- yang tak pernah cocok NULL), filter "semua kasir" menjumlahkan semua baris.
-- Baris ringkasan tidak pernah dihapus (koreksi = penyesuaian bertanda);
-- DELETE/TRUNCATE dicabut dari role aplikasi seperti tabel sales.

CREATE TABLE sales_daily_summary (
    tenant_id  uuid          NOT NULL,
    outlet_id  uuid          NOT NULL,
    day        date          NOT NULL,  -- tanggal lokal outlet
    cashier_id uuid          NOT NULL DEFAULT '00000000-0000-0000-0000-000000000000',
    sale_count int           NOT NULL DEFAULT 0 CHECK (sale_count >= 0),
    void_count int           NOT NULL DEFAULT 0 CHECK (void_count >= 0),
    subtotal   numeric(18,2) NOT NULL DEFAULT 0 CHECK (subtotal >= 0),
    discount   numeric(18,2) NOT NULL DEFAULT 0 CHECK (discount >= 0),
    tax_store  numeric(18,2) NOT NULL DEFAULT 0 CHECK (tax_store >= 0),
    tax_gov    numeric(18,2) NOT NULL DEFAULT 0 CHECK (tax_gov >= 0),
    total      numeric(18,2) NOT NULL DEFAULT 0 CHECK (total >= 0),
    paid_net   numeric(18,2) NOT NULL DEFAULT 0 CHECK (paid_net >= 0),  -- Σ(paid − change) nota completed
    receivable numeric(18,2) NOT NULL DEFAULT 0 CHECK (receivable >= 0),
    void_total numeric(18,2) NOT NULL DEFAULT 0 CHECK (void_total >= 0), -- Σ total nota void (positif)
    return_count int           NOT NULL DEFAULT 0 CHECK (return_count >= 0),
    return_total numeric(18,2) NOT NULL DEFAULT 0 CHECK (return_total >= 0),
    PRIMARY KEY (tenant_id, outlet_id, day, cashier_id),
    CONSTRAINT sales_daily_summary_outlet_fk FOREIGN KEY (tenant_id, outlet_id) REFERENCES outlets (tenant_id, id) ON DELETE RESTRICT
);

-- Breakdown metode per (outlet, tanggal lokal, kasir, metode). amount_netto =
-- Σ pembayaran − kembalian nota ini untuk metode tunai (persis semantik
-- pay_hdr laporan). Nama/jenis metode dibaca dari payment_methods saat laporan
-- (ganti nama ikut nama baru, sama seperti live query).
CREATE TABLE sales_daily_method_summary (
    tenant_id  uuid          NOT NULL,
    outlet_id  uuid          NOT NULL,
    day        date          NOT NULL,
    cashier_id uuid          NOT NULL DEFAULT '00000000-0000-0000-0000-000000000000',
    method_id  uuid          NOT NULL,
    amount_net numeric(18,2) NOT NULL DEFAULT 0 CHECK (amount_net >= 0),
    fee        numeric(18,2) NOT NULL DEFAULT 0 CHECK (fee >= 0),       -- MDR ditanggung toko
    surcharge  numeric(18,2) NOT NULL DEFAULT 0 CHECK (surcharge >= 0), -- MDR ditagih ke pelanggan
    PRIMARY KEY (tenant_id, outlet_id, day, cashier_id, method_id),
    CONSTRAINT sales_daily_method_summary_outlet_fk FOREIGN KEY (tenant_id, outlet_id) REFERENCES outlets (tenant_id, id) ON DELETE RESTRICT
);

ALTER TABLE sales_daily_summary        ENABLE ROW LEVEL SECURITY;
ALTER TABLE sales_daily_method_summary ENABLE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON sales_daily_summary        USING (tenant_id = app_tenant_id()) WITH CHECK (tenant_id = app_tenant_id());
CREATE POLICY tenant_isolation ON sales_daily_method_summary USING (tenant_id = app_tenant_id()) WITH CHECK (tenant_id = app_tenant_id());

REVOKE DELETE, TRUNCATE ON sales_daily_summary, sales_daily_method_summary FROM aciraba_app;

-- Backfill dari data yang sudah ada (semantik identik live query laporan).
INSERT INTO sales_daily_summary (tenant_id, outlet_id, day, cashier_id,
    sale_count, void_count, subtotal, discount, tax_store, tax_gov, total, paid_net, receivable, void_total)
SELECT s.tenant_id, s.outlet_id, (s.created_at AT TIME ZONE o.timezone)::date AS day,
    coalesce(s.cashier_id, '00000000-0000-0000-0000-000000000000'),
    count(*) FILTER (WHERE s.status = 'completed'),
    count(*) FILTER (WHERE s.status = 'void'),
    coalesce(sum(s.subtotal) FILTER (WHERE s.status = 'completed'), 0),
    coalesce(sum(s.discount) FILTER (WHERE s.status = 'completed'), 0),
    coalesce(sum(s.tax_store) FILTER (WHERE s.status = 'completed'), 0),
    coalesce(sum(s.tax_gov) FILTER (WHERE s.status = 'completed'), 0),
    coalesce(sum(s.total) FILTER (WHERE s.status = 'completed'), 0),
    coalesce(sum(s.paid - s.change) FILTER (WHERE s.status = 'completed'), 0),
    coalesce(sum(s.receivable) FILTER (WHERE s.status = 'completed'), 0),
    coalesce(sum(s.total) FILTER (WHERE s.status = 'void'), 0)
FROM sales s
JOIN outlets o ON o.tenant_id = s.tenant_id AND o.id = s.outlet_id
WHERE s.status IN ('completed', 'void')
GROUP BY s.tenant_id, s.outlet_id, (s.created_at AT TIME ZONE o.timezone)::date,
    coalesce(s.cashier_id, '00000000-0000-0000-0000-000000000000');

INSERT INTO sales_daily_method_summary (tenant_id, outlet_id, day, cashier_id, method_id, amount_net, fee, surcharge)
SELECT tenant_id, outlet_id, day, cashier_id, method_id,
    sum(amt), sum(fee), sum(surcharge)
FROM (
    SELECT s.tenant_id, s.outlet_id, (s.created_at AT TIME ZONE o.timezone)::date AS day,
        coalesce(s.cashier_id, '00000000-0000-0000-0000-000000000000') AS cashier_id,
        p.method_id,
        sum(p.amount) - CASE WHEN pm.kind = 'cash' THEN s.change ELSE 0 END AS amt,
        coalesce(sum(p.fee_amount) FILTER (WHERE p.fee_bearer = 'store'), 0) AS fee,
        coalesce(sum(p.fee_amount) FILTER (WHERE p.fee_bearer = 'customer'), 0) AS surcharge
    FROM sales s
    JOIN outlets o ON o.tenant_id = s.tenant_id AND o.id = s.outlet_id
    JOIN sale_payments p ON p.tenant_id = s.tenant_id AND p.sale_id = s.id
    JOIN payment_methods pm ON pm.tenant_id = s.tenant_id AND pm.id = p.method_id
    WHERE s.status = 'completed'
    GROUP BY (s.created_at AT TIME ZONE o.timezone)::date, s.id, s.change, p.method_id, pm.kind
) per_sale_method
GROUP BY tenant_id, outlet_id, day, cashier_id, method_id;

-- Retur aktif per (outlet retur, tanggal lokal retur, kasir nota asal).
INSERT INTO sales_daily_summary (tenant_id, outlet_id, day, cashier_id, return_count, return_total)
SELECT r.tenant_id, r.outlet_id, (r.created_at AT TIME ZONE o.timezone)::date AS day,
    coalesce(s.cashier_id, '00000000-0000-0000-0000-000000000000'),
    count(*), coalesce(sum(r.total), 0)
FROM sales_returns r
JOIN outlets o ON o.tenant_id = r.tenant_id AND o.id = r.outlet_id
JOIN sales s ON s.tenant_id = r.tenant_id AND s.id = r.sale_id
WHERE r.status = 'completed'
GROUP BY r.tenant_id, r.outlet_id, (r.created_at AT TIME ZONE o.timezone)::date,
    coalesce(s.cashier_id, '00000000-0000-0000-0000-000000000000')
ON CONFLICT (tenant_id, outlet_id, day, cashier_id) DO UPDATE SET
    return_count = sales_daily_summary.return_count + EXCLUDED.return_count,
    return_total = sales_daily_summary.return_total + EXCLUDED.return_total;

-- +goose Down
DROP TABLE sales_daily_method_summary;
DROP TABLE sales_daily_summary;
