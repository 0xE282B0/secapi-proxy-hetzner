package httpserver

import (
	"encoding/json"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"
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

type authStore struct {
	mu              sync.RWMutex
	roles           map[string]authResource
	roleAssignments map[string]authResource
}

func newAuthStore() *authStore {
	return &authStore{
		roles:           map[string]authResource{},
		roleAssignments: map[string]authResource{},
	}
}

func listRoles(store *authStore) http.HandlerFunc {
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
		items := store.list(store.roles, tenant)
		respondJSON(w, http.StatusOK, authIterator{
			Items:    items,
			Metadata: responseMetaObject{Provider: "seca.authorization/v1", Resource: "tenants/" + tenant + "/roles", Verb: http.MethodGet},
		})
	}
}

func roleCRUD(store *authStore) http.HandlerFunc {
	return authCRUD(store, store.roles, "roles", "role")
}

func listRoleAssignments(store *authStore) http.HandlerFunc {
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
		items := store.list(store.roleAssignments, tenant)
		respondJSON(w, http.StatusOK, authIterator{
			Items:    items,
			Metadata: responseMetaObject{Provider: "seca.authorization/v1", Resource: "tenants/" + tenant + "/role-assignments", Verb: http.MethodGet},
		})
	}
}

func roleAssignmentCRUD(store *authStore) http.HandlerFunc {
	return authCRUD(store, store.roleAssignments, "role-assignments", "role-assignment")
}

func authCRUD(store *authStore, bucket map[string]authResource, collection, kind string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			store.mu.RLock()
			defer store.mu.RUnlock()
			tenant, name, key, ok := authPath(r, collection)
			if !ok {
				respondProblem(w, http.StatusBadRequest, "http://secapi.cloud/errors/invalid-request", "Bad Request", "tenant and name are required", r.URL.Path)
				return
			}
			item, found := bucket[key]
			if !found {
				respondProblem(w, http.StatusNotFound, "http://secapi.cloud/errors/resource-not-found", "Not Found", kind+" not found", r.URL.Path)
				return
			}
			item.Metadata.Verb = http.MethodGet
			item.Metadata.Tenant = tenant
			item.Metadata.Name = name
			item.Status.State = "active"
			respondJSON(w, http.StatusOK, item)
		case http.MethodPut:
			tenant, name, key, ok := authPath(r, collection)
			if !ok {
				respondProblem(w, http.StatusBadRequest, "http://secapi.cloud/errors/invalid-request", "Bad Request", "tenant and name are required", r.URL.Path)
				return
			}
			var req authResource
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				respondProblem(w, http.StatusBadRequest, "http://secapi.cloud/errors/invalid-request", "Bad Request", "invalid json body", r.URL.Path)
				return
			}
			now := time.Now().UTC().Format(time.RFC3339)
			store.mu.Lock()
			_, exists := bucket[key]
			state := "creating"
			code := http.StatusCreated
			if exists {
				state = "updating"
				code = http.StatusOK
			}
			item := authResource{
				Metadata: resourceMetadata{
					Name:            name,
					Provider:        "seca.authorization/v1",
					Resource:        "tenants/" + tenant + "/" + collection + "/" + name,
					Verb:            http.MethodPut,
					CreatedAt:       now,
					LastModifiedAt:  now,
					ResourceVersion: 1,
					APIVersion:      "v1",
					Kind:            kind,
					Ref:             "seca.authorization/v1/tenants/" + tenant + "/" + collection + "/" + name,
					Tenant:          tenant,
				},
				Labels: req.Labels,
				Spec:   req.Spec,
				Status: workspaceStatusObject{State: state},
			}
			bucket[key] = item
			store.mu.Unlock()
			respondJSON(w, code, item)
		case http.MethodDelete:
			_, _, key, ok := authPath(r, collection)
			if !ok {
				respondProblem(w, http.StatusBadRequest, "http://secapi.cloud/errors/invalid-request", "Bad Request", "tenant and name are required", r.URL.Path)
				return
			}
			store.mu.Lock()
			delete(bucket, key)
			store.mu.Unlock()
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

func (s *authStore) list(bucket map[string]authResource, tenant string) []authResource {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]authResource, 0)
	prefix := tenant + "/"
	for key, value := range bucket {
		if strings.HasPrefix(key, prefix) {
			out = append(out, value)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].Metadata.Name < out[j].Metadata.Name
	})
	return out
}
