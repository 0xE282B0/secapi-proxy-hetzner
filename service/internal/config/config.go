package config

import (
	"os"
	"strings"
)

type Config struct {
	ListenAddr           string
	LogLevel             string
	DatabaseURL          string
	PublicBaseURL        string
	HetznerToken         string
	HetznerCloudAPIURL   string
	HetznerPrimaryAPIURL string
	ConformanceMode      bool
}

func Load() Config {
	return Config{
		ListenAddr:           getenvDefault("SECA_LISTEN_ADDR", ":8080"),
		LogLevel:             getenvDefault("SECA_LOG_LEVEL", "info"),
		DatabaseURL:          getenvDefault("SECA_DATABASE_URL", "postgres://postgres:postgres@localhost:5432/secapi_proxy?sslmode=disable"),
		PublicBaseURL:        strings.TrimRight(getenvDefault("SECA_PUBLIC_BASE_URL", "http://localhost:8080"), "/"),
		HetznerToken:         getenvFirst("HCLOUD_TOKEN", "HETZNER_API_TOKEN"),
		HetznerCloudAPIURL:   strings.TrimRight(getenvFirstDefault("https://api.hetzner.cloud/v1", "HCLOUD_ENDPOINT", "HETZNER_CLOUD_API_URL"), "/"),
		HetznerPrimaryAPIURL: strings.TrimRight(getenvFirstDefault("https://api.hetzner.com/v1", "HCLOUD_HETZNER_ENDPOINT", "HETZNER_PRIMARY_API_URL"), "/"),
		ConformanceMode:      getenvBool("SECA_CONFORMANCE_MODE"),
	}
}

func getenvDefault(key, fallback string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return fallback
}

func getenvFirst(keys ...string) string {
	for _, key := range keys {
		if val := os.Getenv(key); val != "" {
			return val
		}
	}
	return ""
}

func getenvFirstDefault(fallback string, keys ...string) string {
	if val := getenvFirst(keys...); val != "" {
		return val
	}
	return fallback
}

func getenvBool(key string) bool {
	val := strings.ToLower(strings.TrimSpace(os.Getenv(key)))
	return val == "1" || val == "true" || val == "yes" || val == "on"
}
