package httpserver

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/eu-sovereign-cloud/secapi-proxy-hetzner/internal/state"
)

type routeTableIterator struct {
	Items    []routeTableResource `json:"items"`
	Metadata responseMetaObject   `json:"metadata"`
}

type routeTableResource struct {
	Metadata resourceMetadata      `json:"metadata"`
	Labels   map[string]string     `json:"labels,omitempty"`
	Spec     routeTableSpec        `json:"spec"`
	Status   routeTableStatusObject `json:"status"`
}

type routeTableSpec struct {
	Routes []routeTableRouteSpec `json:"routes"`
}

type routeTableRouteSpec struct {
	DestinationCidrBlock string   `json:"destinationCidrBlock"`
	TargetRef            refObject `json:"targetRef"`
}

type routeTableStatusObject struct {
	State string `json:"state"`
}

func listRouteTables(store *state.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
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

		records := runtimeResourceState.listRouteTablesByScope(tenant, workspace, network)
		items := make([]routeTableResource, 0, len(records))
		for _, rec := range records {
			items = append(items, toRuntimeRouteTableResource(rec, http.MethodGet, "active"))
		}
		respondJSON(w, http.StatusOK, routeTableIterator{
			Items:    items,
			Metadata: responseMetaObject{Provider: "seca.network/v1", Resource: "tenants/" + tenant + "/workspaces/" + workspace + "/networks/" + network + "/route-tables", Verb: http.MethodGet},
		})
	}
}

func routeTableCRUD(store *state.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			getRouteTable(store)(w, r)
		case http.MethodPut:
			putRouteTable(store)(w, r)
		case http.MethodDelete:
			deleteRouteTable(store)(w, r)
		default:
			respondProblem(w, http.StatusMethodNotAllowed, "http://secapi.cloud/errors/invalid-request", "Method Not Allowed", "Only GET, PUT and DELETE are supported", r.URL.Path)
		}
	}
}

func getRouteTable(store *state.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		tenant, workspace, network, name, ok := scopedNetworkNameFromPath(w, r, "route table name is required")
		if !ok {
			return
		}
		if _, ok := workspaceExecutionContext(w, r, store, tenant, workspace); !ok {
			return
		}
		rec, ok := runtimeResourceState.getRouteTable(routeTableRefKey(tenant, workspace, network, name))
		if !ok {
			respondProblem(w, http.StatusNotFound, "http://secapi.cloud/errors/resource-not-found", "Not Found", "route table not found", r.URL.Path)
			return
		}
		respondJSON(w, http.StatusOK, toRuntimeRouteTableResource(rec, http.MethodGet, "active"))
	}
}

func putRouteTable(store *state.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		tenant, workspace, network, name, ok := scopedNetworkNameFromPath(w, r, "route table name is required")
		if !ok {
			return
		}
		if _, ok := workspaceExecutionContext(w, r, store, tenant, workspace); !ok {
			return
		}
		var req routeTableResource
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			respondProblem(w, http.StatusBadRequest, "http://secapi.cloud/errors/invalid-request", "Bad Request", "invalid json body", r.URL.Path)
			return
		}
		region := strings.TrimSpace(req.Metadata.Region)
		if region == "" {
			region = "global"
		}
		now := time.Now().UTC().Format(time.RFC3339)
		rec, created := runtimeResourceState.upsertRouteTable(routeTableRefKey(tenant, workspace, network, name), routeTableRuntimeRecord{
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
		stateValue := "updating"
		code := http.StatusOK
		if created {
			stateValue = "creating"
			code = http.StatusCreated
		}
		respondJSON(w, code, toRuntimeRouteTableResource(rec, http.MethodPut, stateValue))
	}
}

func deleteRouteTable(store *state.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		tenant, workspace, network, name, ok := scopedNetworkNameFromPath(w, r, "route table name is required")
		if !ok {
			return
		}
		if _, ok := workspaceExecutionContext(w, r, store, tenant, workspace); !ok {
			return
		}
		if _, ok := runtimeResourceState.getRouteTable(routeTableRefKey(tenant, workspace, network, name)); !ok {
			respondProblem(w, http.StatusNotFound, "http://secapi.cloud/errors/resource-not-found", "Not Found", "route table not found", r.URL.Path)
			return
		}
		runtimeResourceState.deleteRouteTable(routeTableRefKey(tenant, workspace, network, name))
		respondJSON(w, http.StatusAccepted, map[string]string{"status": "accepted"})
	}
}

func routeTableRefKey(tenant, workspace, network, name string) string {
	return strings.ToLower(strings.TrimSpace(tenant)) + "/" + strings.ToLower(strings.TrimSpace(workspace)) + "/" + strings.ToLower(strings.TrimSpace(network)) + "/" + strings.ToLower(strings.TrimSpace(name))
}

func toRuntimeRouteTableResource(rec routeTableRuntimeRecord, verb, state string) routeTableResource {
	return routeTableResource{
		Metadata: resourceMetadata{
			Name:            rec.Name,
			Provider:        "seca.network/v1",
			Resource:        "tenants/" + rec.Tenant + "/workspaces/" + rec.Workspace + "/networks/" + rec.Network + "/route-tables/" + rec.Name,
			Verb:            verb,
			CreatedAt:       rec.CreatedAt,
			LastModifiedAt:  rec.LastModifiedAt,
			ResourceVersion: rec.ResourceVersion,
			APIVersion:      "v1",
			Kind:            "routing-table",
			Ref:             "seca.network/v1/tenants/" + rec.Tenant + "/workspaces/" + rec.Workspace + "/networks/" + rec.Network + "/route-tables/" + rec.Name,
			Tenant:          rec.Tenant,
			Workspace:       rec.Workspace,
			Network:         rec.Network,
			Region:          rec.Region,
		},
		Labels: rec.Labels,
		Spec:   rec.Spec,
		Status: routeTableStatusObject{State: state},
	}
}
