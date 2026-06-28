package model

import (
	"time"

	"github.com/google/uuid"
)

// Session represents an authenticated login session.
type Session struct {
	ID               uuid.UUID
	UserID           uuid.UUID
	RefreshTokenHash string
	UserAgent        string
	IP               string
	CreatedAt        time.Time
	LastUsedAt       time.Time
	ExpiresAt        time.Time
	Revoked          bool
	Consumed         bool
}
