package usecase

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"time"

	"github.com/aligh5331/gobox/fileupload/internal/domain/model"
	"github.com/aligh5331/gobox/fileupload/internal/domain/repository"
)

// ConfirmUploadUseCase handles marking a pending file as ready after
// the client has uploaded the bytes to S3.
type ConfirmUploadUseCase struct {
	repo        repository.FileRepository
	minioClient MinioClient
}

// NewConfirmUploadUseCase creates a new ConfirmUploadUseCase.
func NewConfirmUploadUseCase(repo repository.FileRepository, minioClient MinioClient) *ConfirmUploadUseCase {
	return &ConfirmUploadUseCase{
		repo:        repo,
		minioClient: minioClient,
	}
}

// Execute confirms that the file has been uploaded to S3 and updates its
// status to ready. Returns the updated File.
func (uc *ConfirmUploadUseCase) Execute(ctx context.Context, fileID, userID uuid.UUID) (*model.File, error) {
	file, err := uc.repo.FindByID(ctx, fileID)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return nil, ErrFileNotFound
		}
		return nil, fmt.Errorf("confirm upload: find file: %w", err)
	}
	if file == nil {
		return nil, ErrFileNotFound
	}

	if file.UserID != userID {
		return nil, ErrPermissionDenied
	}

	if file.Status != model.FileStatusPending {
		return nil, ErrFileNotPending
	}

	// Verify the object exists in MinIO.
	exists, err := uc.minioClient.ObjectExists(ctx, file.StorageKey)
	if err != nil {
		return nil, fmt.Errorf("confirm upload: check object exists: %w", err)
	}
	if !exists {
		return nil, ErrObjectNotExists
	}

	file.Status = model.FileStatusReady
	file.UpdatedAt = time.Now()

	if err := uc.repo.UpdateStatus(ctx, file.ID, model.FileStatusReady); err != nil {
		return nil, fmt.Errorf("confirm upload: update status: %w", err)
	}

	return file, nil
}
