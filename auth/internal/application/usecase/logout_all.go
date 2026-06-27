package usecase

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/rs/zerolog"

	"git.0lab.ir/aligh/gobox/auth/internal/domain/repository"
)

// LogoutAllUseCase handles revocation of all user sessions.
type LogoutAllUseCase struct {
	sessionRepo repository.SessionRepository
	logger      zerolog.Logger
}

// NewLogoutAllUseCase creates a new LogoutAllUseCase.
func NewLogoutAllUseCase(
	sessionRepo repository.SessionRepository,
	logger zerolog.Logger,
) *LogoutAllUseCase {
	return &LogoutAllUseCase{
		sessionRepo: sessionRepo,
		logger:      logger,
	}
}

// Execute revokes all non-revoked sessions for the given user.
func (uc *LogoutAllUseCase) Execute(ctx context.Context, userID uuid.UUID) error {
	if err := uc.sessionRepo.RevokeAllByUserID(ctx, userID); err != nil {
		return fmt.Errorf("logout all: revoke sessions: %w", err)
	}

	uc.logger.Info().
		Str("user_id", userID.String()).
		Msg("all sessions revoked")

	return nil
}
