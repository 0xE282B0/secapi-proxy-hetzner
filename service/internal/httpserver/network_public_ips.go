package httpserver

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/eu-sovereign-cloud/secapi-proxy-hetzner/internal/state"
)

type publicIPIterator struct {
	Items    []publicIPResource `json:"items"`
	Metadata responseMetaObject `json:"metadata"`
}

type publicIPResource struct {
	Metadata resourceMetadata     `json:"metadata"`
	Labels   map[string]string    `json:"labels,omitempty"`
	Spec     publicIPSpec         `json:"spec"`
	Status   publicIPStatusObject `json:"status"`
}

type publicIPSpec struct {
	Version string  `json:"version"`
	Address *string `json:"address,omitempty"`
}

type publicIPStatusObject struct {
	State string `json:"state"`
}

func listPublicIPs(store *state.Store) http.HandlerFunc {
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
		records := runtimeResourceState.listPublicIPsByScope(tenant, workspace)
		items := make([]publicIPResource, 0, len(records))
		for _, rec := range records {
			items = append(items, toRuntimePublicIPResource(rec, http.MethodGet, "active"))
		}
		respondJSON(w, http.StatusOK, publicIPIterator{
			Items:    items,
			Metadata: responseMetaObject{Provider: "seca.network/v1", Resource: "tenants/" + tenant + "/workspaces/" + workspace + "/public-ips", Verb: http.MethodGet},
		})
	}
}

func publicIPCRUD(store *state.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			getPublicIP(store)(w, r)
		case http.MethodPut:
			putPublicIP(store)(w, r)
		case http.MethodDelete:
			deletePublicIP(store)(w, r)
		default:
			respondProblem(w, http.StatusMethodNotAllowed, "http://secapi.cloud/errors/invalid-request", "Method Not Allowed", "Only GET, PUT and DELETE are supported", r.URL.Path)
		}
	}
}

func getPublicIP(store *state.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		tenant, workspace, name, ok := scopedNameFromPath(w, r, "public ip name is required")
		if !ok {
			return
		}
		if _, ok := workspaceExecutionContext(w, r, store, tenant, workspace); !ok {
			return
		}
		rec, ok := runtimeResourceState.getPublicIP(publicIPRef(tenant, workspace, name))
		if !ok {
			respondProblem(w, http.StatusNotFound, "http://secapi.cloud/errors/resource-not-found", "Not Found", "public ip not found", r.URL.Path)
			return
		}
		respondJSON(w, http.StatusOK, toRuntimePublicIPResource(rec, http.MethodGet, "active"))
	}
}

func putPublicIP(store *state.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		tenant, workspace, name, ok := scopedNameFromPath(w, r, "public ip name is required")
		if !ok {
			return
		}
		if _, ok := workspaceExecutionContext(w, r, store, tenant, workspace); !ok {
			return
		}
		var req publicIPResource
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			respondProblem(w, http.StatusBadRequest, "http://secapi.cloud/errors/invalid-request", "Bad Request", "invalid json body", r.URL.Path)
			return
		}
		if strings.TrimSpace(req.Spec.Version) == "" {
			respondProblem(w, http.StatusBadRequest, "http://secapi.cloud/errors/invalid-request", "Bad Request", "spec.version is required", r.URL.Path)
			return
		}
		region := strings.TrimSpace(req.Metadata.Region)
		if region == "" {
			region = "global"
		}
		now := time.Now().UTC().Format(time.RFC3339)
		rec, created := runtimeResourceState.upsertPublicIP(publicIPRef(tenant, workspace, name), publicIPRuntimeRecord{
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
		respondJSON(w, code, toRuntimePublicIPResource(rec, http.MethodPut, stateValue))
	}
}

func deletePublicIP(store *state.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		tenant, workspace, name, ok := scopedNameFromPath(w, r, "public ip name is required")
		if !ok {
			return
		}
		if _, ok := workspaceExecutionContext(w, r, store, tenant, workspace); !ok {
			return
		}
		if _, ok := runtimeResourceState.getPublicIP(publicIPRef(tenant, workspace, name)); !ok {
			respondProblem(w, http.StatusNotFound, "http://secapi.cloud/errors/resource-not-found", "Not Found", "public ip not found", r.URL.Path)
			return
		}
		runtimeResourceState.deletePublicIP(publicIPRef(tenant, workspace, name))
		respondJSON(w, http.StatusAccepted, map[string]string{"status": "accepted"})
	}
}

func publicIPRef(tenant, workspace, name string) string {
	return strings.ToLower(strings.TrimSpace(tenant)) + "/" + strings.ToLower(strings.TrimSpace(workspace)) + "/" + strings.ToLower(strings.TrimSpace(name))
}

func toRuntimePublicIPResource(rec publicIPRuntimeRecord, verb, state string) publicIPResource {
	return publicIPResource{
		Metadata: resourceMetadata{
			Name:            rec.Name,
			Provider:        "seca.network/v1",
			Resource:        "tenants/" + rec.Tenant + "/workspaces/" + rec.Workspace + "/public-ips/" + rec.Name,
			Verb:            verb,
			CreatedAt:       rec.CreatedAt,
			LastModifiedAt:  rec.LastModifiedAt,
			ResourceVersion: rec.ResourceVersion,
			APIVersion:      "v1",
			Kind:            "public-ip",
			Ref:             "seca.network/v1/tenants/" + rec.Tenant + "/workspaces/" + rec.Workspace + "/public-ips/" + rec.Name,
			Tenant:          rec.Tenant,
			Workspace:       rec.Workspace,
			Region:          rec.Region,
		},
		Labels: rec.Labels,
		Spec:   rec.Spec,
		Status: publicIPStatusObject{State: state},
	}
}
