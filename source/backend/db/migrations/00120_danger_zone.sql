-- +goose Up
-- +goose StatementBegin

-- Danger zone (Owner): membersihkan transaksi satu tenant. Tabel transaksi bersifat append-only bagi aplikasi (tanpa hak DELETE,
-- jurnal terposting dilindungi RLS), jadi penghapusan hanya lewat fungsi SECURITY DEFINER ini (pemilik skema melewati RLS).
-- Fungsi hanya mau menghapus tenant dari sesi yang sedang berjalan (app_tenant_id); pemeriksaan Owner + kata sandi ada di API.
-- Agar data tetap sinkron, mutasi stok akibat transaksi, ledger deposit/poin member, ledger kredit pemasok, dan shift kasir ikut
-- dihapus. Master data (barang, member, pemasok, akun, dll.) TIDAK disentuh.

-- Hapus semua baris tenant dari satu tabel. FK self-referensi (nota revisi saling menunjuk) dilepas sementara lalu dipasang
-- ulang dan divalidasi, sama seperti tenant_purge_core.
CREATE FUNCTION danger_wipe(p_tenant uuid, p_table regclass) RETURNS bigint
LANGUAGE plpgsql SECURITY DEFINER SET search_path = public, pg_temp AS
$$
DECLARE
    v_fks text[];
    v_fk  text;
    v_n   bigint;
BEGIN
    SELECT array_agg(k.conname || E'\t' || pg_get_constraintdef(k.oid)) INTO v_fks
      FROM pg_constraint k WHERE k.contype = 'f' AND k.conrelid = p_table AND k.confrelid = p_table;
    FOREACH v_fk IN ARRAY COALESCE(v_fks, '{}') LOOP
        EXECUTE format('ALTER TABLE %s DROP CONSTRAINT %I', p_table, split_part(v_fk, E'\t', 1));
    END LOOP;
    EXECUTE format('DELETE FROM %s WHERE tenant_id = $1', p_table) USING p_tenant;
    GET DIAGNOSTICS v_n = ROW_COUNT;
    FOREACH v_fk IN ARRAY COALESCE(v_fks, '{}') LOOP
        EXECUTE format('ALTER TABLE %s ADD CONSTRAINT %I %s NOT VALID', p_table, split_part(v_fk, E'\t', 1), split_part(v_fk, E'\t', 2));
        EXECUTE format('ALTER TABLE %s VALIDATE CONSTRAINT %I', p_table, split_part(v_fk, E'\t', 1));
    END LOOP;
    RETURN v_n;
END
$$;

-- Hapus jurnal bersumber transaksi (source_type) beserta barisnya, jurnal balik yang menunjuknya, dan kecocokan rekonsiliasi bank.
-- Saldo agregat per bulan (account_period_balances) dikurangi sebesar baris jurnal terposting yang dihapus.
CREATE FUNCTION danger_journals(p_tenant uuid, p_sources text[]) RETURNS bigint
LANGUAGE plpgsql SECURITY DEFINER SET search_path = public, pg_temp AS
$$
DECLARE
    v_ids uuid[];
    v_n   bigint;
BEGIN
    SELECT array_agg(e.id) INTO v_ids FROM journal_entries e
     WHERE e.tenant_id = p_tenant AND (e.source_type = ANY (p_sources)
        OR e.reverses_id IN (SELECT x.id FROM journal_entries x WHERE x.tenant_id = p_tenant AND x.source_type = ANY (p_sources)));
    IF v_ids IS NULL THEN
        RETURN 0;
    END IF;
    UPDATE account_period_balances b SET debit = b.debit - s.d, credit = b.credit - s.c
      FROM (SELECT e.outlet_id, l.account_id, date_trunc('month', e.entry_date)::date AS m, sum(l.debit) AS d, sum(l.credit) AS c
              FROM journal_lines l JOIN journal_entries e ON e.tenant_id = l.tenant_id AND e.id = l.entry_id
             WHERE e.tenant_id = p_tenant AND e.id = ANY (v_ids) AND e.status = 'posted'
             GROUP BY 1, 2, 3) s
     WHERE b.tenant_id = p_tenant AND b.outlet_id = s.outlet_id AND b.account_id = s.account_id AND b.period_month = s.m;
    DELETE FROM bank_reconciliation_matches WHERE journal_line_id IN
        (SELECT l.id FROM journal_lines l WHERE l.tenant_id = p_tenant AND l.entry_id = ANY (v_ids));
    UPDATE journal_entries SET reverses_id = NULL WHERE tenant_id = p_tenant AND id = ANY (v_ids) AND reverses_id IS NOT NULL;
    DELETE FROM journal_entries WHERE tenant_id = p_tenant AND id = ANY (v_ids); -- baris jurnal ikut (ON DELETE CASCADE)
    GET DIAGNOSTICS v_n = ROW_COUNT;
    RETURN v_n;
