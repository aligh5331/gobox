package repository

import (
	"context"

	"github.com/google/uuid"

	"github.com/aligh5331/gobox/auth/internal/domain/model"
)

// SessionRepository defines the persistence contract for Session entities.
type SessionRepository interface {
	// Create persists a new session.
	Create(ctx context.Context, session *model.Session) error
	// FindByID retrieves a session by its primary key.
	FindByID(ctx context.Context, id uuid.UUID) (*model.Session, error)
	// FindByUserID retrieves all sessions belonging to a user.
	FindByUserID(ctx context.Context, userID uuid.UUID) ([]model.Session, error)
	// FindByRefreshToken scans non-expired sessions for a refresh token
	// match using bcrypt comparison. Returns sessions regardless of their
	// revoked or consumed state.
	FindByRefreshToken(ctx context.Context, rawToken string) (*model.Session, error)
	// Delete removes a session by its primary key.
	Delete(ctx context.Context, id uuid.UUID) error
	// Revoke marks a session as revoked.
	Revoke(ctx context.Context, id uuid.UUID) error
	// RevokeAllByUserID marks every non-revoked session for a user as revoked.
	RevokeAllByUserID(ctx context.Context, userID uuid.UUID) error
	// Rotate atomically deletes the old session and creates a new one.
	// Returns the newly created Session. Returns ErrTokenReuse if the
	// old session was already deleted (token theft detected).
	Rotate(ctx context.Context, oldSessionID uuid.UUID, newSession *model.Session) (*model.Session, error)
}
