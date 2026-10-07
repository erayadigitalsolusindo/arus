-- name: PlatformAdminByEmail :one
SELECT *
FROM platform_admins WHERE lower(email) = lower(sqlc.arg(email));

-- name: PlatformAdminByID :one
SELECT *
FROM platform_admins WHERE id = $1;

-- name: PlatformAdminList :many
SELECT id, email, name, active, last_login_at, created_at, created_by, (totp_enabled_at IS NOT NULL)::boolean AS mfa_enabled
FROM platform_admins ORDER BY created_at, id;

-- name: PlatformAdminCount :one
SELECT count(*) FROM platform_admins;

-- name: PlatformAuditInsert :exec
INSERT INTO platform_audit_log (admin_id, admin_name, action, tenant_id, tenant_name, details, ip, request_id)
VALUES (sqlc.narg(admin_id), sqlc.arg(admin_name), sqlc.arg(action), sqlc.narg(tenant_id), sqlc.arg(tenant_name),
        sqlc.arg(details), sqlc.arg(ip), sqlc.arg(request_id));

-- name: PlatformAuditList :many
SELECT id, admin_id, admin_name, action, tenant_id, tenant_name, details, ip, request_id, created_at
FROM platform_audit_log
WHERE (sqlc.narg(tenant_id)::uuid IS NULL OR tenant_id = sqlc.narg(tenant_id))
  AND (sqlc.narg(admin_id)::uuid IS NULL OR admin_id = sqlc.narg(admin_id))
  AND (sqlc.narg(action_prefix)::text IS NULL OR action LIKE sqlc.narg(action_prefix) || '%' ESCAPE '\')
  AND (sqlc.narg(before_id)::bigint IS NULL OR id < sqlc.narg(before_id))
ORDER BY id DESC
LIMIT sqlc.arg(max_rows);

-- Dijalankan di bawah WithTenant(tenant yang dituju): RLS membatasi baris ke tenant itu.
-- name: PlatformTenantGet :one
SELECT id, code, name, active, created_at FROM tenants WHERE id = $1;

-- name: PlatformTenantSetActive :execrows
UPDATE tenants SET active = sqlc.arg(active) WHERE id = sqlc.arg(id);

-- name: PlatformTenantOutlets :many
SELECT id, code, name, active FROM outlets WHERE tenant_id = $1 ORDER BY created_at, code;

-- name: PlatformTenantUsers :many
SELECT u.id, u.name, u.email, u.active, r.name AS role_name,
       u.last_login_at, (u.email_verified_at IS NOT NULL)::boolean AS email_verified, u.created_at
FROM users u JOIN roles r ON r.tenant_id = u.tenant_id AND r.id = u.role_id
WHERE u.tenant_id = $1 ORDER BY u.created_at, u.id;

-- name: PlatformRecoveryRemaining :one
SELECT count(*) FROM platform_recovery_codes WHERE admin_id = $1 AND used_at IS NULL;
