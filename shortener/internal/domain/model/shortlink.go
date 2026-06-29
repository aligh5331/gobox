// Package model holds pure domain entities with no framework dependencies.
package model

import (
	"time"

	"github.com/google/uuid"
)

// ShortLink represents a short link pointing to a file.
type ShortLink struct {
	ID        uuid.UUID
	FileID    uuid.UUID
	UserID    uuid.UUID
	Slug      string
	TargetURL string
	HitCount  int64
	ExpiresAt *time.Time
	CreatedAt time.Time
}

// IsExpired returns true if the link has an expiry and it has passed.
func (s *ShortLink) IsExpired() bool {
	if s.ExpiresAt == nil {
		return false
	}
	return time.Now().After(*s.ExpiresAt)
}
