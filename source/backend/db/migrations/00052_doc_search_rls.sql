-- +goose Up
-- Pencarian dokumen (pembelian, retur beli, retur jual, penjualan) yang tetap memakai indeks di bawah RLS.
--
-- Masalah (diukur 2026-10-10, 365 rb pembelian / 430 rb penjualan satu tenant): indeks trigram dari 00039, 00046, 00048
-- tidak pernah terpakai. (1) Di bawah RLS, syarat ILIKE/lower() tidak LEAKPROOF sehingga tidak bisa menjadi syarat indeks
-- bagi role aplikasi. (2) Bahkan sebagai pemilik tabel, syarat "doc_no ILIKE … OR s.name ILIKE …" melintasi tabel yang
-- di-JOIN sehingga tidak bisa digabung jadi BitmapOr. Akibatnya setiap pencarian memindai seluruh rentang (0,4–2,4 detik).
--
-- Pola (sama dengan item_search, 00051): fungsi SECURITY DEFINER *_search_ids mengembalikan id KANDIDAT (maks p_limit,
-- tanpa urutan) di dalam jendela tenant + outlet + rentang tanggal. Tenant SELALU dari app_tenant_id() transaksi pemanggil
-- (tanpa tenant = kosong). Nama pemasok/member/kasir lebih dulu diubah menjadi daftar id (tabel kecil), lalu setiap cabang
-- syarat ditulis pada kolom tabel dokumen itu sendiri sehingga perencana bisa memakai BitmapOr atas indeks trigram + btree.
-- SQL dibentuk dinamis (EXECUTE … USING) agar direncanakan dengan pola sebenarnya; cabang yang daftar id-nya kosong dibuang.
-- Pemanggil (Go) membaca baris lengkap, ringkasan, dan keyset lewat query biasa (di bawah RLS) dengan "id = ANY(kandidat)".
-- Bila kandidat melebihi batas (kata yang sangat umum), pemanggil memakai query ILIKE lama — pada kasus itu menyusuri
-- indeks urutan memang lebih cepat dan ringkasan memang harus menjumlah banyak baris.
CREATE EXTENSION IF NOT EXISTS pg_trgm;
CREATE EXTENSION IF NOT EXISTS btree_gin;

-- Indeks trigram lama tanpa tenant_id diganti indeks ber-tenant (btree_gin): kata umum tidak menyentuh tenant lain.
DROP INDEX IF EXISTS purchases_docno_trgm_idx;
DROP INDEX IF EXISTS purchases_invoice_trgm_idx;
DROP INDEX IF EXISTS suppliers_name_trgm_idx;
DROP INDEX IF EXISTS purchase_returns_docno_trgm_idx;
DROP INDEX IF EXISTS sales_docno_trgm_idx;
DROP INDEX IF EXISTS sales_returns_docno_trgm_idx;
DROP INDEX IF EXISTS members_name_trgm_idx;
DROP INDEX IF EXISTS users_name_trgm_idx; -- pengguna per tenant sedikit; indeks btree tenant sudah cukup

-- Teks cari pembelian = nomor dokumen + pemisah \x01 + nomor faktur pemasok (pemisah mencegah pola melintasi dua kolom;
-- karakter kontrol ditolak sanitize sehingga tidak pernah ada di kata cari). Ekspresi harus sama persis dengan di fungsi.
CREATE INDEX purchases_search_trgm_idx ON purchases USING gin (tenant_id, (doc_no || E'\x01' || supplier_invoice_no) gin_trgm_ops);
CREATE STATISTICS purchases_search_stx ON ((doc_no || E'\x01' || supplier_invoice_no)) FROM purchases;
CREATE INDEX suppliers_name_trgm_idx ON suppliers USING gin (tenant_id, name gin_trgm_ops);
CREATE INDEX purchase_returns_docno_trgm_idx ON purchase_returns USING gin (tenant_id, doc_no gin_trgm_ops);
CREATE INDEX sales_docno_trgm_idx ON sales USING gin (tenant_id, doc_no gin_trgm_ops);
CREATE INDEX sales_returns_docno_trgm_idx ON sales_returns USING gin (tenant_id, doc_no gin_trgm_ops);
CREATE INDEX members_name_trgm_idx ON members USING gin (tenant_id, name gin_trgm_ops);
CREATE INDEX members_code_trgm_idx ON members USING gin (tenant_id, code gin_trgm_ops);
-- Cabang "pemasok/member retur = id" butuh indeks agar bisa ikut BitmapOr.
CREATE INDEX sales_returns_member_idx ON sales_returns (tenant_id, member_id) WHERE member_id IS NOT NULL;
CREATE INDEX purchase_returns_supplier_idx ON purchase_returns (tenant_id, supplier_id);
ANALYZE purchases;

