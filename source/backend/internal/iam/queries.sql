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
SELECT u.id, u.name, u.email, u.phone, u.active, u.last_login_at, u.created_at, u.email_verified_at, u.role_id, r.name AS role_name, r.is_system AS role_is_system, r.permissions AS role_permissions
FROM users u
JOIN roles r ON r.tenant_id = u.tenant_id AND r.id = u.role_id
WHERE u.tenant_id = $1
ORDER BY lower(u.name);

-- name: IamGetUser :one
SELECT u.id, u.name, u.email, u.phone, u.active, u.last_login_at, u.created_at, u.email_verified_at, u.role_id, r.name AS role_name, r.is_system AS role_is_system, r.permissions AS role_permissions
FROM users u
JOIN roles r ON r.tenant_id = u.tenant_id AND r.id = u.role_id
WHERE u.tenant_id = $1 AND u.id = $2;

-- name: IamCreateUser :one
INSERT INTO users (tenant_id, role_id, email, name, phone, password_hash)
VALUES ($1, $2, $3, $4, $5, $6)
RETURNING id;

-- name: IamUpdateUser :execrows
-- Menonaktifkan akun juga mencabut token akses yang masih hidup (tokens_valid_after). valid_after memakai JAM APLIKASI
-- (sama dengan iat token), bukan now() DB, agar selisih jam DB-aplikasi tidak merusak perbandingan.
UPDATE users SET name = $3, phone = $4, role_id = $5, active = $6,
       tokens_valid_after = CASE WHEN users.active AND NOT $6 THEN sqlc.arg(valid_after)::timestamptz ELSE users.tokens_valid_after END
WHERE tenant_id = $1 AND id = $2;

-- name: IamSetPassword :execrows
-- Mengganti password mencabut semua token akses yang sudah terbit.
UPDATE users SET password_hash = $3, tokens_valid_after = sqlc.arg(valid_after)::timestamptz WHERE tenant_id = $1 AND id = $2;

-- name: IamLockActiveOwners :many
-- Mengunci baris pemilik aktif selama transaksi: dua perubahan bersamaan tidak bisa sama-sama menyisakan nol pemilik.
SELECT u.id FROM users u
JOIN roles r ON r.tenant_id = u.tenant_id AND r.id = u.role_id
WHERE u.tenant_id = $1 AND u.active AND r.is_system
FOR UPDATE OF u;

-- name: IamListUserOutlets :many
SELECT user_id, outlet_id FROM user_outlets WHERE tenant_id = $1;

-- name: IamGetUserOutlets :many
SELECT outlet_id FROM user_outlets WHERE tenant_id = $1 AND user_id = $2;

-- name: IamDeleteUserOutlets :exec
DELETE FROM user_outlets WHERE tenant_id = $1 AND user_id = $2;

-- name: IamAssignUserOutlet :exec
INSERT INTO user_outlets (tenant_id, user_id, outlet_id) VALUES ($1, $2, $3) ON CONFLICT DO NOTHING;

-- name: IamMarkEmailVerified :execrows
-- Verifikasi manual oleh admin; tidak menimpa waktu verifikasi yang sudah ada.
UPDATE users SET email_verified_at = now() WHERE tenant_id = $1 AND id = $2 AND email_verified_at IS NULL;
