package httpserver

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/eu-sovereign-cloud/secapi-proxy-hetzner/internal/provider/hetzner"
	"github.com/eu-sovereign-cloud/secapi-proxy-hetzner/internal/state"
)

func listNetworksProvider(provider NetworkProvider, store *state.Store) http.HandlerFunc {
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
		items, err := provider.ListNetworks(ctx)
		if err != nil {
			respondFromError(w, err, r.URL.Path)
			return
		}

		routeRefs, err := listNetworkRouteTableRefs(r.Context(), store, tenant, workspace)
		if err != nil {
			respondProblem(w, http.StatusInternalServerError, "http://secapi.cloud/errors/internal", "Internal Server Error", "failed to list network route table refs", r.URL.Path)
			return
		}
		now := time.Now().UTC().Format(time.RFC3339)
		out := make([]networkResource, 0, len(items))
		for _, item := range items {
			out = append(out, toProviderNetworkResource(item, tenant, workspace, workspaceRegion, routeRefs[item.Name], http.MethodGet, "active", now))
		}
		respondJSON(w, http.StatusOK, networkIterator{
			Items:    out,
			Metadata: responseMetaObject{Provider: "seca.network/v1", Resource: "tenants/" + tenant + "/workspaces/" + workspace + "/networks", Verb: http.MethodGet},
		})
	}
}

func networkCRUDProvider(provider NetworkProvider, store *state.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			getNetworkProvider(provider, store)(w, r)
		case http.MethodPut:
			putNetworkProvider(provider, store)(w, r)
		case http.MethodDelete:
			deleteNetworkProvider(provider, store)(w, r)
		default:
			respondProblem(w, http.StatusMethodNotAllowed, "http://secapi.cloud/errors/invalid-request", "Method Not Allowed", "Only GET, PUT and DELETE are supported", r.URL.Path)
		}
	}
}

func getNetworkProvider(provider NetworkProvider, store *state.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		tenant, workspace, name, ok := scopedNameFromPath(w, r, "network name is required")
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
		item, err := provider.GetNetwork(ctx, name)
		if err != nil {
			respondFromError(w, err, r.URL.Path)
			return
		}
		if item == nil {
			respondProblem(w, http.StatusNotFound, "http://secapi.cloud/errors/resource-not-found", "Not Found", "network not found", r.URL.Path)
			return
		}
		routeRef, err := getNetworkRouteTableRef(r.Context(), store, tenant, workspace, name)
		if err != nil {
			respondProblem(w, http.StatusInternalServerError, "http://secapi.cloud/errors/internal", "Internal Server Error", "failed to load network route table ref", r.URL.Path)
			return
		}
		now := time.Now().UTC().Format(time.RFC3339)
		respondJSON(w, http.StatusOK, toProviderNetworkResource(*item, tenant, workspace, workspaceRegion, routeRef, http.MethodGet, "active", now))
	}
}

func putNetworkProvider(provider NetworkProvider, store *state.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		tenant, workspace, name, ok := scopedNameFromPath(w, r, "network name is required")
		if !ok {
			return
		}
		ctx, ok := workspaceExecutionContext(w, r, store, tenant, workspace)
		if !ok {
			return
		}

		var req networkResource
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			respondProblem(w, http.StatusBadRequest, "http://secapi.cloud/errors/invalid-request", "Bad Request", "invalid json body", r.URL.Path)
			return
		}
		if strings.TrimSpace(req.Spec.SkuRef.Resource) == "" {
			respondProblem(w, http.StatusBadRequest, "http://secapi.cloud/errors/invalid-request", "Bad Request", "spec.skuRef is required", r.URL.Path)
			return
		}
		if req.Spec.Cidr.IPv4 == nil || strings.TrimSpace(*req.Spec.Cidr.IPv4) == "" {
			respondProblem(w, http.StatusBadRequest, "http://secapi.cloud/errors/invalid-request", "Bad Request", "spec.cidr.ipv4 is required", r.URL.Path)
			return
		}
		workspaceRegion, ok := workspaceRegionOrDefault(r.Context(), store, tenant, workspace)
		if !ok {
			respondProblem(w, http.StatusInternalServerError, "http://secapi.cloud/errors/internal", "Internal Server Error", "failed to resolve workspace", r.URL.Path)
			return
		}
		responseRegion := workspaceRegion
		if reqRegion := strings.TrimSpace(req.Metadata.Region); reqRegion != "" {
			responseRegion = strings.ToLower(reqRegion)
		}

		item, created, err := provider.CreateOrUpdateNetwork(ctx, hetzner.NetworkCreateRequest{
			Name:   name,
			CIDR:   strings.TrimSpace(*req.Spec.Cidr.IPv4),
			Labels: req.Labels,
		})
		if err != nil {
			respondFromError(w, err, r.URL.Path)
			return
		}
		if item == nil {
			respondProblem(w, http.StatusInternalServerError, "http://secapi.cloud/errors/internal-server-error", "Internal Server Error", "provider returned empty network", r.URL.Path)
			return
		}
		routeRef := strings.TrimSpace(req.Spec.RouteTableRef.Resource)
		if routeRef != "" {
			if err := store.UpsertResourceBinding(r.Context(), state.ResourceBinding{
				Tenant:      tenant,
				Workspace:   workspace,
				Kind:        resourceBindingKindNetworkRouteTableRef,
				SecaRef:     networkRouteTableRefKey(tenant, workspace, name),
				ProviderRef: routeRef,
				Status:      "active",
			}); err != nil {
				respondProblem(w, http.StatusInternalServerError, "http://secapi.cloud/errors/internal", "Internal Server Error", "failed to save network route table ref", r.URL.Path)
				return
			}
		} else {
			_ = store.DeleteResourceBinding(r.Context(), networkRouteTableRefKey(tenant, workspace, name))
		}
		stateValue, code := upsertStateAndCode(created)
		now := time.Now().UTC().Format(time.RFC3339)
		respondJSON(w, code, toProviderNetworkResource(*item, tenant, workspace, responseRegion, routeRef, http.MethodPut, stateValue, now))
	}
}

