// Package postgres provides GORM-based repository implementations.
package postgres

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/aligh5331/gobox/fileupload/internal/domain/model"
)

// paginationCursor is an opaque cursor for keyset pagination.
type paginationCursor struct {
	LastID uuid.UUID `json:"last_id"`
}

// encodeCursor encodes a pagination cursor into a base64 string.
func encodeCursor(lastID uuid.UUID) string {
	c := paginationCursor{LastID: lastID}
	data, err := json.Marshal(c)
	if err != nil {
		return ""
	}
	return base64.URLEncoding.EncodeToString(data)
}

// decodeCursor decodes a base64 cursor string.
func decodeCursor(token string) (uuid.UUID, error) {
	if token == "" {
		return uuid.Nil, nil
	}
	data, err := base64.URLEncoding.DecodeString(token)
	if err != nil {
		return uuid.Nil, fmt.Errorf("decode cursor: %w", err)
	}
	var c paginationCursor
	if err := json.Unmarshal(data, &c); err != nil {
		return uuid.Nil, fmt.Errorf("unmarshal cursor: %w", err)
	}
	return c.LastID, nil
}

// FileRepository is the GORM-backed implementation of repository.FileRepository.
type FileRepository struct {
	db *gorm.DB
}

// NewFileRepository creates a new FileRepository.
func NewFileRepository(db *gorm.DB) *FileRepository {
	return &FileRepository{db: db}
}

func (r *FileRepository) Create(ctx context.Context, file *model.File) error {
	return r.db.WithContext(ctx).Create(toDTO(file)).Error
}

func (r *FileRepository) FindByID(ctx context.Context, id uuid.UUID) (*model.File, error) {
	var dto FileDTO
	err := r.db.WithContext(ctx).
		Where("deleted_at IS NULL").
		First(&dto, "id = ?", id).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, nil
		}
		return nil, fmt.Errorf("postgres: find by id: %w", err)
	}
	return dto.toDomain(), nil
}

func (r *FileRepository) FindByIDAndUser(ctx context.Context, id uuid.UUID, userID string) (*model.File, error) {
	var dto FileDTO
	err := r.db.WithContext(ctx).
		Where("id = ? AND user_id = ? AND deleted_at IS NULL", id, userID).
		First(&dto).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, nil
		}
		return nil, fmt.Errorf("postgres: find by id and user: %w", err)
	}
	return dto.toDomain(), nil
}

func (r *FileRepository) FindByUserID(ctx context.Context, userID uuid.UUID, cursor string, limit int) ([]*model.File, string, error) {
	var dtos []FileDTO
	query := r.db.WithContext(ctx).
		Model(&FileDTO{}).
		Where("user_id = ? AND deleted_at IS NULL", userID).
		Order("created_at DESC").
		Limit(limit + 1)

	if cursor != "" {
		lastID, err := decodeCursor(cursor)
		if err != nil {
			return nil, "", fmt.Errorf("postgres: invalid cursor: %w", err)
		}
		query = query.Where("created_at < (SELECT created_at FROM files WHERE id = ?)", lastID)
	}

	if err := query.Find(&dtos).Error; err != nil {
		return nil, "", fmt.Errorf("postgres: find by user: %w", err)
	}

	files := make([]*model.File, len(dtos))
	for i := range dtos {
		files[i] = dtos[i].toDomain()
	}

	var nextToken string
	if len(files) > limit {
		nextToken = encodeCursor(files[limit-1].ID)
		files = files[:limit]
	}

	return files, nextToken, nil
}

func (r *FileRepository) UpdateStatus(ctx context.Context, id uuid.UUID, status model.FileStatus) error {
	result := r.db.WithContext(ctx).
		Model(&FileDTO{}).
		Where("id = ? AND deleted_at IS NULL", id).
		Update("status", string(status))
	if result.Error != nil {
		return fmt.Errorf("postgres: update status: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return fmt.Errorf("postgres: file not found: %s", id)
	}
	return nil
}

func (r *FileRepository) Update(ctx context.Context, file *model.File) error {
	return r.db.WithContext(ctx).Save(toDTO(file)).Error
}

func (r *FileRepository) SoftDelete(ctx context.Context, id uuid.UUID) error {
	result := r.db.WithContext(ctx).
		Model(&FileDTO{}).
		Where("id = ? AND deleted_at IS NULL", id).
		Update("deleted_at", gorm.Expr("NOW()"))
	if result.Error != nil {
		return fmt.Errorf("postgres: soft delete: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return fmt.Errorf("postgres: file not found: %s", id)
	}
	return nil
}
