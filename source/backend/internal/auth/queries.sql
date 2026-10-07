-- Pencarian akun sebelum tenant diketahui (login, refresh) memakai fungsi SECURITY DEFINER dan ditulis
-- manual di accounts.go: sqlc tidak membaca tipe hasil fungsi RETURNS TABLE.

-- name: CreateTenant :one
INSERT INTO tenants (id, code, name) VALUES ($1, $2, $3)
RETURNING id, code, name;

-- name: CreateOutlet :one
INSERT INTO outlets (tenant_id, code, name) VALUES ($1, $2, $3)
RETURNING id, code, name;

-- name: CreateRole :one
INSERT INTO roles (tenant_id, name, permissions, is_system) VALUES ($1, $2, $3, true)
RETURNING id, name;

-- name: CreateUser :one
INSERT INTO users (tenant_id, role_id, email, name, phone, password_hash, terms_accepted_at, terms_version)
VALUES ($1, $2, $3, $4, $5, $6, now(), $7)
RETURNING id, email, name;

-- name: TouchLastLogin :exec
UPDATE users SET last_login_at = now() WHERE tenant_id = $1 AND id = $2;

-- name: AuthSetPassword :execrows
-- Dipakai alur lupa password: mengganti password, mencabut token akses yang masih hidup, dan menandai email terverifikasi
-- (pemakai tautan di email terbukti memiliki alamatnya).
UPDATE users SET password_hash = $3, tokens_valid_after = sqlc.arg(valid_after)::timestamptz, email_verified_at = COALESCE(email_verified_at, now())
WHERE tenant_id = $1 AND id = $2;

-- name: AuthMarkEmailVerified :execrows
UPDATE users SET email_verified_at = COALESCE(email_verified_at, now()) WHERE tenant_id = $1 AND id = $2;

-- name: GetAppSetting :one
SELECT value FROM app_settings WHERE key = $1;
