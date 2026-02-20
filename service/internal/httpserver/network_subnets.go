package httpserver

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/eu-sovereign-cloud/secapi-proxy-hetzner/internal/state"
)

const resourceBindingKindSubnet = "subnet"

type subnetIterator struct {
	Items    []subnetResource   `json:"items"`
	Metadata responseMetaObject `json:"metadata"`
}

type subnetResource struct {
	Metadata resourceMetadata   `json:"metadata"`
	Labels   map[string]string  `json:"labels,omitempty"`
	Spec     subnetSpec         `json:"spec"`
	Status   subnetStatusObject `json:"status"`
}

type subnetSpec struct {
	Cidr networkCIDR `json:"cidr"`
	Zone string      `json:"zone"`
}

type subnetStatusObject struct {
	State string `json:"state"`
}

type subnetBindingPayload struct {
	Name    string            `json:"name"`
	Network string            `json:"network"`
	Region  string            `json:"region"`
	Labels  map[string]string `json:"labels,omitempty"`
	Spec    subnetSpec        `json:"spec"`
}

func listSubnets(store *state.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			respondProblem(w, http.StatusMethodNotAllowed, "http://secapi.cloud/errors/invalid-request", "Method Not Allowed", "Only GET is supported", r.URL.Path)
			return
		}
		tenant, workspace, ok := scopeFromPath(w, r)
		if !ok {
			return
		}
		network := strings.ToLower(strings.TrimSpace(r.PathValue("network")))
		if network == "" {
			respondProblem(w, http.StatusBadRequest, "http://secapi.cloud/errors/invalid-request", "Bad Request", "network name is required", r.URL.Path)
			return
		}
		if _, ok := workspaceExecutionContext(w, r, store, tenant, workspace); !ok {
			return
		}
		bindings, err := store.ListResourceBindings(r.Context(), tenant, workspace, resourceBindingKindSubnet)
		if err != nil {
			respondProblem(w, http.StatusInternalServerError, "http://secapi.cloud/errors/internal", "Internal Server Error", "failed to list subnets", r.URL.Path)
			return
		}
		items := make([]subnetResource, 0, len(bindings))
		for _, binding := range bindings {
			payload, err := parseSubnetBinding(binding.ProviderRef)
			if err != nil {
				continue
			}
			if strings.ToLower(strings.TrimSpace(payload.Network)) != network {
				continue
			}
			items = append(items, toSubnetResourceFromBinding(binding, payload, tenant, workspace, http.MethodGet, "active"))
		}
		respondJSON(w, http.StatusOK, subnetIterator{
			Items:    items,
			Metadata: responseMetaObject{Provider: "seca.network/v1", Resource: "tenants/" + tenant + "/workspaces/" + workspace + "/networks/" + network + "/subnets", Verb: http.MethodGet},
		})
	}
}

func subnetCRUD(store *state.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			getSubnet(store)(w, r)
		case http.MethodPut:
			putSubnet(store)(w, r)
		case http.MethodDelete:
			deleteSubnet(store)(w, r)
		default:
			respondProblem(w, http.StatusMethodNotAllowed, "http://secapi.cloud/errors/invalid-request", "Method Not Allowed", "Only GET, PUT and DELETE are supported", r.URL.Path)
		}
	}
}

func getSubnet(store *state.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		tenant, workspace, network, name, ok := scopedNetworkNameFromPath(w, r, "subnet name is required")
		if !ok {
			return
		}
		if _, ok := workspaceExecutionContext(w, r, store, tenant, workspace); !ok {
			return
		}
		ref := subnetRefKey(tenant, workspace, network, name)
		binding, err := store.GetResourceBinding(r.Context(), ref)
		if err != nil {
			respondProblem(w, http.StatusInternalServerError, "http://secapi.cloud/errors/internal", "Internal Server Error", "failed to load subnet", r.URL.Path)
			return
		}
		if binding == nil {
			respondProblem(w, http.StatusNotFound, "http://secapi.cloud/errors/resource-not-found", "Not Found", "subnet not found", r.URL.Path)
			return
		}
		payload, err := parseSubnetBinding(binding.ProviderRef)
		if err != nil {
			respondProblem(w, http.StatusInternalServerError, "http://secapi.cloud/errors/internal", "Internal Server Error", "invalid subnet payload", r.URL.Path)
			return
		}
		respondJSON(w, http.StatusOK, toSubnetResourceFromBinding(*binding, payload, tenant, workspace, http.MethodGet, "active"))
	}
}

