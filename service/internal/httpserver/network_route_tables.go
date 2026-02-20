package httpserver

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/eu-sovereign-cloud/secapi-proxy-hetzner/internal/state"
)

const (
	resourceBindingKindRouteTable          = "route-table"
	resourceBindingKindNetworkRouteTableRef = "network-route-table-ref"
)

type routeTableIterator struct {
	Items    []routeTableResource `json:"items"`
	Metadata responseMetaObject   `json:"metadata"`
}

type routeTableResource struct {
	Metadata resourceMetadata       `json:"metadata"`
	Labels   map[string]string      `json:"labels,omitempty"`
	Spec     routeTableSpec         `json:"spec"`
	Status   routeTableStatusObject `json:"status"`
}

type routeTableSpec struct {
	Routes []routeTableRouteSpec `json:"routes"`
}

type routeTableRouteSpec struct {
	DestinationCidrBlock string    `json:"destinationCidrBlock"`
	TargetRef            refObject `json:"targetRef"`
}

type routeTableStatusObject struct {
	State string `json:"state"`
}

type routeTableBindingPayload struct {
	Name    string            `json:"name"`
	Network string            `json:"network"`
	Region  string            `json:"region"`
	Labels  map[string]string `json:"labels,omitempty"`
	Spec    routeTableSpec    `json:"spec"`
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
		network := strings.ToLower(strings.TrimSpace(r.PathValue("network")))
		if network == "" {
			respondProblem(w, http.StatusBadRequest, "http://secapi.cloud/errors/invalid-request", "Bad Request", "network name is required", r.URL.Path)
			return
		}
		if _, ok := workspaceExecutionContext(w, r, store, tenant, workspace); !ok {
			return
		}

		bindings, err := store.ListResourceBindings(r.Context(), tenant, workspace, resourceBindingKindRouteTable)
		if err != nil {
			respondProblem(w, http.StatusInternalServerError, "http://secapi.cloud/errors/internal", "Internal Server Error", "failed to list route tables", r.URL.Path)
			return
		}
		items := make([]routeTableResource, 0, len(bindings))
		for _, binding := range bindings {
			payload, err := parseRouteTableBinding(binding.ProviderRef)
			if err != nil {
				continue
			}
			if strings.ToLower(strings.TrimSpace(payload.Network)) != network {
				continue
			}
			items = append(items, toRouteTableResourceFromBinding(binding, payload, tenant, workspace, http.MethodGet, "active"))
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
		ref := routeTableRefKey(tenant, workspace, network, name)
		binding, err := store.GetResourceBinding(r.Context(), ref)
		if err != nil {
			respondProblem(w, http.StatusInternalServerError, "http://secapi.cloud/errors/internal", "Internal Server Error", "failed to load route table", r.URL.Path)
			return
		}
		if binding == nil {
			respondProblem(w, http.StatusNotFound, "http://secapi.cloud/errors/resource-not-found", "Not Found", "route table not found", r.URL.Path)
			return
		}
		payload, err := parseRouteTableBinding(binding.ProviderRef)
		if err != nil {
			respondProblem(w, http.StatusInternalServerError, "http://secapi.cloud/errors/internal", "Internal Server Error", "invalid route table payload", r.URL.Path)
			return
		}
		respondJSON(w, http.StatusOK, toRouteTableResourceFromBinding(*binding, payload, tenant, workspace, http.MethodGet, "active"))
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

		ref := routeTableRefKey(tenant, workspace, network, name)
		existing, err := store.GetResourceBinding(r.Context(), ref)
		if err != nil {
			respondProblem(w, http.StatusInternalServerError, "http://secapi.cloud/errors/internal", "Internal Server Error", "failed to load route table", r.URL.Path)
			return
		}

		payload := routeTableBindingPayload{
			Name:    name,
			Network: network,
			Region:  runtimeRegionOrDefault(req.Metadata.Region),
			Labels:  req.Labels,
			Spec:    req.Spec,
		}
		raw, err := json.Marshal(payload)
		if err != nil {
			respondProblem(w, http.StatusInternalServerError, "http://secapi.cloud/errors/internal", "Internal Server Error", "failed to encode route table", r.URL.Path)
			return
		}
		if err := store.UpsertResourceBinding(r.Context(), state.ResourceBinding{
			Tenant:      tenant,
			Workspace:   workspace,
			Kind:        resourceBindingKindRouteTable,
			SecaRef:     ref,
			ProviderRef: string(raw),
			Status:      "active",
		}); err != nil {
			respondProblem(w, http.StatusInternalServerError, "http://secapi.cloud/errors/internal", "Internal Server Error", "failed to save route table", r.URL.Path)
			return
		}
		binding, err := store.GetResourceBinding(r.Context(), ref)
		if err != nil || binding == nil {
			respondProblem(w, http.StatusInternalServerError, "http://secapi.cloud/errors/internal", "Internal Server Error", "failed to load route table", r.URL.Path)
			return
		}

		stateValue, code := "updating", http.StatusOK
		if existing == nil {
			stateValue, code = "creating", http.StatusCreated
		}
		respondJSON(w, code, toRouteTableResourceFromBinding(*binding, payload, tenant, workspace, http.MethodPut, stateValue))
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
		ref := routeTableRefKey(tenant, workspace, network, name)
		binding, err := store.GetResourceBinding(r.Context(), ref)
		if err != nil {
			respondProblem(w, http.StatusInternalServerError, "http://secapi.cloud/errors/internal", "Internal Server Error", "failed to load route table", r.URL.Path)
			return
		}
		if binding == nil {
			respondProblem(w, http.StatusNotFound, "http://secapi.cloud/errors/resource-not-found", "Not Found", "route table not found", r.URL.Path)
			return
		}
		if err := store.DeleteResourceBinding(r.Context(), ref); err != nil {
			respondProblem(w, http.StatusInternalServerError, "http://secapi.cloud/errors/internal", "Internal Server Error", "failed to delete route table", r.URL.Path)
			return
		}
		respondJSON(w, http.StatusAccepted, map[string]string{"status": "accepted"})
	}
}

