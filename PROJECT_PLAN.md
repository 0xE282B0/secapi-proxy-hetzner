# SecAPI on Hetzner: Project Plan

## 1. Goal

Build a provider service that implements the SECA (SecAPI) contract and executes operations against Hetzner Cloud.

Outcome: clients using SecAPI can manage Hetzner resources through a standard interface.

## 2. Scope (v1)

Implement Foundation APIs first:

- `Foundation/Authorization-v1` (minimal viable support)
- `Foundation/Workspace-v1`
- `Foundation/Region-v1`
- `Foundation/Compute-v1`
- `Foundation/Network-v1`
- `Foundation/Storage-v1`

Defer Extensions to later milestones:

- `Loadbalancer-v1beta1` (likely feasible)
- `Natgateway-v1beta1` (likely partial/emulated)
- `Objectstorage-v1beta1` (likely unsupported on Hetzner Cloud API)
- `Kubernetes-v1beta1` (likely unsupported)
- `Activitylog-v1beta1` (possible via action/event translation)
- `Wellknown-v1` (should be implemented early for capability discovery)

## 3. Architecture

Codebase layout:

- `cmd/secapi-proxy-hetzner`: service entrypoint
- `internal/http`: HTTP server + middleware + routing
- `internal/secapi`: request/response shaping, validation, API versioning
- `internal/provider/hetzner`: Hetzner adapter (hcloud-go client usage)
- `internal/translate`: SecAPI <-> Hetzner model translators
- `internal/state`: persistent metadata store (resource mapping + async operations)
- `internal/auth`: token handling and workspace scoping
- `internal/ops`: long-running operation tracking/reconciliation

Core design decisions:

- Keep a strict provider interface to allow future backends.
- Persist mapping records (`secapi_resource_id <-> hetzner_resource_id`) in local database.
- Support idempotency keys and reconciliation loops for eventually consistent behavior.
- Implement `Wellknown` capability endpoint to advertise supported/unsupported features.
- Use `pglite` as embedded Postgres-compatible persistence for local/runtime state.
- Use `sqlc` to generate type-safe data access from SQL queries.
- Use a migration tool (`golang-migrate`) to version and apply schema changes.

## 4. Capability mapping (first pass)

Planned mapping from SecAPI to Hetzner:

- Workspace -> provider-owned workspace context backed by token/project metadata
- Region -> Hetzner locations/datacenters
- SKU -> Hetzner server types, volume tiers, network capabilities
- Compute/Instance -> Hetzner servers
- Storage/BlockStorage -> Hetzner volumes
- Storage/Image -> Hetzner images
- Network/VNet + Subnet -> Hetzner networks/subnets
- Network/SecurityGroup -> Hetzner firewalls
- Network/PublicIP -> Hetzner primary IPs / floating IPs
- LoadBalancer extension -> Hetzner load balancers (later milestone)

Known gaps to handle explicitly:

- Full SecAPI RBAC semantics may exceed Hetzner token model.
- Route tables and advanced networking might need partial support.
- NAT gateway may require instance-based emulation.
- Object storage and managed Kubernetes are not native Hetzner Cloud API primitives.

## 5. Phased delivery plan

## Phase 0: Project bootstrap (1 week)

1. Initialize Go module, toolchain, lints, formatting, CI workflow.
2. Add OpenAPI-driven contract checks against SecAPI spec artifacts.
3. Create provider abstraction and stub Hetzner adapter.
4. Add local dev config (`HETZNER_TOKEN`, endpoint, logging level).
5. Add persistence stack bootstrap (`pglite` init, `golang-migrate` migrations, `sqlc generate`).

Exit criteria:

- Service starts, health endpoint works, CI passes, contract test scaffold exists.

## Phase 1: Read-only foundation (1-2 weeks)

1. Implement `Wellknown` and capability declaration.
2. Implement `Region` list/get via Hetzner location/datacenter APIs.
3. Implement read-only `SKU` and image catalog endpoints.
4. Add error translation layer (Hetzner API -> SecAPI error shapes).

Exit criteria:

- SecAPI clients can discover supported capabilities, regions, and SKUs.

## Phase 2: Compute + Storage CRUD (2-3 weeks)

1. Implement instance create/get/list/delete/start/stop/reboot.
2. Implement volume create/attach/detach/list/delete.
3. Implement image read/list and supported image operations.
4. Persist resource mapping and operation records in DB.

Exit criteria:

- End-to-end compute/storage lifecycle works through SecAPI endpoints.

## Phase 3: Network core CRUD (2-3 weeks)

1. Implement network and subnet lifecycle.
2. Implement security groups and rules via firewalls.
3. Implement public IP lifecycle and attach/detach.
4. Document partial/unsupported route-table semantics (if needed).

Exit criteria:

- End-to-end network lifecycle works for common workloads.

## Phase 4: Workspace + Authorization minimum support (1-2 weeks)

1. Define workspace tenancy model (single-token, token-per-workspace, or delegated gateway).
2. Implement workspace CRUD metadata.
3. Implement minimal role/assignment semantics compatible with provider constraints.
4. Add explicit compatibility matrix for unsupported RBAC features.

Exit criteria:

- Multi-workspace separation and minimal authz behavior are operational.

## Phase 5: Extensions + hardening (2-4 weeks)

1. Implement load balancer extension.
2. Add activity log adapter (if feasible from Hetzner actions).
3. Add optional NAT emulation strategy, if required.
4. Production hardening: retries, rate limiting, tracing, metrics, audit logs.

Exit criteria:

- Stable beta release with documented extension support matrix.

## 6. Testing strategy

1. Unit tests for translators (SecAPI model <-> Hetzner model).
2. Contract tests against SecAPI OpenAPI definitions.
3. Integration tests with mocked hcloud-go and optional live Hetzner project.
4. Golden tests for error and operation state mappings.
5. Conformance-like test suite: create/list/get/delete for each supported resource type.

## 7. Operations and deployment

1. Containerize service (`Dockerfile`) with minimal runtime image.
2. Provide Helm chart or Terraform module for deployment.
3. Add configuration for secret management of Hetzner tokens.
4. Add SLOs, dashboards, alerts for API latency and provider error rates.
5. Define upgrade strategy for spec changes and backward compatibility.

## 8. Risks and mitigations

1. Spec/provider mismatch: keep a capability matrix and return explicit unsupported errors.
2. Async behavior differences: normalize through operation state machine.
3. Rate limits and transient failures: use exponential backoff + circuit breaking.
4. Security model mismatch: document trust boundaries and minimum viable authz contract.

## 9. Immediate next implementation tasks

1. Create skeleton service and provider interfaces.
2. Implement `Wellknown` and `Region` endpoints first.
3. Add translation package and golden tests.
4. Add `pglite`-backed mapping store for resource IDs and operation tracking.
5. Add migration scripts and generated `sqlc` query package for persistence access.
6. Add first live integration test for region listing with Hetzner token.
