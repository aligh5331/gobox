// Command fileupload is the entry point for the FileUpload gRPC service.
package main

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"os/signal"
	"syscall"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	"github.com/rs/zerolog"
	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"

	pb "github.com/aligh5331/gobox-proto/gen/fileupload/v1"
	"github.com/aligh5331/gobox/fileupload/internal/application/usecase"
	"github.com/aligh5331/gobox/fileupload/internal/infrastructure/minio"
	pgRepo "github.com/aligh5331/gobox/fileupload/internal/infrastructure/postgres"
	grpcServer "github.com/aligh5331/gobox/fileupload/internal/interface/grpc"
	"github.com/aligh5331/gobox/fileupload/pkg/config"
	"github.com/aligh5331/gobox/fileupload/pkg/logger"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "config: %v\n", err)
		os.Exit(1)
	}

	log := logger.New(cfg.LogLevel)
	log.Info().Msg("starting fileupload service")

	// Connect to Postgres.
	db, err := gorm.Open(postgres.Open(cfg.DatabaseURL), &gorm.Config{
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

	// Initialize repository.
	fileRepo := pgRepo.NewFileRepository(db)

	// Initialize S3/MinIO client.
	s3Client, err := minio.NewClient(
		cfg.S3Endpoint,
		cfg.S3AccessKey,
		cfg.S3SecretKey,
		cfg.S3Bucket,
		false,
		cfg.S3PublicEndpoint,
	)
	if err != nil {
		log.Fatal().Err(err).Msg("failed to create S3 client")
	}

	// Initialize use cases.
	initiateUploadUC := usecase.NewInitiateUploadUseCase(fileRepo, s3Client)
	confirmUploadUC := usecase.NewConfirmUploadUseCase(fileRepo, s3Client)
	getFileUC := usecase.NewGetFileUseCase(fileRepo)
	listFilesUC := usecase.NewListFilesUseCase(fileRepo)
	deleteFileUC := usecase.NewDeleteFileUseCase(fileRepo, s3Client)
	getDownloadURLUC := usecase.NewGetDownloadURLUseCase(fileRepo, s3Client)

	// Create gRPC server.
	grpcSrv := grpcServer.NewServer(
		initiateUploadUC,
		confirmUploadUC,
		getFileUC,
		listFilesUC,
		deleteFileUC,
		getDownloadURLUC,
	)

	// Start gRPC listener.
	lis, err := net.Listen("tcp", cfg.GRPCAddr)
	if err != nil {
		log.Fatal().Err(err).Str("address", cfg.GRPCAddr).Msg("failed to listen")
	}

	s := grpc.NewServer()
	pb.RegisterFileUploadServiceServer(s, grpcSrv)
	reflection.Register(s)

	// Graceful shutdown.
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	go func() {
		log.Info().Str("address", cfg.GRPCAddr).Msg("gRPC server starting")
		if err := s.Serve(lis); err != nil {
			log.Fatal().Err(err).Msg("gRPC server error")
		}
	}()

	<-ctx.Done()
	log.Info().Msg("shutting down gRPC server...")
	s.GracefulStop()
	log.Info().Msg("server stopped")
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
