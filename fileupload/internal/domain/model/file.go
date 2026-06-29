// Package model holds pure domain entities with no framework dependencies.
package model

import (
	"time"

	"github.com/google/uuid"
)

// FileStatus represents the lifecycle state of a file.
type FileStatus string

const (
	FileStatusPending FileStatus = "pending"
	FileStatusReady   FileStatus = "ready"
	FileStatusFailed  FileStatus = "failed"
)

// File represents a file's metadata in the system.
// The actual blob is stored in S3/MinIO; only metadata lives in Postgres.
type File struct {
	ID         uuid.UUID
	UserID     uuid.UUID
	Name       string
	Size       int64
	MimeType   string
	StorageKey string
	Status     FileStatus
	CreatedAt  time.Time
	UpdatedAt  time.Time
	DeletedAt  *time.Time
}
