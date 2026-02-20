#!/usr/bin/env bash
set -euo pipefail

# End-to-end internet-gateway example for local dev.
# Prerequisites:
# - API running at $BASE (default: http://localhost:8080)
# - token-provisioner running (workspace must become active)
# - SECA_INTERNET_GATEWAY_NAT_VM=true in service runtime

BASE="${BASE:-http://localhost:8080}"
TENANT="${TENANT:-dev}"
REGION="${REGION:-nbg1}"

WS="${WS:-igw-e2e-$RANDOM}"
NET="${NET:-net-$RANDOM}"
RT="${RT:-rt-$RANDOM}"
IGW="${IGW:-igw-$RANDOM}"

echo "Using:"
echo "  BASE=$BASE"
echo "  TENANT=$TENANT"
echo "  REGION=$REGION"
echo "  WS=$WS"
echo "  NET=$NET"
echo "  RT=$RT"
echo "  IGW=$IGW"

echo "==> Create workspace"
curl -fsS -X PUT "$BASE/workspace/v1/tenants/$TENANT/workspaces/$WS" \
  -H 'content-type: application/json' \
  -d "{\"metadata\":{\"name\":\"$WS\",\"tenant\":\"$TENANT\",\"region\":\"$REGION\"},\"spec\":{}}" | jq

echo "==> Wait for workspace to become active (token-provisioner)"
for _ in $(seq 1 60); do
  state="$(curl -fsS "$BASE/workspace/v1/tenants/$TENANT/workspaces/$WS" | jq -r '.status.state')"
  if [[ "$state" == "active" ]]; then
    break
  fi
  sleep 1
done
if [[ "${state:-}" != "active" ]]; then
  echo "workspace did not become active in time (last state=${state:-unknown})" >&2
  exit 1
fi

echo "==> Create network"
curl -fsS -X PUT "$BASE/network/v1/tenants/$TENANT/workspaces/$WS/networks/$NET" \
  -H 'content-type: application/json' \
  -d "{\"metadata\":{\"name\":\"$NET\",\"tenant\":\"$TENANT\",\"workspace\":\"$WS\",\"region\":\"$REGION\"},\"spec\":{\"cidr\":{\"ipv4\":\"10.10.0.0/16\"},\"routeTableRef\":\"route-tables/$RT\",\"skuRef\":\"skus/hcloud-network\"}}" | jq

echo "==> Create internet gateway"
curl -fsS -X PUT "$BASE/network/v1/tenants/$TENANT/workspaces/$WS/internet-gateways/$IGW" \
  -H 'content-type: application/json' \
  -d "{\"metadata\":{\"name\":\"$IGW\",\"tenant\":\"$TENANT\",\"workspace\":\"$WS\",\"region\":\"$REGION\"},\"spec\":{\"egressOnly\":true}}" | jq

echo "==> Attach route-table default route to internet gateway"
curl -fsS -X PUT "$BASE/network/v1/tenants/$TENANT/workspaces/$WS/networks/$NET/route-tables/$RT" \
  -H 'content-type: application/json' \
  -d "{\"metadata\":{\"name\":\"$RT\",\"tenant\":\"$TENANT\",\"workspace\":\"$WS\",\"network\":\"$NET\",\"region\":\"$REGION\"},\"spec\":{\"routes\":[{\"destinationCidrBlock\":\"0.0.0.0/0\",\"targetRef\":\"internet-gateways/$IGW\"}]}}" | jq

echo "==> Show resources"
curl -fsS "$BASE/network/v1/tenants/$TENANT/workspaces/$WS/networks/$NET" | jq
curl -fsS "$BASE/network/v1/tenants/$TENANT/workspaces/$WS/internet-gateways/$IGW" | jq
curl -fsS "$BASE/network/v1/tenants/$TENANT/workspaces/$WS/networks/$NET/route-tables/$RT" | jq

echo "Done. Optional provider checks:"
echo "  hcloud server list -o columns=id,name,labels | grep \"seca.kind=internet-gateway\""
echo "  hcloud network describe \"$NET\""
