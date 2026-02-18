-- name: UpsertWorkspaceProviderCredential :one
INSERT INTO workspace_provider_credentials (
  tenant, workspace, provider, project_ref, api_endpoint, api_token_encrypted
) VALUES (
  $1, $2, $3, $4, $5, $6
)
ON CONFLICT (tenant, workspace, provider) DO UPDATE SET
  project_ref = EXCLUDED.project_ref,
  api_endpoint = EXCLUDED.api_endpoint,
  api_token_encrypted = EXCLUDED.api_token_encrypted,
  deleted_at = NULL,
  updated_at = NOW()
RETURNING *;

-- name: GetWorkspaceProviderCredential :one
SELECT *
FROM workspace_provider_credentials
WHERE tenant = $1
  AND workspace = $2
  AND provider = $3
  AND deleted_at IS NULL
LIMIT 1;

-- name: SoftDeleteWorkspaceProviderCredential :execrows
UPDATE workspace_provider_credentials
SET deleted_at = NOW(),
    updated_at = NOW()
WHERE tenant = $1
  AND workspace = $2
  AND provider = $3
  AND deleted_at IS NULL;
