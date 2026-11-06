-- name: ProfileGet :one
SELECT u.id, u.name, u.email, u.phone, u.job_title, u.bio, u.location, u.avatar_image_id, u.banner_image_id,
       u.last_login_at, u.created_at, (u.email_verified_at IS NOT NULL)::boolean AS email_verified,
       (u.totp_enabled_at IS NOT NULL)::boolean AS mfa_enabled,
       r.name AS role_name, t.name AS tenant_name, t.code AS tenant_code,
       (SELECT count(*) FROM outlets o WHERE o.tenant_id = u.tenant_id AND o.active
          AND (r.is_system OR EXISTS (SELECT 1 FROM user_outlets uo WHERE uo.tenant_id = u.tenant_id AND uo.user_id = u.id AND uo.outlet_id = o.id)))::int AS outlet_count
FROM users u
JOIN roles r ON r.tenant_id = u.tenant_id AND r.id = u.role_id
JOIN tenants t ON t.id = u.tenant_id
WHERE u.tenant_id = @tenant_id AND u.id = @id;

-- name: ProfileUpdate :execrows
UPDATE users SET name = @name, phone = sqlc.narg('phone'), job_title = sqlc.narg('job_title'),
       bio = sqlc.narg('bio'), location = sqlc.narg('location')
WHERE tenant_id = @tenant_id AND id = @id;

-- name: ProfileImages :one
SELECT avatar_image_id, banner_image_id FROM users WHERE tenant_id = @tenant_id AND id = @id FOR UPDATE;

-- name: ProfileSetAvatar :exec
UPDATE users SET avatar_image_id = sqlc.narg('image_id') WHERE tenant_id = @tenant_id AND id = @id;

-- name: ProfileSetBanner :exec
UPDATE users SET banner_image_id = sqlc.narg('image_id') WHERE tenant_id = @tenant_id AND id = @id;

-- name: ProfileCredentials :one
SELECT password_hash FROM users WHERE tenant_id = @tenant_id AND id = @id FOR UPDATE;

-- name: ProfileSetPassword :execrows
UPDATE users SET password_hash = @password_hash, tokens_valid_after = @valid_after::timestamptz
WHERE tenant_id = @tenant_id AND id = @id;

-- name: ProfileActivity :many
-- Jejak aktivitas pengguna sendiri dari audit log (keyset created_at,id; indeks audit_log_tenant_actor_idx).
SELECT id, action, entity, entity_id, ip, created_at FROM audit_log
WHERE tenant_id = @tenant_id AND actor_id = @actor_id
  AND (sqlc.narg('before_at')::timestamptz IS NULL OR (created_at, id) < (sqlc.narg('before_at')::timestamptz, @before_id::bigint))
ORDER BY created_at DESC, id DESC
LIMIT @lim;

-- name: ProfileStats :one
SELECT count(*)::int AS actions, (count(*) FILTER (WHERE action = 'auth.login'))::int AS logins
FROM audit_log
WHERE tenant_id = @tenant_id AND actor_id = @actor_id AND created_at >= @since;
