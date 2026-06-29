package postgres

import (
	"time"

	"github.com/google/uuid"

	"github.com/aligh5331/gobox/fileupload/internal/domain/model"
)

// FileDTO is the GORM-backed database mapping for a file record.
// It carries GORM struct tags and is separate from the pure domain model.
type FileDTO struct {
	ID         uuid.UUID  `gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`
	UserID     uuid.UUID  `gorm:"type:uuid;not null;index"`
	Name       string     `gorm:"type:varchar(255);not null"`
	Size       int64      `gorm:"not null;default:0"`
	MimeType   string     `gorm:"type:varchar(127);not null;default:'application/octet-stream'"`
	StorageKey string     `gorm:"type:text;not null"`
	Status     string     `gorm:"type:varchar(16);not null;default:'pending'"`
	CreatedAt  time.Time  `gorm:"autoCreateTime"`
	UpdatedAt  time.Time  `gorm:"autoUpdateTime"`
	DeletedAt  *time.Time `gorm:"index"`
}

// TableName overrides the GORM table name for FileDTO.
func (FileDTO) TableName() string {
	return "files"
}

// toDomain converts a FileDTO to a domain File.
func (d *FileDTO) toDomain() *model.File {
	return &model.File{
		ID:         d.ID,
		UserID:     d.UserID,
		Name:       d.Name,
		Size:       d.Size,
		MimeType:   d.MimeType,
		StorageKey: d.StorageKey,
		Status:     model.FileStatus(d.Status),
		CreatedAt:  d.CreatedAt,
		UpdatedAt:  d.UpdatedAt,
		DeletedAt:  d.DeletedAt,
	}
}

// toDTO converts a domain File to a FileDTO.
func toDTO(f *model.File) *FileDTO {
	return &FileDTO{
		ID:         f.ID,
		UserID:     f.UserID,
		Name:       f.Name,
		Size:       f.Size,
		MimeType:   f.MimeType,
		StorageKey: f.StorageKey,
		Status:     string(f.Status),
		CreatedAt:  f.CreatedAt,
		UpdatedAt:  f.UpdatedAt,
		DeletedAt:  f.DeletedAt,
	}
}
