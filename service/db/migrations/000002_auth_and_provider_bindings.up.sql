CREATE TABLE IF NOT EXISTS auth_roles (
  id BIGSERIAL PRIMARY KEY,
  tenant TEXT NOT NULL,
  name TEXT NOT NULL,
  labels JSONB NOT NULL DEFAULT '{}'::jsonb,
  spec JSONB NOT NULL,
  status JSONB NOT NULL,
  resource_version BIGINT NOT NULL DEFAULT 1,
  deleted_at TIMESTAMPTZ,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  UNIQUE (tenant, name)
);

CREATE INDEX IF NOT EXISTS auth_roles_tenant_active_idx
  ON auth_roles (tenant, name)
  WHERE deleted_at IS NULL;

CREATE TABLE IF NOT EXISTS auth_role_assignments (
  id BIGSERIAL PRIMARY KEY,
  tenant TEXT NOT NULL,
  name TEXT NOT NULL,
  labels JSONB NOT NULL DEFAULT '{}'::jsonb,
  spec JSONB NOT NULL,
  status JSONB NOT NULL,
  resource_version BIGINT NOT NULL DEFAULT 1,
  deleted_at TIMESTAMPTZ,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  UNIQUE (tenant, name)
);

CREATE INDEX IF NOT EXISTS auth_role_assignments_tenant_active_idx
  ON auth_role_assignments (tenant, name)
  WHERE deleted_at IS NULL;

CREATE TABLE IF NOT EXISTS workspace_provider_credentials (
  id BIGSERIAL PRIMARY KEY,
  tenant TEXT NOT NULL,
  workspace TEXT NOT NULL,
  provider TEXT NOT NULL,
  project_ref TEXT,
  api_endpoint TEXT,
  api_token_encrypted TEXT NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  deleted_at TIMESTAMPTZ,
  UNIQUE (tenant, workspace, provider)
);

CREATE INDEX IF NOT EXISTS workspace_provider_credentials_active_idx
  ON workspace_provider_credentials (tenant, workspace, provider)
  WHERE deleted_at IS NULL;