END
$$;

-- Hapus mutasi stok berdasarkan jenis sumbernya, lalu hitung ulang rantai saldo (balance_after) dan saldo bucket dari mutasi yang tersisa.
CREATE FUNCTION danger_restock(p_tenant uuid, p_ref_types text[]) RETURNS bigint
LANGUAGE plpgsql SECURITY DEFINER SET search_path = public, pg_temp AS
$$
DECLARE
    n bigint;
BEGIN
    DELETE FROM stock_movements WHERE tenant_id = p_tenant AND ref_type = ANY (p_ref_types);
    GET DIAGNOSTICS n = ROW_COUNT;
    UPDATE stock_movements m SET balance_after = r.run
      FROM (SELECT id, sum(qty_delta) OVER (PARTITION BY outlet_id, item_id, bucket ORDER BY id) AS run
              FROM stock_movements WHERE tenant_id = p_tenant) r
     WHERE m.tenant_id = p_tenant AND m.id = r.id AND m.balance_after <> r.run;
    UPDATE stock_balances b SET qty = COALESCE((SELECT sum(m.qty_delta) FROM stock_movements m
        WHERE m.tenant_id = p_tenant AND m.outlet_id = b.outlet_id AND m.item_id = b.item_id AND m.bucket = b.bucket), 0), updated_at = now()
     WHERE b.tenant_id = p_tenant;
    RETURN n;
END
$$;

CREATE FUNCTION danger_guard(p_tenant uuid) RETURNS void
LANGUAGE plpgsql STABLE SECURITY DEFINER SET search_path = public, pg_temp AS
$$
BEGIN
    IF p_tenant IS NULL OR p_tenant IS DISTINCT FROM app_tenant_id() THEN
        RAISE EXCEPTION 'tenant tidak sesuai sesi' USING ERRCODE = '42501';
    END IF;
END
$$;

-- 1. Penjualan: nota + retur + piutang yang lahir dari nota + pembayarannya, tagihan EraPay, ringkasan harian, jurnal otomatisnya.
CREATE FUNCTION danger_clear_sales(p_tenant uuid) RETURNS jsonb
LANGUAGE plpgsql SECURITY DEFINER SET search_path = public, pg_temp AS
$$
DECLARE
    v jsonb := '{}';
    n bigint;
