package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/labstack/echo/v4"

	"github.com/aligh5331/gobox/core/internal/infrastructure/grpcclient"
	"github.com/aligh5331/gobox/core/internal/infrastructure/grpcclient/fileupload"
	"github.com/aligh5331/gobox/core/internal/infrastructure/grpcclient/thumbgen"
	"github.com/aligh5331/gobox/core/internal/interface/rest"
	"github.com/aligh5331/gobox/core/internal/interface/rest/handler"
	"github.com/aligh5331/gobox/core/internal/interface/rest/middleware"
	"github.com/aligh5331/gobox/core/pkg/config"
	"github.com/aligh5331/gobox/core/pkg/jwtutil"
	"github.com/aligh5331/gobox/core/pkg/logger"
)

func main() {
	// Load configuration.
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to load config: %v\n", err)
		os.Exit(1)
	}

	// Initialize logger.
	log := logger.New(cfg.LogLevel)
	log.Info().
		Int("port", cfg.HTTPPort).
		Str("auth_grpc", cfg.AuthGRPCAddr).
		Str("fileupload_grpc", cfg.FileUploadGRPCAddr).
		Msg("starting core api")

	// Create root context for lifecycle management.
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Initialize JWKS cache (fail-fast on first fetch).
	jwksCache, err := jwtutil.NewJWKSCache(
		ctx,
		cfg.AuthHTTPAddr,
		config.JWKSRefreshInterval(),
		log,
	)
	if err != nil {
		log.Fatal().Err(err).Msg("failed to initialize JWKS cache")
	}

	// Dial Auth gRPC.
	authClient, err := grpcclient.NewAuthClient(ctx, cfg.AuthGRPCAddr)
	if err != nil {
		log.Fatal().Err(err).Msg("failed to connect to auth gRPC")
	}
	defer authClient.Close()

	// Dial FileUpload gRPC.
	fileuploadClient, err := fileupload.NewClient(ctx, cfg.FileUploadGRPCAddr)
	if err != nil {
		log.Fatal().Err(err).Msg("failed to connect to fileupload gRPC")
	}
	defer fileuploadClient.Close()

	// Create ThumbGen stub (no-op until Phase 5).
	thumbgenClient, err := thumbgen.NewClient(ctx, cfg.ThumbGenGRPCAddr)
	if err != nil {
		log.Fatal().Err(err).Msg("failed to create thumbgen client")
	}
	defer thumbgenClient.Close()

	// Build Echo server.
	e := echo.New()
	e.HideBanner = true
	e.HidePort = true

	// Set global error handler.
	e.HTTPErrorHandler = middleware.HTTPErrorHandler

	// Register routes.
	authHandler := rest.NewAuthHandler(authClient)
	meHandler := rest.NewMeHandler(authClient)
	fileHandler := handler.NewFileHandler(fileuploadClient, thumbgenClient, log)
	jwtMW := middleware.JWTAuth(jwksCache, log)
	rest.RegisterRoutes(e, authHandler, meHandler, fileHandler, jwtMW)

	// Start server.
	go func() {
		addr := fmt.Sprintf(":%d", cfg.HTTPPort)
		log.Info().Str("addr", addr).Msg("http server listening")
		if err := e.Start(addr); err != nil && err != http.ErrServerClosed {
			log.Fatal().Err(err).Msg("http server error")
		}
	}()

	// Graceful shutdown.
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	log.Info().Msg("shutting down...")

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()
	if err := e.Shutdown(shutdownCtx); err != nil {
		log.Error().Err(err).Msg("http server shutdown error")
	}
	log.Info().Msg("shutdown complete")
}
