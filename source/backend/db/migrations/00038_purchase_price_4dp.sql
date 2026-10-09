-- +goose Up

-- Harga beli per satuan boleh sampai 4 desimal supaya "sub total ÷ qty" (mis. 1.000 ÷ 3) bisa disimpan tepat dan
-- qty × harga kembali ke angka yang diketik pengguna. Nilai baris dan HPP tetap dibulatkan 2 desimal.
ALTER TABLE purchase_lines ALTER COLUMN unit_price TYPE numeric(18,4);

-- +goose Down
ALTER TABLE purchase_lines ALTER COLUMN unit_price TYPE numeric(18,2);
