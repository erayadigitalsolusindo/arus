-- +goose Up

-- Force-save nota QRIS EraPay saat penyedia tidak dapat dihubungi (DOKU down → webhook tidak jalan). Nota tetap tersimpan dan
-- dapat dicetak, tetapi ditandai `erapay_pending = true` ("perlu dicek di EraPay"). Penanda ini mesti disetujui PIN penyetuju
-- berizin `qris_force.approve`, dan semua pemakaian PIN dicatat di audit log. Penjadwal rekonsiliasi (Go) memeriksa charge yang
-- ter-link ke nota-nota seperti ini: bila EraPay menyatakan lunas → flag dihapus; bila menyatakan gagal/batal/kedaluwarsa →
-- nota di-VOID otomatis dan stok dikembalikan.
ALTER TABLE sales ADD COLUMN erapay_pending boolean NOT NULL DEFAULT false;

-- Penjadwal rekonsiliasi mencari charge yang perlu dicek: ter-link ke nota, berstatus belum-akhir (pending/unknown), dan nota
-- masih aktif (completed, belum batalkan/supersede) serta berflag erapay_pending.
CREATE INDEX sales_erapay_pending_idx ON sales (tenant_id, id) WHERE erapay_pending;

-- +goose StatementBegin
CREATE FUNCTION erapay_reconcile_targets()
RETURNS TABLE (tenant_id uuid, outlet_id uuid, sale_id uuid, charge_id uuid, payment_id text, doc_no text, amount numeric)
LANGUAGE sql STABLE SECURITY DEFINER SET search_path = public, pg_temp AS
$$
    SELECT s.tenant_id, s.outlet_id, c.sale_id, c.id, c.erapay_payment_id, s.doc_no, c.amount
    FROM erapay_charges c
    JOIN sales s ON s.tenant_id = c.tenant_id AND s.id = c.sale_id
    WHERE c.sale_id IS NOT NULL
      AND c.erapay_payment_id IS NOT NULL
      AND c.status IN ('pending', 'unknown')
      AND s.erapay_pending
      AND s.status = 'completed'
      AND s.superseded_by IS NULL
    ORDER BY c.created_at, c.id
$$;
-- +goose StatementEnd
REVOKE ALL ON FUNCTION erapay_reconcile_targets() FROM PUBLIC;
GRANT EXECUTE ON FUNCTION erapay_reconcile_targets() TO aciraba_app;

-- +goose Down
DROP FUNCTION erapay_reconcile_targets();
DROP INDEX sales_erapay_pending_idx;
ALTER TABLE sales DROP COLUMN erapay_pending;
