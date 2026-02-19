package httpserver

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/eu-sovereign-cloud/secapi-proxy-hetzner/internal/state"
)

type workspaceIterator struct {
	Items    []workspaceResource `json:"items"`
	Metadata responseMetaObject  `json:"metadata"`
}

type workspaceResource struct {
	Metadata resourceMetadata      `json:"metadata"`
	Labels   map[string]string     `json:"labels,omitempty"`
	Spec     map[string]any        `json:"spec"`
	Status   workspaceStatusObject `json:"status"`
}

type workspaceStatusObject struct {
	State         string `json:"state"`
	ResourceCount *int   `json:"resourceCount,omitempty"`
}

func listWorkspaces(store *state.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			respondProblem(w, http.StatusMethodNotAllowed, "http://secapi.cloud/errors/invalid-request", "Method Not Allowed", "Only GET is supported", r.URL.Path)
			return
		}
		tenant := r.PathValue("tenant")
		if tenant == "" {
			respondProblem(w, http.StatusBadRequest, "http://secapi.cloud/errors/invalid-request", "Bad Request", "tenant is required", r.URL.Path)
			return
		}
		workspaces, err := store.ListWorkspaces(r.Context(), tenant)
		if err != nil {
			respondProblem(w, http.StatusInternalServerError, "http://secapi.cloud/errors/internal", "Internal Server Error", "failed to list workspaces", r.URL.Path)
			return
		}
		items := make([]workspaceResource, 0, len(workspaces))
		for _, item := range workspaces {
			items = append(items, toWorkspaceResource(item, http.MethodGet, false))
		}
		respondJSON(w, http.StatusOK, workspaceIterator{
			Items:    items,
			Metadata: responseMetaObject{Provider: "seca.workspace/v1", Resource: "tenants/" + tenant + "/workspaces", Verb: http.MethodGet},
		})
	}
}

func workspaceCRUD(store *state.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			getWorkspace(store)(w, r)
		case http.MethodPut:
			putWorkspace(store)(w, r)
		case http.MethodDelete:
			deleteWorkspace(store)(w, r)
		default:
			respondProblem(w, http.StatusMethodNotAllowed, "http://secapi.cloud/errors/invalid-request", "Method Not Allowed", "Only GET, PUT and DELETE are supported", r.URL.Path)
		}
	}
}

func getWorkspace(store *state.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		tenant := r.PathValue("tenant")
		name := strings.ToLower(r.PathValue("name"))
		if tenant == "" || name == "" {
			respondProblem(w, http.StatusBadRequest, "http://secapi.cloud/errors/invalid-request", "Bad Request", "tenant and workspace name are required", r.URL.Path)
			return
		}
		item, err := store.GetWorkspace(r.Context(), tenant, name)
		if err != nil {
			respondProblem(w, http.StatusInternalServerError, "http://secapi.cloud/errors/internal", "Internal Server Error", "failed to get workspace", r.URL.Path)
			return
		}
		if item == nil {
			respondProblem(w, http.StatusNotFound, "http://secapi.cloud/errors/resource-not-found", "Not Found", "workspace not found", r.URL.Path)
			return
		}
		respondJSON(w, http.StatusOK, toWorkspaceResource(*item, http.MethodGet, true))
	}
}

func putWorkspace(store *state.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		tenant := r.PathValue("tenant")
		name := strings.ToLower(r.PathValue("name"))
		if tenant == "" || name == "" {
			respondProblem(w, http.StatusBadRequest, "http://secapi.cloud/errors/invalid-request", "Bad Request", "tenant and workspace name are required", r.URL.Path)
			return
		}
		var req workspaceResource
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			respondProblem(w, http.StatusBadRequest, "http://secapi.cloud/errors/invalid-request", "Bad Request", "invalid json body", r.URL.Path)
			return
		}

		region := strings.TrimSpace(req.Metadata.Region)
		if region == "" {
			region = "fsn1"
		}

		existing, err := store.GetWorkspace(r.Context(), tenant, name)
		if err != nil {
			respondProblem(w, http.StatusInternalServerError, "http://secapi.cloud/errors/internal", "Internal Server Error", "failed to check existing workspace", r.URL.Path)
			return
		}
		statusState := "creating"
		code := http.StatusCreated
		if existing != nil {
			code = http.StatusOK
			statusState = "updating"
		}
		desired := state.WorkspaceResource{
			Tenant: tenant,
			Name:   name,
			Region: region,
			Labels: req.Labels,
			Spec:   req.Spec,
			Status: map[string]any{"state": statusState},
		}
		saved, err := store.UpsertWorkspace(r.Context(), desired)
		if err != nil {
			respondProblem(w, http.StatusInternalServerError, "http://secapi.cloud/errors/internal", "Internal Server Error", "failed to save workspace", r.URL.Path)
			return
		}
		respondJSON(w, code, toWorkspaceResource(*saved, http.MethodPut, false))
	}
}

func deleteWorkspace(store *state.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		tenant := r.PathValue("tenant")
		name := strings.ToLower(r.PathValue("name"))
		if tenant == "" || name == "" {
			respondProblem(w, http.StatusBadRequest, "http://secapi.cloud/errors/invalid-request", "Bad Request", "tenant and workspace name are required", r.URL.Path)
			return
		}
		deleted, err := store.SoftDeleteWorkspace(r.Context(), tenant, name)
		if err != nil {
			respondProblem(w, http.StatusInternalServerError, "http://secapi.cloud/errors/internal", "Internal Server Error", "failed to delete workspace", r.URL.Path)
			return
		}
		if !deleted {
			respondProblem(w, http.StatusNotFound, "http://secapi.cloud/errors/resource-not-found", "Not Found", "workspace not found", r.URL.Path)
			return
		}
		respondJSON(w, http.StatusAccepted, map[string]string{"status": "accepted"})
	}
}

func toWorkspaceResource(item state.WorkspaceResource, verb string, forceActive bool) workspaceResource {
	stateValue, _ := item.Status["state"].(string)
	if stateValue == "" {
		stateValue = "active"
	}
	if forceActive {
		stateValue = "active"
	}
	return workspaceResource{
		Metadata: resourceMetadata{
			Name:            item.Name,
			Provider:        "seca.workspace/v1",
			Resource:        "tenants/" + item.Tenant + "/workspaces/" + item.Name,
			Verb:            verb,
			CreatedAt:       item.CreatedAt.Format(time.RFC3339),
			LastModifiedAt:  item.UpdatedAt.Format(time.RFC3339),
			ResourceVersion: item.ResourceVersion,
			APIVersion:      "v1",
			Kind:            "workspace",
			Ref:             "seca.workspace/v1/tenants/" + item.Tenant + "/workspaces/" + item.Name,
			Tenant:          item.Tenant,
			Region:          item.Region,
		},
		Labels: item.Labels,
		Spec:   item.Spec,
		Status: workspaceStatusObject{State: stateValue},
	}
}
