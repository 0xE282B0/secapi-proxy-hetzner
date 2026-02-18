package httpserver

import (
	"encoding/json"
	"net/http"
	"strings"
	"sync"
	"time"
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

type workspaceStore struct {
	mu    sync.RWMutex
	items map[string]workspaceResource
}

func newWorkspaceStore() *workspaceStore {
	return &workspaceStore{items: make(map[string]workspaceResource)}
}

func (s *workspaceStore) upsert(tenant, name string, resource workspaceResource) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.items[tenant+"/"+name] = resource
}

func (s *workspaceStore) get(tenant, name string) (workspaceResource, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	item, ok := s.items[tenant+"/"+name]
	return item, ok
}

func (s *workspaceStore) list(tenant string) []workspaceResource {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]workspaceResource, 0, len(s.items))
	for key, item := range s.items {
		if strings.HasPrefix(key, tenant+"/") {
			out = append(out, item)
		}
	}
	return out
}

func (s *workspaceStore) delete(tenant, name string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.items, tenant+"/"+name)
}

func listWorkspaces(store *workspaceStore) http.HandlerFunc {
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
		items := store.list(tenant)
		respondJSON(w, http.StatusOK, workspaceIterator{
			Items:    items,
			Metadata: responseMetaObject{Provider: "seca.workspace/v1", Resource: "tenants/" + tenant + "/workspaces", Verb: http.MethodGet},
		})
	}
}

func workspaceCRUD(store *workspaceStore) http.HandlerFunc {
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

func getWorkspace(store *workspaceStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		tenant := r.PathValue("tenant")
		name := strings.ToLower(r.PathValue("name"))
		if tenant == "" || name == "" {
			respondProblem(w, http.StatusBadRequest, "http://secapi.cloud/errors/invalid-request", "Bad Request", "tenant and workspace name are required", r.URL.Path)
			return
		}
		item, ok := store.get(tenant, name)
		if !ok {
			respondProblem(w, http.StatusNotFound, "http://secapi.cloud/errors/resource-not-found", "Not Found", "workspace not found", r.URL.Path)
			return
		}
		item.Metadata.Verb = http.MethodGet
		item.Status.State = "active"
		respondJSON(w, http.StatusOK, item)
	}
}

func putWorkspace(store *workspaceStore) http.HandlerFunc {
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

		now := time.Now().UTC().Format(time.RFC3339)
		_, exists := store.get(tenant, name)
		status := "creating"
		code := http.StatusCreated
		if exists {
			status = "updating"
			code = http.StatusOK
		}
		item := workspaceResource{
			Metadata: resourceMetadata{
				Name:            name,
				Provider:        "seca.workspace/v1",
				Resource:        "tenants/" + tenant + "/workspaces/" + name,
				Verb:            http.MethodPut,
				CreatedAt:       now,
				LastModifiedAt:  now,
				ResourceVersion: 1,
				APIVersion:      "v1",
				Kind:            "workspace",
				Ref:             "seca.workspace/v1/tenants/" + tenant + "/workspaces/" + name,
				Tenant:          tenant,
				Region:          req.Metadata.Region,
			},
			Labels: req.Labels,
			Spec:   req.Spec,
			Status: workspaceStatusObject{State: status},
		}
		if item.Metadata.Region == "" {
			item.Metadata.Region = "fsn1"
		}
		store.upsert(tenant, name, item)
		respondJSON(w, code, item)
	}
}

func deleteWorkspace(store *workspaceStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		tenant := r.PathValue("tenant")
		name := strings.ToLower(r.PathValue("name"))
		if tenant == "" || name == "" {
			respondProblem(w, http.StatusBadRequest, "http://secapi.cloud/errors/invalid-request", "Bad Request", "tenant and workspace name are required", r.URL.Path)
			return
		}
		if _, ok := store.get(tenant, name); !ok {
			respondProblem(w, http.StatusNotFound, "http://secapi.cloud/errors/resource-not-found", "Not Found", "workspace not found", r.URL.Path)
			return
		}
		store.delete(tenant, name)
		respondJSON(w, http.StatusAccepted, map[string]string{"status": "accepted"})
	}
}
