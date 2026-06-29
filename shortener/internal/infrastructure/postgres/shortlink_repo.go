// Package postgres provides GORM-based repository implementations.
package postgres

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/aligh5331/gobox/shortener/internal/domain/model"
	"github.com/aligh5331/gobox/shortener/internal/domain/repository"
)

// paginationCursor is an opaque cursor for keyset pagination.
type paginationCursor struct {
	LastID    uuid.UUID `json:"last_id"`
	CreatedAt time.Time `json:"created_at"`
}

// ShortLinkRepository is the GORM-backed implementation of repository.ShortLinkRepository.
type ShortLinkRepository struct {
	db *gorm.DB
}

// NewShortLinkRepository creates a new ShortLinkRepository.
func NewShortLinkRepository(db *gorm.DB) *ShortLinkRepository {
	return &ShortLinkRepository{db: db}
}

func (r *ShortLinkRepository) Create(ctx context.Context, link *model.ShortLink) error {
	err := r.db.WithContext(ctx).Create(toShortLinkDTO(link)).Error
	if err != nil {
		if isDuplicate(err) {
			return fmt.Errorf("postgres: %w", repository.ErrDuplicateSlug)
		}
		return fmt.Errorf("postgres: create short link: %w", err)
	}
	return nil
}

func (r *ShortLinkRepository) FindBySlug(ctx context.Context, slug string) (*model.ShortLink, error) {
	var dto ShortLinkDTO
	err := r.db.WithContext(ctx).Where("slug = ?", slug).First(&dto).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, nil
		}
		return nil, fmt.Errorf("postgres: find by slug: %w", err)
	}
	return dto.toDomain(), nil
}

func (r *ShortLinkRepository) FindByID(ctx context.Context, id uuid.UUID) (*model.ShortLink, error) {
	var dto ShortLinkDTO
	err := r.db.WithContext(ctx).First(&dto, "id = ?", id).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, nil
		}
		return nil, fmt.Errorf("postgres: find by id: %w", err)
	}
	return dto.toDomain(), nil
}

func (r *ShortLinkRepository) FindByUserID(ctx context.Context, userID uuid.UUID, cursor string, limit int) ([]*model.ShortLink, string, error) {
	var dtos []ShortLinkDTO
	query := r.db.WithContext(ctx).
		Model(&ShortLinkDTO{}).
		Where("user_id = ?", userID).
		Order("created_at DESC").
		Limit(limit + 1)

	if cursor != "" {
		lastID, lastCreatedAt, err := decodeCursor(cursor)
		if err != nil {
			return nil, "", fmt.Errorf("postgres: invalid cursor: %w", err)
		}
		query = query.Where("(created_at < ? OR (created_at = ? AND id < ?))", lastCreatedAt, lastCreatedAt, lastID)
	}

	if err := query.Find(&dtos).Error; err != nil {
		return nil, "", fmt.Errorf("postgres: find by user: %w", err)
	}

	links := make([]*model.ShortLink, len(dtos))
	for i := range dtos {
		links[i] = dtos[i].toDomain()
	}

	var nextToken string
	if len(links) > limit {
		nextToken = encodeCursor(links[limit-1].ID, links[limit-1].CreatedAt)
		links = links[:limit]
	}

	return links, nextToken, nil
}

func (r *ShortLinkRepository) Delete(ctx context.Context, id uuid.UUID) error {
	result := r.db.WithContext(ctx).Delete(&ShortLinkDTO{}, "id = ?", id)
	if result.Error != nil {
		return fmt.Errorf("postgres: delete: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return fmt.Errorf("postgres: %w", repository.ErrNotFound)
	}
	return nil
}

func (r *ShortLinkRepository) IncrementHitCount(ctx context.Context, slug string) error {
	result := r.db.WithContext(ctx).
		Model(&ShortLinkDTO{}).
		Where("slug = ?", slug).
		UpdateColumn("hit_count", gorm.Expr("hit_count + 1"))
	if result.Error != nil {
		return fmt.Errorf("postgres: increment hit count: %w", result.Error)
	}
	return nil
}

// isDuplicate checks if a GORM error is a PostgreSQL unique constraint violation.
func isDuplicate(err error) bool {
	return err != nil && contains(err.Error(), "duplicate key value violates unique constraint")
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && searchString(s, substr)
}

func searchString(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

func encodeCursor(lastID uuid.UUID, createdAt time.Time) string {
	c := paginationCursor{LastID: lastID, CreatedAt: createdAt}
	data, err := json.Marshal(c)
	if err != nil {
		return ""
	}
	return base64.URLEncoding.EncodeToString(data)
}

func decodeCursor(token string) (uuid.UUID, time.Time, error) {
	if token == "" {
		return uuid.Nil, time.Time{}, nil
	}
	data, err := base64.URLEncoding.DecodeString(token)
	if err != nil {
		return uuid.Nil, time.Time{}, fmt.Errorf("decode cursor: %w", err)
	}
	var c paginationCursor
	if err := json.Unmarshal(data, &c); err != nil {
		return uuid.Nil, time.Time{}, fmt.Errorf("unmarshal cursor: %w", err)
	}
	return c.LastID, c.CreatedAt, nil
}
