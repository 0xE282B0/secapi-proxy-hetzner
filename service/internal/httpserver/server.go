package httpserver

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/eu-sovereign-cloud/secapi-proxy-hetzner/internal/config"
	"github.com/eu-sovereign-cloud/secapi-proxy-hetzner/internal/state"
)

type statusResponse struct {
	Status string `json:"status"`
}

func New(cfg config.Config, store *state.Store) *http.Server {
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", healthz)
	mux.HandleFunc("/readyz", readyz(store))

	return &http.Server{
		Addr:              cfg.ListenAddr,
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
	}
}

func healthz(w http.ResponseWriter, _ *http.Request) {
	respondJSON(w, http.StatusOK, statusResponse{Status: "ok"})
}

func readyz(store *state.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		if err := store.Ping(ctx); err != nil {
			respondJSON(w, http.StatusServiceUnavailable, statusResponse{Status: "db_unavailable"})
			return
		}
		respondJSON(w, http.StatusOK, statusResponse{Status: "ready"})
	}
}

func respondJSON(w http.ResponseWriter, code int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(payload)
}
