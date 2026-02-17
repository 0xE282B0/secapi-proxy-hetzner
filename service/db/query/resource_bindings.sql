-- name: CreateResourceBinding :one
INSERT INTO resource_bindings (
  tenant, workspace, kind, seca_ref, provider_ref, status
) VALUES (
  $1, $2, $3, $4, $5, $6
)
RETURNING *;

-- name: UpsertResourceBinding :one
INSERT INTO resource_bindings (
  tenant, workspace, kind, seca_ref, provider_ref, status
) VALUES (
  $1, $2, $3, $4, $5, $6
)
ON CONFLICT (seca_ref) DO UPDATE
SET
  provider_ref = EXCLUDED.provider_ref,
  status = EXCLUDED.status,
  updated_at = NOW()
RETURNING *;

-- name: GetResourceBindingBySecaRef :one
SELECT *
FROM resource_bindings
WHERE seca_ref = $1;

-- name: ListResourceBindingsByScopeAndKind :many
SELECT *
FROM resource_bindings
WHERE tenant = $1
  AND workspace = $2
  AND kind = $3
ORDER BY seca_ref;

-- name: DeleteResourceBindingBySecaRef :exec
DELETE FROM resource_bindings
WHERE seca_ref = $1;
