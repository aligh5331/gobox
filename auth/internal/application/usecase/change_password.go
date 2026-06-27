package usecase

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/rs/zerolog"
	"golang.org/x/crypto/bcrypt"

	"git.0lab.ir/aligh/gobox/auth/internal/domain/repository"
)

// ChangePasswordUseCase handles password changes.
type ChangePasswordUseCase struct {
	userRepo repository.UserRepository
	logger   zerolog.Logger
}

// NewChangePasswordUseCase creates a new ChangePasswordUseCase.
func NewChangePasswordUseCase(
	userRepo repository.UserRepository,
	logger zerolog.Logger,
) *ChangePasswordUseCase {
	return &ChangePasswordUseCase{
		userRepo: userRepo,
		logger:   logger,
	}
}

// Execute changes the user's password after verifying the old one.
func (uc *ChangePasswordUseCase) Execute(ctx context.Context, userID uuid.UUID, oldPass, newPass string) error {
	user, err := uc.userRepo.FindByID(ctx, userID)
	if err != nil {
		if isNotFound(err) {
			return ErrUserNotFound
		}
		return fmt.Errorf("change password: find user: %w", err)
	}

	// Verify old password
	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(oldPass)); err != nil {
		return ErrInvalidPassword
	}

	// Validate new password
	if err := validatePassword(newPass); err != nil {
		return err
	}

	// Hash and update
	hashed, err := hashToken(newPass)
	if err != nil {
		return fmt.Errorf("change password: hash new password: %w", err)
	}

	user.PasswordHash = hashed
	user.UpdatedAt = time.Now()

	if err := uc.userRepo.Update(ctx, user); err != nil {
		return fmt.Errorf("change password: update: %w", err)
	}

	uc.logger.Info().
		Str("user_id", userID.String()).
		Msg("password changed")

	return nil
}
