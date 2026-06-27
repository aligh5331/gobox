// Package repository defines port interfaces that the domain layer depends on.
package repository

import (
	"context"

	"github.com/google/uuid"

	"github.com/aligh5331/gobox/fileupload/internal/domain/model"
)

// FileRepository defines the persistence contract for File entities.
type FileRepository interface {
	// Create persists a new file record.
	Create(ctx context.Context, file *model.File) error

	// FindByID retrieves a file by its primary key.
	// Returns nil, nil if not found.
	FindByID(ctx context.Context, id uuid.UUID) (*model.File, error)

	// FindByIDAndUser retrieves a file by ID and user ID (not soft-deleted).
	// Returns nil, nil if not found.
	FindByIDAndUser(ctx context.Context, id uuid.UUID, userID string) (*model.File, error)

	// FindByUserID returns a page of files for a given user, ordered by
	// created_at descending. The cursor is an opaque base64-encoded token;
	// pass empty string for the first page. Returns the file list and a
	// next page token (empty if this is the last page).
	FindByUserID(ctx context.Context, userID uuid.UUID, cursor string, limit int) ([]*model.File, string, error)

	// UpdateStatus changes the file's status.
	UpdateStatus(ctx context.Context, id uuid.UUID, status model.FileStatus) error

	// Update persists all mutable fields of a file record.
	Update(ctx context.Context, file *model.File) error

	// SoftDelete sets the deleted_at timestamp for a file.
	SoftDelete(ctx context.Context, id uuid.UUID) error
}
