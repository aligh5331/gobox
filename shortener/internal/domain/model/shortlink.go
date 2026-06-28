// Package model holds pure domain entities with no framework dependencies.
package model

import (
	"time"

	"github.com/google/uuid"
)

// ShortLink represents a short link pointing to a file.
type ShortLink struct {
	ID        uuid.UUID  `gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`
	FileID    uuid.UUID  `gorm:"type:uuid;not null;index"`
	UserID    uuid.UUID  `gorm:"type:uuid;not null;index"`
	Slug      string     `gorm:"type:varchar(6);not null;uniqueIndex"`
	TargetURL string     `gorm:"type:text;not null;default:''"`
	HitCount  int64      `gorm:"not null;default:0"`
	ExpiresAt *time.Time `gorm:"index"`
	CreatedAt time.Time  `gorm:"autoCreateTime"`
}

// TableName overrides the GORM table name.
func (ShortLink) TableName() string {
	return "short_links"
}

// IsExpired returns true if the link has an expiry and it has passed.
func (s *ShortLink) IsExpired() bool {
	if s.ExpiresAt == nil {
		return false
	}
	return time.Now().After(*s.ExpiresAt)
}
