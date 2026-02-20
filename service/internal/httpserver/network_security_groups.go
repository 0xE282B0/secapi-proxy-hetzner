package httpserver

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/eu-sovereign-cloud/secapi-proxy-hetzner/internal/state"
)

type securityGroupIterator struct {
	Items    []securityGroupResource `json:"items"`
	Metadata responseMetaObject      `json:"metadata"`
}

type securityGroupResource struct {
	Metadata resourceMetadata       `json:"metadata"`
	Labels   map[string]string      `json:"labels,omitempty"`
	Spec     securityGroupSpec      `json:"spec"`
	Status   securityGroupStatusObj `json:"status"`
}

type securityGroupSpec struct {
	Rules []securityGroupRuleSpec `json:"rules"`
}

type securityGroupRuleSpec struct {
	Direction string `json:"direction"`
}

type securityGroupStatusObj struct {
	State string `json:"state"`
}

func listSecurityGroups(store *state.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// TODO: Replace this in-memory security-group shim with provider-backed implementation.
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
		records := runtimeResourceState.listSecurityGroupsByScope(tenant, workspace)
		items := make([]securityGroupResource, 0, len(records))
		for _, rec := range records {
			items = append(items, toRuntimeSecurityGroupResource(rec, http.MethodGet, "active"))
		}
		respondJSON(w, http.StatusOK, securityGroupIterator{
			Items:    items,
			Metadata: responseMetaObject{Provider: "seca.network/v1", Resource: "tenants/" + tenant + "/workspaces/" + workspace + "/security-groups", Verb: http.MethodGet},
		})
	}
}

func securityGroupCRUD(store *state.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			getSecurityGroup(store)(w, r)
		case http.MethodPut:
			putSecurityGroup(store)(w, r)
		case http.MethodDelete:
			deleteSecurityGroup(store)(w, r)
		default:
			respondProblem(w, http.StatusMethodNotAllowed, "http://secapi.cloud/errors/invalid-request", "Method Not Allowed", "Only GET, PUT and DELETE are supported", r.URL.Path)
		}
	}
}

func getSecurityGroup(store *state.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		tenant, workspace, name, ok := scopedNameFromPath(w, r, "security group name is required")
		if !ok {
			return
		}
		if _, ok := workspaceExecutionContext(w, r, store, tenant, workspace); !ok {
			return
		}
		rec, ok := runtimeResourceState.getSecurityGroup(securityGroupRef(tenant, workspace, name))
		if !ok {
			respondProblem(w, http.StatusNotFound, "http://secapi.cloud/errors/resource-not-found", "Not Found", "security group not found", r.URL.Path)
			return
		}
		respondJSON(w, http.StatusOK, toRuntimeSecurityGroupResource(rec, http.MethodGet, "active"))
	}
}

func putSecurityGroup(store *state.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		tenant, workspace, name, ok := scopedNameFromPath(w, r, "security group name is required")
		if !ok {
			return
		}
		if _, ok := workspaceExecutionContext(w, r, store, tenant, workspace); !ok {
			return
		}
		var req securityGroupResource
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			respondProblem(w, http.StatusBadRequest, "http://secapi.cloud/errors/invalid-request", "Bad Request", "invalid json body", r.URL.Path)
			return
		}
		region := runtimeRegionOrDefault(req.Metadata.Region)
		now := time.Now().UTC().Format(time.RFC3339)
		rec, created := runtimeResourceState.upsertSecurityGroup(securityGroupRef(tenant, workspace, name), securityGroupRuntimeRecord{
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
		respondJSON(w, code, toRuntimeSecurityGroupResource(rec, http.MethodPut, stateValue))
	}
}

func deleteSecurityGroup(store *state.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		tenant, workspace, name, ok := scopedNameFromPath(w, r, "security group name is required")
		if !ok {
			return
		}
		if _, ok := workspaceExecutionContext(w, r, store, tenant, workspace); !ok {
			return
		}
		if _, ok := runtimeResourceState.getSecurityGroup(securityGroupRef(tenant, workspace, name)); !ok {
			respondProblem(w, http.StatusNotFound, "http://secapi.cloud/errors/resource-not-found", "Not Found", "security group not found", r.URL.Path)
			return
		}
		runtimeResourceState.deleteSecurityGroup(securityGroupRef(tenant, workspace, name))
		respondJSON(w, http.StatusAccepted, map[string]string{"status": "accepted"})
	}
}

func securityGroupRef(tenant, workspace, name string) string {
	return strings.ToLower(strings.TrimSpace(tenant)) + "/" + strings.ToLower(strings.TrimSpace(workspace)) + "/" + strings.ToLower(strings.TrimSpace(name))
}

func toRuntimeSecurityGroupResource(rec securityGroupRuntimeRecord, verb, state string) securityGroupResource {
	return securityGroupResource{
		Metadata: resourceMetadata{
			Name:            rec.Name,
			Provider:        "seca.network/v1",
			Resource:        "tenants/" + rec.Tenant + "/workspaces/" + rec.Workspace + "/security-groups/" + rec.Name,
			Verb:            verb,
			CreatedAt:       rec.CreatedAt,
			LastModifiedAt:  rec.LastModifiedAt,
			ResourceVersion: rec.ResourceVersion,
			APIVersion:      "v1",
			Kind:            "security-group",
			Ref:             "seca.network/v1/tenants/" + rec.Tenant + "/workspaces/" + rec.Workspace + "/security-groups/" + rec.Name,
			Tenant:          rec.Tenant,
			Workspace:       rec.Workspace,
			Region:          rec.Region,
		},
		Labels: rec.Labels,
		Spec:   rec.Spec,
		Status: securityGroupStatusObj{State: state},
	}
}
