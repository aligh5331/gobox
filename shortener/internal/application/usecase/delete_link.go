package usecase

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"

	"github.com/aligh5331/gobox/shortener/internal/domain/repository"
)

// DeleteLinkUseCase handles deleting a short link.
type DeleteLinkUseCase struct {
	repo repository.ShortLinkRepository
}

// NewDeleteLinkUseCase creates a new DeleteLinkUseCase.
func NewDeleteLinkUseCase(repo repository.ShortLinkRepository) *DeleteLinkUseCase {
	return &DeleteLinkUseCase{
		repo: repo,
	}
}

// Execute deletes a short link, verifying the caller owns it.
func (uc *DeleteLinkUseCase) Execute(ctx context.Context, linkID, userID string) error {
	linkUUID, err := uuid.Parse(linkID)
	if err != nil {
		return fmt.Errorf("delete link: invalid link_id: %w", err)
	}

	// Fetch the link to verify ownership.
	link, err := uc.repo.FindByID(ctx, linkUUID)
	if err != nil {
		return fmt.Errorf("delete link: find: %w", err)
	}
	if link == nil {
		return ErrLinkNotFound
	}
	if link.UserID.String() != userID {
		return ErrPermissionDenied
	}

	if err := uc.repo.Delete(ctx, linkUUID); err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return ErrLinkNotFound
		}
		return fmt.Errorf("delete link: %w", err)
	}

	return nil
}
