package httpserver

import (
	"encoding/json"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/eu-sovereign-cloud/secapi-proxy-hetzner/internal/state"
)

type authIterator struct {
	Items    []authResource     `json:"items"`
	Metadata responseMetaObject `json:"metadata"`
}

type authResource struct {
	Metadata resourceMetadata      `json:"metadata"`
	Labels   map[string]string     `json:"labels,omitempty"`
	Spec     map[string]any        `json:"spec"`
	Status   workspaceStatusObject `json:"status"`
}

func listRoles(store *state.Store) http.HandlerFunc {
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
		items, err := store.ListRoles(r.Context(), tenant)
		if err != nil {
			respondFromError(w, err, r.URL.Path)
			return
		}
		out := make([]authResource, 0, len(items))
		for _, item := range items {
			out = append(out, toAuthResource("roles", "role", http.MethodGet, item))
		}
		sort.Slice(out, func(i, j int) bool {
			return out[i].Metadata.Name < out[j].Metadata.Name
		})
		respondJSON(w, http.StatusOK, authIterator{
			Items:    out,
			Metadata: responseMetaObject{Provider: "seca.authorization/v1", Resource: "tenants/" + tenant + "/roles", Verb: http.MethodGet},
		})
	}
}

func roleCRUD(store *state.Store) http.HandlerFunc {
	return authCRUD(store, "roles", "role")
}

func listRoleAssignments(store *state.Store) http.HandlerFunc {
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
		items, err := store.ListRoleAssignments(r.Context(), tenant)
		if err != nil {
			respondFromError(w, err, r.URL.Path)
			return
		}
		out := make([]authResource, 0, len(items))
		for _, item := range items {
			out = append(out, toAuthResource("role-assignments", "role-assignment", http.MethodGet, item))
		}
		sort.Slice(out, func(i, j int) bool {
			return out[i].Metadata.Name < out[j].Metadata.Name
		})
		respondJSON(w, http.StatusOK, authIterator{
			Items:    out,
			Metadata: responseMetaObject{Provider: "seca.authorization/v1", Resource: "tenants/" + tenant + "/role-assignments", Verb: http.MethodGet},
		})
	}
}

func roleAssignmentCRUD(store *state.Store) http.HandlerFunc {
	return authCRUD(store, "role-assignments", "role-assignment")
}

func authCRUD(store *state.Store, collection, kind string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			tenant, name, _, ok := authPath(r, collection)
			if !ok {
				respondProblem(w, http.StatusBadRequest, "http://secapi.cloud/errors/invalid-request", "Bad Request", "tenant and name are required", r.URL.Path)
				return
			}
			item, err := getAuthResource(r, store, collection, tenant, name)
			if err != nil {
				respondFromError(w, err, r.URL.Path)
				return
			}
			if item == nil {
				respondProblem(w, http.StatusNotFound, "http://secapi.cloud/errors/resource-not-found", "Not Found", kind+" not found", r.URL.Path)
				return
			}
			out := toAuthResource(collection, kind, http.MethodGet, *item)
			out.Status.State = "active"
			respondJSON(w, http.StatusOK, out)
		case http.MethodPut:
			tenant, name, _, ok := authPath(r, collection)
			if !ok {
				respondProblem(w, http.StatusBadRequest, "http://secapi.cloud/errors/invalid-request", "Bad Request", "tenant and name are required", r.URL.Path)
				return
			}
			var req authResource
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				respondProblem(w, http.StatusBadRequest, "http://secapi.cloud/errors/invalid-request", "Bad Request", "invalid json body", r.URL.Path)
				return
			}

			existing, err := getAuthResource(r, store, collection, tenant, name)
			if err != nil {
				respondFromError(w, err, r.URL.Path)
				return
			}
			stateValue := "creating"
			code := http.StatusCreated
			if existing != nil {
				stateValue = "updating"
				code = http.StatusOK
			}

			dbRecord := state.AuthResource{
				Tenant: tenant,
				Name:   name,
				Labels: req.Labels,
				Spec:   req.Spec,
				Status: map[string]any{"state": stateValue},
			}
			if err := upsertAuthResource(r, store, collection, dbRecord); err != nil {
				respondFromError(w, err, r.URL.Path)
				return
			}

			stored, err := getAuthResource(r, store, collection, tenant, name)
			if err != nil {
				respondFromError(w, err, r.URL.Path)
				return
			}
			if stored == nil {
				respondProblem(w, http.StatusServiceUnavailable, "http://secapi.cloud/errors/provider-unavailable", "Service Unavailable", "failed to persist auth resource", r.URL.Path)
				return
			}
			out := toAuthResource(collection, kind, http.MethodPut, *stored)
			out.Status.State = stateValue
			respondJSON(w, code, out)
		case http.MethodDelete:
			tenant, name, _, ok := authPath(r, collection)
			if !ok {
				respondProblem(w, http.StatusBadRequest, "http://secapi.cloud/errors/invalid-request", "Bad Request", "tenant and name are required", r.URL.Path)
				return
			}
			if err := softDeleteAuthResource(r, store, collection, tenant, name); err != nil {
				respondFromError(w, err, r.URL.Path)
				return
			}
			w.WriteHeader(http.StatusAccepted)
		default:
			respondProblem(w, http.StatusMethodNotAllowed, "http://secapi.cloud/errors/invalid-request", "Method Not Allowed", "Only GET, PUT and DELETE are supported", r.URL.Path)
		}
	}
}

