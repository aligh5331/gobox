package usecase

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	"github.com/aligh5331/gobox/auth/internal/domain/model"
	"github.com/aligh5331/gobox/auth/internal/domain/repository"
)

func TestUpdateProfileUseCase_Success(t *testing.T) {
	userRepo := new(MockUserRepo)
	logger := zerolog.Nop()

	uc := NewUpdateProfileUseCase(userRepo, logger)
	userID := uuid.New()

	original := &model.User{
		ID:    userID,
		Email: "alice@example.com",
		Name:  "Alice",
	}

	userRepo.On("FindByID", mock.Anything, userID).Return(original, nil)
	userRepo.On("Update", mock.Anything, mock.MatchedBy(func(u *model.User) bool {
		return u.Name == "Alice Updated" && u.Email == "alice@example.com"
	})).Return(nil)

	updated, err := uc.Execute(context.Background(), userID, "Alice Updated")
	assert.NoError(t, err)
	assert.Equal(t, "Alice Updated", updated.Name)
	assert.Equal(t, "alice@example.com", updated.Email)
	userRepo.AssertExpectations(t)
}

func TestUpdateProfileUseCase_EmptyName(t *testing.T) {
	userRepo := new(MockUserRepo)
	logger := zerolog.Nop()

	uc := NewUpdateProfileUseCase(userRepo, logger)
	userID := uuid.New()

	original := &model.User{
		ID:    userID,
		Email: "alice@example.com",
		Name:  "Alice",
	}

	userRepo.On("FindByID", mock.Anything, userID).Return(original, nil)

	updated, err := uc.Execute(context.Background(), userID, "")
	assert.ErrorIs(t, err, ErrInvalidName)
	assert.Nil(t, updated)
}

func TestUpdateProfileUseCase_NotFound(t *testing.T) {
	userRepo := new(MockUserRepo)
	logger := zerolog.Nop()

	uc := NewUpdateProfileUseCase(userRepo, logger)
	userID := uuid.New()

	userRepo.On("FindByID", mock.Anything, userID).
		Return(nil, repository.ErrNotFound)

	updated, err := uc.Execute(context.Background(), userID, "New Name")
	assert.ErrorIs(t, err, ErrUserNotFound)
	assert.Nil(t, updated)
}
