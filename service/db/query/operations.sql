-- name: CreateOperation :one
INSERT INTO operations (
  operation_id, seca_ref, provider_action_id, phase, error_text
) VALUES (
  $1, $2, $3, $4, $5
)
RETURNING *;

-- name: GetOperationByID :one
SELECT *
FROM operations
WHERE operation_id = $1;

-- name: ListOperationsBySecaRef :many
SELECT *
FROM operations
WHERE seca_ref = $1
ORDER BY created_at DESC;
