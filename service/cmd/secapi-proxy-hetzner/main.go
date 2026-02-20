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
	servers := httpserver.New(cfg, store, regionService, regionService, regionService, regionService)
	log.Printf("runtime mode: conformance=%t (SECA_CONFORMANCE_MODE)", cfg.ConformanceMode)
	log.Printf("runtime mode: internet_gateway_nat_vm=%t (SECA_INTERNET_GATEWAY_NAT_VM)", cfg.InternetGatewayNATVM)

	go func() {
		log.Printf("starting secapi-proxy-hetzner public api on %s", cfg.ListenAddr)
		if err := servers.Public.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("public http server failed: %v", err)
		}
	}()
	go func() {
		log.Printf("starting secapi-proxy-hetzner admin api on %s", cfg.AdminListenAddr)
		if err := servers.Admin.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("admin http server failed: %v", err)
		}
	}()

	<-ctx.Done()
	stop()

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := servers.Public.Shutdown(shutdownCtx); err != nil {
		log.Printf("public graceful shutdown failed: %v", err)
	}
	if err := servers.Admin.Shutdown(shutdownCtx); err != nil {
		log.Printf("admin graceful shutdown failed: %v", err)
	}
}
