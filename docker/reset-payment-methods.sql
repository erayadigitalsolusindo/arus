-- Reset metode pembayaran semua tenant: hanya Tunai, Deposit Member, Kredit Pemasok (+ EraPay bila terhubung).
-- Metode lain dihapus; yang sudah dipakai transaksi (FK) tidak bisa dihapus, jadi diarsipkan (active = false).
-- "EraPay QRIS" diganti nama menjadi "EraPay". Idempotent. Jalankan sebagai superuser (melewati RLS):
--   docker exec -i eds_postgres psql -U $DB_USER -d $DB_NAME < docker/reset-payment-methods.sql
BEGIN;

UPDATE payment_methods p SET name = 'EraPay'
WHERE p.name = 'EraPay QRIS'
  AND NOT EXISTS (SELECT 1 FROM payment_methods o WHERE o.tenant_id = p.tenant_id AND lower(o.name) = 'erapay');

DO $$
DECLARE r record;
BEGIN
  FOR r IN SELECT id FROM payment_methods p
           WHERE p.kind NOT IN ('cash', 'deposit', 'supplier_credit')
             AND NOT EXISTS (SELECT 1 FROM erapay_methods e WHERE e.method_id = p.id)
  LOOP
    BEGIN
      DELETE FROM payment_methods WHERE id = r.id;
    EXCEPTION WHEN foreign_key_violation THEN
      UPDATE payment_methods SET active = false WHERE id = r.id;
    END;
  END LOOP;
END $$;

SELECT t.name AS tenant, p.name, p.kind, p.active FROM payment_methods p JOIN tenants t ON t.id = p.tenant_id ORDER BY 1, 2;
COMMIT;
