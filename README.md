# secapi-proxy-hetzner

Phase 0 scaffold for a SecAPI-compatible proxy backed by Hetzner APIs.

## Local development

```bash
make bootstrap
make fmt
make test
make build
```

Run server:

```bash
SECA_DATABASE_URL='postgres://postgres:postgres@localhost:5432/secapi_proxy?sslmode=disable' \
HCLOUD_TOKEN='your-token' \
SECA_PUBLIC_BASE_URL='http://localhost:8080' \
make run
```

Health endpoints:

- `GET /healthz`
- `GET /readyz`
- `GET /.wellknown/secapi`
- `GET /v1/regions`
- `GET /v1/regions/{name}`
- `GET /compute/v1/tenants/{tenant}/skus`
- `GET /compute/v1/tenants/{tenant}/skus/{name}`
- `GET /storage/v1/tenants/{tenant}/images`
- `GET /storage/v1/tenants/{tenant}/images/{name}`
- `GET /compute/v1/tenants/{tenant}/workspaces/{workspace}/instances`
- `GET|PUT|DELETE /compute/v1/tenants/{tenant}/workspaces/{workspace}/instances/{name}`
- `POST /compute/v1/tenants/{tenant}/workspaces/{workspace}/instances/{name}/start`
- `POST /compute/v1/tenants/{tenant}/workspaces/{workspace}/instances/{name}/stop`
- `POST /compute/v1/tenants/{tenant}/workspaces/{workspace}/instances/{name}/restart`
- `GET /storage/v1/tenants/{tenant}/workspaces/{workspace}/block-storages`
- `GET|PUT|DELETE /storage/v1/tenants/{tenant}/workspaces/{workspace}/block-storages/{name}`
- `POST /storage/v1/tenants/{tenant}/workspaces/{workspace}/block-storages/{name}/attach`
- `POST /storage/v1/tenants/{tenant}/workspaces/{workspace}/block-storages/{name}/detach`

## Hetzner token setup

1. Open your Hetzner Cloud project in the console.
2. Go to `Security` -> `API Tokens`.
3. Create a token for your dev project.
4. Export it before running the service:

```bash
export HCLOUD_TOKEN='<token-from-hetzner-console>'
```

Optional endpoint overrides:

```bash
export HCLOUD_ENDPOINT='https://api.hetzner.cloud/v1'
export HCLOUD_HETZNER_ENDPOINT='https://api.hetzner.com/v1'
```

## Persistence stack

- Embedded/local target: `pglite` (Postgres-compatible)
- Schema migrations: `golang-migrate`
- Query code generation: `sqlc`

Commands:

```bash
make migrate-up
make migrate-down
make sqlc-gen
```

## CI/CD mapping to Make targets

- Verify: `make ci-verify`
- Unit: `make ci-unit`
- Integration: `make ci-integration`
- Contract: `make ci-contract`
- Package: `make ci-package`

## Docker

```bash
make docker-build
make docker-run
```

## Docker Compose

Start Postgres + migrations + API service:

```bash
export HCLOUD_TOKEN='<token-from-hetzner-console>'
docker compose up --build
```

Check service:

```bash
curl -s http://localhost:8080/healthz
curl -s http://localhost:8080/v1/regions
curl -s http://localhost:8080/compute/v1/tenants/dev/skus
curl -s http://localhost:8080/storage/v1/tenants/dev/images
```

Run full Phase 1 smoke test:

```bash
make phase1-smoke
```

Run Phase 2 smoke checks:

```bash
make phase2-smoke
```
