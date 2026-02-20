package httpserver

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/eu-sovereign-cloud/secapi-proxy-hetzner/internal/state"
)

const resourceBindingKindPublicIP = "public-ip"

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

type publicIPBindingPayload struct {
	Name   string            `json:"name"`
	Region string            `json:"region"`
	Labels map[string]string `json:"labels,omitempty"`
	Spec   publicIPSpec      `json:"spec"`
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
		bindings, err := store.ListResourceBindings(r.Context(), tenant, workspace, resourceBindingKindPublicIP)
		if err != nil {
			respondProblem(w, http.StatusInternalServerError, "http://secapi.cloud/errors/internal", "Internal Server Error", "failed to list public ips", r.URL.Path)
			return
		}
		items := make([]publicIPResource, 0, len(bindings))
		for _, binding := range bindings {
			payload, err := parsePublicIPBinding(binding.ProviderRef)
			if err != nil {
				continue
			}
			items = append(items, toPublicIPResourceFromBinding(binding, payload, tenant, workspace, http.MethodGet, "active"))
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
		ref := publicIPRef(tenant, workspace, name)
		binding, err := store.GetResourceBinding(r.Context(), ref)
		if err != nil {
			respondProblem(w, http.StatusInternalServerError, "http://secapi.cloud/errors/internal", "Internal Server Error", "failed to load public ip", r.URL.Path)
			return
		}
		if binding == nil {
			respondProblem(w, http.StatusNotFound, "http://secapi.cloud/errors/resource-not-found", "Not Found", "public ip not found", r.URL.Path)
			return
		}
		payload, err := parsePublicIPBinding(binding.ProviderRef)
		if err != nil {
			respondProblem(w, http.StatusInternalServerError, "http://secapi.cloud/errors/internal", "Internal Server Error", "invalid public ip payload", r.URL.Path)
			return
		}
		respondJSON(w, http.StatusOK, toPublicIPResourceFromBinding(*binding, payload, tenant, workspace, http.MethodGet, "active"))
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
		ref := publicIPRef(tenant, workspace, name)
		existing, err := store.GetResourceBinding(r.Context(), ref)
		if err != nil {
			respondProblem(w, http.StatusInternalServerError, "http://secapi.cloud/errors/internal", "Internal Server Error", "failed to load public ip", r.URL.Path)
			return
		}
		payload := publicIPBindingPayload{
			Name:   name,
			Region: runtimeRegionOrDefault(req.Metadata.Region),
			Labels: req.Labels,
			Spec:   req.Spec,
		}
		raw, err := json.Marshal(payload)
		if err != nil {
			respondProblem(w, http.StatusInternalServerError, "http://secapi.cloud/errors/internal", "Internal Server Error", "failed to encode public ip", r.URL.Path)
			return
		}
		if err := store.UpsertResourceBinding(r.Context(), state.ResourceBinding{
			Tenant:      tenant,
			Workspace:   workspace,
			Kind:        resourceBindingKindPublicIP,
			SecaRef:     ref,
			ProviderRef: string(raw),
			Status:      "active",
		}); err != nil {
			respondProblem(w, http.StatusInternalServerError, "http://secapi.cloud/errors/internal", "Internal Server Error", "failed to save public ip", r.URL.Path)
			return
		}
		binding, err := store.GetResourceBinding(r.Context(), ref)
		if err != nil || binding == nil {
			respondProblem(w, http.StatusInternalServerError, "http://secapi.cloud/errors/internal", "Internal Server Error", "failed to load public ip", r.URL.Path)
			return
		}
		stateValue, code := "updating", http.StatusOK
		if existing == nil {
			stateValue, code = "creating", http.StatusCreated
		}
		respondJSON(w, code, toPublicIPResourceFromBinding(*binding, payload, tenant, workspace, http.MethodPut, stateValue))
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
		ref := publicIPRef(tenant, workspace, name)
		binding, err := store.GetResourceBinding(r.Context(), ref)
		if err != nil {
			respondProblem(w, http.StatusInternalServerError, "http://secapi.cloud/errors/internal", "Internal Server Error", "failed to load public ip", r.URL.Path)
			return
		}
		if binding == nil {
			respondProblem(w, http.StatusNotFound, "http://secapi.cloud/errors/resource-not-found", "Not Found", "public ip not found", r.URL.Path)
			return
		}
		if err := store.DeleteResourceBinding(r.Context(), ref); err != nil {
			respondProblem(w, http.StatusInternalServerError, "http://secapi.cloud/errors/internal", "Internal Server Error", "failed to delete public ip", r.URL.Path)
			return
		}
		respondJSON(w, http.StatusAccepted, map[string]string{"status": "accepted"})
	}
}

func publicIPRef(tenant, workspace, name string) string {
	return "seca.network/v1/tenants/" + strings.ToLower(strings.TrimSpace(tenant)) +
		"/workspaces/" + strings.ToLower(strings.TrimSpace(workspace)) +
		"/public-ips/" + strings.ToLower(strings.TrimSpace(name))
}

func parsePublicIPBinding(raw string) (publicIPBindingPayload, error) {
	var payload publicIPBindingPayload
	err := json.Unmarshal([]byte(raw), &payload)
	return payload, err
}

func toPublicIPResourceFromBinding(
	binding state.ResourceBinding,
	payload publicIPBindingPayload,
	tenant,
	workspace,
	verb,
	stateValue string,
) publicIPResource {
	createdAt := time.Now().UTC().Format(time.RFC3339)
	updatedAt := createdAt
	if !binding.CreatedAt.IsZero() {
		createdAt = binding.CreatedAt.UTC().Format(time.RFC3339)
	}
	if !binding.UpdatedAt.IsZero() {
		updatedAt = binding.UpdatedAt.UTC().Format(time.RFC3339)
	}
	return publicIPResource{
		Metadata: resourceMetadata{
			Name:            payload.Name,
			Provider:        "seca.network/v1",
			Resource:        "tenants/" + tenant + "/workspaces/" + workspace + "/public-ips/" + payload.Name,
			Verb:            verb,
			CreatedAt:       createdAt,
			LastModifiedAt:  updatedAt,
			ResourceVersion: 1,
			APIVersion:      "v1",
			Kind:            "public-ip",
			Ref:             "seca.network/v1/tenants/" + tenant + "/workspaces/" + workspace + "/public-ips/" + payload.Name,
			Tenant:          tenant,
			Workspace:       workspace,
			Region:          defaultRegion(strings.ToLower(strings.TrimSpace(payload.Region))),
		},
		Labels: payload.Labels,
		Spec:   payload.Spec,
		Status: publicIPStatusObject{State: stateValue},
	}
}

