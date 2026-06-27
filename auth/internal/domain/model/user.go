// Package model holds pure domain entities with no framework dependencies.
package model

import (
	"time"

	"github.com/google/uuid"
)

// User represents a registered user in the system.
type User struct {
	ID           uuid.UUID
	Email        string
	Name         string
	PasswordHash string
	CreatedAt    time.Time
	UpdatedAt    time.Time
}
