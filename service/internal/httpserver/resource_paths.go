package httpserver

import (
	"fmt"
	"net/http"
	"strings"
	"time"
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
