-- name: ApprovalUsers :many
-- Kandidat penyetuju untuk outlet ini: pengguna aktif ber-PIN. Izin dicek di Go (role.permissions), akses outlet di sini.
SELECT u.id, u.name, r.permissions,
       (coalesce(r.permissions ->> '*' = 'true', false) OR EXISTS (SELECT 1 FROM user_outlets uo WHERE uo.tenant_id = u.tenant_id AND uo.user_id = u.id AND uo.outlet_id = @outlet_id))::boolean AS outlet_ok
FROM users u JOIN roles r ON r.tenant_id = u.tenant_id AND r.id = u.role_id
WHERE u.tenant_id = @tenant_id AND u.active AND u.pin_hash IS NOT NULL
ORDER BY lower(u.name), u.id;

-- name: ApprovalUserGet :one
SELECT u.id, u.name, u.active, u.pin_hash, r.permissions,
       (coalesce(r.permissions ->> '*' = 'true', false) OR EXISTS (SELECT 1 FROM user_outlets uo WHERE uo.tenant_id = u.tenant_id AND uo.user_id = u.id AND uo.outlet_id = @outlet_id))::boolean AS outlet_ok
FROM users u JOIN roles r ON r.tenant_id = u.tenant_id AND r.id = u.role_id
WHERE u.tenant_id = @tenant_id AND u.id = @id;

-- name: ApprovalSelf :one
SELECT u.password_hash, (u.pin_hash IS NOT NULL)::boolean AS has_pin, r.permissions
FROM users u JOIN roles r ON r.tenant_id = u.tenant_id AND r.id = u.role_id
WHERE u.tenant_id = $1 AND u.id = $2 AND u.active;

-- name: ApprovalSetPin :exec
UPDATE users SET pin_hash = $3 WHERE tenant_id = $1 AND id = $2;
