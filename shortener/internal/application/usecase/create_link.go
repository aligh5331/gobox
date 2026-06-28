package usecase

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/aligh5331/gobox/shortener/internal/domain/model"
	"github.com/aligh5331/gobox/shortener/internal/domain/repository"
	"github.com/aligh5331/gobox/shortener/pkg/slug"
)

// SlugGenerator generates unique slugs.
type SlugGenerator interface {
	Generate() (string, error)
}

// CreateLinkInput contains the parameters for creating a short link.
type CreateLinkInput struct {
	UserID    string
	FileID    string
	TargetURL string
	ExpiresAt *time.Time
}

// CreateLinkOutput contains the result of creating a short link.
type CreateLinkOutput struct {
	Link *model.ShortLink
}

// CreateLinkUseCase handles creating a new short link.
type CreateLinkUseCase struct {
	repo   repository.ShortLinkRepository
	slugs  SlugGenerator
	config CreateLinkConfig
}

// CreateLinkConfig holds configuration for the create link use case.
type CreateLinkConfig struct {
	BaseURL string
}

// NewCreateLinkUseCase creates a new CreateLinkUseCase.
func NewCreateLinkUseCase(repo repository.ShortLinkRepository, slugs SlugGenerator, baseURL string) *CreateLinkUseCase {
	return &CreateLinkUseCase{
		repo:  repo,
		slugs: slugs,
		config: CreateLinkConfig{
			BaseURL: baseURL,
		},
	}
}

// Execute creates a new short link, retrying on slug collision up to MaxRetries times.
func (uc *CreateLinkUseCase) Execute(ctx context.Context, input CreateLinkInput) (*CreateLinkOutput, error) {
	if input.FileID == "" {
		return nil, ErrMissingFileID
	}

	userUUID, err := uuid.Parse(input.UserID)
	if err != nil {
		return nil, fmt.Errorf("create link: invalid user_id: %w", err)
	}

	fileUUID, err := uuid.Parse(input.FileID)
	if err != nil {
		return nil, fmt.Errorf("create link: invalid file_id: %w", err)
	}

	var link *model.ShortLink
	for range slug.MaxRetries {
		slugStr, genErr := uc.slugs.Generate()
		if genErr != nil {
			return nil, fmt.Errorf("create link: generate slug: %w", genErr)
		}

		now := time.Now()
		link = &model.ShortLink{
			ID:        uuid.New(),
			FileID:    fileUUID,
			UserID:    userUUID,
			Slug:      slugStr,
			TargetURL: input.TargetURL,
			ExpiresAt: input.ExpiresAt,
			CreatedAt: now,
		}

		if err := uc.repo.Create(ctx, link); err != nil {
			if errors.Is(err, repository.ErrDuplicateSlug) {
				continue // collision — retry with a new slug
			}
			return nil, fmt.Errorf("create link: %w", err)
		}

		// Success.
		return &CreateLinkOutput{Link: link}, nil
	}

	return nil, ErrSlugCollision
}
