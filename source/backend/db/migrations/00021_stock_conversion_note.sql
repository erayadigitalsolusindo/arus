-- +goose Up
-- Catatan pecah satuan boleh berformat markdown (beberapa baris) → batas dinaikkan dari 200 ke 1000 karakter.
ALTER TABLE stock_conversions DROP CONSTRAINT stock_conversions_note_check;
ALTER TABLE stock_conversions ADD CONSTRAINT stock_conversions_note_check CHECK (char_length(note) <= 1000);

-- +goose Down
ALTER TABLE stock_conversions DROP CONSTRAINT stock_conversions_note_check;
ALTER TABLE stock_conversions ADD CONSTRAINT stock_conversions_note_check CHECK (char_length(note) <= 200);
