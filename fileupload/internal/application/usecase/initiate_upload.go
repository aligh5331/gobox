package usecase

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/aligh5331/gobox/fileupload/internal/domain/model"
	"github.com/aligh5331/gobox/fileupload/internal/domain/repository"
)

const (
	// InitiateUploadTTL is the default TTL for presigned PUT URLs.
	InitiateUploadTTL = 15 * time.Minute
)

// InitiateUploadOutput contains the result of initiating an upload.
type InitiateUploadOutput struct {
	FileID        uuid.UUID
	UploadURL     string
	UploadHeaders map[string]string
}

// MinioClient defines the interface for MinIO operations used by use cases.
type MinioClient interface {
	PresignedPutURL(ctx context.Context, objectKey string, ttl time.Duration) (url string, headers map[string]string, err error)
	PresignedGetURL(ctx context.Context, objectKey string, ttl time.Duration) (url string, expiresAt time.Time, err error)
	ObjectExists(ctx context.Context, objectKey string) (bool, error)
	RemoveObject(ctx context.Context, objectKey string) error
}

// InitiateUploadUseCase handles creating a pending file record and generating
// a presigned PUT URL for direct client-to-S3 upload.
type InitiateUploadUseCase struct {
	repo        repository.FileRepository
	minioClient MinioClient
}

// NewInitiateUploadUseCase creates a new InitiateUploadUseCase.
func NewInitiateUploadUseCase(repo repository.FileRepository, minioClient MinioClient) *InitiateUploadUseCase {
	return &InitiateUploadUseCase{
		repo:        repo,
		minioClient: minioClient,
	}
}

// Execute creates a pending File record and returns a presigned PUT URL.
func (uc *InitiateUploadUseCase) Execute(ctx context.Context, userID uuid.UUID, name string, size int64, mimeType string) (*InitiateUploadOutput, error) {
	if name == "" {
		return nil, ErrInvalidName
	}
	if size <= 0 {
		return nil, ErrInvalidSize
	}
	if mimeType == "" {
		return nil, ErrInvalidMimeType
	}

	now := time.Now()
	fileID := uuid.New()
	storageKey := fmt.Sprintf("uploads/%s_%s", fileID.String(), name)

	file := &model.File{
		ID:         fileID,
		UserID:     userID,
		Name:       name,
		Size:       size,
		MimeType:   mimeType,
		StorageKey: storageKey,
		Status:     model.FileStatusPending,
		CreatedAt:  now,
		UpdatedAt:  now,
	}

	if err := uc.repo.Create(ctx, file); err != nil {
		return nil, fmt.Errorf("initiate upload: create file: %w", err)
	}

	uploadURL, uploadHeaders, err := uc.minioClient.PresignedPutURL(ctx, storageKey, InitiateUploadTTL)
	if err != nil {
		return nil, fmt.Errorf("initiate upload: presigned put url: %w", err)
	}

	return &InitiateUploadOutput{
		FileID:        file.ID,
		UploadURL:     uploadURL,
		UploadHeaders: uploadHeaders,
	}, nil
}
