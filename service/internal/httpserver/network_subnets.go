package httpserver

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/eu-sovereign-cloud/secapi-proxy-hetzner/internal/state"
)

type subnetIterator struct {
	Items    []subnetResource  `json:"items"`
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

func listSubnets(store *state.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// TODO: Replace this in-memory subnet shim with provider-backed implementation.
		if r.Method != http.MethodGet {
			respondProblem(w, http.StatusMethodNotAllowed, "http://secapi.cloud/errors/invalid-request", "Method Not Allowed", "Only GET is supported", r.URL.Path)
			return
		}
		tenant, workspace, ok := scopeFromPath(w, r)
		if !ok {
			return
		}
		network := strings.ToLower(r.PathValue("network"))
		if network == "" {
			respondProblem(w, http.StatusBadRequest, "http://secapi.cloud/errors/invalid-request", "Bad Request", "network name is required", r.URL.Path)
			return
		}
		if _, ok := workspaceExecutionContext(w, r, store, tenant, workspace); !ok {
			return
		}
		records := runtimeResourceState.listSubnetsByScope(tenant, workspace, network)
		items := make([]subnetResource, 0, len(records))
		for _, rec := range records {
			items = append(items, toRuntimeSubnetResource(rec, http.MethodGet, "active"))
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
		rec, ok := runtimeResourceState.getSubnet(subnetRefKey(tenant, workspace, network, name))
		if !ok {
			respondProblem(w, http.StatusNotFound, "http://secapi.cloud/errors/resource-not-found", "Not Found", "subnet not found", r.URL.Path)
			return
		}
		respondJSON(w, http.StatusOK, toRuntimeSubnetResource(rec, http.MethodGet, "active"))
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
		region := runtimeRegionOrDefault(req.Metadata.Region)
		now := time.Now().UTC().Format(time.RFC3339)
		rec, created := runtimeResourceState.upsertSubnet(subnetRefKey(tenant, workspace, network, name), subnetRuntimeRecord{
			Tenant:         tenant,
			Workspace:      workspace,
			Network:        network,
			Name:           name,
			Region:         region,
			Labels:         req.Labels,
			Spec:           req.Spec,
			CreatedAt:      now,
			LastModifiedAt: now,
		})
		stateValue, code := upsertStateAndCode(created)
		respondJSON(w, code, toRuntimeSubnetResource(rec, http.MethodPut, stateValue))
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
		if _, ok := runtimeResourceState.getSubnet(subnetRefKey(tenant, workspace, network, name)); !ok {
			respondProblem(w, http.StatusNotFound, "http://secapi.cloud/errors/resource-not-found", "Not Found", "subnet not found", r.URL.Path)
			return
		}
		runtimeResourceState.deleteSubnet(subnetRefKey(tenant, workspace, network, name))
		respondJSON(w, http.StatusAccepted, map[string]string{"status": "accepted"})
	}
}

func subnetRefKey(tenant, workspace, network, name string) string {
	return strings.ToLower(strings.TrimSpace(tenant)) + "/" + strings.ToLower(strings.TrimSpace(workspace)) + "/" + strings.ToLower(strings.TrimSpace(network)) + "/" + strings.ToLower(strings.TrimSpace(name))
}

func toRuntimeSubnetResource(rec subnetRuntimeRecord, verb, state string) subnetResource {
	return subnetResource{
		Metadata: resourceMetadata{
			Name:            rec.Name,
			Provider:        "seca.network/v1",
			Resource:        "tenants/" + rec.Tenant + "/workspaces/" + rec.Workspace + "/networks/" + rec.Network + "/subnets/" + rec.Name,
			Verb:            verb,
			CreatedAt:       rec.CreatedAt,
			LastModifiedAt:  rec.LastModifiedAt,
			ResourceVersion: rec.ResourceVersion,
			APIVersion:      "v1",
			Kind:            "subnet",
			Ref:             "seca.network/v1/tenants/" + rec.Tenant + "/workspaces/" + rec.Workspace + "/networks/" + rec.Network + "/subnets/" + rec.Name,
			Tenant:          rec.Tenant,
			Workspace:       rec.Workspace,
			Network:         rec.Network,
			Region:          rec.Region,
		},
		Labels: rec.Labels,
		Spec:   rec.Spec,
		Status: subnetStatusObject{State: state},
	}
}
