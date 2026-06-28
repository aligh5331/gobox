// Package repository defines port interfaces that the domain layer depends on.
package repository

import (
	"context"

	"github.com/google/uuid"

	"github.com/aligh5331/gobox/shortener/internal/domain/model"
)

// ShortLinkRepository defines the persistence contract for ShortLink entities.
type ShortLinkRepository interface {
	// Create persists a new short link. Returns ErrDuplicateSlug if slug
	// violates the unique constraint.
	Create(ctx context.Context, link *model.ShortLink) error

	// FindBySlug retrieves a short link by its slug.
	// Returns nil, nil if not found.
	FindBySlug(ctx context.Context, slug string) (*model.ShortLink, error)

	// FindByID retrieves a short link by its primary key.
	// Returns nil, nil if not found.
	FindByID(ctx context.Context, id uuid.UUID) (*model.ShortLink, error)

	// FindByUserID returns a page of short links for a given user, ordered by
	// created_at descending. The cursor is an opaque base64-encoded token;
	// pass empty string for the first page. Returns the link list and a
	// next page token (empty if this is the last page).
	FindByUserID(ctx context.Context, userID uuid.UUID, cursor string, limit int) ([]*model.ShortLink, string, error)

	// Delete hard-deletes a short link by its primary key.
	Delete(ctx context.Context, id uuid.UUID) error

	// IncrementHitCount atomically increments the hit_count for a slug.
	IncrementHitCount(ctx context.Context, slug string) error
}
