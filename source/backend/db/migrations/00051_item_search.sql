-- +goose Up
-- Pencarian barang yang tetap cepat untuk katalog besar (>100 ribu barang per tenant, ratusan tenant per server).
--
-- Mengapa lewat fungsi SECURITY DEFINER: di bawah Row Level Security, Postgres hanya memakai indeks untuk syarat
-- yang operatornya LEAKPROOF. ILIKE (texticlike) dan lower() tidak leakproof, sehingga indeks trigram tidak pernah
-- terpakai oleh role aplikasi (aciraba_app) dan setiap pencarian memindai seluruh barang tenant (±250 ms pada
-- 150 ribu barang). Fungsi di bawah dijalankan sebagai pemilik tabel (melewati RLS), tetapi tenant TETAP diambil
-- dari app_tenant_id() milik transaksi pemanggil (db.WithTenant) — tanpa tenant, hasilnya kosong (fail closed).
-- Fungsi hanya mengembalikan id + kunci urut; baris lengkap dibaca ulang lewat query biasa (di bawah RLS).
--
-- btree_gin: tenant_id ikut di dalam indeks GIN, sehingga kata umum ("serum") tidak menyentuh barang tenant lain.
CREATE EXTENSION IF NOT EXISTS pg_trgm;
CREATE EXTENSION IF NOT EXISTS btree_gin;

-- Teks cari = nama + kode + barcode. Ekspresi ini harus sama persis dengan yang dipakai item_search().
CREATE INDEX items_search_trgm_idx ON items
    USING gin (tenant_id, (name || ' ' || sku || ' ' || coalesce(barcode, '')) gin_trgm_ops);

-- Statistik ekspresi: tanpa ini perencana menebak ±1% barang cocok untuk setiap kata, lalu memilih menyusuri indeks
-- nama sampai LIMIT terpenuhi. Untuk kata yang jarang/tidak ada, itu berarti memindai seluruh barang tenant (±150 ms).
-- Dengan statistik, kata jarang memakai indeks trigram dan kata yang sangat umum ("ml") tetap menyusuri indeks nama.
CREATE STATISTICS items_search_stx ON ((name || ' ' || sku || ' ' || coalesce(barcode, ''))) FROM items;
ANALYZE items;

-- item_search: id barang yang teks carinya memuat SEMUA pola (AND), urut nama lalu id, paginasi keyset.
-- p_patterns: pola ILIKE yang sudah di-escape pemanggil, mis. '%wardah%' (maks 8; kosong = semua barang).
-- p_active: NULL = semua status. p_after_name/p_after_id: kunci baris terakhir halaman sebelumnya (NULL = halaman 1).
-- Satu syarat ILIKE per pola dibentuk dinamis agar perencana memakai indeks untuk setiap kata; nilai tetap lewat USING.
-- +goose StatementBegin
CREATE FUNCTION item_search(p_patterns text[], p_active boolean, p_after_name text, p_after_id uuid, p_limit integer)
RETURNS TABLE (id uuid, sort_name text)
LANGUAGE plpgsql STABLE SECURITY DEFINER
SET search_path = public, pg_temp
AS $$
DECLARE
    tid uuid := app_tenant_id();
    n   integer := coalesce(array_length(p_patterns, 1), 0);
    q   text := 'SELECT i.id, lower(i.name) FROM items i WHERE i.tenant_id = $1';
BEGIN
    IF tid IS NULL OR n > 8 OR p_limit IS NULL OR p_limit < 1 OR p_limit > 201 THEN
        RETURN;
    END IF;
    IF p_active IS NOT NULL THEN
        q := q || ' AND i.active = $6';
    END IF;
    FOR k IN 1..n LOOP
        q := q || format(' AND (i.name || '' '' || i.sku || '' '' || coalesce(i.barcode, '''')) ILIKE $2[%s]', k);
    END LOOP;
    IF p_after_id IS NOT NULL THEN
        q := q || ' AND (lower(i.name), i.id) > ($3, $4)';
    END IF;
    q := q || ' ORDER BY lower(i.name), i.id LIMIT $5';
    RETURN QUERY EXECUTE q USING tid, p_patterns, p_after_name, p_after_id, p_limit, p_active;
END
$$;
-- +goose StatementEnd

-- item_find_exact: barang yang kodenya (tanpa membedakan huruf) atau barcode barangnya persis p_code.
-- Dipakai kasir agar kode yang diketik lengkap langsung ditemukan walau juga cocok sebagian dengan kode lain
-- (ITM-000012 vs ITM-0000120). Barcode satuan tambahan sudah ditangani GET /items/by-barcode.
-- +goose StatementBegin
CREATE FUNCTION item_find_exact(p_code text, p_active boolean)
RETURNS TABLE (id uuid)
LANGUAGE sql STABLE SECURITY DEFINER
SET search_path = public, pg_temp
AS $$
    SELECT i.id FROM items i
    WHERE i.tenant_id = app_tenant_id()
      AND (lower(i.sku) = lower(p_code) OR i.barcode = p_code)
      AND (p_active IS NULL OR i.active = p_active)
    ORDER BY lower(i.name), i.id
    LIMIT 20
$$;
-- +goose StatementEnd

REVOKE ALL ON FUNCTION item_search(text[], boolean, text, uuid, integer) FROM PUBLIC;
REVOKE ALL ON FUNCTION item_find_exact(text, boolean) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION item_search(text[], boolean, text, uuid, integer), item_find_exact(text, boolean) TO aciraba_app;

-- +goose Down
DROP FUNCTION IF EXISTS item_find_exact(text, boolean);
DROP FUNCTION IF EXISTS item_search(text[], boolean, text, uuid, integer);
DROP STATISTICS IF EXISTS items_search_stx;
DROP INDEX IF EXISTS items_search_trgm_idx;
