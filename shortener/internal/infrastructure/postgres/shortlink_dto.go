package postgres

import (
	"time"

	"github.com/google/uuid"

	"github.com/aligh5331/gobox/shortener/internal/domain/model"
)

// ShortLinkDTO is the GORM-backed database mapping for a short link record.
// It carries GORM struct tags and is separate from the pure domain model.
type ShortLinkDTO struct {
	ID        uuid.UUID  `gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`
	FileID    uuid.UUID  `gorm:"type:uuid;not null;index"`
	UserID    uuid.UUID  `gorm:"type:uuid;not null;index"`
	Slug      string     `gorm:"type:varchar(6);not null;uniqueIndex"`
	TargetURL string     `gorm:"type:text;not null;default:''"`
	HitCount  int64      `gorm:"not null;default:0"`
	ExpiresAt *time.Time `gorm:"index"`
	CreatedAt time.Time  `gorm:"autoCreateTime"`
}

// TableName overrides the GORM table name for ShortLinkDTO.
func (ShortLinkDTO) TableName() string {
	return "short_links"
}

// toDomain converts a ShortLinkDTO to a domain ShortLink.
func (d *ShortLinkDTO) toDomain() *model.ShortLink {
	return &model.ShortLink{
		ID:        d.ID,
		FileID:    d.FileID,
		UserID:    d.UserID,
		Slug:      d.Slug,
		TargetURL: d.TargetURL,
		HitCount:  d.HitCount,
		ExpiresAt: d.ExpiresAt,
		CreatedAt: d.CreatedAt,
	}
}

// toDTO converts a domain ShortLink to a ShortLinkDTO.
func toShortLinkDTO(l *model.ShortLink) *ShortLinkDTO {
	return &ShortLinkDTO{
		ID:        l.ID,
		FileID:    l.FileID,
		UserID:    l.UserID,
		Slug:      l.Slug,
		TargetURL: l.TargetURL,
		HitCount:  l.HitCount,
		ExpiresAt: l.ExpiresAt,
		CreatedAt: l.CreatedAt,
	}
}
