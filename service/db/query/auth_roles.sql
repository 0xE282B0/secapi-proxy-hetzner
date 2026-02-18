-- name: UpsertAuthRole :one
INSERT INTO auth_roles (
  tenant, name, labels, spec, status
) VALUES (
  $1, $2, $3, $4, $5
)
ON CONFLICT (tenant, name) DO UPDATE
SET
  labels = EXCLUDED.labels,
  spec = EXCLUDED.spec,
  status = EXCLUDED.status,
  resource_version = auth_roles.resource_version + 1,
  deleted_at = NULL,
  updated_at = NOW()
RETURNING *;

-- name: GetAuthRole :one
SELECT *
FROM auth_roles
WHERE tenant = $1
  AND name = $2
  AND deleted_at IS NULL;

-- name: ListAuthRolesByTenant :many
SELECT *
FROM auth_roles
WHERE tenant = $1
  AND deleted_at IS NULL
ORDER BY name;

-- name: SoftDeleteAuthRole :execrows
UPDATE auth_roles
SET
  deleted_at = NOW(),
  resource_version = resource_version + 1,
  updated_at = NOW()
WHERE tenant = $1
  AND name = $2
  AND deleted_at IS NULL;
