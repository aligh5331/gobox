// Package config loads application configuration from environment variables.
package config

import (
	"fmt"
	"os"
)

// Config holds all configuration for the auth service.
// All fields are populated from environment variables at startup.
type Config struct {
	DatabaseURL            string
	GRPCPort               string
	HTTPPort               string
	JWTPrivateKeyPath      string
	JWTPreviousPrivateKeyPath string
	LogLevel               string
}

// Load reads configuration from environment variables.
// Returns an error if any required variable is missing.
func Load() (*Config, error) {
	cfg := &Config{
		GRPCPort: envOrDefault("GRPC_PORT", "8081"),
		HTTPPort: envOrDefault("HTTP_PORT", "8084"),
		LogLevel: envOrDefault("LOG_LEVEL", "info"),
	}

	cfg.DatabaseURL = os.Getenv("DATABASE_URL")
	if cfg.DatabaseURL == "" {
		return nil, fmt.Errorf("config: DATABASE_URL is required")
	}

	cfg.JWTPrivateKeyPath = os.Getenv("JWT_PRIVATE_KEY_PATH")
	if cfg.JWTPrivateKeyPath == "" {
		return nil, fmt.Errorf("config: JWT_PRIVATE_KEY_PATH is required")
	}

	cfg.JWTPreviousPrivateKeyPath = os.Getenv("JWT_PREVIOUS_PRIVATE_KEY_PATH")

	return cfg, nil
}

func envOrDefault(key, defaultVal string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return defaultVal
}
