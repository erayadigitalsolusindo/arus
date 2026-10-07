-- +goose Up

-- Saldo awal stok (Fase 4.2, FR-ONB-07): per outlet, saldo awal (movement OPENING) hanya boleh diinput/diubah sampai
-- "tanggal mulai operasional" dikunci. NULL = belum dikunci. Setelah dikunci, perubahan stok hanya lewat opname.
ALTER TABLE outlets
    ADD COLUMN stock_locked_at timestamptz,
    ADD COLUMN ops_start_date  date,
    ADD CONSTRAINT outlets_ops_lock_pair CHECK ((stock_locked_at IS NULL) = (ops_start_date IS NULL));

-- +goose Down
ALTER TABLE outlets DROP CONSTRAINT outlets_ops_lock_pair, DROP COLUMN ops_start_date, DROP COLUMN stock_locked_at;
