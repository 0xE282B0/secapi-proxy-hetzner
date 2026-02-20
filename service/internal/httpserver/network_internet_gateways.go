package httpserver

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/eu-sovereign-cloud/secapi-proxy-hetzner/internal/config"
	"github.com/eu-sovereign-cloud/secapi-proxy-hetzner/internal/provider/hetzner"
	"github.com/eu-sovereign-cloud/secapi-proxy-hetzner/internal/state"
)

const resourceBindingKindInternetGateway = "internet-gateway"

type internetGatewayIterator struct {
	Items    []internetGatewayResource `json:"items"`
	Metadata responseMetaObject        `json:"metadata"`
}

type internetGatewayResource struct {
	Metadata resourceMetadata            `json:"metadata"`
	Labels   map[string]string           `json:"labels,omitempty"`
	Spec     internetGatewaySpec         `json:"spec"`
	Status   internetGatewayStatusObject `json:"status"`
}

type internetGatewaySpec struct {
	EgressOnly *bool `json:"egressOnly,omitempty"`
}

type internetGatewayStatusObject struct {
	State string `json:"state"`
}

type internetGatewayBindingPayload struct {
	Name        string              `json:"name"`
	Region      string              `json:"region"`
	Labels      map[string]string   `json:"labels,omitempty"`
	Spec        internetGatewaySpec `json:"spec"`
	Networks    []string            `json:"networks,omitempty"`
	RouteTables []string            `json:"routeTables,omitempty"`
	ProviderRef string              `json:"providerRef,omitempty"`
}

func listInternetGateways(store *state.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			respondProblem(w, http.StatusMethodNotAllowed, "http://secapi.cloud/errors/invalid-request", "Method Not Allowed", "Only GET is supported", r.URL.Path)
			return
		}
		tenant, workspace, ok := scopeFromPath(w, r)
		if !ok {
			return
		}
		if _, ok := workspaceExecutionContext(w, r, store, tenant, workspace); !ok {
			return
		}
		bindings, err := store.ListResourceBindings(r.Context(), tenant, workspace, resourceBindingKindInternetGateway)
		if err != nil {
			respondProblem(w, http.StatusInternalServerError, "http://secapi.cloud/errors/internal", "Internal Server Error", "failed to list internet gateways", r.URL.Path)
			return
		}
		items := make([]internetGatewayResource, 0, len(bindings))
		for _, binding := range bindings {
			payload, err := parseInternetGatewayBinding(binding.ProviderRef)
			if err != nil {
				continue
			}
			items = append(items, toInternetGatewayResourceFromBinding(binding, payload, tenant, workspace, http.MethodGet, "active"))
		}
		respondJSON(w, http.StatusOK, internetGatewayIterator{
			Items:    items,
			Metadata: responseMetaObject{Provider: "seca.network/v1", Resource: "tenants/" + tenant + "/workspaces/" + workspace + "/internet-gateways", Verb: http.MethodGet},
		})
	}
}

func internetGatewayCRUD(store *state.Store, computeProvider ComputeStorageProvider, cfg config.Config) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			getInternetGateway(store)(w, r)
		case http.MethodPut:
			putInternetGateway(store, computeProvider, cfg)(w, r)
		case http.MethodDelete:
			deleteInternetGateway(store, computeProvider, cfg)(w, r)
		default:
			respondProblem(w, http.StatusMethodNotAllowed, "http://secapi.cloud/errors/invalid-request", "Method Not Allowed", "Only GET, PUT and DELETE are supported", r.URL.Path)
		}
	}
}

func getInternetGateway(store *state.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		tenant, workspace, name, ok := scopedNameFromPath(w, r, "internet gateway name is required")
		if !ok {
			return
		}
		if _, ok := workspaceExecutionContext(w, r, store, tenant, workspace); !ok {
			return
		}
		ref := internetGatewayRef(tenant, workspace, name)
		binding, err := store.GetResourceBinding(r.Context(), ref)
		if err != nil {
			respondProblem(w, http.StatusInternalServerError, "http://secapi.cloud/errors/internal", "Internal Server Error", "failed to load internet gateway", r.URL.Path)
			return
		}
		if binding == nil {
			respondProblem(w, http.StatusNotFound, "http://secapi.cloud/errors/resource-not-found", "Not Found", "internet gateway not found", r.URL.Path)
			return
		}
		payload, err := parseInternetGatewayBinding(binding.ProviderRef)
		if err != nil {
			respondProblem(w, http.StatusInternalServerError, "http://secapi.cloud/errors/internal", "Internal Server Error", "invalid internet gateway payload", r.URL.Path)
			return
		}
		if networks, routeTables, usageErr := resolveInternetGatewayRouteUsage(r.Context(), store, tenant, workspace, name); usageErr == nil {
			payload.Networks = networks
			payload.RouteTables = routeTables
		}
		respondJSON(w, http.StatusOK, toInternetGatewayResourceFromBinding(*binding, payload, tenant, workspace, http.MethodGet, "active"))
	}
}

