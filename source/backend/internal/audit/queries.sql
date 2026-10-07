-- name: AuditInsert :exec
INSERT INTO audit_log (tenant_id, outlet_id, actor_id, actor_name, action, entity, entity_id, details, ip, request_id)
VALUES (sqlc.arg(tenant_id), sqlc.narg(outlet_id), sqlc.narg(actor_id), sqlc.arg(actor_name), sqlc.arg(action),
        sqlc.arg(entity), sqlc.arg(entity_id), sqlc.arg(details), sqlc.arg(ip), sqlc.arg(request_id));

-- name: AuditList :many
-- Daftar terbaru dulu dengan keyset pagination (created_at, id). Semua filter opsional; `action_prefix` sudah di-escape pemanggil.
SELECT id, outlet_id, actor_id, actor_name, action, entity, entity_id, details, ip, request_id, created_at
FROM audit_log
WHERE tenant_id = sqlc.arg(tenant_id)
  AND (sqlc.narg(entity)::text IS NULL OR entity = sqlc.narg(entity))
  AND (sqlc.narg(entity_id)::text IS NULL OR entity_id = sqlc.narg(entity_id))
  AND (sqlc.narg(action_prefix)::text IS NULL OR action LIKE sqlc.narg(action_prefix) || '%' ESCAPE '\')
  AND (sqlc.narg(actor_id)::uuid IS NULL OR actor_id = sqlc.narg(actor_id))
  AND (sqlc.narg(from_at)::timestamptz IS NULL OR created_at >= sqlc.narg(from_at))
  AND (sqlc.narg(to_at)::timestamptz IS NULL OR created_at < sqlc.narg(to_at))
  AND (sqlc.narg(cursor_at)::timestamptz IS NULL OR (created_at, id) < (sqlc.narg(cursor_at)::timestamptz, sqlc.narg(cursor_id)::bigint))
ORDER BY created_at DESC, id DESC
LIMIT sqlc.arg(max_rows);
