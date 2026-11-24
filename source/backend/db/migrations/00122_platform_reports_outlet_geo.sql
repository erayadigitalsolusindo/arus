-- +goose Up

-- Lokasi outlet (GPS). Keduanya terisi atau keduanya kosong.
ALTER TABLE outlets
    ADD COLUMN latitude  numeric(9,6) CHECK (latitude  BETWEEN -90  AND 90),
    ADD COLUMN longitude numeric(9,6) CHECK (longitude BETWEEN -180 AND 180),
    ADD CONSTRAINT outlets_geo_pair_chk CHECK ((latitude IS NULL) = (longitude IS NULL));

-- Laporan lanjutan Platform Admin (lintas tenant, melewati RLS; admin diverifikasi di DB).
-- +goose StatementBegin
-- Penjualan per tenant pada [p_from, p_to) dibanding periode sebelumnya yang sama panjang. Hanya nota 'completed'.
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

-- Tren 6 bulan terakhir (bulan kalender WIB) satu tenant.
CREATE FUNCTION platform_report_sales_monthly(p_admin uuid, p_tenant uuid)
RETURNS TABLE (month date, sales_count bigint, revenue numeric)
LANGUAGE plpgsql STABLE SECURITY DEFINER SET search_path = public, pg_temp AS
$$
BEGIN
    PERFORM platform_assert_admin(p_admin);
    RETURN QUERY
    SELECT m::date, coalesce(x.c, 0), coalesce(x.r, 0)
      FROM generate_series(date_trunc('month', now() AT TIME ZONE 'Asia/Jakarta') - interval '5 months',
                           date_trunc('month', now() AT TIME ZONE 'Asia/Jakarta'), interval '1 month') m
      LEFT JOIN (SELECT date_trunc('month', s.created_at AT TIME ZONE 'Asia/Jakarta') AS mm, count(*) AS c, sum(s.total) AS r
                   FROM sales s
                  WHERE s.tenant_id = p_tenant AND s.status = 'completed'
                    AND s.created_at >= (date_trunc('month', now() AT TIME ZONE 'Asia/Jakarta') - interval '5 months') AT TIME ZONE 'Asia/Jakarta'
                  GROUP BY 1) x ON x.mm = m
     ORDER BY m;
END
$$;

-- Langganan yang berakhir dalam p_days hari ke depan, atau sudah berakhir <= 30 hari lalu (tenant kembali ke paket default).
CREATE FUNCTION platform_report_expiring(p_admin uuid, p_days int)
RETURNS TABLE (tenant_id uuid, code text, name text, active boolean, owner_email text, plan_name text, plan_price numeric,
               expires_at timestamptz, note text, outlet_count bigint, user_count bigint, last_login_at timestamptz,
               revenue_30d numeric)
LANGUAGE plpgsql STABLE SECURITY DEFINER SET search_path = public, pg_temp AS
$$
BEGIN
    PERFORM platform_assert_admin(p_admin);
    RETURN QUERY
    SELECT t.id, t.code, t.name, t.active,
           (SELECT u.email FROM users u JOIN roles r ON r.tenant_id = u.tenant_id AND r.id = u.role_id
             WHERE u.tenant_id = t.id AND r.is_system ORDER BY u.created_at LIMIT 1),
           p.name, p.price, s.expires_at, coalesce(s.note, '')::text,
           (SELECT count(*) FROM outlets o WHERE o.tenant_id = t.id),
           (SELECT count(*) FROM users u WHERE u.tenant_id = t.id),
           (SELECT max(u.last_login_at) FROM users u WHERE u.tenant_id = t.id),
           (SELECT coalesce(sum(x.total), 0) FROM sales x
             WHERE x.tenant_id = t.id AND x.status = 'completed' AND x.created_at >= now() - interval '30 days')
      FROM tenant_subscriptions s
      JOIN tenants t ON t.id = s.tenant_id
      JOIN plans p ON p.id = s.plan_id
     WHERE s.expires_at IS NOT NULL
       AND s.expires_at < now() + make_interval(days => LEAST(GREATEST(p_days, 1), 365))
       AND s.expires_at >= now() - interval '30 days'
     ORDER BY s.expires_at, t.name;
END
$$;

-- Semua outlet beserta koordinatnya (kosong = belum diisi).
CREATE FUNCTION platform_outlet_locations(p_admin uuid)
RETURNS TABLE (tenant_id uuid, tenant_name text, outlet_id uuid, outlet_name text, address text, active boolean,
               latitude numeric, longitude numeric)
LANGUAGE plpgsql STABLE SECURITY DEFINER SET search_path = public, pg_temp AS
$$
BEGIN
    PERFORM platform_assert_admin(p_admin);
    RETURN QUERY
    SELECT t.id, t.name, o.id, o.name, o.address, o.active, o.latitude, o.longitude
      FROM outlets o JOIN tenants t ON t.id = o.tenant_id
     ORDER BY t.name, o.name;
END
$$;
-- +goose StatementEnd

REVOKE ALL ON FUNCTION platform_report_sales(uuid, timestamptz, timestamptz), platform_report_sales_monthly(uuid, uuid),
    platform_report_expiring(uuid, int), platform_outlet_locations(uuid) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION platform_report_sales(uuid, timestamptz, timestamptz), platform_report_sales_monthly(uuid, uuid),
    platform_report_expiring(uuid, int), platform_outlet_locations(uuid) TO aciraba_app;

-- +goose Down
DROP FUNCTION platform_outlet_locations(uuid);
DROP FUNCTION platform_report_expiring(uuid, int);
DROP FUNCTION platform_report_sales_monthly(uuid, uuid);
DROP FUNCTION platform_report_sales(uuid, timestamptz, timestamptz);
ALTER TABLE outlets DROP CONSTRAINT outlets_geo_pair_chk, DROP COLUMN longitude, DROP COLUMN latitude;
