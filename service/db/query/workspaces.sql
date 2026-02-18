-- name: UpsertWorkspace :one
INSERT INTO workspaces (
  tenant, name, region, labels, spec, status
) VALUES (
  $1, $2, $3, $4, $5, $6
)
ON CONFLICT (tenant, name) DO UPDATE SET
  region = EXCLUDED.region,
  labels = EXCLUDED.labels,
  spec = EXCLUDED.spec,
  status = EXCLUDED.status,
  resource_version = workspaces.resource_version + 1,
  deleted_at = NULL,
  updated_at = NOW()
RETURNING *;

-- name: GetWorkspace :one
SELECT *
FROM workspaces
WHERE tenant = $1
  AND name = $2
  AND deleted_at IS NULL
LIMIT 1;

-- name: ListWorkspacesByTenant :many
SELECT *
FROM workspaces
WHERE tenant = $1
  AND deleted_at IS NULL
ORDER BY name ASC;

-- name: SoftDeleteWorkspace :execrows
UPDATE workspaces
SET deleted_at = NOW(),
    updated_at = NOW()
WHERE tenant = $1
  AND name = $2
  AND deleted_at IS NULL;
