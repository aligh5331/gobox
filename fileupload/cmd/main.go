// Command fileupload is the entry point for the FileUpload gRPC service.
package main

import (
	"context"
	"log/slog"
	"net"
	"os"
	"os/signal"
	"syscall"

	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	pb "github.com/aligh5331/gobox-proto/gen/fileupload/v1"
	"github.com/aligh5331/gobox/fileupload/internal/application/usecase"
	"github.com/aligh5331/gobox/fileupload/internal/infrastructure/minio"
	pgRepo "github.com/aligh5331/gobox/fileupload/internal/infrastructure/postgres"
	grpcServer "github.com/aligh5331/gobox/fileupload/internal/interface/grpc"
	"github.com/aligh5331/gobox/fileupload/pkg/config"
)

func main() {
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo})))

	cfg, err := config.Load()
	if err != nil {
		slog.Error("failed to load config", "error", err)
		os.Exit(1)
	}

	// Connect to Postgres.
	db, err := gorm.Open(postgres.Open(cfg.DatabaseURL), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Warn),
	})
	if err != nil {
		slog.Error("failed to connect to database", "error", err)
		os.Exit(1)
	}

	sqlDB, err := db.DB()
	if err != nil {
		slog.Error("failed to get sql.DB", "error", err)
		os.Exit(1)
	}
	defer sqlDB.Close()

	// Initialize repository.
	fileRepo := pgRepo.NewFileRepository(db)

	// Initialize S3/MinIO client.
	s3Client, err := minio.NewClient(
		cfg.S3Endpoint,
		cfg.S3AccessKey,
		cfg.S3SecretKey,
		cfg.S3Bucket,
		false,
	)
	if err != nil {
		slog.Error("failed to create S3 client", "error", err)
		os.Exit(1)
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
		slog.Error("failed to listen", "address", cfg.GRPCAddr, "error", err)
		os.Exit(1)
	}

	s := grpc.NewServer()
	pb.RegisterFileUploadServiceServer(s, grpcSrv)
	reflection.Register(s)

	// Graceful shutdown.
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	go func() {
		slog.Info("gRPC server starting", "address", cfg.GRPCAddr)
		if err := s.Serve(lis); err != nil {
			slog.Error("gRPC server error", "error", err)
			os.Exit(1)
		}
	}()

	<-ctx.Done()
	slog.Info("shutting down gRPC server...")
	s.GracefulStop()
	slog.Info("server stopped")
}
