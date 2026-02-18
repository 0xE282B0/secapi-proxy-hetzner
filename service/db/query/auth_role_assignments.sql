-- name: UpsertAuthRoleAssignment :one
INSERT INTO auth_role_assignments (
  tenant, name, labels, spec, status
) VALUES (
  $1, $2, $3, $4, $5
)
ON CONFLICT (tenant, name) DO UPDATE
SET
  labels = EXCLUDED.labels,
  spec = EXCLUDED.spec,
  status = EXCLUDED.status,
  resource_version = auth_role_assignments.resource_version + 1,
  deleted_at = NULL,
  updated_at = NOW()
RETURNING *;

-- name: GetAuthRoleAssignment :one
SELECT *
FROM auth_role_assignments
WHERE tenant = $1
  AND name = $2
  AND deleted_at IS NULL;

-- name: ListAuthRoleAssignmentsByTenant :many
SELECT *
FROM auth_role_assignments
WHERE tenant = $1
  AND deleted_at IS NULL
ORDER BY name;

-- name: SoftDeleteAuthRoleAssignment :execrows
UPDATE auth_role_assignments
SET
  deleted_at = NOW(),
  resource_version = resource_version + 1,
  updated_at = NOW()
WHERE tenant = $1
  AND name = $2
  AND deleted_at IS NULL;
