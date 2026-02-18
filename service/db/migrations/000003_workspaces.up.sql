CREATE TABLE IF NOT EXISTS workspaces (
  id BIGSERIAL PRIMARY KEY,
  tenant TEXT NOT NULL,
  name TEXT NOT NULL,
  region TEXT NOT NULL,
  labels JSONB NOT NULL DEFAULT '{}'::jsonb,
  spec JSONB NOT NULL DEFAULT '{}'::jsonb,
  status JSONB NOT NULL DEFAULT '{"state":"creating"}'::jsonb,
  resource_version BIGINT NOT NULL DEFAULT 1,
  deleted_at TIMESTAMPTZ,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  UNIQUE (tenant, name)
);

CREATE INDEX IF NOT EXISTS workspaces_tenant_active_idx
  ON workspaces (tenant, name)
  WHERE deleted_at IS NULL;