BEGIN
    PERFORM danger_guard(p_tenant);
    DELETE FROM sales_return_lines WHERE tenant_id = p_tenant;                 GET DIAGNOSTICS n = ROW_COUNT; v := v || jsonb_build_object('sales_return_lines', n);
    DELETE FROM sales_returns WHERE tenant_id = p_tenant;                      GET DIAGNOSTICS n = ROW_COUNT; v := v || jsonb_build_object('sales_returns', n);
    DELETE FROM receivable_payments WHERE tenant_id = p_tenant AND receivable_id IN
        (SELECT id FROM receivables WHERE tenant_id = p_tenant AND sale_id IS NOT NULL);
    DELETE FROM receivable_settlements s WHERE s.tenant_id = p_tenant AND NOT EXISTS
        (SELECT 1 FROM receivable_payments p WHERE p.tenant_id = p_tenant AND p.settlement_id = s.id);
    DELETE FROM receivables WHERE tenant_id = p_tenant AND sale_id IS NOT NULL; GET DIAGNOSTICS n = ROW_COUNT; v := v || jsonb_build_object('sales_receivables', n);
    DELETE FROM sale_vouchers WHERE tenant_id = p_tenant;
    DELETE FROM sale_costs WHERE tenant_id = p_tenant;
    DELETE FROM sale_payments WHERE tenant_id = p_tenant;
    DELETE FROM sale_lines WHERE tenant_id = p_tenant;
    v := v || jsonb_build_object('sales', danger_wipe(p_tenant, 'sales'));
    DELETE FROM erapay_charges WHERE tenant_id = p_tenant;
    DELETE FROM sales_daily_method_summary WHERE tenant_id = p_tenant;
    DELETE FROM sales_daily_summary WHERE tenant_id = p_tenant;
    UPDATE vouchers SET used_count = 0 WHERE tenant_id = p_tenant AND used_count <> 0;
    v := v || jsonb_build_object('stock_movements', danger_restock(p_tenant, ARRAY['SALE', 'SALE_VOID', 'SALE_RETURN']));
    -- Poin & deposit member dan shift kasir bersumber dari nota: ledger dikosongkan, saldo kembali nol.
    DELETE FROM member_point_movements WHERE tenant_id = p_tenant;             GET DIAGNOSTICS n = ROW_COUNT; v := v || jsonb_build_object('member_point_movements', n);
    UPDATE members SET points = 0, lifetime_points = 0 WHERE tenant_id = p_tenant AND (points <> 0 OR lifetime_points <> 0);
    DELETE FROM member_deposit_movements WHERE tenant_id = p_tenant;           GET DIAGNOSTICS n = ROW_COUNT; v := v || jsonb_build_object('member_deposit_movements', n);
    DELETE FROM cash_movements WHERE tenant_id = p_tenant;
    DELETE FROM cash_shift_flows WHERE tenant_id = p_tenant;
    DELETE FROM cash_shift_counts WHERE tenant_id = p_tenant;
    DELETE FROM cash_shifts WHERE tenant_id = p_tenant;                        GET DIAGNOSTICS n = ROW_COUNT; v := v || jsonb_build_object('cash_shifts', n);
    v := v || jsonb_build_object('sales_journals', danger_journals(p_tenant, ARRAY['sales_daily', 'sales_returns_daily', 'receivable_payments_daily']));
    RETURN v;
END
$$;

-- 2. Hutang, piutang, dan pembelian: nota beli + retur beli + semua hutang/piutang + pembayaran & pelunasan kolektifnya, jurnalnya.
CREATE FUNCTION danger_clear_purchasing(p_tenant uuid) RETURNS jsonb
LANGUAGE plpgsql SECURITY DEFINER SET search_path = public, pg_temp AS
$$
DECLARE
    v jsonb := '{}';
    n bigint;
BEGIN
    PERFORM danger_guard(p_tenant);
    DELETE FROM purchase_return_lines WHERE tenant_id = p_tenant;              GET DIAGNOSTICS n = ROW_COUNT; v := v || jsonb_build_object('purchase_return_lines', n);
    DELETE FROM purchase_returns WHERE tenant_id = p_tenant;                   GET DIAGNOSTICS n = ROW_COUNT; v := v || jsonb_build_object('purchase_returns', n);
    DELETE FROM payable_payments WHERE tenant_id = p_tenant;                   GET DIAGNOSTICS n = ROW_COUNT; v := v || jsonb_build_object('payable_payments', n);
    DELETE FROM payable_settlements WHERE tenant_id = p_tenant;
    DELETE FROM payables WHERE tenant_id = p_tenant;                           GET DIAGNOSTICS n = ROW_COUNT; v := v || jsonb_build_object('payables', n);
    DELETE FROM receivable_payments WHERE tenant_id = p_tenant;                GET DIAGNOSTICS n = ROW_COUNT; v := v || jsonb_build_object('receivable_payments', n);
    DELETE FROM receivable_settlements WHERE tenant_id = p_tenant;
    DELETE FROM receivables WHERE tenant_id = p_tenant;                        GET DIAGNOSTICS n = ROW_COUNT; v := v || jsonb_build_object('receivables', n);
    DELETE FROM purchase_costs WHERE tenant_id = p_tenant;
    DELETE FROM purchase_lines WHERE tenant_id = p_tenant;
    v := v || jsonb_build_object('purchases', danger_wipe(p_tenant, 'purchases'));
    v := v || jsonb_build_object('stock_movements_purchasing', danger_restock(p_tenant, ARRAY['PURCHASE', 'PURCHASE_RETURN']));
    DELETE FROM supplier_credit_movements WHERE tenant_id = p_tenant;          GET DIAGNOSTICS n = ROW_COUNT; v := v || jsonb_build_object('supplier_credit_movements', n);
    v := v || jsonb_build_object('purchasing_journals', danger_journals(p_tenant,
        ARRAY['purchases_daily', 'purchase_returns_daily', 'receivable_payments_daily', 'payable_payments_daily']));
    RETURN v;
