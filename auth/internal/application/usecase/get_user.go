package usecase

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/rs/zerolog"

	"github.com/aligh5331/gobox/auth/internal/domain/model"
	"github.com/aligh5331/gobox/auth/internal/domain/repository"
)

// GetUserUseCase handles user profile retrieval.
type GetUserUseCase struct {
	userRepo repository.UserRepository
	logger   zerolog.Logger
}

// NewGetUserUseCase creates a new GetUserUseCase.
func NewGetUserUseCase(
	userRepo repository.UserRepository,
	logger zerolog.Logger,
) *GetUserUseCase {
	return &GetUserUseCase{
		userRepo: userRepo,
		logger:   logger,
	}
}

// Execute retrieves a user by their ID.
func (uc *GetUserUseCase) Execute(ctx context.Context, userID uuid.UUID) (*model.User, error) {
	user, err := uc.userRepo.FindByID(ctx, userID)
	if err != nil {
		if isNotFound(err) {
			return nil, ErrUserNotFound
		}
		return nil, fmt.Errorf("get user: find: %w", err)
	}

	return user, nil
}
