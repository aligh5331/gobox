package usecase

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/rs/zerolog"

	"github.com/aligh5331/gobox/auth/internal/domain/model"
	"github.com/aligh5331/gobox/auth/internal/domain/repository"
)

// UpdateProfileUseCase handles user display name updates.
type UpdateProfileUseCase struct {
	userRepo repository.UserRepository
	logger   zerolog.Logger
}

// NewUpdateProfileUseCase creates a new UpdateProfileUseCase.
func NewUpdateProfileUseCase(
	userRepo repository.UserRepository,
	logger zerolog.Logger,
) *UpdateProfileUseCase {
	return &UpdateProfileUseCase{
		userRepo: userRepo,
		logger:   logger,
	}
}

// Execute updates a user's display name.
func (uc *UpdateProfileUseCase) Execute(ctx context.Context, userID uuid.UUID, name string) (*model.User, error) {
	if name == "" {
		return nil, ErrInvalidName
	}

	user, err := uc.userRepo.FindByID(ctx, userID)
	if err != nil {
		if isNotFound(err) {
			return nil, ErrUserNotFound
		}
		return nil, fmt.Errorf("update profile: find user: %w", err)
	}

	user.Name = name
	user.UpdatedAt = time.Now()

	if err := uc.userRepo.Update(ctx, user); err != nil {
		return nil, fmt.Errorf("update profile: update user: %w", err)
	}

	uc.logger.Info().
		Str("user_id", userID.String()).
		Str("new_name", name).
		Msg("profile updated")

	return user, nil
}
