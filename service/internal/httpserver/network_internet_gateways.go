package httpserver

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/eu-sovereign-cloud/secapi-proxy-hetzner/internal/state"
)

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

func listInternetGateways(store *state.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// TODO: Replace this in-memory internet-gateway shim with provider-backed implementation.
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
		records := runtimeResourceState.listInternetGatewaysByScope(tenant, workspace)
		items := make([]internetGatewayResource, 0, len(records))
		for _, rec := range records {
			items = append(items, toRuntimeInternetGatewayResource(rec, http.MethodGet, "active"))
		}
		respondJSON(w, http.StatusOK, internetGatewayIterator{
			Items:    items,
			Metadata: responseMetaObject{Provider: "seca.network/v1", Resource: "tenants/" + tenant + "/workspaces/" + workspace + "/internet-gateways", Verb: http.MethodGet},
		})
	}
}

func internetGatewayCRUD(store *state.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			getInternetGateway(store)(w, r)
		case http.MethodPut:
			putInternetGateway(store)(w, r)
		case http.MethodDelete:
			deleteInternetGateway(store)(w, r)
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
		rec, ok := runtimeResourceState.getInternetGateway(internetGatewayRef(tenant, workspace, name))
		if !ok {
			respondProblem(w, http.StatusNotFound, "http://secapi.cloud/errors/resource-not-found", "Not Found", "internet gateway not found", r.URL.Path)
			return
		}
		respondJSON(w, http.StatusOK, toRuntimeInternetGatewayResource(rec, http.MethodGet, "active"))
	}
}

func putInternetGateway(store *state.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		tenant, workspace, name, ok := scopedNameFromPath(w, r, "internet gateway name is required")
		if !ok {
			return
		}
		if _, ok := workspaceExecutionContext(w, r, store, tenant, workspace); !ok {
			return
		}

		var req internetGatewayResource
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			respondProblem(w, http.StatusBadRequest, "http://secapi.cloud/errors/invalid-request", "Bad Request", "invalid json body", r.URL.Path)
			return
		}
		region := runtimeRegionOrDefault(req.Metadata.Region)
		now := time.Now().UTC().Format(time.RFC3339)
		rec, created := runtimeResourceState.upsertInternetGateway(internetGatewayRef(tenant, workspace, name), internetGatewayRuntimeRecord{
			Tenant:         tenant,
			Workspace:      workspace,
			Name:           name,
			Region:         region,
			Labels:         req.Labels,
			Spec:           req.Spec,
			CreatedAt:      now,
			LastModifiedAt: now,
		})
		stateValue, code := upsertStateAndCode(created)
		respondJSON(w, code, toRuntimeInternetGatewayResource(rec, http.MethodPut, stateValue))
	}
}

func deleteInternetGateway(store *state.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		tenant, workspace, name, ok := scopedNameFromPath(w, r, "internet gateway name is required")
		if !ok {
			return
		}
		if _, ok := workspaceExecutionContext(w, r, store, tenant, workspace); !ok {
			return
		}
		if _, ok := runtimeResourceState.getInternetGateway(internetGatewayRef(tenant, workspace, name)); !ok {
			respondProblem(w, http.StatusNotFound, "http://secapi.cloud/errors/resource-not-found", "Not Found", "internet gateway not found", r.URL.Path)
			return
		}
		runtimeResourceState.deleteInternetGateway(internetGatewayRef(tenant, workspace, name))
		respondJSON(w, http.StatusAccepted, map[string]string{"status": "accepted"})
	}
}

func internetGatewayRef(tenant, workspace, name string) string {
	return strings.ToLower(strings.TrimSpace(tenant)) + "/" + strings.ToLower(strings.TrimSpace(workspace)) + "/" + strings.ToLower(strings.TrimSpace(name))
}

func toRuntimeInternetGatewayResource(rec internetGatewayRuntimeRecord, verb, state string) internetGatewayResource {
	return internetGatewayResource{
		Metadata: resourceMetadata{
			Name:            rec.Name,
			Provider:        "seca.network/v1",
			Resource:        "tenants/" + rec.Tenant + "/workspaces/" + rec.Workspace + "/internet-gateways/" + rec.Name,
			Verb:            verb,
			CreatedAt:       rec.CreatedAt,
			LastModifiedAt:  rec.LastModifiedAt,
			ResourceVersion: rec.ResourceVersion,
			APIVersion:      "v1",
			Kind:            "internet-gateway",
			Ref:             "seca.network/v1/tenants/" + rec.Tenant + "/workspaces/" + rec.Workspace + "/internet-gateways/" + rec.Name,
			Tenant:          rec.Tenant,
			Workspace:       rec.Workspace,
			Region:          rec.Region,
		},
		Labels: rec.Labels,
		Spec:   rec.Spec,
		Status: internetGatewayStatusObject{State: state},
	}
}
