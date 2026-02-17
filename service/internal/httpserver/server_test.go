package httpserver

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/eu-sovereign-cloud/secapi-proxy-hetzner/internal/config"
	"github.com/eu-sovereign-cloud/secapi-proxy-hetzner/internal/provider/hetzner"
)

func TestHealthz(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	w := httptest.NewRecorder()

	healthz(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, w.Code)
	}
}

type fakeRegionProvider struct{}

func (f fakeRegionProvider) ListRegions(_ context.Context) ([]hetzner.Region, error) {
	return []hetzner.Region{
		{
			Name:    "fsn1",
			City:    "Falkenstein",
			Country: "DE",
			Zones:   []string{"fsn1-dc14"},
			Providers: []hetzner.Provider{
				{Name: "hetzner.cloud", Version: "v1", URL: "https://api.hetzner.cloud/v1"},
			},
		},
	}, nil
}

func (f fakeRegionProvider) GetRegion(_ context.Context, name string) (*hetzner.Region, error) {
	if name != "fsn1" {
		return nil, nil
	}
	region := hetzner.Region{
		Name:    "fsn1",
		City:    "Falkenstein",
		Country: "DE",
		Zones:   []string{"fsn1-dc14"},
		Providers: []hetzner.Provider{
			{Name: "hetzner.cloud", Version: "v1", URL: "https://api.hetzner.cloud/v1"},
		},
	}
	return &region, nil
}

func TestWellknown(t *testing.T) {
	cfg := config.Config{PublicBaseURL: "http://localhost:8080"}
	handler := wellknown(cfg)

	req := httptest.NewRequest(http.MethodGet, "/.wellknown/secapi", nil)
	w := httptest.NewRecorder()
	handler(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, w.Code)
	}

	var payload wellknownResponse
	if err := json.NewDecoder(w.Body).Decode(&payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if payload.Version != "v1" {
		t.Fatalf("unexpected version: %s", payload.Version)
	}
	if len(payload.Endpoints) == 0 {
		t.Fatal("expected at least one endpoint")
	}
}

func TestListRegions(t *testing.T) {
	handler := listRegions(fakeRegionProvider{})

	req := httptest.NewRequest(http.MethodGet, "/v1/regions", nil)
	w := httptest.NewRecorder()
	handler(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, w.Code)
	}

	var payload regionIterator
	if err := json.NewDecoder(w.Body).Decode(&payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(payload.Items) != 1 {
		t.Fatalf("expected 1 region, got %d", len(payload.Items))
	}
	if payload.Items[0].Metadata.Name != "fsn1" {
		t.Fatalf("unexpected region name: %s", payload.Items[0].Metadata.Name)
	}
}