func putSubnet(store *state.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		tenant, workspace, network, name, ok := scopedNetworkNameFromPath(w, r, "subnet name is required")
		if !ok {
			return
		}
		if _, ok := workspaceExecutionContext(w, r, store, tenant, workspace); !ok {
			return
		}
		var req subnetResource
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			respondProblem(w, http.StatusBadRequest, "http://secapi.cloud/errors/invalid-request", "Bad Request", "invalid json body", r.URL.Path)
			return
		}
		ref := subnetRefKey(tenant, workspace, network, name)
		existing, err := store.GetResourceBinding(r.Context(), ref)
		if err != nil {
			respondProblem(w, http.StatusInternalServerError, "http://secapi.cloud/errors/internal", "Internal Server Error", "failed to load subnet", r.URL.Path)
			return
		}
		payload := subnetBindingPayload{
			Name:    name,
			Network: network,
			Region:  runtimeRegionOrDefault(req.Metadata.Region),
			Labels:  req.Labels,
			Spec:    req.Spec,
		}
		raw, err := json.Marshal(payload)
		if err != nil {
			respondProblem(w, http.StatusInternalServerError, "http://secapi.cloud/errors/internal", "Internal Server Error", "failed to encode subnet", r.URL.Path)
			return
		}
		if err := store.UpsertResourceBinding(r.Context(), state.ResourceBinding{
			Tenant:      tenant,
			Workspace:   workspace,
			Kind:        resourceBindingKindSubnet,
			SecaRef:     ref,
			ProviderRef: string(raw),
			Status:      "active",
		}); err != nil {
			respondProblem(w, http.StatusInternalServerError, "http://secapi.cloud/errors/internal", "Internal Server Error", "failed to save subnet", r.URL.Path)
			return
		}
		binding, err := store.GetResourceBinding(r.Context(), ref)
		if err != nil || binding == nil {
			respondProblem(w, http.StatusInternalServerError, "http://secapi.cloud/errors/internal", "Internal Server Error", "failed to load subnet", r.URL.Path)
			return
		}
		stateValue, code := "updating", http.StatusOK
		if existing == nil {
			stateValue, code = "creating", http.StatusCreated
		}
		respondJSON(w, code, toSubnetResourceFromBinding(*binding, payload, tenant, workspace, http.MethodPut, stateValue))
	}
}

func deleteSubnet(store *state.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		tenant, workspace, network, name, ok := scopedNetworkNameFromPath(w, r, "subnet name is required")
		if !ok {
			return
		}
		if _, ok := workspaceExecutionContext(w, r, store, tenant, workspace); !ok {
			return
		}
		ref := subnetRefKey(tenant, workspace, network, name)
		binding, err := store.GetResourceBinding(r.Context(), ref)
		if err != nil {
			respondProblem(w, http.StatusInternalServerError, "http://secapi.cloud/errors/internal", "Internal Server Error", "failed to load subnet", r.URL.Path)
			return
		}
		if binding == nil {
			respondProblem(w, http.StatusNotFound, "http://secapi.cloud/errors/resource-not-found", "Not Found", "subnet not found", r.URL.Path)
			return
		}
		if err := store.DeleteResourceBinding(r.Context(), ref); err != nil {
			respondProblem(w, http.StatusInternalServerError, "http://secapi.cloud/errors/internal", "Internal Server Error", "failed to delete subnet", r.URL.Path)
			return
		}
		respondJSON(w, http.StatusAccepted, map[string]string{"status": "accepted"})
	}
}

func subnetRefKey(tenant, workspace, network, name string) string {
	return "seca.network/v1/tenants/" + strings.ToLower(strings.TrimSpace(tenant)) +
		"/workspaces/" + strings.ToLower(strings.TrimSpace(workspace)) +
		"/networks/" + strings.ToLower(strings.TrimSpace(network)) +
		"/subnets/" + strings.ToLower(strings.TrimSpace(name))
}

func parseSubnetBinding(raw string) (subnetBindingPayload, error) {
	var payload subnetBindingPayload
	err := json.Unmarshal([]byte(raw), &payload)
	return payload, err
}

func toSubnetResourceFromBinding(
	binding state.ResourceBinding,
	payload subnetBindingPayload,
	tenant,
	workspace,
	verb,
	stateValue string,
) subnetResource {
	createdAt := time.Now().UTC().Format(time.RFC3339)
	updatedAt := createdAt
	if !binding.CreatedAt.IsZero() {
		createdAt = binding.CreatedAt.UTC().Format(time.RFC3339)
	}
	if !binding.UpdatedAt.IsZero() {
		updatedAt = binding.UpdatedAt.UTC().Format(time.RFC3339)
	}
	return subnetResource{
		Metadata: resourceMetadata{
			Name:            payload.Name,
			Provider:        "seca.network/v1",
			Resource:        "tenants/" + tenant + "/workspaces/" + workspace + "/networks/" + payload.Network + "/subnets/" + payload.Name,
			Verb:            verb,
			CreatedAt:       createdAt,
			LastModifiedAt:  updatedAt,
			ResourceVersion: 1,
			APIVersion:      "v1",
			Kind:            "subnet",
			Ref:             "seca.network/v1/tenants/" + tenant + "/workspaces/" + workspace + "/networks/" + payload.Network + "/subnets/" + payload.Name,
			Tenant:          tenant,
			Workspace:       workspace,
			Network:         payload.Network,
			Region:          defaultRegion(strings.ToLower(strings.TrimSpace(payload.Region))),
		},
		Labels: payload.Labels,
		Spec:   payload.Spec,
		Status: subnetStatusObject{State: stateValue},
	}
}