func deleteNetworkProvider(provider NetworkProvider, store *state.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		tenant, workspace, name, ok := scopedNameFromPath(w, r, "network name is required")
		if !ok {
			return
		}
		ctx, ok := workspaceExecutionContext(w, r, store, tenant, workspace)
		if !ok {
			return
		}
		deleted, err := provider.DeleteNetwork(ctx, name)
		if err != nil {
			respondFromError(w, err, r.URL.Path)
			return
		}
		if !deleted {
			respondProblem(w, http.StatusNotFound, "http://secapi.cloud/errors/resource-not-found", "Not Found", "network not found", r.URL.Path)
			return
		}
		_ = store.DeleteResourceBinding(r.Context(), networkRouteTableRefKey(tenant, workspace, name))
		respondJSON(w, http.StatusAccepted, map[string]string{"status": "accepted"})
	}
}

func toProviderNetworkResource(item hetzner.Network, tenant, workspace, region, routeTableRef, verb, state, now string) networkResource {
	return networkResource{
		Metadata: resourceMetadata{
			Name:            item.Name,
			Provider:        "seca.network/v1",
			Resource:        "tenants/" + tenant + "/workspaces/" + workspace + "/networks/" + item.Name,
			Verb:            verb,
			CreatedAt:       now,
			LastModifiedAt:  now,
			ResourceVersion: 1,
			APIVersion:      "v1",
			Kind:            "network",
			Ref:             "seca.network/v1/tenants/" + tenant + "/workspaces/" + workspace + "/networks/" + item.Name,
			Tenant:          tenant,
			Workspace:       workspace,
			Region:          defaultRegion(strings.ToLower(strings.TrimSpace(region))),
		},
		Labels: item.Labels,
		Spec: networkSpec{
			Cidr: networkCIDR{
				IPv4: stringPtrOrNil(item.CIDR),
			},
			SkuRef:        refObject{Resource: "skus/hcloud-network"},
			RouteTableRef: refObject{Resource: strings.TrimSpace(routeTableRef)},
		},
		Status: networkStatusObject{
			State: state,
			Cidr: networkCIDR{
				IPv4: stringPtrOrNil(item.CIDR),
			},
		},
	}
}

func workspaceRegionOrDefault(ctx context.Context, store *state.Store, tenant, workspace string) (string, bool) {
	ws, err := store.GetWorkspace(ctx, tenant, workspace)
	if err != nil {
		return "", false
	}
	if ws == nil {
		return "global", true
	}
	return defaultRegion(strings.ToLower(strings.TrimSpace(ws.Region))), true
}

func getNetworkRouteTableRef(ctx context.Context, store *state.Store, tenant, workspace, network string) (string, error) {
	binding, err := store.GetResourceBinding(ctx, networkRouteTableRefKey(tenant, workspace, network))
	if err != nil || binding == nil {
		return "", err
	}
	return strings.TrimSpace(binding.ProviderRef), nil
}

func listNetworkRouteTableRefs(ctx context.Context, store *state.Store, tenant, workspace string) (map[string]string, error) {
	bindings, err := store.ListResourceBindings(ctx, tenant, workspace, resourceBindingKindNetworkRouteTableRef)
	if err != nil {
		return nil, err
	}
	out := make(map[string]string, len(bindings))
	for _, binding := range bindings {
		ref := strings.TrimSpace(binding.ProviderRef)
		if ref == "" {
			continue
		}
		key := strings.ToLower(strings.TrimSpace(networkNameFromRouteTableRefBinding(binding.SecaRef)))
		if key == "" {
			continue
		}
		out[key] = ref
	}
	return out, nil
}

func networkNameFromRouteTableRefBinding(secaRef string) string {
	parts := strings.Split(strings.TrimSpace(secaRef), "/")
	for i := 0; i < len(parts)-1; i++ {
		if parts[i] == "networks" {
			return strings.TrimSuffix(parts[i+1], "#routeTableRef")
		}
	}
	return ""
}

func stringPtrOrNil(s string) *string {
	trimmed := strings.TrimSpace(s)
	if trimmed == "" {
		return nil
	}
	v := trimmed
	return &v
}
