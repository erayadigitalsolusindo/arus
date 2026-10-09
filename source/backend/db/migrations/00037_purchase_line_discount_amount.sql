-- +goose Up

-- Diskon baris pembelian boleh persen ATAU rupiah: nilai di bawah 100 = persen, 100 ke atas = nominal rupiah yang
-- dipotong dari nilai baris pada tingkat itu (aturan ada di service Go; kolom hanya menyimpan angka yang diisi).
ALTER TABLE purchase_lines
    DROP CONSTRAINT purchase_lines_disc1_check,
    DROP CONSTRAINT purchase_lines_disc2_check,
    DROP CONSTRAINT purchase_lines_disc3_check,
    DROP CONSTRAINT purchase_lines_disc4_check;
ALTER TABLE purchase_lines
    ALTER COLUMN disc1 TYPE numeric(18,2),
    ALTER COLUMN disc2 TYPE numeric(18,2),
    ALTER COLUMN disc3 TYPE numeric(18,2),
    ALTER COLUMN disc4 TYPE numeric(18,2),
    ADD CONSTRAINT purchase_lines_disc1_check CHECK (disc1 >= 0),
    ADD CONSTRAINT purchase_lines_disc2_check CHECK (disc2 >= 0),
    ADD CONSTRAINT purchase_lines_disc3_check CHECK (disc3 >= 0),
    ADD CONSTRAINT purchase_lines_disc4_check CHECK (disc4 >= 0);

-- +goose Down
ALTER TABLE purchase_lines
    DROP CONSTRAINT purchase_lines_disc1_check,
    DROP CONSTRAINT purchase_lines_disc2_check,
    DROP CONSTRAINT purchase_lines_disc3_check,
    DROP CONSTRAINT purchase_lines_disc4_check;
ALTER TABLE purchase_lines
    ALTER COLUMN disc1 TYPE numeric(5,2),
    ALTER COLUMN disc2 TYPE numeric(5,2),
    ALTER COLUMN disc3 TYPE numeric(5,2),
    ALTER COLUMN disc4 TYPE numeric(5,2),
    ADD CONSTRAINT purchase_lines_disc1_check CHECK (disc1 >= 0 AND disc1 < 100),
    ADD CONSTRAINT purchase_lines_disc2_check CHECK (disc2 >= 0 AND disc2 < 100),
    ADD CONSTRAINT purchase_lines_disc3_check CHECK (disc3 >= 0 AND disc3 < 100),
    ADD CONSTRAINT purchase_lines_disc4_check CHECK (disc4 >= 0 AND disc4 < 100);
