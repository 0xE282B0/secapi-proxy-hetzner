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
SECA_DATABASE_URL='postgres://postgres:postgres@localhost:5432/secapi_proxy?sslmode=disable' make run
```

Health endpoints:

- `GET /healthz`
- `GET /readyz`

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
