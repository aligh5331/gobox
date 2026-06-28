package usecase

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	"github.com/aligh5331/gobox/shortener/internal/domain/model"
	"github.com/aligh5331/gobox/shortener/internal/domain/repository"
)

const (
	// MaxPageSize is the maximum number of links per page.
	MaxPageSize = 200
	// DefaultPageSize is used when no page_size is provided.
	DefaultPageSize = 20
)

// ListLinksInput contains the parameters for listing short links.
type ListLinksInput struct {
	UserID    string
	PageSize  int32
	PageToken string
}

// ListLinksOutput contains a page of short links and pagination metadata.
type ListLinksOutput struct {
	Links         []*model.ShortLink
	NextPageToken string
}

// ListLinksUseCase handles paginated listing of short links for a user.
type ListLinksUseCase struct {
	repo repository.ShortLinkRepository
}

// NewListLinksUseCase creates a new ListLinksUseCase.
func NewListLinksUseCase(repo repository.ShortLinkRepository) *ListLinksUseCase {
	return &ListLinksUseCase{
		repo: repo,
	}
}

// Execute returns a page of short links for the given user.
func (uc *ListLinksUseCase) Execute(ctx context.Context, input ListLinksInput) (*ListLinksOutput, error) {
	userUUID, err := uuid.Parse(input.UserID)
	if err != nil {
		return nil, fmt.Errorf("list links: invalid user_id: %w", err)
	}

	pageSize := clampPageSize(int(input.PageSize))

	links, nextToken, err := uc.repo.FindByUserID(ctx, userUUID, input.PageToken, pageSize)
	if err != nil {
		return nil, fmt.Errorf("list links: %w", err)
	}

	out := &ListLinksOutput{
		Links:         links,
		NextPageToken: nextToken,
	}

	return out, nil
}

// clampPageSize ensures pageSize is within valid bounds.
func clampPageSize(pageSize int) int {
	if pageSize <= 0 {
		return DefaultPageSize
	}
	if pageSize > MaxPageSize {
		return MaxPageSize
	}
	return pageSize
}
