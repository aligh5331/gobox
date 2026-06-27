package usecase

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	"github.com/aligh5331/gobox/fileupload/internal/domain/model"
	"github.com/aligh5331/gobox/fileupload/internal/domain/repository"
)

// ListFilesOutput contains a page of files and the next page token.
type ListFilesOutput struct {
	Files         []*model.File
	NextPageToken string
}

// ListFilesUseCase handles paginated file listing for a user.
type ListFilesUseCase struct {
	repo repository.FileRepository
}

// NewListFilesUseCase creates a new ListFilesUseCase.
func NewListFilesUseCase(repo repository.FileRepository) *ListFilesUseCase {
	return &ListFilesUseCase{
		repo: repo,
	}
}

// Execute returns a page of files for the given user.
func (uc *ListFilesUseCase) Execute(ctx context.Context, userID uuid.UUID, pageSize int, pageToken string) (*ListFilesOutput, error) {
	pageSize = clampPageSize(pageSize)

	// Decode cursor if provided.
	if pageToken != "" {
		if _, err := decodeCursor(pageToken); err != nil {
			return nil, ErrInvalidPageToken
		}
	}

	files, nextToken, err := uc.repo.FindByUserID(ctx, userID, pageToken, pageSize)
	if err != nil {
		return nil, fmt.Errorf("list files: %w", err)
	}

	out := &ListFilesOutput{
		Files:         files,
		NextPageToken: nextToken,
	}

	return out, nil
}
