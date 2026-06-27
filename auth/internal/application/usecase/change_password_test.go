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

func TestChangePasswordUseCase_Success(t *testing.T) {
	userRepo := new(MockUserRepo)
	logger := zerolog.Nop()

	uc := NewChangePasswordUseCase(userRepo, logger)
	userID := uuid.New()

	oldHash, _ := bcrypt.GenerateFromPassword([]byte("OldPass1!"), BcryptCost)
	user := &model.User{
		ID:           userID,
		Email:        "alice@example.com",
		Name:         "Alice",
		PasswordHash: string(oldHash),
	}

	userRepo.On("FindByID", mock.Anything, userID).Return(user, nil)
	userRepo.On("Update", mock.Anything, mock.MatchedBy(func(u *model.User) bool {
		// Verify new password hash matches
		err := bcrypt.CompareHashAndPassword([]byte(u.PasswordHash), []byte("NewPass2!"))
		return err == nil
	})).Return(nil)

	err := uc.Execute(context.Background(), userID, "OldPass1!", "NewPass2!")
	assert.NoError(t, err)
	userRepo.AssertExpectations(t)
}

func TestChangePasswordUseCase_WrongOldPassword(t *testing.T) {
	userRepo := new(MockUserRepo)
	logger := zerolog.Nop()

	uc := NewChangePasswordUseCase(userRepo, logger)
	userID := uuid.New()

	oldHash, _ := bcrypt.GenerateFromPassword([]byte("OldPass1!"), BcryptCost)
	user := &model.User{
		ID:           userID,
		Email:        "alice@example.com",
		Name:         "Alice",
		PasswordHash: string(oldHash),
	}

	userRepo.On("FindByID", mock.Anything, userID).Return(user, nil)

	err := uc.Execute(context.Background(), userID, "WrongPass1!", "NewPass2!")
	assert.ErrorIs(t, err, ErrInvalidPassword)
}

func TestChangePasswordUseCase_WeakNewPassword(t *testing.T) {
	userRepo := new(MockUserRepo)
	logger := zerolog.Nop()

	uc := NewChangePasswordUseCase(userRepo, logger)
	userID := uuid.New()

	oldHash, _ := bcrypt.GenerateFromPassword([]byte("OldPass1!"), BcryptCost)
	user := &model.User{
		ID:           userID,
		Email:        "alice@example.com",
		Name:         "Alice",
		PasswordHash: string(oldHash),
	}

	userRepo.On("FindByID", mock.Anything, userID).Return(user, nil)

	err := uc.Execute(context.Background(), userID, "OldPass1!", "short")
	assert.ErrorIs(t, err, ErrWeakPassword)
}

func TestChangePasswordUseCase_UserNotFound(t *testing.T) {
	userRepo := new(MockUserRepo)
	logger := zerolog.Nop()

	uc := NewChangePasswordUseCase(userRepo, logger)
	userID := uuid.New()

	userRepo.On("FindByID", mock.Anything, userID).
		Return(nil, repository.ErrNotFound)

	err := uc.Execute(context.Background(), userID, "OldPass1!", "NewPass2!")
	assert.ErrorIs(t, err, ErrUserNotFound)
}
