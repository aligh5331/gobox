// Package config provides configuration for the Shortener service.
package config

import (
	"fmt"
	"os"
	"strconv"
)

// Config holds all configuration values for the Shortener service.
type Config struct {
	DatabaseURL        string
	RedisURL           string
	GRPCPort           string
	HTTPPort           string
	FileUploadGRPCAddr string
	BaseURL            string
}

// Load reads configuration from environment variables with defaults.
func Load() (*Config, error) {
	cfg := &Config{
		DatabaseURL:        os.Getenv("DATABASE_URL"),
		RedisURL:           os.Getenv("REDIS_URL"),
		GRPCPort:           getEnv("GRPC_PORT", "9091"),
		HTTPPort:           getEnv("HTTP_PORT", "8082"),
		FileUploadGRPCAddr: os.Getenv("FILEUPLOAD_GRPC_ADDR"),
		BaseURL:            getEnv("BASE_URL", "http://localhost:8082"),
	}

	if err := cfg.validate(); err != nil {
		return nil, fmt.Errorf("config: %w", err)
	}

	return cfg, nil
}

func (c *Config) validate() error {
	if c.DatabaseURL == "" {
		return fmt.Errorf("DATABASE_URL is required")
	}
	if c.RedisURL == "" {
		return fmt.Errorf("REDIS_URL is required")
	}
	if c.FileUploadGRPCAddr == "" {
		return fmt.Errorf("FILEUPLOAD_GRPC_ADDR is required")
	}

	if len(c.GRPCPort) > 0 && c.GRPCPort[0] != ':' {
		c.GRPCPort = ":" + c.GRPCPort
	}
	if len(c.HTTPPort) > 0 && c.HTTPPort[0] != ':' {
		c.HTTPPort = ":" + c.HTTPPort
	}
	if len(c.BaseURL) > 0 && c.BaseURL[len(c.BaseURL)-1] == '/' {
		c.BaseURL = c.BaseURL[:len(c.BaseURL)-1]
	}

	return nil
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func getEnvInt(key string, fallback int) int {
	if v := os.Getenv(key); v != "" {
		n, err := strconv.Atoi(v)
		if err == nil {
			return n
		}
	}
	return fallback
}
