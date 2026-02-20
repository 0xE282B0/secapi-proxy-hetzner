package httpserver

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/eu-sovereign-cloud/secapi-proxy-hetzner/internal/provider/hetzner"
	"github.com/eu-sovereign-cloud/secapi-proxy-hetzner/internal/state"
)

const resourceBindingKindSecurityGroup = "security-group"

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

type securityGroupBindingPayload struct {
	Name   string                `json:"name"`
	Region string                `json:"region"`
	Labels map[string]string     `json:"labels,omitempty"`
	Spec   securityGroupSpec     `json:"spec"`
}

func listSecurityGroups(provider NetworkProvider, store *state.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			respondProblem(w, http.StatusMethodNotAllowed, "http://secapi.cloud/errors/invalid-request", "Method Not Allowed", "Only GET is supported", r.URL.Path)
			return
		}
		tenant, workspace, ok := scopeFromPath(w, r)
		if !ok {
			return
		}
		workspaceRegion, ok := workspaceRegionOrDefault(r.Context(), store, tenant, workspace)
		if !ok {
			respondProblem(w, http.StatusInternalServerError, "http://secapi.cloud/errors/internal", "Internal Server Error", "failed to resolve workspace", r.URL.Path)
			return
		}
		ctx, ok := workspaceExecutionContext(w, r, store, tenant, workspace)
		if !ok {
			return
		}
		itemsFromProvider, err := provider.ListSecurityGroups(ctx)
		if err != nil {
			respondFromError(w, err, r.URL.Path)
			return
		}
		bindings, err := store.ListResourceBindings(r.Context(), tenant, workspace, resourceBindingKindSecurityGroup)
		if err != nil {
			respondProblem(w, http.StatusInternalServerError, "http://secapi.cloud/errors/internal", "Internal Server Error", "failed to list security groups", r.URL.Path)
			return
		}
		bindingsByName := make(map[string]state.ResourceBinding, len(bindings))
		for _, binding := range bindings {
			name := strings.TrimSpace(resourceNameFromRef(binding.SecaRef))
			if name != "" {
				bindingsByName[strings.ToLower(name)] = binding
			}
		}
		items := make([]securityGroupResource, 0, len(itemsFromProvider))
		for _, item := range itemsFromProvider {
			payload := securityGroupBindingPayload{
				Name:   item.Name,
				Region: workspaceRegion,
				Labels: item.Labels,
				Spec:   securityGroupSpec{Rules: toSecurityGroupRuleSpecs(item.Rules)},
			}
			binding, hasBinding := bindingsByName[item.Name]
			if hasBinding {
				parsed, err := parseSecurityGroupBinding(binding.ProviderRef)
				if err == nil {
					payload = parsed
				}
			}
			if len(payload.Labels) == 0 {
				payload.Labels = item.Labels
			}
			if len(payload.Spec.Rules) == 0 {
				payload.Spec.Rules = toSecurityGroupRuleSpecs(item.Rules)
			}
			if payload.Region == "" {
				payload.Region = workspaceRegion
			}
			if !hasBinding {
				binding = state.ResourceBinding{
					CreatedAt: item.CreatedAt,
					UpdatedAt: item.CreatedAt,
				}
			}
			items = append(items, toSecurityGroupResourceFromBinding(binding, payload, tenant, workspace, http.MethodGet, "active"))
		}
		respondJSON(w, http.StatusOK, securityGroupIterator{
			Items:    items,
			Metadata: responseMetaObject{Provider: "seca.network/v1", Resource: "tenants/" + tenant + "/workspaces/" + workspace + "/security-groups", Verb: http.MethodGet},
		})
	}
}

func securityGroupCRUD(provider NetworkProvider, store *state.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			getSecurityGroup(provider, store)(w, r)
		case http.MethodPut:
			putSecurityGroup(provider, store)(w, r)
		case http.MethodDelete:
			deleteSecurityGroup(provider, store)(w, r)
		default:
			respondProblem(w, http.StatusMethodNotAllowed, "http://secapi.cloud/errors/invalid-request", "Method Not Allowed", "Only GET, PUT and DELETE are supported", r.URL.Path)
		}
	}
}

