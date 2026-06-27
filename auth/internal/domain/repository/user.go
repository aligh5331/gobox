// Package repository defines port interfaces that the domain layer depends on.
package repository

import (
	"context"

	"github.com/google/uuid"

	"git.0lab.ir/aligh/gobox/auth/internal/domain/model"
)

// UserRepository defines the persistence contract for User entities.
type UserRepository interface {
	// Create persists a new user.
	Create(ctx context.Context, user *model.User) error
	// FindByID retrieves a user by their primary key.
	FindByID(ctx context.Context, id uuid.UUID) (*model.User, error)
	// FindByEmail retrieves a user by their email address.
	FindByEmail(ctx context.Context, email string) (*model.User, error)
	// Update persists changes to an existing user.
	Update(ctx context.Context, user *model.User) error
}
