package httpserver

import (
	"crypto/subtle"
	"net/http"
	"strings"
)

func requireAdminAuth(expectedToken string, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if strings.TrimSpace(expectedToken) == "" {
			respondProblem(w, http.StatusServiceUnavailable, "http://secapi.cloud/errors/provider-unavailable", "Service Unavailable", "admin auth is not configured", r.URL.Path)
			return
		}
		if !constantTimeBearerMatch(expectedToken, r.Header.Get("Authorization")) {
			respondProblem(w, http.StatusUnauthorized, "http://secapi.cloud/errors/unauthorized", "Unauthorized", "missing or invalid admin token", r.URL.Path)
			return
		}
		next(w, r)
	}
}

func constantTimeBearerMatch(expectedToken, authHeader string) bool {
	const prefix = "Bearer "
	if !strings.HasPrefix(authHeader, prefix) {
		return false
	}
	presented := strings.TrimPrefix(authHeader, prefix)
	if len(presented) != len(expectedToken) {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(presented), []byte(expectedToken)) == 1
}