func getSecurityGroup(provider NetworkProvider, store *state.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		tenant, workspace, name, ok := scopedNameFromPath(w, r, "security group name is required")
		if !ok {
			return
		}
		workspaceRegion, ok := workspaceRegionOrDefault(r.Context(), store, tenant, workspace)
		if !ok {
			respondProblem(w, http.StatusInternalServerError, "http://secapi.cloud/errors/internal", "Internal Server Error", "failed to resolve workspace", r.URL.Path)
			return
		}
		ctx, ok := workspaceExecutionContext(w, r, store, tenant, workspace)
		if !ok {
			return
		}
		item, err := provider.GetSecurityGroup(ctx, name)
		if err != nil {
			respondFromError(w, err, r.URL.Path)
			return
		}
		if item == nil {
			respondProblem(w, http.StatusNotFound, "http://secapi.cloud/errors/resource-not-found", "Not Found", "security group not found", r.URL.Path)
			return
		}
		ref := securityGroupRef(tenant, workspace, name)
		binding, err := store.GetResourceBinding(r.Context(), ref)
		if err != nil {
			respondProblem(w, http.StatusInternalServerError, "http://secapi.cloud/errors/internal", "Internal Server Error", "failed to load security group", r.URL.Path)
			return
		}
		if binding == nil {
			bindings, listErr := store.ListResourceBindings(r.Context(), tenant, workspace, resourceBindingKindSecurityGroup)
			if listErr != nil {
				respondProblem(w, http.StatusInternalServerError, "http://secapi.cloud/errors/internal", "Internal Server Error", "failed to load security group", r.URL.Path)
				return
			}
			for i := range bindings {
				b := bindings[i]
				payload, parseErr := parseSecurityGroupBinding(b.ProviderRef)
				if parseErr == nil && strings.EqualFold(strings.TrimSpace(payload.Name), name) {
					binding = &b
					break
				}
			}
		}
		payload := securityGroupBindingPayload{
			Name:   item.Name,
			Region: workspaceRegion,
			Labels: item.Labels,
			Spec:   securityGroupSpec{Rules: toSecurityGroupRuleSpecs(item.Rules)},
		}
		if binding != nil {
			parsedPayload, parseErr := parseSecurityGroupBinding(binding.ProviderRef)
			if parseErr != nil {
				respondProblem(w, http.StatusInternalServerError, "http://secapi.cloud/errors/internal", "Internal Server Error", "invalid security group payload", r.URL.Path)
				return
			}
			payload = parsedPayload
			if len(payload.Labels) == 0 {
				payload.Labels = item.Labels
			}
			if len(payload.Spec.Rules) == 0 {
				payload.Spec.Rules = toSecurityGroupRuleSpecs(item.Rules)
			}
			if payload.Region == "" {
				payload.Region = workspaceRegion
			}
		}
		outBinding := state.ResourceBinding{CreatedAt: item.CreatedAt, UpdatedAt: item.CreatedAt}
		if binding != nil {
			outBinding = *binding
		}
		respondJSON(w, http.StatusOK, toSecurityGroupResourceFromBinding(outBinding, payload, tenant, workspace, http.MethodGet, "active"))
	}
}

func putSecurityGroup(provider NetworkProvider, store *state.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		tenant, workspace, name, ok := scopedNameFromPath(w, r, "security group name is required")
		if !ok {
			return
		}
		ctx, ok := workspaceExecutionContext(w, r, store, tenant, workspace)
		if !ok {
			return
		}
		var req securityGroupResource
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			respondProblem(w, http.StatusBadRequest, "http://secapi.cloud/errors/invalid-request", "Bad Request", "invalid json body", r.URL.Path)
			return
		}

		item, created, err := provider.CreateOrUpdateSecurityGroup(ctx, hetzner.SecurityGroupCreateRequest{
			Name:   name,
			Labels: req.Labels,
		})
		if err != nil {
			respondFromError(w, err, r.URL.Path)
			return
		}
		if item == nil {
			respondProblem(w, http.StatusInternalServerError, "http://secapi.cloud/errors/internal-server-error", "Internal Server Error", "provider returned empty security group", r.URL.Path)
			return
		}

		ref := securityGroupRef(tenant, workspace, name)
		existing, err := store.GetResourceBinding(r.Context(), ref)
		if err != nil {
			respondProblem(w, http.StatusInternalServerError, "http://secapi.cloud/errors/internal", "Internal Server Error", "failed to load security group", r.URL.Path)
			return
		}

		payload := securityGroupBindingPayload{
			Name:   name,
			Region: runtimeRegionOrDefault(req.Metadata.Region),
			Labels: req.Labels,
			Spec:   req.Spec,
		}
		if payload.Region == "" {
			payload.Region = "global"
		}
		raw, err := json.Marshal(payload)
		if err != nil {
			respondProblem(w, http.StatusInternalServerError, "http://secapi.cloud/errors/internal", "Internal Server Error", "failed to encode security group", r.URL.Path)
			return
		}
		if err := store.UpsertResourceBinding(r.Context(), state.ResourceBinding{
			Tenant:      tenant,
			Workspace:   workspace,
			Kind:        resourceBindingKindSecurityGroup,
			SecaRef:     ref,
			ProviderRef: string(raw),
			Status:      "active",
		}); err != nil {
			respondProblem(w, http.StatusInternalServerError, "http://secapi.cloud/errors/internal", "Internal Server Error", "failed to save security group", r.URL.Path)
			return
		}
		binding, err := store.GetResourceBinding(r.Context(), ref)
		if err != nil || binding == nil {
			respondProblem(w, http.StatusInternalServerError, "http://secapi.cloud/errors/internal", "Internal Server Error", "failed to load security group", r.URL.Path)
			return
		}
		stateValue, code := upsertStateAndCode(created)
		if existing != nil && created {
			stateValue, code = "updating", http.StatusOK
		}
		respondJSON(w, code, toSecurityGroupResourceFromBinding(*binding, payload, tenant, workspace, http.MethodPut, stateValue))
	}
}