-- Batas daftar id pihak (pemasok/member/kasir/nota sumber) yang dibawa ke cabang "= ANY". Lebih dari itu: cabang memakai
-- subquery biasa (hasil tetap benar, hanya tidak ber-indeks; kata seperti itu juga menghasilkan banyak dokumen).

-- purchase_search_ids: pembelian di outlet p_outlet (tanggal pembelian p_from..p_to, NULL = tanpa batas) yang nomor
-- dokumen / nomor faktur / nama pemasoknya cocok dengan p_pattern (pola ILIKE yang sudah di-escape pemanggil).
-- +goose StatementBegin
CREATE FUNCTION purchase_search_ids(p_outlet uuid, p_from date, p_to date, p_pattern text, p_limit integer)
RETURNS SETOF uuid
LANGUAGE plpgsql STABLE SECURITY DEFINER
SET search_path = public, pg_temp
AS $$
DECLARE
    tid uuid := app_tenant_id();
    sup uuid[];
    q   text;
BEGIN
    IF tid IS NULL OR p_outlet IS NULL OR coalesce(p_pattern, '') = '' OR p_limit IS NULL OR p_limit < 1 OR p_limit > 20001 THEN
        RETURN;
    END IF;
    sup := ARRAY(SELECT s.id FROM suppliers s WHERE s.tenant_id = tid AND s.name ILIKE p_pattern LIMIT 5001);
    q := 'SELECT p.id FROM purchases p WHERE p.tenant_id = $1 AND p.outlet_id = $2'
      || CASE WHEN p_from IS NULL THEN '' ELSE ' AND p.purchase_date >= $3' END
      || CASE WHEN p_to IS NULL THEN '' ELSE ' AND p.purchase_date <= $4' END
      || ' AND ((p.doc_no || E''\x01'' || p.supplier_invoice_no) ILIKE $5'
      || CASE WHEN cardinality(sup) = 0 THEN ''
              WHEN cardinality(sup) > 5000 THEN ' OR p.supplier_id IN (SELECT s.id FROM suppliers s WHERE s.tenant_id = $1 AND s.name ILIKE $5)'
              ELSE ' OR p.supplier_id = ANY($6)' END
      || ') LIMIT $7';
    RETURN QUERY EXECUTE q USING tid, p_outlet, p_from, p_to, p_pattern, sup, p_limit;
END
$$;
-- +goose StatementEnd

-- purchase_return_search_ids: retur pembelian di outlet (tanggal retur p_from..p_to) yang nomor returnya, nomor dokumen /
-- faktur nota asalnya, atau nama pemasoknya cocok.
-- +goose StatementBegin
CREATE FUNCTION purchase_return_search_ids(p_outlet uuid, p_from date, p_to date, p_pattern text, p_limit integer)
RETURNS SETOF uuid
LANGUAGE plpgsql STABLE SECURITY DEFINER
SET search_path = public, pg_temp
AS $$
DECLARE
    tid uuid := app_tenant_id();
    sup uuid[];
    pur uuid[];
    q   text;
BEGIN
    IF tid IS NULL OR p_outlet IS NULL OR coalesce(p_pattern, '') = '' OR p_limit IS NULL OR p_limit < 1 OR p_limit > 20001 THEN
        RETURN;
    END IF;
    sup := ARRAY(SELECT s.id FROM suppliers s WHERE s.tenant_id = tid AND s.name ILIKE p_pattern LIMIT 5001);
    pur := ARRAY(SELECT p.id FROM purchases p WHERE p.tenant_id = tid AND p.outlet_id = p_outlet
                   AND (p.doc_no || E'\x01' || p.supplier_invoice_no) ILIKE p_pattern LIMIT 5001);
    q := 'SELECT r.id FROM purchase_returns r WHERE r.tenant_id = $1 AND r.outlet_id = $2'
      || CASE WHEN p_from IS NULL THEN '' ELSE ' AND r.return_date >= $3' END
      || CASE WHEN p_to IS NULL THEN '' ELSE ' AND r.return_date <= $4' END
      || ' AND (r.doc_no ILIKE $5'
      || CASE WHEN cardinality(sup) = 0 THEN ''
              WHEN cardinality(sup) > 5000 THEN ' OR r.supplier_id IN (SELECT s.id FROM suppliers s WHERE s.tenant_id = $1 AND s.name ILIKE $5)'
              ELSE ' OR r.supplier_id = ANY($6)' END
      || CASE WHEN cardinality(pur) = 0 THEN ''
              WHEN cardinality(pur) > 5000 THEN ' OR EXISTS (SELECT 1 FROM purchases p WHERE p.tenant_id = $1 AND p.id = r.purchase_id'
                                             || ' AND (p.doc_no || E''\x01'' || p.supplier_invoice_no) ILIKE $5)'
              ELSE ' OR r.purchase_id = ANY($7)' END
      || ') LIMIT $8';
    RETURN QUERY EXECUTE q USING tid, p_outlet, p_from, p_to, p_pattern, sup, pur, p_limit;
