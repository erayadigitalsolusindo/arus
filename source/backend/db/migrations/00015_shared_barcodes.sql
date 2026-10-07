-- +goose Up

-- Barcode boleh SAMA antar barang (keputusan produk 2026-10-08): barcode pabrik tidak dijamin unik — barang A dari
-- negara CC dan barang B dari negara DD bisa sama-sama berkode 1111. Kasir yang memindai kode kembar memilih barang
-- dari daftar hasil (nama + pembeda). Peringatan "barcode ini juga dipakai barang lain" ditangani form, bukan DB.
-- Pencarian per barcode tetap cepat lewat indeks biasa (bukan unik).
DROP INDEX items_tenant_barcode_key;
DROP INDEX item_units_tenant_barcode_key;
CREATE INDEX items_tenant_barcode_idx      ON items      (tenant_id, barcode) WHERE barcode IS NOT NULL;
CREATE INDEX item_units_tenant_barcode_idx ON item_units (tenant_id, barcode) WHERE barcode IS NOT NULL;

-- Pembeda opsional (mis. negara asal) yang tampil di pilihan kasir untuk barang bernama sama. Bebas isi; bukan kunci.
ALTER TABLE items ADD COLUMN origin text NOT NULL DEFAULT '' CHECK (char_length(origin) <= 100);

-- +goose Down
ALTER TABLE items DROP COLUMN origin;
DROP INDEX item_units_tenant_barcode_idx;
DROP INDEX items_tenant_barcode_idx;
-- Indeks unik lama tidak dibuat ulang otomatis: data kembar yang sudah masuk akan membuatnya gagal.
