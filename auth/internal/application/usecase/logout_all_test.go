package usecase

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

func TestLogoutAllUseCase_Success(t *testing.T) {
	sessionRepo := new(MockSessionRepo)
	logger := zerolog.Nop()

	uc := NewLogoutAllUseCase(sessionRepo, logger)
	userID := uuid.New()

	sessionRepo.On("RevokeAllByUserID", mock.Anything, userID).Return(nil)

	err := uc.Execute(context.Background(), userID)
	assert.NoError(t, err)
	sessionRepo.AssertExpectations(t)
}

func TestLogoutAllUseCase_NoSessions(t *testing.T) {
	sessionRepo := new(MockSessionRepo)
	logger := zerolog.Nop()

	uc := NewLogoutAllUseCase(sessionRepo, logger)
	userID := uuid.New()

	// No active sessions — should still succeed as a no-op
	sessionRepo.On("RevokeAllByUserID", mock.Anything, userID).Return(nil)

	err := uc.Execute(context.Background(), userID)
	assert.NoError(t, err)
	sessionRepo.AssertExpectations(t)
}
