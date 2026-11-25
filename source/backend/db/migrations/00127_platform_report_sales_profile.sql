-- +goose Up
-- Laporan Penjualan Tenant: tambah jenis & lokasi tenant agar bisa difilter.
DROP FUNCTION platform_report_sales(uuid, timestamptz, timestamptz);
-- +goose StatementBegin
CREATE FUNCTION platform_report_sales(p_admin uuid, p_from timestamptz, p_to timestamptz)
RETURNS TABLE (tenant_id uuid, code text, name text, active boolean, plan_name text, outlet_count bigint,
               sales_count bigint, revenue numeric, prev_count bigint, prev_revenue numeric,
               active_days bigint, last_sale_at timestamptz,
               tenant_type text, location text)
LANGUAGE plpgsql STABLE SECURITY DEFINER SET search_path = public, pg_temp AS
$$
DECLARE
    v_prev timestamptz := p_from - (p_to - p_from);
BEGIN
    PERFORM platform_assert_admin(p_admin);
    RETURN QUERY
    WITH agg AS (
        SELECT s.tenant_id,
               count(*) FILTER (WHERE s.created_at >= p_from) AS c,
               coalesce(sum(s.total) FILTER (WHERE s.created_at >= p_from), 0) AS r,
               count(*) FILTER (WHERE s.created_at < p_from) AS pc,
               coalesce(sum(s.total) FILTER (WHERE s.created_at < p_from), 0) AS pr,
               count(DISTINCT (s.created_at AT TIME ZONE 'Asia/Jakarta')::date) FILTER (WHERE s.created_at >= p_from) AS d
          FROM sales s
         WHERE s.status = 'completed' AND s.created_at >= v_prev AND s.created_at < p_to
         GROUP BY s.tenant_id)
    SELECT t.id, t.code, t.name, t.active, p.name,
           (SELECT count(*) FROM outlets o WHERE o.tenant_id = t.id),
           coalesce(a.c, 0), coalesce(a.r, 0), coalesce(a.pc, 0), coalesce(a.pr, 0), coalesce(a.d, 0),
           (SELECT max(x.created_at) FROM sales x WHERE x.tenant_id = t.id AND x.status = 'completed'),
           coalesce(t.tenant_type, ''), coalesce(t.location, '')
      FROM tenants t
      LEFT JOIN agg a ON a.tenant_id = t.id
      LEFT JOIN tenant_subscriptions ts ON ts.tenant_id = t.id AND (ts.expires_at IS NULL OR ts.expires_at > now())
      JOIN plans p ON p.id = coalesce(ts.plan_id, (SELECT d.id FROM plans d WHERE d.is_default))
     ORDER BY coalesce(a.r, 0) DESC, t.name;
END
$$;
-- +goose StatementEnd

REVOKE ALL ON FUNCTION platform_report_sales(uuid, timestamptz, timestamptz) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION platform_report_sales(uuid, timestamptz, timestamptz) TO aciraba_app;

-- +goose Down
DROP FUNCTION platform_report_sales(uuid, timestamptz, timestamptz);
-- +goose StatementBegin
CREATE FUNCTION platform_report_sales(p_admin uuid, p_from timestamptz, p_to timestamptz)
RETURNS TABLE (tenant_id uuid, code text, name text, active boolean, plan_name text, outlet_count bigint,
               sales_count bigint, revenue numeric, prev_count bigint, prev_revenue numeric,
               active_days bigint, last_sale_at timestamptz)
LANGUAGE plpgsql STABLE SECURITY DEFINER SET search_path = public, pg_temp AS
$$
DECLARE
    v_prev timestamptz := p_from - (p_to - p_from);
BEGIN
    PERFORM platform_assert_admin(p_admin);
    RETURN QUERY
    WITH agg AS (
        SELECT s.tenant_id,
               count(*) FILTER (WHERE s.created_at >= p_from) AS c,
               coalesce(sum(s.total) FILTER (WHERE s.created_at >= p_from), 0) AS r,
               count(*) FILTER (WHERE s.created_at < p_from) AS pc,
               coalesce(sum(s.total) FILTER (WHERE s.created_at < p_from), 0) AS pr,
               count(DISTINCT (s.created_at AT TIME ZONE 'Asia/Jakarta')::date) FILTER (WHERE s.created_at >= p_from) AS d
          FROM sales s
         WHERE s.status = 'completed' AND s.created_at >= v_prev AND s.created_at < p_to
         GROUP BY s.tenant_id)
    SELECT t.id, t.code, t.name, t.active, p.name,
           (SELECT count(*) FROM outlets o WHERE o.tenant_id = t.id),
           coalesce(a.c, 0), coalesce(a.r, 0), coalesce(a.pc, 0), coalesce(a.pr, 0), coalesce(a.d, 0),
           (SELECT max(x.created_at) FROM sales x WHERE x.tenant_id = t.id AND x.status = 'completed')
      FROM tenants t
      LEFT JOIN agg a ON a.tenant_id = t.id
      LEFT JOIN tenant_subscriptions ts ON ts.tenant_id = t.id AND (ts.expires_at IS NULL OR ts.expires_at > now())
      JOIN plans p ON p.id = coalesce(ts.plan_id, (SELECT d.id FROM plans d WHERE d.is_default))
     ORDER BY coalesce(a.r, 0) DESC, t.name;
END
$$;
-- +goose StatementEnd

REVOKE ALL ON FUNCTION platform_report_sales(uuid, timestamptz, timestamptz) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION platform_report_sales(uuid, timestamptz, timestamptz) TO aciraba_app;
