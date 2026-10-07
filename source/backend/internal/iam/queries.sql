-- name: IamGetUserPermissions :one
SELECT r.permissions, u.active AS user_active, t.active AS tenant_active
FROM users u
JOIN roles r   ON r.tenant_id = u.tenant_id AND r.id = u.role_id
JOIN tenants t ON t.id = u.tenant_id
WHERE u.tenant_id = $1 AND u.id = $2;

-- name: IamListRoles :many
SELECT r.id, r.name, r.permissions, r.is_system,
       (SELECT count(*) FROM users u WHERE u.tenant_id = r.tenant_id AND u.role_id = r.id) AS user_count
FROM roles r
WHERE r.tenant_id = $1
ORDER BY r.is_system DESC, lower(r.name);

-- name: IamGetRole :one
SELECT id, name, permissions, is_system FROM roles WHERE tenant_id = $1 AND id = $2;

-- name: IamCreateRole :one
INSERT INTO roles (tenant_id, name, permissions) VALUES ($1, $2, $3)
RETURNING id, name, permissions, is_system;

-- name: IamUpdateRole :one
UPDATE roles SET name = $3, permissions = $4
WHERE tenant_id = $1 AND id = $2 AND NOT is_system
RETURNING id, name, permissions, is_system;

-- name: IamDeleteRole :execrows
DELETE FROM roles WHERE tenant_id = $1 AND id = $2 AND NOT is_system;

-- name: IamListUsers :many
SELECT u.id, u.name, u.email, u.phone, u.active, u.last_login_at, u.created_at, u.role_id, r.name AS role_name
FROM users u
JOIN roles r ON r.tenant_id = u.tenant_id AND r.id = u.role_id
WHERE u.tenant_id = $1
ORDER BY lower(u.name);

-- name: IamGetUser :one
SELECT u.id, u.name, u.email, u.phone, u.active, u.last_login_at, u.created_at, u.role_id, r.name AS role_name
FROM users u
JOIN roles r ON r.tenant_id = u.tenant_id AND r.id = u.role_id
WHERE u.tenant_id = $1 AND u.id = $2;

-- name: IamCreateUser :one
INSERT INTO users (tenant_id, role_id, email, name, phone, password_hash)
VALUES ($1, $2, $3, $4, $5, $6)
RETURNING id;

-- name: IamUpdateUser :execrows
UPDATE users SET name = $3, phone = $4, role_id = $5, active = $6
WHERE tenant_id = $1 AND id = $2;

-- name: IamSetPassword :execrows
UPDATE users SET password_hash = $3 WHERE tenant_id = $1 AND id = $2;

-- name: IamLockActiveOwners :many
-- Mengunci baris pemilik aktif selama transaksi: dua perubahan bersamaan tidak bisa sama-sama menyisakan nol pemilik.
SELECT u.id FROM users u
JOIN roles r ON r.tenant_id = u.tenant_id AND r.id = u.role_id
WHERE u.tenant_id = $1 AND u.active AND r.is_system
FOR UPDATE OF u;
