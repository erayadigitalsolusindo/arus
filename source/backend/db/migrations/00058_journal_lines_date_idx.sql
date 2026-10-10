-- +goose Up

-- Laporan keuangan (neraca saldo, laba rugi, neraca, jurnal umum): saldo bulan berjalan dihitung dari baris jurnal pada rentang
-- tanggal lintas akun; tanpa indeks ini harus memindai seluruh journal_lines tenant.
CREATE INDEX journal_lines_date_idx ON journal_lines (tenant_id, entry_date);

-- +goose Down
DROP INDEX IF EXISTS journal_lines_date_idx;
