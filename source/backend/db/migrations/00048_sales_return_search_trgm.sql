-- +goose Up

-- Search filters use ILIKE '%query%' on return numbers, sale numbers, member names, and cashier names.
CREATE EXTENSION IF NOT EXISTS pg_trgm;

CREATE INDEX sales_docno_trgm_idx ON sales USING gin (doc_no gin_trgm_ops);
CREATE INDEX sales_returns_docno_trgm_idx ON sales_returns USING gin (doc_no gin_trgm_ops);
CREATE INDEX members_name_trgm_idx ON members USING gin (name gin_trgm_ops);
CREATE INDEX users_name_trgm_idx ON users USING gin (name gin_trgm_ops);

-- +goose Down
DROP INDEX IF EXISTS users_name_trgm_idx;
DROP INDEX IF EXISTS members_name_trgm_idx;
DROP INDEX IF EXISTS sales_returns_docno_trgm_idx;
DROP INDEX IF EXISTS sales_docno_trgm_idx;
