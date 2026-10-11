-- +goose Up

-- Jurnal otomatis yang dibuat penjadwal malam tidak punya pembuat manusia: created_by boleh kosong, tetapi hanya untuk jurnal
-- bersumber transaksi (source_type terisi). Jurnal manual tetap wajib punya pembuat.
ALTER TABLE journal_entries ALTER COLUMN created_by DROP NOT NULL;
ALTER TABLE journal_entries ADD CONSTRAINT journal_entries_creator_chk CHECK (created_by IS NOT NULL OR source_type IS NOT NULL);

-- Daftar sasaran penjadwal lintas tenant (outlet aktif milik tenant aktif yang sudah menyiapkan pemetaan akun). Tabel tenant dilindungi
-- RLS sehingga role aplikasi tidak bisa mendaftar semua tenant; fungsi ini membuka HANYA kolom yang dibutuhkan.
-- +goose StatementBegin
CREATE FUNCTION auto_journal_targets()
RETURNS TABLE (tenant_id uuid, outlet_id uuid, timezone text)
LANGUAGE sql STABLE SECURITY DEFINER SET search_path = public, pg_temp AS
$$
    SELECT o.tenant_id, o.id, o.timezone
    FROM outlets o JOIN tenants t ON t.id = o.tenant_id
    WHERE o.active AND t.active
      AND EXISTS (SELECT 1 FROM account_mappings m WHERE m.tenant_id = o.tenant_id)
    ORDER BY o.tenant_id, o.code
$$;
-- +goose StatementEnd
REVOKE ALL ON FUNCTION auto_journal_targets() FROM PUBLIC;
GRANT EXECUTE ON FUNCTION auto_journal_targets() TO aciraba_app;

-- +goose Down
DROP FUNCTION auto_journal_targets();
ALTER TABLE journal_entries DROP CONSTRAINT journal_entries_creator_chk;
ALTER TABLE journal_entries ALTER COLUMN created_by SET NOT NULL;
