package httpserver

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/eu-sovereign-cloud/secapi-proxy-hetzner/internal/state"
)

const resourceBindingKindNIC = "nic"

type nicIterator struct {
	Items    []nicResource      `json:"items"`
	Metadata responseMetaObject `json:"metadata"`
}

type nicResource struct {
	Metadata resourceMetadata  `json:"metadata"`
	Labels   map[string]string `json:"labels,omitempty"`
	Spec     nicSpec           `json:"spec"`
	Status   nicStatusObject   `json:"status"`
}

type nicSpec struct {
	Addresses    []string     `json:"addresses,omitempty"`
	PublicIPRefs *[]refObject `json:"publicIpRefs,omitempty"`
	SubnetRef    refObject    `json:"subnetRef"`
}

type nicStatusObject struct {
	State string `json:"state"`
}

type nicBindingPayload struct {
	Name   string            `json:"name"`
	Region string            `json:"region"`
	Labels map[string]string `json:"labels,omitempty"`
	Spec   nicSpec           `json:"spec"`
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
		bindings, err := store.ListResourceBindings(r.Context(), tenant, workspace, resourceBindingKindNIC)
		if err != nil {
			respondProblem(w, http.StatusInternalServerError, "http://secapi.cloud/errors/internal", "Internal Server Error", "failed to list nics", r.URL.Path)
			return
		}
		items := make([]nicResource, 0, len(bindings))
		for _, binding := range bindings {
			payload, err := parseNICBinding(binding.ProviderRef)
			if err != nil {
				continue
			}
			items = append(items, toNICResourceFromBinding(binding, payload, tenant, workspace, http.MethodGet, "active"))
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
		ref := nicRef(tenant, workspace, name)
		binding, err := store.GetResourceBinding(r.Context(), ref)
		if err != nil {
			respondProblem(w, http.StatusInternalServerError, "http://secapi.cloud/errors/internal", "Internal Server Error", "failed to load nic", r.URL.Path)
			return
		}
		if binding == nil {
			respondProblem(w, http.StatusNotFound, "http://secapi.cloud/errors/resource-not-found", "Not Found", "nic not found", r.URL.Path)
			return
		}
		payload, err := parseNICBinding(binding.ProviderRef)
		if err != nil {
			respondProblem(w, http.StatusInternalServerError, "http://secapi.cloud/errors/internal", "Internal Server Error", "invalid nic payload", r.URL.Path)
			return
		}
		respondJSON(w, http.StatusOK, toNICResourceFromBinding(*binding, payload, tenant, workspace, http.MethodGet, "active"))
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
		ref := nicRef(tenant, workspace, name)
		existing, err := store.GetResourceBinding(r.Context(), ref)
		if err != nil {
			respondProblem(w, http.StatusInternalServerError, "http://secapi.cloud/errors/internal", "Internal Server Error", "failed to load nic", r.URL.Path)
			return
		}
		payload := nicBindingPayload{
			Name:   name,
			Region: runtimeRegionOrDefault(req.Metadata.Region),
			Labels: req.Labels,
			Spec:   req.Spec,
		}
		raw, err := json.Marshal(payload)
		if err != nil {
			respondProblem(w, http.StatusInternalServerError, "http://secapi.cloud/errors/internal", "Internal Server Error", "failed to encode nic", r.URL.Path)
			return
		}
		if err := store.UpsertResourceBinding(r.Context(), state.ResourceBinding{
			Tenant:      tenant,
			Workspace:   workspace,
			Kind:        resourceBindingKindNIC,
			SecaRef:     ref,
			ProviderRef: string(raw),
			Status:      "active",
		}); err != nil {
			respondProblem(w, http.StatusInternalServerError, "http://secapi.cloud/errors/internal", "Internal Server Error", "failed to save nic", r.URL.Path)
			return
		}
		binding, err := store.GetResourceBinding(r.Context(), ref)
		if err != nil || binding == nil {
			respondProblem(w, http.StatusInternalServerError, "http://secapi.cloud/errors/internal", "Internal Server Error", "failed to load nic", r.URL.Path)
			return
		}
		stateValue, code := "updating", http.StatusOK
		if existing == nil {
			stateValue, code = "creating", http.StatusCreated
		}
		respondJSON(w, code, toNICResourceFromBinding(*binding, payload, tenant, workspace, http.MethodPut, stateValue))
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
		ref := nicRef(tenant, workspace, name)
		binding, err := store.GetResourceBinding(r.Context(), ref)
		if err != nil {
			respondProblem(w, http.StatusInternalServerError, "http://secapi.cloud/errors/internal", "Internal Server Error", "failed to load nic", r.URL.Path)
			return
		}
		if binding == nil {
			respondProblem(w, http.StatusNotFound, "http://secapi.cloud/errors/resource-not-found", "Not Found", "nic not found", r.URL.Path)
			return
		}
		if err := store.DeleteResourceBinding(r.Context(), ref); err != nil {
			respondProblem(w, http.StatusInternalServerError, "http://secapi.cloud/errors/internal", "Internal Server Error", "failed to delete nic", r.URL.Path)
			return
		}
		respondJSON(w, http.StatusAccepted, map[string]string{"status": "accepted"})
	}
}

func nicRef(tenant, workspace, name string) string {
	return "seca.network/v1/tenants/" + strings.ToLower(strings.TrimSpace(tenant)) +
		"/workspaces/" + strings.ToLower(strings.TrimSpace(workspace)) +
		"/nics/" + strings.ToLower(strings.TrimSpace(name))
}

func parseNICBinding(raw string) (nicBindingPayload, error) {
	var payload nicBindingPayload
	err := json.Unmarshal([]byte(raw), &payload)
	return payload, err
}

func toNICResourceFromBinding(
	binding state.ResourceBinding,
	payload nicBindingPayload,
	tenant,
	workspace,
	verb,
	stateValue string,
) nicResource {
	createdAt := time.Now().UTC().Format(time.RFC3339)
	updatedAt := createdAt
	if !binding.CreatedAt.IsZero() {
		createdAt = binding.CreatedAt.UTC().Format(time.RFC3339)
	}
	if !binding.UpdatedAt.IsZero() {
		updatedAt = binding.UpdatedAt.UTC().Format(time.RFC3339)
	}
	return nicResource{
		Metadata: resourceMetadata{
			Name:            payload.Name,
			Provider:        "seca.network/v1",
			Resource:        "tenants/" + tenant + "/workspaces/" + workspace + "/nics/" + payload.Name,
			Verb:            verb,
			CreatedAt:       createdAt,
			LastModifiedAt:  updatedAt,
			ResourceVersion: 1,
			APIVersion:      "v1",
			Kind:            "nic",
			Ref:             "seca.network/v1/tenants/" + tenant + "/workspaces/" + workspace + "/nics/" + payload.Name,
			Tenant:          tenant,
			Workspace:       workspace,
			Region:          defaultRegion(strings.ToLower(strings.TrimSpace(payload.Region))),
		},
		Labels: payload.Labels,
		Spec:   payload.Spec,
		Status: nicStatusObject{State: stateValue},
	}
}