END
$$;
-- +goose StatementEnd

-- sales_return_search_ids: retur penjualan di outlet (tanggal retur p_from..p_to) yang nomor returnya, nomor nota asalnya,
-- atau nama member-nya cocok.
-- +goose StatementBegin
CREATE FUNCTION sales_return_search_ids(p_outlet uuid, p_from date, p_to date, p_pattern text, p_limit integer)
RETURNS SETOF uuid
LANGUAGE plpgsql STABLE SECURITY DEFINER
SET search_path = public, pg_temp
AS $$
DECLARE
    tid uuid := app_tenant_id();
    mem uuid[];
    sal uuid[];
    q   text;
BEGIN
    IF tid IS NULL OR p_outlet IS NULL OR coalesce(p_pattern, '') = '' OR p_limit IS NULL OR p_limit < 1 OR p_limit > 20001 THEN
        RETURN;
    END IF;
    mem := ARRAY(SELECT m.id FROM members m WHERE m.tenant_id = tid AND m.name ILIKE p_pattern LIMIT 5001);
    sal := ARRAY(SELECT s.id FROM sales s WHERE s.tenant_id = tid AND s.outlet_id = p_outlet AND s.doc_no ILIKE p_pattern LIMIT 5001);
    q := 'SELECT r.id FROM sales_returns r WHERE r.tenant_id = $1 AND r.outlet_id = $2'
      || CASE WHEN p_from IS NULL THEN '' ELSE ' AND r.return_date >= $3' END
      || CASE WHEN p_to IS NULL THEN '' ELSE ' AND r.return_date <= $4' END
      || ' AND (r.doc_no ILIKE $5'
      || CASE WHEN cardinality(mem) = 0 THEN ''
              WHEN cardinality(mem) > 5000 THEN ' OR r.member_id IN (SELECT m.id FROM members m WHERE m.tenant_id = $1 AND m.name ILIKE $5)'
              ELSE ' OR r.member_id = ANY($6)' END
      || CASE WHEN cardinality(sal) = 0 THEN ''
              WHEN cardinality(sal) > 5000 THEN ' OR EXISTS (SELECT 1 FROM sales s WHERE s.tenant_id = $1 AND s.id = r.sale_id AND s.doc_no ILIKE $5)'
              ELSE ' OR r.sale_id = ANY($7)' END
      || ') LIMIT $8';
    RETURN QUERY EXECUTE q USING tid, p_outlet, p_from, p_to, p_pattern, mem, sal, p_limit;
END
$$;
-- +goose StatementEnd

-- sale_search_ids: nota penjualan di outlet p_outlets (created_at p_from <= x < p_to, NULL = tanpa batas) yang nomor
-- notanya, nama member (p_member_code: juga kode member), atau nama kasirnya cocok.
-- +goose StatementBegin
CREATE FUNCTION sale_search_ids(p_outlets uuid[], p_from timestamptz, p_to timestamptz, p_pattern text, p_member_code boolean, p_limit integer)
RETURNS SETOF uuid
LANGUAGE plpgsql STABLE SECURITY DEFINER
SET search_path = public, pg_temp
AS $$
DECLARE
    tid uuid := app_tenant_id();
    mem uuid[];
    usr uuid[];
    q   text;
BEGIN
    IF tid IS NULL OR coalesce(cardinality(p_outlets), 0) = 0 OR coalesce(p_pattern, '') = ''
       OR p_limit IS NULL OR p_limit < 1 OR p_limit > 20001 THEN
        RETURN;
    END IF;
    mem := ARRAY(SELECT m.id FROM members m WHERE m.tenant_id = tid
                   AND (m.name ILIKE p_pattern OR (p_member_code AND m.code ILIKE p_pattern)) LIMIT 5001);
    usr := ARRAY(SELECT u.id FROM users u WHERE u.tenant_id = tid AND u.name ILIKE p_pattern LIMIT 5001);
    q := 'SELECT s.id FROM sales s WHERE s.tenant_id = $1 AND s.outlet_id = ANY($2)'
      || CASE WHEN p_from IS NULL THEN '' ELSE ' AND s.created_at >= $3' END
      || CASE WHEN p_to IS NULL THEN '' ELSE ' AND s.created_at < $4' END
      || ' AND (s.doc_no ILIKE $5'
      || CASE WHEN cardinality(mem) = 0 THEN ''
              WHEN cardinality(mem) > 5000 THEN ' OR s.member_id IN (SELECT m.id FROM members m WHERE m.tenant_id = $1'
                                             || ' AND (m.name ILIKE $5 OR ($6 AND m.code ILIKE $5)))'
              ELSE ' OR s.member_id = ANY($7)' END
      || CASE WHEN cardinality(usr) = 0 THEN '' ELSE ' OR s.cashier_id = ANY($8)' END
      || ') LIMIT $9';
    RETURN QUERY EXECUTE q USING tid, p_outlets, p_from, p_to, p_pattern, p_member_code, mem, usr, p_limit;
