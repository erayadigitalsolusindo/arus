-- +goose Up

-- Live penjualan lintas tenant untuk Platform Admin. Hari = Asia/Jakarta (operator di WIB; penjualan tenant disimpan
-- timestamptz sehingga zona hanya menentukan batas "hari ini").
-- Indeks parsial: umpan "terbaru dulu" lintas tenant dibaca keyset (created_at, id) tanpa sort.
CREATE INDEX sales_live_idx ON sales (created_at DESC, id DESC) WHERE status = 'completed';

-- +goose StatementBegin
CREATE FUNCTION platform_live_feed(p_admin uuid, p_before_at timestamptz, p_before_id uuid, p_limit int)
RETURNS TABLE (id uuid, created_at timestamptz, doc_no text, tenant_id uuid, tenant_name text,
               outlet_name text, cashier text, line_count int, total numeric)
LANGUAGE plpgsql STABLE SECURITY DEFINER SET search_path = public, pg_temp AS
$$
DECLARE
    v_since timestamptz := (now() AT TIME ZONE 'Asia/Jakarta')::date::timestamp AT TIME ZONE 'Asia/Jakarta';
BEGIN
    PERFORM platform_assert_admin(p_admin);
    RETURN QUERY
    SELECT s.id, s.created_at, s.doc_no, s.tenant_id, t.name, o.name, coalesce(u.name, '')::text,
           (SELECT count(*) FROM sale_lines l WHERE l.tenant_id = s.tenant_id AND l.sale_id = s.id)::int, s.total
      FROM (SELECT x.id, x.created_at, x.doc_no, x.tenant_id, x.outlet_id, x.cashier_id, x.total
              FROM sales x
             WHERE x.status = 'completed' AND x.created_at >= v_since
               AND (p_before_at IS NULL OR (x.created_at, x.id) < (p_before_at, p_before_id))
             ORDER BY x.created_at DESC, x.id DESC
             LIMIT LEAST(GREATEST(p_limit, 1), 100)) s
      JOIN tenants t ON t.id = s.tenant_id
      JOIN outlets o ON o.tenant_id = s.tenant_id AND o.id = s.outlet_id
      LEFT JOIN users u ON u.tenant_id = s.tenant_id AND u.id = s.cashier_id
     ORDER BY s.created_at DESC, s.id DESC;
END
$$;

CREATE FUNCTION platform_live_today(p_admin uuid)
RETURNS TABLE (sales_count bigint, sales_total numeric, tenant_count bigint, since timestamptz)
LANGUAGE plpgsql STABLE SECURITY DEFINER SET search_path = public, pg_temp AS
$$
DECLARE
    v_since timestamptz := (now() AT TIME ZONE 'Asia/Jakarta')::date::timestamp AT TIME ZONE 'Asia/Jakarta';
BEGIN
    PERFORM platform_assert_admin(p_admin);
    RETURN QUERY
    SELECT count(*), coalesce(sum(s.total), 0), count(DISTINCT s.tenant_id), v_since
      FROM sales s WHERE s.status = 'completed' AND s.created_at >= v_since;
END
$$;

-- Nama + penjualan terakhir (15 menit) untuk pengguna yang sedang online (daftar dari Redis). Per outlet agar
-- memakai indeks (tenant_id, outlet_id, cashier_id, created_at).
CREATE FUNCTION platform_live_people(p_admin uuid, p_tenants uuid[], p_users uuid[])
RETURNS TABLE (tenant_id uuid, tenant_name text, user_id uuid, user_name text, last_sale_at timestamptz)
LANGUAGE plpgsql STABLE SECURITY DEFINER SET search_path = public, pg_temp AS
$$
BEGIN
    PERFORM platform_assert_admin(p_admin);
    RETURN QUERY
    SELECT u.tenant_id, t.name, u.id, u.name,
           (SELECT max(s.created_at)
              FROM outlets o, LATERAL (SELECT max(x.created_at) AS created_at FROM sales x
                    WHERE x.tenant_id = o.tenant_id AND x.outlet_id = o.id AND x.cashier_id = u.id
                      AND x.created_at > now() - interval '15 minutes') s
             WHERE o.tenant_id = u.tenant_id)
      FROM unnest(p_tenants, p_users) AS p(tid, uid)
      JOIN users u ON u.tenant_id = p.tid AND u.id = p.uid AND u.active
      JOIN tenants t ON t.id = u.tenant_id;
END
$$;
-- +goose StatementEnd

REVOKE ALL ON FUNCTION platform_live_feed(uuid, timestamptz, uuid, int), platform_live_today(uuid),
    platform_live_people(uuid, uuid[], uuid[]) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION platform_live_feed(uuid, timestamptz, uuid, int), platform_live_today(uuid),
    platform_live_people(uuid, uuid[], uuid[]) TO aciraba_app;

-- +goose Down
DROP FUNCTION platform_live_people(uuid, uuid[], uuid[]);
DROP FUNCTION platform_live_today(uuid);
DROP FUNCTION platform_live_feed(uuid, timestamptz, uuid, int);
DROP INDEX sales_live_idx;
