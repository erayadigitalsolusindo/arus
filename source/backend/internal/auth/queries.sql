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