END
$$;
-- +goose StatementEnd

-- Cek kode yang sudah dipakai (kode otomatis barang/member). "lower(sku) = lower($1)" tidak leakproof sehingga di bawah RLS
-- indeks unik (tenant_id, lower(sku)) hanya terpakai sebagian dan seluruh barang tenant dipindai (±30 ms per cek pada
-- 150 rb barang — terasa saat impor). Dijalankan sebagai pemilik tabel, tenant tetap dari app_tenant_id().
-- +goose StatementBegin
CREATE FUNCTION item_sku_taken(p_sku text) RETURNS boolean
LANGUAGE sql STABLE SECURITY DEFINER
SET search_path = public, pg_temp
AS $$
    SELECT EXISTS (SELECT 1 FROM items i WHERE i.tenant_id = app_tenant_id() AND lower(i.sku) = lower(p_sku))
$$;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE FUNCTION member_code_taken(p_code text) RETURNS boolean
LANGUAGE sql STABLE SECURITY DEFINER
SET search_path = public, pg_temp
AS $$
    SELECT EXISTS (SELECT 1 FROM members m WHERE m.tenant_id = app_tenant_id() AND lower(m.code) = lower(p_code))
$$;
-- +goose StatementEnd

REVOKE ALL ON FUNCTION purchase_search_ids(uuid, date, date, text, integer) FROM PUBLIC;
REVOKE ALL ON FUNCTION purchase_return_search_ids(uuid, date, date, text, integer) FROM PUBLIC;
REVOKE ALL ON FUNCTION sales_return_search_ids(uuid, date, date, text, integer) FROM PUBLIC;
REVOKE ALL ON FUNCTION sale_search_ids(uuid[], timestamptz, timestamptz, text, boolean, integer) FROM PUBLIC;
REVOKE ALL ON FUNCTION item_sku_taken(text) FROM PUBLIC;
REVOKE ALL ON FUNCTION member_code_taken(text) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION purchase_search_ids(uuid, date, date, text, integer),
                          purchase_return_search_ids(uuid, date, date, text, integer),
                          sales_return_search_ids(uuid, date, date, text, integer),
                          sale_search_ids(uuid[], timestamptz, timestamptz, text, boolean, integer),
                          item_sku_taken(text), member_code_taken(text) TO aciraba_app;

-- +goose Down
DROP FUNCTION IF EXISTS member_code_taken(text);
DROP FUNCTION IF EXISTS item_sku_taken(text);
DROP FUNCTION IF EXISTS sale_search_ids(uuid[], timestamptz, timestamptz, text, boolean, integer);
DROP FUNCTION IF EXISTS sales_return_search_ids(uuid, date, date, text, integer);
DROP FUNCTION IF EXISTS purchase_return_search_ids(uuid, date, date, text, integer);
DROP FUNCTION IF EXISTS purchase_search_ids(uuid, date, date, text, integer);
DROP INDEX IF EXISTS purchase_returns_supplier_idx;
DROP INDEX IF EXISTS sales_returns_member_idx;
DROP INDEX IF EXISTS members_code_trgm_idx;
DROP INDEX IF EXISTS members_name_trgm_idx;
DROP INDEX IF EXISTS sales_returns_docno_trgm_idx;
DROP INDEX IF EXISTS sales_docno_trgm_idx;
DROP INDEX IF EXISTS purchase_returns_docno_trgm_idx;
DROP INDEX IF EXISTS suppliers_name_trgm_idx;
DROP STATISTICS IF EXISTS purchases_search_stx;
DROP INDEX IF EXISTS purchases_search_trgm_idx;
CREATE INDEX purchases_docno_trgm_idx ON purchases USING gin (doc_no gin_trgm_ops);
CREATE INDEX purchases_invoice_trgm_idx ON purchases USING gin (supplier_invoice_no gin_trgm_ops);
CREATE INDEX suppliers_name_trgm_idx ON suppliers USING gin (name gin_trgm_ops);
CREATE INDEX purchase_returns_docno_trgm_idx ON purchase_returns USING gin (doc_no gin_trgm_ops);
CREATE INDEX sales_docno_trgm_idx ON sales USING gin (doc_no gin_trgm_ops);
CREATE INDEX sales_returns_docno_trgm_idx ON sales_returns USING gin (doc_no gin_trgm_ops);
CREATE INDEX members_name_trgm_idx ON members USING gin (name gin_trgm_ops);
CREATE INDEX users_name_trgm_idx ON users USING gin (name gin_trgm_ops);
