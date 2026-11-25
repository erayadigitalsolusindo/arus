-- +goose Up
-- Pemakaian metode pembayaran per tenant pada [p_from, p_to): jumlah nota & nominal per jenis (EraPay dipisah dari e-wallet biasa).
-- Hanya nota 'completed'; tunai dihitung bersih (dikurangi kembalian). Admin diverifikasi di DB; cakupan tenant disaring di Go.
-- +goose StatementBegin
CREATE FUNCTION platform_report_sales_payments(p_admin uuid, p_from timestamptz, p_to timestamptz)
RETURNS TABLE (tenant_id uuid, kind text, erapay boolean, sales_count bigint, amount numeric)
LANGUAGE plpgsql STABLE SECURITY DEFINER SET search_path = public, pg_temp AS
$$
BEGIN
    PERFORM platform_assert_admin(p_admin);
    RETURN QUERY
    SELECT sp.tenant_id, sp.method,
           EXISTS (SELECT 1 FROM erapay_methods em WHERE em.tenant_id = sp.tenant_id AND em.method_id = sp.method_id),
           count(DISTINCT sp.sale_id),
           sum(sp.amount - CASE WHEN sp.method = 'cash'
                 AND sp.position = (SELECT min(c.position) FROM sale_payments c WHERE c.sale_id = sp.sale_id AND c.method = 'cash')
                 THEN s.change ELSE 0 END)
      FROM sale_payments sp JOIN sales s ON s.tenant_id = sp.tenant_id AND s.id = sp.sale_id
     WHERE s.status = 'completed' AND s.created_at >= p_from AND s.created_at < p_to
     GROUP BY sp.tenant_id, sp.method, sp.method_id;
END
$$;
-- +goose StatementEnd

REVOKE ALL ON FUNCTION platform_report_sales_payments(uuid, timestamptz, timestamptz) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION platform_report_sales_payments(uuid, timestamptz, timestamptz) TO aciraba_app;

-- +goose Down
DROP FUNCTION platform_report_sales_payments(uuid, timestamptz, timestamptz);
