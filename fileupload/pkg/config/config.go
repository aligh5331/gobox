// Package config provides configuration for the FileUpload service.
package config

import (
	"fmt"
	"os"
	"strconv"
)

// Config holds all configuration values for the FileUpload service.
type Config struct {
	// DatabaseURL is the Postgres connection string.
	DatabaseURL string

	// GRPCAddr is the address the gRPC server listens on.
	GRPCAddr string

	// S3Endpoint is the S3/MinIO endpoint URL.
	S3Endpoint string

	// S3Bucket is the S3 bucket name.
	S3Bucket string

	// S3AccessKey is the S3/MinIO access key.
	S3AccessKey string

	// S3SecretKey is the S3/MinIO secret key.
	S3SecretKey string

	// S3PublicEndpoint overrides the endpoint used in presigned URLs.
	// If empty, S3Endpoint is used.
	S3PublicEndpoint string

	// PresignUploadTTLMinutes is the TTL for presigned upload URLs (default 15).
	PresignUploadTTLMinutes int

	// PresignDownloadTTLMinutes is the TTL for presigned download URLs (default 60).
	PresignDownloadTTLMinutes int

	// MaxFileSize is the maximum allowed file size in bytes (default 100 MiB).
	MaxFileSize int64

	// LogLevel is the zerolog log level (debug, info, warn, error).
	// Defaults to "info".
	LogLevel string
}

// Load reads configuration from environment variables with defaults.
func Load() (*Config, error) {
	cfg := &Config{
		DatabaseURL:              os.Getenv("DATABASE_URL"),
		GRPCAddr:                 getEnv("GRPC_PORT", "9090"),
		S3Endpoint:               os.Getenv("S3_ENDPOINT"),
		S3Bucket:                 os.Getenv("S3_BUCKET"),
		S3AccessKey:              os.Getenv("S3_ACCESS_KEY"),
		S3SecretKey:              os.Getenv("S3_SECRET_KEY"),
		S3PublicEndpoint:         os.Getenv("S3_PUBLIC_ENDPOINT"),
		PresignUploadTTLMinutes:  getEnvInt("PRESIGN_UPLOAD_TTL_MINUTES", 15),
		PresignDownloadTTLMinutes: getEnvInt("PRESIGN_DOWNLOAD_TTL_MINUTES", 60),
		MaxFileSize:              int64(getEnvInt("MAX_FILE_SIZE", 100*1024*1024)),
		LogLevel:                 getEnv("LOG_LEVEL", "info"),
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
	if c.S3Endpoint == "" {
		return fmt.Errorf("S3_ENDPOINT is required")
	}
	if c.S3Bucket == "" {
		return fmt.Errorf("S3_BUCKET is required")
	}
	if c.S3AccessKey == "" {
		return fmt.Errorf("S3_ACCESS_KEY is required")
	}
	if c.S3SecretKey == "" {
		return fmt.Errorf("S3_SECRET_KEY is required")
	}

	if c.PresignUploadTTLMinutes < 1 {
		return fmt.Errorf("PRESIGN_UPLOAD_TTL_MINUTES must be >= 1, got %d", c.PresignUploadTTLMinutes)
	}
	if c.PresignDownloadTTLMinutes < 1 {
		return fmt.Errorf("PRESIGN_DOWNLOAD_TTL_MINUTES must be >= 1, got %d", c.PresignDownloadTTLMinutes)
	}

	if c.PresignUploadTTLMinutes > 1440 {
		fmt.Fprintf(os.Stderr, "WARN: presign upload TTL exceeds 24 hours (1440 minutes): ttl_minutes=%d\n", c.PresignUploadTTLMinutes)
	}
	if c.PresignDownloadTTLMinutes > 1440 {
		fmt.Fprintf(os.Stderr, "WARN: presign download TTL exceeds 24 hours (1440 minutes): ttl_minutes=%d\n", c.PresignDownloadTTLMinutes)
	}

	// Prepend ":" if GRPCAddr is just a port number.
	if len(c.GRPCAddr) > 0 && c.GRPCAddr[0] != ':' {
		c.GRPCAddr = ":" + c.GRPCAddr
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
