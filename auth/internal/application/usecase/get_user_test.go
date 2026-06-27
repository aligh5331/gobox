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

func TestGetUserUseCase_Success(t *testing.T) {
	userRepo := new(MockUserRepo)
	logger := zerolog.Nop()

	uc := NewGetUserUseCase(userRepo, logger)
	userID := uuid.New()

	expected := &model.User{
		ID:    userID,
		Email: "alice@example.com",
		Name:  "Alice",
	}

	userRepo.On("FindByID", mock.Anything, userID).Return(expected, nil)

	user, err := uc.Execute(context.Background(), userID)
	assert.NoError(t, err)
	assert.Equal(t, expected.Email, user.Email)
	assert.Equal(t, expected.Name, user.Name)
}

func TestGetUserUseCase_NotFound(t *testing.T) {
	userRepo := new(MockUserRepo)
	logger := zerolog.Nop()

	uc := NewGetUserUseCase(userRepo, logger)
	userID := uuid.MustParse("00000000-0000-0000-0000-000000000000")

	userRepo.On("FindByID", mock.Anything, userID).
		Return(nil, repository.ErrNotFound)

	user, err := uc.Execute(context.Background(), userID)
	assert.ErrorIs(t, err, ErrUserNotFound)
	assert.Nil(t, user)
}