func putInternetGateway(store *state.Store, computeProvider ComputeStorageProvider, cfg config.Config) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		tenant, workspace, name, ok := scopedNameFromPath(w, r, "internet gateway name is required")
		if !ok {
			return
		}
		ctx, ok := workspaceExecutionContext(w, r, store, tenant, workspace)
		if !ok {
			return
		}

		var req internetGatewayResource
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			respondProblem(w, http.StatusBadRequest, "http://secapi.cloud/errors/invalid-request", "Bad Request", "invalid json body", r.URL.Path)
			return
		}
		ref := internetGatewayRef(tenant, workspace, name)
		existing, err := store.GetResourceBinding(r.Context(), ref)
		if err != nil {
			respondProblem(w, http.StatusInternalServerError, "http://secapi.cloud/errors/internal", "Internal Server Error", "failed to load internet gateway", r.URL.Path)
			return
		}
		payload := internetGatewayBindingPayload{
			Name:   name,
			Region: runtimeRegionOrDefault(req.Metadata.Region),
			Labels: req.Labels,
			Spec:   req.Spec,
		}
		networks, routeTables, usageErr := resolveInternetGatewayRouteUsage(r.Context(), store, tenant, workspace, name)
		if usageErr != nil {
			respondProblem(w, http.StatusInternalServerError, "http://secapi.cloud/errors/internal", "Internal Server Error", "failed to resolve internet gateway route usage", r.URL.Path)
			return
		}
		payload.Networks = networks
		payload.RouteTables = routeTables
		providerRef, reconcileErr := reconcileInternetGatewayProvider(ctx, store, computeProvider, cfg, tenant, workspace, payload)
		if reconcileErr != nil {
			respondFromError(w, reconcileErr, r.URL.Path)
			return
		}
		if providerRef != "" {
			payload.ProviderRef = providerRef
		}
		raw, err := json.Marshal(payload)
		if err != nil {
			respondProblem(w, http.StatusInternalServerError, "http://secapi.cloud/errors/internal", "Internal Server Error", "failed to encode internet gateway", r.URL.Path)
			return
		}
		if err := store.UpsertResourceBinding(r.Context(), state.ResourceBinding{
			Tenant:      tenant,
			Workspace:   workspace,
			Kind:        resourceBindingKindInternetGateway,
			SecaRef:     ref,
			ProviderRef: string(raw),
			Status:      "active",
		}); err != nil {
			respondProblem(w, http.StatusInternalServerError, "http://secapi.cloud/errors/internal", "Internal Server Error", "failed to save internet gateway", r.URL.Path)
			return
		}
		binding, err := store.GetResourceBinding(r.Context(), ref)
		if err != nil || binding == nil {
			respondProblem(w, http.StatusInternalServerError, "http://secapi.cloud/errors/internal", "Internal Server Error", "failed to load internet gateway", r.URL.Path)
			return
		}
		stateValue, code := "updating", http.StatusOK
		if existing == nil {
			stateValue, code = "creating", http.StatusCreated
		}
		respondJSON(w, code, toInternetGatewayResourceFromBinding(*binding, payload, tenant, workspace, http.MethodPut, stateValue))
	}
}

func deleteInternetGateway(store *state.Store, computeProvider ComputeStorageProvider, cfg config.Config) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		tenant, workspace, name, ok := scopedNameFromPath(w, r, "internet gateway name is required")
		if !ok {
			return
		}
		ctx, ok := workspaceExecutionContext(w, r, store, tenant, workspace)
		if !ok {
			return
		}
		ref := internetGatewayRef(tenant, workspace, name)
		binding, err := store.GetResourceBinding(r.Context(), ref)
		if err != nil {
			respondProblem(w, http.StatusInternalServerError, "http://secapi.cloud/errors/internal", "Internal Server Error", "failed to load internet gateway", r.URL.Path)
			return
		}
		if binding == nil {
			respondProblem(w, http.StatusNotFound, "http://secapi.cloud/errors/resource-not-found", "Not Found", "internet gateway not found", r.URL.Path)
			return
		}
		if cfg.InternetGatewayNATVM {
			instanceName := internetGatewayInstanceName(workspace, name)
			if _, _, delErr := computeProvider.DeleteInstance(ctx, instanceName); delErr != nil {
				respondFromError(w, delErr, r.URL.Path)
				return
			}
		}
		if err := store.DeleteResourceBinding(r.Context(), ref); err != nil {
			respondProblem(w, http.StatusInternalServerError, "http://secapi.cloud/errors/internal", "Internal Server Error", "failed to delete internet gateway", r.URL.Path)
			return
		}
		respondJSON(w, http.StatusAccepted, map[string]string{"status": "accepted"})
	}
}

