-- +goose Up

-- Isi awal hak akses menu per paket (lihat 00083):
--   PRO  (Rp20.000): POS, penjualan, persediaan, EraPay/QRIS, member, dan sistem dasar.
--   FULL (Rp399.000): semua menu (modules NULL) dan tanpa batas transaksi/pengguna.
-- PRO hanya diisi bila belum diatur (NULL, atau daftar kosong yang mengunci semua menu), agar pengaturan yang sudah diubah Platform Admin tidak tertimpa.
-- Paket bersifat allowlist: menu baru yang ditambahkan kelak terkunci di PRO sampai dicentang di /platform/plans.
UPDATE plans SET
    description = 'POS, penjualan, persediaan, dan QRIS EraPay: 300 transaksi per hari, 4 pengguna.',
    modules = ARRAY[
        -- POS
        'sales_orders', 'sales_returns', 'cash_shifts', 'pos_only',
        -- PIN penyetuju di kasir
        'price_override', 'outlet_switch', 'sale_edit', 'credit_limit', 'shift_close',
        -- EraPay / QRIS
        'payment_methods', 'qris_force',
        -- Penjualan
        'sales_list', 'sales_cost', 'sell_price_history', 'sales_reports', 'sales_reports_cost',
        -- Persediaan
        'items', 'stock_card', 'stock_opening', 'stock_conversion', 'stock_opname', 'stock_transfer', 'stock_reports',
        -- Master pendukung
        'units', 'categories', 'brands', 'principals', 'coupons',
        -- Member (poin, deposit, piutang)
        'members', 'member_categories', 'member_receivables', 'receivable_opening', 'member_deposits',
        -- Sistem
        'users', 'roles', 'outlets', 'subscription'
    ]
WHERE code = 'pro' AND (modules IS NULL OR cardinality(modules) = 0);

INSERT INTO plans (code, name, description, price, duration_days, max_tx_per_day, max_users, is_default, self_service, active, sort_order, modules)
VALUES ('full', 'FULL', 'Semua fitur: akuntansi SIAK, pembelian, laporan lengkap, HalloArus. Transaksi dan pengguna tanpa batas.',
        399000, 30, NULL, NULL, false, true, true, 3, NULL)
ON CONFLICT (code) DO NOTHING;

-- +goose Down
DELETE FROM plans WHERE code = 'full'
  AND NOT EXISTS (SELECT 1 FROM tenant_subscriptions s WHERE s.plan_id = plans.id)
  AND NOT EXISTS (SELECT 1 FROM subscription_payments p WHERE p.plan_id = plans.id);
UPDATE plans SET modules = NULL, description = '300 transaksi per hari, 4 pengguna.' WHERE code = 'pro';