func deleteSecurityGroup(provider NetworkProvider, store *state.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		tenant, workspace, name, ok := scopedNameFromPath(w, r, "security group name is required")
		if !ok {
			return
		}
		ctx, ok := workspaceExecutionContext(w, r, store, tenant, workspace)
		if !ok {
			return
		}
		deleted, err := provider.DeleteSecurityGroup(ctx, name)
		if err != nil {
			respondFromError(w, err, r.URL.Path)
			return
		}
		if !deleted {
			respondProblem(w, http.StatusNotFound, "http://secapi.cloud/errors/resource-not-found", "Not Found", "security group not found", r.URL.Path)
			return
		}
		ref := securityGroupRef(tenant, workspace, name)
		if err := store.DeleteResourceBinding(r.Context(), ref); err != nil {
			respondProblem(w, http.StatusInternalServerError, "http://secapi.cloud/errors/internal", "Internal Server Error", "failed to delete security group", r.URL.Path)
			return
		}
		respondJSON(w, http.StatusAccepted, map[string]string{"status": "accepted"})
	}
}

func securityGroupRef(tenant, workspace, name string) string {
	return "seca.network/v1/tenants/" + strings.ToLower(strings.TrimSpace(tenant)) +
		"/workspaces/" + strings.ToLower(strings.TrimSpace(workspace)) +
		"/security-groups/" + strings.ToLower(strings.TrimSpace(name))
}

func parseSecurityGroupBinding(raw string) (securityGroupBindingPayload, error) {
	var payload securityGroupBindingPayload
	err := json.Unmarshal([]byte(raw), &payload)
	return payload, err
}

func toSecurityGroupResourceFromBinding(
	binding state.ResourceBinding,
	payload securityGroupBindingPayload,
	tenant,
	workspace,
	verb,
	stateValue string,
) securityGroupResource {
	createdAt := time.Now().UTC().Format(time.RFC3339)
	updatedAt := createdAt
	if !binding.CreatedAt.IsZero() {
		createdAt = binding.CreatedAt.UTC().Format(time.RFC3339)
	}
	if !binding.UpdatedAt.IsZero() {
		updatedAt = binding.UpdatedAt.UTC().Format(time.RFC3339)
	}
	return securityGroupResource{
		Metadata: resourceMetadata{
			Name:            payload.Name,
			Provider:        "seca.network/v1",
			Resource:        "tenants/" + tenant + "/workspaces/" + workspace + "/security-groups/" + payload.Name,
			Verb:            verb,
			CreatedAt:       createdAt,
			LastModifiedAt:  updatedAt,
			ResourceVersion: 1,
			APIVersion:      "v1",
			Kind:            "security-group",
			Ref:             "seca.network/v1/tenants/" + tenant + "/workspaces/" + workspace + "/security-groups/" + payload.Name,
			Tenant:          tenant,
			Workspace:       workspace,
			Region:          defaultRegion(strings.ToLower(strings.TrimSpace(payload.Region))),
		},
		Labels: payload.Labels,
		Spec:   payload.Spec,
		Status: securityGroupStatusObj{State: stateValue},
	}
}

func toSecurityGroupRuleSpecs(rules []hetzner.SecurityGroupRule) []securityGroupRuleSpec {
	if len(rules) == 0 {
		return []securityGroupRuleSpec{}
	}
	out := make([]securityGroupRuleSpec, 0, len(rules))
	for _, rule := range rules {
		direction := strings.TrimSpace(rule.Direction)
		if direction == "" {
			continue
		}
		out = append(out, securityGroupRuleSpec{Direction: strings.ToLower(direction)})
	}
	return out
}
