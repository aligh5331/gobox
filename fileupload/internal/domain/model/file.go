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
	ID         uuid.UUID  `gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`
	UserID     uuid.UUID  `gorm:"type:uuid;not null;index"`
	Name       string     `gorm:"type:varchar(255);not null"`
	Size       int64      `gorm:"not null;default:0"`
	MimeType   string     `gorm:"type:varchar(127);not null;default:'application/octet-stream'"`
	StorageKey string     `gorm:"type:text;not null"`
	Status     FileStatus `gorm:"type:varchar(16);not null;default:'pending'"`
	CreatedAt  time.Time  `gorm:"autoCreateTime"`
	UpdatedAt  time.Time  `gorm:"autoUpdateTime"`
	DeletedAt  *time.Time `gorm:"index"`
}

// TableName overrides the GORM table name.
func (File) TableName() string {
	return "files"
}