END
$$;

-- 3. Reset metode pembayaran: semua transaksi di atas dihapus (nota menempel pada metode), metode non-bawaan dihapus, metode
-- bawaan (Tunai, Deposit Member, Kredit Pemasok) dikembalikan ke nama/tarif awal. Dokumen yang memakai metode (nota, pelunasan,
-- ledger deposit & kredit, shift) sudah ikut terhapus oleh kedua fungsi di atas. Tautan EraPay dilepas (dibuat ulang otomatis
-- oleh API bila key EraPay masih terpasang).
CREATE FUNCTION danger_reset_payment_methods(p_tenant uuid) RETURNS jsonb
LANGUAGE plpgsql SECURITY DEFINER SET search_path = public, pg_temp AS
$$
DECLARE
    v      jsonb;
    v_cash uuid;
    n      bigint;
BEGIN
    PERFORM danger_guard(p_tenant);
    SELECT id INTO v_cash FROM payment_methods WHERE tenant_id = p_tenant AND kind = 'cash';
    IF v_cash IS NULL THEN
        RAISE EXCEPTION 'metode tunai tidak ditemukan' USING ERRCODE = 'P0001';
    END IF;
    v := danger_clear_sales(p_tenant) || danger_clear_purchasing(p_tenant);
    DELETE FROM erapay_methods WHERE tenant_id = p_tenant;
    DELETE FROM payment_methods WHERE tenant_id = p_tenant AND NOT is_system; GET DIAGNOSTICS n = ROW_COUNT;
    UPDATE payment_methods SET name = CASE kind WHEN 'cash' THEN 'Tunai' WHEN 'deposit' THEN 'Deposit Member' ELSE 'Kredit Pemasok' END,
           fee_pct = 0, fee_flat = 0, fee_bearer = 'store', small_tx_limit = 0, small_tx_flat = 0, active = true
     WHERE tenant_id = p_tenant AND is_system;
    RETURN v || jsonb_build_object('payment_methods_deleted', n);
END
$$;

REVOKE ALL ON FUNCTION danger_wipe(uuid, regclass), danger_journals(uuid, text[]), danger_restock(uuid, text[]), danger_guard(uuid),
    danger_clear_sales(uuid), danger_clear_purchasing(uuid), danger_reset_payment_methods(uuid) FROM PUBLIC;
-- Fungsi bantu sengaja tanpa GRANT: hanya dipanggil ketiga fungsi di bawah.
GRANT EXECUTE ON FUNCTION danger_clear_sales(uuid), danger_clear_purchasing(uuid), danger_reset_payment_methods(uuid) TO aciraba_app;

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP FUNCTION danger_reset_payment_methods(uuid);
DROP FUNCTION danger_clear_purchasing(uuid);
DROP FUNCTION danger_clear_sales(uuid);
DROP FUNCTION danger_guard(uuid);
DROP FUNCTION danger_restock(uuid, text[]);
DROP FUNCTION danger_journals(uuid, text[]);
DROP FUNCTION danger_wipe(uuid, regclass);
-- +goose StatementEnd
