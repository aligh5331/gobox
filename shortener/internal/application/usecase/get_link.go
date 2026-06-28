package usecase

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	"github.com/aligh5331/gobox/shortener/internal/domain/model"
	"github.com/aligh5331/gobox/shortener/internal/domain/repository"
)

// GetLinkUseCase handles retrieving a short link by slug (public redirect).
type GetLinkUseCase struct {
	repo repository.ShortLinkRepository
}

// NewGetLinkUseCase creates a new GetLinkUseCase.
func NewGetLinkUseCase(repo repository.ShortLinkRepository) *GetLinkUseCase {
	return &GetLinkUseCase{
		repo: repo,
	}
}

// Execute retrieves a short link by its slug.
// Returns ErrLinkNotFound if the slug does not exist.
// Returns ErrLinkExpired if the link has expired.
func (uc *GetLinkUseCase) Execute(ctx context.Context, slug string) (*model.ShortLink, error) {
	link, err := uc.repo.FindBySlug(ctx, slug)
	if err != nil {
		return nil, fmt.Errorf("get link: %w", err)
	}
	if link == nil {
		return nil, ErrLinkNotFound
	}
	if link.IsExpired() {
		return nil, ErrLinkExpired
	}
	return link, nil
}

// GetLinkByIDUseCase handles retrieving a short link by UUID (authenticated).
type GetLinkByIDUseCase struct {
	repo repository.ShortLinkRepository
}

// NewGetLinkByIDUseCase creates a new GetLinkByIDUseCase.
func NewGetLinkByIDUseCase(repo repository.ShortLinkRepository) *GetLinkByIDUseCase {
	return &GetLinkByIDUseCase{
		repo: repo,
	}
}

// Execute retrieves a short link by its UUID, scoped to the requesting user.
func (uc *GetLinkByIDUseCase) Execute(ctx context.Context, linkID, userID string) (*model.ShortLink, error) {
	linkUUID, err := uuid.Parse(linkID)
	if err != nil {
		return nil, fmt.Errorf("get link by id: invalid link_id: %w", err)
	}

	link, err := uc.repo.FindByID(ctx, linkUUID)
	if err != nil {
		return nil, fmt.Errorf("get link by id: %w", err)
	}
	if link == nil {
		return nil, ErrLinkNotFound
	}
	if link.UserID.String() != userID {
		return nil, ErrLinkNotFound
	}
	return link, nil
}
