// Package config reads environment variables into a typed Config struct.
package config

import (
	"fmt"
	"os"
	"strconv"
	"time"
)

// Config holds all environment-driven configuration for the Core API.
type Config struct {
	HTTPPort     int
	AuthGRPCAddr string
	AuthHTTPAddr string
	LogLevel     string
}

// Load reads configuration from environment variables.
func Load() (*Config, error) {
	httpPort, err := getEnvInt("HTTP_PORT", 8080)
	if err != nil {
		return nil, fmt.Errorf("config: %w", err)
	}

	cfg := &Config{
		HTTPPort:     httpPort,
		AuthGRPCAddr: getEnv("AUTH_GRPC_ADDR", "localhost:8081"),
		AuthHTTPAddr: getEnv("AUTH_HTTP_ADDR", "http://localhost:8080"),
		LogLevel:     getEnv("LOG_LEVEL", "info"),
	}
	return cfg, nil
}

// JWKSRefreshInterval returns the JWKS cache refresh interval.
// Defaults to 5 minutes; overridable via JWKS_REFRESH_INTERVAL env var.
func JWKSRefreshInterval() time.Duration {
	v := os.Getenv("JWKS_REFRESH_INTERVAL")
	if v == "" {
		return 5 * time.Minute
	}
	d, err := time.ParseDuration(v)
	if err != nil {
		return 5 * time.Minute
	}
	return d
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func getEnvInt(key string, fallback int) (int, error) {
	v := os.Getenv(key)
	if v == "" {
		return fallback, nil
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return 0, fmt.Errorf("invalid %q: %w", key, err)
	}
	return n, nil
}
