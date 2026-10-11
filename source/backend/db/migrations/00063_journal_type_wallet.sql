-- +goose Up

-- Jenis jurnal otomatis untuk deposit member & kredit pemasok (top-up/tarik/pencairan). Jenis lain sudah tersedia sejak 00056.
ALTER TABLE journal_entries DROP CONSTRAINT journal_entries_type_check;
ALTER TABLE journal_entries ADD CONSTRAINT journal_entries_type_check
    CHECK (type IN ('JU', 'KM', 'KK', 'TK', 'SALES', 'PURCHASE', 'AR', 'AP', 'WALLET', 'OPENING'));

-- +goose Down
ALTER TABLE journal_entries DROP CONSTRAINT journal_entries_type_check;
ALTER TABLE journal_entries ADD CONSTRAINT journal_entries_type_check
    CHECK (type IN ('JU', 'KM', 'KK', 'TK', 'SALES', 'PURCHASE', 'AR', 'AP', 'OPENING'));
