-- name: CreateResourceBinding :one
INSERT INTO resource_bindings (
  tenant, workspace, kind, seca_ref, provider_ref, status
) VALUES (
  $1, $2, $3, $4, $5, $6
)
RETURNING *;

-- name: GetResourceBindingBySecaRef :one
SELECT *
FROM resource_bindings
WHERE seca_ref = $1;
