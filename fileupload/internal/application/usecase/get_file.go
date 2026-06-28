package usecase

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"

	"github.com/aligh5331/gobox/fileupload/internal/domain/model"
	"github.com/aligh5331/gobox/fileupload/internal/domain/repository"
)

// GetFileUseCase handles retrieving a single file's metadata.
type GetFileUseCase struct {
	repo repository.FileRepository
}

// NewGetFileUseCase creates a new GetFileUseCase.
func NewGetFileUseCase(repo repository.FileRepository) *GetFileUseCase {
	return &GetFileUseCase{
		repo: repo,
	}
}

// Execute retrieves a file by ID, verifying the caller owns it.
func (uc *GetFileUseCase) Execute(ctx context.Context, fileID, userID uuid.UUID) (*model.File, error) {
	file, err := uc.repo.FindByID(ctx, fileID)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return nil, ErrFileNotFound
		}
		return nil, fmt.Errorf("get file: find file: %w", err)
	}
	if file == nil {
		return nil, ErrFileNotFound
	}

	if file.UserID != userID {
		return nil, ErrPermissionDenied
	}

	return file, nil
}
