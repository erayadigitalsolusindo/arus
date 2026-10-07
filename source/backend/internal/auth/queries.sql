-- name: CreateTenant :one
INSERT INTO tenants (code, name) VALUES ($1, $2)
RETURNING id, code, name;

-- name: CreateOutlet :one
INSERT INTO outlets (tenant_id, code, name) VALUES ($1, $2, $3)
RETURNING id, code, name;

-- name: CreateRole :one
INSERT INTO roles (tenant_id, name, permissions, is_system) VALUES ($1, $2, $3, true)
RETURNING id, name;

-- name: CreateUser :one
INSERT INTO users (tenant_id, role_id, email, name, phone, password_hash)
VALUES ($1, $2, $3, $4, $5, $6)
RETURNING id, email, name;

-- name: GetAccountByEmail :one
-- Akun + role + tenant + outlet aktif pertama, untuk login. Email dicocokkan case-insensitive (users_email_key).
SELECT u.id AS user_id, u.name AS user_name, u.email, u.password_hash, u.active AS user_active,
       r.name AS role_name,
       t.id AS tenant_id, t.code AS tenant_code, t.name AS tenant_name, t.active AS tenant_active,
       o.id AS outlet_id, o.code AS outlet_code, o.name AS outlet_name
FROM users u
JOIN roles r   ON r.tenant_id = u.tenant_id AND r.id = u.role_id
JOIN tenants t ON t.id = u.tenant_id
JOIN outlets o ON o.tenant_id = u.tenant_id AND o.active
WHERE lower(u.email) = lower($1)
ORDER BY o.created_at, o.code
LIMIT 1;

-- name: GetAccountByID :one
-- Sama seperti GetAccountByEmail, dicari lewat id user (refresh token, /auth/me).
SELECT u.id AS user_id, u.name AS user_name, u.email, u.password_hash, u.active AS user_active,
       r.name AS role_name,
       t.id AS tenant_id, t.code AS tenant_code, t.name AS tenant_name, t.active AS tenant_active,
       o.id AS outlet_id, o.code AS outlet_code, o.name AS outlet_name
FROM users u
JOIN roles r   ON r.tenant_id = u.tenant_id AND r.id = u.role_id
JOIN tenants t ON t.id = u.tenant_id
JOIN outlets o ON o.tenant_id = u.tenant_id AND o.active
WHERE u.id = $1
ORDER BY o.created_at, o.code
LIMIT 1;

-- name: TouchLastLogin :exec
UPDATE users SET last_login_at = now() WHERE tenant_id = $1 AND id = $2;

-- name: GetAppSetting :one
SELECT value FROM app_settings WHERE key = $1;
