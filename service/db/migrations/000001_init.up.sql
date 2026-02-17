CREATE TABLE IF NOT EXISTS resource_bindings (
  id BIGSERIAL PRIMARY KEY,
  tenant TEXT NOT NULL,
  workspace TEXT NOT NULL,
  kind TEXT NOT NULL,
  seca_ref TEXT NOT NULL UNIQUE,
  provider_ref TEXT NOT NULL,
  status TEXT NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS resource_bindings_scope_idx
  ON resource_bindings (tenant, workspace, kind);

CREATE TABLE IF NOT EXISTS operations (
  id BIGSERIAL PRIMARY KEY,
  operation_id TEXT NOT NULL UNIQUE,
  seca_ref TEXT NOT NULL,
  provider_action_id TEXT,
  phase TEXT NOT NULL,
  error_text TEXT,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
