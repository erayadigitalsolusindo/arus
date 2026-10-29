-- +goose Up

-- Gateway yang memproses tagihan (kode dari EraPay: "doku" | "ids"), dipakai untuk label di detail nota. Kosong = tagihan lama
-- sebelum kolom ini ada (label jatuh ke "ID EraPay" saja).
ALTER TABLE erapay_charges ADD COLUMN gateway text;

-- +goose Down
ALTER TABLE erapay_charges DROP COLUMN gateway;
