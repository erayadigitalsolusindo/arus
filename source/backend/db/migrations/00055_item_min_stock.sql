-- +goose Up
-- Batas stok minimum per barang (satuan dasar). 0 = tidak dipantau. Dasbor memberi peringatan "stok menipis" bila
-- saldo total cabang (semua bucket) > 0 dan <= batas ini. Nilai global per barang (belum per cabang).
ALTER TABLE items ADD COLUMN min_stock numeric(18,3) NOT NULL DEFAULT 0 CHECK (min_stock >= 0);

-- +goose Down
ALTER TABLE items DROP COLUMN min_stock;
