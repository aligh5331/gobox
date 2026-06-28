package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"

	"github.com/aligh5331/gobox/auth/internal/domain/model"
	"github.com/aligh5331/gobox/auth/internal/domain/repository"
)

// SessionRepo implements repository.SessionRepository using GORM.
type SessionRepo struct {
	db *gorm.DB
}

// NewSessionRepo creates a new SessionRepo.
func NewSessionRepo(db *gorm.DB) *SessionRepo {
	return &SessionRepo{db: db}
}

func (r *SessionRepo) Create(ctx context.Context, session *model.Session) error {
	m := toGormSession(session)
	if err := r.db.WithContext(ctx).Create(m).Error; err != nil {
		return fmt.Errorf("postgres: create session: %w", err)
	}
	*session = *toDomainSession(m)
	return nil
}

func (r *SessionRepo) FindByID(ctx context.Context, id uuid.UUID) (*model.Session, error) {
	var m SessionModel
	result := r.db.WithContext(ctx).Where("id = ?", id.String()).First(&m)
	if errors.Is(result.Error, gorm.ErrRecordNotFound) {
		return nil, repository.ErrNotFound
	}
	if result.Error != nil {
		return nil, fmt.Errorf("postgres: find session by id: %w", result.Error)
	}
	return toDomainSession(&m), nil
}

func (r *SessionRepo) FindByUserID(ctx context.Context, userID uuid.UUID) ([]model.Session, error) {
	var models []SessionModel
	result := r.db.WithContext(ctx).Where("user_id = ?", userID.String()).Find(&models)
	if result.Error != nil {
		return nil, fmt.Errorf("postgres: find sessions by user id: %w", result.Error)
	}
	sessions := make([]model.Session, len(models))
	for i := range models {
		sessions[i] = *toDomainSession(&models[i])
	}
	return sessions, nil
}

func (r *SessionRepo) FindByRefreshToken(ctx context.Context, rawToken string) (*model.Session, error) {
	now := time.Now()
	var models []SessionModel
	result := r.db.WithContext(ctx).
		Where("expires_at > ?", now).
		Find(&models)
	if result.Error != nil {
		return nil, fmt.Errorf("postgres: find sessions for refresh: %w", result.Error)
	}

	for i := range models {
		err := bcrypt.CompareHashAndPassword([]byte(models[i].RefreshTokenHash), []byte(rawToken))
		if err == nil {
			return toDomainSession(&models[i]), nil
		}
	}
	return nil, repository.ErrNotFound
}

func (r *SessionRepo) Delete(ctx context.Context, id uuid.UUID) error {
	result := r.db.WithContext(ctx).Where("id = ?", id.String()).Delete(&SessionModel{})
	if result.Error != nil {
		return fmt.Errorf("postgres: delete session: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return repository.ErrNotFound
	}
	return nil
}

func (r *SessionRepo) Revoke(ctx context.Context, id uuid.UUID) error {
	result := r.db.WithContext(ctx).
		Model(&SessionModel{}).
		Where("id = ?", id.String()).
		Update("revoked", true)
	if result.Error != nil {
		return fmt.Errorf("postgres: revoke session: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return repository.ErrNotFound
	}
	return nil
}

func (r *SessionRepo) RevokeAllByUserID(ctx context.Context, userID uuid.UUID) error {
	result := r.db.WithContext(ctx).
		Model(&SessionModel{}).
		Where("user_id = ? AND revoked = ?", userID.String(), false).
		Update("revoked", true)
	if result.Error != nil {
		return fmt.Errorf("postgres: revoke all sessions: %w", result.Error)
	}
	return nil
}

func (r *SessionRepo) Rotate(ctx context.Context, oldSessionID uuid.UUID, newSession *model.Session) (*model.Session, error) {
	tx := r.db.Begin()
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	// Mark old session as consumed atomically (fail concurrent reuse)
	result := tx.WithContext(ctx).
		Model(&SessionModel{}).
		Where("id = ? AND consumed = ?", oldSessionID.String(), false).
		Updates(map[string]interface{}{"consumed": true, "revoked": true})
	if result.Error != nil {
		tx.Rollback()
		return nil, fmt.Errorf("postgres: rotate consume old: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		tx.Rollback()
		return nil, repository.ErrTokenReuse
	}

	// Create new session
	m := toGormSession(newSession)
	if err := tx.WithContext(ctx).Create(m).Error; err != nil {
		tx.Rollback()
		return nil, fmt.Errorf("postgres: rotate create new: %w", err)
	}

	if err := tx.Commit().Error; err != nil {
		return nil, fmt.Errorf("postgres: rotate commit: %w", err)
	}

	return toDomainSession(m), nil
}
