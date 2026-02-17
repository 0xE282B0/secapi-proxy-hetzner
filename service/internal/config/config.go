package config

import "os"

type Config struct {
	ListenAddr  string
	LogLevel    string
	DatabaseURL string
}

func Load() Config {
	return Config{
		ListenAddr:  getenvDefault("SECA_LISTEN_ADDR", ":8080"),
		LogLevel:    getenvDefault("SECA_LOG_LEVEL", "info"),
		DatabaseURL: getenvDefault("SECA_DATABASE_URL", "postgres://postgres:postgres@localhost:5432/secapi_proxy?sslmode=disable"),
	}
}

func getenvDefault(key, fallback string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return fallback
}
