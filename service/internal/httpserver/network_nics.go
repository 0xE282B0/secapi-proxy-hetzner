package httpserver

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/eu-sovereign-cloud/secapi-proxy-hetzner/internal/state"
)

type nicIterator struct {
	Items    []nicResource      `json:"items"`
	Metadata responseMetaObject `json:"metadata"`
}

type nicResource struct {
	Metadata resourceMetadata `json:"metadata"`
	Labels   map[string]string `json:"labels,omitempty"`
	Spec     nicSpec          `json:"spec"`
	Status   nicStatusObject  `json:"status"`
}

type nicSpec struct {
	Addresses    []string      `json:"addresses,omitempty"`
	PublicIPRefs *[]refObject  `json:"publicIpRefs,omitempty"`
	SubnetRef    refObject     `json:"subnetRef"`
}

type nicStatusObject struct {
	State string `json:"state"`
}

func listNICs(store *state.Store) http.HandlerFunc {
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
		records := runtimeResourceState.listNICsByScope(tenant, workspace)
		items := make([]nicResource, 0, len(records))
		for _, rec := range records {
			items = append(items, toRuntimeNICResource(rec, http.MethodGet, "active"))
		}
		respondJSON(w, http.StatusOK, nicIterator{
			Items:    items,
			Metadata: responseMetaObject{Provider: "seca.network/v1", Resource: "tenants/" + tenant + "/workspaces/" + workspace + "/nics", Verb: http.MethodGet},
		})
	}
}

func nicCRUD(store *state.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			getNIC(store)(w, r)
		case http.MethodPut:
			putNIC(store)(w, r)
		case http.MethodDelete:
			deleteNIC(store)(w, r)
		default:
			respondProblem(w, http.StatusMethodNotAllowed, "http://secapi.cloud/errors/invalid-request", "Method Not Allowed", "Only GET, PUT and DELETE are supported", r.URL.Path)
		}
	}
}

func getNIC(store *state.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		tenant, workspace, name, ok := scopedNameFromPath(w, r, "nic name is required")
		if !ok {
			return
		}
		if _, ok := workspaceExecutionContext(w, r, store, tenant, workspace); !ok {
			return
		}
		rec, ok := runtimeResourceState.getNIC(nicRef(tenant, workspace, name))
		if !ok {
			respondProblem(w, http.StatusNotFound, "http://secapi.cloud/errors/resource-not-found", "Not Found", "nic not found", r.URL.Path)
			return
		}
		respondJSON(w, http.StatusOK, toRuntimeNICResource(rec, http.MethodGet, "active"))
	}
}

func putNIC(store *state.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		tenant, workspace, name, ok := scopedNameFromPath(w, r, "nic name is required")
		if !ok {
			return
		}
		if _, ok := workspaceExecutionContext(w, r, store, tenant, workspace); !ok {
			return
		}
		var req nicResource
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			respondProblem(w, http.StatusBadRequest, "http://secapi.cloud/errors/invalid-request", "Bad Request", "invalid json body", r.URL.Path)
			return
		}
		if strings.TrimSpace(req.Spec.SubnetRef.Resource) == "" {
			respondProblem(w, http.StatusBadRequest, "http://secapi.cloud/errors/invalid-request", "Bad Request", "spec.subnetRef is required", r.URL.Path)
			return
		}
		region := strings.TrimSpace(req.Metadata.Region)
		if region == "" {
			region = "global"
		}
		now := time.Now().UTC().Format(time.RFC3339)
		rec, created := runtimeResourceState.upsertNIC(nicRef(tenant, workspace, name), nicRuntimeRecord{
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
		respondJSON(w, code, toRuntimeNICResource(rec, http.MethodPut, stateValue))
	}
}

func deleteNIC(store *state.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		tenant, workspace, name, ok := scopedNameFromPath(w, r, "nic name is required")
		if !ok {
			return
		}
		if _, ok := workspaceExecutionContext(w, r, store, tenant, workspace); !ok {
			return
		}
		if _, ok := runtimeResourceState.getNIC(nicRef(tenant, workspace, name)); !ok {
			respondProblem(w, http.StatusNotFound, "http://secapi.cloud/errors/resource-not-found", "Not Found", "nic not found", r.URL.Path)
			return
		}
		runtimeResourceState.deleteNIC(nicRef(tenant, workspace, name))
		respondJSON(w, http.StatusAccepted, map[string]string{"status": "accepted"})
	}
}

func nicRef(tenant, workspace, name string) string {
	return strings.ToLower(strings.TrimSpace(tenant)) + "/" + strings.ToLower(strings.TrimSpace(workspace)) + "/" + strings.ToLower(strings.TrimSpace(name))
}

func toRuntimeNICResource(rec nicRuntimeRecord, verb, state string) nicResource {
	return nicResource{
		Metadata: resourceMetadata{
			Name:            rec.Name,
			Provider:        "seca.network/v1",
			Resource:        "tenants/" + rec.Tenant + "/workspaces/" + rec.Workspace + "/nics/" + rec.Name,
			Verb:            verb,
			CreatedAt:       rec.CreatedAt,
			LastModifiedAt:  rec.LastModifiedAt,
			ResourceVersion: rec.ResourceVersion,
			APIVersion:      "v1",
			Kind:            "nic",
			Ref:             "seca.network/v1/tenants/" + rec.Tenant + "/workspaces/" + rec.Workspace + "/nics/" + rec.Name,
			Tenant:          rec.Tenant,
			Workspace:       rec.Workspace,
			Region:          rec.Region,
		},
		Labels: rec.Labels,
		Spec:   rec.Spec,
		Status: nicStatusObject{State: state},
	}
}
