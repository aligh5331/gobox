package usecase

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"golang.org/x/crypto/bcrypt"

	"git.0lab.ir/aligh/gobox/auth/internal/domain/model"
	"git.0lab.ir/aligh/gobox/auth/internal/domain/repository"
)

func TestLoginUseCase_Success(t *testing.T) {
	userRepo := new(MockUserRepo)
	sessionRepo := new(MockSessionRepo)
	signer := newTestSigner(t)
	logger := zerolog.Nop()

	uc := NewLoginUseCase(userRepo, sessionRepo, signer, logger)
	email := "alice@example.com"
	password := "ValidPass1!"
	hashed, _ := bcrypt.GenerateFromPassword([]byte(password), BcryptCost)
	userID := uuid.New()

	user := &model.User{
		ID:           userID,
		Email:        email,
		Name:         "Alice",
		PasswordHash: string(hashed),
	}

	userRepo.On("FindByEmail", mock.Anything, email).Return(user, nil)
	sessionRepo.On("Create", mock.Anything, mock.AnythingOfType("*model.Session")).Return(nil)

	output, err := uc.Execute(context.Background(), email, password, "", "")
	assert.NoError(t, err)
	assert.NotNil(t, output)
	assert.Equal(t, email, output.User.Email)
	assert.NotEmpty(t, output.AccessToken)
	assert.NotEmpty(t, output.RefreshToken)
	assert.Len(t, output.RefreshToken, 43)

	userRepo.AssertExpectations(t)
	sessionRepo.AssertExpectations(t)
}

func TestLoginUseCase_WrongPassword(t *testing.T) {
	userRepo := new(MockUserRepo)
	sessionRepo := new(MockSessionRepo)
	signer := newTestSigner(t)
	logger := zerolog.Nop()

	uc := NewLoginUseCase(userRepo, sessionRepo, signer, logger)
	email := "alice@example.com"
	password := "ValidPass1!"
	hashed, _ := bcrypt.GenerateFromPassword([]byte(password), BcryptCost)

	user := &model.User{
		ID:           uuid.New(),
		Email:        email,
		Name:         "Alice",
		PasswordHash: string(hashed),
	}

	userRepo.On("FindByEmail", mock.Anything, email).Return(user, nil)

	output, err := uc.Execute(context.Background(), email, "WrongPass1!", "", "")
	assert.ErrorIs(t, err, ErrInvalidCredentials)
	assert.Nil(t, output)
}

func TestLoginUseCase_UnknownEmail(t *testing.T) {
	userRepo := new(MockUserRepo)
	sessionRepo := new(MockSessionRepo)
	signer := newTestSigner(t)
	logger := zerolog.Nop()

	uc := NewLoginUseCase(userRepo, sessionRepo, signer, logger)

	userRepo.On("FindByEmail", mock.Anything, "unknown@example.com").
		Return(nil, repository.ErrNotFound)

	output, err := uc.Execute(context.Background(), "unknown@example.com", "AnyPass1!", "", "")
	assert.ErrorIs(t, err, ErrInvalidCredentials)
	assert.Nil(t, output)
}
