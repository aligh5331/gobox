package usecase

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	"git.0lab.ir/aligh/gobox/auth/internal/domain/model"
	"git.0lab.ir/aligh/gobox/auth/internal/domain/repository"
)

func TestLogoutUseCase_Success(t *testing.T) {
	sessionRepo := new(MockSessionRepo)
	logger := zerolog.Nop()

	uc := NewLogoutUseCase(sessionRepo, logger)
	sessionID := uuid.New()

	session := &model.Session{
		ID:      sessionID,
		UserID:  uuid.New(),
		Revoked: false,
	}

	sessionRepo.On("FindByID", mock.Anything, sessionID).Return(session, nil)
	sessionRepo.On("Revoke", mock.Anything, sessionID).Return(nil)

	err := uc.Execute(context.Background(), sessionID)
	assert.NoError(t, err)
	sessionRepo.AssertExpectations(t)
}

func TestLogoutUseCase_AlreadyRevoked(t *testing.T) {
	sessionRepo := new(MockSessionRepo)
	logger := zerolog.Nop()

	uc := NewLogoutUseCase(sessionRepo, logger)
	sessionID := uuid.New()

	session := &model.Session{
		ID:      sessionID,
		UserID:  uuid.New(),
		Revoked: true,
		ExpiresAt: time.Now().Add(24 * time.Hour),
	}

	sessionRepo.On("FindByID", mock.Anything, sessionID).Return(session, nil)

	err := uc.Execute(context.Background(), sessionID)
	assert.ErrorIs(t, err, ErrSessionAlreadyRevoked)
	sessionRepo.AssertExpectations(t)
}

func TestLogoutUseCase_NotFound(t *testing.T) {
	sessionRepo := new(MockSessionRepo)
	logger := zerolog.Nop()

	uc := NewLogoutUseCase(sessionRepo, logger)
	sessionID := uuid.New()

	sessionRepo.On("FindByID", mock.Anything, sessionID).
		Return(nil, repository.ErrNotFound)

	err := uc.Execute(context.Background(), sessionID)
	assert.ErrorIs(t, err, ErrSessionNotFound)
	sessionRepo.AssertExpectations(t)
}
