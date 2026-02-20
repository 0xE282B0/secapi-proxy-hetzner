# secapi-proxy-hetzner

SECA-compatible proxy for Hetzner Cloud.

Current focus is practical conformance plus real provider behavior for core resources, including an opt-in internet-gateway implementation backed by a managed NAT VM.

## Quick start

```bash
make bootstrap
make fmt
make test
make build
```

Run locally:

```bash
SECA_DATABASE_URL='postgres://postgres:postgres@localhost:5432/secapi_proxy?sslmode=disable' \
SECA_PUBLIC_BASE_URL='http://localhost:8080' \
SECA_ADMIN_LISTEN_ADDR='127.0.0.1:8081' \
SECA_ADMIN_TOKEN='dev-admin-token' \
SECA_CREDENTIALS_KEY='MDEyMzQ1Njc4OWFiY2RlZjAxMjM0NTY3ODlhYmNkZWY=' \
make run
```

Health:

- `GET /healthz`
- `GET /readyz`
- `GET /.wellknown/secapi`

## Docker compose

```bash
docker compose up --build
```

Useful checks:

```bash
curl -s http://localhost:8080/healthz | jq
curl -s http://localhost:8080/v1/regions | jq
curl -s http://localhost:8080/compute/v1/tenants/dev/skus | jq
curl -s http://localhost:8080/storage/v1/tenants/dev/images | jq
```

## Credential model

- No global Hetzner token is used for runtime resource operations.
- Tokens are workspace-scoped and persisted via admin binding.
- `SECA_ADMIN_TOKEN` secures `/admin/v1/...` endpoints.
- `SECA_CREDENTIALS_KEY` is required for at-rest credential encryption.

## Token provisioner (local/conformance)

Use token-provisioner so newly created workspaces in `creating` become usable:

```bash
export SECA_TOKEN_PROVISIONER_HETZNER_TOKEN='<token-from-hetzner-console>'
export SECA_ADMIN_TOKEN='dev-admin-token'
export SECA_PUBLIC_BASE_URL='http://localhost:8080'
export SECA_ADMIN_BASE_URL='http://127.0.0.1:8081'
export SECA_TENANTS='dev'
./scripts/token-provisioner.sh
```

Optional:

- `SECA_TOKEN_PROVISIONER_INTERVAL` (default `1`)
- `HCLOUD_ENDPOINT`
- `HCLOUD_PROJECT_REF`

## Key runtime env vars

- `SECA_ADMIN_TOKEN`
- `SECA_CREDENTIALS_KEY`
- `SECA_LISTEN_ADDR` (default `:8080`)
- `SECA_ADMIN_LISTEN_ADDR` (default `127.0.0.1:8081`)
- `SECA_PUBLIC_BASE_URL` (default `http://localhost:8080`)
- `SECA_DATABASE_URL`
- `SECA_CONFORMANCE_MODE` (bool)
- `SECA_HETZNER_AVAILABILITY_CACHE_TTL` (default `60s`; set `0s` to disable cache)
- `SECA_INTERNET_GATEWAY_NAT_VM` (default `false`)
- `HCLOUD_ENDPOINT`
- `HCLOUD_HETZNER_ENDPOINT`

## Internet gateway (opt-in)

Enable:

```bash
export SECA_INTERNET_GATEWAY_NAT_VM=true
```

Behavior when enabled:

- creates one managed Hetzner VM per SECA internet-gateway
- applies cloud-init to enable IPv4 forwarding + SNAT rules
- syncs network attachments from route-table usage
- programs Hetzner network routes (`destination -> IGW private IP`)
- removes managed VM when no route-table references remain

Notes:

- this is a provider polyfill (Hetzner has no native internet-gateway resource)
- for private-only workload instances, guest default route/DNS behavior may still require explicit handling depending on image/network stack

## Examples

### Internet gateway e2e

```bash
./examples/internet-gateway-e2e.sh
```

This script creates:

1. workspace
2. network
3. internet-gateway
4. route-table default route to internet-gateway

Then prints resulting resources for inspection.

## Conformance

Official runner:

- `https://github.com/eu-sovereign-cloud/conformance`

Targets:

```bash
make conformance-smoke
make conformance-region
make conformance-auth
make conformance-workspace
make conformance-compute
make conformance-storage
make conformance-network
make conformance-foundation
make conformance-foundation-core
make conformance-full
```

Example override:

```bash
make conformance-smoke \
  CONFORMANCE_PROVIDER_REGION_V1='http://localhost:8080' \
  CONFORMANCE_PROVIDER_AUTHORIZATION_V1='http://localhost:8080' \
  CONFORMANCE_CLIENT_AUTH_TOKEN='dev-token' \
  CONFORMANCE_CLIENT_TENANT='dev' \
  CONFORMANCE_CLIENT_REGION='fsn1'
```

Results path:

- `.artifacts/conformance/results`

## CI / Dev commands

- `make ci-verify`
- `make ci-unit`
- `make ci-integration`
- `make ci-contract`
- `make ci-package`
- `make migrate-up`
- `make migrate-down`
- `make sqlc-gen`
