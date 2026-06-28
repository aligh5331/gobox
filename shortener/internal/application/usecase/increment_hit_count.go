package usecase

import (
	"context"
	"fmt"

	"github.com/aligh5331/gobox/shortener/internal/domain/repository"
)

// IncrementHitCountUseCase handles incrementing the hit count for a slug.
type IncrementHitCountUseCase struct {
	repo repository.ShortLinkRepository
}

// NewIncrementHitCountUseCase creates a new IncrementHitCountUseCase.
func NewIncrementHitCountUseCase(repo repository.ShortLinkRepository) *IncrementHitCountUseCase {
	return &IncrementHitCountUseCase{
		repo: repo,
	}
}

// Execute increments the hit count for the given slug.
// Errors are logged and swallowed — this is fire-and-forget.
func (uc *IncrementHitCountUseCase) Execute(ctx context.Context, slug string) error {
	if err := uc.repo.IncrementHitCount(ctx, slug); err != nil {
		return fmt.Errorf("increment hit count: %w", err)
	}
	return nil
}
