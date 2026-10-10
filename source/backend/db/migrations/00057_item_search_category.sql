-- +goose Up
-- Filter kategori untuk pencarian kasir (mobile): item_search menerima p_category (NULL = semua kategori).
-- Isi fungsi sama dengan 00051 (SECURITY DEFINER, tenant dari app_tenant_id(), keyset), ditambah satu syarat
-- i.category_id = $7. Indeks (tenant_id, category_id, lower(name), id) membuat kategori besar tetap berurutan
-- tanpa mengurutkan ulang; kata cari tetap memakai indeks trigram.
DROP FUNCTION IF EXISTS item_search(text[], boolean, text, uuid, integer);

CREATE INDEX items_category_name_idx ON items (tenant_id, category_id, lower(name), id);

-- +goose StatementBegin
CREATE FUNCTION item_search(p_patterns text[], p_active boolean, p_after_name text, p_after_id uuid, p_limit integer, p_category uuid DEFAULT NULL)
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
    IF p_category IS NOT NULL THEN
        q := q || ' AND i.category_id = $7';
    END IF;
    FOR k IN 1..n LOOP
        q := q || format(' AND (i.name || '' '' || i.sku || '' '' || coalesce(i.barcode, '''')) ILIKE $2[%s]', k);
    END LOOP;
    IF p_after_id IS NOT NULL THEN
        q := q || ' AND (lower(i.name), i.id) > ($3, $4)';
    END IF;
    q := q || ' ORDER BY lower(i.name), i.id LIMIT $5';
    RETURN QUERY EXECUTE q USING tid, p_patterns, p_after_name, p_after_id, p_limit, p_active, p_category;
END
$$;
-- +goose StatementEnd

REVOKE ALL ON FUNCTION item_search(text[], boolean, text, uuid, integer, uuid) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION item_search(text[], boolean, text, uuid, integer, uuid) TO aciraba_app;

-- +goose Down
DROP FUNCTION IF EXISTS item_search(text[], boolean, text, uuid, integer, uuid);
DROP INDEX IF EXISTS items_category_name_idx;
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
GRANT EXECUTE ON FUNCTION item_search(text[], boolean, text, uuid, integer) TO aciraba_app;
