package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/aligh5331/gobox/auth/internal/domain/model"
	"github.com/aligh5331/gobox/auth/internal/domain/repository"
)

// UserRepo implements repository.UserRepository using GORM.
type UserRepo struct {
	db *gorm.DB
}

// NewUserRepo creates a new UserRepo.
func NewUserRepo(db *gorm.DB) *UserRepo {
	return &UserRepo{db: db}
}

func (r *UserRepo) Create(ctx context.Context, user *model.User) error {
	m := toGormUser(user)
	if err := r.db.WithContext(ctx).Create(m).Error; err != nil {
		return fmt.Errorf("postgres: create user: %w", err)
	}
	// Copy back generated fields (e.g. ID if set by DB default)
	*user = *toDomainUser(m)
	return nil
}

func (r *UserRepo) FindByID(ctx context.Context, id uuid.UUID) (*model.User, error) {
	var m UserModel
	result := r.db.WithContext(ctx).Where("id = ?", id.String()).First(&m)
	if errors.Is(result.Error, gorm.ErrRecordNotFound) {
		return nil, repository.ErrNotFound
	}
	if result.Error != nil {
		return nil, fmt.Errorf("postgres: find user by id: %w", result.Error)
	}
	return toDomainUser(&m), nil
}

func (r *UserRepo) FindByEmail(ctx context.Context, email string) (*model.User, error) {
	var m UserModel
	result := r.db.WithContext(ctx).Where("email = ?", email).First(&m)
	if errors.Is(result.Error, gorm.ErrRecordNotFound) {
		return nil, repository.ErrNotFound
	}
	if result.Error != nil {
		return nil, fmt.Errorf("postgres: find user by email: %w", result.Error)
	}
	return toDomainUser(&m), nil
}

func (r *UserRepo) Update(ctx context.Context, user *model.User) error {
	m := toGormUser(user)
	result := r.db.WithContext(ctx).Model(&UserModel{}).Where("id = ?", user.ID.String()).Updates(m)
	if result.Error != nil {
		return fmt.Errorf("postgres: update user: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return repository.ErrNotFound
	}
	return nil
}
