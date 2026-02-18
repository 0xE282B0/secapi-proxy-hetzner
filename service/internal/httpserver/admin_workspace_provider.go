package httpserver

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/eu-sovereign-cloud/secapi-proxy-hetzner/internal/provider/hetzner"
	"github.com/eu-sovereign-cloud/secapi-proxy-hetzner/internal/state"
)

type workspaceProviderBindRequest struct {
	APIToken    string `json:"apiToken"`
	APIEndpoint string `json:"apiEndpoint,omitempty"`
	ProjectRef  string `json:"projectRef,omitempty"`
}

type workspaceProviderBindResponse struct {
	Status   string `json:"status"`
	Provider string `json:"provider"`
}

func adminWorkspaceHetznerBinding(store *state.Store, regionProvider RegionProvider) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPut:
			adminPutWorkspaceHetznerBinding(store, regionProvider)(w, r)
		case http.MethodGet:
			adminGetWorkspaceHetznerBinding(store)(w, r)
		case http.MethodDelete:
			adminDeleteWorkspaceHetznerBinding(store)(w, r)
		default:
			respondProblem(w, http.StatusMethodNotAllowed, "http://secapi.cloud/errors/invalid-request", "Method Not Allowed", "Only GET, PUT and DELETE are supported", r.URL.Path)
		}
	}
}

func adminPutWorkspaceHetznerBinding(store *state.Store, regionProvider RegionProvider) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		tenant, workspace, ok := scopeFromPath(w, r)
		if !ok {
			return
		}
		ws, err := store.GetWorkspace(r.Context(), tenant, workspace)
		if err != nil {
			respondProblem(w, http.StatusInternalServerError, "http://secapi.cloud/errors/internal", "Internal Server Error", "failed to resolve workspace", r.URL.Path)
			return
		}
		if ws == nil {
			respondProblem(w, http.StatusNotFound, "http://secapi.cloud/errors/resource-not-found", "Not Found", "workspace not found", r.URL.Path)
			return
		}

		var req workspaceProviderBindRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			respondProblem(w, http.StatusBadRequest, "http://secapi.cloud/errors/invalid-request", "Bad Request", "invalid json body", r.URL.Path)
			return
		}
		req.APIToken = strings.TrimSpace(req.APIToken)
		if req.APIToken == "" {
			respondProblem(w, http.StatusBadRequest, "http://secapi.cloud/errors/invalid-request", "Bad Request", "apiToken is required", r.URL.Path)
			return
		}

		validateCtx := hetzner.WithWorkspaceCredential(r.Context(), hetzner.WorkspaceCredential{
			Token:       req.APIToken,
			CloudAPIURL: strings.TrimSpace(req.APIEndpoint),
		})
		if _, err := regionProvider.ListRegions(validateCtx); err != nil {
			respondProblem(w, http.StatusBadRequest, "http://secapi.cloud/errors/invalid-request", "Bad Request", "hetzner credential validation failed", r.URL.Path)
			return
		}

		_, err = store.UpsertWorkspaceProviderCredential(r.Context(), state.WorkspaceProviderCredential{
			Tenant:      tenant,
			Workspace:   workspace,
			Provider:    "hetzner",
			ProjectRef:  strings.TrimSpace(req.ProjectRef),
			APIEndpoint: strings.TrimSpace(req.APIEndpoint),
			APIToken:    req.APIToken,
		})
		if err != nil {
			respondProblem(w, http.StatusInternalServerError, "http://secapi.cloud/errors/internal", "Internal Server Error", "failed to persist workspace provider credential", r.URL.Path)
			return
		}

		ws.Status = map[string]any{"state": "active"}
		if _, err := store.UpsertWorkspace(r.Context(), *ws); err != nil {
			respondProblem(w, http.StatusInternalServerError, "http://secapi.cloud/errors/internal", "Internal Server Error", "failed to activate workspace", r.URL.Path)
			return
		}

		respondJSON(w, http.StatusOK, workspaceProviderBindResponse{
			Status:   "active",
			Provider: "hetzner",
		})
	}
}

func adminGetWorkspaceHetznerBinding(store *state.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		tenant, workspace, ok := scopeFromPath(w, r)
		if !ok {
			return
		}
		cred, err := store.GetWorkspaceProviderCredential(r.Context(), tenant, workspace, "hetzner")
		if err != nil {
			respondProblem(w, http.StatusInternalServerError, "http://secapi.cloud/errors/internal", "Internal Server Error", "failed to load workspace provider credential", r.URL.Path)
			return
		}
		if cred == nil {
			respondProblem(w, http.StatusNotFound, "http://secapi.cloud/errors/resource-not-found", "Not Found", "workspace provider credential not found", r.URL.Path)
			return
		}
		respondJSON(w, http.StatusOK, map[string]any{
			"provider":    cred.Provider,
			"projectRef":  cred.ProjectRef,
			"apiEndpoint": cred.APIEndpoint,
			"hasToken":    strings.TrimSpace(cred.APIToken) != "",
		})
	}
}

func adminDeleteWorkspaceHetznerBinding(store *state.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		tenant, workspace, ok := scopeFromPath(w, r)
		if !ok {
			return
		}
		deleted, err := store.SoftDeleteWorkspaceProviderCredential(r.Context(), tenant, workspace, "hetzner")
		if err != nil {
			respondProblem(w, http.StatusInternalServerError, "http://secapi.cloud/errors/internal", "Internal Server Error", "failed to delete workspace provider credential", r.URL.Path)
			return
		}
		if !deleted {
			respondProblem(w, http.StatusNotFound, "http://secapi.cloud/errors/resource-not-found", "Not Found", "workspace provider credential not found", r.URL.Path)
			return
		}

		ws, getErr := store.GetWorkspace(r.Context(), tenant, workspace)
		if getErr == nil && ws != nil {
			ws.Status = map[string]any{"state": "creating"}
			_, _ = store.UpsertWorkspace(r.Context(), *ws)
		}
		respondJSON(w, http.StatusAccepted, map[string]string{"status": "accepted"})
	}
}
