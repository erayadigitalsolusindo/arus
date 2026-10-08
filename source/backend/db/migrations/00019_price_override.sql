-- +goose Up

-- Ubah harga jual di kasir (FR-POS-04): hanya dengan persetujuan Owner/Supervisor (izin `price_override.approve`)
-- yang memasukkan PIN-nya. Baris menyimpan harga daftar (hasil hitung server) di samping harga yang berlaku, dan
-- nota menyimpan siapa yang menyetujui (boleh pelaku yang sama bila ia sendiri pemegang izin, mis. Owner).
ALTER TABLE sale_lines ADD COLUMN list_price numeric(18,2) CHECK (list_price >= 0);
UPDATE sale_lines SET list_price = unit_price;
ALTER TABLE sale_lines ALTER COLUMN list_price SET NOT NULL;
ALTER TABLE sale_lines ADD COLUMN price_override boolean NOT NULL DEFAULT false;

ALTER TABLE sales ADD COLUMN approved_by uuid;
ALTER TABLE sales ADD CONSTRAINT sales_approver_fk FOREIGN KEY (tenant_id, approved_by) REFERENCES users (tenant_id, id) ON DELETE RESTRICT;

-- +goose Down
ALTER TABLE sales DROP CONSTRAINT sales_approver_fk;
ALTER TABLE sales DROP COLUMN approved_by;
ALTER TABLE sale_lines DROP COLUMN price_override, DROP COLUMN list_price;