func authPath(r *http.Request, collection string) (tenant, name, key string, ok bool) {
	tenant = r.PathValue("tenant")
	name = strings.ToLower(r.PathValue("name"))
	if tenant == "" || name == "" {
		return "", "", "", false
	}
	return tenant, name, tenant + "/" + collection + "/" + name, true
}

func getAuthResource(r *http.Request, store *state.Store, collection, tenant, name string) (*state.AuthResource, error) {
	switch collection {
	case "roles":
		return store.GetRole(r.Context(), tenant, name)
	case "role-assignments":
		return store.GetRoleAssignment(r.Context(), tenant, name)
	default:
		return nil, nil
	}
}

func upsertAuthResource(r *http.Request, store *state.Store, collection string, resource state.AuthResource) error {
	switch collection {
	case "roles":
		return store.UpsertRole(r.Context(), resource)
	case "role-assignments":
		return store.UpsertRoleAssignment(r.Context(), resource)
	default:
		return nil
	}
}

func softDeleteAuthResource(r *http.Request, store *state.Store, collection, tenant, name string) error {
	switch collection {
	case "roles":
		_, err := store.SoftDeleteRole(r.Context(), tenant, name)
		return err
	case "role-assignments":
		_, err := store.SoftDeleteRoleAssignment(r.Context(), tenant, name)
		return err
	default:
		return nil
	}
}

func toAuthResource(collection, kind, verb string, resource state.AuthResource) authResource {
	now := time.Now().UTC().Format(time.RFC3339)
	statusState := "active"
	if rawState, ok := resource.Status["state"].(string); ok && rawState != "" {
		statusState = strings.ToLower(rawState)
	}
	return authResource{
		Metadata: resourceMetadata{
			Name:            resource.Name,
			Provider:        "seca.authorization/v1",
			Resource:        "tenants/" + resource.Tenant + "/" + collection + "/" + resource.Name,
			Verb:            verb,
			CreatedAt:       now,
			LastModifiedAt:  now,
			ResourceVersion: resource.ResourceVersion,
			APIVersion:      "v1",
			Kind:            kind,
			Ref:             "seca.authorization/v1/tenants/" + resource.Tenant + "/" + collection + "/" + resource.Name,
			Tenant:          resource.Tenant,
		},
		Labels: resource.Labels,
		Spec:   resource.Spec,
		Status: workspaceStatusObject{State: statusState},
	}
}
