package usecase

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"

	"github.com/aligh5331/gobox/fileupload/internal/domain/repository"
)

// DeleteFileUseCase handles soft-deleting a file and scheduling async S3 cleanup.
type DeleteFileUseCase struct {
	repo        repository.FileRepository
	minioClient MinioClient
}

// NewDeleteFileUseCase creates a new DeleteFileUseCase.
func NewDeleteFileUseCase(repo repository.FileRepository, minioClient MinioClient) *DeleteFileUseCase {
	return &DeleteFileUseCase{
		repo:        repo,
		minioClient: minioClient,
	}
}

// Execute soft-deletes a file and spawns an async goroutine to remove the
// S3 object.
func (uc *DeleteFileUseCase) Execute(ctx context.Context, fileID, userID uuid.UUID) error {
	file, err := uc.repo.FindByID(ctx, fileID)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return ErrFileNotFound
		}
		return fmt.Errorf("delete file: find file: %w", err)
	}

	if file.UserID != userID {
		return ErrPermissionDenied
	}

	if err := uc.repo.SoftDelete(ctx, file.ID); err != nil {
		return fmt.Errorf("delete file: soft delete: %w", err)
	}

	// Async S3 cleanup — use context.WithoutCancel so the operation outlives
	// the request if the caller disconnects.
	go func(key string) {
		//nolint:errcheck // best-effort async cleanup
		uc.minioClient.RemoveObject(context.WithoutCancel(ctx), key)
	}(file.StorageKey)

	return nil
}
