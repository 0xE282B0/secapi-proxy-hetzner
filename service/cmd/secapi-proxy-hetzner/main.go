package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os/signal"
	"syscall"
	"time"

	"github.com/eu-sovereign-cloud/secapi-proxy-hetzner/internal/config"
	"github.com/eu-sovereign-cloud/secapi-proxy-hetzner/internal/httpserver"
	"github.com/eu-sovereign-cloud/secapi-proxy-hetzner/internal/provider/hetzner"
	"github.com/eu-sovereign-cloud/secapi-proxy-hetzner/internal/state"
)

func main() {
	cfg := config.Load()
	if cfg.AdminToken == "" {
		log.Fatal("SECA_ADMIN_TOKEN must be configured")
	}
	if cfg.CredentialsKey == "" {
		log.Fatal("SECA_CREDENTIALS_KEY must be configured")
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	store, err := state.New(ctx, cfg.DatabaseURL, cfg.CredentialsKey)
	if err != nil {
		log.Fatalf("db init failed: %v", err)
	}
	defer store.Close()

	regionService := hetzner.NewRegionService(cfg)
	srv := httpserver.New(cfg, store, regionService, regionService, regionService)

	go func() {
		log.Printf("starting secapi-proxy-hetzner on %s", cfg.ListenAddr)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("http server failed: %v", err)
		}
	}()

	<-ctx.Done()
	stop()

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Printf("graceful shutdown failed: %v", err)
	}
}
