-- +goose Up

-- Akun sistem: dirujuk otomatis oleh aplikasi (kas, piutang, persediaan, PPN, ekuitas, penjualan, HPP); service menolak hapus,
-- nonaktifkan, dan ubah kode/kelas/induk. Daftar kode sama dengan systemCodes di internal/accounting/template.go.
ALTER TABLE accounts ADD COLUMN is_system boolean NOT NULL DEFAULT false;
UPDATE accounts SET is_system = true WHERE code IN ('1000','1100','1110','1120','1130','1140','1210','1310','1330',
    '2000','2100','2110','2130','2140','3000','3100','3200','4000','4100','5000','5100','5200','6000');

-- +goose Down
ALTER TABLE accounts DROP COLUMN is_system;
