package httpserver

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/eu-sovereign-cloud/secapi-proxy-hetzner/internal/provider/hetzner"
	"github.com/eu-sovereign-cloud/secapi-proxy-hetzner/internal/state"
)

func scopeFromPath(w http.ResponseWriter, r *http.Request) (string, string, bool) {
	tenant := r.PathValue("tenant")
	workspace := r.PathValue("workspace")
	if tenant == "" || workspace == "" {
		respondProblem(w, http.StatusBadRequest, "http://secapi.cloud/errors/invalid-request", "Bad Request", "tenant and workspace are required", r.URL.Path)
		return "", "", false
	}
	return tenant, workspace, true
}

func scopedNameFromPath(w http.ResponseWriter, r *http.Request, nameErr string) (string, string, string, bool) {
	tenant, workspace, ok := scopeFromPath(w, r)
	if !ok {
		return "", "", "", false
	}
	name := strings.ToLower(r.PathValue("name"))
	if name == "" {
		respondProblem(w, http.StatusBadRequest, "http://secapi.cloud/errors/invalid-request", "Bad Request", nameErr, r.URL.Path)
		return "", "", "", false
	}
	return tenant, workspace, name, true
}

func scopedNetworkNameFromPath(w http.ResponseWriter, r *http.Request, nameErr string) (string, string, string, string, bool) {
	tenant, workspace, ok := scopeFromPath(w, r)
	if !ok {
		return "", "", "", "", false
	}
	network := strings.ToLower(r.PathValue("network"))
	if network == "" {
		respondProblem(w, http.StatusBadRequest, "http://secapi.cloud/errors/invalid-request", "Bad Request", "network name is required", r.URL.Path)
		return "", "", "", "", false
	}
	name := strings.ToLower(r.PathValue("name"))
	if name == "" {
		respondProblem(w, http.StatusBadRequest, "http://secapi.cloud/errors/invalid-request", "Bad Request", nameErr, r.URL.Path)
		return "", "", "", "", false
	}
	return tenant, workspace, network, name, true
}

func workspaceExecutionContext(w http.ResponseWriter, r *http.Request, store *state.Store, tenant, workspace string) (context.Context, bool) {
	ws, err := store.GetWorkspace(r.Context(), tenant, workspace)
	if err != nil {
		respondProblem(w, http.StatusInternalServerError, "http://secapi.cloud/errors/internal", "Internal Server Error", "failed to resolve workspace", r.URL.Path)
		return nil, false
	}
	if ws == nil {
		respondProblem(w, http.StatusNotFound, "http://secapi.cloud/errors/resource-not-found", "Not Found", "workspace not found", r.URL.Path)
		return nil, false
	}
	ws, err = waitForActiveWorkspace(r.Context(), store, tenant, workspace, ws, 2*time.Second, 500*time.Millisecond)
	if err != nil {
		respondProblem(w, http.StatusInternalServerError, "http://secapi.cloud/errors/internal", "Internal Server Error", "failed to resolve workspace", r.URL.Path)
		return nil, false
	}
	if ws == nil {
		respondProblem(w, http.StatusConflict, "http://secapi.cloud/errors/resource-conflict", "Conflict", "workspace is not active", r.URL.Path)
		return nil, false
	}

	cred, err := store.GetWorkspaceProviderCredential(r.Context(), tenant, workspace, "hetzner")
	if err != nil {
		respondProblem(w, http.StatusInternalServerError, "http://secapi.cloud/errors/internal", "Internal Server Error", "failed to resolve workspace credentials", r.URL.Path)
		return nil, false
	}
	if cred == nil || strings.TrimSpace(cred.APIToken) == "" {
		respondProblem(w, http.StatusConflict, "http://secapi.cloud/errors/resource-conflict", "Conflict", "workspace has no hetzner credentials", r.URL.Path)
		return nil, false
	}
	ctx := hetzner.WithWorkspaceCredential(r.Context(), hetzner.WorkspaceCredential{
		Token:       cred.APIToken,
		CloudAPIURL: cred.APIEndpoint,
	})
	return ctx, true
}

func waitForActiveWorkspace(ctx context.Context, store *state.Store, tenant, workspace string, ws *state.WorkspaceResource, timeout, interval time.Duration) (*state.WorkspaceResource, error) {
	stateValue, _ := ws.Status["state"].(string)
	current := strings.ToLower(strings.TrimSpace(stateValue))
	if current == "active" {
		return ws, nil
	}
	if current != "creating" {
		return nil, nil
	}

	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
			return nil, nil
		case <-time.After(interval):
		}

		refreshed, err := store.GetWorkspace(ctx, tenant, workspace)
		if err != nil {
			return nil, err
		}
		if refreshed == nil {
			return nil, nil
		}

		stateValue, _ = refreshed.Status["state"].(string)
		current = strings.ToLower(strings.TrimSpace(stateValue))
		if current == "active" {
			return refreshed, nil
		}
		if current != "creating" {
			return nil, nil
		}
	}

	return nil, nil
}

func resourceNameFromRef(ref string) string {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return ""
	}
	parts := strings.Split(ref, "/")
	return strings.ToLower(parts[len(parts)-1])
}

func regionFromZone(zone string) string {
	if zone == "" {
		return ""
	}
	if idx := strings.Index(zone, "-dc"); idx > 0 {
		return strings.ToLower(zone[:idx])
	}
	return strings.ToLower(zone)
}

func defaultRegion(value string) string {
	if value == "" {
		return "global"
	}
	return strings.ToLower(value)
}

func normalizeProviderBlockStorageSizeGB(size int) int {
	// Hetzner volume limits are stricter than conformance generated values.
	// Keep API-facing spec as requested, but normalize provider call values.
	if size < 10 {
		return 10
	}
	if size > 100 {
		return 100
	}
	return size
}

func computeInstanceRef(tenant, workspace, name string) string {
	return "seca.compute/v1/tenants/" + tenant + "/workspaces/" + workspace + "/instances/" + name
}

func blockStorageRef(tenant, workspace, name string) string {
	return "seca.storage/v1/tenants/" + tenant + "/workspaces/" + workspace + "/block-storages/" + name
}

func operationID(prefix, name string) string {
	return fmt.Sprintf("%s-%s-%d", prefix, name, time.Now().UnixNano())
}
