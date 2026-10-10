-- +goose Up

-- Data kepala/kaki struk per outlet (FR-POS-15). Struk disusun server dari data ini + nota;
-- cetak ulang dicatat di audit_log (aksi sale.reprint), jadi tidak butuh tabel baru.
ALTER TABLE outlets
    ADD COLUMN address        text NOT NULL DEFAULT '',
    ADD COLUMN phone          text NOT NULL DEFAULT '',
    ADD COLUMN receipt_header text NOT NULL DEFAULT '',
    ADD COLUMN receipt_footer text NOT NULL DEFAULT '',
    ADD CONSTRAINT outlets_address_len CHECK (char_length(address) <= 200),
    ADD CONSTRAINT outlets_phone_len CHECK (char_length(phone) <= 30),
    ADD CONSTRAINT outlets_receipt_header_len CHECK (char_length(receipt_header) <= 300),
    ADD CONSTRAINT outlets_receipt_footer_len CHECK (char_length(receipt_footer) <= 300);

-- +goose Down
ALTER TABLE outlets
    DROP CONSTRAINT outlets_receipt_footer_len,
    DROP CONSTRAINT outlets_receipt_header_len,
    DROP CONSTRAINT outlets_phone_len,
    DROP CONSTRAINT outlets_address_len,
    DROP COLUMN receipt_footer,
    DROP COLUMN receipt_header,
    DROP COLUMN phone,
    DROP COLUMN address;
