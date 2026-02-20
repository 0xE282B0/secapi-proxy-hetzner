package httpserver

import (
	"net/http"
	"strings"
)

func runtimeRegionOrDefault(region string) string {
	trimmed := strings.TrimSpace(region)
	if trimmed == "" {
		return "global"
	}
	return trimmed
}

func upsertStateAndCode(created bool) (string, int) {
	if created {
		return "creating", http.StatusCreated
	}
	return "updating", http.StatusOK
}

