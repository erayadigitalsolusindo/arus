-- +goose Up

-- Pencarian nota pembelian memakai ILIKE '%…%' pada nomor dokumen, nomor faktur pemasok, dan nama pemasok.
-- Indeks trigram membuat pola itu tetap cepat saat data membesar (ratusan ribu nota per tahun).
-- pg_trgm adalah extension "trusted" (dapat dibuat pemilik database tanpa superuser).
CREATE EXTENSION IF NOT EXISTS pg_trgm;

CREATE INDEX purchases_docno_trgm_idx   ON purchases USING gin (doc_no gin_trgm_ops);
CREATE INDEX purchases_invoice_trgm_idx ON purchases USING gin (supplier_invoice_no gin_trgm_ops);
CREATE INDEX suppliers_name_trgm_idx    ON suppliers USING gin (name gin_trgm_ops);

-- +goose Down

DROP INDEX IF EXISTS suppliers_name_trgm_idx;
DROP INDEX IF EXISTS purchases_invoice_trgm_idx;
DROP INDEX IF EXISTS purchases_docno_trgm_idx;
