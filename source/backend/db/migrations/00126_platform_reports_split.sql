-- +goose Up
-- Izin "reports" dipecah per laporan: role lama yang punya reports mendapat ketiganya (perilaku tidak berubah).
UPDATE platform_roles
   SET permissions = (permissions - 'reports')
       || jsonb_build_object('reports_sales', permissions->'reports', 'reports_expiring', permissions->'reports', 'reports_locations', permissions->'reports')
 WHERE permissions ? 'reports';

-- +goose Down
UPDATE platform_roles SET permissions = (permissions - 'reports_sales' - 'reports_expiring' - 'reports_locations')
       || jsonb_build_object('reports', permissions->'reports_sales')
 WHERE permissions ? 'reports_sales';
