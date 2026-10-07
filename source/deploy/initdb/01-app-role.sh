#!/bin/sh
# Dijalankan otomatis Postgres hanya saat volume data masih kosong (inisialisasi pertama).
# Membuat role runtime `aciraba_app`: BUKAN pemilik tabel dan tanpa BYPASSRLS, sehingga Row Level Security
# (backend/db/migrations/00004_rls.sql) selalu berlaku untuk aplikasi. Pemilik skema (POSTGRES_USER) hanya untuk migration.
set -e
psql -v ON_ERROR_STOP=1 --username "$POSTGRES_USER" --dbname "$POSTGRES_DB" <<SQL
CREATE ROLE aciraba_app LOGIN PASSWORD '${POSTGRES_APP_PASSWORD}' NOSUPERUSER NOBYPASSRLS NOCREATEDB NOCREATEROLE;
GRANT CONNECT ON DATABASE "${POSTGRES_DB}" TO aciraba_app;
GRANT USAGE ON SCHEMA public TO aciraba_app;
SQL
