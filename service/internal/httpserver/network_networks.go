package httpserver

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/eu-sovereign-cloud/secapi-proxy-hetzner/internal/state"
)

type networkIterator struct {
	Items    []networkResource `json:"items"`
	Metadata responseMetaObject `json:"metadata"`
}

type networkResource struct {
	Metadata resourceMetadata    `json:"metadata"`
	Labels   map[string]string   `json:"labels,omitempty"`
	Spec     networkSpec         `json:"spec"`
	Status   networkStatusObject `json:"status"`
}

type networkSpec struct {
	Cidr         networkCIDR `json:"cidr"`
	SkuRef       refObject   `json:"skuRef"`
	RouteTableRef refObject  `json:"routeTableRef,omitempty"`
}

type networkCIDR struct {
	IPv4 *string `json:"ipv4,omitempty"`
	IPv6 *string `json:"ipv6,omitempty"`
}

type networkStatusObject struct {
	State      string      `json:"state"`
	Cidr       networkCIDR `json:"cidr"`
	Conditions []any       `json:"conditions,omitempty"`
}

func listNetworks(store *state.Store) http.HandlerFunc {
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
		records := runtimeResourceState.listNetworksByScope(tenant, workspace)
		items := make([]networkResource, 0, len(records))
		for _, rec := range records {
			items = append(items, toRuntimeNetworkResource(rec, http.MethodGet, "active"))
		}
		respondJSON(w, http.StatusOK, networkIterator{
			Items:    items,
			Metadata: responseMetaObject{Provider: "seca.network/v1", Resource: "tenants/" + tenant + "/workspaces/" + workspace + "/networks", Verb: http.MethodGet},
		})
	}
}

func networkCRUD(store *state.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			getNetwork(store)(w, r)
		case http.MethodPut:
			putNetwork(store)(w, r)
		case http.MethodDelete:
			deleteNetwork(store)(w, r)
		default:
			respondProblem(w, http.StatusMethodNotAllowed, "http://secapi.cloud/errors/invalid-request", "Method Not Allowed", "Only GET, PUT and DELETE are supported", r.URL.Path)
		}
	}
}

func getNetwork(store *state.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		tenant, workspace, name, ok := scopedNameFromPath(w, r, "network name is required")
		if !ok {
			return
		}
		if _, ok := workspaceExecutionContext(w, r, store, tenant, workspace); !ok {
			return
		}
		rec, ok := runtimeResourceState.getNetwork(networkRef(tenant, workspace, name))
		if !ok {
			respondProblem(w, http.StatusNotFound, "http://secapi.cloud/errors/resource-not-found", "Not Found", "network not found", r.URL.Path)
			return
		}
		respondJSON(w, http.StatusOK, toRuntimeNetworkResource(rec, http.MethodGet, "active"))
	}
}

func putNetwork(store *state.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		tenant, workspace, name, ok := scopedNameFromPath(w, r, "network name is required")
		if !ok {
			return
		}
		if _, ok := workspaceExecutionContext(w, r, store, tenant, workspace); !ok {
			return
		}
		var req networkResource
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			respondProblem(w, http.StatusBadRequest, "http://secapi.cloud/errors/invalid-request", "Bad Request", "invalid json body", r.URL.Path)
			return
		}
		if strings.TrimSpace(req.Spec.SkuRef.Resource) == "" {
			respondProblem(w, http.StatusBadRequest, "http://secapi.cloud/errors/invalid-request", "Bad Request", "spec.skuRef is required", r.URL.Path)
			return
		}

		region := strings.TrimSpace(req.Metadata.Region)
		if region == "" {
			region = "global"
		}
		now := time.Now().UTC().Format(time.RFC3339)
		rec, created := runtimeResourceState.upsertNetwork(networkRef(tenant, workspace, name), networkRuntimeRecord{
			Tenant:         tenant,
			Workspace:      workspace,
			Name:           name,
			Region:         region,
			Labels:         req.Labels,
			Spec:           req.Spec,
			CreatedAt:      now,
			LastModifiedAt: now,
		})

		stateValue := "updating"
		code := http.StatusOK
		if created {
			stateValue = "creating"
			code = http.StatusCreated
		}
		respondJSON(w, code, toRuntimeNetworkResource(rec, http.MethodPut, stateValue))
	}
}

func deleteNetwork(store *state.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		tenant, workspace, name, ok := scopedNameFromPath(w, r, "network name is required")
		if !ok {
			return
		}
		if _, ok := workspaceExecutionContext(w, r, store, tenant, workspace); !ok {
			return
		}
		if _, ok := runtimeResourceState.getNetwork(networkRef(tenant, workspace, name)); !ok {
			respondProblem(w, http.StatusNotFound, "http://secapi.cloud/errors/resource-not-found", "Not Found", "network not found", r.URL.Path)
			return
		}
		runtimeResourceState.deleteNetwork(networkRef(tenant, workspace, name))
		respondJSON(w, http.StatusAccepted, map[string]string{"status": "accepted"})
	}
}

func networkRef(tenant, workspace, name string) string {
	return strings.ToLower(strings.TrimSpace(tenant)) + "/" + strings.ToLower(strings.TrimSpace(workspace)) + "/" + strings.ToLower(strings.TrimSpace(name))
}

func toRuntimeNetworkResource(rec networkRuntimeRecord, verb, state string) networkResource {
	return networkResource{
		Metadata: resourceMetadata{
			Name:            rec.Name,
			Provider:        "seca.network/v1",
			Resource:        "tenants/" + rec.Tenant + "/workspaces/" + rec.Workspace + "/networks/" + rec.Name,
			Verb:            verb,
			CreatedAt:       rec.CreatedAt,
			LastModifiedAt:  rec.LastModifiedAt,
			ResourceVersion: rec.ResourceVersion,
			APIVersion:      "v1",
			Kind:            "network",
			Ref:             "seca.network/v1/tenants/" + rec.Tenant + "/workspaces/" + rec.Workspace + "/networks/" + rec.Name,
			Tenant:          rec.Tenant,
			Workspace:       rec.Workspace,
			Region:          rec.Region,
		},
		Labels: rec.Labels,
		Spec:   rec.Spec,
		Status: networkStatusObject{
			State: state,
			Cidr:  rec.Spec.Cidr,
		},
	}
}
