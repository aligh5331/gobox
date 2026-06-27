package usecase

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	"github.com/aligh5331/gobox/auth/internal/domain/model"
	"github.com/aligh5331/gobox/auth/internal/domain/repository"
)

func TestRefreshTokenUseCase_Success(t *testing.T) {
	userRepo := new(MockUserRepo)
	sessionRepo := new(MockSessionRepo)
	signer := newTestSigner(t)
	logger := zerolog.Nop()

	uc := NewRefreshTokenUseCase(userRepo, sessionRepo, signer, logger)

	userID := uuid.New()
	sessionID := uuid.New()
	refreshToken := "rt_original"

	session := &model.Session{
		ID:        sessionID,
		UserID:    userID,
		ExpiresAt: time.Now().Add(24 * time.Hour),
		Revoked:   false,
	}
	user := &model.User{
		ID:    userID,
		Email: "alice@example.com",
		Name:  "Alice",
	}

	sessionRepo.On("FindByRefreshToken", mock.Anything, refreshToken).Return(session, nil)
	userRepo.On("FindByID", mock.Anything, userID).Return(user, nil)
	sessionRepo.On("Rotate", mock.Anything, sessionID, mock.AnythingOfType("*model.Session")).
		Return(&model.Session{ID: uuid.New(), UserID: userID}, nil)

	output, err := uc.Execute(context.Background(), refreshToken)
	assert.NoError(t, err)
	assert.NotNil(t, output)
	assert.NotEmpty(t, output.AccessToken)
	assert.NotEmpty(t, output.RefreshToken)

	userRepo.AssertExpectations(t)
	sessionRepo.AssertExpectations(t)
}

func TestRefreshTokenUseCase_ExpiredSession(t *testing.T) {
	userRepo := new(MockUserRepo)
	sessionRepo := new(MockSessionRepo)
	signer := newTestSigner(t)
	logger := zerolog.Nop()

	uc := NewRefreshTokenUseCase(userRepo, sessionRepo, signer, logger)

	refreshToken := "rt_expired"
	session := &model.Session{
		ID:        uuid.New(),
		UserID:    uuid.New(),
		ExpiresAt: time.Now().Add(-24 * time.Hour),
		Revoked:   false,
	}

	sessionRepo.On("FindByRefreshToken", mock.Anything, refreshToken).Return(session, nil)

	output, err := uc.Execute(context.Background(), refreshToken)
	assert.ErrorIs(t, err, ErrSessionExpired)
	assert.Nil(t, output)
}

func TestRefreshTokenUseCase_RevokedSession(t *testing.T) {
	userRepo := new(MockUserRepo)
	sessionRepo := new(MockSessionRepo)
	signer := newTestSigner(t)
	logger := zerolog.Nop()

	uc := NewRefreshTokenUseCase(userRepo, sessionRepo, signer, logger)

	refreshToken := "rt_revoked"
	session := &model.Session{
		ID:        uuid.New(),
		UserID:    uuid.New(),
		ExpiresAt: time.Now().Add(24 * time.Hour),
		Revoked:   true,
	}

	sessionRepo.On("FindByRefreshToken", mock.Anything, refreshToken).Return(session, nil)

	output, err := uc.Execute(context.Background(), refreshToken)
	assert.ErrorIs(t, err, ErrSessionRevoked)
	assert.Nil(t, output)
}

func TestRefreshTokenUseCase_TokenTheft(t *testing.T) {
	userRepo := new(MockUserRepo)
	sessionRepo := new(MockSessionRepo)
	signer := newTestSigner(t)
	logger := zerolog.Nop()

	uc := NewRefreshTokenUseCase(userRepo, sessionRepo, signer, logger)

	userID := uuid.New()
	sessionID := uuid.New()
	refreshToken := "rt_stolen"

	session := &model.Session{
		ID:        sessionID,
		UserID:    userID,
		ExpiresAt: time.Now().Add(24 * time.Hour),
		Revoked:   false,
	}
	user := &model.User{
		ID:    userID,
		Email: "alice@example.com",
		Name:  "Alice",
	}

	sessionRepo.On("FindByRefreshToken", mock.Anything, refreshToken).Return(session, nil)
	userRepo.On("FindByID", mock.Anything, userID).Return(user, nil)
	sessionRepo.On("Rotate", mock.Anything, sessionID, mock.AnythingOfType("*model.Session")).
		Return(nil, repository.ErrTokenReuse)

	output, err := uc.Execute(context.Background(), refreshToken)
	assert.ErrorIs(t, err, ErrTokenTheftDetected)
	assert.Nil(t, output)
}

func TestRefreshTokenUseCase_InvalidToken(t *testing.T) {
	userRepo := new(MockUserRepo)
	sessionRepo := new(MockSessionRepo)
	signer := newTestSigner(t)
	logger := zerolog.Nop()

	uc := NewRefreshTokenUseCase(userRepo, sessionRepo, signer, logger)

	sessionRepo.On("FindByRefreshToken", mock.Anything, "invalid").
		Return(nil, repository.ErrNotFound)

	output, err := uc.Execute(context.Background(), "invalid")
	assert.ErrorIs(t, err, ErrInvalidCredentials)
	assert.Nil(t, output)
}
