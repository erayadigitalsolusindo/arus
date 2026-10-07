-- name: AuthzGetUserAccess :one
SELECT r.permissions, u.name, u.active AS user_active, t.active AS tenant_active, u.tokens_valid_after
FROM users u
JOIN roles r   ON r.tenant_id = u.tenant_id AND r.id = u.role_id
JOIN tenants t ON t.id = u.tenant_id
WHERE u.tenant_id = $1 AND u.id = $2;

-- name: AuthzListAccessibleOutlets :many
-- Outlet aktif yang boleh diakses pengguna: semua bila Owner (izin `*`), selain itu yang ditugaskan di user_outlets.
SELECT o.id FROM outlets o
WHERE o.tenant_id = sqlc.arg(tenant_id) AND o.active
  AND (sqlc.arg(all_outlets)::boolean OR EXISTS (SELECT 1 FROM user_outlets uo WHERE uo.tenant_id = o.tenant_id AND uo.user_id = sqlc.arg(user_id) AND uo.outlet_id = o.id));
