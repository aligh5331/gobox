// Command main is the entry point for the auth service.
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
	"github.com/rs/zerolog"
	"golang.org/x/sync/errgroup"
	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"

	pb "github.com/aligh5331/gobox-proto/gen/auth/v1"
	"github.com/aligh5331/gobox/auth/internal/application/usecase"
	authpostgres "github.com/aligh5331/gobox/auth/internal/infrastructure/postgres"
	grpcserver "github.com/aligh5331/gobox/auth/internal/interface/grpc"
	"github.com/aligh5331/gobox/auth/internal/interface/rest"
	"github.com/aligh5331/gobox/auth/pkg/config"
	"github.com/aligh5331/gobox/auth/pkg/jwtutil"
	"github.com/aligh5331/gobox/auth/pkg/logger"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "config: %v\n", err)
		os.Exit(1)
	}

	log := logger.New(cfg.LogLevel)
	log.Info().Msg("starting auth service")

	// -----------------------------------------------------------------------
	// Database
	// -----------------------------------------------------------------------
	db, err := gorm.Open(postgres.Open(cfg.DatabaseURL), &gorm.Config{
		Logger: newGormLogger(log),
	})
	if err != nil {
		log.Fatal().Err(err).Msg("failed to connect to database")
	}

	sqlDB, err := db.DB()
	if err != nil {
		log.Fatal().Err(err).Msg("failed to get underlying sql.DB")
	}
	sqlDB.SetMaxOpenConns(25)
	sqlDB.SetMaxIdleConns(5)
	sqlDB.SetConnMaxLifetime(5 * time.Minute)

	// -----------------------------------------------------------------------
	// Migrations
	// -----------------------------------------------------------------------
	if err := runMigrations(cfg.DatabaseURL, log); err != nil {
		log.Fatal().Err(err).Msg("failed to run migrations")
	}
	log.Info().Msg("database migrations completed")

	// -----------------------------------------------------------------------
	// Dependencies
	// -----------------------------------------------------------------------
	keyManager, err := jwtutil.NewKeyManager(
		cfg.JWTPrivateKeyPath,
		cfg.JWTPreviousPrivateKeyPath,
	)
	if err != nil {
		log.Fatal().Err(err).Msg("failed to initialize key manager")
	}

	userRepo := authpostgres.NewUserRepo(db)
	sessionRepo := authpostgres.NewSessionRepo(db)

	// Use cases
	registerUC := usecase.NewRegisterUseCase(userRepo, sessionRepo, keyManager, log)
	loginUC := usecase.NewLoginUseCase(userRepo, sessionRepo, keyManager, log)
	refreshTokenUC := usecase.NewRefreshTokenUseCase(userRepo, sessionRepo, keyManager, log)
	logoutUC := usecase.NewLogoutUseCase(sessionRepo, log)
	logoutAllUC := usecase.NewLogoutAllUseCase(sessionRepo, log)
	getUserUC := usecase.NewGetUserUseCase(userRepo, log)
	updateProfileUC := usecase.NewUpdateProfileUseCase(userRepo, log)
	changePasswordUC := usecase.NewChangePasswordUseCase(userRepo, log)
	validateSessionUC := usecase.NewValidateSessionUseCase(sessionRepo)

	// -----------------------------------------------------------------------
	// gRPC server
	// -----------------------------------------------------------------------
	grpcListener, err := net.Listen("tcp", ":"+cfg.GRPCPort)
	if err != nil {
		log.Fatal().Err(err).Str("port", cfg.GRPCPort).Msg("failed to listen on gRPC port")
	}

	grpcSrv := grpc.NewServer()
	authSrv := grpcserver.NewAuthServer(
		registerUC,
		loginUC,
		refreshTokenUC,
		logoutUC,
		logoutAllUC,
		getUserUC,
		updateProfileUC,
		changePasswordUC,
		validateSessionUC,
		keyManager,
	)
	pb.RegisterAuthServiceServer(grpcSrv, authSrv)
	reflection.Register(grpcSrv)

	// -----------------------------------------------------------------------
	// HTTP (REST) server
	// -----------------------------------------------------------------------
	restSrv := rest.NewServer(keyManager, log)

	// -----------------------------------------------------------------------
	// Graceful shutdown
	// -----------------------------------------------------------------------
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	g, gCtx := errgroup.WithContext(ctx)

	g.Go(func() error {
		log.Info().Str("port", cfg.GRPCPort).Msg("gRPC server starting")
		if err := grpcSrv.Serve(grpcListener); err != nil {
			return fmt.Errorf("gRPC server: %w", err)
		}
		return nil
	})

	g.Go(func() error {
		log.Info().Str("port", cfg.HTTPPort).Msg("HTTP server starting")
		addr := ":" + cfg.HTTPPort
		if err := restSrv.Start(addr); err != nil && !errors.Is(err, http.ErrServerClosed) {
			return fmt.Errorf("HTTP server: %w", err)
		}
		return nil
	})

	g.Go(func() error {
		<-gCtx.Done()
		log.Info().Msg("shutting down servers...")

		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		grpcSrv.GracefulStop()

		if err := restSrv.Shutdown(shutdownCtx); err != nil {
			log.Error().Err(err).Msg("HTTP server shutdown error")
		}

		if err := sqlDB.Close(); err != nil {
			log.Error().Err(err).Msg("database close error")
		}
		return nil
	})

	if err := g.Wait(); err != nil {
		log.Error().Err(err).Msg("service stopped with error")
		os.Exit(1)
	}

	log.Info().Msg("auth service stopped")
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

// newGormLogger wraps a zerolog logger for GORM.
func newGormLogger(log zerolog.Logger) gormlogger.Interface {
	return &gormLogAdapter{log: log}
}

type gormLogAdapter struct {
	log zerolog.Logger
}

func (l *gormLogAdapter) LogMode(level gormlogger.LogLevel) gormlogger.Interface {
	return l
}

func (l *gormLogAdapter) Info(ctx context.Context, msg string, args ...interface{}) {
	l.log.Info().Msgf(msg, args...)
}

func (l *gormLogAdapter) Warn(ctx context.Context, msg string, args ...interface{}) {
	l.log.Warn().Msgf(msg, args...)
}

func (l *gormLogAdapter) Error(ctx context.Context, msg string, args ...interface{}) {
	l.log.Error().Msgf(msg, args...)
}

func (l *gormLogAdapter) Trace(ctx context.Context, begin time.Time, fc func() (string, int64), err error) {
	elapsed := time.Since(begin)
	sql, rows := fc()
	l.log.Debug().
		Str("sql", sql).
		Int64("rows", rows).
		Dur("elapsed", elapsed).
		Msg("database query")
}