func internetGatewayRef(tenant, workspace, name string) string {
	return "seca.network/v1/tenants/" + strings.ToLower(strings.TrimSpace(tenant)) +
		"/workspaces/" + strings.ToLower(strings.TrimSpace(workspace)) +
		"/internet-gateways/" + strings.ToLower(strings.TrimSpace(name))
}

func parseInternetGatewayBinding(raw string) (internetGatewayBindingPayload, error) {
	var payload internetGatewayBindingPayload
	err := json.Unmarshal([]byte(raw), &payload)
	return payload, err
}

func resolveInternetGatewayRouteUsage(ctx context.Context, store *state.Store, tenant, workspace, gatewayName string) ([]string, []string, error) {
	bindings, err := store.ListResourceBindings(ctx, tenant, workspace, resourceBindingKindRouteTable)
	if err != nil {
		return nil, nil, err
	}
	gatewayName = strings.ToLower(strings.TrimSpace(gatewayName))
	networkSet := map[string]struct{}{}
	routeTableSet := map[string]struct{}{}
	for _, binding := range bindings {
		payload, parseErr := parseRouteTableBinding(binding.ProviderRef)
		if parseErr != nil {
			continue
		}
		for _, route := range payload.Spec.Routes {
			if strings.ToLower(strings.TrimSpace(resourceNameFromRef(route.TargetRef.Resource))) != gatewayName {
				continue
			}
			if n := strings.ToLower(strings.TrimSpace(payload.Network)); n != "" {
				networkSet[n] = struct{}{}
			}
			if rt := strings.ToLower(strings.TrimSpace(payload.Name)); rt != "" {
				routeTableSet[rt] = struct{}{}
			}
			break
		}
	}
	networks := make([]string, 0, len(networkSet))
	for network := range networkSet {
		networks = append(networks, network)
	}
	sort.Strings(networks)

	routeTables := make([]string, 0, len(routeTableSet))
	for routeTable := range routeTableSet {
		routeTables = append(routeTables, routeTable)
	}
	sort.Strings(routeTables)
	return networks, routeTables, nil
}

func reconcileInternetGatewayProvider(
	ctx context.Context,
	store *state.Store,
	computeProvider ComputeStorageProvider,
	cfg config.Config,
	tenant, workspace string,
	payload internetGatewayBindingPayload,
) (string, error) {
	if !cfg.InternetGatewayNATVM {
		return "", nil
	}
	if computeProvider == nil {
		return "", fmt.Errorf("internet-gateway provisioning is enabled but compute provider is not available")
	}
	region := strings.ToLower(strings.TrimSpace(payload.Region))
	if region == "" {
		workspaceRegion, ok := workspaceRegionOrDefault(ctx, store, tenant, workspace)
		if !ok {
			return "", fmt.Errorf("failed to resolve workspace region")
		}
		region = workspaceRegion
	}

	instanceName := internetGatewayInstanceName(workspace, payload.Name)
	if len(payload.RouteTables) == 0 {
		_, _, err := computeProvider.DeleteInstance(ctx, instanceName)
		return "", err
	}

	_, _, _, err := computeProvider.CreateOrUpdateInstance(ctx, hetzner.InstanceCreateRequest{
		Name:      instanceName,
		SKUName:   "cax11",
		ImageName: "ubuntu-24.04",
		Region:    region,
		UserData:  internetGatewayNATCloudInit(payload),
		Labels: withSecaProviderLabels(
			payload.Labels,
			tenant,
			workspace,
			"internet-gateway",
			payload.Name,
			internetGatewayRef(tenant, workspace, payload.Name),
		),
	})
	if err != nil {
		return "", err
	}
	instance, err := computeProvider.GetInstance(ctx, instanceName)
	if err != nil {
		return "", err
	}
	if instance == nil {
		return "", fmt.Errorf("internet-gateway instance %q not found after create", instanceName)
	}
	for _, networkName := range payload.Networks {
		trimmed := strings.ToLower(strings.TrimSpace(networkName))
		if trimmed == "" {
			continue
		}
		if _, _, attachErr := computeProvider.AttachInstanceToNetwork(ctx, instanceName, trimmed); attachErr != nil {
			return "", attachErr
		}
	}
	return fmt.Sprintf("instances/%s", instance.Name), nil
}

