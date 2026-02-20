# SecAPI Proxy Hetzner — Current Project Plan

## 1. Objective
Ship a production-credible SECA proxy on Hetzner where conformance passes are not based on test-only shortcuts, and core networking semantics behave predictably for real workloads.

## 2. Current baseline
- `make conformance-full` passes in current development flow.
- Workspace-scoped credentials are the active model (no global Hetzner token fallback for runtime operations).
- Network resources are persisted as SECA abstractions where Hetzner has no direct equivalent.
- Internet gateway now has an opt-in provider-backed implementation (`SECA_INTERNET_GATEWAY_NAT_VM=true`):
  - managed NAT VM lifecycle
  - network attachment sync
  - route-table-triggered reconcile
  - Hetzner network route programming (`destination -> IGW private IP`)

## 3. What is done
1. Provider-backed `networks` + persistent SECA overlays (`routeTableRef`, metadata shaping).
2. Persistent abstractions for `route-tables`, `subnets`, `nics`, `public-ips`.
3. Provider-backed `security-groups` via Hetzner firewalls.
4. Provider-backed (opt-in) `internet-gateways` via managed NAT VM.
5. Route-table updates now trigger:
   - IGW reconcile
   - Hetzner route upsert/delete for IGW targets.
6. Tracing labels added to provider-managed resources for cleanup/debug.

## 4. Remaining product gaps (highest priority)
1. Guest readiness for private-only workload instances:
   - default route and DNS are not always automatically usable in tested environments.
   - real-world NAT worked only after manual guest route adjustment.
2. Stronger e2e/integration coverage for route synchronization and IGW lifecycle edge cases.
3. Better observability and troubleshooting outputs around route/IGW reconciliation.

## 5. Execution plan

### Phase A — Workload egress readiness
1. Define expected behavior for private-only instance boot networking under SECA.
2. Implement deterministic route/DNS bootstrap strategy for created instances (without breaking existing conformance behavior).
3. Add regression tests for private-only egress reachability assumptions.

Exit criteria:
- Private-only instance can consistently reach internet through IGW/NAT path without manual post-boot host fixes.

### Phase B — Route synchronization hardening
1. Add integration-style tests for:
   - route add -> Hetzner route created
   - route change -> Hetzner route updated
   - route remove/delete -> Hetzner route deleted
2. Validate reconcile behavior under repeated idempotent PUTs and mixed route targets.
3. Ensure stale route cleanup on IGW removal and route-table deletion remains safe.

Exit criteria:
- Route-table and Hetzner route state remain converged under retries and updates.

### Phase C — Observability + operability
1. Add structured logs for:
   - IGW reconcile actions
   - route upsert/delete decisions
   - gateway IP resolution failures
2. Add basic counters/metrics hooks for reconcile successes/failures.
3. Document runbook for e2e NAT diagnostics (`hcloud` + `curl` + guest checks).

Exit criteria:
- Operational debugging does not require code inspection for common network failures.

## 6. Guardrails
1. Keep non-SECA custom endpoints out of public API surface.
2. Keep test-mode behavior explicitly gated and documented.
3. Prefer provider-backed behavior over conformance-only emulation when feasible.
4. Keep changes reversible with clear feature flags where risk is higher.

## 7. Immediate next tasks
1. Implement and test private-only instance route/DNS bootstrap behavior.
2. Add route sync integration tests (IGW target path).
3. Run:
   - `go test ./...` (service)
   - `make conformance-full`
   - real Hetzner IGW/NAT e2e checklist
