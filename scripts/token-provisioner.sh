#!/usr/bin/env bash
set -euo pipefail

if ! command -v curl >/dev/null 2>&1; then
  echo "error: curl is required" >&2
  exit 1
fi

if ! command -v jq >/dev/null 2>&1; then
  echo "error: jq is required" >&2
  exit 1
fi

if [ -z "${SECA_TOKEN_PROVISIONER_HETZNER_TOKEN:-}" ]; then
  echo "error: SECA_TOKEN_PROVISIONER_HETZNER_TOKEN is required" >&2
  exit 1
fi

if [ -z "${SECA_ADMIN_TOKEN:-}" ]; then
  echo "error: SECA_ADMIN_TOKEN is required" >&2
  exit 1
fi

SECA_PUBLIC_BASE_URL="${SECA_PUBLIC_BASE_URL:-http://localhost:8080}"
SECA_ADMIN_BASE_URL="${SECA_ADMIN_BASE_URL:-http://127.0.0.1:8081}"
SECA_TENANTS="${SECA_TENANTS:-dev}"
SECA_TOKEN_PROVISIONER_INTERVAL="${SECA_TOKEN_PROVISIONER_INTERVAL:-1}"

log() {
  printf '[token-provisioner] %s\n' "$*" >&2
}

build_payload() {
  local payload
  payload="$(jq -nc --arg token "$SECA_TOKEN_PROVISIONER_HETZNER_TOKEN" '{apiToken:$token}')"

  if [ -n "${HCLOUD_ENDPOINT:-}" ]; then
    payload="$(printf '%s' "$payload" | jq --arg v "$HCLOUD_ENDPOINT" '.apiEndpoint = $v')"
  fi
  if [ -n "${HCLOUD_PROJECT_REF:-}" ]; then
    payload="$(printf '%s' "$payload" | jq --arg v "$HCLOUD_PROJECT_REF" '.projectRef = $v')"
  fi

  printf '%s' "$payload"
}

bind_workspace() {
  local tenant="$1"
  local workspace="$2"
  local payload="$3"
  local url="${SECA_ADMIN_BASE_URL}/admin/v1/tenants/${tenant}/workspaces/${workspace}/providers/hetzner"
  local result body code

  result="$(
    curl -sS -X PUT "$url" \
      -H "authorization: Bearer ${SECA_ADMIN_TOKEN}" \
      -H "content-type: application/json" \
      --data "$payload" \
      -w '\n%{http_code}'
  )"

  body="${result%$'\n'*}"
  code="${result##*$'\n'}"

  if [ "$code" = "200" ]; then
    log "activated tenant=${tenant} workspace=${workspace}"
    return 0
  fi

  log "bind failed tenant=${tenant} workspace=${workspace} status=${code} body=${body}"
  return 1
}

poll_once() {
  local payload tenant url list workspaces workspace
  payload="$(build_payload)"

  OLDIFS="$IFS"
  IFS=','
  for tenant in $SECA_TENANTS; do
    tenant="$(printf '%s' "$tenant" | xargs)"
    [ -n "$tenant" ] || continue

    url="${SECA_PUBLIC_BASE_URL}/workspace/v1/tenants/${tenant}/workspaces"
    if ! list="$(curl -fsS "$url" 2>/dev/null)"; then
      log "list failed tenant=${tenant}"
      continue
    fi

    workspaces="$(printf '%s' "$list" | jq -r '.items[]? | select(.status.state == "creating") | .metadata.name')"
    if [ -z "$workspaces" ]; then
      continue
    fi

    while IFS= read -r workspace; do
      [ -n "$workspace" ] || continue
      bind_workspace "$tenant" "$workspace" "$payload" || true
    done <<EOF
$workspaces
EOF
  done
  IFS="$OLDIFS"
}

log "starting (public=${SECA_PUBLIC_BASE_URL} admin=${SECA_ADMIN_BASE_URL} tenants=${SECA_TENANTS} interval=${SECA_TOKEN_PROVISIONER_INTERVAL}s)"
while true; do
  poll_once
  sleep "$SECA_TOKEN_PROVISIONER_INTERVAL"
done
