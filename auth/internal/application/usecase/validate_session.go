package usecase

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/aligh5331/gobox/auth/internal/domain/repository"
)

// ValidateSessionOutput contains the result of session validation.
type ValidateSessionOutput struct {
	UserID string
	Valid  bool
}

// ValidateSessionUseCase handles validating whether a session is still active.
type ValidateSessionUseCase struct {
	sessionRepo repository.SessionRepository
}

// NewValidateSessionUseCase creates a new ValidateSessionUseCase.
func NewValidateSessionUseCase(
	sessionRepo repository.SessionRepository,
) *ValidateSessionUseCase {
	return &ValidateSessionUseCase{
		sessionRepo: sessionRepo,
	}
}

// Execute validates that a session exists, is not revoked, and has not expired.
// Returns Valid: true with the UserID if the session is active.
// Returns Valid: false (no error) for expected invalid states (not found, revoked, expired).
// Returns an error only for unexpected infrastructure failures.
func (uc *ValidateSessionUseCase) Execute(ctx context.Context, sessionID uuid.UUID) (*ValidateSessionOutput, error) {
	session, err := uc.sessionRepo.FindByID(ctx, sessionID)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return &ValidateSessionOutput{Valid: false}, nil
		}
		return nil, fmt.Errorf("validate session: %w", err)
	}

	if session.Revoked {
		return &ValidateSessionOutput{Valid: false}, nil
	}

	if time.Now().After(session.ExpiresAt) {
		return &ValidateSessionOutput{Valid: false}, nil
	}

	return &ValidateSessionOutput{
		Valid:  true,
		UserID: session.UserID.String(),
	}, nil
}