func internetGatewayNATCloudInit(payload internetGatewayBindingPayload) string {
	egressOnly := true
	if payload.Spec.EgressOnly != nil {
		egressOnly = *payload.Spec.EgressOnly
	}
	egressMarker := "true"
	if !egressOnly {
		egressMarker = "false"
	}

	return fmt.Sprintf(`#cloud-config
write_files:
  - path: /usr/local/sbin/seca-igw-init.sh
    permissions: "0755"
    content: |
      #!/usr/bin/env bash
      set -euo pipefail

      # TODO: bind NAT to resolved SECA network attachments once IGW->network
      # reconciliation is provider-backed.
      EGRESS_IFACE="$(ip -4 route show default | awk '/default/ {print $5; exit}')"
      if [[ -z "${EGRESS_IFACE}" ]]; then
        exit 0
      fi

      sysctl -w net.ipv4.ip_forward=1
      cat >/etc/sysctl.d/99-seca-igw.conf <<'EOF'
      net.ipv4.ip_forward=1
      EOF
      sysctl -p /etc/sysctl.d/99-seca-igw.conf

      # Basic SNAT for private ranges through default egress.
      iptables -t nat -C POSTROUTING -o "${EGRESS_IFACE}" -j MASQUERADE 2>/dev/null || \
        iptables -t nat -A POSTROUTING -o "${EGRESS_IFACE}" -j MASQUERADE
      iptables -C FORWARD -m conntrack --ctstate RELATED,ESTABLISHED -j ACCEPT 2>/dev/null || \
        iptables -A FORWARD -m conntrack --ctstate RELATED,ESTABLISHED -j ACCEPT
      iptables -C FORWARD -i "${EGRESS_IFACE}" -j ACCEPT 2>/dev/null || \
        iptables -A FORWARD -i "${EGRESS_IFACE}" -j ACCEPT
      iptables -C FORWARD -o "${EGRESS_IFACE}" -j ACCEPT 2>/dev/null || \
        iptables -A FORWARD -o "${EGRESS_IFACE}" -j ACCEPT

runcmd:
  - [bash, -lc, "/usr/local/sbin/seca-igw-init.sh"]
final_message: "SECA internet-gateway init complete (egressOnly=%s)"
`, egressMarker)
}

func internetGatewayInstanceName(workspace, gatewayName string) string {
	workspace = strings.ToLower(strings.TrimSpace(workspace))
	gatewayName = strings.ToLower(strings.TrimSpace(gatewayName))
	full := "seca-igw-" + workspace + "-" + gatewayName
	if len(full) <= 63 {
		return full
	}
	const keep = 63
	return full[:keep]
}

func toInternetGatewayResourceFromBinding(
	binding state.ResourceBinding,
	payload internetGatewayBindingPayload,
	tenant,
	workspace,
	verb,
	stateValue string,
) internetGatewayResource {
	createdAt := time.Now().UTC().Format(time.RFC3339)
	updatedAt := createdAt
	if !binding.CreatedAt.IsZero() {
		createdAt = binding.CreatedAt.UTC().Format(time.RFC3339)
	}
	if !binding.UpdatedAt.IsZero() {
		updatedAt = binding.UpdatedAt.UTC().Format(time.RFC3339)
	}
	return internetGatewayResource{
		Metadata: resourceMetadata{
			Name:            payload.Name,
			Provider:        "seca.network/v1",
			Resource:        "tenants/" + tenant + "/workspaces/" + workspace + "/internet-gateways/" + payload.Name,
			Verb:            verb,
			CreatedAt:       createdAt,
			LastModifiedAt:  updatedAt,
			ResourceVersion: 1,
			APIVersion:      "v1",
			Kind:            "internet-gateway",
			Ref:             "seca.network/v1/tenants/" + tenant + "/workspaces/" + workspace + "/internet-gateways/" + payload.Name,
			Tenant:          tenant,
			Workspace:       workspace,
			Region:          defaultRegion(strings.ToLower(strings.TrimSpace(payload.Region))),
		},
		Labels: payload.Labels,
		Spec:   payload.Spec,
		Status: internetGatewayStatusObject{State: stateValue},
	}
}
