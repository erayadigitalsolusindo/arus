-- +goose Up

-- Pencarian retur pembelian memakai ILIKE '%…%' pada nomor retur (nomor nota/faktur/pemasok sudah berindeks trigram, 00039).
CREATE INDEX purchase_returns_docno_trgm_idx ON purchase_returns USING gin (doc_no gin_trgm_ops);

-- +goose Down

DROP INDEX IF EXISTS purchase_returns_docno_trgm_idx;