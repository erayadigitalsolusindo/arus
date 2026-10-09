-- +goose Up

-- Opname langsung: dokumen yang langsung berstatus 'completed' tanpa draf. Dua mode input:
--   replace = hasil hitung menggantikan stok (stok jadi sebesar yang dimasukkan),
--   adjust  = jumlah dimasukkan sebagai tambah/kurang (+/−) terhadap stok.
-- Baris dokumen tetap memakai snapshot_qty (stok sebelum), counted_qty (stok sesudah), diff.
ALTER TABLE stock_counts
    ADD COLUMN kind text NOT NULL DEFAULT 'session' CHECK (kind IN ('session', 'quick')),
    ADD COLUMN adjust_mode text CHECK (adjust_mode IN ('replace', 'adjust')),
    ADD CONSTRAINT stock_counts_kind_chk CHECK ((kind = 'quick') = (adjust_mode IS NOT NULL));

-- +goose Down
ALTER TABLE stock_counts DROP CONSTRAINT stock_counts_kind_chk, DROP COLUMN adjust_mode, DROP COLUMN kind;
