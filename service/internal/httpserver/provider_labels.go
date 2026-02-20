package httpserver

import (
	"crypto/sha1"
	"encoding/hex"
	"strings"
)

const (
	secaLabelManaged   = "seca.managed"
	secaLabelTenant    = "seca.tenant"
	secaLabelWorkspace = "seca.workspace"
	secaLabelKind      = "seca.kind"
	secaLabelName      = "seca.name"
	secaLabelRef       = "seca.ref"
)

func withSecaProviderLabels(
	user map[string]string,
	tenant, workspace, kind, name, secaRef string,
) map[string]string {
	out := make(map[string]string, len(user)+6)
	for k, v := range user {
		out[k] = v
	}
	// System labels are always enforced to simplify dangling-resource cleanup.
	out[secaLabelManaged] = "true"
	out[secaLabelTenant] = compactLabelValue(tenant)
	out[secaLabelWorkspace] = compactLabelValue(workspace)
	out[secaLabelKind] = compactLabelValue(kind)
	out[secaLabelName] = compactLabelValue(name)
	out[secaLabelRef] = compactLabelValue(secaRef)
	return out
}

func compactLabelValue(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	// Keep label values short and stable to avoid provider-specific limits.
	if len(value) <= 63 {
		return value
	}
	sum := sha1.Sum([]byte(value))
	return "sha1-" + hex.EncodeToString(sum[:8])
}