func routeTableRefKey(tenant, workspace, network, name string) string {
	return "seca.network/v1/tenants/" + strings.ToLower(strings.TrimSpace(tenant)) +
		"/workspaces/" + strings.ToLower(strings.TrimSpace(workspace)) +
		"/networks/" + strings.ToLower(strings.TrimSpace(network)) +
		"/route-tables/" + strings.ToLower(strings.TrimSpace(name))
}

func networkRouteTableRefKey(tenant, workspace, network string) string {
	return "seca.network/v1/tenants/" + strings.ToLower(strings.TrimSpace(tenant)) +
		"/workspaces/" + strings.ToLower(strings.TrimSpace(workspace)) +
		"/networks/" + strings.ToLower(strings.TrimSpace(network)) +
		"#routeTableRef"
}

func parseRouteTableBinding(raw string) (routeTableBindingPayload, error) {
	var payload routeTableBindingPayload
	err := json.Unmarshal([]byte(raw), &payload)
	return payload, err
}

func toRouteTableResourceFromBinding(
	binding state.ResourceBinding,
	payload routeTableBindingPayload,
	tenant,
	workspace,
	verb,
	stateValue string,
) routeTableResource {
	createdAt := time.Now().UTC().Format(time.RFC3339)
	updatedAt := createdAt
	if !binding.CreatedAt.IsZero() {
		createdAt = binding.CreatedAt.UTC().Format(time.RFC3339)
	}
	if !binding.UpdatedAt.IsZero() {
		updatedAt = binding.UpdatedAt.UTC().Format(time.RFC3339)
	}
	return routeTableResource{
		Metadata: resourceMetadata{
			Name:            payload.Name,
			Provider:        "seca.network/v1",
			Resource:        "tenants/" + tenant + "/workspaces/" + workspace + "/networks/" + payload.Network + "/route-tables/" + payload.Name,
			Verb:            verb,
			CreatedAt:       createdAt,
			LastModifiedAt:  updatedAt,
			ResourceVersion: 1,
			APIVersion:      "v1",
			Kind:            "routing-table",
			Ref:             "seca.network/v1/tenants/" + tenant + "/workspaces/" + workspace + "/networks/" + payload.Network + "/route-tables/" + payload.Name,
			Tenant:          tenant,
			Workspace:       workspace,
			Network:         payload.Network,
			Region:          defaultRegion(strings.ToLower(strings.TrimSpace(payload.Region))),
		},
		Labels: payload.Labels,
		Spec:   payload.Spec,
		Status: routeTableStatusObject{State: stateValue},
	}
}

