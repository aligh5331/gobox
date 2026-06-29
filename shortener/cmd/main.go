// Command shortener is the entry point for the Shortener service.
package main

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	"github.com/labstack/echo/v4"
	"github.com/rs/zerolog"
	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"
	gormpg "gorm.io/driver/postgres"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"

	pb "github.com/aligh5331/gobox-proto/gen/shortener/v1"
	"github.com/aligh5331/gobox/shortener/internal/application/usecase"
	pgRepo "github.com/aligh5331/gobox/shortener/internal/infrastructure/postgres"
	"github.com/aligh5331/gobox/shortener/internal/infrastructure/redis"
	grpcServer "github.com/aligh5331/gobox/shortener/internal/interface/grpc"
	"github.com/aligh5331/gobox/shortener/internal/interface/rest/handler"
	"github.com/aligh5331/gobox/shortener/pkg/config"
	"github.com/aligh5331/gobox/shortener/pkg/logger"
	"github.com/aligh5331/gobox/shortener/pkg/slug"
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
		Str("grpc_port", cfg.GRPCPort).
		Str("http_port", cfg.HTTPPort).
		Str("fileupload_grpc", cfg.FileUploadGRPCAddr).
		Msg("starting shortener service")

	// Connect to Postgres.
	db, err := gorm.Open(gormpg.Open(cfg.DatabaseURL), &gorm.Config{
		Logger: gormlogger.Default.LogMode(gormlogger.Warn),
	})
	if err != nil {
		log.Fatal().Err(err).Msg("failed to connect to database")
	}
	sqlDB, err := db.DB()
	if err != nil {
		log.Fatal().Err(err).Msg("failed to get sql.DB")
	}
	defer sqlDB.Close()

	// Run database migrations.
	if err := runMigrations(cfg.DatabaseURL, log); err != nil {
		log.Fatal().Err(err).Msg("failed to run migrations")
	}
	log.Info().Msg("database migrations completed")

	// Connect to Redis.
	redisCache, err := redis.NewCache(cfg.RedisURL)
	if err != nil {
		log.Fatal().Err(err).Msg("failed to connect to redis")
	}
	defer redisCache.Close()

	// Initialize slug generator.
	slugGen := slug.NewGenerator()

	// Initialize repositories.
	shortLinkRepo := pgRepo.NewShortLinkRepository(db)

	// Initialize use cases.
	createLinkUC := usecase.NewCreateLinkUseCase(shortLinkRepo, slugGen, cfg.BaseURL)
	getLinkUC := usecase.NewGetLinkUseCase(shortLinkRepo)
	deleteLinkUC := usecase.NewDeleteLinkUseCase(shortLinkRepo)
	listLinksUC := usecase.NewListLinksUseCase(shortLinkRepo)
	incrHitCountUC := usecase.NewIncrementHitCountUseCase(shortLinkRepo)

	// Create gRPC server.
	grpcSrv := grpcServer.NewServer(
		createLinkUC,
		getLinkUC,
		deleteLinkUC,
		listLinksUC,
	)

	// Start gRPC server.
	grpcLis, err := net.Listen("tcp", cfg.GRPCPort)
	if err != nil {
		log.Fatal().Err(err).Str("port", cfg.GRPCPort).Msg("failed to listen grpc")
	}

	gRPCServer := grpc.NewServer()
	pb.RegisterShortenerServiceServer(gRPCServer, grpcSrv)
	reflection.Register(gRPCServer)

	go func() {
		log.Info().Str("addr", cfg.GRPCPort).Msg("gRPC server listening")
		if err := gRPCServer.Serve(grpcLis); err != nil {
			log.Fatal().Err(err).Msg("gRPC server error")
		}
	}()

	// Create redirect handler with FileUpload gRPC client.
	redirectHandler, err := handler.NewRedirectHandler(
		getLinkUC,
		incrHitCountUC,
		redisCache,
		cfg.FileUploadGRPCAddr,
		log,
	)
	if err != nil {
		log.Fatal().Err(err).Msg("failed to create redirect handler")
	}
	defer redirectHandler.Close()

	// Start HTTP server (public redirects).
	e := echo.New()
	e.HideBanner = true
	e.HidePort = true
	e.GET("/s/:slug", redirectHandler.Redirect)
	e.GET("/health", healthCheck)

	go func() {
		log.Info().Str("addr", cfg.HTTPPort).Msg("HTTP server listening")
		if err := e.Start(cfg.HTTPPort); err != nil && err != http.ErrServerClosed {
			log.Fatal().Err(err).Msg("HTTP server error")
		}
	}()

	// Graceful shutdown.
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	log.Info().Msg("shutting down...")

	gRPCServer.GracefulStop()

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()
	if err := e.Shutdown(shutdownCtx); err != nil {
		log.Error().Err(err).Msg("HTTP server shutdown error")
	}

	log.Info().Msg("shutdown complete")
}

func runMigrations(dbURL string, log zerolog.Logger) error {
	m, err := migrate.New("file://migrations", dbURL)
	if err != nil {
		return fmt.Errorf("migrate init: %w", err)
	}
	defer m.Close()

	if err := m.Up(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		return fmt.Errorf("migrate up: %w", err)
	}
	return nil
}

func healthCheck(c echo.Context) error {
	return c.JSON(http.StatusOK, map[string]string{"status": "ok"})
}
