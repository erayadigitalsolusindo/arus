-- +goose Up
-- Rincian penjualan satu tenant pada [p_from, p_to): metode pembayaran, outlet, barang terlaris, ringkasan. Hanya nota 'completed'
-- (void dihitung terpisah). Tunai dihitung bersih (dikurangi kembalian). Admin diverifikasi di DB.
-- +goose StatementBegin
CREATE FUNCTION platform_report_sales_detail(p_admin uuid, p_tenant uuid, p_from timestamptz, p_to timestamptz)
RETURNS jsonb
LANGUAGE plpgsql STABLE SECURITY DEFINER SET search_path = public, pg_temp AS
$$
DECLARE
    v jsonb;
BEGIN
    PERFORM platform_assert_admin(p_admin);
    SELECT jsonb_build_object(
        'payments', coalesce((
            SELECT jsonb_agg(x ORDER BY x.amount DESC) FROM (
                SELECT sp.method_name AS name, sp.method AS kind,
                       EXISTS (SELECT 1 FROM erapay_methods em WHERE em.tenant_id = sp.tenant_id AND em.method_id = sp.method_id) AS erapay,
                       count(DISTINCT sp.sale_id) AS count,
                       sum(sp.amount - CASE WHEN sp.method = 'cash'
                             AND sp.position = (SELECT min(c.position) FROM sale_payments c WHERE c.sale_id = sp.sale_id AND c.method = 'cash')
                             THEN s.change ELSE 0 END) AS amount
                  FROM sale_payments sp JOIN sales s ON s.tenant_id = sp.tenant_id AND s.id = sp.sale_id
                 WHERE s.tenant_id = p_tenant AND s.status = 'completed' AND s.created_at >= p_from AND s.created_at < p_to
                 GROUP BY sp.method_name, sp.method, sp.method_id, sp.tenant_id) x), '[]'::jsonb),
        'outlets', coalesce((
            SELECT jsonb_agg(x ORDER BY x.revenue DESC) FROM (
                SELECT o.name, count(*) AS count, sum(s.total) AS revenue
                  FROM sales s JOIN outlets o ON o.tenant_id = s.tenant_id AND o.id = s.outlet_id
                 WHERE s.tenant_id = p_tenant AND s.status = 'completed' AND s.created_at >= p_from AND s.created_at < p_to
                 GROUP BY o.id, o.name) x), '[]'::jsonb),
        'items', coalesce((
            SELECT jsonb_agg(x ORDER BY x.revenue DESC) FROM (
                SELECT l.name, sum(l.qty * l.factor) AS qty, sum(l.line_total) AS revenue
                  FROM sale_lines l JOIN sales s ON s.tenant_id = l.tenant_id AND s.id = l.sale_id
                 WHERE s.tenant_id = p_tenant AND s.status = 'completed' AND s.created_at >= p_from AND s.created_at < p_to
                 GROUP BY l.item_id, l.name ORDER BY sum(l.line_total) DESC LIMIT 5) x), '[]'::jsonb),
        'summary', (
            SELECT jsonb_build_object(
                'voids', count(*) FILTER (WHERE s.status = 'void'),
                'discount', coalesce(sum(s.discount) FILTER (WHERE s.status = 'completed'), 0),
                'max_ticket', coalesce(max(s.total) FILTER (WHERE s.status = 'completed'), 0),
                'peak_hour', (SELECT extract(hour FROM h.created_at AT TIME ZONE 'Asia/Jakarta')::int
                                FROM sales h WHERE h.tenant_id = p_tenant AND h.status = 'completed' AND h.created_at >= p_from AND h.created_at < p_to
                               GROUP BY 1 ORDER BY count(*) DESC, 1 LIMIT 1))
              FROM sales s WHERE s.tenant_id = p_tenant AND s.created_at >= p_from AND s.created_at < p_to))
      INTO v;
    RETURN v;
END
$$;
-- +goose StatementEnd

REVOKE ALL ON FUNCTION platform_report_sales_detail(uuid, uuid, timestamptz, timestamptz) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION platform_report_sales_detail(uuid, uuid, timestamptz, timestamptz) TO aciraba_app;

-- +goose Down
DROP FUNCTION platform_report_sales_detail(uuid, uuid, timestamptz, timestamptz);
