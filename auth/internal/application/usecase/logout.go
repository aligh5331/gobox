package usecase

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/rs/zerolog"

	"github.com/aligh5331/gobox/auth/internal/domain/repository"
)

// LogoutUseCase handles single session revocation.
type LogoutUseCase struct {
	sessionRepo repository.SessionRepository
	logger      zerolog.Logger
}

// NewLogoutUseCase creates a new LogoutUseCase.
func NewLogoutUseCase(
	sessionRepo repository.SessionRepository,
	logger zerolog.Logger,
) *LogoutUseCase {
	return &LogoutUseCase{
		sessionRepo: sessionRepo,
		logger:      logger,
	}
}

// Execute revokes the session with the given ID.
func (uc *LogoutUseCase) Execute(ctx context.Context, sessionID uuid.UUID) error {
	session, err := uc.sessionRepo.FindByID(ctx, sessionID)
	if err != nil {
		if isNotFound(err) {
			return ErrSessionNotFound
		}
		return fmt.Errorf("logout: find session: %w", err)
	}

	if session.Revoked {
		return ErrSessionAlreadyRevoked
	}

	if err := uc.sessionRepo.Revoke(ctx, sessionID); err != nil {
		return fmt.Errorf("logout: revoke session: %w", err)
	}

	uc.logger.Info().
		Str("session_id", sessionID.String()).
		Str("user_id", session.UserID.String()).
		Msg("session revoked")

	return nil
}
